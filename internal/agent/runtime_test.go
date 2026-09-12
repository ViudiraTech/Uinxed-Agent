package agent

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

type memStore struct {
	mu sync.Mutex
	s  map[string]domain.Session
}

func newMemStore() *memStore { return &memStore{s: map[string]domain.Session{}} }
func (m *memStore) ListSessions(context.Context) ([]domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Session, 0, len(m.s))
	for _, s := range m.s {
		out = append(out, s.Clone())
	}
	return out, nil
}
func (m *memStore) SearchSessions(ctx context.Context, q string, limit int) ([]domain.Session, error) {
	ss, _ := m.ListSessions(ctx)
	var out []domain.Session
	for _, s := range ss {
		if strings.Contains(s.Name, q) {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *memStore) LoadSession(_ context.Context, id string) (domain.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.s[id]
	if !ok {
		return domain.Session{}, sql.ErrNoRows
	}
	return s.Clone(), nil
}
func (m *memStore) SaveSession(_ context.Context, s domain.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s[s.ID] = s.Clone()
	return nil
}
func (m *memStore) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.s, id)
	return nil
}
func (m *memStore) DeleteAllSessions(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s = map[string]domain.Session{}
	return nil
}
func (m *memStore) Close() error { return nil }

type scriptedProvider struct {
	mu       sync.Mutex
	calls    int
	requests []provider.Request
	mode     string
}

func (p *scriptedProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.requests = append(p.requests, req)
	mode := p.mode
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
		if mode == "tool" && n == 1 {
			send(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{Index: 0, ID: "c1", Type: "function", Function: domain.ToolCallFunction{Name: "calc", Arguments: `{"expr":"1+1"}`}}}})
			send(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		send(provider.Event{Kind: provider.EventContent, Text: "done"})
		send(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*scriptedProvider) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*scriptedProvider) CheckKey(context.Context, string) error   { return nil }

func waitFinished(t *testing.T, r *Runtime, session string) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e := <-r.Events():
			if e.Kind == domain.EventError {
				if d, ok := e.Data.(domain.ErrorData); ok {
					t.Fatalf("runtime error: %s", d.Message)
				}
			}
			if e.Kind == domain.EventAgentFinished && e.SessionID == session {
				return
			}
		case <-timer.C:
			t.Fatal("timed out waiting for runtime")
		}
	}
}

func TestRuntimeExecutesToolRoundAndPersists(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(), ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir()}
	_ = st.SaveSession(context.Background(), sess)
	fp := &scriptedProvider{mode: "tool"}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return fp, nil })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "calculate"); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, "s")
	got, err := st.LoadSession(context.Background(), "s")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) < 4 {
		t.Fatalf("messages=%#v", got.Messages)
	}
	var sawTool, sawFinal bool
	for _, m := range got.Messages {
		if m.Role == domain.RoleTool && strings.Contains(m.Content, "2") {
			sawTool = true
		}
		if m.Role == domain.RoleAssistant && m.Content == "done" {
			sawFinal = true
		}
	}
	if !sawTool || !sawFinal {
		t.Fatalf("tool=%v final=%v messages=%#v", sawTool, sawFinal, got.Messages)
	}
}

func TestFileReferenceIsEphemeralNotPersisted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("reference payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(), ProviderID: "p", Model: "test", AgentID: "build", CWD: root}
	_ = st.SaveSession(context.Background(), sess)
	fp := &scriptedProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return fp, nil })
	defer r.Close()
	prompt := "inspect @README.md please"
	if _, err := r.StartTurn(context.Background(), "s", prompt); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, "s")
	got, _ := st.LoadSession(context.Background(), "s")
	if len(got.Messages) == 0 || got.Messages[0].Content != prompt {
		t.Fatalf("persisted prompt=%#v", got.Messages)
	}
	for _, m := range got.Messages {
		if strings.Contains(m.Content, "reference payload") {
			t.Fatal("referenced file content leaked into persisted conversation")
		}
	}
	fp.mu.Lock()
	defer fp.mu.Unlock()
	if len(fp.requests) == 0 {
		t.Fatal("provider got no request")
	}
	joined := ""
	for _, m := range fp.requests[0].Messages {
		joined += "\n" + m.Content
	}
	if !strings.Contains(joined, "reference payload") || !strings.Contains(joined, "<referenced_file") {
		t.Fatalf("reference missing from provider request: %s", joined)
	}
}

