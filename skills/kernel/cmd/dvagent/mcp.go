package main

// mcp.go：MCP server 壳（`dvagent mcp`）。同一组内核函数的第二张皮——只加协议
// 语义，不加新行为（防双层协议漂移）。实现 MCP 的 JSON-RPC 2.0 stdio 传输
// （newline-delimited），覆盖 tools/list + tools/call；工具面与 CLI 子命令同构。

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

// mcpTool 是一个 MCP 工具描述。
type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func strProp(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func numProp(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func schema(props map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

var mcpTools = []mcpTool{
	{"status", "全局蒸馏：项目列表+各阶段 done/pending+失败清单（冷启动唯一入口）",
		schema(map[string]any{}, []string{})},
	{"claim", "领 n 个任务+租约，返回自包含任务包",
		schema(map[string]any{"project": strProp("项目名"), "owner": strProp("认领者"),
			"n": numProp("数量(默认1)"), "types": strProp("类型过滤,逗号分隔")}, []string{"project", "owner"})},
	{"submit", "批量提交：逐条跑验收器，返回逐条 verdict",
		schema(map[string]any{"project": strProp("项目名"), "task_ids": strProp("逗号分隔任务id")},
			[]string{"project", "task_ids"})},
	{"note", "追加任务笔记（append-only）",
		schema(map[string]any{"project": strProp("项目名"), "task_id": strProp(""), "text": strProp("笔记内容")},
			[]string{"project", "task_id", "text"})},
	{"doc-search", "全书检索：回蒸馏命中（global页号+bbox+截断）",
		schema(map[string]any{"project": strProp("项目名"), "query": strProp("查询串"), "max": numProp("上限(默认5)")},
			[]string{"project", "query"})},
	{"assemble-verify", "复核 done 产物自验收以来未被篡改（装配前闸门）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"deliver", "装配交付：漂移闸门→归集→主文件→书签→standalone→盘点",
		schema(map[string]any{"project": strProp("项目名"), "cls": strProp("样式类名(可选)")},
			[]string{"project"})},
	// —— 生成器/流程主命令（与 CLI 同函数零漂移）——
	{"gen-classify", "扫描图目录铺 classify 任务（含 imgxxx 编号登记）",
		schema(map[string]any{"project": strProp("项目名"), "images": strProp("图目录(默认 source/images)")},
			[]string{"project"})},
	{"gen-convert", "章节 md 铺 convert 任务（md 拷沙箱作覆盖率对照源）",
		schema(map[string]any{"project": strProp("项目名"), "chapters": strProp("章节目录(默认 chapters)")},
			[]string{"project"})},
	{"gen-check", "为每个 convert 铺 blocked 核对任务（转换 done 自动解锁）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"gen-draw", "按 classify 结论铺 draw 任务（覆盖分组下组员跟随代表）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"gen-stylefix", "SK-I19 全书级样式后段：首跑铺 stylefix-assess（前置=convert/check 全落定），assess done 后重跑按 blocks.json 铺 stylefix 块",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"gen-img2text", "doc_index 图片条目铺 img2text 小轮转任务",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"gen-final", "全书落定后铺终审任务（T7）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"style-collect", "styled_text 提取内容汇总进 style 素材池（选材数据面）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"memory_put", "项目共享记忆区写入（memory/，多会话共享；key=相对路径，content=内容）",
		schema(map[string]any{"project": strProp("项目名"), "key": strProp("相对路径key"), "content": strProp("内容")}, []string{"project", "key", "content"})},
	{"memory_get", "项目共享记忆区读取（原始内容）",
		schema(map[string]any{"project": strProp("项目名"), "key": strProp("相对路径key")}, []string{"project", "key"})},
	{"memory_list", "项目共享记忆区列表（可选前缀过滤）",
		schema(map[string]any{"project": strProp("项目名"), "prefix": strProp("前缀(可空)")}, []string{"project"})},
	{"memory_rm", "项目共享记忆区删除",
		schema(map[string]any{"project": strProp("项目名"), "key": strProp("相对路径key")}, []string{"project", "key"})},
	{"img2text-collect", "img2text 描述回填 doc_index Content",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"ctx", "img2text 上下文（对齐旧 get_more_context）：单次双方向按 md 行返回 {image_line, range, lines}",
		schema(map[string]any{"project": strProp("项目名"), "task_id": strProp("任务id"),
			"up": numProp("上行数(默认5)"), "down": numProp("下行数(默认5)")},
			[]string{"project", "task_id"})},
	{"page-render", "渲染原书某页（可 bbox 裁剪）到 work/renders/",
		schema(map[string]any{"project": strProp("项目名"), "global": numProp("全书页码"), "bbox": strProp("x0,y0,x1,y1(可选)")},
			[]string{"project", "global"})},
	{"prep-organize", "content_list 拼章节 md（PREP）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"prep-index", "建 doc_index+页表（PREP）",
		schema(map[string]any{"project": strProp("项目名"), "md_names": strProp("逗号分隔，默认chapters/*.md")},
			[]string{"project"})},
	{"viewer-build", "生成静态看图台（分组/对比/调参导出）",
		schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"ids-list", "查人类编号映射表（imgxxx/vecxxx）",
		schema(map[string]any{"project": strProp("项目名"), "kind": strProp("img|vec(可选)")},
			[]string{"project"})},
	{"pause", "暂停项目（停派新任务）", schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
	{"resume", "恢复项目", schema(map[string]any{"project": strProp("项目名")}, []string{"project"})},
}

// serveMCP 在 stdin/stdout 上跑 MCP 协议循环。
func serveMCP(workspace string) {
	s := &mcpServer{workspace: workspace}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			continue // 非 JSON 行忽略
		}
		switch req.Method {
		case "initialize":
			enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "docvision-skills", "version": "0.1"}}})
		case "tools/list":
			enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"tools": mcpTools}})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": s.callTool(p.Name, p.Arguments)})
		case "notifications/initialized", "notifications/cancelled":
			// 通知无需响应
		default:
			enc.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method}})
		}
	}
}

