package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/config"
)

// The highlight has to start and end exactly on the dragged cells: a selection
// that bleeds a column either way copies text the user never pointed at.
func TestPaintColumnsHighlightsTheSelectedCellsOnly(t *testing.T) {
	on, off := selectionPaint(ThemeByName("claude").SelectionBg)
	line := "hello world"

	if got, want := paintColumns(line, 6, 11, on, off), "hello "+on+"world"+off; got != want {
		t.Fatalf("paintColumns = %q, want %q", got, want)
	}
	// An empty span is not a selection and must not emit styling at all.
	if got := paintColumns(line, 3, 3, on, off); got != line {
		t.Fatalf("empty span rewrote the line: %q", got)
	}
	if got := paintColumns(line, 5, 5, on, off); got != line {
		t.Fatalf("zero-width span rewrote the line: %q", got)
	}
}

// Rendered lines carry a reset at the end of every styled segment. A reset inside
// the selection would drop the background halfway across it, so the highlight is
// re-applied after each one.
func TestPaintColumnsSurvivesAnEmbeddedReset(t *testing.T) {
	on, off := selectionPaint(ThemeByName("claude").SelectionBg)
	line := "\x1b[31mabc\x1b[0mdef"
	want := "\x1b[31m" + on + "abc\x1b[0m" + on + "def" + off
	if got := paintColumns(line, 0, 6, on, off); got != want {
		t.Fatalf("paintColumns = %q, want %q", got, want)
	}
	// A selection that ends before the reset leaves that segment untouched.
	got := paintColumns(line, 0, 3, on, off)
	if strings.Count(got, on) != 1 {
		t.Fatalf("the highlight was re-applied outside the span: %q", got)
	}
}

// Half of a wide glyph cannot be painted, so a selection that starts or ends
// inside one takes the whole character. This is the CJK case: every Chinese
// character is two cells wide.
func TestSelectionPullsWideGlyphsWholly(t *testing.T) {
	on, off := selectionPaint(ThemeByName("claude").SelectionBg)
	line := "中文abc"

	// Columns 0-1 are the first half of 中; the glyph must come along.
	if got, want := paintColumns(line, 0, 1, on, off), on+"中"+off+"文abc"; got != want {
		t.Fatalf("paintColumns = %q, want %q", got, want)
	}
	// Extraction has to agree with what was painted.
	if got := plainColumns(line, 0, 1); got != "中" {
		t.Fatalf("plainColumns = %q, want the whole glyph", got)
	}
	if got := plainColumns(line, 2, 3); got != "文" {
		t.Fatalf("plainColumns = %q, want the whole glyph", got)
	}
	if got := plainColumns(line, 4, 7); got != "abc" {
		t.Fatalf("plainColumns = %q, want %q", got, "abc")
	}
	// Edges past the end of the line are clamped rather than panicking.
	if got := plainColumns(line, 4, 99); got != "abc" {
		t.Fatalf("plainColumns beyond the line = %q", got)
	}
}

func TestPlainColumnsDropsStyling(t *testing.T) {
	line := "\x1b[2m  \x1b[0m\x1b[1mbold\x1b[0m tail"
	if got := plainColumns(line, 0, 12); got != "  bold tail" {
		t.Fatalf("plainColumns = %q", got)
	}
}

// A drag copies exactly the rows and columns it covered, and each row is
// right-trimmed: the frame is padded to the terminal width, and that padding is
// not text the user selected.
func TestSelectionReportsTheDraggedText(t *testing.T) {
	m := &Model{cfg: config.Config{Mouse: true}, width: 20, height: 6}
	m.frameText = []string{"  ✻ Working", "  first line     ", "  second line    ", "", "", ""}

	m.startSelection(selectionPoint{row: 1, col: 2})
	m.dragSelection(selectionPoint{row: 2, col: 8})
	if got, want := m.selectedText(), "first line\n  second"; got != want {
		t.Fatalf("selectedText = %q, want %q", got, want)
	}

	// Dragging upward selects the same block: normalization is by reading order,
	// not by drag direction.
	m.startSelection(selectionPoint{row: 2, col: 8})
	m.dragSelection(selectionPoint{row: 1, col: 2})
	if got, want := m.selectedText(), "first line\n  second"; got != want {
		t.Fatalf("upward drag = %q, want %q", got, want)
	}
}

