package tui

import (
	"os"
	"runtime"
	"strings"
)

// Claude Code's thinking spinner is a sparkle that grows and shrinks rather than
// a cycle that rotates: the frames run forward, then back over the same ground.
// Glyphs.spinner indexes with frame % len(Spinner), so storing the sequence as a
// palindrome reproduces that bounce without touching the accessor.
//
// The base sets differ by terminal, not only by platform. Ghostty and most
// non-macOS terminals cannot be relied on to render the full sparkle range, so
// one slot degrades to a plain asterisk there.
var (
	spinnerMac     = []string{"·", "✢", "✳", "✶", "✻", "✽"}
	spinnerGhostty = []string{"·", "✢", "✳", "✶", "✻", "*"}
	spinnerOther   = []string{"·", "✢", "*", "✶", "✻", "✽"}
)

// spinnerFrames selects the frame sequence for the running terminal.
func spinnerFrames() []string {
	return spinnerFramesFor(runtime.GOOS, os.Getenv("TERM_PROGRAM"), os.Getenv("TERM"))
}

// spinnerFramesFor is split out from spinnerFrames so the platform choice is
// testable without mutating the process environment.
//
// The terminal check precedes the platform check deliberately: Ghostty is a
// narrower condition than "is macOS", and its limitation exists precisely
// because a glyph the macOS set otherwise assumes is unavailable there.
func spinnerFramesFor(goos, termProgram, term string) []string {
	switch {
	case isGhostty(termProgram, term):
		return bounce(spinnerGhostty)
	case goos == "darwin":
		return bounce(spinnerMac)
	default:
		return bounce(spinnerOther)
	}
}

func isGhostty(termProgram, term string) bool {
	return strings.Contains(strings.ToLower(termProgram), "ghostty") ||
		strings.Contains(strings.ToLower(term), "ghostty")
}

// bounce expands a base frame set into the forward-then-back sequence. The
// repeated middle frame and the repeated endpoint are both dropped, so indexing
// the result with frame % len loops seamlessly: the last frame hands back to the
// first without a visible stutter.
func bounce(base []string) []string {
	if len(base) < 2 {
		return append([]string(nil), base...)
	}
	out := make([]string, 0, 2*len(base)-2)
	out = append(out, base...)
	for i := len(base) - 2; i > 0; i-- {
		out = append(out, base[i])
	}
	return out
}
