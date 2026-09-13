package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// TreeTool renders a directory tree so a model can build a structural picture
// of a project in one call instead of walking list_dir level by level.
type TreeTool struct{}

func (*TreeTool) Name() string { return "tree" }
func (*TreeTool) Description() string {
	return "以树形列出目录结构；depth 限制深度（默认 3，最大 6），忽略 .git 与 node_modules。"
}
func (*TreeTool) Category() Category { return CategoryRead }
func (*TreeTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("目录路径，默认 ."), "depth": nump("最大深度，默认 3")})
}

// treeIgnore is the fixed ignore set. It is deliberately not configurable:
// the entries are noise in every repository, and a knob the model can turn is
// a knob it will turn to escape a huge directory.
var treeIgnore = map[string]bool{".git": true, "node_modules": true}

const (
	treeMaxDepth     = 6
	treeMaxEntries   = 2000
	treeMaxDirsAtLvl = 200
)

func (*TreeTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path  string `json:"path"`
		Depth int    `json:"depth"`
	}
	_ = decode(raw, &a)
	if a.Path == "" {
		a.Path = "."
	}
	if a.Depth <= 0 {
		a.Depth = 3
	}
	if a.Depth > treeMaxDepth {
		a.Depth = treeMaxDepth
	}
	root, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("not a directory: %s", a.Path)
	}
	var b strings.Builder
	shown := 0
	truncated := false
	var walk func(dir string, prefix string, depth int)
	walk = func(dir string, prefix string, depth int) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if depth > a.Depth || shown >= treeMaxEntries {
			truncated = true
			return
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		sort.Slice(ents, func(i, j int) bool {
			di, dj := ents[i].IsDir(), ents[j].IsDir()
			if di != dj {
				return di
			}
			return ents[i].Name() < ents[j].Name()
		})
		count := 0
		for _, e := range ents {
			if treeIgnore[e.Name()] {
				continue
			}
			count++
		}
		if count > treeMaxDirsAtLvl {
			count = treeMaxDirsAtLvl
			truncated = true
		}
		i := 0
		for _, e := range ents {
			if treeIgnore[e.Name()] || i >= count {
				if i >= count {
					break
				}
				continue
			}
			i++
			if shown >= treeMaxEntries {
				truncated = true
				return
			}
			shown++
			connector, childPrefix := "├── ", prefix+"│   "
			if i == count {
				connector, childPrefix = "└── ", prefix+"    "
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			} else if info, err := e.Info(); err == nil && info.Mode()&os.ModeSymlink != 0 {
				name += " -> " + linkTarget(dir, name)
			}
			fmt.Fprintf(&b, "%s%s%s\n", prefix, connector, name)
			if e.IsDir() {
				walk(filepath.Join(dir, e.Name()), childPrefix, depth+1)
			}
		}
	}
	walk(root, "", 1)
	if truncated || shown >= treeMaxEntries {
		b.WriteString("…\n")
	}
	return Result{Content: strings.TrimSuffix(b.String(), "\n"), Metadata: map[string]any{"path": root, "entries": shown, "truncated": truncated}}, nil
}

func linkTarget(dir, name string) string {
	dst, err := os.Readlink(filepath.Join(dir, name))
	if err != nil {
		return "?"
	}
	return dst
}
