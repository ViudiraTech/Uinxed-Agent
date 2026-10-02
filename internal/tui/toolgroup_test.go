package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func callsNamed(names ...string) []domain.ToolCall {
	out := make([]domain.ToolCall, 0, len(names))
	for i, n := range names {
		out = append(out, domain.ToolCall{
			ID:       fmt.Sprintf("c%d", i),
			Function: domain.ToolCallFunction{Name: n},
		})
	}
	return out
}

// Grouping is what makes the row count differ from the call count, and the
// virtualizer's estimate is built from the rows — so this is the number that
// keeps a block's height honest while its content is unchanged.
func TestToolRowsCollapsesRuns(t *testing.T) {
	cases := []struct {
		name  string
		calls []domain.ToolCall
		want  int
	}{
		{"a lone read keeps its own card and result", callsNamed("read_file"), 2},
		{"two reads fold into one row", callsNamed("read_file", "grep"), 1},
		{"a longer run is still one row", callsNamed("read_file", "grep", "glob", "bash"), 1},
		{"shell folds in too", callsNamed("bash", "grep"), 1},
		{"a mutation breaks the run", callsNamed("read_file", "grep", "write_file"), 1 + 2},
		{"independent tools stay independent", callsNamed("write_file", "todo_write"), 4},
	}
	for _, c := range cases {
		if got := toolRows(c.calls); got != c.want {
			t.Fatalf("%s: toolRows = %d, want %d", c.name, got, c.want)
		}
	}
}

// use_skill is CategoryRead in the tool registry, but folding a skill activation
// into "Read 2 files" would misreport what the agent actually did.
func TestSkillActivationNeverJoinsAReadGroup(t *testing.T) {
	runs := planToolRuns(callsNamed("read_file", "use_skill", "grep"))
	if len(runs) != 3 {
		t.Fatalf("expected three runs, got %d: %#v", len(runs), runs)
	}
	for _, r := range runs {
		if r.group {
			t.Fatalf("use_skill must not be absorbed into a group: %#v", r)
		}
	}
}

func TestGroupPhraseUsesTenseAndCounts(t *testing.T) {
	calls := callsNamed("read_file", "list_dir", "grep")
	if got := groupPhrase(calls, true); got != "Reading 2 files, searching 1 pattern" {
		t.Fatalf("in-flight phrase = %q", got)
	}
	if got := groupPhrase(calls, false); got != "Read 2 files, searched 1 pattern" {
		t.Fatalf("settled phrase = %q", got)
	}
}

func TestGroupToggleOpensEveryMember(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s", Messages: []domain.Message{{
		ID: "a", Role: domain.RoleAssistant,
		ToolCalls: callsNamed("read_file", "grep"),
	}}}, 80)

	c.ToggleTool(groupPrefix + "c0,c1")
	if !c.expandedTools["c0"] || !c.expandedTools["c1"] {
		t.Fatal("opening a grouped row must expand every member")
	}
	c.ToggleTool(groupPrefix + "c0,c1")
	if c.expandedTools["c0"] || c.expandedTools["c1"] {
		t.Fatal("closing a grouped row must collapse every member")
	}

	// A plain id must still toggle only itself.
	c.ToggleTool("c0")
	if !c.expandedTools["c0"] || c.expandedTools["c1"] {
		t.Fatal("an ungrouped toggle leaked to another call")
	}
}

// The aggregate row is a summary, not a substitute: opening it has to bring back
// every command and its result, or a destructive call would be unrecoverable
// from the transcript.
func TestGroupedRowHidesNothingWhenOpened(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s", Messages: []domain.Message{{
		ID: "a", Role: domain.RoleAssistant,
		ToolCalls: []domain.ToolCall{
			{ID: "c0", Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"rm -rf build"}`}},
			{ID: "c1", Function: domain.ToolCallFunction{Name: "read_file", Arguments: `{"path":"a.go"}`}},
		},
	}}}, 80)
	acts := map[string]domain.ToolActivity{
		"c0": {CallID: "c0", Name: "bash", State: "success", Output: "removed"},
		"c1": {CallID: "c1", Name: "read_file", State: "success", Output: "package a"},
	}
	render := func() string {
		var b strings.Builder
		for _, l := range c.renderBlock(c.blocks[0], ThemeByName("claude"), acts, "", 0) {
			b.WriteString(stripANSI(l.Text))
			b.WriteByte('\n')
		}
		return b.String()
	}

	collapsed := render()
	if !strings.Contains(collapsed, "Read 1 file, ran 1 command") {
		t.Fatalf("collapsed group row missing:\n%s", collapsed)
	}
	if strings.Contains(collapsed, "rm -rf build") {
		t.Fatalf("a collapsed group must not inline its commands:\n%s", collapsed)
	}

	c.ToggleTool(groupPrefix + "c0,c1")
	opened := render()
	for _, want := range []string{"rm -rf build", "Read(a.go)", "removed", "package a"} {
		if !strings.Contains(opened, want) {
			t.Fatalf("opening the group lost %q:\n%s", want, opened)
		}
	}
}

// Grouping changes a block's height while its content stays put, which is
// exactly the case the render cache keys on. A cached block must not survive a
// change in whether its run is open.
func TestGroupingInvalidatesTheRenderCache(t *testing.T) {
	c := NewConversation()
	c.SetSession(domain.Session{ID: "s", Messages: []domain.Message{{
		ID: "a", Role: domain.RoleAssistant,
		ToolCalls: []domain.ToolCall{
			{ID: "c0", Function: domain.ToolCallFunction{Name: "bash", Arguments: `{"cmd":"ls"}`}},
			{ID: "c1", Function: domain.ToolCallFunction{Name: "grep", Arguments: `{"pattern":"x"}`}},
		},
	}}}, 80)
	acts := map[string]domain.ToolActivity{
		"c0": {CallID: "c0", Name: "bash", State: "success", Output: "ok"},
		"c1": {CallID: "c1", Name: "grep", State: "success", Output: "hit"},
	}
	b := &c.blocks[0]

	closed := c.renderCached(b, ThemeByName("claude"), acts, "", 0)
	c.ToggleTool(groupPrefix + "c0,c1")
	opened := c.renderCached(b, ThemeByName("claude"), acts, "", 0)

	if len(opened) <= len(closed) {
		t.Fatalf("opening the group returned %d lines against %d collapsed; the cache served a stale block",
			len(opened), len(closed))
	}
}
