package markdown

import (
	"charm.land/glamour/v2/ansi"
)

// Style carries the palette a caller wants Markdown rendered in, as hex
// strings. Empty fields mean "no color", which is how NO_COLOR terminals get
// plain text without a separate code path.
//
// Body text is deliberately absent: it renders in the terminal's own
// foreground, the way mainstream agent CLIs do. Only accents — headings, code,
// links, list markers — carry theme colors. That also keeps those accents
// visible, since a Text color would override them for the glyphs themselves.
type Style struct {
	Muted     string
	Primary   string
	Secondary string
	Accent    string
	Tool      string
	Success   string
	Error     string
	Border    string
	// CodeTheme names a chroma style for fenced code blocks. Empty falls back
	// to glamour's default, which is a light palette that clashes with dark
	// terminal themes.
	CodeTheme string
	// Bullet is the marker for unordered list items. Empty uses a bullet dot;
	// ASCII-only terminals pass "-".
	Bullet string
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func boolPtr(v bool) *bool { return &v }

// ansiConfig translates a Style into glamour's renderer configuration. Using
// explicit styles rather than glamour's built-in dark/light presets is what
// keeps rendered Markdown consistent with the active terminal theme — the
// presets paint their own palette and a jarring inline-code background.
func ansiConfig(s Style) ansi.StyleConfig {
	zero := uint(0)
	text := ansi.StylePrimitive{}
	muted := ansi.StylePrimitive{Color: strPtr(s.Muted)}
	bold := ansi.StylePrimitive{Bold: boolPtr(true)}
	bullet := s.Bullet
	if bullet == "" {
		bullet = "•"
	}

	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			// No document margin: the transcript supplies its own gutter, and
			// a built-in 2-space margin would double it up.
			Margin: &zero,
		},
		Paragraph: ansi.StyleBlock{},
		Text:      text,

		Heading: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Primary)}},
		H1:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Primary)}},
		H2:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Primary)}},
		H3:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Secondary)}},
		H4:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Secondary)}},
		H5:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Secondary)}},
		H6:      ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true), Color: strPtr(s.Secondary)}},

		Strong:        bold,
		Emph:          ansi.StylePrimitive{Italic: boolPtr(true)},
		Strikethrough: ansi.StylePrimitive{CrossedOut: boolPtr(true), Color: strPtr(s.Muted)},

		// Inline code is colored only. A background fill here is the single
		// loudest element glamour's presets introduce, and it fights every theme.
		// No padding either: it would push list items into a double space.
		Code: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr(s.Accent)}},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{StylePrimitive: text, Margin: &zero},
			Theme:      s.CodeTheme,
		},

		Link:     ansi.StylePrimitive{Color: strPtr(s.Primary), Underline: boolPtr(true)},
		LinkText: ansi.StylePrimitive{Color: strPtr(s.Primary)},

		Item:        ansi.StylePrimitive{Color: strPtr(s.Secondary), BlockPrefix: bullet + " "},
		Enumeration: ansi.StylePrimitive{Color: strPtr(s.Secondary), BlockPrefix: ". "},
		List:        ansi.StyleList{LevelIndent: 2},

		HorizontalRule: ansi.StylePrimitive{Color: strPtr(s.Border)},
		BlockQuote:     ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: strPtr(s.Muted), Italic: boolPtr(true)}},

		Task: ansi.StyleTask{
			StylePrimitive: muted,
			Ticked:         "[x] ",
			Unticked:       "[ ] ",
		},
	}
}
