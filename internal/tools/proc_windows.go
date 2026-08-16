//go:build windows

package tools

import (
	"os"
	"os/exec"
	"strconv"
)

func shellCommand(command string) (string, []string) {
	return "cmd.exe", []string{"/D", "/S", "/C", command}
}
func prepareProcess(cmd *exec.Cmd) {}
func killProcessTree(p *os.Process) {
	if p == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(p.Pid)).Run()
	_ = p.Kill()
}
