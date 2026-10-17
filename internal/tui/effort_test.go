package tui

import (
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// The marker rides the track and has to sit above its own label — that is what
// makes this a scale rather than a cursor parked somewhere along a line.
func TestEffortMarkerSitsAboveItsOwnLabel(t *testing.T) {
	levels := config.EffortLevels()
	for sel, want := range levels {
		m := layoutModel(t)
		m.effortSel = sel
		lines := m.renderEffortSlider(ThemeByName("claude"), 80)
		if len(lines) < 4 {
			t.Fatalf("slider rendered %d lines", len(lines))
		}
		// Measured in display columns, not bytes: the rule glyph is three bytes
		// wide in UTF-8 and one column on screen, and indexing the string
		// directly would silently compare the wrong positions.
		track := []rune(stripANSI(lines[2]))
		labels := []rune(stripANSI(lines[3]))
		marker := -1
		for i, r := range track {
			if r == '▲' {
				marker = i
				break
			}
		}
		if marker < 0 {
			t.Fatalf("%s: no marker on the track %q", want, string(track))
		}
		col := lipgloss.Width(string(track[:marker]))
		from := col - len(want)/2
		if from < 0 || from+len(want) > len(labels) {
			t.Fatalf("%s: marker at column %d is off the label row %q", want, col, string(labels))
		}
		if got := string(labels[from : from+len(want)]); got != want {
			t.Fatalf("%s: marker at column %d sits above %q, not %q", want, col, got, want)
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

// The composer ring is a still image on purpose. It sat right above the input and
// repainted with every frame, which is the one place motion competes with typing;
// the animated half of the effect is the panel's.
func TestSupercodeRingHoldsStill(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": true}
	m.cfg.Animations = true
	th := ThemeByName("claude")
	first := m.supercodeRule(th, 60, lipgloss.NewStyle())
	for frame := 1; frame < 40; frame++ {
		m.activityFrame = frame
		if got := m.supercodeRule(th, 60, lipgloss.NewStyle()); got != first {
			t.Fatalf("the composer ring moved on frame %d", frame)
		}
	}
	// It still has to say what mode is on: the ring alone is not a label.
	if !strings.Contains(stripANSI(first), "supercode") {
		t.Fatalf("the rule carries no mode label: %q", stripANSI(first))
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

// The slider carries the mode as a stop on the scale: the label and the divider,
// not a separate key. This is what the user is being asked to slide onto.
func TestEffortSliderCarriesTheSupercodeStop(t *testing.T) {
	th := ThemeByName("claude")
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": true}
	lines := m.renderEffortSlider(th, 80)
	plain := stripANSI(strings.Join(lines, "\n"))

	labelsRow := stripANSI(lines[3])
	if !strings.Contains(labelsRow, "supercode") {
		t.Fatalf("supercode should be the last stop on the scale row:\n%s", plain)
	}
	if !strings.Contains(stripANSI(lines[2]), "┊") {
		t.Fatalf("the divider should mark it off from the levels:\n%s", plain)
	}
	if !strings.Contains(plain, "xhigh + workflows") {
		t.Fatalf("the stop should say what the mode is:\n%s", plain)
	}
}

// There is no Tab shortcut any more: the last stop is the toggle, so the hint must
// not advertise a key that does nothing.
func TestSliderHintDoesNotMentionTab(t *testing.T) {
	m := layoutModel(t)
	for _, w := range []int{46, 80} {
		got := strings.ToLower(stripANSI(strings.Join(m.renderEffortSlider(ThemeByName("claude"), w), "\n")))
		if strings.Contains(got, "tab") {
			t.Fatalf("w=%d still advertises tab:\n%s", w, got)
		}
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

// The surface is a closed form of the frame, not a timer: repainting one frame has
// to be identical, and advancing the frame has to move the water.
func TestRippleIsStablePerFrameAndMovesBetweenFrames(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	rowOf := func(frame int) string {
		return strings.Join(rippleRows(60, 6, frame, 1), "\n")
	}
	if rowOf(7) != rowOf(7) {
		t.Fatal("the same frame rendered two different surfaces")
	}
	distinct := map[string]bool{}
	for f := 0; f < 12; f++ {
		distinct[rowOf(f)] = true
	}
	if len(distinct) < 6 {
		t.Fatalf("the surface only took %d shapes across 12 frames", len(distinct))
	}
}

// Dynamic means the picture changed; coherent means most of it did not. A field
// that changes every cell every frame is static noise, one that changes too few
// reads as frozen. This pins the middle.
func TestRipplesMoveWithoutBoiling(t *testing.T) {
	const cols, rows = 40, 8
	level := func(frame int) []float64 {
		out := make([]float64, 0, cols*rows)
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				out = append(out, rippleLevel(c, r, cols, rows, frame))
			}
		}
		return out
	}

	prev := level(0)
	for frame := 1; frame < 9; frame++ {
		cur := level(frame)
		if len(prev) != len(cur) {
			t.Fatal("surface changed size between frames")
		}
		var mean float64
		for i := range cur {
			mean += cur[i]
		}
		mean /= float64(len(cur))
		// Water sits around mid-height. A frame that is all crests or all troughs
		// is the failure mode where the surface reads as a flash.
		if mean < 0.3 || mean > 0.7 {
			t.Fatalf("frame %d has mean height %.2f", frame, mean)
		}
		changed := 0
		for i := range cur {
			if math.Abs(cur[i]-prev[i]) > 0.04 {
				changed++
			}
		}
		if changed < len(cur)/20 || changed > len(cur)*7/10 {
			t.Fatalf("frame %d moved %d of %d cells", frame, changed, len(cur))
		}
		prev = cur
	}
}

// Every row is exactly the width it was asked for. The panel clips by wrapping, so
// one column over is not an overflow — it is an extra line and a broken box.
func TestRippleRowsFillTheWidth(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, width := range []int{1, 9, 40, 97} {
		rows := rippleRows(width, 3, 4, 1)
		if len(rows) != 3 {
			t.Fatalf("width %d rendered %d rows", width, len(rows))
		}
		for i, r := range rows {
			if got := lipgloss.Width(r); got != width {
				t.Fatalf("width %d row %d is %d columns", width, i, got)
			}
		}
	}
}

// The rings have to travel outward, not just shimmer in place: the same cell at
// two frames should differ, and a cell's level should stay inside [0,1].
func TestRippleLevelStaysBounded(t *testing.T) {
	for frame := 0; frame < 20; frame++ {
		for r := 0; r < 14; r++ {
			for c := 0; c < 40; c++ {
				if v := rippleLevel(c, r, 40, 14, frame); v < 0 || v > 1 {
					t.Fatalf("frame %d cell (%d,%d) = %f, want [0,1]", frame, c, r, v)
				}
			}
		}
	}
}

// NO_COLOR strips backgrounds, and a colour-only surface would be invisible. The
// glyph fallback is what keeps the mode visible on a monochrome terminal.
func TestRippleFallsBackToGlyphsWithoutColour(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "1")
	rows := rippleRows(40, 6, 3, 1)
	glyphs := 0
	for i, r := range rows {
		if strings.Contains(r, "\x1b[") {
			t.Fatalf("row %d emitted colour under NO_COLOR: %q", i, r)
		}
		if got := lipgloss.Width(r); got != 40 {
			t.Fatalf("row %d is %d columns, want 40", i, got)
		}
		glyphs += strings.Count(r, "░") + strings.Count(r, "▒") + strings.Count(r, "▓") + strings.Count(r, "█")
	}
	if glyphs == 0 {
		t.Fatal("no glyph texture on a monochrome terminal")
	}
}

// The panel is the surface's home: it starts below the last line of text and fills
// every row from there to the bottom border. Nothing that the user has to read
// sits on top of the water, and the water does not stop halfway down.
func TestSurfaceFillsThePanelBelowTheText(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	m := layoutModel(t)
	m.width, m.height = 110, 40
	m.overlay = overlayEffort
	m.effortSel = len(config.EffortLevels())
	m.session.Metadata = map[string]any{"supercode": true}
	m.activityFrame = 3

	rows := strings.Split(m.renderOverlay(ThemeByName("claude")), "\n")
	footer, border := -1, -1
	for i, row := range rows {
		if strings.Contains(stripANSI(row), "esc cancel") {
			footer = i
		}
		if strings.Contains(row, "╰") {
			border = i
		}
	}
	if footer < 0 || border <= footer {
		t.Fatalf("footer at %d, bottom border at %d", footer, border)
	}
	for i := 0; i <= footer; i++ {
		if strings.Contains(rows[i], "\x1b[48;") {
			t.Fatalf("row %d: the surface is drawn above the text:\n%s", i, stripANSI(rows[i]))
		}
	}
	for i := footer + 1; i < border; i++ {
		if !strings.Contains(rows[i], "\x1b[48;") {
			t.Fatalf("row %d: a gap in the surface below the text:\n%s", i, stripANSI(rows[i]))
		}
	}
	// The water owns the panel below the text, not a strip at the bottom of it.
	if got, want := border-footer-1, len(rows)-footer-1; got < want/2 {
		t.Fatalf("the surface covers %d rows below the text, want at least %d", got, want/2)
	}
}

// The entrance is a spread, not a fade: it starts with nothing covered, grows
// outward from the sources, and only ends when every cell of the area is reached.
func TestSurfaceSpreadsFromItsSources(t *testing.T) {
	const cols, rows = 60, 10
	reached := func(reveal float64) int {
		reach := rippleSpread(cols, rows, 4) * reveal
		n := 0
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				if _, d := rippleCell(c, r, cols, rows, 4); d <= reach {
					n++
				}
			}
		}
		return n
	}

	if got := reached(0); got != 0 {
		t.Fatalf("%d cells are already covered at reveal 0", got)
	}
	prev := 0
	for _, reveal := range []float64{0.15, 0.4, 0.7, 1} {
		got := reached(reveal)
		if got <= prev {
			t.Fatalf("reveal %.2f covers %d cells, no more than the %d before it", reveal, got, prev)
		}
		prev = got
	}
	if prev != cols*rows {
		t.Fatalf("a full reveal reaches %d of %d cells", prev, cols*rows)
	}
}

// At reveal 0 the panel is genuinely empty: no shades and no crest glyphs.
func TestSurfaceStartsEmpty(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	for i, row := range rippleRows(40, 5, 3, 0) {
		if strings.TrimSpace(stripANSI(row)) != "" || strings.Contains(row, "\x1b[48;") {
			t.Fatalf("row %d already has surface at reveal 0: %q", i, stripANSI(row))
		}
	}
}

// Switching the mode on arms the entrance at the frame it happened; the reveal
// then walks from nothing to full over rippleGrowFrames.
func TestEntranceStartsWhenTheModeComesOn(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": false}
	m.activityFrame = 40
	m.noteRippleTransition()
	if m.rippleArmed {
		t.Fatal("the entrance armed while the mode was off")
	}
	if got := m.rippleReveal(); got != 1 {
		t.Fatalf("a mode that is off reports reveal %v", got)
	}

	m.session.Metadata["supercode"] = true
	m.noteRippleTransition()
	if !m.rippleArmed || m.rippleStart != 40 {
		t.Fatalf("armed=%v start=%d, want true/40", m.rippleArmed, m.rippleStart)
	}
	if got := m.rippleReveal(); got != 0 {
		t.Fatalf("reveal at the first frame is %v, want 0", got)
	}
	m.activityFrame = 40 + rippleGrowFrames/2
	if got := m.rippleReveal(); got <= 0 || got >= 1 {
		t.Fatalf("reveal halfway is %v, want between 0 and 1", got)
	}
	m.activityFrame = 40 + rippleGrowFrames
	if got := m.rippleReveal(); got != 1 {
		t.Fatalf("reveal after the growth window is %v, want 1", got)
	}
}

// And with the mode off there is no surface and no timer: the panel is a plain
// form that renders the same bytes at every frame.
func TestPanelIsPlainWithoutSupercode(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	m := layoutModel(t)
	m.width, m.height = 110, 40
	m.overlay = overlayEffort
	m.session.Metadata = map[string]any{"supercode": false}

	first := m.renderOverlay(ThemeByName("claude"))
	if strings.Contains(first, "\x1b[48;") {
		t.Fatal("the surface rendered with supercode off")
	}
	for frame := 1; frame < 9; frame++ {
		m.activityFrame = frame
		if got := m.renderOverlay(ThemeByName("claude")); got != first {
			t.Fatalf("frame %d changed a panel with supercode off", frame)
		}
	}
}

// The panel clips at its content width by wrapping, so a row one column too wide
// does not overflow — it silently becomes two rows and every line below it shifts.
// That is what broke the scale before: the header, the track, the labels and the
// caption all wrapped. This pins the budget for every width the overlay can take.
func TestEffortSliderFitsItsPanel(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	th := ThemeByName("claude")
	levels := config.EffortLevels()
	for w := 20; w <= 160; w++ {
		for sel := 0; sel <= len(levels); sel++ {
			m := layoutModel(t)
			m.effortSel = sel
			budget := max(14, w-6)
			for i, line := range m.renderEffortSlider(th, w) {
				if got := lipgloss.Width(line); got > budget {
					t.Fatalf("w=%d sel=%d row %d: %d columns, budget %d (%q)",
						w, sel, i, got, budget, stripANSI(line))
				}
			}
		}
	}
}

// Narrow panels used to drop the tail slot and then place the caret at a column
// past the end of the track, so opening the slider on supercode showed no
// selection at all. Whatever the width, the chosen stop must have a caret.
func TestEffortMarkerIsAlwaysVisible(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	th := ThemeByName("claude")
	levels := config.EffortLevels()
	for w := 20; w <= 140; w++ {
		for sel := 0; sel <= len(levels); sel++ {
			m := layoutModel(t)
			m.effortSel = sel
			found := 0
			for _, line := range m.renderEffortSlider(th, w) {
				found += strings.Count(stripANSI(line), "▲")
			}
			if found != 1 {
				t.Fatalf("w=%d sel=%d: %d markers, want exactly 1", w, sel, found)
			}
		}
	}
}

// The last stop is the toggle: arriving on it switches the mode on for this
// session, and leaving it switches the mode back off. No separate key.
func TestMovingOntoTheStopSwitchesTheMode(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": false}
	m.effortSel = len(config.EffortLevels()) - 1 // the deepest level, "max"

	if cmd := m.moveEffortStop(1); cmd == nil {
		t.Fatal("arriving on the stop issued no toggle")
	}
	if m.effortSel != len(config.EffortLevels()) {
		t.Fatalf("marker is at %d, want the supercode stop", m.effortSel)
	}
	if !m.supercodeOn() {
		t.Fatal("the mode did not come on when the marker reached the stop")
	}

	if cmd := m.moveEffortStop(-1); cmd == nil {
		t.Fatal("leaving the stop issued no toggle")
	}
	if m.supercodeOn() {
		t.Fatal("the mode stayed on after the marker left the stop")
	}
}

// Moving between ordinary levels must not touch the toggle, or every arrow key
// would write to the store.
func TestMovingBetweenLevelsDoesNotToggle(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": false}
	m.effortSel = 1
	if cmd := m.moveEffortStop(1); cmd != nil {
		t.Fatal("moving between levels issued a toggle")
	}
	if m.supercodeOn() {
		t.Fatal("moving between levels switched the mode on")
	}
}

// Tab used to flip supercode. It must do nothing now: a key the hint does not
// mention should not change the mode.
func TestTabNoLongerTogglesSupercode(t *testing.T) {
	m := layoutModel(t)
	m.session.Metadata = map[string]any{"supercode": false}
	if cmd := m.handleEffortKey(tea.KeyPressMsg{Code: tea.KeyTab}); cmd != nil {
		t.Fatal("tab still issued a command")
	}
	if m.supercodeOn() {
		t.Fatal("tab still toggled the mode")
	}
}

// With animations disabled there is no frame clock, so the entrance must resolve
// immediately: an unreachable reveal would leave the panel permanently empty.
func TestEntranceResolvesWithoutAnimations(t *testing.T) {
	m := layoutModel(t)
	m.cfg.Animations = false
	m.session.Metadata = map[string]any{"supercode": true}
	m.noteRippleTransition()
	if !m.rippleArmed {
		t.Fatal("the mode did not arm the entrance")
	}
	if got := m.rippleReveal(); got != 1 {
		t.Fatalf("reveal = %v with animations off, want 1", got)
	}
}
