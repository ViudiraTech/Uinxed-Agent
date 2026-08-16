package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runTool(t *testing.T, tool Tool, cwd string, v any) (Result, error) {
	t.Helper()
	b, _ := json.Marshal(v)
	return tool.Execute(context.Background(), b, ExecutionContext{CWD: cwd})
}

func TestFileToolsAtomicReadEditAndBoundary(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, &WriteFileTool{}, root, map[string]any{"path": "a/b.txt", "content": "one\ntwo\n"}); err != nil {
		t.Fatal(err)
	}
	res, err := runTool(t, &ReadFileTool{}, root, map[string]any{"path": "a/b.txt", "offset": 1, "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "1: one") || !strings.Contains(res.Content, "2: two") {
		t.Fatalf("read=%q", res.Content)
	}
	if _, err := runTool(t, &EditFileTool{}, root, map[string]any{"path": "a/b.txt", "old": "two", "new": "TWO"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "a", "b.txt"))
	if string(got) != "one\nTWO\n" {
		t.Fatalf("edited=%q", got)
	}
	if _, err := runTool(t, &WriteFileTool{}, root, map[string]any{"path": "../escape.txt", "content": "bad"}); err == nil {
		t.Fatal("path escape unexpectedly allowed")
	}
}

func TestEditRequiresUniqueOldText(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "x.txt")
	_ = os.WriteFile(p, []byte("x x"), 0o644)
	_, err := runTool(t, &EditFileTool{}, root, map[string]any{"path": "x.txt", "old": "x", "new": "y"})
	if err == nil || !strings.Contains(err.Error(), "不唯一") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadRejectsBinary(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "bin"), []byte{0, 1, 2, 3}, 0o644)
	_, err := runTool(t, &ReadFileTool{}, root, map[string]any{"path": "bin"})
	if err == nil {
		t.Fatal("binary read unexpectedly allowed")
	}
}

func TestSymlinkCannotEscapeRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600)
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "link")); err != nil {
		t.Skip(err)
	}
	_, err := runTool(t, &ReadFileTool{}, root, map[string]any{"path": "link"})
	if err == nil {
		t.Fatal("escaping symlink unexpectedly allowed")
	}
}
