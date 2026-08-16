package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ReadFileTool struct{}

func (*ReadFileTool) Name() string { return "read_file" }
func (*ReadFileTool) Description() string {
	return "读取工作目录内的文本文件，支持按行范围读取并检测二进制文件。"
}
func (*ReadFileTool) Category() Category { return CategoryRead }
func (*ReadFileTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("文件路径"), "offset": nump("起始行号（1 开始）"), "limit": nump("读取行数，默认 500，最大 5000")}, "path")
}
func (*ReadFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	p, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(p)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := f.Read(head)
	if isBinary(head[:n]) {
		return Result{}, errors.New("refusing to read binary file")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Result{}, err
	}
	if a.Offset < 1 {
		a.Offset = 1
	}
	if a.Limit <= 0 {
		a.Limit = 500
	}
	if a.Limit > 5000 {
		a.Limit = 5000
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var b strings.Builder
	line := 0
	written := 0
	truncated := false
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		line++
		if line < a.Offset {
			continue
		}
		if written >= a.Limit {
			truncated = true
			continue
		}
		fmt.Fprintf(&b, "%d: %s\n", line, sc.Text())
		written++
	}
	if err := sc.Err(); err != nil {
		return Result{}, err
	}
	return Result{Content: strings.TrimSuffix(b.String(), "\n"), Metadata: map[string]any{"path": p, "totalLines": line, "truncated": truncated}}, nil
}

type WriteFileTool struct{}

func (*WriteFileTool) Name() string { return "write_file" }
func (*WriteFileTool) Description() string {
	return "原子覆盖写入工作目录内的文本文件；自动创建父目录并尽量保留现有权限。"
}
func (*WriteFileTool) Category() Category { return CategoryWrite }
func (*WriteFileTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("文件路径"), "content": strp("文件完整内容")}, "path", "content")
}
func (*WriteFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct{ Path, Content string }
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	p, err := resolveInside(env.CWD, a.Path, true)
	if err != nil {
		return Result{}, err
	}
	if err := atomicWrite(ctx, p, []byte(a.Content)); err != nil {
		return Result{}, err
	}
	return Result{Content: "written", Metadata: map[string]any{"path": p, "bytes": len(a.Content)}}, nil
}

type EditFileTool struct{}

func (*EditFileTool) Name() string { return "edit_file" }
func (*EditFileTool) Description() string {
	return "在工作目录内精确替换唯一文本片段，使用原子写入；old 必须恰好出现一次。"
}
func (*EditFileTool) Category() Category { return CategoryWrite }
func (*EditFileTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("文件路径"), "old": strp("要替换的原文"), "new": strp("替换后的内容")}, "path", "old", "new")
}
func (*EditFileTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if a.Old == "" {
		return Result{}, errors.New("old 不能为空")
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
	count := bytes.Count(data, []byte(a.Old))
	if count == 0 {
		return Result{}, errors.New("未找到要替换的文本（old 不匹配）")
	}
	if count > 1 {
		return Result{}, fmt.Errorf("old 文本出现 %d 次，不唯一，请包含更多上下文", count)
	}
	next := bytes.Replace(data, []byte(a.Old), []byte(a.New), 1)
	if err := atomicWrite(ctx, p, next); err != nil {
		return Result{}, err
	}
	return Result{Content: "edited", Metadata: map[string]any{"path": p, "replaced": 1}}, nil
}

type ListDirTool struct{}

func (*ListDirTool) Name() string { return "list_dir" }
func (*ListDirTool) Description() string {
	return "列出工作目录内目录内容，返回名称、类型、大小和权限。"
}
func (*ListDirTool) Category() Category { return CategoryRead }
func (*ListDirTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("目录路径，默认 .")})
}
func (*ListDirTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(raw, &a)
	if a.Path == "" {
		a.Path = "."
	}
	p, err := resolveInside(env.CWD, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	ents, err := os.ReadDir(p)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	var b strings.Builder
	limit := len(ents)
	if limit > 500 {
		limit = 500
	}
	for _, e := range ents[:limit] {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		typ := "file"
		if e.IsDir() {
			typ = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typ = "symlink"
		}
		fmt.Fprintf(&b, "%s\t%s\t%d\t%s\n", typ, e.Name(), info.Size(), info.Mode().Perm())
	}
	return Result{Content: strings.TrimSuffix(b.String(), "\n"), Metadata: map[string]any{"path": p, "total": len(ents), "truncated": len(ents) > 500}}, nil
}

func atomicWrite(ctx context.Context, path string, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return os.Rename(name, path)
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	m["additionalProperties"] = false
	return m
}
func strp(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func nump(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}
func intArg(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		if i != 0 {
			return i
		}
	}
	return def
}
