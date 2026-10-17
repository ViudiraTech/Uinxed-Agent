package tui

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/charmbracelet/x/ansi"
)

// effortGuide pairs each level with what it is for. The guide line is the point
// of the slider: Claude Code swaps it as you move along the scale, which is what
// makes a row of five words a choice rather than a vocabulary test.
var effortGuide = map[string]string{
	"low":    "Quick exchanges you review as you go — brainstorming, a first sketch, a rename",
	"medium": "Day-to-day work with a clear scope, such as implementing a described feature",
	"high":   "Work where verification matters or edge cases are likely, such as fixing a bug",
	"xhigh":  "Deeper reasoning at higher token spend",
	"max":    "Hard problems worked through without you. May show diminishing returns and overthink",
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func effortIndex(level string) int {
	for i, l := range config.EffortLevels() {
		if l == level {
			return i
		}
	}
	return -1
}

// supercodeOn reports the concurrent-orchestration toggle, session override
// first. Like Claude Code's ultracode it is independent of the effort level.
func (m *Model) supercodeOn() bool {
	if v, ok := m.session.Metadata["supercode"].(bool); ok {
		return v
	}
	return m.cfg.Supercode
}

func (m *Model) openEffortSlider() tea.Cmd {
	m.effortSel = effortIndex(m.currentEffort())
	if m.effortSel < 0 {
		m.effortSel = effortIndex("high")
	}
	if m.supercodeOn() {
		// The marker opens where the mode is, rather than on the level it left
		// alone, so the slider shows the state it is actually in.
		m.effortSel = len(config.EffortLevels())
	}
	m.overlay = overlayEffort
	m.setFocus(FocusOverlay)
	// The surface only moves if the frame advances, and an idle session would
	// otherwise leave it frozen on the frame it opened with.
	return m.startAnimation()
}

// effortStops is the scale's length: the levels plus supercode, which sits past
// the divider at the end rather than among them.
func effortStops() int { return len(config.EffortLevels()) + 1 }

// handleEffortKey drives the slider. There is no separate key for supercode: the
// last stop is the mode, so moving onto it turns the mode on and moving off it
// turns the mode back off. Enter saves the setting as the default and s applies it
// to this session only — the same split Claude Code draws between "save" and
// "this session".
func (m *Model) handleEffortKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.closeOverlay()
	case "left", "h":
		return m.moveEffortStop(-1)
	case "right", "l":
		return m.moveEffortStop(1)
	case "enter":
		return m.applyEffort(false)
	case "s":
		return m.applyEffort(true)
	}
	return nil
}

// moveEffortStop moves the marker and keeps the mode in step with it. The toggle
// is applied to this session as the marker arrives, because a slider stop that
// only takes effect on a later keypress is not a stop, it is a selection. The
// move is not saved as the default; that is what Enter is for.
func (m *Model) moveEffortStop(delta int) tea.Cmd {
	next := min(max(0, m.effortSel+delta), effortStops()-1)
	if next == m.effortSel {
		return nil
	}
	m.effortSel = next
	onStop := next >= len(config.EffortLevels())
	if onStop == m.supercodeOn() {
		return nil
	}
	return m.setSupercode(onStop, true)
}

func (m *Model) applyEffort(sessionOnly bool) tea.Cmd {
	levels := config.EffortLevels()
	if m.effortSel >= len(levels) {
		// The stop already turned the mode on when the marker reached it. Enter
		// only has work left to do when it is asked to persist that as the
		// default; s has nothing to add, because the move was session-only.
		m.closeOverlay()
		if sessionOnly || !m.supercodeOn() {
			return nil
		}
		return m.setSupercode(true, false)
	}
	if m.effortSel < 0 || m.effortSel > len(levels) {
		return nil
	}
	level := levels[m.effortSel]
	m.closeOverlay()
	sid := m.session.ID
	return asyncOp("set_effort", func() (any, error) {
		if err := m.ctrl.SetEffort(m.ctx, sid, level, sessionOnly); err != nil {
			return nil, err
		}
		return level, nil
	})
}

