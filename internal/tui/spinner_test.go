package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

var spinnerBases = map[string][]string{
	"mac":     spinnerMac,
	"ghostty": spinnerGhostty,
	"other":   spinnerOther,
}

// bounce must produce a sequence that grows to the peak and returns, with no
// frame repeated back to back. A repeated frame — including one spanning the
// wrap back to index 0 — reads as a stutter once per animation cycle.
func TestBounceRisesThenFallsWithoutStutter(t *testing.T) {
	for name, base := range spinnerBases {
		got := bounce(base)
		n := len(base)
		if len(got) != 2*n-2 {
			t.Fatalf("%s: bounce = %d frames, want %d", name, len(got), 2*n-2)
		}
		for i := 0; i < n; i++ {
			if got[i] != base[i] {
				t.Fatalf("%s: frame %d = %q, want the base frame %q", name, i, got[i], base[i])
			}
		}
		// The tail mirrors the body, minus the peak it already visited.
		for j := 1; j <= n-2; j++ {
			if want := base[n-1-j]; got[n-1+j] != want {
				t.Fatalf("%s: frame %d = %q, want the mirror %q", name, n-1+j, got[n-1+j], want)
			}
		}
		for i := range got {
			if next := got[(i+1)%len(got)]; next == got[i] {
				t.Fatalf("%s: %q repeats at %d, so the cycle stutters: %v", name, got[i], i, got)
			}
		}
	}
}

// Every frame lands in a single terminal cell. A wide glyph here would shift the
// whole working line on each tick.
func TestSpinnerFramesAreSingleCell(t *testing.T) {
	for name, base := range spinnerBases {
		for _, f := range bounce(base) {
			if got := lipgloss.Width(f); got != 1 {
				t.Fatalf("%s: frame %q measures %d cells, want 1", name, f, got)
			}
		}
	}
}

func TestSpinnerFramesForPicksTerminalThenPlatform(t *testing.T) {
	cases := []struct {
		name              string
		goos, termProgram string
		term              string
		want              []string
	}{
		{"macOS", "darwin", "", "", bounce(spinnerMac)},
		{"plain linux", "linux", "", "xterm-256color", bounce(spinnerOther)},
		{"ghostty via TERM_PROGRAM", "linux", "ghostty", "", bounce(spinnerGhostty)},
		{"ghostty via TERM", "linux", "", "xterm-ghostty", bounce(spinnerGhostty)},
		// A terminal that cannot render the full sparkle set outranks the
		// platform default, which is why the terminal check runs first.
		{"ghostty on macOS", "darwin", "ghostty", "", bounce(spinnerGhostty)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := spinnerFramesFor(c.goos, c.termProgram, c.term)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

// The ASCII set is the LANG=C contract: it must contain no rune that a
// non-UTF-8 terminal cannot render, so it keeps its rotating frames.
func TestASCIISpinnerStaysPlainASCII(t *testing.T) {
	for _, f := range asciiGlyphs.Spinner {
		if lipgloss.Width(f) != 1 {
			t.Fatalf("ascii spinner frame %q is not one cell", f)
		}
		for _, r := range f {
			if r > 127 {
				t.Fatalf("ascii spinner frame %q contains the non-ASCII rune %q", f, r)
			}
		}
	}
}

// The default glyph set is what the unicode theme renders, so it must carry the
// platform frames rather than whatever literal glyphs.go used to declare.
func TestDefaultGlyphsCarryPlatformSpinner(t *testing.T) {
	want := spinnerFrames()
	got := defaultGlyphs().Spinner
	if len(got) != len(want) {
		t.Fatalf("default spinner = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("default spinner = %v, want %v", got, want)
		}
	}
}
