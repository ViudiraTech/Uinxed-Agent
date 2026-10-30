package tui

import (
	"strings"

	md "github.com/ViudiraTech/Uinxed-Agent/internal/markdown"
)

// streamMarkdown renders a growing assistant message in full Markdown while it
// is still arriving.
//
// Re-rendering the whole message through glamour on every frame is not an
// option: a 7 KB reply costs ~5.6 ms, against an 8 ms render tick, so the
// stream would visibly stutter. The text is therefore split at the last
// paragraph boundary that is already closed (and never inside an open fence).
// That closed prefix is rendered once per boundary and memoized under its byte
// length; everything after the boundary is the "tail" and is re-rendered on its
// own each frame. The two are stitched back together, so the reader sees styled
// Markdown from the first closed paragraph instead of a wall of plain text that
// only snaps into shape when the turn ends.
type streamMarkdown struct {
	id    string
	width int
	style string
	// cut is the byte length of the closed prefix already rendered into lines.
	cut   int
	lines []string
	// plain handles a tail too long to re-render every frame. It wraps
	// incrementally, which is cheap enough for an unbroken paragraph that is
	// thousands of bytes long.
	plain streamWrap
}

// streamTailMax bounds the tail that is re-rendered through glamour each frame.
// Measured at roughly 0.9 ms for 1 KB, so this keeps a frame's tail work under
// a couple of milliseconds. A longer unbroken paragraph falls back to plain
// wrapping until a blank line closes it.
const streamTailMax = 2048

func (s *streamMarkdown) render(id, content string, width int, styleKey string, cache *md.Cache, style md.Style) []string {
	if width < 20 {
		width = 20
	}
	cut := stableCut(content)
	// A new message, a width/style change, or text that shrank invalidates the
	// rendered prefix. Shrinking happens when the stream is reset or a provider
	// resends a shorter snapshot; the boundary alone cannot be trusted then.
	if id != s.id || width != s.width || styleKey != s.style || cut < s.cut {
		*s = streamMarkdown{id: id, width: width, style: styleKey}
	}
	if cut > s.cut {
		rendered, err := cache.Render(id+"#stream", cut, width, styleKey, content[:cut], style)
		if err != nil {
			s.lines = wrapPlain(content[:cut], width)
		} else {
			s.lines = strings.Split(rendered, "\n")
		}
		s.cut = cut
	}

	out := make([]string, len(s.lines), len(s.lines)+4)
	copy(out, s.lines)

	tail := content[cut:]
	if strings.TrimSpace(tail) == "" {
		return out
	}
	// glamour gives a heading no trailing blank line, so a stitched tail has to
	// keep the gap itself — otherwise a heading would sit glued to the paragraph
	// that follows it.
	if len(out) > 0 && !headingTail(content[:cut]) {
		out = append(out, "")
	}
	out = append(out, s.renderTail(tail, width, style)...)
	return out
}

func (s *streamMarkdown) renderTail(tail string, width int, style md.Style) []string {
	if len(tail) > streamTailMax {
		// Too long to glamour every frame. Incremental wrapping keeps the frame
		// cheap; the next closed paragraph turns it into styled Markdown.
		return s.plain.wrap(tail, width, s.id+"#tail")
	}
	rendered, err := md.RenderOnce(width, tail, style)
	if err != nil {
		return wrapPlain(tail, width)
	}
	return strings.Split(rendered, "\n")
}

// stableCut returns the byte offset just past the last blank line that is not
// inside a fenced code block. Everything before it is closed: a later chunk of
// the message cannot change how it renders, which is what makes caching the
// prefix sound.
//
// A fence is tracked because a blank line inside ``` is code, not a paragraph
// break; cutting there would render an unterminated fence as a separate
// document.
func stableCut(content string) int {
	cut := 0
	inFence := false
	var fenceByte byte
	fenceLen := 0
	// Walk line by line without allocating a slice of lines.
	for start := 0; start <= len(content); {
		end := strings.IndexByte(content[start:], '\n')
		var line string
		next := len(content) + 1
		if end < 0 {
			line = content[start:]
		} else {
			line = content[start : start+end]
			next = start + end + 1
		}
		trimmed := strings.TrimLeft(line, " \t")
		if n, ch := fenceRun(trimmed); n >= 3 {
			switch {
			case !inFence:
				inFence, fenceByte, fenceLen = true, ch, n
			case ch == fenceByte && n >= fenceLen:
				inFence = false
			}
		} else if !inFence && end >= 0 && strings.TrimSpace(line) == "" && start > 0 {
			// The blank line closes every block above it, so the safe cut is
			// straight after it.
			cut = next
		}
		if next > len(content) {
			break
		}
		start = next
	}
	return cut
}

// fenceRun reports the length and byte of an opening fence run (``` or ~~~) at
// the start of a line, or zero when the line does not open one.
func fenceRun(line string) (int, byte) {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return 0, 0
	}
	ch := line[0]
	n := 0
	for n < len(line) && line[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return n, ch
}

// headingTail reports whether the last non-blank line of s is an ATX heading.
func headingTail(s string) bool {
	trimmed := strings.TrimRight(s, " \t\n")
	if i := strings.LastIndexByte(trimmed, '\n'); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	trimmed = strings.TrimLeft(trimmed, " \t")
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	return n > 0 && n <= 6 && n < len(trimmed) && trimmed[n] == ' '
}
