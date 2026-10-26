package latex

// T242 现场（用户报的截图）：一整章转换跑完、submit 了 470 个文件、核对也跑完了，
// 可核对的 submit 里 `status: issues`、`issues` 是空串。代码给这份留档写了一句占位
// 文案（the checker reported problems but gave no description），随后按"打回"处理：
// 作废整章、从头重转一条全新的转换会话。于是一棵跑好的产物没了，而拿回来的只有
// 一句没有任何内容的话——用户看到的就是"我明明切回了已经跑完的那一次，它又跳去
// 新的转换了"。
//
// 判据：issues 为空 = 没有信息，不是判决。先在同一条会话里要一份清单（工具没坏，
// 参数是空的，一句话能救回来）；要不到就按"这次没结论"换核对会话重核，产物与
// 转换会话都不动——与 T222/T233 同一条路。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckerEmptyIssuesAsksForListInsteadOfReconverting(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{checkerEmptyIssues: 1})
	if run.err != nil {
		t.Fatalf("清单补上之后应正常收尾，实际 %v", run.err)
	}
	// 同一条会话里问了一次，就拿到了清单/结论：不该为此另开核对会话。
	trs, _ := filepath.Glob(filepath.Join(run.proj, "work", "sessions", "checker_chapter_01*.jsonl"))
	if len(trs) != 1 {
		t.Errorf("空清单应在同一条会话里补，实际核对了 %d 条会话（%v）", len(trs), trs)
	}
	asked := 0
	for _, text := range run.stats.userTexts {
		if strings.HasPrefix(strings.TrimSpace(text), checkerIssuesReminderMark) {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("应向会话要一次问题清单，实际发了 %d 次", asked)
	}
	if run.stats.convert != 1 {
		t.Errorf("空清单不是内容问题，不该重转整章，实际转换请求 %d 次", run.stats.convert)
	}
	if entries, err := os.ReadDir(filepath.Join(run.proj, "work", "attempts")); err == nil && len(entries) > 0 {
		t.Errorf("空清单不该作废产物，实际 work/attempts/ 有 %d 项", len(entries))
	}
}

// 另一半：清单要不到时也不能把它当判决用。按"核对没给结论"收尾——产物留在原处、
// 只重开核对会话，绝不重转整章。
func TestCheckerEmptyIssuesNeverReconverts(t *testing.T) {
	run := runRescueChapter(t, rescueChapterOptions{checkerEmptyIssues: 99})
	if run.err == nil {
		t.Fatal("核对始终没给出可用结论，应如实上报失败")
	}
	if !strings.Contains(run.err.Error(), "没有给出结论") {
		t.Errorf("失败原因应说明核对没给结论，实际：%v", run.err)
	}
	if run.stats.convert != 1 {
		t.Errorf("空清单不该重转整章，实际转换请求 %d 次", run.stats.convert)
	}
	texPath := filepath.Join(run.proj, "work", "chapters", "chapter_01.tex")
	if _, err := os.Stat(texPath); err != nil {
		t.Fatalf("产物应保留在原处，实际 stat err = %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(run.proj, "work", "attempts")); err == nil && len(entries) > 0 {
		t.Errorf("空清单不该归档产物，实际 work/attempts/ 有 %d 项", len(entries))
	}
	// 标记成"没结论"，重跑时才会先重核这一份，而不是静默采纳或直接作废。
	marker, err := os.ReadFile(texPath + ".checker")
	if err != nil {
		t.Fatalf("应在 .checker 里留下结论，实际 stat err = %v", err)
	}
	if !checkerNoVerdict(string(marker)) {
		t.Errorf(".checker 里应是「没结论」那类文案，实际：%q", string(marker))
	}
	// 复盘那一轮不该把"核对没给结论"当成内容问题喂回转换会话。
	for i, text := range run.stats.userTexts {
		if strings.Contains(text, "The checker reviewed your submitted chapter") {
			t.Errorf("核对没给结论，转换会话却收到了第 %d 条反馈：\n%s", i+1, text)
		}
	}
}
