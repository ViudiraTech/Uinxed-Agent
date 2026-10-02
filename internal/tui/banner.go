package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	terminalutil "github.com/ViudiraTech/Uinxed-Agent/internal/terminal"
)

// bannerMaxInner caps the welcome card so it stays a card rather than stretching
// to the width of a wide terminal.
const bannerMaxInner = 68

// bannerMinInner is the narrowest the card is still readable at. Below this the
// banner is dropped entirely: a cramped box of truncated hints helps nobody.
const bannerMinInner = 20

// bannerLines is the card a brand-new session opens with.
//
// It is transcript content rather than a fixed header: it fills the empty screen
// and then scrolls away with the first message, instead of holding a row for the
// life of the session. Nothing here is generated — every hint names something
// this build actually does.
func bannerLines(t Theme, width int, model, cwd string) []renderLine {
	inner := min(width-4, bannerMaxInner)
	if inner < bannerMinInner {
		return nil
	}

	accent := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	body := lipgloss.NewStyle().Foreground(t.Text)
	muted := lipgloss.NewStyle().Foreground(t.Muted)
	key := lipgloss.NewStyle().Foreground(t.Secondary)

	// Each row is "│ " + inner columns + " │"; with the two corner characters
	// that makes the whole card inner+4 wide, which is what the caller fits.
	row := func(s string) renderLine {
		return renderLine{Text: accent.Render("│ ") + fitLine(s, inner) + accent.Render(" │")}
	}
	blank := row("")

	lines := []renderLine{
		{Text: accent.Render("╭" + strings.Repeat("─", inner+2) + "╮")},
		row(accent.Render("✻") + body.Render(" Welcome to Uinxed-Agent!")),
		blank,
		row(muted.Render("  ") + key.Render("/help") + muted.Render(" for commands and shortcuts")),
		row(muted.Render("  ") + key.Render("/connect") + muted.Render(" to add a provider, ") + key.Render("/model") + muted.Render(" to pick one")),
		row(muted.Render("  ") + key.Render("Ctrl+P") + muted.Render(" opens the command palette")),
		blank,
		row(muted.Render("  cwd: ") + body.Render(truncWidth(terminalutil.SanitizeText(cwd), inner-7))),
	}
	if model != "" {
		lines = append(lines, row(muted.Render("  model: ")+body.Render(truncWidth(terminalutil.SanitizeText(model), inner-9))))
	}
	lines = append(lines,
		blank,
		row(muted.Render("  Tips")),
		row(muted.Render("  1. Ask for a change and the agent reads, edits and runs the tests")),
		row(muted.Render("  2. ")+key.Render("@file")+muted.Render(" attaches a file, ")+key.Render("@explorer")+muted.Render(" delegates a search")),
		row(muted.Render("  3. ")+key.Render("/diff")+muted.Render(" reviews what changed before you commit")),
		renderLine{Text: accent.Render("╰" + strings.Repeat("─", inner+2) + "╯")},
	)
	return lines
}
