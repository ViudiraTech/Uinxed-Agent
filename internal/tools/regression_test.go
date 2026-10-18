package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A pattern beginning with "-" must reach rg as a pattern, not as an option.
// Options such as --pre run a command, from a tool the approval layer treats as
// read-only, so this is a command-execution path rather than a cosmetic bug.
func TestGrepPatternCannotInjectFlags(t *testing.T) {
	rg, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("ripgrep is not installed; the Go fallback has no argv to inject")
	}
	root := t.TempDir()
	script := filepath.Join(root, "pre.sh")
	marker := filepath.Join(root, "PWNED")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("--pre="+script+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runTool(t, &GrepTool{}, root, map[string]any{"pattern": "--pre=" + script}); err != nil {
		// A match failure is fine; the assertion is that nothing executed.
		t.Logf("grep returned %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		_ = rg // the binary was found and executed the option
		t.Fatal("the pattern was interpreted as an rg option and executed a command")
	}
}

// Editing through a symlink must change the target and keep the link, not
// replace the link with a regular file while the target keeps its old contents.
func TestEditThroughSymlinkWritesTheTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := runTool(t, &EditFileTool{}, root, map[string]any{"path": "link.txt", "old": "before", "new": "after"}); err != nil {
		t.Fatalf("edit through symlink failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "after\n" {
		t.Fatalf("target content = %q, want the edit to land on it", got)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
}

func TestWriteThroughSymlinkWritesTheTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.txt")
	link := filepath.Join(root, "link.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := runTool(t, &WriteFileTool{}, root, map[string]any{"path": "link.txt", "content": "new\n"}); err != nil {
		t.Fatalf("write through symlink failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new\n" {
		t.Fatalf("target content = %q", got)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a regular file")
	}
}

// A symlink that points outside the working directory must not be followed by a
// write, even though the link itself is inside it.
func TestWriteThroughEscapingSymlinkIsRefused(t *testing.T) {
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Symlink(victim, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := runTool(t, &WriteFileTool{}, root, map[string]any{"path": "link.txt", "content": "overwritten\n"}); err == nil {
		t.Fatal("a write through a symlink that escapes the working directory was allowed")
	}
	got, _ := os.ReadFile(victim)
	if string(got) != "keep\n" {
		t.Fatalf("the outside file was modified: %q", got)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// The tool promises to commit only the listed paths. A bare "git commit" commits
// the whole index, sweeping in whatever happened to be staged already.
func TestGitCommitOnlyCommitsListedPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "t@example.com")
	gitRun(t, dir, "config", "user.name", "t")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "base.txt")
	gitRun(t, dir, "commit", "-q", "-m", "base")

	if err := os.WriteFile(filepath.Join(dir, "wanted.txt"), []byte("wanted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets.env"), []byte("TOKEN=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "wanted.txt", "secrets.env")

	if _, err := runTool(t, &GitCommitTool{}, dir, map[string]any{
		"message": "only wanted",
		"paths":   []map[string]any{{"path": "wanted.txt"}},
	}); err != nil {
		t.Fatalf("git_commit: %v", err)
	}
	committed := gitRun(t, dir, "show", "--name-only", "--pretty=format:", "HEAD")
	if strings.Contains(committed, "secrets.env") {
		t.Fatalf("an unrelated staged file was swept into the commit: %q", committed)
	}
	if !strings.Contains(committed, "wanted.txt") {
		t.Fatalf("the requested file was not committed: %q", committed)
	}
	// And it is still staged, not lost.
	staged := gitRun(t, dir, "diff", "--cached", "--name-only")
	if !strings.Contains(staged, "secrets.env") {
		t.Fatalf("the unrelated file was unstaged: %q", staged)
	}
}

// A command killed by the timeout reports 124 (the timeout convention), not the
// -1 a signal-killed process yields.
func TestBashTimeoutReports124(t *testing.T) {
	res, err := runTool(t, &BashTool{}, t.TempDir(), map[string]any{"cmd": "sleep 10", "timeout": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode == nil {
		t.Fatal("no exit code")
	}
	if *res.ExitCode != 124 {
		t.Fatalf("timeout reported exit %d, want 124", *res.ExitCode)
	}
	if timed, _ := res.Metadata["timed_out"].(bool); !timed {
		t.Fatal("timed_out metadata is false")
	}
}
