package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type BashTool struct{}

func (*BashTool) Name() string { return "bash" }
func (*BashTool) Description() string {
	return "在工作目录执行 shell 命令；stdout/stderr 流式返回，支持超时和取消，取消时清理子进程树。"
}
func (*BashTool) Category() Category { return CategoryShell }
func (*BashTool) Schema() map[string]any {
	return obj(map[string]any{"cmd": strp("要执行的 shell 命令"), "timeout": nump("超时秒数，默认 30，最大 1800")}, "cmd")
}
func (*BashTool) Execute(ctx context.Context, raw json.RawMessage, env ExecutionContext) (Result, error) {
	var a struct {
		Cmd     string `json:"cmd"`
		Timeout int    `json:"timeout"`
	}
	if err := decode(raw, &a); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(a.Cmd) == "" {
		return Result{}, errors.New("cmd 不能为空")
	}
	if a.Timeout <= 0 {
		a.Timeout = 30
	}
	if a.Timeout > 1800 {
		a.Timeout = 1800
	}
	if a.Timeout < 1 {
		a.Timeout = 1
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout)*time.Second)
	defer cancel()
	name, args := shellCommand(a.Cmd)
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = env.CWD
	prepareProcess(cmd)
	var out, er cappedWriter
	out.limit = 256 << 10
	er.limit = 128 << 10
	out.callback = func(s string) {
		if env.OnOutput != nil {
			env.OnOutput("stdout", s)
		}
	}
	er.callback = func(s string) {
		if env.OnOutput != nil {
			env.OnOutput("stderr", s)
		}
	}
	cmd.Stdout = &out
	cmd.Stderr = &er
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-cctx.Done():
			killProcessTree(cmd.Process)
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	close(done)
	code := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			code = ee.ExitCode()
		} else if cctx.Err() != nil {
			code = 124
		} else {
			return Result{}, waitErr
		}
	}
	res := Result{Content: out.String(), ExitCode: &code, Metadata: map[string]any{"stderr": er.String(), "timed_out": errors.Is(cctx.Err(), context.DeadlineExceeded)}}
	if cctx.Err() != nil && !errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return res, cctx.Err()
	}
	return res, nil
}

type cappedWriter struct {
	mu        sync.Mutex
	b         strings.Builder
	limit     int
	truncated bool
	callback  func(string)
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if w.callback != nil {
		w.callback(string(p))
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	remain := w.limit - w.b.Len()
	if remain > 0 {
		if len(p) > remain {
			w.b.Write(p[:remain])
			w.truncated = true
		} else {
			w.b.Write(p)
		}
	} else {
		w.truncated = true
	}
	return n, nil
}
func (w *cappedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.b.String()
	if w.truncated {
		s += "\n…[truncated]"
	}
	return s
}
func parseTimeout(s string) int { v, _ := strconv.Atoi(s); return v }
