package markdown

import (
	"regexp"
	"strings"
	"testing"
)

// stripANSI removes SGR sequences so assertions can look for rendered text
// across the per-run color resets glamour emits.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return sgr.ReplaceAllString(s, "") }

var testStyle = Style{
	Muted: "#565F89", Primary: "#7AA2F7", Secondary: "#BB9AF7",
	Accent: "#7DCFFF", Tool: "#BB9AF7", Success: "#9ECE6A", Error: "#F7768E",
	Border: "#292E42", CodeTheme: "onedark",
}

func TestCacheKeyIncludesWidthStyleAndVersion(t *testing.T) {
	c := NewCache(64)
	content := "# Header\n\n`code` and **bold**"
	a, err := c.Render("m", 1, 80, "dark", content, testStyle)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Render("m", 1, 40, "dark", content, testStyle)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.items) != 2 {
		t.Fatalf("cache entries=%d", len(c.items))
	}
	if a == "" || b == "" {
		t.Fatal("empty render")
	}
	_, _ = c.Render("m", 2, 80, "dark", content+"!", testStyle)
	_, _ = c.Render("m", 2, 80, "light", content+"!", testStyle)
	if len(c.items) != 4 {
		t.Fatalf("cache entries=%d", len(c.items))
	}
}

// TestStyleMapsMarkdownToTheme pins the reason the renderer takes a Style at
// all: headings and inline code must come out in the caller's palette rather
// than glamour's built-in preset.
func TestStyleMapsMarkdownToTheme(t *testing.T) {
	c := NewCache(64)
	out, err := c.Render("m", 1, 60, "dark", "# Heading\n\n`inline`", testStyle)
	if err != nil {
		t.Fatal(err)
	}
	// lipgloss emits 24-bit color as "38;2;R;G;B"; check the heading's Primary
	// and the inline code's Accent both made it into the output.
	if !strings.Contains(out, "38;2;122;162;247") {
		t.Errorf("heading should use the theme primary color:\n%q", out)
	}
	if !strings.Contains(out, "38;2;125;207;255") {
		t.Errorf("inline code should use the theme accent color:\n%q", out)
	}
}

// TestNoColorStyleEmitsNoColorEscapes is the NO_COLOR contract for Markdown: an
// empty Style must not emit color, though bold/italic attributes may remain.
func TestNoColorStyleEmitsNoColorEscapes(t *testing.T) {
	c := NewCache(64)
	out, err := c.Render("m", 1, 60, "plain", "# Heading\n\n`inline` and **bold**", Style{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
		t.Fatalf("empty style must not emit color sequences:\n%q", out)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "Heading") || !strings.Contains(plain, "inline") {
		t.Fatalf("content missing from plain render:\n%q", plain)
	}
}

// TestListPrefixesPreserved guards against rebuilding the style config from
// scratch silently dropping bullet markers.
func TestListPrefixesPreserved(t *testing.T) {
	c := NewCache(64)
	out, err := c.Render("m", 1, 60, "dark", "- one\n- two\n\n1. first\n2. second", testStyle)
	if err != nil {
		t.Fatal(err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "• one") || !strings.Contains(plain, "• two") {
		t.Errorf("bullet list markers missing:\n%q", plain)
	}
	if !strings.Contains(plain, "1. first") || !strings.Contains(plain, "2. second") {
		t.Errorf("enumeration markers missing:\n%q", plain)
	}
}

func BenchmarkMarkdownCold(b *testing.B) {
	content := strings.Repeat("## Heading\nText with **bold** and `code`.\n\n```go\nfunc main(){}\n```\n", 80)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := NewCache(64)
		_, _ = c.Render("m", i, 100, "dark", content, testStyle)
	}
}

func BenchmarkMarkdownCached(b *testing.B) {
	content := strings.Repeat("Text with **bold**.\n", 80)
	c := NewCache(64)
	_, _ = c.Render("m", 1, 100, "dark", content, testStyle)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render("m", 1, 100, "dark", content, testStyle)
	}
}