func TestRuntimeRejectsDuplicateRun(t *testing.T) {
	// Use a provider that never completes until cancelled.
	st := newMemStore()
	_ = st.SaveSession(context.Background(), domain.Session{ID: "s", Name: "s", ProviderID: "p", Model: "x", AgentID: "build", CWD: t.TempDir(), CreatedAt: time.Now(), UpdatedAt: time.Now()})
	p := blockingProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.StartTurn(context.Background(), "s", "two"); err == nil {
		t.Fatal("duplicate run accepted")
	}
	if !r.Cancel("s") {
		t.Fatal("cancel returned false")
	}
}

type blockingProvider struct{}

func (blockingProvider) Stream(ctx context.Context, _ provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event)
	go func() { <-ctx.Done(); close(ch) }()
	return ch, nil
}
func (blockingProvider) Models(context.Context) ([]string, error) { return nil, nil }
func (blockingProvider) CheckKey(context.Context, string) error   { return nil }

var _ = errors.Is

func TestRuntimeAlwaysFinishesLifecycleOnProviderSetupError(t *testing.T) {
	st := newMemStore()
	_ = st.SaveSession(context.Background(), domain.Session{ID: "s", Name: "s", ProviderID: "missing", Model: "x", AgentID: "build", CWD: t.TempDir(), CreatedAt: time.Now(), UpdatedAt: time.Now()})
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return nil, errors.New("provider setup failed") })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "hello"); err != nil {
		t.Fatal(err)
	}
	var started, failed, finished bool
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for !finished {
		select {
		case e := <-r.Events():
			switch e.Kind {
			case domain.EventAgentStarted:
				started = true
			case domain.EventError:
				failed = true
			case domain.EventAgentFinished:
				if a, ok := e.Data.(domain.AgentEvent); !ok || a.Run.State != "failed" {
					t.Fatalf("finish=%#v", e.Data)
				}
				finished = true
			}
		case <-timer.C:
			t.Fatal("missing terminal lifecycle event")
		}
	}
	if !started || !failed {
		t.Fatalf("started=%v failed=%v finished=%v", started, failed, finished)
	}
}

func TestRuntimeCloseWaitsForActiveTurnAndRejectsNewRuns(t *testing.T) {
	st := newMemStore()
	_ = st.SaveSession(context.Background(), domain.Session{ID: "s", Name: "s", ProviderID: "p", Model: "x", AgentID: "build", CWD: t.TempDir(), CreatedAt: time.Now(), UpdatedAt: time.Now()})
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return blockingProvider{}, nil })
	if _, err := r.StartTurn(context.Background(), "s", "one"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { r.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not wait for/cancel active turn")
	}
	if r.IsRunningAny() {
		t.Fatal("runtime still reports active runs after Close")
	}
	if _, err := r.StartTurn(context.Background(), "s", "two"); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("StartTurn after Close err=%v", err)
	}
}

func TestCancelAndWaitWaitsForSessionRun(t *testing.T) {
	st := newMemStore()
	_ = st.SaveSession(context.Background(), domain.Session{ID: "s", Name: "s", ProviderID: "p", Model: "x", AgentID: "build", CWD: t.TempDir(), CreatedAt: time.Now(), UpdatedAt: time.Now()})
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return blockingProvider{}, nil })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "one"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cancelled, err := r.CancelAndWait(ctx, "s")
	if err != nil {
		t.Fatal(err)
	}
	if !cancelled || r.IsRunningAny() {
		t.Fatalf("cancelled=%v running=%v", cancelled, r.IsRunningAny())
	}
}

func TestTerminalEventDeliveryCannotBlockClose(t *testing.T) {
	st := newMemStore()
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return blockingProvider{}, nil })
	// Saturate the event queue and verify bounded terminal delivery still lets
	// Close finish. This simulates a TUI consumer that has already stopped.
	for i := 0; i < cap(r.events); i++ {
		r.events <- domain.Event{Kind: domain.EventStatus}
	}
	start := time.Now()
	r.emitTerminal(domain.Event{Kind: domain.EventAgentFinished})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("terminal delivery blocked too long: %v", elapsed)
	}
	r.Close()
}

