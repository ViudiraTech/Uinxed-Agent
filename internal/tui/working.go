package tui

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// workingVerbs are the present participles the working line draws from, in the
// spirit of Claude Code's spinner verbs: a turn that lasts a minute should read
// as something happening, not as a frozen "Working". Drawn from Claude Code's
// published spinner-verb tables.
var workingVerbs = []string{
	"Accomplishing", "Actioning", "Actualizing", "Architecting", "Baking", "Beaming",
	"Beboppin'", "Befuddling", "Billowing", "Blanching", "Bloviating", "Boogieing",
	"Boondoggling", "Booping", "Bootstrapping", "Brewing", "Burrowing", "Calculating",
	"Canoodling", "Caramelizing", "Cascading", "Catapulting", "Cerebrating",
	"Channeling", "Choreographing", "Churning", "Clauding", "Coalescing", "Cogitating",
	"Combobulating", "Composing", "Computing", "Concocting", "Considering",
	"Contemplating", "Cooking", "Crafting", "Creating", "Crunching", "Crystallizing",
	"Cultivating", "Deciphering", "Deliberating", "Determining", "Dilly-dallying",
	"Discombobulating", "Doing", "Doodling", "Drizzling", "Ebbing", "Effecting",
	"Elucidating", "Embellishing", "Enchanting", "Envisioning", "Evaporating",
	"Fermenting", "Fiddle-faddling", "Finagling", "Flambéing", "Flibbertigibbeting",
	"Flowing", "Flummoxing", "Fluttering", "Forging", "Forming", "Frolicking",
	"Frosting", "Gallivanting", "Galloping", "Garnishing", "Generating", "Germinating",
	"Gitifying", "Grooving", "Gusting", "Hashing", "Hatching", "Herding", "Honking",
	"Hullaballooing", "Hyperspacing", "Ideating", "Imagining", "Improvising",
	"Incubating", "Inferring", "Infusing", "Ionizing", "Jitterbugging", "Marinating",
	"Moonwalking", "Mulling", "Musing", "Noodling", "Oscillating", "Perambulating",
	"Percolating", "Philosophizing", "Plotting", "Pondering", "Puzzling", "Puttering",
	"Ruminating", "Scheming", "Scribbling", "Shucking", "Simmering", "Spelunking",
	"Spinning", "Stewing", "Synthesizing", "Thinking", "Tinkering", "Transmuting",
	"Twisting", "Untangling", "Vibing", "Weaving", "Whirring", "Wrangling", "Zesting",
	"Zigzagging",
}

// completionVerbs are Claude Code's past-tense set, shown once a turn settles.
var completionVerbs = []string{"Baked", "Brewed", "Churned", "Cogitated", "Cooked", "Crunched", "Sautéed", "Worked"}

// pickIndex is a seam so tests can pin a verb instead of asserting against a
// random draw. math/rand/v2's top-level functions are goroutine-safe, which
// matters here: Update runs on the Bubble Tea goroutine but tests call it too.
var pickIndex = func(n int) int { return rand.IntN(n) }

const (
	// stallAfter is how long the model may produce nothing before the working
	// line starts warning that it is stuck.
	stallAfter = 20 * time.Second
	// stallRamp is how long the blend from the accent toward the error color
	// takes once stalling has begun.
	stallRamp = 40 * time.Second
)

// beginTurn opens a new turn: it draws the verb once, so it stays put for the
// whole turn rather than reshuffling every frame, and zeroes the usage counter
// the runtime reports cumulatively.
//
// It is idempotent within a turn. A submit sets busy immediately and the
// matching EventAgentStarted arrives afterwards over the event stream; rolling
// the verb again there would visibly change it a beat into the turn.
func (m *Model) beginTurn() {
	if m.turnVerb != "" {
		return
	}
	m.turnVerb = workingVerbs[pickIndex(len(workingVerbs))]
	m.completionVerb = ""
	m.turnElapsed = 0
	m.turnUsage = domain.Usage{}
	m.lastActivityAt = time.Now()
}

// finishTurn freezes what the settled line needs. Clearing turnVerb is what
// lets the next beginTurn draw a fresh one.
func (m *Model) finishTurn() {
	if m.busySince.IsZero() {
		return
	}
	m.completionVerb = completionVerbs[pickIndex(len(completionVerbs))]
	m.turnElapsed = time.Since(m.busySince)
	m.turnVerb = ""
}

// clearWorkingLine drops per-turn state when the turn no longer applies — the
// user switched sessions, so a completion line about another conversation
// would be a lie.
func (m *Model) clearWorkingLine() {
	m.turnVerb = ""
	m.completionVerb = ""
	m.turnElapsed = 0
	m.turnUsage = domain.Usage{}
	m.lastActivityAt = time.Time{}
}

