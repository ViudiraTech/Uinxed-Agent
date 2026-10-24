package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// The Responses API emits every tool call in one event and leaves Index at its
// zero value. Two parallel calls must stay two calls: the index fallback is only
// for deltas that carry no ID.
func TestParallelToolCallsStaySeparate(t *testing.T) {
	var acc toolAccumulator
	acc.Add([]domain.ToolCall{{
		Index: 0, ID: "call_A", Type: "function",
		Function: domain.ToolCallFunction{Name: "read_file", Arguments: `{"path":"a.go"}`},
	}})
	acc.Add([]domain.ToolCall{{
		Index: 0, ID: "call_B", Type: "function",
		Function: domain.ToolCallFunction{Name: "grep", Arguments: `{"pattern":"x"}`},
	}})

	got := acc.Calls()
	if len(got) != 2 {
		t.Fatalf("got %d calls, want 2: %#v", len(got), got)
	}
	if got[0].ID != "call_A" || got[0].Function.Name != "read_file" {
		t.Fatalf("first call corrupted: %#v", got[0])
	}
	if got[1].ID != "call_B" || got[1].Function.Name != "grep" {
		t.Fatalf("second call corrupted: %#v", got[1])
	}
}

// Argument deltas carry no ID, so they must still merge by index.
func TestToolArgumentDeltasMergeByIndex(t *testing.T) {
	var acc toolAccumulator
	acc.Add([]domain.ToolCall{{
		Index: 1, ID: "call_A", Type: "function",
		Function: domain.ToolCallFunction{Name: "read_file"},
	}})
	acc.Add([]domain.ToolCall{{Index: 1, Function: domain.ToolCallFunction{Arguments: `{"path":`}}})
	acc.Add([]domain.ToolCall{{Index: 1, Function: domain.ToolCallFunction{Arguments: `"a.go"}`}}})

	got := acc.Calls()
	if len(got) != 1 {
		t.Fatalf("got %d calls, want 1: %#v", len(got), got)
	}
	if got[0].Function.Arguments != `{"path":"a.go"}` {
		t.Fatalf("arguments = %q", got[0].Function.Arguments)
	}
}

// gatedProvider blocks the first round until the test releases it, so a change
// made "mid-turn" is genuinely after the turn loaded the session.
type gatedProvider struct {
	started chan struct{}
	gate    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   int
	write   bool
}

func (p *gatedProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	p.once.Do(func() { close(p.started) })
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		emit := func(e provider.Event) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if n == 1 {
			select {
			case <-p.gate:
			case <-ctx.Done():
				return
			}
			if p.write {
				emit(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
					Index: 0, ID: "c-write", Type: "function",
					Function: domain.ToolCallFunction{Name: "write_file", Arguments: `{"path":"out.txt","content":"hi"}`},
				}}})
				emit(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
				return
			}
		}
		emit(provider.Event{Kind: provider.EventContent, Text: "done"})
		emit(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*gatedProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*gatedProvider) CheckKey(context.Context, string) error   { return nil }

func newGatedRuntime(t *testing.T, mode string, write bool) (*Runtime, *gatedProvider, *memStore) {
	t.Helper()
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": mode}}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	p := &gatedProvider{started: make(chan struct{}), gate: make(chan struct{}), write: write}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	t.Cleanup(r.Close)
	if _, err := r.StartTurn(context.Background(), "s", "go"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.started:
	case <-time.After(3 * time.Second):
		t.Fatal("provider never called")
	}
	return r, p, st
}