// TestRuntimePublishesEachRoundSeparately guards the live transcript. One turn
// spans several model rounds, and the UI needs a boundary event per round so it
// can settle that round's text and tool calls. Without it every round is
// concatenated into a single streaming blob and the tool calls in between are
// never shown.
func TestRuntimePublishesEachRoundSeparately(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s", Name: "s", CreatedAt: time.Now(), UpdatedAt: time.Now(), ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir()}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return &scriptedProvider{mode: "tool"}, nil })
	defer r.Close()
	if _, err := r.StartTurn(context.Background(), "s", "calculate"); err != nil {
		t.Fatal(err)
	}

	var published []domain.Message
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	finished := false
	for !finished {
		select {
		case e := <-r.Events():
			switch e.Kind {
			case domain.EventMessageAdded:
				if msg, ok := e.Data.(domain.Message); ok {
					published = append(published, msg)
				}
			case domain.EventAgentFinished:
				finished = true
			case domain.EventError:
				if d, ok := e.Data.(domain.ErrorData); ok {
					t.Fatalf("runtime error: %s", d.Message)
				}
			}
		case <-timeout.C:
			t.Fatal("timed out waiting for the turn to finish")
		}
	}

	if len(published) != 3 {
		t.Fatalf("expected user turn + two model rounds, got %d: %#v", len(published), published)
	}
	if published[0].Role != domain.RoleUser || published[0].Content != "calculate" {
		t.Errorf("first published message should be the user turn: %#v", published[0])
	}
	if published[1].Role != domain.RoleAssistant || len(published[1].ToolCalls) != 1 {
		t.Errorf("second should be the round that called a tool: %#v", published[1])
	}
	if published[2].Role != domain.RoleAssistant || published[2].Content != "done" {
		t.Errorf("third should be the final answer: %#v", published[2])
	}
}

func TestToolAccumulatorHandlesOffsetIndicesWithoutGhostPadding(t *testing.T) {
	var acc toolAccumulator
	// Simulate streaming tool calls starting at Index = 2 (e.g. from Claude adapter)
	acc.Add([]domain.ToolCall{
		{Index: 2, ID: "call_web", Type: "function", Function: domain.ToolCallFunction{Name: "web_search"}},
	})
	acc.Add([]domain.ToolCall{
		{Index: 2, Function: domain.ToolCallFunction{Arguments: `{"query":"test"}`}},
	})
	acc.Add([]domain.ToolCall{
		{Index: 3, ID: "call_skill", Type: "function", Function: domain.ToolCallFunction{Name: "use_skill"}},
	})
	acc.Add([]domain.ToolCall{
		{Index: 3, Function: domain.ToolCallFunction{Arguments: `{"skill":"core"}`}},
	})

	calls := acc.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %#v", len(calls), calls)
	}
	if calls[0].Function.Name != "web_search" || calls[0].ID != "call_web" || calls[0].Function.Arguments != `{"query":"test"}` {
		t.Errorf("call 0 mismatch: %#v", calls[0])
	}
	if calls[1].Function.Name != "use_skill" || calls[1].ID != "call_skill" || calls[1].Function.Arguments != `{"skill":"core"}` {
		t.Errorf("call 1 mismatch: %#v", calls[1])
	}
}

func TestRuntimeExecutesToolRoundWithOffsetIndices(t *testing.T) {
	st := newMemStore()
	sess := domain.Session{ID: "s-offset", Name: "s-offset", CreatedAt: time.Now(), UpdatedAt: time.Now(), ProviderID: "p", Model: "test", AgentID: "build", CWD: t.TempDir()}
	_ = st.SaveSession(context.Background(), sess)

	// Scripted provider emits tool calls starting with Index = 2
	fp := &scriptedOffsetProvider{}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return fp, nil })
	defer r.Close()

	if _, err := r.StartTurn(context.Background(), sess.ID, "calculate offset"); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, r, sess.ID)

	got, err := st.LoadSession(context.Background(), sess.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify no ghost tool calls and no nameless tool results
	for i, m := range got.Messages {
		if m.Role == domain.RoleAssistant {
			for _, tc := range m.ToolCalls {
				if tc.Function.Name == "" {
					t.Errorf("msg[%d] assistant has nameless tool call: %#v", i, tc)
				}
			}
		}
		if m.Role == domain.RoleTool {
			if m.ToolCallID == "" {
				t.Errorf("msg[%d] tool message has empty tool_call_id: %#v", i, m)
			}
			if strings.Contains(m.Content, "unknown tool") {
				t.Errorf("msg[%d] tool message reports unknown tool error: %#v", i, m)
			}
		}
	}
}

type scriptedOffsetProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *scriptedOffsetProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
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
		if n == 1 {
			// Offset index = 2
			send(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{Index: 2, ID: "call_offset_2", Type: "function", Function: domain.ToolCallFunction{Name: "calc", Arguments: `{"expr":"2+2"}`}}}})
			send(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		send(provider.Event{Kind: provider.EventContent, Text: "offset done"})
		send(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*scriptedOffsetProvider) Models(context.Context) ([]string, error) {
	return []string{"test"}, nil
}
func (*scriptedOffsetProvider) CheckKey(context.Context, string) error { return nil }
