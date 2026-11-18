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
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/deliver"
	"github.com/LingNc/DocVision/skills/kernel/internal/docindex"
	"github.com/LingNc/DocVision/skills/kernel/internal/project"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
	"github.com/LingNc/DocVision/skills/kernel/internal/verify"
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
		ck := verify.NewChecker(p.Root)
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
	}
	return fail(fmt.Errorf("未知工具: %s", name))
}