func (m *Model) toggleSupercode() tea.Cmd { return m.setSupercode(!m.supercodeOn(), false) }

// handleEffortCommand is /effort: no argument opens the slider, a level name sets
// it, auto drops the session's override, and supercode flips the toggle.
func (m *Model) handleEffortCommand(arg string) tea.Cmd {
	sid := m.session.ID
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
		return m.openEffortSlider()
	case "auto":
		return asyncOp("clear_effort", func() (any, error) { return nil, m.ctrl.ClearEffort(m.ctx, sid) })
	case "supercode":
		return m.toggleSupercode()
	case "supercode on", "supercode off":
		on := strings.HasSuffix(strings.ToLower(strings.TrimSpace(arg)), "on")
		return m.setSupercode(on, false)
	}
	level := strings.ToLower(strings.TrimSpace(arg))
	if !config.ValidEffort(level) {
		m.showError(fmt.Errorf("effort must be %s, auto, or supercode",
			strings.Join(config.EffortLevels(), ", ")))
		return nil
	}
	return asyncOp("set_effort", func() (any, error) {
		if err := m.ctrl.SetEffort(m.ctx, sid, level, false); err != nil {
			return nil, err
		}
		return level, nil
	})
}

// rainbowRamp is the seven colors Claude Code's rainbow_* tokens name — red,
// orange, yellow, green, blue, indigo, violet — in that order. The docs name the
// colors without publishing values, so these are the standard hues.
var rainbowRamp = []string{"#FF5F56", "#FFA657", "#FFD75F", "#4EBA65", "#4FA8FF", "#7B7BFF", "#B57BFF"}

// rippleBand is how many columns the travelling ring is thick.
const rippleBand = 3

// rippleColor reports which rainbow color a column takes, and whether the
// expanding ring is over it at all. The ring grows from the centre outward and
// wraps at the edge, so the wave keeps emanating rather than stopping dead.
func rippleColor(col, width, phase int) (int, bool) {
	if width <= 0 {
		return 0, false
	}
	d := col - width/2
	if d < 0 {
		d = -d
	}
	// The ring has to stop expanding once it reaches the edge, so the cycle is
	// the centre-to-edge distance. Going further leaves phases lit nowhere,
	// which reads as the ripple stalling before it restarts.
	span := width/2 + 1
	r := phase % span
	if d > r || d <= r-rippleBand {
		return 0, false
	}
	return (col + phase) % len(rainbowRamp), true
}

