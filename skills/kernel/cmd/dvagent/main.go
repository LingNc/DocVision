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
		ck := &verify.Checker{ProjectRoot: p.Root}
		results := map[string]any{}
		for _, id := range strings.Split(args[2], ",") {
			t, err := q.Get(id)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			errs := ck.Run(t)
			v := task.Verdict{At: time.Now(), Pass: len(errs) == 0, Errors: errs}
			nt, err := q.RecordVerdict(id, v, 6)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			unlocked := q.Unblock(id)
			results[id] = map[string]any{"status": nt.Status, "errors": errs, "unblocked": len(unlocked)}
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

	default:
		die(fmt.Errorf("未知子命令: %s", args[0]))
	}
}

func mustProject(reg *project.Registry, name string) *project.Project {
	p, ok := reg.Projects[name]
	if !ok {
		die(fmt.Errorf("项目未登记: %s（先 register）", name))
	}
	return p
}
