package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Theme struct {
	Name                                                                                                         string
	Primary, Secondary, Text, Muted, Border, Success, Warning, Error, DiffAdd, DiffDelete, Tool, User, Assistant color.Color
}

func theme(name string) Theme {
	switch name {
	case "light":
		return Theme{Name: "light", Primary: lipgloss.Color("#5B5BD6"), Secondary: lipgloss.Color("#6B7280"), Text: lipgloss.Color("#16181D"), Muted: lipgloss.Color("#6B7280"), Border: lipgloss.Color("#D2D5DA"), Success: lipgloss.Color("#15803D"), Warning: lipgloss.Color("#A16207"), Error: lipgloss.Color("#B91C1C"), DiffAdd: lipgloss.Color("#15803D"), DiffDelete: lipgloss.Color("#B91C1C"), Tool: lipgloss.Color("#7C3AED"), User: lipgloss.Color("#2563EB"), Assistant: lipgloss.Color("#111827")}
	case "dark":
		return Theme{Name: "dark", Primary: lipgloss.Color("#8B8CF8"), Secondary: lipgloss.Color("#9CA3AF"), Text: lipgloss.Color("#E5E7EB"), Muted: lipgloss.Color("#727985"), Border: lipgloss.Color("#343A46"), Success: lipgloss.Color("#56D364"), Warning: lipgloss.Color("#E3B341"), Error: lipgloss.Color("#F85149"), DiffAdd: lipgloss.Color("#3FB950"), DiffDelete: lipgloss.Color("#F85149"), Tool: lipgloss.Color("#BC8CFF"), User: lipgloss.Color("#58A6FF"), Assistant: lipgloss.Color("#E6EDF3")}
	default:
		return Theme{Name: "uinxed", Primary: lipgloss.Color("#A78BFA"), Secondary: lipgloss.Color("#67E8F9"), Text: lipgloss.Color("#E8EAF0"), Muted: lipgloss.Color("#747B8B"), Border: lipgloss.Color("#2A2F3A"), Success: lipgloss.Color("#4ADE80"), Warning: lipgloss.Color("#FACC15"), Error: lipgloss.Color("#FB7185"), DiffAdd: lipgloss.Color("#4ADE80"), DiffDelete: lipgloss.Color("#FB7185"), Tool: lipgloss.Color("#C084FC"), User: lipgloss.Color("#67E8F9"), Assistant: lipgloss.Color("#E8EAF0")}
	}
}

// ThemeByName returns a semantic theme by its public configuration name.
func ThemeByName(name string) Theme { return theme(name) }
