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

	"github.com/LingNc/DocVision/skills/kernel/internal/config"
	"github.com/LingNc/DocVision/skills/kernel/internal/deliver"
	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/gen"
	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/prep"
	"github.com/LingNc/DocVision/skills/kernel/internal/project"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
	"github.com/LingNc/DocVision/skills/kernel/internal/viewer"
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

	case "gen-classify":
		// dvagent gen-classify <项目名> [images相对目录，默认 source/images]
		// 接线：把图目录铺成 classify 任务队列（幂等）。
		if len(args) < 2 {
			die(fmt.Errorf("gen-classify 需要: <项目名> [images目录]"))
		}
		p := mustProject(reg, args[1])
		images := "source/images"
		if len(args) > 2 {
			images = args[2]
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.GenClassify(q, p.Root, images)
		if err != nil {
			die(err)
		}
		out(rep)

	case "gen-convert":
		// dvagent gen-convert <项目名> [chapters相对目录，默认 chapters]
		if len(args) < 2 {
			die(fmt.Errorf("gen-convert 需要: <项目名> [chapters目录]"))
		}
		p := mustProject(reg, args[1])
		chapters := "chapters"
		if len(args) > 2 {
			chapters = args[2]
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.GenConvert(q, p.Root, chapters)
		if err != nil {
			die(err)
		}
		out(rep)

	case "gen-check":
		// dvagent gen-check <项目名> —— 为队列中每个 convert 任务铺一个 blocked 核对任务。
		if len(args) < 2 {
			die(fmt.Errorf("gen-check 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.GenCheck(q)
		if err != nil {
			die(err)
		}
		out(rep)

	case "gen-draw":
		// dvagent gen-draw <项目名> —— 读 classify 结论，为 vector 图铺 draw 任务。
		if len(args) < 2 {
			die(fmt.Errorf("gen-draw 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.GenDraw(q, p.Root)
		if err != nil {
			die(err)
		}
		out(rep)

	case "assemble-verify":
		// dvagent assemble-verify <项目名> —— 复核 done 任务产物自验收以来未被篡改。
		if len(args) < 2 {
			die(fmt.Errorf("assemble-verify 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		drifts := verify.CheckDrift(q.Tasks, p.Root)
		clean, lines := verify.DriftSummary(drifts)
		out(map[string]any{"clean": clean, "issues": lines, "detail": drifts})

	case "doc-search":
		// dvagent doc-search <项目名> <query> [max] —— 索引检索，回蒸馏命中（页号+bbox+截断）。
		if len(args) < 3 {
			die(fmt.Errorf("doc-search 需要: <项目名> <query> [max]"))
		}
		p := mustProject(reg, args[1])
		max := 5
		if len(args) > 3 {
			fmt.Sscanf(args[3], "%d", &max)
		}
		idx, err := docindex.Load(filepath.Join(p.Root, "doc_index.json"))
		if err != nil {
			die(fmt.Errorf("索引未建（先跑 index 构建）: %w", err))
		}
		hits := idx.Search(args[2], max)
		out(map[string]any{"query": args[2], "hits": hits, "total": len(hits)})

	case "page-render":
		// dvagent page-render <项目名> <global页> [x0,y0,x1,y1]
		// 渲染原书页到 work/renders/（可裁剪），回路径+物理尺寸。
		if len(args) < 3 {
			die(fmt.Errorf("page-render 需要: <项目名> <global页> [x0,y0,x1,y1]"))
		}
		p := mustProject(reg, args[1])
		pages, err := prep.LoadPageIndex(filepath.Join(p.Root, "doc_index_pages.json"))
		if err != nil {
			die(fmt.Errorf("页表未建: %w", err))
		}
		global := 0
		fmt.Sscanf(args[2], "%d", &global)
		var bbox *[4]float64
		if len(args) > 3 {
			var b [4]float64
			if n, _ := fmt.Sscanf(args[3], "%f,%f,%f,%f", &b[0], &b[1], &b[2], &b[3]); n == 4 {
				bbox = &b
			}
		}
		res, err := prep.RenderPage(pages, global, bbox, filepath.Join(p.Root, "work", "renders"), nil)
		if err != nil {
			die(err)
		}
		out(res)

	case "deliver":
		// dvagent deliver <项目名> [cls名] —— 装配交付：漂移闸门→归集→主文件→书签→standalone→盘点。
		if len(args) < 2 {
			die(fmt.Errorf("deliver 需要: <项目名> [cls名]"))
		}
		p := mustProject(reg, args[1])
		cls := ""
		if len(args) > 2 {
			cls = args[2]
		}
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := deliver.Deliver(q, p.Root, cls)
		if err != nil {
			die(err)
		}
		out(rep)

	case "mcp":
		// dvagent mcp —— MCP server 壳：同一组内核函数的协议皮（stdio JSON-RPC）。
		serveMCP(*ws)

	case "config-get":
		// dvagent config-get <项目名> —— 查看项目配置（用户命令；不进 MCP，agent 无权调用）。
		if len(args) < 2 {
			die(fmt.Errorf("config-get 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		cfg, err := config.Load(p.Root)
		if err != nil {
			die(err)
		}
		out(map[string]any{"path": config.Path(p.Root), "config": cfg})

	case "config-set":
		// dvagent config-set <项目名> <key> <value> —— 用户调阈值专用。
		// key 形如 figure.min_overlap / figure.accept_min_submits / split.pages_per_part。
		if len(args) < 4 {
			die(fmt.Errorf("config-set 需要: <项目名> <key> <value>"))
		}
		p := mustProject(reg, args[1])
		cfg, err := config.Load(p.Root)
		if err != nil {
			die(err)
		}
		var f float64
		fmt.Sscanf(args[3], "%g", &f)
		switch args[2] {
		case "figure.min_overlap":
			cfg.Figure.MinOverlap = f
		case "figure.max_scale_dev":
			cfg.Figure.MaxScaleDev = f
		case "figure.accept_min_submits":
			cfg.Figure.AcceptMinSubmits = int(f)
		case "split.pages_per_part":
			cfg.Split.PagesPerPart = int(f)
		case "convert.max_lines_per_chapter":
			cfg.Convert.MaxLinesPerChapter = int(f)
		default:
			die(fmt.Errorf("未知配置键: %s（可用：figure.min_overlap/figure.max_scale_dev/figure.accept_min_submits/split.pages_per_part/convert.max_lines_per_chapter）", args[2]))
		}
		if err := config.Save(p.Root, cfg); err != nil {
			die(err)
		}
		out(map[string]any{"updated": args[2], "value": args[3], "path": config.Path(p.Root)})

	case "ids-list":
		// dvagent ids-list <项目名> [img|vec|pdfp] —— 人类编号映射表（imgxxx/vecxxx/pdfpxx）。
		if len(args) < 2 {
			die(fmt.Errorf("ids-list 需要: <项目名> [kind]"))
		}
		p := mustProject(reg, args[1])
		tbl, err := ids.Load(p.Root)
		if err != nil {
			die(err)
		}
		if len(args) > 2 && args[2] != "" {
			out(map[string]any{args[2]: tbl.List(ids.Kind(args[2]))})
		} else {
			out(map[string]any{
				"img": tbl.List(ids.KindImg),
				"vec": tbl.List(ids.KindVec),
				"path": ids.PathOf(p.Root),
			})
		}

	case "ctx-up", "ctx-down":
		// dvagent ctx-up|ctx-down <项目名> <task_id> [n=3]
		// T6 img2text 小轮转工具：按任务的 entry_seq 取 doc_index 相邻文字块（上文/下文）。
		if len(args) < 3 {
			die(fmt.Errorf("ctx-up/ctx-down 需要: <项目名> <task_id> [n]"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		tk, err := q.Get(args[2])
		if err != nil {
			die(err)
		}
		seq, _ := tk.Accept["entry_seq"].(float64)
		n := 3
		if len(args) > 3 {
			fmt.Sscanf(args[3], "%d", &n)
		}
		idx, err := docindex.Load(filepath.Join(p.Root, "doc_index.json"))
		if err != nil {
			die(err)
		}
		up := args[0] == "ctx-up"
		var blocks []map[string]any
		count := 0
		for _, e := range idx.Entries {
			if e.Type != "text" || e.Text == "" {
				continue
			}
			before := e.Seq < int(seq)
			if (up && before) || (!up && !before) {
				blocks = append(blocks, map[string]any{
					"seq": e.Seq, "global": e.Global, "text": e.Text,
				})
				count++
				if count >= n {
					break
				}
			}
		}
		out(map[string]any{"task": tk.ID, "entry_seq": seq, "up": up, "blocks": blocks})

	case "viewer-build":
		// dvagent viewer-build <项目名> —— 生成静态看图台 work/viewer/index.html。
		if len(args) < 2 {
			die(fmt.Errorf("viewer-build 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := viewer.Build(p.Root, q)
		if err != nil {
			die(err)
		}
		out(rep)

	case "viewer-apply":
		// dvagent viewer-apply <项目名> <regroup.json路径> —— 消费重分组请求，写覆盖文件。
		if len(args) < 3 {
			die(fmt.Errorf("viewer-apply 需要: <项目名> <regroup.json路径>"))
		}
		p := mustProject(reg, args[1])
		raw, err := os.ReadFile(args[2])
		if err != nil {
			die(err)
		}
		var req viewer.RegroupRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			die(fmt.Errorf("regroup.json 解析失败: %w", err))
		}
		ov, err := viewer.ApplyRegroup(p.Root, req)
		if err != nil {
			die(err)
		}
		out(map[string]any{"threshold": ov.Threshold, "groups": len(ov.Groups), "override": viewer.OverridePath(p.Root)})

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