// Blank edges are an artifact of ending a drag past the content, so they are
// dropped. Leading indentation is not: it is what keeps a copied code block
// readable, and the transcript's own gutter rides along with it.
func TestSelectionDropsBlankEdgesButKeepsIndentation(t *testing.T) {
	m := &Model{width: 20, height: 4}
	m.frameText = []string{"", "  alpha  ", "", ""}
	m.startSelection(selectionPoint{row: 0, col: 0})
	m.dragSelection(selectionPoint{row: 3, col: 19})
	if got := m.selectedText(); got != "  alpha" {
		t.Fatalf("selectedText = %q, want %q", got, "  alpha")
	}
}

// A click is not a selection: press and release on one cell must leave no
// highlight and no clipboard write.
func TestClickWithoutDragIsNotASelection(t *testing.T) {
	m := &Model{cfg: config.Config{Mouse: true}, width: 20, height: 4}
	m.frameText = []string{"text", "", "", ""}

	m.startSelection(selectionPoint{row: 0, col: 1})
	m.dragSelection(selectionPoint{row: 0, col: 1})
	if m.hasSelection() {
		t.Fatal("a press and release on one cell became a selection")
	}
	if cmd := m.endSelection(); cmd != nil {
		t.Fatal("a click copied something")
	}
	if m.sel.active {
		t.Fatal("the drag was left open after release")
	}
}

// The drag ends by copying, and the toast says what left the terminal without
// dumping the text back over the screen it came from.
func TestDragReleaseCopiesAndReports(t *testing.T) {
	m := &Model{cfg: config.Config{Mouse: true}, width: 40, height: 4}
	m.frameText = []string{"  alpha", "  beta", "", ""}

	m.startSelection(selectionPoint{row: 0, col: 2})
	m.dragSelection(selectionPoint{row: 1, col: 6})
	if cmd := m.endSelection(); cmd == nil {
		t.Fatal("finishing a drag returned no copy command")
	}
	if !strings.Contains(m.toast, "copied 2 lines") {
		t.Fatalf("toast = %q", m.toast)
	}
	// The highlight stays after the copy so the reader can see what it covered.
	if !m.hasSelection() {
		t.Fatal("the selection disappeared on release")
	}

	// A single-line selection counts characters instead.
	m.clearSelection()
	m.startSelection(selectionPoint{row: 0, col: 2})
	m.dragSelection(selectionPoint{row: 0, col: 6})
	m.endSelection()
	if !strings.Contains(m.toast, "copied 5 chars") {
		t.Fatalf("toast = %q", m.toast)
	}
}

// Only the transcript is selectable. A press on the composer or on a tool card
// belongs to that control; a press on plain transcript or on its background
// region starts a drag.
func TestOnlyTheTranscriptStartsASelection(t *testing.T) {
	m := &Model{cfg: config.Config{Mouse: true}, width: 40, height: 20}
	press := tea.Mouse{X: 3, Y: 3, Button: tea.MouseLeft}

	if !m.canSelect(press, Region{Kind: ActionChat}, true) {
		t.Fatal("a press on the transcript did not start a selection")
	}
	if !m.canSelect(press, Region{}, false) {
		t.Fatal("a press on unpainted transcript background did not start a selection")
	}
	if m.canSelect(press, Region{Kind: ActionPrompt}, true) {
		t.Fatal("the composer was treated as selectable text")
	}
	if m.canSelect(press, Region{Kind: ActionTool}, true) {
		t.Fatal("a tool card was treated as selectable text")
	}
	if m.canSelect(tea.Mouse{X: 3, Y: 3, Button: tea.MouseRight}, Region{Kind: ActionChat}, true) {
		t.Fatal("a right click started a selection")
	}
	m.overlay = overlayHelp
	if m.canSelect(press, Region{Kind: ActionChat}, true) {
		t.Fatal("a selection started underneath an overlay")
	}
}

