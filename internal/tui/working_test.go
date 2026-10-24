package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

// pinPick makes the verb draw deterministic so these tests assert on the line
// rather than on whichever word a random draw produced.
func pinPick(t *testing.T, idx int) {
	t.Helper()
	prev := pickIndex
	pickIndex = func(int) int { return idx }
	t.Cleanup(func() { pickIndex = prev })
}

func TestFormatTokenCount(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"}, {834, "834"}, {999, "999"},
		{1000, "1k"}, {1200, "1.2k"}, {18300, "18.3k"},
		{1_000_000, "1M"}, {1_260_000, "1.3M"},
	}
	for _, c := range cases {
		if got := formatTokenCount(c.in); got != c.want {
			t.Fatalf("formatTokenCount(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The working line spaces its units ("1m 52s"); formatDuration is the compact
// variant used by tool metadata and must not be reused here.
func TestFormatWorkingElapsed(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""}, {500 * time.Millisecond, ""}, {time.Second, "1s"},
		{12 * time.Second, "12s"}, {59 * time.Second, "59s"},
		{time.Minute, "1m 0s"}, {time.Minute + 52*time.Second, "1m 52s"},
		{8*time.Minute + 39*time.Second, "8m 39s"},
		{time.Hour, "1h"}, {time.Hour + 2*time.Minute, "1h 2m"},
	}
	for _, c := range cases {
		if got := formatWorkingElapsed(c.in); got != c.want {
			t.Fatalf("formatWorkingElapsed(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The verb is drawn once per turn. A line that reshuffled its word every 120ms
// would be unreadable, and beginTurn is called twice per turn by design: the
// submit op opens the turn and EventAgentStarted arrives after it.
func TestWorkingVerbHoldsForTheTurnAndChangesOnTheNext(t *testing.T) {
	pinPick(t, 0)
	m := &Model{}
	m.beginTurn()
	first := m.turnVerb
	if first == "" {
		t.Fatal("beginTurn must draw a verb")
	}
	m.beginTurn()
	if m.turnVerb != first {
		t.Fatalf("verb changed mid-turn: %q -> %q", first, m.turnVerb)
	}

	m.busySince = time.Now()
	m.finishTurn()
	if m.turnVerb != "" {
		t.Fatal("finishTurn must clear the verb so the next turn redraws")
	}
	if m.completionVerb == "" {
		t.Fatal("finishTurn must choose a completion verb")
	}

	pinPick(t, 1)
	m.beginTurn()
	if m.turnVerb != workingVerbs[1] {
		t.Fatalf("next turn verb = %q, want %q", m.turnVerb, workingVerbs[1])
	}
	if m.completionVerb != "" {
		t.Fatal("a new turn must clear the settled line")
	}
}

// The marker is a fixed snowflake. It used to borrow the thinking spinner's
// bouncing frames, so a settled turn froze on whichever sparkle happened to be
// in flight — a dot on one turn, an asterisk on the next.
func TestWorkingGlyphIsAFixedSnowflake(t *testing.T) {
	th := ThemeByName("claude")
	m := &Model{session: domain.Session{ID: "s1"}, busy: true, busySince: time.Now().Add(-time.Minute)}
	m.beginTurn()
	for frame := 0; frame < 2*len(th.Glyphs.Spinner); frame++ {
		m.activityFrame = frame
		if got := stripANSI(m.renderWorkingLine(th)); !strings.HasPrefix(got, th.Glyphs.Thinking+" ") {
			t.Fatalf("frame %d drew %q, want the fixed snowflake", frame, got)
		}
	}
	// Reduced motion must keep the same marker rather than swapping in a dot.
	m.cfg.Animations = false
	if got := stripANSI(m.renderWorkingLine(th)); !strings.HasPrefix(got, th.Glyphs.Thinking+" ") {
		t.Fatalf("reduced motion drew %q, want the fixed snowflake", got)
	}

	m.busy = false
	m.finishTurn()
	if got := stripANSI(m.renderWorkingLine(th)); !strings.HasPrefix(got, th.Glyphs.Thinking+" ") {
		t.Fatalf("the settled line drew %q, want the fixed snowflake", got)
	}
}

// Providers report usage once per round, so the counter has to extrapolate the
// round in flight. Reasoning is counted because on a thinking model it is
// everything being generated before the first answer token.
func TestWorkingTokenCounterFollowsStreamedThinking(t *testing.T) {
	th := ThemeByName("claude")
	m := &Model{session: domain.Session{ID: "s1"}, busy: true, busySince: time.Now()}
	m.beginTurn()

	if got := stripANSI(m.renderWorkingLine(th)); strings.Contains(got, "tokens") {
		t.Fatalf("nothing has streamed yet, got %q", got)
	}
	m.streamReasoning = strings.Repeat("想", 200)
	if got := stripANSI(m.renderWorkingLine(th)); !strings.Contains(got, "↓ 200 tokens") {
		t.Fatalf("thinking tokens are missing from %q", got)
	}
	m.streamReasoning += strings.Repeat("想", 800)
	if got := stripANSI(m.renderWorkingLine(th)); !strings.Contains(got, "↓ 1k tokens") {
		t.Fatalf("the counter did not follow the stream: %q", got)
	}

	// The answer counts too, and it adds to what the provider already billed.
	m.turnUsage = domain.Usage{OutputTokens: 500}
	m.streamReasoning = ""
	m.streamContent = strings.Repeat("a", 400) // ≈ 100 tokens
	if got := stripANSI(m.renderWorkingLine(th)); !strings.Contains(got, "↓ 600 tokens") {
		t.Fatalf("billed total and live estimate did not add up: %q", got)
	}
}

// One ratio cannot serve both scripts: Latin runs about four characters to the
// token, CJK about one per character.
func TestStreamedTokensSplitsByScript(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0}, {"hi", 1}, {"hello world", 3}, {"想", 1}, {"思考内容", 4}, {"a想", 1 + 1},
	}
	for _, c := range cases {
		if got := streamedTokens(c.in); got != c.want {
			t.Fatalf("streamedTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := streamedTokens("hello", "世界"); got != 2+2 {
		t.Fatalf("parts must add up, got %d", got)
	}
}

func TestWorkingColorRampsFromAccentToError(t *testing.T) {
	// See components_test.go: NO_COLOR would strip the palette and turn every
	// comparison into "" == "".
	t.Setenv("CLICOLOR_FORCE", "1")
	th := ThemeByName("claude")
	accent, fail := hexColor(th.Primary), hexColor(th.Error)

	m := &Model{lastActivityAt: time.Now()}
	if got := hexColor(m.workingColor(th)); got != accent {
		t.Fatalf("fresh activity should keep the accent, got %s want %s", got, accent)
	}

	m.lastActivityAt = time.Now().Add(-stallAfter - stallRamp - time.Second)
	if got := hexColor(m.workingColor(th)); got != fail {
		t.Fatalf("a long stall should reach the error color, got %s want %s", got, fail)
	}

	m.lastActivityAt = time.Now().Add(-stallAfter - stallRamp/2)
	if mid := hexColor(m.workingColor(th)); mid == accent || mid == fail {
		t.Fatalf("mid-ramp color %s should sit between %s and %s", mid, accent, fail)
	}
}

// The NO_COLOR palette is built from lipgloss.NoColor, which carries no
// components to interpolate. This is the path that would divide by nothing.
func TestBlendColorSurvivesNoColor(t *testing.T) {
	nc := lipgloss.NoColor{}
	red := lipgloss.Color("#FF0000")
	if _, ok := blendColor(nc, red, 0.5).(lipgloss.NoColor); !ok {
		t.Fatal("blending from NoColor must pass it through")
	}
	if _, ok := blendColor(red, nc, 1).(lipgloss.NoColor); ok {
		t.Fatal("blending into NoColor must keep the base color, not the target")
	}
}

func TestUsageEventFeedsTheWorkingCounter(t *testing.T) {
	m := &Model{session: domain.Session{ID: "s1"}}
	m.handleRuntime(domain.NewEvent(domain.EventUsageChanged, "s1", domain.Usage{OutputTokens: 18300}))
	if m.turnUsage.OutputTokens != 18300 {
		t.Fatalf("turnUsage = %#v, want 18300 output tokens", m.turnUsage)
	}
	// Another session's usage must not leak into this session's line.
	m.handleRuntime(domain.NewEvent(domain.EventUsageChanged, "s2", domain.Usage{OutputTokens: 1}))
	if m.turnUsage.OutputTokens != 18300 {
		t.Fatalf("a foreign session's usage overwrote this one: %#v", m.turnUsage)
	}
}

func TestWorkingLineLifecycle(t *testing.T) {
	th := ThemeByName("claude")
	pinPick(t, 0)
	m := &Model{session: domain.Session{ID: "s1"}}

	if m.workingLineVisible() || m.renderWorkingLine(th) != "" {
		t.Fatal("an idle model must neither reserve nor draw a working line")
	}

	m.busy, m.busySince = true, time.Now().Add(-90*time.Second)
	m.beginTurn()
	if !m.workingLineVisible() {
		t.Fatal("a busy model must reserve the working row")
	}
	live := stripANSI(m.renderWorkingLine(th))
	if !strings.Contains(live, m.turnVerb+"…") {
		t.Fatalf("live line = %q, want the verb", live)
	}
	if !strings.Contains(live, "1m 30s") {
		t.Fatalf("live line = %q, want the elapsed time", live)
	}
	if strings.Contains(live, "tokens") {
		t.Fatal("a turn with no reported usage must not claim a token count")
	}

	m.turnUsage = domain.Usage{OutputTokens: 18300}
	live = stripANSI(m.renderWorkingLine(th))
	if !strings.Contains(live, "↓ 18.3k tokens") {
		t.Fatalf("live line = %q, want the token counter", live)
	}

	m.busy = false
	m.finishTurn()
	settled := stripANSI(m.renderWorkingLine(th))
	if !strings.Contains(settled, m.completionVerb+" for ") {
		t.Fatalf("settled line = %q, want the past-tense form", settled)
	}
	if strings.Contains(settled, "tokens") {
		t.Fatalf("the settled line must drop the live segments: %q", settled)
	}
	if !m.workingLineVisible() {
		t.Fatal("the settled line must remain visible until the next turn")
	}
}

// Switching sessions must drop another conversation's per-turn state, but
// reloading the same session after a turn finishes must keep the settled line —
// EventAgentFinished reloads the same session, so keying this on "setSession was
// called" rather than "the session changed" would erase it on arrival.
func TestWorkingLineIsScopedToItsSession(t *testing.T) {
	pinPick(t, 0)
	// setSession reaches into the conversation, so this needs a real one rather
	// than the zero value.
	m := &Model{session: domain.Session{ID: "s1"}, conv: NewConversation()}
	m.busy, m.busySince = true, time.Now()
	m.beginTurn()
	m.finishTurn()
	m.busy = false
	if m.completionVerb == "" {
		t.Fatal("setup: expected a settled line")
	}

	m.setSession(domain.Session{ID: "s1"})
	if m.completionVerb == "" {
		t.Fatal("reloading the same session erased the settled line")
	}

	m.setSession(domain.Session{ID: "s2"})
	if m.completionVerb != "" || m.turnVerb != "" || m.workingLineVisible() {
		t.Fatal("switching sessions must drop the previous turn's working line")
	}
}

func TestStatusRowKeepsOnlyTheInterruptHintWhileBusy(t *testing.T) {
	m := layoutModel(t)
	th := ThemeByName("claude")
	m.busy = true
	got := stripANSI(m.busyIndicator(th))
	if got != "esc to interrupt" {
		t.Fatalf("busy indicator = %q, want just the interrupt hint", got)
	}
}

// The working line adds a row to renderBase's hand-rolled arithmetic, so the
// exact-height contract has to hold while a turn is running, not only at rest.
func TestLayoutFitsEveryTerminalSizeWhileBusy(t *testing.T) {
	pinPick(t, 0)
	for _, w := range []int{40, 60, 100, 140} {
		for _, h := range []int{10, 24, 40} {
			m := layoutModel(t)
			m.cfg.Theme = "claude"
			m.width, m.height = w, h
			m.prompt.SetValue("next instruction")
			m.busy, m.busySince = true, time.Now().Add(-90*time.Second)
			m.beginTurn()
			m.turnUsage = domain.Usage{OutputTokens: 18300}
			m.resize()

			out := m.renderBase(themeFor(m.cfg))
			lines := strings.Split(out, "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d busy: got %d lines, want %d", w, h, len(lines), h)
			}
			for i, l := range lines {
				if got := lipgloss.Width(l); got != w {
					t.Fatalf("%dx%d busy line %d: width %d, want %d\n%q", w, h, i, got, w, stripANSI(l))
				}
			}
			if !strings.Contains(stripANSI(out), workingVerbs[0]+"…") {
				t.Fatalf("%dx%d busy: the working line is missing from the view", w, h)
			}
		}
	}
}

// The sweep has to match Claude Code's arithmetic, not merely look like a sweep:
// the cycle spans the text width plus twenty columns, and the index counts down
// from width+10 — the descending count is what makes the highlight travel
// right-to-left.
func TestComputeGlimmerIndexMatchesClaudeCode(t *testing.T) {
	const width = 10
	if got := computeGlimmerIndex(0, width); got != width+10 {
		t.Fatalf("tick 0 = %d, want %d", got, width+10)
	}
	if got := computeGlimmerIndex(1, width); got != width+9 {
		t.Fatalf("ticks must step down by one, got %d", got)
	}
	if got := computeGlimmerIndex(width+20, width); got != width+10 {
		t.Fatalf("the cycle must wrap at width+20, got %d", got)
	}
}

// The band is three columns wide, and a glyph that straddles an edge is pulled
// wholly into the highlight rather than cut — so a wide or multibyte character
// never gets split across the style boundary.
func TestShimmerSegmentsBandThreeColumns(t *testing.T) {
	text := "Reading" // width 7
	before, hot, after := shimmerSegments(text, 4)
	if before != "Rea" || hot != "din" || after != "g" {
		t.Fatalf("segments = %q / %q / %q, want \"Rea\" / \"din\" / \"g\"", before, hot, after)
	}
	if got := displayWidth(before) + displayWidth(hot) + displayWidth(after); got != displayWidth(text) {
		t.Fatalf("split covers %d columns, want %d", got, displayWidth(text))
	}
}

// Off the text the band highlights nothing and the whole string stays in the
// leading segment, which is how the -100 sentinel disables the effect.
func TestShimmerSegmentsOffText(t *testing.T) {
	for _, idx := range []int{shimmerDisabled, 99} {
		if b, s, a := shimmerSegments("Reading", idx); b != "Reading" || s != "" || a != "" {
			t.Fatalf("idx %d split = %q / %q / %q", idx, b, s, a)
		}
	}
}

// The highlight has to actually travel. A band pinned to one column would be a
// static word with one oddly-colored character.
func TestShimmerBandTravels(t *testing.T) {
	text := "Accomplishing…"
	seen := map[int]bool{}
	for frame := 0; frame < 40; frame++ {
		idx := computeGlimmerIndex(glimmerTick(frame), displayWidth(text))
		before, _, _ := shimmerSegments(text, idx)
		seen[displayWidth(before)] = true
	}
	if len(seen) < 5 {
		t.Fatalf("the band reached only %d distinct positions across 40 frames", len(seen))
	}
}

// Reduced motion drops the sweep. It must not dim the verb in the process:
// Claude Code's sentinel makes the whole string the leading segment, which would
// grey out a line that is still meant to read as active.
func TestShimmerSplitsTheVerbOnlyWhenAnimating(t *testing.T) {
	th := ThemeByName("claude")
	spans := func(s string) int { return strings.Count(s, "\x1b[") }

	static := &Model{cfg: config.Config{Animations: false}, lastActivityAt: time.Now()}
	if got := stripANSI(static.shimmerVerb(th, "Herding…")); got != "Herding…" {
		t.Fatalf("reduced motion lost the verb text: %q", got)
	}
	baseline := spans(static.shimmerVerb(th, "Herding…"))

	// The band spends part of its cycle past the end of the word, where nothing
	// is highlighted and the verb renders as one dim span — so the sweep has to
	// be sampled across the cycle, not at a single frame.
	for frame := 0; frame < 60; frame++ {
		m := &Model{cfg: config.Config{Animations: true}, activityFrame: frame, lastActivityAt: time.Now()}
		if spans(m.shimmerVerb(th, "Herding…")) > baseline {
			return
		}
	}
	t.Fatal("no frame in the cycle split the verb; the sweep never renders")
}

// Compaction runs before the turn's own message is even sent, and it produces no
// output while it works. The working line has to name the phase, or a turn that
// spends its first seconds summarising reads as one that never answered.
func TestWorkingLineNamesCompaction(t *testing.T) {
	pinPick(t, 0)
	th := ThemeByName("claude")
	m := &Model{session: domain.Session{ID: "s1"}, busy: true, busySince: time.Now().Add(-2 * time.Second)}
	m.beginTurn()
	verb := m.turnVerb

	m.noteCompaction("started")
	if got := stripANSI(m.renderWorkingLine(th)); !strings.Contains(got, "Compacting context") {
		t.Fatalf("compaction is not named in %q", got)
	}
	// The turn's own verb belongs to the model thinking, which has not started.
	if got := stripANSI(m.renderWorkingLine(th)); strings.Contains(got, verb) {
		t.Fatalf("the turn verb leaked into the compaction line: %q", got)
	}

	m.noteCompaction("completed")
	if m.compacting {
		t.Fatal("completion left the line stuck in the compaction phase")
	}
	if got := stripANSI(m.renderWorkingLine(th)); !strings.Contains(got, verb) {
		t.Fatalf("the turn verb did not come back: %q", got)
	}
}

// A compaction is silent by design, so its own lack of output must not be read
// as a stalled turn — that ramp recolors the line as an error twenty seconds in.
func TestCompactionDoesNotRampTowardTheErrorColor(t *testing.T) {
	m := &Model{busy: true, busySince: time.Now()}
	m.beginTurn()
	m.noteCompaction("started")
	m.lastActivityAt = time.Now().Add(-(stallAfter + stallRamp))
	if got := m.stallIntensity(); got != 0 {
		t.Fatalf("stall intensity during compaction = %v, want 0", got)
	}

	// Without the phase, the same idle time is a stall.
	m.noteCompaction("completed")
	if got := m.stallIntensity(); got != 1 {
		t.Fatalf("stall intensity after compaction = %v, want 1", got)
	}
}

// A failed compaction aborts the turn without a terminal compaction event, so
// ending the turn has to clear the phase as well.
func TestTurnEndClearsTheCompactionPhase(t *testing.T) {
	m := &Model{busy: true, busySince: time.Now()}
	m.beginTurn()
	m.noteCompaction("started")
	m.finishTurn()
	if m.compacting {
		t.Fatal("finishTurn left the compaction phase armed")
	}

	m.noteCompaction("started")
	m.clearWorkingLine()
	if m.compacting {
		t.Fatal("clearWorkingLine left the compaction phase armed")
	}

	// An unknown payload is treated as the end rather than as a reason to stay.
	m.noteCompaction("failed")
	if m.compacting {
		t.Fatal("an unrecognised compaction payload did not close the phase")
	}
}

// The warning has to say where the automatic compaction starts, not only how
// full the context already is: the percentage alone explains nothing.
func TestContextWarningToastNamesBothThresholds(t *testing.T) {
	m := &Model{session: domain.Session{ID: "s"}}
	m.handleRuntime(domain.Event{Kind: domain.EventContextWarning, SessionID: "s", Data: domain.ContextWarning{Percent: 54, CompactPercent: 62}})
	if !strings.Contains(m.toast, "54%") || !strings.Contains(m.toast, "62%") {
		t.Fatalf("toast = %q, want both the fill and the compaction threshold", m.toast)
	}

	// A warning about another session is not this session's business.
	m.toast = ""
	m.handleRuntime(domain.Event{Kind: domain.EventContextWarning, SessionID: "other", Data: domain.ContextWarning{Percent: 54, CompactPercent: 62}})
	if m.toast != "" {
		t.Fatalf("a foreign session's warning surfaced in this one: %q", m.toast)
	}
}

// /compact runs outside a turn, and a failed one never reports back. Opening the
// next turn has to drop the leftover phase — while a compaction that the new turn
// already announced must survive the submit op's second beginTurn.
func TestBeginTurnClearsAStaleCompactionPhase(t *testing.T) {
	pinPick(t, 0)
	m := &Model{busy: true, busySince: time.Now()}
	m.noteCompaction("started") // a /compact that never finished
	m.beginTurn()
	if m.compacting {
		t.Fatal("the new turn inherited the failed /compact's phase")
	}

	// The turn's own compaction arrives after beginTurn; the duplicate beginTurn
	// the submit op issues must not erase it.
	m.noteCompaction("started")
	m.beginTurn()
	if !m.compacting {
		t.Fatal("a repeated beginTurn dropped this turn's compaction")
	}
}
