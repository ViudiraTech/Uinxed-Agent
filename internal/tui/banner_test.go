package tui

import (
	"fmt"
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
	// The card is only offered to a session that opened empty; the model records
	// that once, when the session is opened.
	m.freshSession = true
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

// The card is transcript content, not a header that gets deleted: the first
// message pushes the conversation beneath it instead of making a whole block
// vanish and re-anchoring the screen.
func TestBannerStaysAboveTheFirstMessage(t *testing.T) {
	m := emptyModel(t)
	m.width, m.height = 100, 30
	m.resize()
	before := strings.Split(stripANSI(m.renderBase(themeFor(m.cfg))), "\n")
	for _, want := range []string{"Welcome to Uinxed-Agent", "/connect", "Ctrl+P", "/home/someone/projects"} {
		if !strings.Contains(strings.Join(before, "\n"), want) {
			t.Fatalf("banner missing %q:\n%s", want, strings.Join(before, "\n"))
		}
	}

	m.session.Messages = []domain.Message{{ID: "u1", Role: domain.RoleUser, Content: "hello there"}}
	m.conv.SetSession(m.session, 100)
	after := strings.Split(stripANSI(m.renderBase(themeFor(m.cfg))), "\n")

	if !strings.Contains(strings.Join(after, "\n"), "Welcome to Uinxed-Agent") {
		t.Fatalf("the welcome card must survive the first message:\n%s", strings.Join(after, "\n"))
	}
	if before[0] != after[0] {
		t.Fatalf("the card jumped off its top row: %q -> %q", before[0], after[0])
	}
	bottom := -1
	for i, l := range after {
		if strings.Contains(l, "╰") {
			bottom = i
			break
		}
	}
	if bottom < 0 {
		t.Fatalf("no card bottom border:\n%s", strings.Join(after, "\n"))
	}
	if next := strings.Join(after[bottom+1:bottom+2], "\n"); !strings.Contains(next, "hello there") {
		t.Fatalf("the message should sit just under the card, got %q:\n%s", next, strings.Join(after, "\n"))
	}
}

// A full transcript scrolls the card off the top, and scrolling back to the
// origin brings it back. That only works while the card's height is part of the
// scroll estimate; otherwise the top of the scroll range stops short of it and
// the last rows can never be reached.
func TestBannerScrollsOffAndBack(t *testing.T) {
	m := emptyModel(t)
	for i := 0; i < 40; i++ {
		m.session.Messages = append(m.session.Messages, domain.Message{
			ID: fmt.Sprintf("u%d", i), Role: domain.RoleUser, Content: fmt.Sprintf("message %d with a little body text", i),
		})
	}
	m.width, m.height = 100, 30
	m.resize()
	m.conv.SetSession(m.session, 100)

	if got := stripANSI(m.renderBase(themeFor(m.cfg))); strings.Contains(got, "Welcome to Uinxed-Agent") {
		t.Fatalf("a long conversation should scroll the card away, not pin it:\n%s", got)
	}
	m.conv.ScrollUp(10000)
	top := strings.Split(stripANSI(m.renderBase(themeFor(m.cfg))), "\n")
	if !strings.Contains(strings.Join(top, "\n"), "Welcome to Uinxed-Agent") {
		t.Fatalf("scrolling to the top must reveal the card:\n%s", strings.Join(top, "\n"))
	}
	if !strings.Contains(top[0], "╭") {
		t.Fatalf("the card should start on the first row, got %q", top[0])
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
