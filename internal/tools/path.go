package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func resolveInside(root, input string, forCreate bool) (string, error) {
	if root == "" {
		return "", errors.New("working directory is empty")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal := rootAbs
	if v, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootReal = v
	}
	var p string
	if filepath.IsAbs(input) {
		p = filepath.Clean(input)
	} else {
		p = filepath.Join(rootAbs, input)
	}
	p, err = filepath.Abs(p)
	if err != nil {
		return "", err
	}
	check := p
	if forCreate {
		parent := filepath.Dir(p)
		if real, err := evalExistingPrefix(parent); err == nil {
			check = filepath.Join(real, filepath.Base(p))
		}
	} else if real, err := filepath.EvalSymlinks(p); err == nil {
		check = real
	}
	rel, err := filepath.Rel(rootReal, check)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes working directory")
	}
	return p, nil
}

// writeTarget resolves a path for a write. When the final component is a symlink
// the *target* is written, so the link survives and the change lands where the
// user sees the file. Resolving is also the security check: a link pointing
// outside the working directory must be refused, not followed by a write.
func writeTarget(root, input string) (string, error) {
	resolved, err := resolveInside(root, input, true)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		// Nothing there yet: the write creates the file at the resolved path.
		return resolved, nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal := rootAbs
	if v, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootReal = v
	}
	rel, err := filepath.Rel(rootReal, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes working directory")
	}
	return real, nil
}

func evalExistingPrefix(p string) (string, error) {
	cur := p
	var suffix []string
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				real = filepath.Join(real, suffix[i])
			}
			return real, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func isBinary(data []byte) bool {
	if len(data) > 8192 {
		data = data[:8192]
	}
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
