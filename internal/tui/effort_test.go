package tui

import (
	"strings"
	"testing"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// The caret is placed by column arithmetic rather than by hand-padding, so this
// is what keeps it pointing at the stop it claims to mark.
func TestEffortCaretSitsUnderTheSelectedStop(t *testing.T) {
	levels := config.EffortLevels()
	for sel, want := range levels {
		m := layoutModel(t)
		m.effortSel = sel
		lines := m.renderEffortSlider(ThemeByName("claude"), 80)
		if len(lines) < 4 {
			t.Fatalf("slider rendered %d lines", len(lines))
		}
		scale := stripANSI(lines[2])
		caret := stripANSI(lines[3])
		col := strings.Index(caret, "▲")
		if col < 0 {
			t.Fatalf("%s: no caret on %q", want, caret)
		}
		if col+len(want) > len(scale) {
			t.Fatalf("%s: caret at column %d runs past the scale %q", want, col, scale)
		}
		if got := scale[col : col+len(want)]; got != want {
			t.Fatalf("caret at column %d points at %q, want %q\nscale: %q", col, got, want, scale)
		}
	}
}

// The description swapping as you move is the whole point: a row of five words
// on its own does not tell you which one you want.
func TestEffortGuideFollowsTheSelection(t *testing.T) {
	m := layoutModel(t)
	th := ThemeByName("claude")

	m.effortSel = effortIndex("low")
	low := stripANSI(strings.Join(m.renderEffortSlider(th, 90), "\n"))
	m.effortSel = effortIndex("max")
	high := stripANSI(strings.Join(m.renderEffortSlider(th, 90), "\n"))

	if low == high {
		t.Fatal("the slider renders identically at low and max")
	}
	if !strings.Contains(low, "brainstorming") {
		t.Fatalf("low should describe quick exchanges:\n%s", low)
	}
	if !strings.Contains(high, "diminishing returns") {
		t.Fatalf("max should warn about diminishing returns:\n%s", high)
	}
}

// /effort takes levels, but supercode is not one of them any more.
func TestEffortLevelsExcludeSupercode(t *testing.T) {
	if config.ValidEffort("supercode") {
		t.Fatal("supercode is a toggle, not a level")
	}
	for _, lv := range []string{"low", "medium", "high", "xhigh", "max"} {
		if !config.ValidEffort(lv) {
			t.Fatalf("%s should be a valid level", lv)
		}
		if effortIndex(lv) < 0 {
			t.Fatalf("%s has no stop on the slider", lv)
		}
	}
	if got := len(config.EffortLevels()); got != 5 {
		t.Fatalf("EffortLevels() has %d entries, want 5", got)
	}
}

// The toggle reads the session override first, then the configured default.
func TestSupercodePrefersTheSessionOverride(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Supercode = true
	if !m.supercodeOn() {
		t.Fatal("the configured default should apply with no override")
	}
	m.session.Metadata = map[string]any{"supercode": false}
	if m.supercodeOn() {
		t.Fatal("a session override must win over the default")
	}
}
