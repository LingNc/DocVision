// dvagent 是 skills 内核的 agent 操作面（CLI 优先；MCP 壳后续包同一组函数）。
//
// 用法：dvagent -workspace <根目录> <command> [args]
//
// 子命令：status / register / activate / claim / submit / note / profile-update
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/project"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
)

func die(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}

func out(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func main() {
	ws := flag.String("workspace", ".", "skills 工作区根目录（含 skills_registry.json）")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法: dvagent -workspace <根> <status|register|activate|claim|submit|note|profile-update> ...")
		os.Exit(2)
	}

	reg, err := project.Load(project.RegistryPath(*ws))
	if err != nil {
		die(err)
	}

	switch args[0] {
	case "status":
		// 冷启动仪式的唯一入口：注册表 + 每本书的队列蒸馏。
		digest := map[string]any{"projects": reg.Digest()}
		for name, p := range reg.Projects {
			if p.Lifecycle == project.Archived {
				continue
			}
			q, err := queue.Load(queue.ProjectQueuePath(p.Root))
			if err == nil {
				digest[name] = q.Digest()
			}
		}
		out(digest)

	case "register":
		// dvagent register <名字> <项目根>
		if len(args) < 3 {
			die(fmt.Errorf("register 需要: <名字> <项目根>"))
		}
		p, err := reg.Register(args[1], args[2])
		if err != nil {
			die(err)
		}
		out(p)

	case "activate":
		if len(args) < 2 {
			die(fmt.Errorf("activate 需要: <名字>"))
		}
		p, err := reg.Activate(args[1])
		if err != nil {
			die(err)
		}
		out(p)

	case "claim":
		// dvagent claim <项目名> <owner> [n] [类型,类型...]
		if len(args) < 3 {
			die(fmt.Errorf("claim 需要: <项目名> <owner> [n] [types]"))
		}
		p := mustProject(reg, args[1])
		if p.Lifecycle != project.Active {
			die(fmt.Errorf("项目 %s 处于 %s，不可派任务（先 activate）", args[1], p.Lifecycle))
		}
		n := 1
		if len(args) > 3 {
			fmt.Sscanf(args[3], "%d", &n)
		}
		var types []task.Type
		if len(args) > 4 && args[4] != "" {
			for _, s := range strings.Split(args[4], ",") {
				types = append(types, task.Type(s))
			}
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		claimed := q.Claim(args[2], n, 1800, types...)
		// 沙箱不存在的补建（inputs/out/scratch 骨架）。
		for _, t := range claimed {
			for _, d := range []string{"inputs", "scratch", "out"} {
				_ = os.MkdirAll(filepath.Join(p.Root, t.Sandbox, d), 0o755)
			}
		}
		out(claimed)

	case "submit":
		// dvagent submit <项目名> <task_id,task_id,...>
		// 逐条跑验收器，落 verdict，自动 unblock。
		if len(args) < 3 {
			die(fmt.Errorf("submit 需要: <项目名> <task_id,...>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		ck := verify.NewChecker(p.Root)
		results := map[string]any{}
		for _, id := range strings.Split(args[2], ",") {
			t, err := q.Get(id)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			errs, notes := ck.Run(t)
			v := task.Verdict{At: time.Now(), Pass: len(errs) == 0, Errors: errs, Notes: notes}
			nt, err := q.RecordVerdict(id, v, 6)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			// 只有验收通过才解除依赖：打回的任务不解锁核对/父任务。
			unlocked := 0
			if nt.Status == task.StatusDone {
				unlocked = len(q.Unblock(id))
			}
			results[id] = map[string]any{"status": nt.Status, "errors": errs, "notes": notes, "unlocked": unlocked}
		}
		out(results)

	case "note":
		// dvagent note <项目名> <task_id> <文本> —— 追加任务笔记（append-only）。
		if len(args) < 4 {
			die(fmt.Errorf("note 需要: <项目名> <task_id> <文本>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		t, err := q.Get(args[2])
		if err != nil {
			die(err)
		}
		f, err := os.OpenFile(filepath.Join(p.Root, t.Sandbox, "notes.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			die(err)
		}
		defer f.Close()
		fmt.Fprintf(f, "\n## %s\n\n%s\n", time.Now().Format("01-02 15:04"), strings.Join(args[3:], " "))
		out(map[string]any{"noted": args[2]})

	case "profile-update":
		// dvagent profile-update <项目名> <文本>
		if len(args) < 3 {
			die(fmt.Errorf("profile-update 需要: <项目名> <文本>"))
		}
		p := mustProject(reg, args[1])
		if err := p.ProfileAppend(strings.Join(args[2:], " ")); err != nil {
			die(err)
		}
		out(map[string]any{"profile_updated": args[1]})

	case "split":
		// dvagent split <项目名> <parent_task_id> <segments.json>
		if len(args) < 4 {
			die(fmt.Errorf("split 需要: <项目名> <parent_task_id> <segments.json>"))
		}
		p := mustProject(reg, args[1])
		b, err := os.ReadFile(args[3])
		if err != nil {
			die(err)
		}
		var segs []queue.Segment
		if err := json.Unmarshal(b, &segs); err != nil {
			die(fmt.Errorf("segments.json 解析失败: %w", err))
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		if _, err := q.Get(args[2]); err != nil {
			die(err)
		}
		if errs := queue.ValidateSplit(segs, splitChapterLines(p.Root, args[2])); len(errs) > 0 {
			out(map[string]any{"split": "REJECTED", "errors": errs})
			break
		}
		children, err := q.FanOutSplit(args[2], segs)
		if err != nil {
			die(err)
		}
		out(map[string]any{"split": "OK", "children": len(children)})

	case "issue-add":
		// dvagent issue-add <项目名> <task_id> <描述> —— 落盘问题并生成回炉任务。
		if len(args) < 4 {
			die(fmt.Errorf("issue-add 需要: <项目名> <task_id> <描述>"))
		}
		p := mustProject(reg, args[1])
		store, err := queue.LoadIssues(queue.IssueStorePath(p.Root))
		if err != nil {
			die(err)
		}
		iss := &queue.Issue{
			ID:      fmt.Sprintf("i%06d", len(store.Issues)+1),
			TaskID:  args[2],
			Project: args[1],
			Desc:    strings.Join(args[3:], " "),
			Status:  "open",
		}
		if err := store.Add(iss); err != nil {
			die(err)
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rw, err := q.SpawnRework(iss)
		if err != nil {
			die(err)
		}
		if err := store.Save(); err != nil {
			die(err)
		}
		out(map[string]any{"issue": iss.ID, "rework_task": rw.ID})

	case "issue-resolve":
		if len(args) < 3 {
			die(fmt.Errorf("issue-resolve 需要: <项目名> <issue_id>"))
		}
		p := mustProject(reg, args[1])
		store, err := queue.LoadIssues(queue.IssueStorePath(p.Root))
		if err != nil {
			die(err)
		}
		if err := store.Resolve(args[2]); err != nil {
			die(err)
		}
		out(map[string]any{"resolved": args[2]})

	case "pause":
		// dvagent pause <项目名> —— 停派新任务（已领任务边界完成）。
		if len(args) < 2 {
			die(fmt.Errorf("pause 需要: <项目名>"))
		}
		p, err := reg.Pause(args[1])
		if err != nil {
			die(err)
		}
		out(map[string]any{"paused": p.Name})

	case "resume":
		// dvagent resume <项目名> —— 同 activate：置 active 并把其余 active 的 paused。
		if len(args) < 2 {
			die(fmt.Errorf("resume 需要: <项目名>"))
		}
		p, err := reg.Activate(args[1])
		if err != nil {
			die(err)
		}
		out(map[string]any{"resumed": p.Name})

	case "abort":
		// dvagent abort <项目名> —— 封存（终止）：队列冻结只读，保留制品与档案。
		if len(args) < 2 {
			die(fmt.Errorf("abort 需要: <项目名>"))
		}
		p, ok := reg.Projects[args[1]]
		if !ok {
			die(fmt.Errorf("项目未登记: %s", args[1]))
		}
		p.Lifecycle = project.Archived
		if err := reg.Save(); err != nil {
			die(err)
		}
		out(map[string]any{"archived": p.Name})

	default:
		die(fmt.Errorf("未知子命令: %s", args[0]))
	}
}

// splitChapterLines 取拆分父任务主输入的行数（缺输入时退化为只查结构）。
func splitChapterLines(projectRoot, taskID string) int {
	q, err := queue.Load(queue.ProjectQueuePath(projectRoot))
	if err != nil {
		return 0
	}
	t, err := q.Get(taskID)
	if err != nil || len(t.Inputs) == 0 {
		return 0
	}
	b, err := os.ReadFile(filepath.Join(projectRoot, t.Sandbox, t.Inputs[0]))
	if err != nil {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++ // 末尾无换行时补最后一行
	}
	return n
}

func mustProject(reg *project.Registry, name string) *project.Project {
	p, ok := reg.Projects[name]
	if !ok {
		die(fmt.Errorf("项目未登记: %s（先 register）", name))
	}
	return p
}
