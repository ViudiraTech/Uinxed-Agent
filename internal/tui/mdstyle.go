package tui

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
	md "github.com/ViudiraTech/Uinxed-Agent/internal/markdown"
)

// chromaByTheme maps each UI theme onto a chroma style for fenced code blocks.
// glamour's default is a light palette, which looks broken under every dark
// theme this app ships.
var chromaByTheme = map[string]string{
	"uinxed":     "onedark",
	"tokyonight": "onedark",
	"catppuccin": "catppuccin-mocha",
	"gruvbox":    "gruvbox",
	"nord":       "nord",
	"dracula":    "dracula",
	"dark":       "github-dark",
	"light":      "github",
}

// mdStyle adapts the active theme to the Markdown renderer's palette. Colors
// are converted back to hex because glamour's style config takes strings; a
// NoColor token (NO_COLOR / TERM=dumb) maps to "" which means "no styling".
// Body text is left unset so it renders in the terminal's own foreground.
func mdStyle(t Theme) md.Style {
	return md.Style{
		Muted:     hexColor(t.Muted),
		Primary:   hexColor(t.Primary),
		Secondary: hexColor(t.Secondary),
		Accent:    hexColor(t.Accent),
		Tool:      hexColor(t.Tool),
		Success:   hexColor(t.Success),
		Error:     hexColor(t.Error),
		Border:    hexColor(t.Border),
		CodeTheme: chromaByTheme[t.Name],
		Bullet:    t.Glyphs.ListBullet,
	}
}

// mdStyleKey identifies a rendered style for the Markdown cache. Glyphs matter
// to Markdown because the list bullet comes from the glyph set; the no-color
// state matters because it changes whether any escapes are emitted at all.
func mdStyleKey(t Theme) string {
	key := t.Name + "," + t.Glyphs.ListBullet
	if _, ok := t.Primary.(lipgloss.NoColor); ok {
		key += ",nocolor"
	}
	return key
}

func hexColor(c color.Color) string {
	if c == nil {
		return ""
	}
	if _, ok := c.(lipgloss.NoColor); ok {
		return ""
	}
	r, g, b, a := c.RGBA()
	if a == 0 {
		return ""
	}
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