// Tightening the approval mode while a turn runs has to apply to that turn. A
// safety control that only lands on the next turn is not a control. A file write
// is the discriminating call: auto-edit runs it silently, read-only asks first.
func TestModeChangeMidTurnAppliesToTheTurn(t *testing.T) {
	r, p, st := newGatedRuntime(t, "auto-edit", true)
	// Exactly what Controller.SetApprovalMode writes.
	if err := st.UpdateSettings(context.Background(), "s", func(s *domain.Session) {
		if s.Metadata == nil {
			s.Metadata = map[string]any{}
		}
		s.Metadata["mode"] = "read-only"
	}); err != nil {
		t.Fatal(err)
	}
	close(p.gate)

	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-r.Events():
			switch e.Kind {
			case domain.EventApprovalRequested:
				// Correct: the tightened mode now gates the write.
				if req, ok := e.Data.(domain.ApprovalRequest); ok {
					r.ResolveApproval(e.RunID, req.ID, Decision{Allow: false})
				}
				waitFinished(t, r, "s")
				return
			case domain.EventToolStarted:
				t.Fatal("the write ran without a prompt after the turn was switched to read-only")
			case domain.EventAgentFinished:
				t.Fatal("the turn finished without the read-only prompt the mid-turn switch should have caused")
			}
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

// A setting written while the turn runs must survive the turn's own saves: the
// turn's session copy is stale by design, so only the keys it wrote may be
// applied over the stored settings.
func TestTurnSavesKeepMidTurnSettings(t *testing.T) {
	r, p, st := newGatedRuntime(t, "auto-edit", false)
	if err := st.UpdateSettings(context.Background(), "s", func(s *domain.Session) {
		if s.Metadata == nil {
			s.Metadata = map[string]any{}
		}
		s.Metadata["mode"] = "full-auto"
		s.Metadata["effort"] = "max"
	}); err != nil {
		t.Fatal(err)
	}
	close(p.gate)
	waitFinished(t, r, "s")

	got, err := st.LoadSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata["mode"] != "full-auto" || got.Metadata["effort"] != "max" {
		t.Fatalf("mid-turn settings were reverted: %v", got.Metadata)
	}
}

// delegateChildProvider: round 1 asks for a delegate, round 2 (the child) blocks
// until cancelled.
type delegateChildProvider struct {
	started chan struct{}
	once    sync.Once
	mu      sync.Mutex
	calls   int
}

func (p *delegateChildProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		emit := func(e provider.Event) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if n == 1 {
			emit(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-delegate", Type: "function",
				Function: domain.ToolCallFunction{Name: "delegate", Arguments: `{"agent":"explorer","task":"look around"}`},
			}}})
			emit(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		if n == 2 {
			p.once.Do(func() { close(p.started) })
			<-ctx.Done()
			return
		}
		emit(provider.Event{Kind: provider.EventContent, Text: "done"})
		emit(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*delegateChildProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*delegateChildProvider) CheckKey(context.Context, string) error   { return nil }

// A delegate child owns a live session and a running loop, so it must hold the
// per-session run slot: Cancel and CancelAndWait are how Esc and DeleteSession
// stop it.
func TestDelegateChildCanBeCancelled(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "full-auto"}}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	p := &delegateChildProvider{started: make(chan struct{})}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	t.Cleanup(r.Close)

	if _, err := r.StartTurn(context.Background(), "s", "go delegate"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the delegate child never started")
	}

	list, err := st.ListSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	child := ""
	for _, s := range list {
		if s.ParentID == "s" {
			child = s.ID
		}
	}
	if child == "" {
		t.Fatal("no child session was persisted")
	}
	if !r.Cancel(child) {
		t.Fatal("Cancel on a running delegate child returned false")
	}
	waitFinished(t, r, "s")
}

// commandAliasProvider asks for one bash call spelled the way Claude Code
// spells it — `command` plus a `description` — then answers.
type commandAliasProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *commandAliasProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		emit := func(e provider.Event) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if n == 1 {
			emit(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-bash", Type: "function",
				Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"command":"echo alias-ran","description":"say it"}`},
			}}})
			emit(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		emit(provider.Event{Kind: provider.EventContent, Text: "done"})
		emit(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*commandAliasProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*commandAliasProvider) CheckKey(context.Context, string) error   { return nil }

// A bash call that names the command `command` must run, not fail the turn with
// `invalid tool arguments: json: unknown field "command"`. The model has no way
// to learn the house spelling from a decoder error, so the tool layer accepts
// the schema it was trained on instead of making the user retry.
func TestBashCallSpelledCommandRuns(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "full-auto"}}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	p := &commandAliasProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	t.Cleanup(r.Close)

	if _, err := r.StartTurn(context.Background(), "s", "run it"); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, "s")

	got, err := st.LoadSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	var results []string
	for _, m := range got.Messages {
		if m.Role == domain.RoleTool {
			results = append(results, m.Content)
		}
	}
	out := strings.Join(results, "\n")
	if strings.Contains(out, "unknown field") {
		t.Fatalf("the alias was rejected again: %s", out)
	}
	if !strings.Contains(out, "alias-ran") {
		t.Fatalf("bash produced no output for the aliased command: %s", out)
	}
}
