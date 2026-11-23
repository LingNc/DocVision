//go:build windows

package luadraw

import "os/exec"

// setProcessGroup 在 Windows 上不做进程组（T326 的 Unix 实现见
// procgroup_unix.go）；超时仍由 runCmdTimeout 杀直接子进程兜住。
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
