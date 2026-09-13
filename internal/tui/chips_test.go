package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"github.com/ViudiraTech/Uinxed-Agent/internal/agent"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// isAgentRef mirrors what fileRefsIn needs from the agent registry.
func isAgentRef(id string) bool {
	d := agent.Get(id)
	return d.ID == id && d.CanSubagent()
}

func TestFilterHistoryNewestFirst(t *testing.T) {
	h := []string{"one", "two words", "three", "another two"}
	got := filterHistory(h, "")
	if len(got) != 4 || got[0] != "another two" || got[3] != "one" {
		t.Fatalf("empty query order = %#v, want newest first", got)
	}
	got = filterHistory(h, "two")
	if len(got) != 2 || got[0] != "another two" || got[1] != "two words" {
		t.Fatalf("filtered = %#v", got)
	}
	if got := filterHistory(h, "no match anywhere"); len(got) != 0 {
		t.Fatalf("unmatched query returned %#v", got)
	}
	// Case-insensitive matching, matching shell history search behaviour.
	if got := filterHistory(h, "TWO"); len(got) != 2 {
		t.Fatalf("case-insensitive match failed: %#v", got)
	}
}

func TestFileRefsInMirrorsRuntimeRules(t *testing.T) {
	text := "@src/main.go look at this and @README.md, plus @general for the task and @skill:search and @src/main.go again"
	refs := fileRefsIn(text, isAgentRef)
	want := []string{"src/main.go", "README.md"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %#v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs = %#v, want %v", refs, want)
		}
	}
}

func TestFileRefsInCapsAndSkipsNonPaths(t *testing.T) {
	text := "@a @b @c @d @e @f @g @h @i @j"
	refs := fileRefsIn(text, isAgentRef)
	if len(refs) != 8 {
		t.Fatalf("cap not applied: %d refs", len(refs))
	}
	// Empty and agent mentions vanish entirely.
	if got := fileRefsIn("@ @general @skill:x", isAgentRef); len(got) != 0 {
		t.Fatalf("non-path tokens leaked: %#v", got)
	}
}

func TestRemoveFileRef(t *testing.T) {
	cases := []struct {
		text, ref, want string
	}{
		{"@a.txt explain", "a.txt", "explain"},
		{"explain @a.txt please", "a.txt", "explain please"},
		{"@a.txt @b.txt both", "b.txt", "@a.txt both"},
		{"no refs here", "a.txt", "no refs here"},
	}
	for _, c := range cases {
		if got := removeFileRef(c.text, c.ref); got != c.want {
			t.Errorf("removeFileRef(%q, %q) = %q, want %q", c.text, c.ref, got, c.want)
		}
	}
}

// TestRenderFileChipsRegistersRegions checks the chip row exists and every chip
// carries a removal region at the rendered position.
func TestRenderFileChipsRegistersRegions(t *testing.T) {
	root := t.TempDir()
	_ = root
	ta := textarea.New()
	ta.SetWidth(80)
	_ = ta.Focus()
	ta.SetValue("@a.txt and @b.txt")
	store, err := config.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &Model{prompt: ta, cfg: store.Snapshot()}
	lines, regs := m.renderFileChips(themeFor(m.cfg), 80)
	if len(lines) != 1 {
		t.Fatalf("chips rendered %d lines", len(lines))
	}
	if len(regs) != 2 {
		t.Fatalf("regions = %d, want one per chip", len(regs))
	}
	for _, r := range regs {
		if r.Kind != ActionFileChip || r.Rect.W <= 0 {
			t.Fatalf("bad region %#v", r)
		}
	}
	if !strings.Contains(lines[0], "@a.txt") || !strings.Contains(lines[0], "@b.txt") {
		t.Fatalf("chip line = %q", lines[0])
	}
	// Regions must not overlap: they sit side by side on one line.
	if regs[0].Rect.X+regs[0].Rect.W > regs[1].Rect.X {
		t.Fatalf("chip regions overlap: %#v %#v", regs[0].Rect, regs[1].Rect)
	}
}
