package latex

import (
	"strings"
	"testing"
)

// T66：提交必须勾全清单——少一个 id 就 REJECTED，不许带病过关。
func TestSubmitBlockFixRequiresAllChecked(t *testing.T) {
	t.Run("缺勾打回", func(t *testing.T) {
		submit := &SubmitBlockFixTool{Assigned: []string{"P1", "P2", "P3"}}
		res, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true}],"checked_problem_ids":["P1","P2"]}`)
		if err != nil {
			t.Fatal(err)
		}
		if submit.Submitted {
			t.Fatal("缺勾却提交成功了")
		}
		if !strings.Contains(res.Text, "P3") {
			t.Fatalf("回执没点名缺失的 P3: %q", res.Text)
		}
	})
	t.Run("全勾通过", func(t *testing.T) {
		submit := &SubmitBlockFixTool{Assigned: []string{"P1", "P2"}}
		res, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true,"note":"ok"}],"checked_problem_ids":["P2","P1"]}`)
		if err != nil {
			t.Fatal(err)
		}
		if !submit.Submitted || !strings.Contains(res.Text, "SUBMITTED") {
			t.Fatalf("全勾却被拒: %q", res.Text)
		}
	})
	t.Run("未配置清单时不受限", func(t *testing.T) {
		submit := &SubmitBlockFixTool{}
		if _, err := submit.Execute(`{"chapters":[{"chapter":"chapter_001","resolved":true}],"checked_problem_ids":[]}`); err != nil {
			t.Fatal(err)
		}
		if !submit.Submitted {
			t.Fatal("无清单场景不应被打回")
		}
	})
}
