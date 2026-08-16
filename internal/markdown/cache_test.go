package markdown

import (
	"strings"
	"testing"
)

func TestCacheKeyIncludesWidthThemeAndVersion(t *testing.T) {
	c := NewCache(64)
	content := "# Header\n\n`code` and **bold**"
	a, err := c.Render("m", 1, 80, "dark", content)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Render("m", 1, 40, "dark", content)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.items) != 2 {
		t.Fatalf("cache entries=%d", len(c.items))
	}
	if a == "" || b == "" {
		t.Fatal("empty render")
	}
	_, _ = c.Render("m", 2, 80, "dark", content+"!")
	_, _ = c.Render("m", 2, 80, "light", content+"!")
	if len(c.items) != 4 {
		t.Fatalf("cache entries=%d", len(c.items))
	}
}

func BenchmarkMarkdownCold(b *testing.B) {
	content := strings.Repeat("## Heading\nText with **bold** and `code`.\n\n```go\nfunc main(){}\n```\n", 80)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := NewCache(64)
		_, _ = c.Render("m", i, 100, "dark", content)
	}
}

func BenchmarkMarkdownCached(b *testing.B) {
	content := strings.Repeat("Text with **bold**.\n", 80)
	c := NewCache(64)
	_, _ = c.Render("m", 1, 100, "dark", content)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = c.Render("m", 1, 100, "dark", content)
	}
}
