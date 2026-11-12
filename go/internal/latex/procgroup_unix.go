//go:build !windows

package latex

import (
	"os/exec"
	"syscall"
)

// setProcessGroup 把命令放进自己的进程组（T326）：latexmk 会再拉起 sh -c
// xelatex，只有按组杀才能把整棵进程树收掉，否则孙子进程变孤儿继续空转。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 杀掉整组；拿不到 pgid 时退回只杀直接子进程。
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if pgid, err := syscall.Getpgid(cmd.Process.Pid); err == nil && pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}
	_ = cmd.Process.Kill()
}
