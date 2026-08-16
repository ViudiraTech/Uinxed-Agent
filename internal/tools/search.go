package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type GrepTool struct{}

func (*GrepTool) Name() string { return "grep" }
func (*GrepTool) Description() string {
	return "按正则搜索代码。存在 ripgrep 时优先使用 rg（遵守 .gitignore），否则使用 Go fallback。"
}
func (*GrepTool) Category() Category { return CategoryRead }
func (*GrepTool) Schema() map[string]any {
	return obj(map[string]any{"pattern": strp("正则表达式"), "path": strp("搜索目录，默认 ."), "include": strp("文件 glob，如 *.go")}, "pattern")
}
func (*GrepTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if a.Path == "" {
		a.Path = "."
	}
	root, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	if rg, err := exec.LookPath("rg"); err == nil {
		args := []string{"--line-number", "--color", "never", "--no-heading", "--max-count", "200", a.Pattern, root}
		if a.Include != "" {
			args = append([]string{"--glob", a.Include}, args...)
		}
		cmd := exec.CommandContext(ctx, rg, args...)
		out, err := cmd.CombinedOutput()
		if err == nil || len(out) > 0 {
			return Result{Content: truncate(string(out), 64<<10), Metadata: map[string]any{"engine": "rg"}}, nil
		}
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return Result{}, err
	}
	var matches []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(matches) >= 200 {
			return nil
		}
		if a.Include != "" {
			ok, _ := filepath.Match(a.Include, d.Name())
			if !ok {
				return nil
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		n := 0
		for sc.Scan() {
			n++
			if re.MatchString(sc.Text()) {
				rel, _ := filepath.Rel(root, path)
				matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, n, truncate(sc.Text(), 500)))
				if len(matches) >= 200 {
					break
				}
			}
		}
		return nil
	})
	return Result{Content: strings.Join(matches, "\n"), Metadata: map[string]any{"engine": "go", "count": len(matches), "truncated": len(matches) >= 200}}, err
}

type GlobTool struct{}

func (*GlobTool) Name() string { return "glob" }
func (*GlobTool) Description() string {
	return "按 glob 查找工作目录内文件。存在 rg 时使用其文件索引，否则使用 Go fallback。"
}
func (*GlobTool) Category() Category { return CategoryRead }
func (*GlobTool) Schema() map[string]any {
	return obj(map[string]any{"pattern": strp("glob，如 **/*.go"), "path": strp("搜索根目录，默认 .")}, "pattern")
}
func (*GlobTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if a.Path == "" {
		a.Path = "."
	}
	root, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	if rg, err := exec.LookPath("rg"); err == nil {
		cmd := exec.CommandContext(ctx, rg, "--files", "--glob", a.Pattern, root)
		out, e := cmd.Output()
		if e == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) > 500 {
				lines = lines[:500]
			}
			for i := range lines {
				if rel, e := filepath.Rel(root, lines[i]); e == nil {
					lines[i] = rel
				}
			}
			return Result{Content: strings.Join(lines, "\n"), Metadata: map[string]any{"engine": "rg", "count": len(lines)}}, nil
		}
	}
	re, err := globRegex(a.Pattern)
	if err != nil {
		return Result{}, err
	}
	var found []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if path != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if re.MatchString(rel) {
			found = append(found, rel)
		}
		if len(found) >= 500 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(found)
	return Result{Content: strings.Join(found, "\n"), Metadata: map[string]any{"engine": "go", "count": len(found), "truncated": len(found) >= 500}}, err
}

func skipDir(n string) bool {
	switch n {
	case ".git", "node_modules", "vendor", "build", "dist", ".cache":
		return true
	}
	return false
}
func globRegex(pattern string) (*regexp.Regexp, error) {
	p := filepath.ToSlash(pattern)
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '*':
			if i+1 < len(p) && p[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(p[i])
		default:
			b.WriteByte(p[i])
		}
	}
	b.WriteByte('$')
	return regexp.Compile(b.String())
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]"
}
