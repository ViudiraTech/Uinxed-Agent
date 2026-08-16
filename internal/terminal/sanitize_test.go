package terminal

import "testing"

func TestSanitizeTextStripsTerminalInjection(t *testing.T) {
	in := "ok\x1b[2J\x1b[Hafter\n\x1b]52;c;Y2xpcGJvYXJk\x07safe\rX"
	got := SanitizeText(in)
	if got != "okafter\nsafeX" {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeTextPreservesUnicodeAndTabs(t *testing.T) {
	in := "中文\t🙂\nnext"
	if got := SanitizeText(in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestSanitizeTextStripsBidiOverrides(t *testing.T) {
	if got := SanitizeText("abc\u202egolang"); got != "abcgolang" {
		t.Fatalf("got %q", got)
	}
}
