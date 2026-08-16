package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestDiffTrackedAndUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "tracked.txt")
	runGit(t, dir, "commit", "-qm", "base")
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Diff(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Files) != 2 {
		t.Fatalf("files=%d: %#v", len(s.Files), s.Files)
	}
	if !strings.Contains(s.Unified, "tracked.txt") || !strings.Contains(s.Unified, "new.txt") {
		t.Fatalf("unified missing files:\n%s", s.Unified)
	}
	text, err := FileDiff(context.Background(), dir, "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "+hello") {
		t.Fatalf("untracked diff missing content: %s", text)
	}
}
