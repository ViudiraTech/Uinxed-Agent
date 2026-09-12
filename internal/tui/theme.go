package tui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette holds the semantic colors of a theme. It is embedded in Theme so
// callers keep writing t.Primary, and so a color-free build is a single
// assignment (see noColorPalette) rather than twenty field copies.
//
// Background-bearing tokens (card, header, pill) are deliberately absent: the
// layout is whitespace-driven and uses the terminal's own background.
type Palette struct {
	Primary     color.Color
	Secondary   color.Color
	Accent      color.Color
	Text        color.Color
	Muted       color.Color
	Border      color.Color
	SelectionBg color.Color
	SelectionFg color.Color
	Success     color.Color
	Warning     color.Color
	Error       color.Color
	DiffAdd     color.Color
	DiffDelete  color.Color
	Tool        color.Color
	User        color.Color
	Assistant   color.Color
	Reasoning   color.Color
	Gutter      color.Color
}

type Theme struct {
	Name   string
	Glyphs Glyphs
	Palette
}

// noColorPalette is used when NO_COLOR is set or the terminal reports TERM=dumb.
var noColorPalette = Palette{
	Primary: lipgloss.NoColor{}, Secondary: lipgloss.NoColor{}, Accent: lipgloss.NoColor{},
	Text: lipgloss.NoColor{}, Muted: lipgloss.NoColor{}, Border: lipgloss.NoColor{},
	SelectionBg: lipgloss.NoColor{}, SelectionFg: lipgloss.NoColor{},
	Success: lipgloss.NoColor{}, Warning: lipgloss.NoColor{}, Error: lipgloss.NoColor{},
	DiffAdd: lipgloss.NoColor{}, DiffDelete: lipgloss.NoColor{},
	Tool: lipgloss.NoColor{}, User: lipgloss.NoColor{}, Assistant: lipgloss.NoColor{},
	Reasoning: lipgloss.NoColor{}, Gutter: lipgloss.NoColor{},
}

func theme(name string) Theme {
	t := namedTheme(strings.ToLower(strings.TrimSpace(name)))
	t.Glyphs = defaultGlyphs()
	if noColorEnv() {
		t.Palette = noColorPalette
	}
	return t
}

// noColorEnv implements the NO_COLOR convention (presence of the variable,
// regardless of value, disables color) plus the traditional TERM=dumb signal.
// CLICOLOR_FORCE overrides both, which keeps CI screenshot tooling usable.
func noColorEnv() bool {
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return true
	}
	return os.Getenv("TERM") == "dumb"
}