// supercodeRule draws the composer's top rule while orchestration mode is on.
//
// The ring is deliberately static. It used to travel with the frame ticker, and a
// row that repaints eight times a second directly above the input is the one place
// movement competes with typing: it pulls the eye to the border instead of the text
// and leaves the line looking unsettled while the user types. The mode still reads
// as active from the tag and the coloured ring; the animated half of the effect
// lives in the effort panel, where nothing else moves.
func (m *Model) supercodeRule(t Theme, w int, base lipgloss.Style) string {
	if w <= 0 {
		return ""
	}
	ramp := make([]lipgloss.Style, len(rainbowRamp))
	for i, c := range rainbowRamp {
		ramp[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Bold(true)
	}
	tag := lipgloss.NewStyle().Foreground(t.Text).Bold(true)

	label := " supercode "
	labelAt := 3
	showLabel := w > labelAt+lipgloss.Width(label)+2 && m.supercodeOn()

	// One fixed phase: the ring is a still image of the same wave.
	phase := w / 3

	// Unlit columns all share one style, so they are flushed as a single run
	// rather than a Render call each: at 200 columns the naive form spent more
	// time styling this one row than the entire rest of the frame.
	var b strings.Builder
	pending := 0 // unlit columns not yet written
	flush := func() {
		if pending > 0 {
			b.WriteString(base.Render(strings.Repeat(t.Glyphs.Rule, pending)))
			pending = 0
		}
	}
	for col := 0; col < w; {
		if showLabel && col == labelAt {
			flush()
			b.WriteString(tag.Render(label))
			col += lipgloss.Width(label)
			continue
		}
		if idx, lit := rippleColor(col, w, phase); lit {
			flush()
			b.WriteString(ramp[idx].Render(t.Glyphs.Rule))
		} else {
			pending++
		}
		col++
	}
	flush()
	return b.String()
}

// rippleShades is the violet ramp the surface is drawn from, deep water first.
// The two darkest entries sit close to a dark terminal background, which is what
// makes the troughs of the wave read as depth instead of as empty cells.
var rippleShades = []string{"#2A1B4A", "#3D2870", "#553A9E", "#7B5BD6", "#9B7BFF", "#C4A8FF"}

// rippleGlyphs is the density ramp the surface falls back to when the terminal has
// no colour to give it: ░▒▓█ is the set ASCII water renderers converge on, and
// each glyph is one step deeper.
var rippleGlyphs = []string{"░", "▒", "▓", "█"}

// rippleCellStyles and rippleGlyphStyles are built once: the surface paints every
// cell of the panel's dead space, and building a style per cell showed up in the
// frame budget.
var (
	rippleCellStyles  = rippleStyles(false)
	rippleGlyphStyles = rippleStyles(true)
)

func rippleStyles(foreground bool) []lipgloss.Style {
	out := make([]lipgloss.Style, len(rippleShades))
	for i, c := range rippleShades {
		if foreground {
			out[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Bold(true)
			continue
		}
		out[i] = lipgloss.NewStyle().Background(lipgloss.Color(c))
	}
	return out
}

// waveHash maps a cell to a stable value in [0,1). It is the only randomness in
// the effect: everything else is a closed form of the frame, so a repaint of the
// same frame is pixel-identical and the picture can be tested.
func waveHash(x, y int) float64 {
	// The lowbias32 finaliser: every input bit reaches every output bit, so
	// neighbouring cells do not share a bias.
	h := uint32(x)*0x2545F491 + uint32(y)*0x9E3779B1
	h ^= h >> 16
	h *= 0x7FEB352D
	h ^= h >> 15
	h *= 0x846CA68B
	h ^= h >> 16
	return float64(h) / float64(^uint32(0))
}

// rippleSources are the points the surface is disturbed from, as fractions of the
// panel's width and height. Two sources rather than one: a single source draws a
// clean bullseye, and it is the crossing of two wave trains that reads as water.
var rippleSources = [][3]float64{{0.34, 0.58, 1}, {0.78, 0.40, 0.45}}

// rippleLevel is the surface height at one cell, in [0,1].
func rippleLevel(col, row, cols, rows, frame int) float64 {
	height, _ := rippleCell(col, row, cols, rows, frame)
	return height
}

// rippleCell returns the surface height at a cell and how far that cell is from
// the nearest source. The distance is what the entrance animation uses: switching
// the mode on spreads the water outward from its sources instead of snapping to a
// finished panel.
func rippleCell(col, row, cols, rows, frame int) (height, dist float64) {
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}
	var sum float64
	nearest := math.MaxFloat64
	for i, src := range rippleSources {
		cx := src[0]*float64(cols) + 2.4*math.Sin(float64(frame)*0.022+float64(i)*2.1)
		cy := src[1]*float64(rows) + 1.8*math.Cos(float64(frame)*0.017+float64(i)*1.7)
		// Terminal cells are about twice as tall as they are wide. Without the
		// aspect correction every ring comes out as an ellipse.
		dx := (float64(col) - cx) * 0.5
		dy := float64(row) - cy
		d := math.Hypot(dx, dy)
		nearest = math.Min(nearest, d)
		// The decay is what keeps this water instead of a plasma: each source's
		// rings fade before they reach the other's, so the surface reads as two
		// disturbances rather than as an interference wallpaper. Fading in over the
		// first half-wavelength stops the impact point from being a solid bright
		// disc; water shows the disturbance, not a lamp.
		sum += src[2] * math.Sin(d*0.72-float64(frame)*0.36+float64(i)*1.3) * min(1, d*0.45) / (1 + d*0.34)
	}
	// A slow swell under the rings keeps the troughs from looking like holes.
	v := 0.5 + sum*0.62 + 0.05*math.Sin(float64(col)*0.06-float64(row)*0.17+float64(frame)*0.03)
	return min(1, max(0, v)), nearest
}

// rippleGrowFrames is how long the water takes to reach the far corner after the
// mode comes on: about two seconds at the spinner's 120 ms cadence. Long enough to
// read as a spread, short enough that the panel is not blank for a beat.
const rippleGrowFrames = 16

// rippleReveal is how far the water has spread, in [0,1]. A session that was
// already in the mode before the panel opened shows the finished surface: the
// entrance belongs to switching the mode on, not to opening the slider.
// noteRippleTransition starts the water's entrance whenever the mode changes:
// switching it on always grows the surface from its sources, and switching it off
// clears the entrance so the next switch-on grows again.
func (m *Model) noteRippleTransition() {
	on := m.supercodeOn()
	if on == m.rippleArmed {
		return
	}
	m.rippleArmed = on
	m.rippleStart = m.activityFrame
}

func (m *Model) rippleReveal() float64 {
	if !m.rippleArmed || !m.cfg.Animations {
		// With animations off there is no frame clock to grow the entrance, so the
		// surface is shown finished rather than never arriving at all.
		return 1
	}
	age := m.activityFrame - m.rippleStart
	if age >= rippleGrowFrames {
		return 1
	}
	x := float64(max(0, age)) / rippleGrowFrames
	// Ease out: the water covers ground quickly at first and slows as it reaches
	// the edges, the way a spreading ring does.
	return 1 - (1-x)*(1-x)
}

// rippleSpread is the farthest any cell of the area is from the nearest source, so
// reveal can be expressed as a fraction of this panel rather than in raw distances.
// It scans the grid instead of checking the corners: with two sources the farthest
// point sits on the ridge between them, which is usually not a corner.
func rippleSpread(cols, rows, frame int) float64 {
	if cols <= 0 || rows <= 0 {
		return 0
	}
	spread := 0.0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if _, d := rippleCell(c, r, cols, rows, frame); d > spread {
				spread = d
			}
		}
	}
	return spread
}

