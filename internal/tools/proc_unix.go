//go:build !windows

package tools

import (
	"os"
	"os/exec"
	"syscall"
)

func shellCommand(command string) (string, []string) { return "/bin/bash", []string{"-lc", command} }
func prepareProcess(cmd *exec.Cmd)                   { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func killProcessTree(p *os.Process) {
	if p == nil {
		return
	}
	_ = syscall.Kill(-p.Pid, syscall.SIGTERM)
	_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
}
