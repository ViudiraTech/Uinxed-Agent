package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// streamWrap incrementally wraps a growing streamed message.
//
// Re-wrapping the whole accumulated text on every repaint is O(n) per frame and
// therefore O(n²) across a long answer; measured at ~450µs per frame for a 7KB
// reply, which is enough to make generation feel sluggish. Because hard wrapping
// is greedy from the left, only the final (still open) line can change when more
// text arrives, so each frame wraps just the newly appended tail.
type streamWrap struct {
	id       string
	width    int
	consumed int
	lines    []string
}

// wrap returns the wrapped lines for content, continuing from the previous call
// when possible. id identifies the message being streamed: consecutive model
// rounds are different messages, and without it a longer second round would be
// mistaken for a continuation of the first and kept on screen. The returned
// slice is owned by the streamWrap and must not be mutated by the caller.
func (s *streamWrap) wrap(content string, width int, id string) []string {
	if width < 1 {
		width = 1
	}
	// A new message, a width change, or text that shrank all mean the previous
	// run cannot be extended.
	if id != s.id || width != s.width || s.consumed > len(content) {
		s.id = id
		s.width = width
		s.consumed = 0
		s.lines = s.lines[:0]
	}
	if content == "" {
		// Matches wrapPlain(""), so the streaming and completed paths agree.
		if len(s.lines) == 0 {
			s.lines = append(s.lines, "")
		}
		return s.lines
	}
	if s.consumed == len(content) {
		return s.lines
	}
	s.extend(content[s.consumed:], width)
	s.consumed = len(content)
	return s.lines
}

func (s *streamWrap) extend(tail string, width int) {
	for i, part := range strings.Split(tail, "\n") {
		if i > 0 {
			// An explicit newline closes the previous line unconditionally.
			if part == "" {
				s.lines = append(s.lines, "")
				continue
			}
			s.lines = append(s.lines, strings.Split(ansi.Hardwrap(part, width, false), "\n")...)
			continue
		}
		// The first segment continues the line that was still open.
		if len(s.lines) == 0 {
			s.lines = append(s.lines, "")
		}
		last := len(s.lines) - 1
		s.lines[last] += part
		if displayWidth(s.lines[last]) > width {
			wrapped := strings.Split(ansi.Hardwrap(s.lines[last], width, false), "\n")
			s.lines = append(s.lines[:last], wrapped...)
		}
	}
}
