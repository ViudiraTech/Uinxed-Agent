package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/approval"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// approvalProvider emits one bash tool call on the first round and replies with
// plain content afterwards. It records every request so a test can assert what
// the model received, including the structured denial.
type approvalProvider struct {
	mu       sync.Mutex
	requests []provider.Request
}

func (p *approvalProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	n := len(p.requests)
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		send := func(e provider.Event) bool {
			select {
			case ch <- e:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if n == 0 {
			send(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-bash", Type: "function",
				Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"echo approved-run"}`},
			}}})
			send(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		send(provider.Event{Kind: provider.EventContent, Text: "turn complete"})
		send(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*approvalProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*approvalProvider) CheckKey(context.Context, string) error   { return nil }

// newApprovalRuntime builds a runtime whose session is in read-only mode, so a
// bash call produces an approval prompt.
func newApprovalRuntime(t *testing.T) (*Runtime, *approvalProvider, *memStore, string) {
	t.Helper()
	st := newMemStore()
	sess := domain.Session{
		ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "read-only"},
	}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	p := &approvalProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	t.Cleanup(r.Close)
	return r, p, st, "s"
}

// approvalEvent waits for an approval request targeted at runID.
func approvalEvent(t *testing.T, r *Runtime, runID string) domain.ApprovalRequest {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e := <-r.Events():
			if e.Kind != domain.EventApprovalRequested {
				continue
			}
			req, ok := e.Data.(domain.ApprovalRequest)
			if !ok {
				t.Fatalf("approval event carried %#v", e.Data)
			}
			if e.RunID != runID {
				t.Fatalf("approval run id = %q, want %q", e.RunID, runID)
			}
			return req
		case <-deadline:
			t.Fatal("no approval prompt arrived")
			return domain.ApprovalRequest{}
		}
	}
}

func waitFinishedState(t *testing.T, r *Runtime, session string) domain.AgentRun {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-r.Events():
			if e.Kind == domain.EventAgentFinished && e.SessionID == session {
				a, ok := e.Data.(domain.AgentEvent)
				if !ok {
					t.Fatalf("finish data %#v", e.Data)
				}
				return a.Run
			}
		case <-deadline:
			t.Fatal("timed out waiting for the turn to finish")
			return domain.AgentRun{}
		}
	}
}

func startID(t *testing.T, r *Runtime, session, prompt string) string {
	t.Helper()
	runID, err := r.StartTurn(context.Background(), session, prompt)
	if err != nil {
		t.Fatal(err)
	}
	// Drain the AgentStarted event so later helpers can find prompts by RunID.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case e := <-r.Events():
			if e.Kind == domain.EventAgentStarted && e.SessionID == session {
				if a, ok := e.Data.(domain.AgentEvent); ok && a.Run.ID == runID {
					return runID
				}
			}
		case <-deadline:
			t.Fatal("AgentStarted event not observed")
			return runID
		}
	}
}

// TestRuntimeApprovalAllow runs the full loop: prompt presented, user allows,
// tool executes, model receives the real result.
func TestRuntimeApprovalAllow(t *testing.T) {
	r, _, st, sid := newApprovalRuntime(t)
	runID := startID(t, r, sid, "run a command")
	req := approvalEvent(t, r, runID)
	if req.ToolName != "bash" || req.Category != "shell" {
		t.Fatalf("prompt = %#v", req)
	}
	if !r.ResolveApproval(runID, req.ID, Decision{Allow: true}) {
		t.Fatal("ResolveApproval rejected a live request")
	}
	run := waitFinishedState(t, r, sid)
	if run.State != "done" {
		t.Fatalf("run state = %q", run.State)
	}
	got, _ := st.LoadSession(context.Background(), sid)
	var sawResult bool
	for _, m := range got.Messages {
		if m.Role == domain.RoleTool && strings.Contains(m.Content, "approved-run") {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatalf("tool result missing from transcript: %#v", got.Messages)
	}
}

// TestRuntimeApprovalDenyFeedsStructuredError pins the deny path: the model's
// next round receives {"error":"user denied: …"} as the tool result and no tool
// execution happened.
func TestRuntimeApprovalDenyFeedsStructuredError(t *testing.T) {
	r, p, st, sid := newApprovalRuntime(t)
	runID := startID(t, r, sid, "run a command")
	req := approvalEvent(t, r, runID)
	r.ResolveApproval(runID, req.ID, Decision{Allow: false, Reason: "do not run commands"})
	run := waitFinishedState(t, r, sid)
	if run.State != "done" {
		t.Fatalf("run state = %q (a denial must not fail the turn)", run.State)
	}
	got, _ := st.LoadSession(context.Background(), sid)
	var sawDenial bool
	for _, m := range got.Messages {
		if m.Role == domain.RoleTool && strings.Contains(m.Content, "user denied") && strings.Contains(m.Content, "do not run commands") {
			sawDenial = true
		}
	}
	if !sawDenial {
		t.Fatalf("structured denial missing from transcript: %#v", got.Messages)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) < 2 {
		t.Fatalf("model did not get a follow-up round: %d requests", len(p.requests))
	}
	var modelSawDenial bool
	for _, m := range p.requests[1].Messages {
		if m.Role == domain.RoleTool && strings.Contains(m.Content, `"error"`) && strings.Contains(m.Content, "user denied") {
			modelSawDenial = true
		}
	}
	if !modelSawDenial {
		t.Fatalf("model never received the denial: %#v", p.requests[1].Messages)
	}
}

// TestRuntimeApprovalCancelAbortsTurn covers Esc-during-prompt semantics at the
// runtime level: cancelling the run releases the waiter and the turn ends as
// cancelled, with no decision delivered.
func TestRuntimeApprovalCancelAbortsTurn(t *testing.T) {
	r, _, _, sid := newApprovalRuntime(t)
	runID := startID(t, r, sid, "run a command")
	req := approvalEvent(t, r, runID)
	if !r.Cancel(sid) {
		t.Fatal("cancel reported no active run")
	}
	run := waitFinishedState(t, r, sid)
	if run.State != "cancelled" {
		t.Fatalf("run state = %q, want cancelled", run.State)
	}
	if r.ResolveApproval(runID, req.ID, Decision{Allow: true}) {
		t.Fatal("stale prompt remained resolvable after cancellation")
	}
}

// TestRuntimePlanModeDeniesWithoutPrompt pins plan mode's defining property
// end-to-end: a write call is refused with no approval event, the tool result
// explains the refusal, and the system prompt carries the plan instructions.
func TestRuntimePlanModeDeniesWithoutPrompt(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{
		ID: "s-plan", Name: "s-plan", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "plan"},
	}
	_ = st.SaveSession(context.Background(), sess)
	p := &approvalProvider2{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	defer r.Close()
	runID := startID(t, r, "s-plan", "write something")
	run := waitFinishedState(t, r, "s-plan")
	if run.State != "done" {
		t.Fatalf("run state = %q", run.State)
	}
	_ = runID
	// No approval event may exist: the loop above would have surfaced one only
	// after AgentFinished, which cannot happen for a denied-only round.
	select {
	case e := <-r.Events():
		if e.Kind == domain.EventApprovalRequested {
			t.Fatalf("plan mode produced an approval prompt: %#v", e)
		}
	case <-time.After(100 * time.Millisecond):
	}
	got, _ := st.LoadSession(context.Background(), "s-plan")
	var sawDenial bool
	for _, m := range got.Messages {
		if m.Role == domain.RoleTool && strings.Contains(m.Content, "plan mode is read-only") {
			sawDenial = true
		}
	}
	if !sawDenial {
		t.Fatalf("plan-mode denial missing from transcript: %#v", got.Messages)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	found := false
	for _, m := range p.requests[0].Messages {
		if m.Role == domain.RoleSystem && strings.Contains(m.Content, "Plan mode") {
			found = true
		}
	}
	if !found {
		t.Fatal("plan instructions missing from the system prompt")
	}
}

// approvalProvider2 emits a write_file call, then finishes.
type approvalProvider2 struct {
	mu       sync.Mutex
	requests []provider.Request
}

func (p *approvalProvider2) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	n := len(p.requests)
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		if n == 0 {
			ch <- provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-write", Type: "function",
				Function: domain.ToolCallFunction{Name: "write_file", Arguments: `{"path":"x.txt","content":"hi"}`},
			}}}
			ch <- provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"}
			return
		}
		ch <- provider.Event{Kind: provider.EventContent, Text: "here is the plan"}
		ch <- provider.Event{Kind: provider.EventDone, FinishReason: "stop"}
	}()
	return ch, nil
}
func (*approvalProvider2) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*approvalProvider2) CheckKey(context.Context, string) error   { return nil }

// TestRuntimeSessionGrantSkipsPrompt covers the "always allow" fast path inside
// a turn: after one grant, a second bash call in a later round does not prompt.
func TestRuntimeSessionGrantSkipsPrompt(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{
		ID: "s-g", Name: "s-g", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir(),
		Metadata: map[string]any{"mode": "read-only"},
	}
	_ = st.SaveSession(context.Background(), sess)
	p := &multiCallProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	defer r.Close()
	startID(t, r, "s-g", "run two commands")

	prompts := 0
	deadline := time.After(5 * time.Second)
	resolved := map[string]bool{}
	for {
		select {
		case e := <-r.Events():
			switch ev := e.Data.(type) {
			case domain.ApprovalRequest:
				prompts++
				if prompts > 1 {
					t.Fatal("second bash call prompted despite the session grant")
				}
				r.ResolveApproval(e.RunID, ev.ID, Decision{Allow: true, Always: true})
			case domain.AgentEvent:
				if ev.Run.State == "done" && ev.Run.SessionID == "s-g" {
					if prompts != 1 {
						t.Fatalf("prompts=%d, want exactly 1", prompts)
					}
					return
				}
			}
		case <-deadline:
			t.Fatalf("turn did not finish; prompts=%d resolved=%v", prompts, resolved)
		}
	}
}

// multiCallProvider issues two bash calls across two rounds.
type multiCallProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *multiCallProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	n := p.calls
	p.calls++
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		if n < 2 {
			ch <- provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: string(rune('x' + n)), Type: "function",
				Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"echo round"}`},
			}}}
			ch <- provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"}
			return
		}
		ch <- provider.Event{Kind: provider.EventContent, Text: "all done"}
		ch <- provider.Event{Kind: provider.EventDone, FinishReason: "stop"}
	}()
	return ch, nil
}
func (*multiCallProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*multiCallProvider) CheckKey(context.Context, string) error   { return nil }