// rippleRows renders the surface across the panel's empty space. It is called with
// the rows it may fill, because the sources are placed relative to the visible
// area: a fixed grid would put them off-screen on a short terminal.
//
// reveal is how much of the area the water has reached, in [0,1]: at 0 nothing is
// drawn, at 1 the panel is full. That is the entrance — switching the mode on
// spreads the surface out from its sources instead of presenting it finished.
func rippleRows(width, rows, frame int, reveal float64) []string {
	if width <= 0 || rows <= 0 {
		return nil
	}
	reveal = min(1, max(0, reveal))
	reach := rippleSpread(width, rows, frame) * reveal
	// NO_COLOR strips backgrounds, which would leave an invisible surface. The
	// glyph fallback draws the crests as characters instead.
	plain := noColorEnv()
	out := make([]string, 0, rows)
	for r := 0; r < rows; r++ {
		var b strings.Builder
		for c := 0; c < width; c++ {
			v, d := rippleCell(c, r, width, rows, frame)
			if d > reach+1e-9 {
				// The wavefront has not arrived here yet.
				b.WriteString(" ")
				continue
			}
			// Feather the leading edge into the deepest shade, so the front is a
			// dark swelling rather than a hard cut across the panel. The band has
			// to scale with the reach: a fixed one swallows the whole surface while
			// the entrance is still small.
			if feather := math.Max(0.8, reach*0.15); d > reach-feather {
				v = min(v, 0.16)
			}
			if plain {
				// Monochrome keeps only the upper half of the surface as glyphs:
				// a full-height field of blocks would swamp the text above it, and
				// the crests are where the shape of the water actually reads.
				if v < 0.54 {
					b.WriteString(" ")
					continue
				}
				b.WriteString(rippleGlyphs[min(len(rippleGlyphs)-1, int((v-0.54)/0.46*float64(len(rippleGlyphs))))])
				continue
			}
			idx := min(len(rippleShades)-1, int(v*float64(len(rippleShades))))
			// A narrow band of heights is drawn as a glyph, so the surface carries
			// contour lines instead of being one smooth gradient. The band is a
			// ring in space, which is what makes it read as water rather than as a
			// colour field.
			if (v > 0.66 && v < 0.72) || (v > 0.80 && v < 0.86) {
				b.WriteString(rippleGlyphStyles[idx].Render("~"))
				continue
			}
			// A scattered highlight on the brightest crests.
			if v > 0.93 && waveHash(c+7, r*11+frame/5) > 0.80 {
				b.WriteString(rippleGlyphStyles[idx].Render("·"))
				continue
			}
			b.WriteString(rippleCellStyles[idx].Render(" "))
		}
		out = append(out, b.String())
	}
	return out
}

