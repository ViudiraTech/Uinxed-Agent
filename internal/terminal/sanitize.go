package terminal

import (
	"strings"
	"unicode/utf8"
)

// SanitizeText removes terminal control sequences from untrusted model/tool/user
// text while preserving ordinary Unicode, newlines and tabs. In particular it
// strips CSI/OSC escape sequences (including OSC 52 clipboard sequences), C0/C1
// controls, carriage returns and bidi override/isolate controls that can spoof
// terminal layout.
func SanitizeText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch c {
		case 0x1b: // ESC
			i++
			if i >= len(s) {
				continue
			}
			switch s[i] {
			case '[': // CSI: ESC [ ... final-byte
				i++
				for i < len(s) {
					v := s[i]
					i++
					if v >= 0x40 && v <= 0x7e {
						break
					}
				}
			case ']': // OSC: ESC ] ... BEL or ST (ESC \\)
				i++
				for i < len(s) {
					if s[i] == 0x07 {
						i++
						break
					}
					if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			default:
				// Two-byte ESC sequences (cursor save/restore, charset select, etc.).
				i++
			}
			continue
		case 0x9b: // 8-bit CSI
			i++
			for i < len(s) {
				v := s[i]
				i++
				if v >= 0x40 && v <= 0x7e {
					break
				}
			}
			continue
		case 0x9d: // 8-bit OSC
			i++
			for i < len(s) && s[i] != 0x07 {
				i++
			}
			if i < len(s) {
				i++
			}
			continue
		case '\n', '\t':
			b.WriteByte(c)
			i++
			continue
		case '\r':
			// A bare CR can overwrite the beginning of a rendered terminal line.
			i++
			continue
		}
		if c < 0x20 || (c >= 0x7f && c <= 0x9f) {
			i++
			continue
		}
		// Decode UTF-8 through a range so we can remove bidi/zero-width controls
		// without damaging non-ASCII text.
		if c >= 0x80 {
			r, size := decodeRune(s[i:])
			if size <= 0 {
				i++
				continue
			}
			i += size
			if isSpoofingControl(r) {
				continue
			}
			b.WriteRune(r)
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func decodeRune(s string) (rune, int) {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 0 {
		return 0, 0
	}
	return r, size
}

func isSpoofingControl(r rune) bool {
	switch r {
	case '\u200b', '\u200e', '\u200f',
		'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
		'\u2066', '\u2067', '\u2068', '\u2069', '\ufeff':
		return true
	}
	return false
}
