package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
	"github.com/ViudiraTech/Uinxed-Agent/internal/provider"
)

// toolCallScript emits one call of a single tool on the first model round of
// every turn (request indices alternate call, content, call, content, …) and
// plain content afterwards. Every request is recorded so a test can assert what
// the model received (including structured denial results).
type toolCallScript struct {
	mu       sync.Mutex
	requests []provider.Request
	tool     string
	args     string
}

func (p *toolCallScript) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	n := len(p.requests)
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	ch := make(chan provider.Event, 8)
	go func() {
		defer close(ch)
		send := func(e provider.Event) {
			select {
			case ch <- e:
			case <-ctx.Done():
			}
		}
		if n%2 == 0 {
			send(provider.Event{Kind: provider.EventToolCall, ToolCalls: []domain.ToolCall{{
				Index: 0, ID: "c-1", Type: "function",
				Function: domain.ToolCallFunction{Name: p.tool, Arguments: p.args},
			}}})
			send(provider.Event{Kind: provider.EventDone, FinishReason: "tool_calls"})
			return
		}
		send(provider.Event{Kind: provider.EventContent, Text: "done"})
		send(provider.Event{Kind: provider.EventDone, FinishReason: "stop"})
	}()
	return ch, nil
}
func (*toolCallScript) Models(context.Context) ([]string, error) { return []string{"test"}, nil }
func (*toolCallScript) CheckKey(context.Context, string) error   { return nil }

// startSession seeds a session with the given agent/mode and returns a runtime
// driven by p. The events channel is left open for the caller to drain.
func startSession(t *testing.T, id, agent, mode string, p provider.Provider) (*Runtime, *memStore) {
	t.Helper()
	st := newMemStore()
	sess := domain.Session{
		ID: id, Name: id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ProviderID: "p", Model: "test", AgentID: agent, CWD: t.TempDir(),
		Metadata: map[string]any{"mode": mode},
	}
	if err := st.SaveSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(st, nil, func(string) (provider.Provider, error) { return p, nil })
	t.Cleanup(r.Close)
	return r, st
}

// finishedWithoutPrompts drains events until the session's turn finishes and
// fails if an approval prompt appears on the way.
func finishedWithoutPrompts(t *testing.T, r *Runtime, session string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case e := <-r.Events():
			if e.Kind == domain.EventApprovalRequested {
				t.Fatalf("unexpected approval prompt: %#v", e.Data)
			}
			if e.Kind == domain.EventAgentFinished && e.SessionID == session {
				return
			}
		case <-deadline:
			t.Fatal("turn did not finish")
		}
	}
}

