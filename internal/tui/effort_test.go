package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

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

// The tag rides on a row that is part of the exact-width contract the whole
// screen is built on, so it must fill the composer exactly at every size.
func TestSupercodeRuleKeepsTheComposerWidth(t *testing.T) {
	for _, w := range []int{20, 24, 40, 60, 100, 140} {
		m := layoutModel(t)
		m.session.Metadata = map[string]any{"supercode": true}
		got := m.supercodeRule(ThemeByName("claude"), w, lipgloss.NewStyle())
		if lipgloss.Width(got) != w {
			t.Fatalf("width %d: rule rendered %d columns", w, lipgloss.Width(got))
		}
		if w >= 30 && !strings.Contains(stripANSI(got), "supercode") {
			t.Fatalf("width %d: the tag is missing:\n%s", w, stripANSI(got))
		}
	}
}

// The sweep is the animation: the same frame has to render the same bytes, and
// moving frames have to move the highlight.
func TestSupercodeTagSweepsAcrossFrames(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": true}
	m.cfg.Animations = true
	th := ThemeByName("claude")
	seen := map[string]bool{}
	for frame := 0; frame < 60; frame++ {
		m.activityFrame = frame
		seen[m.supercodeRule(th, 60, lipgloss.NewStyle())] = true
	}
	if len(seen) < 5 {
		t.Fatalf("the tag rendered only %d distinct states across 60 frames", len(seen))
	}
}

// Reduced motion holds it still. Claude Code's disabled sentinel would instead
// make the whole label the leading segment and render it dim, greying out an
// indicator that is meant to read as active.
func TestSupercodeTagHoldsStillWithoutAnimations(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": true}
	m.cfg.Animations = false
	th := ThemeByName("claude")
	first := m.supercodeRule(th, 60, lipgloss.NewStyle())
	for frame := 1; frame < 20; frame++ {
		m.activityFrame = frame
		if got := m.supercodeRule(th, 60, lipgloss.NewStyle()); got != first {
			t.Fatalf("reduced motion animated the tag on frame %d", frame)
		}
	}
}

// It is an indicator: it must mean something, so it appears and disappears with
// the state it reports.
func TestComposerTagTracksTheSupercodeToggle(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 100, 30
	m.resize()
	if strings.Contains(stripANSI(m.renderBase(themeFor(m.cfg))), "supercode") {
		t.Fatal("the tag should be absent while supercode is off")
	}
	m.session.Metadata = map[string]any{"supercode": true}
	if !strings.Contains(stripANSI(m.renderBase(themeFor(m.cfg))), "supercode") {
		t.Fatal("the tag should mark the composer once supercode is on")
	}
}

// The wave belongs on the slider page too: turning supercode on there is the
// moment it is meant to register.
func TestEffortSliderCarriesTheRippleWhenSupercodeIsOn(t *testing.T) {
	th := ThemeByName("claude")
	m := layoutModel(t)
	off := stripANSI(strings.Join(m.renderEffortSlider(th, 80), "\n"))

	m.session.Metadata = map[string]any{"supercode": true}
	on := stripANSI(strings.Join(m.renderEffortSlider(th, 80), "\n"))

	if !strings.Contains(on, "supercode on") {
		t.Fatalf("the badge is missing:\n%s", on)
	}
	if len(on) <= len(off) {
		t.Fatal("supercode on should add the ripple row to the slider")
	}
	// The ripple is colour, so it only exists in the styled output.
	raw := strings.Join(m.renderEffortSlider(th, 80), "\n")
	if raw == on {
		t.Fatal("the ripple row carries no styling at all")
	}
}

// The ring has to actually travel, and only ever light a band rather than the
// whole rule.
func TestRippleIsABandThatMoves(t *testing.T) {
	const width = 40
	lit := map[int]int{}
	for phase := 0; phase < 40; phase++ {
		n := 0
		for col := 0; col < width; col++ {
			if _, ok := rippleColor(col, width, phase); ok {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("phase %d lit nothing", phase)
		}
		if n > 2*rippleBand+2 {
			t.Fatalf("phase %d lit %d columns, which is not a band", phase, n)
		}
		lit[n]++
	}
	if len(lit) < 2 {
		t.Fatal("the ring never changed size")
	}
}

// The ripple renders the rule a column at a time, so it is the one path here
// that scales with terminal width. This is the cost of the effect.
func BenchmarkSupercodeRule(b *testing.B) {
	m := layoutModelB(b)
	m.session.Metadata = map[string]any{"supercode": true}
	m.cfg.Animations = true
	base := lipgloss.NewStyle()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.activityFrame = i
		m.supercodeRule(ThemeByName("claude"), 200, base)
	}
}
