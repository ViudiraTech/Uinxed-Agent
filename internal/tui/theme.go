package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	Name        string
	Primary     color.Color
	Secondary   color.Color
	Accent      color.Color
	Text        color.Color
	Muted       color.Color
	Border      color.Color
	CardBg      color.Color
	HeaderBg    color.Color
	StatusBg    color.Color
	SelectionBg color.Color
	SelectionFg color.Color
	PillBg      color.Color
	PillFg      color.Color
	Success     color.Color
	Warning     color.Color
	Error       color.Color
	DiffAdd     color.Color
	DiffDelete  color.Color
	Tool        color.Color
	User        color.Color
	Assistant   color.Color
}

func theme(name string) Theme {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "tokyonight":
		return Theme{
			Name:        "tokyonight",
			Primary:     lipgloss.Color("#7AA2F7"),
			Secondary:   lipgloss.Color("#BB9AF7"),
			Accent:      lipgloss.Color("#7DCFFF"),
			Text:        lipgloss.Color("#C0CAF5"),
			Muted:       lipgloss.Color("#565F89"),
			Border:      lipgloss.Color("#292E42"),
			CardBg:      lipgloss.Color("#1F2335"),
			HeaderBg:    lipgloss.Color("#16161E"),
			StatusBg:    lipgloss.Color("#16161E"),
			SelectionBg: lipgloss.Color("#3D59A1"),
			SelectionFg: lipgloss.Color("#FFFFFF"),
			PillBg:      lipgloss.Color("#1F2335"),
			PillFg:      lipgloss.Color("#C0CAF5"),
			Success:     lipgloss.Color("#9ECE6A"),
			Warning:     lipgloss.Color("#E0AF68"),
			Error:       lipgloss.Color("#F7768E"),
			DiffAdd:     lipgloss.Color("#9ECE6A"),
			DiffDelete:  lipgloss.Color("#F7768E"),
			Tool:        lipgloss.Color("#BB9AF7"),
			User:        lipgloss.Color("#7AA2F7"),
			Assistant:   lipgloss.Color("#C0CAF5"),
		}
	case "catppuccin":
		return Theme{
			Name:        "catppuccin",
			Primary:     lipgloss.Color("#CBA6F7"),
			Secondary:   lipgloss.Color("#89B4FA"),
			Accent:      lipgloss.Color("#FAB387"),
			Text:        lipgloss.Color("#CDD6F4"),
			Muted:       lipgloss.Color("#6C7086"),
			Border:      lipgloss.Color("#45475A"),
			CardBg:      lipgloss.Color("#181825"),
			HeaderBg:    lipgloss.Color("#11111B"),
			StatusBg:    lipgloss.Color("#11111B"),
			SelectionBg: lipgloss.Color("#CBA6F7"),
			SelectionFg: lipgloss.Color("#11111B"),
			PillBg:      lipgloss.Color("#313244"),
			PillFg:      lipgloss.Color("#CDD6F4"),
			Success:     lipgloss.Color("#A6E3A1"),
			Warning:     lipgloss.Color("#F9E2AF"),
			Error:       lipgloss.Color("#F38BA8"),
			DiffAdd:     lipgloss.Color("#A6E3A1"),
			DiffDelete:  lipgloss.Color("#F38BA8"),
			Tool:        lipgloss.Color("#F5C2E7"),
			User:        lipgloss.Color("#89DCEB"),
			Assistant:   lipgloss.Color("#CDD6F4"),
		}
	case "light":
		return Theme{
			Name:        "light",
			Primary:     lipgloss.Color("#4F46E5"),
			Secondary:   lipgloss.Color("#6B7280"),
			Accent:      lipgloss.Color("#DB2777"),
			Text:        lipgloss.Color("#1F2937"),
			Muted:       lipgloss.Color("#9CA3AF"),
			Border:      lipgloss.Color("#D1D5DB"),
			CardBg:      lipgloss.Color("#F3F4F6"),
			HeaderBg:    lipgloss.Color("#F9FAFB"),
			StatusBg:    lipgloss.Color("#F3F4F6"),
			SelectionBg: lipgloss.Color("#4F46E5"),
			SelectionFg: lipgloss.Color("#FFFFFF"),
			PillBg:      lipgloss.Color("#E5E7EB"),
			PillFg:      lipgloss.Color("#1F2937"),
			Success:     lipgloss.Color("#16A34A"),
			Warning:     lipgloss.Color("#D97706"),
			Error:       lipgloss.Color("#DC2626"),
			DiffAdd:     lipgloss.Color("#16A34A"),
			DiffDelete:  lipgloss.Color("#DC2626"),
			Tool:        lipgloss.Color("#7C3AED"),
			User:        lipgloss.Color("#2563EB"),
			Assistant:   lipgloss.Color("#111827"),
		}
	case "dark":
		return Theme{
			Name:        "dark",
			Primary:     lipgloss.Color("#38BDF8"),
			Secondary:   lipgloss.Color("#94A3B8"),
			Accent:      lipgloss.Color("#F472B6"),
			Text:        lipgloss.Color("#F1F5F9"),
			Muted:       lipgloss.Color("#64748B"),
			Border:      lipgloss.Color("#334155"),
			CardBg:      lipgloss.Color("#1E293B"),
			HeaderBg:    lipgloss.Color("#0F172A"),
			StatusBg:    lipgloss.Color("#0F172A"),
			SelectionBg: lipgloss.Color("#0284C7"),
			SelectionFg: lipgloss.Color("#FFFFFF"),
			PillBg:      lipgloss.Color("#1E293B"),
			PillFg:      lipgloss.Color("#F1F5F9"),
			Success:     lipgloss.Color("#4ADE80"),
			Warning:     lipgloss.Color("#FBBF24"),
			Error:       lipgloss.Color("#F87171"),
			DiffAdd:     lipgloss.Color("#4ADE80"),
			DiffDelete:  lipgloss.Color("#F87171"),
			Tool:        lipgloss.Color("#C084FC"),
			User:        lipgloss.Color("#38BDF8"),
			Assistant:   lipgloss.Color("#F8FAFC"),
		}
	default: // "uinxed" Cyberpunk Modern
		return Theme{
			Name:        "uinxed",
			Primary:     lipgloss.Color("#8B5CF6"),
			Secondary:   lipgloss.Color("#06B6D4"),
			Accent:      lipgloss.Color("#EC4899"),
			Text:        lipgloss.Color("#E2E8F0"),
			Muted:       lipgloss.Color("#64748B"),
			Border:      lipgloss.Color("#334155"),
			CardBg:      lipgloss.Color("#1E293B"),
			HeaderBg:    lipgloss.Color("#0F172A"),
			StatusBg:    lipgloss.Color("#0F172A"),
			SelectionBg: lipgloss.Color("#8B5CF6"),
			SelectionFg: lipgloss.Color("#FFFFFF"),
			PillBg:      lipgloss.Color("#1E293B"),
			PillFg:      lipgloss.Color("#E2E8F0"),
			Success:     lipgloss.Color("#10B981"),
			Warning:     lipgloss.Color("#F59E0B"),
			Error:       lipgloss.Color("#EF4444"),
			DiffAdd:     lipgloss.Color("#10B981"),
			DiffDelete:  lipgloss.Color("#EF4444"),
			Tool:        lipgloss.Color("#A855F7"),
			User:        lipgloss.Color("#38BDF8"),
			Assistant:   lipgloss.Color("#F1F5F9"),
		}
	}
}

// ThemeByName returns a semantic theme by its public configuration name.
func ThemeByName(name string) Theme { return theme(name) }
