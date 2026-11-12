//go:build !windows

package latex

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// T326：超时必须**真的**收得回来。真实事故：latexmk 到点被杀，它拉起的
// `sh -c xelatex` 变孤儿继续 100% CPU 空转；而 CombinedOutput 还握着那个被
// 孙子进程继承的管道，于是工具调用又陪着挂了 45 分钟。
func TestRunCmdTimeoutKillsGroup(t *testing.T) {
	// 孙子进程自己也 sleep：管道会被它继承（旧实现就是卡在这里）。
	before := sleepPids(t)
	cmd := exec.Command("sh", "-c", "sleep 30 & sleep 30")
	start := time.Now()
	_, err, timedOut := runCmdTimeout(cmd, 300*time.Millisecond)
	elapsed := time.Since(start)
	if !timedOut {
		t.Fatalf("应当判成超时（err=%v）", err)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("超时收尾太慢：%v", elapsed)
	}
	// 整组应当被杀干净：调用之后不该多出任何 sleep 进程（按进程名精确匹配，
	// 不能用 -f，否则 pgrep 会匹配到它自己那条命令行）。
	time.Sleep(200 * time.Millisecond)
	for pid := range sleepPids(t) {
		if !before[pid] {
			t.Fatalf("还有孤儿 sleep 进程活着：pid %d", pid)
		}
	}
}

// sleepPids 返回当前名字精确为 sleep 的进程。
func sleepPids(t *testing.T) map[int]bool {
	t.Helper()
	out, _ := exec.Command("pgrep", "-x", "sleep").Output()
	got := map[int]bool{}
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			got[pid] = true
		}
	}
	return got
}

// 正常命令照旧拿到输出、不判超时。
func TestRunCmdTimeoutNormal(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo hello")
	out, err, timedOut := runCmdTimeout(cmd, 5*time.Second)
	if err != nil || timedOut {
		t.Fatalf("正常命令不该失败：err=%v timedOut=%v", err, timedOut)
	}
	if out != "hello\n" {
		t.Fatalf("输出不对：%q", out)
	}
}
