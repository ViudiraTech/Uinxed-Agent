package tui

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

// textSelection is the terminal-style drag selection over the rendered frame.
//
// It is deliberately screen-anchored rather than message-anchored. The drag
// happens across what is on screen, and the transcript is virtualized: there is
// no stable, cheap mapping from a cell back to the message that produced it.
// Everything that moves the frame underneath the anchor — a scroll, a resize, a
// turn settling — drops the selection, because a highlight that no longer covers
// the text it was drawn over is worse than no highlight at all.
type textSelection struct {
	anchor selectionPoint
	cursor selectionPoint
	// active is true between press and release. It is also the only window in
	// which pointer motion is a drag rather than a hover.
	active bool
	// moved separates a drag from a click. A press and release on the same cell
	// is a click and has to reach the region underneath instead of copying.
	moved bool
}

// selectionPoint is a screen cell: the row indexes the rendered frame, the
// column counts display columns from the left edge.
type selectionPoint struct{ row, col int }

func (m *Model) startSelection(p selectionPoint) {
	p = m.clampPoint(p)
	m.sel = textSelection{anchor: p, cursor: p, active: true}
	// A hover highlight left over from where the pointer was before the press
	// would sit next to the selection looking like part of it.
	m.hover = ""
}

// dragSelection grows the selection to the cell under the pointer. The anchor
// stays put, so dragging back past it shrinks the selection instead of starting
// a new one.
func (m *Model) dragSelection(p selectionPoint) {
	if !m.sel.active {
		return
	}
	p = m.clampPoint(p)
	if p != m.sel.anchor {
		m.sel.moved = true
	}
	m.sel.cursor = p
}

// endSelection finishes a drag and copies what it covers. It returns nil for a
// click, which is what lets the region's own action run.
func (m *Model) endSelection() tea.Cmd {
	if !m.sel.active {
		return nil
	}
	m.sel.active = false
	if !m.sel.moved {
		m.clearSelection()
		return nil
	}
	text := m.selectedText()
	if strings.TrimSpace(text) == "" {
		m.clearSelection()
		return nil
	}
	m.showToast("✓ copied " + describeSelection(text))
	return tea.SetClipboard(text)
}

func (m *Model) clearSelection() { m.sel = textSelection{} }

// canSelect reports whether a press should open a drag selection. Only the
// transcript qualifies: the composer and the status row are controls, and a tool
// card, a thinking header or a chip under the pointer has an action of its own.
func (m *Model) canSelect(mouse tea.Mouse, r Region, hasRegion bool) bool {
	if mouse.Button != tea.MouseLeft || !m.cfg.Mouse || m.overlay != overlayNone {
		return false
	}
	return !hasRegion || r.Kind == ActionChat
}

// hasSelection reports whether the highlight should be drawn. A selection stays
// on screen after the copy so the reader can see what left the terminal.
func (m *Model) hasSelection() bool {
	return m.sel.moved && m.sel.cursor != m.sel.anchor
}

// selectionRange normalizes the drag into reading order: the top-left cell
// first, the bottom-right cell second.
func (m *Model) selectionRange() (selectionPoint, selectionPoint, bool) {
	if !m.hasSelection() {
		return selectionPoint{}, selectionPoint{}, false
	}
	a, b := m.sel.anchor, m.sel.cursor
	if b.row < a.row || (b.row == a.row && b.col < a.col) {
		a, b = b, a
	}
	if len(m.frameText) == 0 {
		return selectionPoint{}, selectionPoint{}, false
	}
	if a.row < 0 {
		a = selectionPoint{}
	}
	// The frame can shrink under a selection — the working line disappears, a
	// todo block empties — and a row that no longer exists has nothing to cover.
	if a.row >= len(m.frameText) {
		return selectionPoint{}, selectionPoint{}, false
	}
	if b.row >= len(m.frameText) {
		b.row = len(m.frameText) - 1
		b.col = max(0, displayWidth(m.frameText[b.row])-1)
	}
	return a, b, true
}

// clampPoint keeps a pointer position inside the frame. A drag that leaves the
// transcript — down into the composer, or past the last column — keeps growing
// against the edge instead of stopping dead.
func (m *Model) clampPoint(p selectionPoint) selectionPoint {
	if p.row < 0 {
		p.row = 0
	}
	if m.height > 0 && p.row > m.height-1 {
		p.row = m.height - 1
	}
	if p.col < 0 {
		p.col = 0
	}
	if m.width > 0 && p.col > m.width-1 {
		p.col = m.width - 1
	}
	return p
}

