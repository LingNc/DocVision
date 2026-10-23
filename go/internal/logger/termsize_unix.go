//go:build !windows

package logger

import (
	"os"
	"syscall"
	"unsafe"
)

// terminalRows returns the visible height of the terminal attached to
// stdout, or 0 when it cannot be queried (not a terminal, bad fd, platform
// without TIOCGWINSZ). The live block uses it to stay within one screen:
// 块比屏幕高时，光标上移会被终端夹在顶部，擦除就变成把上面的历史一起
// 滚掉（T226）。
func terminalRows() int {
	var ws struct{ row, col, xpixel, ypixel uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0
	}
	return int(ws.row)
}
