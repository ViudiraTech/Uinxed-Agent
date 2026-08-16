package indexer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSearchAndIgnoreHeavyDirs(t *testing.T) {
	root := t.TempDir()
	for p, body := range map[string]string{"internal/agent/runtime.go": "x", "README.md": "x", "node_modules/pkg/a.js": "x", ".git/objects/x": "x"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := New(root)
	defer idx.Close()
	if err := idx.Build(context.Background()); err != nil {
		t.Fatal(err)
	}
	files := idx.Files()
	for _, p := range files {
		if p == "node_modules/pkg/a.js" || p == ".git/objects/x" {
			t.Fatalf("ignored path indexed: %s", p)
		}
	}
	m := idx.Search("runt", 5)
	if len(m) == 0 || m[0].Path != "internal/agent/runtime.go" {
		t.Fatalf("matches=%#v files=%#v", m, files)
	}
}

func BenchmarkFuzzySearch10k(b *testing.B) {
	idx := New(".")
	idx.files = make([]string, 10000)
	for i := range idx.files {
		idx.files[i] = "internal/pkg/file_" + itoa(i) + "_runtime.go"
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("runtime", 20)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var a [20]byte
	i := len(a)
	for n > 0 {
		i--
		a[i] = byte('0' + n%10)
		n /= 10
	}
	return string(a[i:])
}
