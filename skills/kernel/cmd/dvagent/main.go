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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/config"
	"github.com/LingNc/DocVision/skills/kernel/internal/deliver"
	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/envcheck"
	"github.com/LingNc/DocVision/skills/kernel/internal/gen"
	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/mdimg"
	"github.com/LingNc/DocVision/skills/kernel/internal/mineruapi"
	"github.com/LingNc/DocVision/skills/kernel/internal/prep"
	"github.com/LingNc/DocVision/skills/kernel/internal/project"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/troubleshoot"
	"github.com/LingNc/DocVision/skills/kernel/internal/upgrade"
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


// skillRoot 从可执行文件位置解析技能根（kernel/bin/dvagent → 技能根 ../../）。
func skillRoot() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "..", "..")
	}
	return "."
}

// hasContentList part 根是否已有 content_list（prep-mineru-api 断点跳过判据；
// 归一后产物只落 part 根，不看 auto 层）。
func hasContentList(dir string) bool {
	matches, _ := filepath.Glob(filepath.Join(dir, "*_content_list.json"))
	return len(matches) > 0
}

// imgRefRe 匹配 md 图片行 ![cap](path)，取路径供 chapterImageSet 提取图像 sha。
var imgRefRe = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)

// chapterImageSet 从章 md 提取插图 sha 集合（gen-draw 按章按需铺的过滤依据；
// 匹配 md 图片行里的图像路径 basename 去扩展名）。
func chapterImageSet(mdPath string) (map[string]bool, error) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		return nil, fmt.Errorf("按章按需铺需要章 md（%s）：%w", mdPath, err)
	}
	set := map[string]bool{}
	for _, m := range imgRefRe.FindAllStringSubmatch(string(data), -1) {
		stem := strings.TrimSuffix(filepath.Base(m[1]), filepath.Ext(m[1]))
		if stem != "" {
			set[stem] = true
		}
	}
	return set, nil
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

	case "claim-wait":
		// dvagent claim-wait <项目名> <owner> [--interval N] [--timeout N] [--types t1,t2]
		// 阻塞等待内核分发（多子会话模式语义，docs/08-figure-flow.md §3）：无任务可领时
		// 轮询睡眠，直到领到任务或超时（timeout=0 不限）。会话阻塞不占 CPU，可被用户随时停掉。
		if len(args) < 3 {
			die(fmt.Errorf("claim-wait 需要: <项目名> <owner> [--interval N] [--timeout N] [--types t1,t2]"))
		}
		p := mustProject(reg, args[1])
		if p.Lifecycle != project.Active {
			die(fmt.Errorf("项目 %s 处于 %s，不可派任务（先 activate）", args[1], p.Lifecycle))
		}
		interval, timeout := 5, 0
		var wtypes []task.Type
		for i := 3; i < len(args); i++ {
			switch args[i] {
			case "--interval":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &interval)
				}
			case "--timeout":
				i++
				if i < len(args) {
					fmt.Sscanf(args[i], "%d", &timeout)
				}
			case "--types":
				i++
				if i < len(args) {
					for _, s := range strings.Split(args[i], ",") {
						wtypes = append(wtypes, task.Type(s))
					}
				}
			default:
				die(fmt.Errorf("未知参数: %s（可用 --interval/--timeout/--types）", args[i]))
			}
		}
		if interval < 1 {
			interval = 5
		}
		var deadline time.Time
		if timeout > 0 {
			deadline = time.Now().Add(time.Duration(timeout) * time.Second)
		}
		for {
			q, err := queue.Load(queue.ProjectQueuePath(p.Root))
			if err != nil {
				die(err)
			}
			claimed := q.Claim(args[2], 1, 1800, wtypes...)
			if len(claimed) > 0 {
				for _, t := range claimed {
					for _, d := range []string{"inputs", "scratch", "out"} {
						_ = os.MkdirAll(filepath.Join(p.Root, t.Sandbox, d), 0o755)
					}
				}
				out(claimed)
				return
			}
			if !deadline.IsZero() && time.Now().After(deadline) {
				out(map[string]any{"claimed": 0, "timeout": true})
				return
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}

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
		ck := verify.NewChecker(p.Root).WithSkillRoot(skillRoot())
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
		// dvagent gen-draw <项目名> [章md名] —— 读 classify 结论，为 vector 图铺 draw 任务。
		// 带章参数=按章按需铺（只铺该章插图里的 vector 图，convert 阶段需要时再铺）；
		// 不带=全书（默认，行为同旧）。见 docs/08-figure-flow.md。
		if len(args) < 2 {
			die(fmt.Errorf("gen-draw 需要: <项目名> [章md名]"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		var allowed map[string]bool
		if len(args) >= 3 {
			allowed, err = chapterImageSet(filepath.Join(p.Root, "chapters", args[2]))
			if err != nil {
				die(err)
			}
		}
		rep, err := gen.GenDraw(q, p.Root, allowed)
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
		cfg, err := config.LoadLayered(skillRoot(), p.Root)
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
		cfg, err := config.LoadLayered(skillRoot(), p.Root)
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
		case "style.font_family":
			cfg.Style.FontFamily = args[3]
		case "style.fallback_font":
			cfg.Style.FallbackFont = args[3]
		case "mineru.api_base_url":
			cfg.MinerU.APIBaseURL = args[3]
		case "mineru.model_version":
			cfg.MinerU.ModelVersion = args[3]
		case "mineru.language":
			cfg.MinerU.Language = args[3]
		case "mineru.poll_interval":
			cfg.MinerU.PollInterval = int(f)
		case "mineru.poll_timeout":
			cfg.MinerU.PollTimeout = int(f)
		case "mineru.upload_timeout":
			cfg.MinerU.UploadTimeout = int(f)
		default:
			die(fmt.Errorf("未知配置键: %s（可用：figure.min_overlap/figure.max_scale_dev/figure.accept_min_submits/split.pages_per_part/convert.max_lines_per_chapter/style.font_family/style.fallback_font/mineru.api_base_url/mineru.model_version/mineru.language/mineru.poll_interval/mineru.poll_timeout/mineru.upload_timeout）", args[2]))
		}
		if err := config.Save(p.Root, cfg); err != nil {
			die(err)
		}
		out(map[string]any{"updated": args[2], "value": args[3], "path": config.Path(p.Root)})

	case "troubleshoot":
		// dvagent troubleshoot <start|collect|draft|validate|submit|relay-submit> [工作区根=.] [--url U] [--author A]（docs/07 状态机）
		if len(args) < 2 {
			die(fmt.Errorf("troubleshoot 需要: <start|collect|draft|validate|submit|relay-submit> [工作区根] [--url] [--author]"))
		}
		troot := *ws
		rest := args[2:]
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "--") {
			troot = rest[0]
			rest = rest[1:]
		}
		var flagURL, flagAuthor string
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case "--url":
				if i+1 >= len(rest) {
					die(fmt.Errorf("--url 需要值"))
				}
				flagURL = rest[i+1]
				i++
			case "--author":
				if i+1 >= len(rest) {
					die(fmt.Errorf("--author 需要值"))
				}
				flagAuthor = rest[i+1]
				i++
			default:
				die(fmt.Errorf("未知参数: %s（可用 --url/--author）", rest[i]))
			}
		}
		switch args[1] {
		case "start":
			st, err := troubleshoot.Start(troot)
			if err != nil {
				die(err)
			}
			out(map[string]any{"state": st, "dir": troubleshoot.Dir(troot), "next": "troubleshoot collect <根>"})
		case "collect":
			doctorJSON, _ := json.Marshal(envcheck.Check("", nil))
			st, err := troubleshoot.Collect(troot, string(doctorJSON))
			if err != nil {
				die(err)
			}
			out(map[string]any{"state": st, "next": "填写 work/troubleshoot/report.md 各节 → troubleshoot draft <根>"})
		case "draft":
			st, err := troubleshoot.Draft(troot)
			if err != nil {
				die(err)
			}
			out(map[string]any{"state": st, "next": "troubleshoot validate <根>"})
		case "validate":
			errs, st, err := troubleshoot.Validate(troot)
			if err != nil {
				die(err)
			}
			next := "按 errors 修正后重跑 validate"
			if len(errs) == 0 {
				next = "troubleshoot submit <根>（生成 issue.md）"
			}
			out(map[string]any{"state": st, "pass": len(errs) == 0, "errors": errs, "next": next})
		case "submit":
			path, final, err := troubleshoot.Submit(troot)
			if err != nil {
				die(err)
			}
			out(map[string]any{"state": "submitted", "issue": path, "chars": len(final),
				"next": "GitHub 开 issue 粘贴该文件（授权流程见 docs/07-troubleshoot.md）；用户拒绝授权→ troubleshoot relay-submit <根> 走匿名兜底"})
		case "relay-submit":
			// 匿名兜底：经用户部署的 relay 服务器代发 issue（服务器端 token，docs/07 兜底段）
			if st := troubleshoot.Status(troot); st != troubleshoot.StateSubmitted {
				die(fmt.Errorf("relay-submit 需先走完 submit（当前状态 %s）：start→collect→draft→validate→submit", st))
			}
			relayURL := flagURL
			if relayURL == "" {
				if cfg, err := config.LoadLayered(skillRoot(), troot); err == nil {
					relayURL = cfg.Upstream.RelayURL
				}
			}
			if relayURL == "" {
				die(fmt.Errorf("relay 地址未配置：--url 传入，或在技能 config.yaml 写 upstream.relay_url（见 docs/06/07）"))
			}
			author := flagAuthor
			if author == "" {
				author = os.Getenv("DOCVISION_AGENT_NAME")
			}
			if author == "" {
				author = "anonymous-agent"
			}
			issueURL, err := troubleshoot.SubmitViaRelay(troot, relayURL, author)
			if err != nil {
				die(err)
			}
			out(map[string]any{"state": "submitted", "via": "relay", "issue_url": issueURL})
		default:
			die(fmt.Errorf("troubleshoot 子步骤: start/collect/draft/validate/submit/relay-submit"))
		}

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
				"img":  tbl.List(ids.KindImg),
				"vec":  tbl.List(ids.KindVec),
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
		// 全量收集后截断：ctx-up 取最近的 n 个上文（倒序输出，最近的在前）；
		// ctx-down 取最近的 n 个下文（正序）。带文字的图片条目（文字化描述回填）也算上下文。
		var matched []docindex.DocEntry
		for _, e := range idx.Entries {
			if e.Text == "" {
				continue
			}
			if up && e.Seq < int(seq) {
				matched = append(matched, e)
			}
			if !up && e.Seq > int(seq) {
				matched = append(matched, e)
			}
		}
		var blocks []map[string]any
		if up {
			for i := len(matched) - 1; i >= 0 && len(blocks) < n; i-- {
				e := matched[i]
				blocks = append(blocks, map[string]any{"seq": e.Seq, "global": e.Global, "text": e.Text})
			}
		} else {
			for _, e := range matched {
				if len(blocks) >= n {
					break
				}
				blocks = append(blocks, map[string]any{"seq": e.Seq, "global": e.Global, "text": e.Text})
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

	case "img2text-collect":
		// dvagent img2text-collect <项目名> —— 汇总 done 的 img2text 描述，回填 doc_index Content。
		if len(args) < 2 {
			die(fmt.Errorf("img2text-collect 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		n, err := gen.CollectImg2Text(q, p.Root)
		if err != nil {
			die(err)
		}
		out(map[string]any{"filled": n})

	case "gen-final":
		// dvagent gen-final <项目名> —— 全书落定后铺终审任务（T7）。
		if len(args) < 2 {
			die(fmt.Errorf("gen-final 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.GenFinal(q)
		if err != nil {
			die(err)
		}
		out(rep)

	case "doctor":
		// dvagent doctor [项目名] —— 操作前环境体检：工具链/字体/可操作建议。
		// 项目名可选（给了额外查项目 fonts/ 与配置回退）。
		root := ""
		if len(args) > 1 {
			p := mustProject(reg, args[1])
			root = p.Root
		}
		out(envcheck.Check(root, nil))

	case "luadraw-search":
		// dvagent luadraw-search <关键词...> —— 检索 luadraw 文档索引：
		// 命中条目的层级/标题/源码位置(doc-src-en)/PDF 物理页码（配合 pdftoppm 预览图）。
		// 多关键词 AND（均须命中标题，忽略大小写与空格）。
		if len(args) < 2 {
			die(fmt.Errorf("luadraw-search 需要关键词：dvagent luadraw-search <关键词...>"))
		}
		raw, err := os.ReadFile(filepath.Join(skillRoot(), "docs", "luadraw", "luadraw-index.json"))
		if err != nil {
			die(fmt.Errorf("luadraw 索引缺失（安装包应自带 docs/luadraw/luadraw-index.json）: %w", err))
		}
		var idx struct {
			Entries []struct {
				Kind    string `json:"kind"`
				Title   string `json:"title"`
				Tex     string `json:"tex"`
				Line    int    `json:"line"`
				PDFPage int    `json:"pdf_page"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(raw, &idx); err != nil {
			die(fmt.Errorf("luadraw 索引解析失败: %w", err))
		}
		kws := make([]string, 0, len(args)-1)
		for _, a := range args[1:] {
			kws = append(kws, strings.ToLower(strings.TrimSpace(a)))
		}
		hits := []map[string]any{}
		for _, e := range idx.Entries {
			t := strings.ToLower(strings.ReplaceAll(e.Title, " ", ""))
			ok := true
			for _, kw := range kws {
				if kw == "" {
					continue
				}
				if !strings.Contains(t, strings.ReplaceAll(kw, " ", "")) {
					ok = false
					break
				}
			}
			if ok {
				hits = append(hits, map[string]any{"kind": e.Kind, "title": e.Title,
					"tex": e.Tex, "line": e.Line, "pdf_page": e.PDFPage})
			}
			if len(hits) >= 30 {
				break
			}
		}
		out(map[string]any{"hits": hits, "count": len(hits),
			"tip": "pdftoppm -f <页> -l <页> docs/luadraw/luadraw-doc-en.pdf /tmp/pg 看该页预览图"})

	case "style-collect":
		// dvagent style-collect <项目名> —— 汇总 styled_text 提取内容进 style 素材池（选材数据面）。
		if len(args) < 2 {
			die(fmt.Errorf("style-collect 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		rep, err := gen.CollectStyleMaterials(q, p.Root)
		if err != nil {
			die(err)
		}
		out(rep)

	case "prep-split":
		// dvagent prep-split <项目名> <整本pdf> [每份页数=100] —— PDF 拆 part（提速版）。
		if len(args) < 3 {
			die(fmt.Errorf("prep-split 需要: <项目名> <pdf路径> [每份页数]"))
		}
		p := mustProject(reg, args[1])
		n := 100
		if len(args) > 3 {
			fmt.Sscanf(args[3], "%d", &n)
		}
		names, err := prep.SplitFast(args[2], filepath.Join(p.Root, "source", "parts"), prep.SplitFastConfig{PagesPerPart: n}, nil)
		if err != nil {
			die(err)
		}
		out(map[string]any{"parts": names, "dir": "source/parts"})

	case "prep-mineru":
		// dvagent prep-mineru <项目名> <part1.pdf> [part2.pdf ...] —— 逐个 part 跑 MinerU。
		if len(args) < 3 {
			die(fmt.Errorf("prep-mineru 需要: <项目名> <part.pdf...>"))
		}
		p := mustProject(reg, args[1])
		mineruOut := filepath.Join(p.Root, "mineru")
		var results []any
		for _, part := range args[2:] {
			res, err := prep.RunMinerU(part, mineruOut, prep.MinerUConfig{}, nil)
			if err != nil {
				results = append(results, map[string]any{"part": part, "error": err.Error()})
				continue
			}
			results = append(results, map[string]any{"part": part, "content_list": res.ContentList, "auto": res.AutoDir})
		}
		out(map[string]any{"mineru_output": mineruOut, "results": results})

	case "prep-organize":
		// dvagent prep-organize <项目名> —— content_list 拼章节 md（chapters/）。
		if len(args) < 2 {
			die(fmt.Errorf("prep-organize 需要: <项目名>"))
		}
		p := mustProject(reg, args[1])
		var autoDirs []string
		entries, err := os.ReadDir(filepath.Join(p.Root, "mineru"))
		if err != nil {
			die(fmt.Errorf("mineru 目录不存在（先 prep-mineru）: %w", err))
		}
		for _, e := range entries {
			if e.IsDir() {
				autoDir := filepath.Join(p.Root, "mineru", e.Name(), "auto")
				// 兼容 part 根产物（老布局/上游直出）：auto 缺失时用 part 根本身
				if _, serr := os.Stat(autoDir); serr != nil {
					autoDir = filepath.Join(p.Root, "mineru", e.Name())
				}
				autoDirs = append(autoDirs, autoDir)
			}
		}
		sort.Strings(autoDirs)
		outs, err := prep.Organize(autoDirs, filepath.Join(p.Root, "chapters"))
		if err != nil {
			die(err)
		}
		out(map[string]any{"md_files": outs})

	case "prep-index":
		// dvagent prep-index <项目名> [md名,md名...] —— 建 doc_index + 页表。
		if len(args) < 2 {
			die(fmt.Errorf("prep-index 需要: <项目名> [md名列表（默认 chapters/*.md）]"))
		}
		p := mustProject(reg, args[1])
		mdNames := args[2:]
		if len(mdNames) == 0 {
			matches, _ := filepath.Glob(filepath.Join(p.Root, "chapters", "*.md"))
			for _, m := range matches {
				mdNames = append(mdNames, filepath.Base(m))
			}
		}
		if len(mdNames) == 0 {
			die(fmt.Errorf("chapters/ 无 md（先 prep-organize）"))
		}
		// 页表来源：part 名 → 拆分产物 source/parts/<part>.pdf（page-render 的渲染源）。
		docindex.OriginPDFsFunc = func(mineruOutput, splitDir, subject, part string) string {
			cand := filepath.Join(p.Root, "source", "parts", part+".pdf")
			if _, err := os.Stat(cand); err == nil {
				return cand
			}
			return ""
		}
		idx, pages, err := docindex.Build(filepath.Join(p.Root, "mineru"), "", mdNames, filepath.Join(p.Root, "chapters"), filepath.Join(p.Root, "doc_index.json"))
		if err != nil {
			die(err)
		}
		out(map[string]any{"entries": len(idx.Entries), "total_pages": idx.Total(), "page_srcs": len(pages.Srcs)})

	case "prep-mineru-api":
		// dvagent prep-mineru-api <项目名> <part1.pdf> [part2.pdf ...]
		// MinerU API 模式：上传→排队→轮询→下载解压（token=环境变量 MINERU_TOKEN）。
		// 断点：已下载的 part 跳过（幂等，重跑续传）。
		if len(args) < 3 {
			die(fmt.Errorf("prep-mineru-api 需要: <项目名> <part.pdf...>"))
		}
		p := mustProject(reg, args[1])
		cfg, err := config.LoadLayered(skillRoot(), p.Root)
		if err != nil {
			die(err)
		}
		mc := mineruapi.DefaultConfig()
		if cfg.MinerU.APIBaseURL != "" {
			mc.APIBaseURL = cfg.MinerU.APIBaseURL
		}
		if cfg.MinerU.ModelVersion != "" {
			mc.ModelVersion = cfg.MinerU.ModelVersion
		}
		// 零值安全：config.yaml 没写的键不得覆盖出厂默认（否则 mineru 段少写几行，
		// is_ocr/language 被静默重置成 false/""——对齐老线默认 vlm/true/ch 的保险）。
		mc.IsOCR = cfg.MinerU.IsOCR || mc.IsOCR
		mc.EnableFormula = cfg.MinerU.EnableFormula || mc.EnableFormula
		mc.EnableTable = cfg.MinerU.EnableTable || mc.EnableTable
		if cfg.MinerU.Language != "" {
			mc.Language = cfg.MinerU.Language
		}
		if cfg.MinerU.PollInterval > 0 {
			mc.PollInterval = cfg.MinerU.PollInterval
		}
		mc.PollTimeout = cfg.MinerU.PollTimeout
		if cfg.MinerU.UploadTimeout > 0 {
			mc.UploadTimeout = cfg.MinerU.UploadTimeout
		}
		client, err := mineruapi.NewClient(mc, "")
		if err != nil {
			die(err)
		}
		// profile 档案：技能目录 data/profile.env（token/私有端点的规定存档位，600 权限）
		if exe, eerr := os.Executable(); eerr == nil {
			client.LoadProfile(filepath.Join(filepath.Dir(exe), "..", "..", "data", "profile.env"))
		}
		client.Printf = func(f string, a ...any) { fmt.Fprintf(os.Stderr, f, a...) }
		var results []any
		for _, part := range args[2:] {
			stem := strings.TrimSuffix(filepath.Base(part), filepath.Ext(part))
			dst := filepath.Join(p.Root, "mineru", stem)
			// 断点 ①：已有产物直接跳过。判据=part 根有 content_list（归一后的唯一落点；
			// 老判据"auto 存在"会把半截 auto/ 当完整产物，或永不命中——结构已收敛 part 根）。
			if hasContentList(dst) {
				results = append(results, map[string]any{"part": stem, "skipped": "已有产物"})
				continue
			}
			// 断点 ②（旧线语义）：batch_id 落盘 .mineru_task.json——重跑时服务端仍
			// 认得且非 failed 就续等/续下（不重传、不重复排队），否则重传。
			var item *mineruapi.ExtractResultItem
			batch := ""
			statePath := filepath.Join(dst, ".mineru_task.json")
			if raw, rerr := os.ReadFile(statePath); rerr == nil {
				var st struct {
					BatchID string `json:"batch_id"`
				}
				if json.Unmarshal(raw, &st) == nil && st.BatchID != "" {
					if valid, old := client.CheckOldTaskValid(st.BatchID); valid {
						batch = st.BatchID
						if old != nil {
							item = old // 已 done：直接续下载
						} else {
							item, err = client.PollBatch(batch)
							if err != nil {
								results = append(results, map[string]any{"part": stem, "batch": batch, "error": err.Error()})
								continue
							}
						}
					}
				}
			}
			if item == nil { // 无断点或断点失效 → 重传
				batch, err = client.CreateUploadTask(part)
				if err != nil {
					results = append(results, map[string]any{"part": stem, "error": err.Error()})
					continue
				}
				_ = os.MkdirAll(dst, 0o755)
				_ = os.WriteFile(statePath, []byte(fmt.Sprintf(`{"batch_id":%q,"file":%q}`, batch, filepath.Base(part))), 0o644)
				item, err = client.PollBatch(batch)
				if err != nil {
					results = append(results, map[string]any{"part": stem, "batch": batch, "error": err.Error()})
					continue
				}
			}
			if derr := client.DownloadAndExtract(item.FullZipURL, dst); derr != nil {
				results = append(results, map[string]any{"part": stem, "batch": batch, "error": "下载解压: " + derr.Error()})
				continue
			}
			// MinerU 上游已直接回 part 根布局（<data_id>_content_list.json + images/），
			// 不再套 <data_id>/ 目录——跳过归位，避免搬空成 <part>/auto/<data_id>/（auto 错位、图丢失）。
			// NormalizeLayout 重写后自识别三种布局（标准 auto/上游直出 part 根/中毒嵌套），
			// 正确布局原样返回、嵌套摊平——无需调用方特判。
			autoDir, nerr := mineruapi.NormalizeLayout(dst)
			if nerr != nil {
				results = append(results, map[string]any{"part": stem, "batch": batch, "error": "布局归位: " + nerr.Error()})
				continue
			}
			results = append(results, map[string]any{"part": stem, "batch": batch, "status": "done", "auto": autoDir})
		}
		out(map[string]any{"results": results})

	case "img2text-assemble":
		// dvagent img2text-assemble <项目名> <源md> <输出md>
		// 档位2 重组装：逐图任务决策 → 文字/mermaid/保留 三态替换，产出干净 md。
		if len(args) < 4 {
			die(fmt.Errorf("img2text-assemble 需要: <项目名> <源md> <输出md>"))
		}
		p := mustProject(reg, args[1])
		q, err := queue.Load(queue.ProjectQueuePath(p.Root))
		if err != nil {
			die(err)
		}
		src, err := os.ReadFile(args[2])
		if err != nil {
			die(err)
		}
		repls, err := mdimg.LoadReplacements(q, p.Root)
		if err != nil {
			die(err)
		}
		assembled, shifts := mdimg.AssembleWithShifts(string(src), repls)
		// 页码标记（P21 逐字移植）：原文行级对齐 → 改写后内容插 [P. N]；
		// 已有标记（T69 不回注）或无 mineru 产物时静默跳过。
		mdName := filepath.Base(args[2])
		lines, starts := mdimg.LineStarts(string(src))
		if plan := mdimg.PlanPageMarks(filepath.Join(p.Root, "mineru"), mdName, lines); plan != nil && !mdimg.HasPageMarks([]byte(assembled)) {
			var marks []mdimg.PageMark
			assembled, marks = plan.ApplyTo(assembled, lines, starts, shifts)
			_ = plan.Audit(marks)
		}
		if err := os.MkdirAll(filepath.Dir(args[3]), 0o755); err != nil {
			die(err)
		}
		if err := os.WriteFile(args[3], []byte(assembled), 0o644); err != nil {
			die(err)
		}
		nText, nMmd, nKeep := 0, 0, 0
		for _, r := range repls {
			switch {
			case r.Text != "":
				nText++
			case r.Mermaid != "":
				nMmd++
			default:
				nKeep++
			}
		}
		out(map[string]any{"output": args[3], "text_replaced": nText, "mermaid": nMmd, "kept": nKeep})

	case "upgrade-check":
		// dvagent upgrade-check —— 开跑/排障前自检上游是否有更新或已修（SKILL.md 第 0 步）
		sroot := "."
		if exe, eerr := os.Executable(); eerr == nil {
			sroot = filepath.Join(filepath.Dir(exe), "..", "..")
		}
		res, err := upgrade.Check(upgrade.DefaultUpstream(), sroot, upgrade.VersionFilePath(sroot), "")
		if err != nil {
			die(err)
		}
		out(res)
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
