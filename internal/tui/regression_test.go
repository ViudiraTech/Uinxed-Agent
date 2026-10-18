package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// Switching sessions while a turn runs must not wedge the composer, and the turn's
// completion must clear the busy state even though it is no longer the visible
// session. A foreign run is only a subagent when it actually is one.
func TestSwitchSessionWhileRunning(t *testing.T) {
	m := layoutModel(t)
	m.session = domain.Session{ID: "A"}

	started := domain.NewEvent(domain.EventAgentStarted, "A", domain.AgentEvent{
		Run: domain.AgentRun{ID: "run-a", SessionID: "A", AgentID: "build", State: "running"},
	})
	m.handleRuntime(started)
	if !m.busy || m.mainRunSessionID != "A" {
		t.Fatalf("turn did not start: busy=%v owner=%q", m.busy, m.mainRunSessionID)
	}

	// Viewing another session leaves the composer usable.
	m.setSession(domain.Session{ID: "B"})
	if m.busy {
		t.Fatal("the composer is blocked while viewing a different session")
	}
	// Coming back to the running session shows it as busy again.
	m.setSession(domain.Session{ID: "A"})
	if !m.busy {
		t.Fatal("returning to the running session lost its busy state")
	}
	m.setSession(domain.Session{ID: "B"})

	// The turn finishes while session B is on screen.
	finished := domain.NewEvent(domain.EventAgentFinished, "A", domain.AgentEvent{
		Run: domain.AgentRun{ID: "run-a", SessionID: "A", State: "done", ParentID: ""},
	})
	m.handleRuntime(finished)
	if m.busy {
		t.Fatal("busy stayed true after the owning turn finished")
	}
	if m.mainRunSessionID != "" {
		t.Fatalf("turn owner not cleared: %q", m.mainRunSessionID)
	}
	if _, ok := m.subagents["run-a"]; ok {
		t.Fatal("another session's main run was recorded as a subagent")
	}

	// And returning to A is not stuck busy either.
	m.setSession(domain.Session{ID: "A"})
	if m.busy {
		t.Fatal("returning to the finished session shows it as busy")
	}
}

// A running delegate's progress has to reach the conversation renderer, or the
// whole tracker in update.go feeds a renderer that never sees it.
func TestSubagentsReachTheRenderer(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 100, 30
	m.resize()
	if m.subagents == nil {
		m.subagents = map[string]domain.AgentRun{}
	}
	if m.subagentProgress == nil {
		m.subagentProgress = map[string]subagentProgress{}
	}
	m.subagents["run-child"] = domain.AgentRun{
		ID: "run-child", ParentID: "parent", SessionID: "child", AgentID: "explorer",
		Task: "look around", State: "running", StartedAt: time.Now(),
	}
	m.subagentProgress["run-child"] = subagentProgress{Tools: 3, LastTool: "grep", UpdatedAt: time.Now()}

	_ = m.renderBase(ThemeByName("claude"))
	if len(m.conv.subagents) == 0 {
		t.Fatal("renderBase produced a conversation with no subagents")
	}
	if len(m.conv.subagentProgress) == 0 {
		t.Fatal("renderBase produced a conversation with no subagent progress")
	}
}

// The toast is documented as transient. It used to occupy the status row forever,
// hiding errors and the configured statusline command.
func TestToastExpires(t *testing.T) {
	m := layoutModel(t)
	m.showToast("✓ theme")
	if got := stripANSI(m.busyIndicator(ThemeByName("claude"))); !strings.Contains(got, "theme") {
		t.Fatalf("a fresh toast is not shown: %q", got)
	}
	m.toastUntil = time.Now().Add(-time.Hour)
	if got := stripANSI(m.busyIndicator(ThemeByName("claude"))); strings.Contains(got, "theme") {
		t.Fatalf("an expired toast is still on the status row: %q", got)
	}
	// The error text behind it shows again.
	m.errorText = "boom"
	if got := stripANSI(m.busyIndicator(ThemeByName("claude"))); !strings.Contains(got, "boom") {
		t.Fatalf("the expired toast still masks the error: %q", got)
	}
}

