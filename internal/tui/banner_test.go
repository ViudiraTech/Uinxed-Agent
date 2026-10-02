package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/ViudiraTech/Uinxed-Agent/internal/domain"
)

func emptyModel(t *testing.T) *Model {
	t.Helper()
	m := layoutModel(t)
	m.session.Messages = nil
	m.session.Todos = nil
	m.conv.SetSession(m.session, 100)
	return m
}

// The card is the only bordered thing the transcript ever draws, so its width
// has to be exactly right at every terminal size — too wide and it wraps,
// breaking the whole screen's column contract.
func TestBannerFitsEveryTerminalWidth(t *testing.T) {
	for _, w := range []int{20, 24, 40, 60, 80, 100, 140} {
		m := emptyModel(t)
		m.width, m.height = w, 30
		m.resize()
		out := m.renderBase(themeFor(m.cfg))
		for i, l := range strings.Split(out, "\n") {
			if got := lipgloss.Width(l); got != w {
				t.Fatalf("width %d line %d: rendered %d columns\n%q", w, i, got, stripANSI(l))
			}
		}
	}
}

// A too-narrow terminal drops the card rather than drawing a truncated box.
func TestBannerIsDroppedWhenTooNarrow(t *testing.T) {
	if got := bannerLines(ThemeByName("claude"), bannerMinInner+3, "m", "/tmp"); got != nil {
		t.Fatalf("a %d-column card should not render, got %d lines", bannerMinInner+3, len(got))
	}
}

// The card carries real content, sanitized, and leaves as soon as the session
// has something to say.
func TestBannerShowsThenScrollsAway(t *testing.T) {
	m := emptyModel(t)
	m.width, m.height = 100, 30
	m.resize()
	out := stripANSI(m.renderBase(themeFor(m.cfg)))
	for _, want := range []string{"Welcome to Uinxed-Agent", "/connect", "Ctrl+P", "/home/someone/projects"} {
		if !strings.Contains(out, want) {
			t.Fatalf("banner missing %q:\n%s", want, out)
		}
	}

	m.session.Messages = []domain.Message{{ID: "u1", Role: domain.RoleUser, Content: "hello there"}}
	m.conv.SetSession(m.session, 100)
	if got := stripANSI(m.renderBase(themeFor(m.cfg))); strings.Contains(got, "Welcome to Uinxed-Agent") {
		t.Fatalf("the welcome card must leave once the session has content:\n%s", got)
	}
}

// Untrusted session text reaches the card, so it has to be sanitized like every
// other display path.
func TestBannerSanitizesModelAndCWD(t *testing.T) {
	lines := bannerLines(ThemeByName("claude"), 90, "evil\x1b[31mmodel", "/tmp/\x1b]52;c;payload\x07dir")
	for _, l := range lines {
		if strings.Contains(l.Text, "\x1b]52") || strings.Contains(l.Text, "[31m") {
			t.Fatalf("escape sequence survived into the banner: %q", l.Text)
		}
	}
}
