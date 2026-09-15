package analyze

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// P12 的两行标记（img2text 文本日志）：
//
//	"[mermaid-fix] 就地修复轮用尽，启动升级修复会话" —— 升级会话启动一次
//	"[mermaid-fix] 编译错误累计 N/M，进入备选模型" —— 触发一次备选模型接管
const (
	mermaidFixStartMarker    = "[mermaid-fix] 就地修复轮用尽，启动升级修复会话"
	mermaidFixEscalateMarker = "[mermaid-fix] 编译错误累计"
	mermaidFixEscalateNote   = "，进入备选模型"
)

// CountMermaidFixEscalations 扫描日志文件，返回升级会话启动数与触发备选模型的
// 次数。只做子串计数（标记是 Go 侧 logger 拼出的固定前缀），解析失败按 0 处理
//——统计是锦上添花，不能因为读不了日志让 analyze 挂掉。
func CountMermaidFixEscalations(paths []string) (sessions, escalations int) {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			if containsSub(line, mermaidFixStartMarker) {
				sessions++
			}
			if containsSub(line, mermaidFixEscalateMarker) && containsSub(line, mermaidFixEscalateNote) {
				escalations++
			}
		}
		_ = f.Close()
	}
	return sessions, escalations
}

// PrintMermaidFixNote 在 img2text 进度摘要之后给出升级会话的触发统计（无触发
// 不打印——绝大多数运行不会走到升级会话）。
func PrintMermaidFixNote(paths []string) {
	sessions, escalations := CountMermaidFixEscalations(paths)
	if sessions > 0 {
		fmt.Printf("  mermaid 升级修复会话: 启动 %d 次", sessions)
		if escalations > 0 {
			fmt.Printf("，其中 %d 次触发备选模型", escalations)
		}
		fmt.Println()
	}
}

func containsSub(s, sub string) bool { return strings.Contains(s, sub) }