type mcpServer struct{ workspace string }

func (s *mcpServer) callTool(name string, args json.RawMessage) map[string]any {
	fail := func(err error) map[string]any {
		return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "错误: " + err.Error()}}}
	}
	ok := func(v any) map[string]any {
		b, _ := json.MarshalIndent(v, "", "  ")
		return map[string]any{"content": []map[string]any{{"type": "text", "text": string(b)}}}
	}
	var a map[string]any
	_ = json.Unmarshal(args, &a)
	str := func(k string) string { v, _ := a[k].(string); return v }
	getInt := func(k string, def int) int {
		if v, ok := a[k].(float64); ok {
			return int(v)
		}
		return def
	}

	reg, err := project.Load(project.RegistryPath(s.workspace))
	if err != nil {
		return fail(err)
	}
	mustProject := func(name string) (*project.Project, error) {
		p, ok2 := reg.Projects[name]
		if !ok2 {
			return nil, fmt.Errorf("项目未登记: %s", name)
		}
		return p, nil
	}
	loadQ := func(p *project.Project) (*queue.Queue, error) {
		return queue.Load(queue.ProjectQueuePath(p.Root))
	}
	// pq = mustProject + loadQ 的组合简写。
	pq := func(name string) (*project.Project, *queue.Queue, error) {
		p, err := mustProject(name)
		if err != nil {
			return nil, nil, err
		}
		q, err := loadQ(p)
		if err != nil {
			return nil, nil, err
		}
		return p, q, nil
	}

	switch name {
	case "status":
		digest := map[string]any{"projects": reg.Digest()}
		for n, p := range reg.Projects {
			if p.Lifecycle == project.Archived {
				continue
			}
			if q, err := loadQ(p); err == nil {
				digest[n] = q.Digest()
			}
		}
		return ok(digest)

	case "claim":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		if p.Lifecycle != project.Active {
			return fail(fmt.Errorf("项目 %s 处于 %s", p.Name, p.Lifecycle))
		}
		q, err := loadQ(p)
		if err != nil {
			return fail(err)
		}
		var types []task.Type
		if t := str("types"); t != "" {
			for _, s2 := range strings.Split(t, ",") {
				types = append(types, task.Type(s2))
			}
		}
		claimed := q.Claim(str("owner"), getInt("n", 1), 1800, types...)
		for _, tk := range claimed {
			for _, d := range []string{"inputs", "scratch", "out"} {
				_ = os.MkdirAll(filepath.Join(p.Root, tk.Sandbox, d), 0o755)
			}
		}
		return ok(claimed)

	case "submit":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		q, err := loadQ(p)
		if err != nil {
			return fail(err)
		}
		ck := verify.NewChecker(p.Root).WithSkillRoot(skillRoot())
		results := map[string]any{}
		for _, id := range strings.Split(str("task_ids"), ",") {
			tk, err := q.Get(id)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			errs, notes := ck.Run(tk)
			v := task.Verdict{At: time.Now(), Pass: len(errs) == 0, Errors: errs, Notes: notes}
			nt, err := q.RecordVerdict(id, v, 6)
			if err != nil {
				results[id] = err.Error()
				continue
			}
			unlocked := 0
			if nt.Status == task.StatusDone {
				unlocked = len(q.Unblock(id))
			}
			results[id] = map[string]any{"status": nt.Status, "errors": errs, "notes": notes, "unlocked": unlocked}
		}
		return ok(results)

	case "note":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		q, err := loadQ(p)
		if err != nil {
			return fail(err)
		}
		tk, err := q.Get(str("task_id"))
		if err != nil {
			return fail(err)
		}
		f, err := os.OpenFile(filepath.Join(p.Root, tk.Sandbox, "notes.md"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		fmt.Fprintf(f, "\n## %s\n\n%s\n", time.Now().Format("01-02 15:04"), str("text"))
		return ok(map[string]any{"noted": str("task_id")})

	case "doc-search":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		idx, err := docindex.Load(filepath.Join(p.Root, "doc_index.json"))
		if err != nil {
			return fail(fmt.Errorf("索引未建: %w", err))
		}
		hits := idx.Search(str("query"), getInt("max", 5))
		return ok(map[string]any{"query": str("query"), "hits": hits, "total": len(hits)})

	case "assemble-verify":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		q, err := loadQ(p)
		if err != nil {
			return fail(err)
		}
		drifts := verify.CheckDrift(q.Tasks, p.Root)
		clean, lines := verify.DriftSummary(drifts)
		return ok(map[string]any{"clean": clean, "issues": lines, "detail": drifts})

	case "deliver":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		q, err := loadQ(p)
		if err != nil {
			return fail(err)
		}
		rep, err := deliver.Deliver(q, p.Root, str("cls"))
		if err != nil {
			return fail(err)
		}
		return ok(rep)

	// —— 生成器/流程（与 CLI 同函数）——
	case "gen-classify":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		images := str("images")
		if images == "" {
			images = "source/images"
		}
		rep, err := gen.GenClassify(q, p.Root, images)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "gen-convert":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		ch := str("chapters")
		if ch == "" {
			ch = "chapters"
		}
		rep, err := gen.GenConvert(q, p.Root, ch)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "gen-check":
		_, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := gen.GenCheck(q)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "gen-draw":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := gen.GenDraw(q, p.Root, nil) // MCP 默认全书；按章按需走 CLI gen-draw <项目> <章md>
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "gen-stylefix":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		q, qerr := queue.Load(queue.ProjectQueuePath(p.Root))
		if qerr != nil {
			return fail(qerr)
		}
		rep, err := gen.GenStylefixAssess(q)
		if err != nil {
			return fail(err)
		}
		if rep.AssessAdded == 1 {
			_ = q.Save()
			return ok(map[string]any{"phase": "assess", "report": rep})
		}
		rep2, err := gen.GenStylefixBlocks(q, p.Root)
		if err != nil {
			return ok(map[string]any{"phase": "assess", "report": rep, "next": err.Error()})
		}
		if rep2.BlocksAdded > 0 {
			_ = q.Save()
		}
		return ok(map[string]any{"phase": "blocks", "report": rep2})
	case "gen-img2text":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := gen.GenImg2Text(q, p.Root)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "gen-final":
		_, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := gen.GenFinal(q)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "memory_put":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		k, n, err := memoryPut(p.Root, str("key"), []byte(str("content")))
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"stored": k, "bytes": n})
	case "memory_get":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		data, err := memoryGet(p.Root, str("key"))
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"content": string(data)})
	case "memory_list":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		keys, err := memoryList(p.Root, str("prefix"))
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"keys": keys, "count": len(keys)})
	case "memory_rm":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		if err := memoryRm(p.Root, str("key")); err != nil {
			return fail(err)
		}
		return ok(map[string]any{"removed": str("key")})
	case "style-collect":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := gen.CollectStyleMaterials(q, p.Root)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "img2text-collect":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		n, err := gen.CollectImg2Text(q, p.Root)
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"filled": n})
	case "ctx":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		upN, downN := 5, 5
		if s := str("up"); s != "" {
			fmt.Sscanf(s, "%d", &upN)
		}
		if s := str("down"); s != "" {
			fmt.Sscanf(s, "%d", &downN)
		}
		res, err := runCtx(p.Root, str("task_id"), upN, downN)
		if err != nil {
			return fail(err)
		}
		return ok(res)

	case "page-render":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		pages, err := prep.LoadPageIndex(filepath.Join(p.Root, "doc_index_pages.json"))
		if err != nil {
			return fail(fmt.Errorf("页表未建: %w", err))
		}
		var bbox *[4]float64
		if b := str("bbox"); b != "" {
			var bb [4]float64
			if _, err := fmt.Sscanf(b, "%f,%f,%f,%f", &bb[0], &bb[1], &bb[2], &bb[3]); err == nil {
				bbox = &bb
			}
		}
		res, err := prep.RenderPage(pages, getInt("global", 0), bbox, filepath.Join(p.Root, "work", "renders"), nil)
		if err != nil {
			return fail(err)
		}
		return ok(res)
	case "prep-organize":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		var autoDirs []string
		entries, err := os.ReadDir(filepath.Join(p.Root, "mineru"))
		if err != nil {
			return fail(fmt.Errorf("mineru 目录不存在（先 prep-mineru）: %w", err))
		}
		for _, e := range entries {
			if e.IsDir() {
				autoDirs = append(autoDirs, filepath.Join(p.Root, "mineru", e.Name(), "auto"))
			}
		}
		sort.Strings(autoDirs)
		outs, err := prep.Organize(autoDirs, filepath.Join(p.Root, "chapters"))
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"md_files": outs})
	case "prep-index":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		mdNames := []string{}
		if s := str("md_names"); s != "" {
			for _, x := range strings.Split(s, ",") {
				mdNames = append(mdNames, strings.TrimSpace(x))
			}
		}
		if len(mdNames) == 0 {
			matches, _ := filepath.Glob(filepath.Join(p.Root, "chapters", "*.md"))
			for _, m := range matches {
				mdNames = append(mdNames, filepath.Base(m))
			}
		}
		if len(mdNames) == 0 {
			return fail(fmt.Errorf("chapters/ 无 md（先 prep-organize）"))
		}
		idx, pages, err := docindex.Build(filepath.Join(p.Root, "mineru"), "", mdNames, filepath.Join(p.Root, "chapters"), filepath.Join(p.Root, "doc_index.json"))
		if err != nil {
			return fail(err)
		}
		return ok(map[string]any{"entries": len(idx.Entries), "total_pages": idx.Total(), "page_srcs": len(pages.Srcs)})
	case "viewer-build":
		p, q, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		rep, err := viewer.Build(p.Root, q)
		if err != nil {
			return fail(err)
		}
		return ok(rep)
	case "ids-list":
		p, _, err := pq(str("project"))
		if err != nil {
			return fail(err)
		}
		tbl, err := ids.Load(p.Root)
		if err != nil {
			return fail(err)
		}
		if k := str("kind"); k != "" {
			return ok(map[string]any{k: tbl.List(ids.Kind(k))})
		}
		return ok(map[string]any{"img": tbl.List(ids.KindImg), "vec": tbl.List(ids.KindVec)})
	case "pause":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		if _, err := reg.Pause(p.Name); err != nil {
			return fail(err)
		}
		return ok(map[string]any{"paused": p.Name})
	case "resume":
		p, err := mustProject(str("project"))
		if err != nil {
			return fail(err)
		}
		if _, err := reg.Activate(p.Name); err != nil {
			return fail(err)
		}
		return ok(map[string]any{"resumed": p.Name})
	}
	return fail(fmt.Errorf("未知工具: %s", name))
}