func namedTheme(name string) Theme {
	switch name {
	case "tokyonight":
		return Theme{Name: "tokyonight", Palette: Palette{
			Primary: lipgloss.Color("#7AA2F7"), Secondary: lipgloss.Color("#BB9AF7"), Accent: lipgloss.Color("#7DCFFF"),
			Text: lipgloss.Color("#C0CAF5"), Muted: lipgloss.Color("#565F89"), Border: lipgloss.Color("#292E42"),
			SelectionBg: lipgloss.Color("#3D59A1"), SelectionFg: lipgloss.Color("#FFFFFF"),
			Success: lipgloss.Color("#9ECE6A"), Warning: lipgloss.Color("#E0AF68"), Error: lipgloss.Color("#F7768E"),
			DiffAdd: lipgloss.Color("#9ECE6A"), DiffDelete: lipgloss.Color("#F7768E"),
			Tool: lipgloss.Color("#BB9AF7"), User: lipgloss.Color("#7AA2F7"), Assistant: lipgloss.Color("#C0CAF5"),
			Reasoning: lipgloss.Color("#565F89"), Gutter: lipgloss.Color("#3B4261"),
		}}
	case "catppuccin":
		return Theme{Name: "catppuccin", Palette: Palette{
			Primary: lipgloss.Color("#CBA6F7"), Secondary: lipgloss.Color("#89B4FA"), Accent: lipgloss.Color("#FAB387"),
			Text: lipgloss.Color("#CDD6F4"), Muted: lipgloss.Color("#6C7086"), Border: lipgloss.Color("#45475A"),
			SelectionBg: lipgloss.Color("#45475A"), SelectionFg: lipgloss.Color("#CDD6F4"),
			Success: lipgloss.Color("#A6E3A1"), Warning: lipgloss.Color("#F9E2AF"), Error: lipgloss.Color("#F38BA8"),
			DiffAdd: lipgloss.Color("#A6E3A1"), DiffDelete: lipgloss.Color("#F38BA8"),
			Tool: lipgloss.Color("#F5C2E7"), User: lipgloss.Color("#89DCEB"), Assistant: lipgloss.Color("#CDD6F4"),
			Reasoning: lipgloss.Color("#6C7086"), Gutter: lipgloss.Color("#45475A"),
		}}
	case "gruvbox":
		return Theme{Name: "gruvbox", Palette: Palette{
			Primary: lipgloss.Color("#83A598"), Secondary: lipgloss.Color("#D3869B"), Accent: lipgloss.Color("#FABD2F"),
			Text: lipgloss.Color("#EBDBB2"), Muted: lipgloss.Color("#928374"), Border: lipgloss.Color("#3C3836"),
			SelectionBg: lipgloss.Color("#504945"), SelectionFg: lipgloss.Color("#EBDBB2"),
			Success: lipgloss.Color("#B8BB26"), Warning: lipgloss.Color("#FABD2F"), Error: lipgloss.Color("#FB4934"),
			DiffAdd: lipgloss.Color("#B8BB26"), DiffDelete: lipgloss.Color("#FB4934"),
			Tool: lipgloss.Color("#D3869B"), User: lipgloss.Color("#8EC07C"), Assistant: lipgloss.Color("#EBDBB2"),
			Reasoning: lipgloss.Color("#928374"), Gutter: lipgloss.Color("#504945"),
		}}
	case "nord":
		return Theme{Name: "nord", Palette: Palette{
			Primary: lipgloss.Color("#88C0D0"), Secondary: lipgloss.Color("#B48EAD"), Accent: lipgloss.Color("#EBCB8B"),
			Text: lipgloss.Color("#D8DEE9"), Muted: lipgloss.Color("#4C566A"), Border: lipgloss.Color("#434C5E"),
			SelectionBg: lipgloss.Color("#434C5E"), SelectionFg: lipgloss.Color("#ECEFF4"),
			Success: lipgloss.Color("#A3BE8C"), Warning: lipgloss.Color("#EBCB8B"), Error: lipgloss.Color("#BF616A"),
			DiffAdd: lipgloss.Color("#A3BE8C"), DiffDelete: lipgloss.Color("#BF616A"),
			Tool: lipgloss.Color("#B48EAD"), User: lipgloss.Color("#81A1C1"), Assistant: lipgloss.Color("#E5E9F0"),
			Reasoning: lipgloss.Color("#616E88"), Gutter: lipgloss.Color("#4C566A"),
		}}
	case "dracula":
		return Theme{Name: "dracula", Palette: Palette{
			Primary: lipgloss.Color("#BD93F9"), Secondary: lipgloss.Color("#8BE9FD"), Accent: lipgloss.Color("#FF79C6"),
			Text: lipgloss.Color("#F8F8F2"), Muted: lipgloss.Color("#6272A4"), Border: lipgloss.Color("#44475A"),
			SelectionBg: lipgloss.Color("#44475A"), SelectionFg: lipgloss.Color("#F8F8F2"),
			Success: lipgloss.Color("#50FA7B"), Warning: lipgloss.Color("#F1FA8C"), Error: lipgloss.Color("#FF5555"),
			DiffAdd: lipgloss.Color("#50FA7B"), DiffDelete: lipgloss.Color("#FF5555"),
			Tool: lipgloss.Color("#FF79C6"), User: lipgloss.Color("#8BE9FD"), Assistant: lipgloss.Color("#F8F8F2"),
			Reasoning: lipgloss.Color("#6272A4"), Gutter: lipgloss.Color("#44475A"),
		}}
	case "light":
		return Theme{Name: "light", Palette: Palette{
			Primary: lipgloss.Color("#4F46E5"), Secondary: lipgloss.Color("#6B7280"), Accent: lipgloss.Color("#DB2777"),
			Text: lipgloss.Color("#1F2937"), Muted: lipgloss.Color("#9CA3AF"), Border: lipgloss.Color("#D1D5DB"),
			SelectionBg: lipgloss.Color("#4F46E5"), SelectionFg: lipgloss.Color("#FFFFFF"),
			Success: lipgloss.Color("#16A34A"), Warning: lipgloss.Color("#D97706"), Error: lipgloss.Color("#DC2626"),
			DiffAdd: lipgloss.Color("#16A34A"), DiffDelete: lipgloss.Color("#DC2626"),
			Tool: lipgloss.Color("#7C3AED"), User: lipgloss.Color("#2563EB"), Assistant: lipgloss.Color("#111827"),
			Reasoning: lipgloss.Color("#9CA3AF"), Gutter: lipgloss.Color("#D1D5DB"),
		}}
	case "dark":
		return Theme{Name: "dark", Palette: Palette{
			Primary: lipgloss.Color("#38BDF8"), Secondary: lipgloss.Color("#94A3B8"), Accent: lipgloss.Color("#F472B6"),
			Text: lipgloss.Color("#F1F5F9"), Muted: lipgloss.Color("#64748B"), Border: lipgloss.Color("#334155"),
			SelectionBg: lipgloss.Color("#0284C7"), SelectionFg: lipgloss.Color("#FFFFFF"),
			Success: lipgloss.Color("#4ADE80"), Warning: lipgloss.Color("#FBBF24"), Error: lipgloss.Color("#F87171"),
			DiffAdd: lipgloss.Color("#4ADE80"), DiffDelete: lipgloss.Color("#F87171"),
			Tool: lipgloss.Color("#C084FC"), User: lipgloss.Color("#38BDF8"), Assistant: lipgloss.Color("#F8FAFC"),
			Reasoning: lipgloss.Color("#64748B"), Gutter: lipgloss.Color("#475569"),
		}}
	default: // "uinxed"
		return Theme{Name: "uinxed", Palette: Palette{
			Primary: lipgloss.Color("#8B5CF6"), Secondary: lipgloss.Color("#06B6D4"), Accent: lipgloss.Color("#EC4899"),
			Text: lipgloss.Color("#E2E8F0"), Muted: lipgloss.Color("#64748B"), Border: lipgloss.Color("#334155"),
			SelectionBg: lipgloss.Color("#8B5CF6"), SelectionFg: lipgloss.Color("#FFFFFF"),
			Success: lipgloss.Color("#10B981"), Warning: lipgloss.Color("#F59E0B"), Error: lipgloss.Color("#EF4444"),
			DiffAdd: lipgloss.Color("#10B981"), DiffDelete: lipgloss.Color("#EF4444"),
			Tool: lipgloss.Color("#A855F7"), User: lipgloss.Color("#38BDF8"), Assistant: lipgloss.Color("#F1F5F9"),
			Reasoning: lipgloss.Color("#64748B"), Gutter: lipgloss.Color("#475569"),
		}}
	}
}

// ThemeByName returns a semantic theme by its public configuration name.
func ThemeByName(name string) Theme { return theme(name) }
