package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type File struct {
	Status         string
	Path           string
	Added, Deleted int
}
type Snapshot struct {
	Files   []File
	Unified string
}

func Diff(ctx context.Context, cwd string) (Snapshot, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return Snapshot{}, err
	}
	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	raw, err := cmd.Output()
	if err != nil {
		return Snapshot{}, err
	}
	parts := strings.Split(string(raw), "\x00")
	var files []File
	for _, p := range parts {
		if len(p) < 4 {
			continue
		}
		status := strings.TrimSpace(p[:2])
		path := strings.TrimSpace(p[3:])
		if path != "" {
			files = append(files, File{Status: status, Path: filepath.ToSlash(path)})
		}
	}
	diffCmd := exec.CommandContext(ctx, "git", "-C", cwd, "diff", "--no-ext-diff", "--no-color", "--find-renames", "HEAD")
	d, _ := diffCmd.Output()
	var b strings.Builder
	b.Write(d)
	for _, f := range files {
		if strings.Contains(f.Status, "?") {
			u := exec.CommandContext(ctx, "git", "-C", cwd, "diff", "--no-index", "--", os.DevNull, f.Path)
			x, _ := u.Output()
			b.Write(x)
		}
	}
	// Parse simple +/- counts.
	lines := strings.Split(b.String(), "\n")
	idx := -1
	for _, l := range lines {
		if strings.HasPrefix(l, "+++ b/") {
			p := strings.TrimPrefix(l, "+++ b/")
			idx = -1
			for i := range files {
				if files[i].Path == p {
					idx = i
					break
				}
			}
			continue
		}
		if idx >= 0 && strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			files[idx].Added++
		}
		if idx >= 0 && strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			files[idx].Deleted++
		}
	}
	return Snapshot{Files: files, Unified: b.String()}, nil
}

func FileDiff(ctx context.Context, cwd, path string) (string, error) {
	if strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("invalid path")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "diff", "--no-ext-diff", "--no-color", "--find-renames", "HEAD", "--", path)
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return string(out), nil
	}
	cmd = exec.CommandContext(ctx, "git", "-C", cwd, "diff", "--no-index", "--", os.DevNull, path)
	out, e := cmd.Output()
	if len(out) > 0 {
		return string(out), nil
	}
	return "", e
}