// selectedText extracts the plain text under the selection. Each line is
// right-trimmed because the frame is padded to the terminal width, and blank
// lines at either edge are dropped because a drag usually ends a row past the
// content it meant to copy.
func (m *Model) selectedText() string {
	a, b, ok := m.selectionRange()
	if !ok {
		return ""
	}
	out := make([]string, 0, b.row-a.row+1)
	for row := a.row; row <= b.row; row++ {
		line := m.frameText[row]
		from, to := 0, displayWidth(line)
		if row == a.row {
			from = a.col
		}
		if row == b.row {
			to = b.col + 1
		}
		out = append(out, strings.TrimRight(plainColumns(line, from, to), " "))
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	if len(out) == 0 {
		return ""
	}
	return terminalutil.SanitizeText(strings.Join(out, "\n"))
}

// paintFrame redraws the cached frame with the selection highlighted. It takes
// no argument and rebuilds from frameText so a repaint can never stack one
// highlight on top of another: the animation loop redraws this screen every
// 120 ms.
func (m *Model) paintFrame(t Theme) string {
	a, b, ok := m.selectionRange()
	if !ok {
		return strings.Join(m.frameText, "\n")
	}
	on, off := selectionPaint(t.SelectionBg)
	lines := make([]string, len(m.frameText))
	copy(lines, m.frameText)
	for row := a.row; row <= b.row && row < len(lines); row++ {
		from, to := 0, displayWidth(lines[row])
		if row == a.row {
			from = a.col
		}
		if row == b.row {
			to = b.col + 1
		}
		lines[row] = paintColumns(lines[row], from, to, on, off)
	}
	return strings.Join(lines, "\n")
}

// escapeAt returns the escape sequence starting s, if it starts with one, and
// its byte length. Styled content only ever carries CSI (SGR) and the occasional
// OSC, so those are the two forms recognized; anything else is treated as text.
func escapeAt(s string) (string, int) {
	if len(s) < 2 || s[0] != 0x1b {
		return "", 0
	}
	switch s[1] {
	case '[':
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7e {
				return s[:i+1], i + 1
			}
		}
		return "", 0
	case ']':
		for i := 2; i < len(s); i++ {
			if s[i] == 0x07 { // BEL
				return s[:i+1], i + 1
			}
			if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ST
				return s[:i+2], i + 2
			}
		}
		return "", 0
	}
	return "", 0
}

// runeColumns is the display width of one rune. Wide characters occupy two
// columns and combining marks none, which is what makes the column arithmetic
// line up with the cells the terminal paints.
func runeColumns(r rune) int { return ansi.StringWidth(string(r)) }

// sgrClearsBackground reports whether an SGR sequence resets the background —
// a full reset, or the explicit "default background" parameter. Inside a
// selection one of these would punch a hole in the highlight, so the highlight
// has to be re-applied right after it.
func sgrClearsBackground(seq string) bool {
	if !strings.HasPrefix(seq, "\x1b[") || !strings.HasSuffix(seq, "m") {
		return false
	}
	body := seq[2 : len(seq)-1]
	if body == "" {
		return true
	}
	for _, param := range strings.Split(body, ";") {
		switch param {
		case "", "0", "49":
			return true
		}
	}
	return false
}

// paintColumns tints the display columns [from, to) of one styled line.
//
// The line already carries SGR sequences, so the highlight is injected at the
// first selected cell and every background reset inside the span re-applies it;
// a reset that landed inside the selection would otherwise end the highlight
// early. A glyph that straddles either edge is pulled wholly into the selection:
// half of a wide CJK character cannot be painted.
func paintColumns(line string, from, to int, on, off string) string {
	if from >= to {
		return line
	}
	var b strings.Builder
	b.Grow(len(line) + len(on) + len(off))
	col := 0
	open := false
	for i := 0; i < len(line); {
		if seq, n := escapeAt(line[i:]); n > 0 {
			b.WriteString(seq)
			if open && sgrClearsBackground(seq) {
				b.WriteString(on)
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := runeColumns(r)
		if !open && col+w > from && col < to {
			b.WriteString(on)
			open = true
		}
		b.WriteString(line[i : i+size])
		col += w
		if open && col >= to {
			b.WriteString(off)
			open = false
		}
		i += size
	}
	if open {
		b.WriteString(off)
	}
	return b.String()
}

// plainColumns returns the text of the display columns [from, to), with the
// styling dropped. It is the extraction side of paintColumns and shares its
// column arithmetic, so copied text is exactly what the highlight covered.
func plainColumns(line string, from, to int) string {
	if from >= to {
		return ""
	}
	var b strings.Builder
	col := 0
	for i := 0; i < len(line); {
		if _, n := escapeAt(line[i:]); n > 0 {
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := runeColumns(r)
		if col+w > from && col < to {
			b.WriteString(line[i : i+size])
		}
		col += w
		if col >= to {
			break
		}
		i += size
	}
	return b.String()
}

// selectionPaint derives the escape sequences the theme's selection background
// needs. The sequence is taken from lipgloss rather than assembled by hand so a
// 256-color theme and a truecolor one both emit whatever the rest of the UI
// emits. With no color at all the highlight falls back to reverse video, which
// is the one selection affordance that survives a monochrome terminal.
func selectionPaint(c color.Color) (on, off string) {
	const reverseOn, reverseOff = "\x1b[7m", "\x1b[27m"
	if c == nil {
		return reverseOn, reverseOff
	}
	rendered := lipgloss.NewStyle().Background(c).Render("x")
	if i := strings.Index(rendered, "x"); i > 0 {
		return rendered[:i], "\x1b[49m"
	}
	return reverseOn, reverseOff
}

// describeSelection is what the copy toast reports: enough to tell a line from a
// snippet, without dumping the text back over the screen it came from.
func describeSelection(text string) string {
	if lines := strings.Count(text, "\n") + 1; lines > 1 {
		return fmt.Sprintf("%d lines", lines)
	}
	return fmt.Sprintf("%d chars", utf8.RuneCountInString(text))
}
