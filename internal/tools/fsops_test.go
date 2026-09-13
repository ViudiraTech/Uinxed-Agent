package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mustWrite is test scaffolding for file fixtures.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMultiEditAppliesAllOrNothing(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "alpha\nbeta\ngamma\n")
	res, err := runTool(t, &MultiEditTool{}, root, map[string]any{
		"path": "f.txt",
		"edits": []map[string]string{
			{"old_string": "alpha", "new_string": "ALPHA"},
			{"old_string": "gamma", "new_string": "GAMMA"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "2 edits") {
		t.Fatalf("result=%q", res.Content)
	}
	got, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(got) != "ALPHA\nbeta\nGAMMA\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestMultiEditSequentialEditsCanCompose(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "one two\n")
	// A later edit may target text an earlier one produced.
	_, err := runTool(t, &MultiEditTool{}, root, map[string]any{
		"path": "f.txt",
		"edits": []map[string]string{
			{"old_string": "one", "new_string": "ONE"},
			{"old_string": "ONE two", "new_string": "ONE TWO"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(got) != "ONE TWO\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestMultiEditRejectsMissingMatchWithoutWriting(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "keep me\n")
	_, err := runTool(t, &MultiEditTool{}, root, map[string]any{
		"path": "f.txt",
		"edits": []map[string]string{
			{"old_string": "keep", "new_string": "DROP"},
			{"old_string": "not present anywhere", "new_string": "x"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not match") && !strings.Contains(err.Error(), "匹配") {
		t.Fatalf("err=%v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(got) != "keep me\n" {
		t.Fatalf("failed batch must not touch the file, got %q", got)
	}
}

func TestMultiEditRejectsAmbiguousMatch(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "f.txt"), "x x\n")
	_, err := runTool(t, &MultiEditTool{}, root, map[string]any{
		"path":  "f.txt",
		"edits": []map[string]string{{"old_string": "x", "new_string": "y"}},
	})
	if err == nil {
		t.Fatal("ambiguous edit accepted")
	}
}

func TestDeleteFileRemovesSingleFileOnly(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "d/f.txt"), "x")
	if _, err := runTool(t, &DeleteFileTool{}, root, map[string]any{"path": "d/f.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "d/f.txt")); !os.IsNotExist(err) {
		t.Fatal("file still present")
	}
	if _, err := os.Stat(filepath.Join(root, "d")); err != nil {
		t.Fatal("parent directory must survive")
	}
	// A directory must be refused: this tool never deletes trees.
	if _, err := runTool(t, &DeleteFileTool{}, root, map[string]any{"path": "d"}); err == nil {
		t.Fatal("directory delete accepted")
	}
}

func TestMoveAndCopyFile(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src.txt"), "payload")
	if _, err := runTool(t, &CopyFileTool{}, root, map[string]any{"src": "src.txt", "dst": "sub/copy.txt"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "sub", "copy.txt"))
	if string(got) != "payload" {
		t.Fatalf("copy=%q", got)
	}
	if _, err := runTool(t, &MoveFileTool{}, root, map[string]any{"src": "src.txt", "dst": "sub/renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "src.txt")); !os.IsNotExist(err) {
		t.Fatal("source survived a move")
	}
	// Neither may clobber an existing destination.
	if _, err := runTool(t, &CopyFileTool{}, root, map[string]any{"src": "sub/copy.txt", "dst": "sub/renamed.txt"}); err == nil {
		t.Fatal("copy overwrote an existing destination")
	}
	if _, err := runTool(t, &MoveFileTool{}, root, map[string]any{"src": "sub/copy.txt", "dst": "sub/renamed.txt"}); err == nil {
		t.Fatal("move overwrote an existing destination")
	}
}

func TestMakeDirCreatesParents(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, &MakeDirTool{}, root, map[string]any{"path": "a/b/c"}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(root, "a", "b", "c"))
	if err != nil || !st.IsDir() {
		t.Fatalf("stat=%v err=%v", st, err)
	}
}

func TestMakeDirRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, &MakeDirTool{}, root, map[string]any{"path": "../outside"}); err == nil {
		t.Fatal("../ escape accepted")
	}
}

func TestFileOpsRejectTraversal(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	cases := []struct {
		tool Tool
		args map[string]any
	}{
		{&MultiEditTool{}, map[string]any{"path": "../victim.txt", "edits": []map[string]string{{"old_string": "a", "new_string": "b"}}}},
		{&DeleteFileTool{}, map[string]any{"path": "../victim.txt"}},
		{&CopyFileTool{}, map[string]any{"src": "../victim.txt", "dst": "ok.txt"}},
		{&CopyFileTool{}, map[string]any{"src": "ok.txt", "dst": "../evil.txt"}},
		{&MoveFileTool{}, map[string]any{"src": "../victim.txt", "dst": "ok.txt"}},
		{&MoveFileTool{}, map[string]any{"src": "ok.txt", "dst": "../evil.txt"}},
		{&GitCommitTool{}, map[string]any{"message": "m", "paths": []map[string]string{{"path": "../victim.txt"}}}},
	}
	mustWrite(t, filepath.Join(outside, "victim.txt"), "do not touch")
	for i, c := range cases {
		if _, err := runTool(t, c.tool, root, c.args); err == nil {
			t.Errorf("case %d: ../ escape accepted", i)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(outside, "victim.txt")); string(got) != "do not touch" {
		t.Fatalf("outside file was modified: %q", got)
	}
}

func TestFileOpsRejectSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "secret")
	mustWrite(t, filepath.Join(root, "inside.txt"), "inside")
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	// A symlink pointing outside must not become a writing position, for
	// create-style operations the target is checked through its real prefix.
	if _, err := runTool(t, &CopyFileTool{}, root, map[string]any{"src": "inside.txt", "dst": "link/escape.txt"}); err == nil {
		t.Fatal("copy through escaping symlink accepted")
	}
	if _, err := runTool(t, &MoveFileTool{}, root, map[string]any{"src": "inside.txt", "dst": "link/escape.txt"}); err == nil {
		t.Fatal("move through escaping symlink accepted")
	}
	if _, err := runTool(t, &MakeDirTool{}, root, map[string]any{"path": "link/escape"}); err == nil {
		t.Fatal("make_dir through escaping symlink accepted")
	}
	// Deleting the link target's content through the link is refused as well;
	// the link resolves outside the root.
	if _, err := runTool(t, &DeleteFileTool{}, root, map[string]any{"path": "link/secret.txt"}); err == nil {
		t.Fatal("delete through escaping symlink accepted")
	}
	if got, _ := os.ReadFile(filepath.Join(outside, "secret.txt")); string(got) != "secret" {
		t.Fatalf("outside file modified: %q", got)
	}
}

// TestTreeRendersAndIgnores covers depth limits and the fixed ignore set.
func TestTreeRendersAndIgnores(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), "package main")
	mustWrite(t, filepath.Join(root, "internal", "deep", "leaf.go"), "package deep")
	mustWrite(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "js")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := runTool(t, &TreeTool{}, root, map[string]any{"path": ".", "depth": 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"main.go", "internal/", "deep/", "leaf.go"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("tree missing %q:\n%s", want, res.Content)
		}
	}
	if strings.Contains(res.Content, "node_modules") || strings.Contains(res.Content, ".git") {
		t.Errorf("tree leaked ignored entries:\n%s", res.Content)
	}
}

func TestTreeDepthLimit(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a", "b", "c", "d", "deep.txt"), "x")
	res, err := runTool(t, &TreeTool{}, root, map[string]any{"depth": 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "deep.txt") {
		t.Errorf("depth=2 leaked level-4 content:\n%s", res.Content)
	}
}

func TestTreeRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, &TreeTool{}, root, map[string]any{"path": ".."}); err == nil {
		t.Fatal("tree accepted a path outside the working directory")
	}
}

// TestGitCommitAddsOnlyListedPaths runs against a real temporary repository.
func TestGitCommitAddsOnlyListedPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	if _, err := runGit(context.Background(), "-C", root, "init"); err != nil {
		t.Skip("git init failed:", err)
	}
	runGit(context.Background(), "-C", root, "config", "user.email", "t@example.com")
	runGit(context.Background(), "-C", root, "config", "user.name", "t")
	mustWrite(t, filepath.Join(root, "commit_me.txt"), "yes")
	mustWrite(t, filepath.Join(root, "leave_me.txt"), "no")
	if _, err := runTool(t, &GitCommitTool{}, root, map[string]any{
		"message": "only one file",
		"paths":   []map[string]string{{"path": "commit_me.txt"}},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := runTool(t, &GitStatusTool{}, root, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "leave_me.txt") {
		t.Fatalf("unlisted file was committed: %s", out.Content)
	}
	if strings.Contains(out.Content, "commit_me.txt") {
		t.Fatalf("listed file did not commit cleanly: %s", out.Content)
	}
}

func TestGitCommitRefusesTraversalAndEmptyMessage(t *testing.T) {
	root := t.TempDir()
	if _, err := runTool(t, &GitCommitTool{}, root, map[string]any{
		"message": "",
		"paths":   []map[string]string{{"path": "x"}},
	}); err == nil {
		t.Fatal("empty message accepted")
	}
	if _, err := runTool(t, &GitCommitTool{}, root, map[string]any{
		"message": "m",
		"paths":   []map[string]string{{"path": "../x"}},
	}); err == nil {
		t.Fatal("../ path accepted by git_commit")
	}
	if _, err := runTool(t, &GitCommitTool{}, root, map[string]any{"message": "m"}); err == nil {
		t.Fatal("missing paths accepted")
	}
}

func TestNewToolsAreRegisteredWithExpectedCategories(t *testing.T) {
	r := DefaultRegistry()
	cases := map[string]Category{
		"multi_edit": CategoryWrite, "delete_file": CategoryWrite, "move_file": CategoryWrite,
		"copy_file": CategoryWrite, "make_dir": CategoryWrite,
		"git_status": CategoryRead, "git_diff": CategoryRead, "git_log": CategoryRead,
		"git_commit": CategoryWrite, "tree": CategoryRead,
	}
	for name, want := range cases {
		got, ok := r.CategoryOf(name)
		if !ok {
			t.Errorf("%s not registered", name)
			continue
		}
		if got != want {
			t.Errorf("%s category = %q, want %q", name, got, want)
		}
	}
	if _, ok := r.CategoryOf("nonexistent"); ok {
		t.Error("CategoryOf reported a category for an unknown tool")
	}
	if c, ok := CategoryOf("multi_edit"); !ok || c != CategoryWrite {
		t.Errorf("package-level CategoryOf = (%q, %v)", c, ok)
	}
}

func TestGitToolsRejectBinaryGarbageArgs(t *testing.T) {
	root := t.TempDir()
	_, err := (&GitCommitTool{}).Execute(context.Background(), json.RawMessage(`{"message":"m","paths":[{"path":123}]}`), ExecutionContext{CWD: root})
	if err == nil {
		t.Fatal("malformed arguments accepted")
	}
}