// touchActivity records that the model produced something, which is what the
// stall ramp measures against.
func (m *Model) touchActivity() { m.lastActivityAt = time.Now() }

// workingLineVisible reports whether the working line occupies a row. It is
// consulted before rendering so the layout can reserve the row rather than have
// the transcript jump when a turn starts.
func (m *Model) workingLineVisible() bool {
	return m.busy || m.completionVerb != ""
}

// renderWorkingLine draws the Claude Code status line: the live form names what
// is happening and how long it has been happening,
//
//	✻ Herding… (8m 39s · ↓ 834 tokens)
//
// and the settled form reports it in the past tense,
//
//	✻ Worked for 1m 52s
func (m *Model) renderWorkingLine(t Theme) string {
	glyph := t.Glyphs.spinner(m.activityFrame)
	if !m.cfg.Animations {
		// Reduced motion: Claude Code holds a single dot instead of animating.
		// With the tick stopped the sparkle would sit frozen on one frame of a
		// grow-and-shrink sequence, which reads as a rendering glitch.
		glyph = "●"
	}
	style := lipgloss.NewStyle().Foreground(m.workingColor(t)).Bold(true)

	if !m.busy {
		if m.completionVerb == "" {
			return ""
		}
		elapsed := formatWorkingElapsed(m.turnElapsed)
		if elapsed == "" {
			return style.Render(glyph + " " + m.completionVerb)
		}
		return style.Render(glyph + " " + m.completionVerb + " for " + elapsed)
	}

	var parts []string
	if elapsed := formatWorkingElapsed(time.Since(m.busySince)); elapsed != "" {
		parts = append(parts, elapsed)
	}
	if n := m.turnUsage.OutputTokens; n > 0 {
		parts = append(parts, "↓ "+formatTokenCount(n)+" tokens")
	}
	// Reasoning streams ahead of the answer on thinking models, so this is the
	// window where "thinking" is literally what is happening.
	if m.streamReasoning != "" && m.streamContent == "" {
		parts = append(parts, "thinking")
	}

	verb := m.turnVerb
	if verb == "" {
		// A turn whose EventAgentStarted was missed (a resumed run) still needs
		// a label; the plain fallback beats an empty line.
		verb = "Working"
	}
	line := glyph + " " + verb + "…"
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, " · ") + ")"
	}
	return style.Render(line)
}

// workingColor blends the accent toward the error color as a stalled turn drags
// on, so a wedged provider looks different from a slow one.
func (m *Model) workingColor(t Theme) color.Color {
	if m.lastActivityAt.IsZero() {
		return t.Primary
	}
	since := time.Since(m.lastActivityAt)
	if since <= stallAfter {
		return t.Primary
	}
	return blendColor(t.Primary, t.Error, float64(since-stallAfter)/float64(stallRamp))
}

// blendColor interpolates between two theme colors. The NO_COLOR palette is
// built from lipgloss.NoColor, which carries no components to interpolate, so a
// color-free run keeps the base color rather than dividing by nothing.
func blendColor(from, to color.Color, f float64) color.Color {
	if _, ok := from.(lipgloss.NoColor); ok {
		return from
	}
	if _, ok := to.(lipgloss.NoColor); ok {
		return from
	}
	if f <= 0 {
		return from
	}
	if f >= 1 {
		return to
	}
	fr, fg, fb, _ := from.RGBA()
	tr, tg, tb, _ := to.RGBA()
	// RGBA() returns 16-bit components; /257 is the exact 8-bit rescale.
	mix := func(a, b uint32) uint8 {
		return uint8((float64(a)*(1-f) + float64(b)*f) / 257)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", mix(fr, tr), mix(fg, tg), mix(fb, tb)))
}

// formatTokenCount renders a token count the way Claude Code does: exact under a
// thousand, then a compact suffix with one decimal.
func formatTokenCount(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10)
	case n < 1_000_000:
		return trimTrailingZero(float64(n)/1000) + "k"
	default:
		return trimTrailingZero(float64(n)/1_000_000) + "M"
	}
}

func trimTrailingZero(f float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0")
}

// formatWorkingElapsed renders an elapsed span with a space between units
// ("1m 52s"), which is Claude Code's working-line form. formatDuration in
// view.go is the compact variant ("1m52s") used by tool metadata.
//
// Sub-second spans return empty so a just-started turn does not flicker through
// "0s" before it has anything to report.
func formatWorkingElapsed(d time.Duration) string {
	if d < time.Second {
		return ""
	}
	d = d.Round(time.Second)
	h := d / time.Hour
	min := (d % time.Hour) / time.Minute
	s := (d % time.Minute) / time.Second
	switch {
	case h > 0 && min > 0:
		return fmt.Sprintf("%dh %dm", h, min)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case min > 0:
		return fmt.Sprintf("%dm %ds", min, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