// ripplePulse walks the ramp up and back down, so the panel's border breathes with
// the surface instead of snapping between the darkest and brightest shade. The two
// darkest shades are skipped: a border that fades into the background looks like a
// rendering bug rather than an effect.
func ripplePulse(frame int) string {
	shades := rippleShades[2:]
	period := 2 * (len(shades) - 1)
	i := frame / 2 % period
	if i >= len(shades) {
		i = period - i
	}
	return shades[i]
}

// setSupercode applies the orchestration toggle. It is separate from
// toggleSupercode so the slider's last stop can turn it on without inverting it
// when it is already on.
func (m *Model) setSupercode(on, sessionOnly bool) tea.Cmd {
	sid := m.session.ID
	// Flip the local session first. The surface and the composer border are drawn
	// from this field, and they have to change on the keypress rather than a store
	// round trip later — otherwise sliding onto the stop shows nothing for a beat.
	// handleOp resyncs from the store if the write fails.
	if m.session.Metadata == nil {
		m.session.Metadata = map[string]any{}
	}
	m.session.Metadata["supercode"] = on
	m.noteRippleTransition()
	return asyncOp("set_supercode", func() (any, error) {
		if err := m.ctrl.SetSupercode(m.ctx, sid, on, sessionOnly); err != nil {
			return nil, err
		}
		return on, nil
	})
}

// wrapGuide word-wraps the guide line. wrapPlain hard-wraps, which is right for
// paths and identifiers — breaking those anywhere is harmless — but wrong for a
// sentence, where it drops half a word onto the next line.
func wrapGuide(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	return strings.Split(ansi.Wordwrap(s, width, "-"), "\n")
}