// lastToolResult returns the content of the final tool-role message, which is
// what the model saw for its last call.
func lastToolResult(t *testing.T, st *memStore, session string) string {
	t.Helper()
	got, err := st.LoadSession(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(got.Messages) - 1; i >= 0; i-- {
		if got.Messages[i].Role == domain.RoleTool {
			return got.Messages[i].Content
		}
	}
	t.Fatalf("no tool message in transcript: %#v", got.Messages)
	return ""
}

// TestRuntimePlanWritePersistsPlan pins plan mode's new tool: plan_write is
// allowed without prompting and its steps land in session metadata, from which
// the /plan overlay reads them.
func TestRuntimePlanWritePersistsPlan(t *testing.T) {
	p := &toolCallScript{tool: "plan_write", args: `{"steps":[{"subject":"Add plan tool","details":"runtime wiring","status":"in_progress"},{"subject":"Ship it"}]}`}
	r, st := startSession(t, "s-planwrite", "plan", "plan", p)
	startID(t, r, "s-planwrite", "plan it")
	finishedWithoutPrompts(t, r, "s-planwrite")

	got, err := st.LoadSession(context.Background(), "s-planwrite")
	if err != nil {
		t.Fatal(err)
	}
	steps := domain.PlanFromMetadata(got.Metadata)
	if len(steps) != 2 {
		t.Fatalf("plan steps = %d, want 2 (metadata=%#v)", len(steps), got.Metadata["plan"])
	}
	if steps[0].Subject != "Add plan tool" || steps[0].Details != "runtime wiring" || steps[0].Status != domain.TodoInProgress {
		t.Errorf("step 1 = %#v", steps[0])
	}
	if steps[1].Subject != "Ship it" || steps[1].Status != domain.TodoPending {
		t.Errorf("step 2 = %#v", steps[1])
	}
}

// TestRuntimePlanWriteDeniedOutsidePlan pins the plan-mode gate: in any other
// mode the call is refused with an explanatory denial and no prompt appears.
func TestRuntimePlanWriteDeniedOutsidePlan(t *testing.T) {
	p := &toolCallScript{tool: "plan_write", args: `{"steps":[{"subject":"nope"}]}`}
	r, st := startSession(t, "s-notplan", "build", "auto-edit", p)
	startID(t, r, "s-notplan", "try to plan")
	finishedWithoutPrompts(t, r, "s-notplan")

	if res := lastToolResult(t, st, "s-notplan"); !strings.Contains(res, "plan_write is only available in plan mode") {
		t.Fatalf("model did not see the plan-mode denial: %s", res)
	}
	got, _ := st.LoadSession(context.Background(), "s-notplan")
	if len(domain.PlanFromMetadata(got.Metadata)) != 0 {
		t.Fatal("plan was persisted outside plan mode")
	}
}

// TestRuntimeSwitchModeExitsPlanWithoutPrompt pins the user-settled exception:
// leaving plan mode is applied immediately, with no approval round-trip.
func TestRuntimeSwitchModeExitsPlanWithoutPrompt(t *testing.T) {
	p := &toolCallScript{tool: "switch_mode", args: `{"mode":"auto-edit","reason":"ready to implement"}`}
	r, st := startSession(t, "s-exit", "plan", "plan", p)
	startID(t, r, "s-exit", "wrap up")
	finishedWithoutPrompts(t, r, "s-exit")

	got, err := st.LoadSession(context.Background(), "s-exit")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := got.Metadata["mode"].(string); v != "auto-edit" {
		t.Fatalf("mode after exit = %q, want auto-edit", v)
	}
}

// TestRuntimeSwitchModeIntoPlanWithoutPrompt pins the other half of the
// exception: entering plan mode is also frictionless.
func TestRuntimeSwitchModeIntoPlanWithoutPrompt(t *testing.T) {
	p := &toolCallScript{tool: "switch_mode", args: `{"mode":"plan"}`}
	r, st := startSession(t, "s-enter", "build", "auto-edit", p)
	startID(t, r, "s-enter", "need a plan first")
	finishedWithoutPrompts(t, r, "s-enter")

	got, _ := st.LoadSession(context.Background(), "s-enter")
	if v, _ := got.Metadata["mode"].(string); v != "plan" {
		t.Fatalf("mode after enter = %q, want plan", v)
	}
}

// TestRuntimeSwitchModeSidewaysRequiresConsent pins that every other switch is
// gated on the user. The approve path applies the new mode; the deny path
// leaves it untouched and feeds the model a structured denial.
func TestRuntimeSwitchModeSidewaysRequiresConsent(t *testing.T) {
	t.Run("allow applies", func(t *testing.T) {
		p := &toolCallScript{tool: "switch_mode", args: `{"mode":"full-auto"}`}
		r, st := startSession(t, "s-allow", "build", "read-only", p)
		runID := startID(t, r, "s-allow", "escalate")
		req := approvalEvent(t, r, runID)
		if req.ToolName != "switch_mode" {
			t.Fatalf("tool name = %q", req.ToolName)
		}
		if !strings.Contains(req.Summary, "full-auto") {
			t.Fatalf("summary should name the target mode, got %q", req.Summary)
		}
		r.ResolveApproval(runID, req.ID, Decision{Allow: true})
		finishedWithoutPrompts(t, r, "s-allow")
		got, _ := st.LoadSession(context.Background(), "s-allow")
		if v, _ := got.Metadata["mode"].(string); v != "full-auto" {
			t.Fatalf("mode after allowed switch = %q, want full-auto", v)
		}
	})
	t.Run("deny refuses", func(t *testing.T) {
		p := &toolCallScript{tool: "switch_mode", args: `{"mode":"full-auto"}`}
		r, st := startSession(t, "s-deny", "build", "read-only", p)
		runID := startID(t, r, "s-deny", "escalate")
		req := approvalEvent(t, r, runID)
		r.ResolveApproval(runID, req.ID, Decision{Allow: false, Reason: "stay cautious"})
		finishedWithoutPrompts(t, r, "s-deny")
		got, _ := st.LoadSession(context.Background(), "s-deny")
		if v, _ := got.Metadata["mode"].(string); v != "read-only" {
			t.Fatalf("mode after denied switch = %q, want read-only", v)
		}
		if res := lastToolResult(t, st, "s-deny"); !strings.Contains(res, "user denied") {
			t.Fatalf("model did not see the denial: %s", res)
		}
	})
}

// TestRuntimeSubagentCannotSwitchMode closes the escape hatch plan mode's
// promise would otherwise leak through: a delegate child inherits the parent's
// plan mode and must not be able to leave it on its own.
func TestRuntimeSubagentCannotSwitchMode(t *testing.T) {
	p := &toolCallScript{tool: "switch_mode", args: `{"mode":"full-auto"}`}
	r, st := startSession(t, "s-child", "general", "plan", p)
	// Mark the session as a delegate child so the runtime applies its
	// subagent rule, not the mode policy.
	base, _ := st.LoadSession(context.Background(), "s-child")
	base.ParentID = "s-root"
	if err := st.SaveSession(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	startID(t, r, "s-child", "child turn")
	finishedWithoutPrompts(t, r, "s-child")

	if res := lastToolResult(t, st, "s-child"); !strings.Contains(res, "subagents cannot switch modes") {
		t.Fatalf("child did not see the subagent denial: %s", res)
	}
	got, _ := st.LoadSession(context.Background(), "s-child")
	if v, _ := got.Metadata["mode"].(string); v != "plan" {
		t.Fatalf("child escaped plan mode on its own: mode=%q", v)
	}
}

// TestRuntimeSwitchModeGrantSkipsPrompt pins that choosing "always allow
// switch_mode" at the prompt silences the confirmation for every later switch
// in the session, including sideways ones that would otherwise ask.
func TestRuntimeSwitchModeGrantSkipsPrompt(t *testing.T) {
	p := &toolCallScript{tool: "switch_mode", args: `{"mode":"auto-edit"}`}
	r, st := startSession(t, "s-grant", "build", "read-only", p)
	runID := startID(t, r, "s-grant", "escalate once")
	req := approvalEvent(t, r, runID)
	r.ResolveApproval(runID, req.ID, Decision{Allow: true, Always: true})
	finishedWithoutPrompts(t, r, "s-grant")
	got, _ := st.LoadSession(context.Background(), "s-grant")
	if v, _ := got.Metadata["mode"].(string); v != "auto-edit" {
		t.Fatalf("mode after granted switch = %q, want auto-edit", v)
	}

	// Second turn: the next switch is covered by the grant, so it applies with
	// no prompt at all.
	p.args = `{"mode":"full-auto"}`
	startID(t, r, "s-grant", "escalate again")
	finishedWithoutPrompts(t, r, "s-grant")
	got, _ = st.LoadSession(context.Background(), "s-grant")
	if v, _ := got.Metadata["mode"].(string); v != "full-auto" {
		t.Fatalf("mode after grant-covered switch = %q, want full-auto", v)
	}
}

// TestSwitchModeTargetParsing pins the pre-execution argument extraction used
// for the verdict: missing or corrupt mode fields fall back to the default
// rather than derailing the decision.
func TestSwitchModeTargetParsing(t *testing.T) {
	if got := switchModeTarget(json.RawMessage(`{"mode":"plan"}`)); string(got) != "plan" {
		t.Errorf("plan target = %q", got)
	}
	if got := switchModeTarget(nil); got != "auto-edit" {
		t.Errorf("missing args = %q, want the default mode", got)
	}
	if got := switchModeTarget(json.RawMessage(`{"mode":"yolo"}`)); got != "auto-edit" {
		t.Errorf("invalid mode = %q, want the default mode", got)
	}
}