// The file-chip click target is the chip row, which sits above the todo block and
// the working line — not directly above the composer.
func TestFileChipRegionMatchesTheRenderedRow(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 100, 30
	m.resize()
	m.session.Todos = []domain.Todo{
		{ID: "t1", Subject: "Read existing auth code", Status: domain.TodoCompleted},
		{ID: "t2", Subject: "Extract session handling", Status: domain.TodoInProgress},
	}
	m.busy = true
	m.busySince = time.Now()
	m.prompt.SetValue("look at @view.go please")

	out := m.renderBase(ThemeByName("claude"))
	rows := strings.Split(out, "\n")
	// The chip line carries the removal mark; the composer still contains the raw
	// @token, so matching on that alone would compare the region to the prompt.
	chipRow := -1
	for i, row := range rows {
		if strings.Contains(stripANSI(row), "@view.go ×") {
			chipRow = i
		}
	}
	if chipRow < 0 {
		t.Fatal("no chip row rendered")
	}
	regRow := -1
	for _, r := range m.regions {
		if r.Kind == ActionFileChip {
			regRow = r.Rect.Y
		}
	}
	if regRow < 0 {
		t.Fatal("no file-chip region")
	}
	if regRow != chipRow {
		t.Fatalf("chip region row %d, chip rendered on row %d (%q vs %q)",
			regRow, chipRow, stripANSI(rows[regRow]), stripANSI(rows[chipRow]))
	}
}

// The plan dialog's choices are the escape hatch. They stay on screen while the
// plan body scrolls above them.
func TestPlanApprovalShowsEveryChoice(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 80, 24
	m.resize()
	m.overlay = overlayApproval
	m.approval = &domain.ApprovalRequest{ToolName: "exit_plan", Summary: "split the auth module"}
	m.session.Metadata = map[string]any{"mode": "plan", "plan": planJSON(6)}

	out := stripANSI(m.renderOverlay(ThemeByName("claude")))
	for _, want := range []string{"1. Auto-accept edits", "2. Manually approve edits", "3. Keep planning", "3 keep planning"} {
		if !strings.Contains(out, want) {
			t.Fatalf("choice %q is off screen:\n%s", want, out)
		}
	}
}

func planJSON(n int) string {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"subject":"step %d","status":"pending","details":"some detail for step %d"}`, i+1, i+1)
	}
	b.WriteString("]")
	return b.String()
}

// Scrolling up by one block must move the window by one block. c.scroll used to
// be applied twice, so every page-up overshot once the bottom overscan ran out.
func TestScrollMovesOneBlockPerStep(t *testing.T) {
	c := NewConversation()
	var msgs []domain.Message
	for i := 0; i < 100; i++ {
		msgs = append(msgs, domain.Message{ID: fmt.Sprintf("m%d", i), Role: domain.RoleUser, Content: fmt.Sprintf("msg-%d", i)})
	}
	c.SetSession(domain.Session{ID: "s", Messages: msgs}, 80)
	re := regexp.MustCompile(`msg-(\d+)`)

	visible := func() (int, int) {
		lines := c.Render(20, ThemeByName("claude"), RenderOptions{})
		var b strings.Builder
		for _, l := range lines {
			b.WriteString(stripANSI(l.Text))
			b.WriteByte('\n')
		}
		lo, hi := -1, -1
		for _, m := range re.FindAllStringSubmatch(b.String(), -1) {
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			if lo < 0 || n < lo {
				lo = n
			}
			if n > hi {
				hi = n
			}
		}
		return lo, hi
	}

	prevHi := -1
	for scroll := 0; scroll <= 40; scroll += 2 {
		c.scroll = scroll
		lo, hi := visible()
		if lo < 0 {
			t.Fatalf("scroll %d rendered no messages", scroll)
		}
		if prevHi >= 0 && prevHi-hi > 1 {
			t.Fatalf("scroll %d jumped %d blocks (prev top %d, now %d)", scroll, prevHi-hi, prevHi, hi)
		}
		prevHi = hi
	}
}