// renderEffortSlider draws the reasoning scale:
//
//	Faster                                     Smarter
//
//	───────────────▲───────────────┊────▲──────────────
//	  low  medium  high  xhigh  max  ┊  supercode
//	                                   xhigh + workflows
//
// The track and its marker come first and the labels sit under it, so the marker
// reads as a position on a scale rather than a cursor in a list. Faster and
// Smarter label the ends, because that is the trade the scale is making.
//
// supercode sits past a divider at the end: it is a mode, not a deeper setting of
// the same dial, and Claude Code hangs ultracode off the same track for the same
// reason. Moving the marker onto it switches the mode on — the stop is the toggle —
// and the mode's water then fills the rest of the panel.
//
// Width is the panel's outer width, not the drawing surface's. The rounded box
// spends two columns on its border and two on padding before the slider paints
// anything, so the usable width is w-6 — see the note below, this is the bug the
// layout used to have.
func (m *Model) renderEffortSlider(t Theme, w int) []string {
	levels := config.EffortLevels()
	n := len(levels)
	sel := min(max(0, m.effortSel), n)

	// The panel is a rounded box with one column of padding on each side, and
	// renderOverlay sizes it from the overlay width. Border and padding are spent
	// before the slider draws: at w columns the content box is w-6. Painting one
	// column more makes lipgloss wrap the row, which is what split the header and
	// pulled the labels off the track.
	inner := max(14, w-6)
	if inner < 22 {
		return m.renderCompactEffortSlider(t, inner, sel)
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(t.Secondary)
	dim := lipgloss.NewStyle().Foreground(t.Muted)
	pick := lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	guide := lipgloss.NewStyle().Foreground(t.Text)
	line := lipgloss.NewStyle().Foreground(t.Border)

	tailLabel := "supercode"
	// "  ┊  supercode": two columns before the tick, the tick itself, two after
	// it, then the label.
	tailW := 2 + 1 + 2 + len(tailLabel)

	names, showTail := effortLabelLayout(levels, inner, tailW)
	labelW := 0
	for _, lv := range names {
		labelW += len(lv)
	}

	// Levels are spread evenly across the columns left after the tail, which is
	// what puts the marker above its own label rather than merely somewhere along
	// the line.
	leftW := inner - 3
	if showTail {
		leftW = inner - tailW
	}
	gap := 2
	if n > 1 {
		if g := (leftW - labelW) / (n - 1); g > gap {
			gap = g
		}
	}

	starts := make([]int, n)
	var labels strings.Builder
	col := 0
	for i, lv := range names {
		if i > 0 {
			labels.WriteString(strings.Repeat(" ", gap))
			col += gap
		}
		starts[i] = col
		if i == sel {
			labels.WriteString(pick.Render(lv))
		} else {
			labels.WriteString(dim.Render(lv))
		}
		col += len(lv)
	}

	divider := col + 2
	superAt := divider + 3
	tailOn := sel == n || m.supercodeOn()
	tailStyle := dim
	if tailOn {
		tailStyle = pick
	}
	if showTail {
		// Same row as the levels: supercode is the last stop on the scale, not a
		// caption underneath it.
		labels.WriteString(strings.Repeat(" ", max(0, superAt-col)) + tailStyle.Render(tailLabel))
	}

	// The marker is clamped to the track. Without the clamp, a narrow panel put
	// the caret past the last column and the slider opened on supercode with no
	// visible selection at all.
	markAt := starts[min(sel, n-1)] + len(names[min(sel, n-1)])/2
	if sel == n {
		if showTail {
			markAt = superAt + len(tailLabel)/2
		} else {
			markAt = inner - 2
		}
	}
	markAt = min(max(0, markAt), inner-1)

	// The divider is a tick on the track rather than a gap in it, and the marker
	// wins if the two ever land on the same column.
	specials := map[int]string{markAt: pick.Render(t.Glyphs.Caret)}
	switch {
	case showTail && divider != markAt:
		specials[divider] = pick.Render("┊")
	case !showTail && inner >= 26 && markAt >= 2:
		// No room for the label, but the stop is still on the scale: a dim tick
		// keeps the last level from looking like the end of the track.
		specials[markAt-2] = dim.Render("┊")
	}

	head := title.Render("Faster")
	right := title.Render("Smarter")
	if pad := inner - lipgloss.Width(head) - lipgloss.Width(right); pad >= 1 {
		head += strings.Repeat(" ", pad) + right
	}
	out := []string{head, "", ruleRow(t, inner, line, specials), labels.String()}

	if showTail && tailOn {
		// Right-aligned, because the tail is the rightmost thing on the scale and
		// the caption belongs under it. Anchoring it at superAt instead ran it off
		// the content width and pushed the box open.
		caption := "xhigh + workflows"
		out = append(out, dim.Render(strings.Repeat(" ", max(0, inner-len(caption)))+caption))
	}

	out = append(out, "")
	body := max(10, inner-2)
	text := shortEffortGuide[levels[min(sel, n-1)]]
	if inner >= 46 {
		text = effortGuide[levels[min(sel, n-1)]]
	}
	if sel == n {
		text = "Orchestration mode: xhigh + multi-agent workflows."
		if inner >= 60 {
			text = "Orchestration mode: xhigh reasoning plus multi-agent workflows for every substantive task."
		}
	}
	for _, l := range wrapGuide(text, body) {
		out = append(out, "  "+guide.Render(l))
	}

	hint := "←/→ adjust · enter save as default · s this session only · esc cancel"
	if inner < 64 {
		hint = "←/→ adjust · enter save · s session · esc"
	}
	out = append(out, "")
	for _, l := range wrapGuide(hint, body) {
		out = append(out, "  "+dim.Render(l))
	}
	return fitRows(out, inner)
}

// effortLabelLayout picks the widest label set the panel can hold and whether the
// supercode tail comes with it. The stages degrade in order of information lost:
// full words with the mode name, full words alone, abbreviations with the mode
// name, abbreviations alone. The caller never has to reason about which one it
// got.
func effortLabelLayout(levels []string, inner, tailW int) ([]string, bool) {
	n := len(levels)
	full := levels
	short := shortEffortNames(levels)

	width := func(names []string) int {
		w := 2 * (n - 1) // the minimum one-column gap between each pair
		for _, lv := range names {
			w += len(lv)
		}
		return w
	}
	for _, stage := range []struct {
		names []string
		tail  bool
	}{
		{full, true}, {full, false}, {short, true}, {short, false},
	} {
		w := width(stage.names)
		if stage.tail {
			w += 3 + tailW
		} else {
			w += 3 // room for the supercode caret past the last label
		}
		if inner >= w {
			return stage.names, stage.tail
		}
	}
	return short, false
}

// shortEffortNames abbreviates the level labels for panels too narrow to carry
// the full words. An unknown level keeps its first three bytes rather than
// dropping out of the scale.
func shortEffortNames(levels []string) []string {
	short := map[string]string{"low": "lo", "medium": "med", "high": "hi", "xhigh": "xh", "max": "mx"}
	out := make([]string, len(levels))
	for i, lv := range levels {
		if s, ok := short[lv]; ok {
			out[i] = s
			continue
		}
		if len(lv) > 3 {
			lv = lv[:3]
		}
		out[i] = lv
	}
	return out
}

var shortEffortGuide = map[string]string{
	"low":    "Quick sketch",
	"medium": "Scoped work",
	"high":   "Verify carefully",
	"xhigh":  "Deeper reasoning",
	"max":    "Hard problems",
}

// renderCompactEffortSlider is the degenerate form. Below 22 columns the five
// labels cannot fit at any abbreviation, so the panel keeps the track, the marker
// and the name of the stop under them. Losing the scale is better than wrapping
// it into something that no longer lines up.
func (m *Model) renderCompactEffortSlider(t Theme, inner, sel int) []string {
	levels := config.EffortLevels()
	n := len(levels)
	name := levels[min(sel, n-1)]
	if sel == n {
		name = "supercode"
	}
	dim := lipgloss.NewStyle().Foreground(t.Muted)
	pick := lipgloss.NewStyle().Bold(true).Foreground(t.Primary)
	line := lipgloss.NewStyle().Foreground(t.Border)

	pos := 0
	if n > 0 {
		pos = sel * max(1, inner-1) / n
	}
	pos = min(max(0, pos), inner-1)
	// Center the name under the caret, but never let it run past the panel: at
	// the last stop the caret sits on the final column, and centering would push
	// the label off the edge.
	pad := min(max(0, pos-len(name)/2), max(0, inner-len(name)))
	out := []string{
		ruleRow(t, inner, line, map[int]string{pos: pick.Render(t.Glyphs.Caret)}),
		dim.Render(strings.Repeat(" ", pad) + name),
		"",
		dim.Render("←/→ · enter · esc"),
	}
	return fitRows(out, inner)
}

// fitRows enforces the panel's content width. The arithmetic above aims to fit,
// but a long label or an unusual theme glyph can still overrun at some width; a
// hard trim keeps every row from wrapping and breaking the box alignment.
func fitRows(lines []string, width int) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fitLine(l, width)
	}
	return out
}

// ruleRow draws a rule with a few styled glyphs placed at fixed columns. The
// runs between them are flushed whole: styling each column separately costs more
// than the rest of the frame put together on a wide terminal.
func ruleRow(t Theme, width int, run lipgloss.Style, specials map[int]string) string {
	var b strings.Builder
	pending := 0
	flush := func() {
		if pending > 0 {
			b.WriteString(run.Render(strings.Repeat(t.Glyphs.Rule, pending)))
			pending = 0
		}
	}
	for c := 0; c < width; c++ {
		if s, ok := specials[c]; ok {
			flush()
			b.WriteString(s)
			continue
		}
		pending++
	}
	flush()
	return b.String()
}