// The frame is rebuilt every animation tick, and the highlight is computed from
// the cached copy each time; painting the already-painted string would stack one
// background sequence on another and eventually corrupt the line.
func TestPaintFrameDoesNotCompound(t *testing.T) {
	th := ThemeByName("claude")
	on, _ := selectionPaint(th.SelectionBg)
	m := &Model{width: 20, height: 3}
	m.frameText = []string{"hello world", "", ""}
	m.startSelection(selectionPoint{row: 0, col: 0})
	m.dragSelection(selectionPoint{row: 0, col: 4})

	first := m.paintFrame(th)
	second := m.paintFrame(th)
	if first != second {
		t.Fatalf("repaint changed the frame:\n first=%q\nsecond=%q", first, second)
	}
	if got := strings.Count(first, on); got != 1 {
		t.Fatalf("the frame carries %d highlight sequences, want 1: %q", got, first)
	}
	// The cached frame itself is never mutated.
	if m.frameText[0] != "hello world" {
		t.Fatalf("the cached frame was painted in place: %q", m.frameText[0])
	}
}

// The end-to-end path: a real frame is rendered, the cached lines are what the
// paint pass reads, and the cells a drag covers come back as the same text.
func TestViewPaintsTheDragOverTheRealFrame(t *testing.T) {
	m := layoutModel(t)
	m.width, m.height = 80, 24
	m.resize()
	view := m.View()
	if len(m.frameText) == 0 || view.Content == "" {
		t.Fatal("the frame was not cached")
	}

	// Pick a transcript row that actually carries text.
	row := -1
	for i, line := range m.frameText {
		if strings.TrimSpace(stripANSI(line)) != "" && m.layout.chat.Contains(2, i) {
			row = i
			break
		}
	}
	if row < 0 || row+1 >= len(m.frameText) {
		t.Fatal("no transcript row to drag over")
	}
	pristineRow := m.frameText[row]

	m.startSelection(selectionPoint{row: row, col: 0})
	m.dragSelection(selectionPoint{row: row + 1, col: 60})
	if got := strings.TrimSpace(m.selectedText()); got == "" {
		t.Fatal("the drag selected nothing")
	}

	painted := m.View()
	if !strings.Contains(painted.Content, selectionOn(t, m)) {
		t.Fatal("the rendered frame carries no highlight")
	}
	// Highlighting is styling only: the text under it must be untouched.
	if got, want := stripANSI(painted.Content), stripANSI(strings.Join(m.frameText, "\n")); got != want {
		t.Fatal("painting the selection changed the text on screen")
	}
	// And the cache is rebuilt pristine, so the next paint starts from the text
	// rather than from a highlighted copy of it.
	if got := m.frameText[row]; got != pristineRow {
		t.Fatalf("the cached row was painted in place:\n got %q\nwant %q", got, pristineRow)
	}
}

// selectionOn is the escape pair the theme's highlight uses. It is derived the
// same way the renderer derives it, so a test never hardcodes a palette.
func selectionOn(t *testing.T, m *Model) string {
	t.Helper()
	on, _ := selectionPaint(themeFor(m.cfg).SelectionBg)
	return on
}

// The frame can shrink under a live selection. A row that no longer exists has
// nothing to paint, and indexing it would take the whole screen down.
func TestSelectionSurvivesAFrameThatShrinks(t *testing.T) {
	m := &Model{width: 20, height: 8}
	m.frameText = []string{"alpha", "beta", "gamma", "", ""}
	m.startSelection(selectionPoint{row: 2, col: 0})
	m.dragSelection(selectionPoint{row: 4, col: 4})

	m.frameText = []string{"alpha", "beta"}
	if got := m.selectedText(); got != "" {
		t.Fatalf("selectedText after the frame shrank = %q, want empty", got)
	}
	if painted := m.paintFrame(ThemeByName("claude")); painted != "alpha\nbeta" {
		t.Fatalf("paintFrame after the frame shrank = %q", painted)
	}

	// A drag ending past the new last row clamps instead of dropping everything.
	m.frameText = []string{"alpha", "beta"}
	m.startSelection(selectionPoint{row: 0, col: 0})
	m.dragSelection(selectionPoint{row: 7, col: 19})
	if got, want := m.selectedText(), "alpha\nbeta"; got != want {
		t.Fatalf("clamped selection = %q, want %q", got, want)
	}
}
