package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	gitutil "github.com/ViudiraTech/Uinxed-Agent/internal/git"
)

// The git tools shell out to the git binary in the session's working directory.
// They never pass user input to a shell: every argument goes through
// exec.Command's argv list, and commit paths are add'ed one by one.

func gitDir(env ExecutionContext) (string, error) {
	if strings.TrimSpace(env.CWD) == "" {
		return "", errors.New("working directory is empty")
	}
	return env.CWD, nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", errors.New("git is not installed")
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return string(out), nil
}

type GitStatusTool struct{}

func (*GitStatusTool) Name() string { return "git_status" }
func (*GitStatusTool) Description() string {
	return "查看 git 工作区状态（porcelain 格式，含未跟踪文件）。"
}
func (*GitStatusTool) Category() Category { return CategoryRead }
func (*GitStatusTool) Schema() map[string]any {
	return obj(map[string]any{})
}

func (*GitStatusTool) Execute(ctx context.Context, _ json.RawMessage, env ExecutionContext) (Result, error) {
	dir, err := gitDir(env)
	if err != nil {
		return Result{}, err
	}
	out, err := runGit(ctx, dir, "status", "--porcelain=v1", "-b", "--untracked-files=all")
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(out) == "" {
		out = "clean"
	}
	return Result{Content: strings.TrimSuffix(out, "\n")}, nil
}

type GitDiffTool struct{}

func (*GitDiffTool) Name() string { return "git_diff" }
func (*GitDiffTool) Description() string {
	return "查看 git 差异；path 可选，默认整个工作区（含未跟踪文件）。"
}
func (*GitDiffTool) Category() Category { return CategoryRead }
func (*GitDiffTool) Schema() map[string]any {
	return obj(map[string]any{"path": strp("可选，限定单个文件或目录")})
}

func (*GitDiffTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(raw, &a)
	dir, err := gitDir(env)
	if err != nil {
		return Result{}, err
	}
	// Reuse the controller's diff reader: it already folds untracked files in
	// and produces the unified text the UI renders.
	snap, err := gitutil.Diff(ctx, dir)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(a.Path) == "" {
		if strings.TrimSpace(snap.Unified) == "" {
			return Result{Content: "no changes"}, nil
		}
		return Result{Content: snap.Unified, Metadata: map[string]any{"files": len(snap.Files)}}, nil
	}
	p, err := resolveInside(dir, a.Path, false)
	if err != nil {
		return Result{}, err
	}
	d, err := gitutil.FileDiff(ctx, dir, p)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(d) == "" {
		return Result{Content: "no changes for " + a.Path}, nil
	}
	return Result{Content: d}, nil
}

type GitLogTool struct{}

func (*GitLogTool) Name() string { return "git_log" }
func (*GitLogTool) Description() string {
	return "查看最近提交记录；count 可选，默认 10，最大 50。"
}
func (*GitLogTool) Category() Category { return CategoryRead }
func (*GitLogTool) Schema() map[string]any {
	return obj(map[string]any{"count": nump("返回条数")})
}

func (*GitLogTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Count int `json:"count"`
	}
	_ = decode(raw, &a)
	if a.Count <= 0 {
		a.Count = 10
	}
	if a.Count > 50 {
		a.Count = 50
	}
	dir, err := gitDir(env)
	if err != nil {
		return Result{}, err
	}
	out, err := runGit(ctx, dir, "log", "--oneline", fmt.Sprintf("-%d", a.Count))
	if err != nil {
		return Result{}, err
	}
	return Result{Content: strings.TrimSuffix(out, "\n")}, nil
}

type GitCommitTool struct{}

func (*GitCommitTool) Name() string { return "git_commit" }
func (*GitCommitTool) Description() string {
	return "git add 指定路径并提交；只 add 列出的路径，不 amend，不 push。message 必填。"
}
func (*GitCommitTool) Category() Category { return CategoryWrite }
func (*GitCommitTool) Schema() map[string]any {
	return obj(map[string]any{
		"message": strp("提交信息"),
		"paths":   arr(map[string]any{"path": strp("要提交的文件或目录")}, "要 add 的路径列表", "path"),
	}, "message", "paths")
}

type commitPath struct {
	Path string `json:"path"`
}

func (*GitCommitTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Message string       `json:"message"`
		Paths   []commitPath `json:"paths"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(a.Message) == "" {
		return Result{}, errors.New("message 不能为空")
	}
	if len(a.Paths) == 0 {
		return Result{}, errors.New("paths 不能为空：只 add 明确列出的路径")
	}
	dir, err := gitDir(env)
	if err != nil {
		return Result{}, err
	}
	for _, p := range a.Paths {
		if strings.TrimSpace(p.Path) == "" {
			return Result{}, errors.New("empty path in paths")
		}
		// Bound-check every path before handing it to git add.
		resolved, err := resolveInside(dir, p.Path, false)
		if err != nil {
			return Result{}, err
		}
		if _, err := runGit(ctx, dir, "add", "--", resolved); err != nil {
			return Result{}, err
		}
	}
	out, err := runGit(ctx, dir, "commit", "-m", a.Message)
	if err != nil {
		return Result{}, err
	}
	summary, _ := runGit(ctx, dir, "log", "-1", "--oneline")
	return Result{Content: strings.TrimSpace(summary), Metadata: map[string]any{"output": strings.TrimSpace(out)}}, nil
}
