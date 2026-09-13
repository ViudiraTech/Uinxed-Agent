package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MultiEditTool applies several replacements to one file in a single atomic
// commit. Every old_string must match exactly once before anything is written:
// a partial application would leave the file in a state the model did not
// intend, which is the failure mode this tool exists to prevent.
type MultiEditTool struct{}

func (*MultiEditTool) Name() string { return "multi_edit" }
func (*MultiEditTool) Description() string {
	return "对单个文件做多处替换并一次性原子写回；任一 old_string 不匹配或匹配多处则整体失败，不落盘。"
}
func (*MultiEditTool) Category() Category { return CategoryWrite }
func (*MultiEditTool) Schema() map[string]any {
	return obj(map[string]any{
		"path":  strp("文件路径"),
		"edits": arr(map[string]any{"old_string": strp("要替换的原文"), "new_string": strp("替换后的内容")}, "按顺序应用的替换列表"),
	}, "path", "edits")
}

type multiEdit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func (*MultiEditTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path  string      `json:"path"`
		Edits []multiEdit `json:"edits"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if len(a.Edits) == 0 {
		return Result{}, errors.New("edits 不能为空")
	}
	p, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Result{}, err
	}
	if isBinary(data) {
		return Result{}, errors.New("refusing to edit binary file")
	}
	// Validate every replacement against the evolving buffer. Sequential
	// application means a later edit may target text an earlier one produced;
	// validation must therefore replay the same order.
	buf := data
	for i, e := range a.Edits {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		if e.OldString == "" {
			return Result{}, fmt.Errorf("edit %d: old_string 不能为空", i+1)
		}
		count := bytes.Count(buf, []byte(e.OldString))
		if count == 0 {
			return Result{}, fmt.Errorf("edit %d: 未找到要替换的文本（old_string 不匹配），已放弃全部修改", i+1)
		}
		if count > 1 {
			return Result{}, fmt.Errorf("edit %d: old_string 出现 %d 次，不唯一，已放弃全部修改", i+1, count)
		}
		buf = bytes.Replace(buf, []byte(e.OldString), []byte(e.NewString), 1)
	}
	if err := atomicWrite(ctx, p, buf); err != nil {
		return Result{}, err
	}
	return Result{Content: fmt.Sprintf("applied %d edits", len(a.Edits)), Metadata: map[string]any{"path": p, "replaced": len(a.Edits)}}, nil
}

type DeleteFileTool struct{}

func (*DeleteFileTool) Name() string { return "delete_file" }
func (*DeleteFileTool) Description() string {
	return "删除工作目录内的文件；不跟随符号链接，不删除目录。"
}
func (*DeleteFileTool) Category() Category { return CategoryWrite }
func (*DeleteFileTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("文件路径")}, "path")
}

func (*DeleteFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct{ Path string }
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	p, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	// RemoveAll would happily delete a directory tree, and even os.Remove
	// deletes an empty directory; this tool is scoped to single files so a
	// mistaken path cannot destroy a directory.
	if st, err := os.Lstat(p); err == nil && st.IsDir() {
		return Result{}, errors.New("refusing to delete a directory; use bash rm for that")
	}
	if err := os.Remove(p); err != nil {
		return Result{}, err
	}
	return Result{Content: "deleted", Metadata: map[string]any{"path": p}}, nil
}

type MoveFileTool struct{}

func (*MoveFileTool) Name() string { return "move_file" }
func (*MoveFileTool) Description() string {
	return "移动或重命名工作目录内的文件；目标父目录自动创建，不覆盖已存在的目标。"
}
func (*MoveFileTool) Category() Category { return CategoryWrite }
func (*MoveFileTool) Schema() map[string]any {
	return obj(map[string]any{"src": strp("源路径"), "dst": strp("目标路径")}, "src", "dst")
}

func (*MoveFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct{ Src, Dst string }
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	src, err := resolveInside(env.CWD, a.Src, false)
	if err != nil {
		return Result{}, err
	}
	dst, err := resolveInside(env.CWD, a.Dst, true)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return Result{}, fmt.Errorf("destination already exists: %s", a.Dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Result{}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return Result{}, err
	}
	return Result{Content: "moved", Metadata: map[string]any{"src": src, "dst": dst}}, nil
}

type CopyFileTool struct{}

func (*CopyFileTool) Name() string { return "copy_file" }
func (*CopyFileTool) Description() string {
	return "复制工作目录内的文件；目标父目录自动创建，不覆盖已存在的目标。"
}
func (*CopyFileTool) Category() Category { return CategoryWrite }
func (*CopyFileTool) Schema() map[string]any {
	return obj(map[string]any{"src": strp("源路径"), "dst": strp("目标路径")}, "src", "dst")
}

func (*CopyFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct{ Src, Dst string }
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	src, err := resolveInside(env.CWD, a.Src, false)
	if err != nil {
		return Result{}, err
	}
	dst, err := resolveInside(env.CWD, a.Dst, true)
	if err != nil {
		return Result{}, err
	}
	if _, err := os.Lstat(dst); err == nil {
		return Result{}, fmt.Errorf("destination already exists: %s", a.Dst)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return Result{}, err
	}
	if err := atomicWrite(ctx, dst, data); err != nil {
		return Result{}, err
	}
	return Result{Content: "copied", Metadata: map[string]any{"src": src, "dst": dst, "bytes": len(data)}}, nil
}

type MakeDirTool struct{}

func (*MakeDirTool) Name() string { return "make_dir" }
func (*MakeDirTool) Description() string {
	return "在工作目录内创建目录（含父目录）。"
}
func (*MakeDirTool) Category() Category { return CategoryWrite }
func (*MakeDirTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("目录路径")}, "path")
}

func (*MakeDirTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct{ Path string }
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	p, err := resolveInside(env.CWD, a.Path, true)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return Result{}, err
	}
	return Result{Content: "created", Metadata: map[string]any{"path": p}}, nil
}

// arr builds a JSON-schema array-of-objects property.
func arr(items map[string]any, desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type":                 "object",
			"properties":           items,
			"required":             []string{"old_string", "new_string"},
			"additionalProperties": false,
		},
	}
}