// TestRuntimeRootSessionWalk verifies the grant-inheritance root walk.
func TestRuntimeRootSessionWalk(t *testing.T) {
	st := newMemStore()
	ctx := context.Background()
	_ = st.SaveSession(ctx, domain.Session{ID: "root", Name: "root", ProviderID: "p", Model: "m", AgentID: "build", CWD: ".", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = st.SaveSession(ctx, domain.Session{ID: "child", ParentID: "root", Name: "child", ProviderID: "p", Model: "m", AgentID: "general", CWD: ".", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = st.SaveSession(ctx, domain.Session{ID: "grandchild", ParentID: "child", Name: "gc", ProviderID: "p", Model: "m", AgentID: "general", CWD: ".", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return nil, nil })
	defer r.Close()
	for _, c := range []struct{ in, want string }{{"root", "root"}, {"child", "root"}, {"grandchild", "root"}, {"missing", "missing"}} {
		if got := r.rootSessionID(ctx, c.in); got != c.want {
			t.Errorf("rootSessionID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestModeFromMetadata checks the session-metadata fallback chain.
func TestModeFromMetadata(t *testing.T) {
	if got := modeFromMetadata(map[string]any{"mode": "plan"}, approval.ModeAutoEdit); got != approval.ModePlan {
		t.Errorf("explicit mode = %q", got)
	}
	if got := modeFromMetadata(map[string]any{}, approval.ModeFullAuto); got != approval.ModeFullAuto {
		t.Errorf("fallback mode = %q", got)
	}
	if got := modeFromMetadata(map[string]any{"mode": "corrupt"}, approval.ModeReadOnly); got != approval.DefaultMode {
		t.Errorf("corrupt mode should normalize to the default (as config.validate does), got %q", got)
	}
}
