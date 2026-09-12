package tui

import (
	"os"
	"runtime"
	"strings"
	"sync"
)

// Glyphs holds every single-character marker the UI draws. Routing all of them
// through one value keeps the interface free of hard-coded Nerd Font
// private-use codepoints, which render as tofu on an unpatched terminal.
//
// Fields may hold more than one rune; callers must measure with lipgloss.Width
// rather than assuming a fixed column count.
type Glyphs struct {
	Assistant  string // assistant turn and tool-call prefix
	User       string // user turn and composer prefix
	Result     string // tool result subtree prefix
	Thinking   string // reasoning marker
	Success    string
	Failure    string
	Pending    string
	Bullet     string // picker / list selection marker
	ListBullet string // Markdown bullet list marker
	Cursor     string // text cursor drawn at the end of an active query
	TodoDone   string
	TodoOpen   string
	BarFull    string
	BarEmpty   string
	Sep        string // status bar segment separator
	Rule       string // horizontal rule drawn above and below the composer
	VBar       string // vertical divider between panes
	Spinner    []string
}

var unicodeGlyphs = Glyphs{
	Assistant:  "⏺",
	User:       "❯",
	Result:     "⎿",
	Thinking:   "✻",
	Success:    "✓",
	Failure:    "✗",
	Pending:    "○",
	Bullet:     "▸",
	ListBullet: "•",
	Cursor:     "▏",
	TodoDone:   "✓",
	TodoOpen:   "◻",
	BarFull:    "■",
	BarEmpty:   "·",
	Sep:        "·",
	Rule:       "─",
	VBar:       "│",
	Spinner:    []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

var asciiGlyphs = Glyphs{
	Assistant:  "*",
	User:       ">",
	Result:     "|",
	Thinking:   "~",
	Success:    "+",
	Failure:    "x",
	Pending:    "-",
	Bullet:     ">",
	ListBullet: "-",
	Cursor:     "|",
	TodoDone:   "[x]",
	TodoOpen:   "[ ]",
	BarFull:    "#",
	BarEmpty:   "-",
	Sep:        "|",
	Rule:       "-",
	VBar:       "|",
	Spinner:    []string{"|", "/", "-", "\\"},
}

var (
	glyphAutoOnce sync.Once
	glyphAuto     Glyphs
)

// glyphSet resolves a configured glyph mode. "auto" (and the empty string)
// picks ASCII when the locale is explicitly non-UTF-8, so a LANG=C terminal
// degrades cleanly instead of printing replacement characters.
func glyphSet(mode string) Glyphs {
	switch mode {
	case "ascii":
		return asciiGlyphs
	case "unicode":
		return unicodeGlyphs
	default:
		glyphAutoOnce.Do(func() {
			if localeUTF8() {
				glyphAuto = unicodeGlyphs
			} else {
				glyphAuto = asciiGlyphs
			}
		})
		return glyphAuto
	}
}

func defaultGlyphs() Glyphs { return glyphSet("auto") }

// localeUTF8 inspects the POSIX locale variables. An unset locale means the
// terminal was never told otherwise, which on macOS, Windows and most Linux
// desktops implies UTF-8 support; only an explicit non-UTF-8 locale downgrades.
func localeUTF8() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := os.Getenv(k)
		if v == "" {
			continue
		}
		u := strings.ToUpper(v)
		return strings.Contains(u, "UTF-8") || strings.Contains(u, "UTF8")
	}
	return true
}

func (g Glyphs) spinner(frame int) string {
	if len(g.Spinner) == 0 {
		return ""
	}
	if frame < 0 {
		frame = -frame
	}
	return g.Spinner[frame%len(g.Spinner)]
}
