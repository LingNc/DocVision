package sessionview

// /api/figure-override —— 逐图门槛与"尽力还原接受"的落点（T307/P78）。
//
// 两处界面共用一个入口：核对页按 (project, md, img) 定位一条进度记录，会话页
// 手里只有作图转录名（vector_<成品名>.jsonl），所以三种写法都收——给了 path
// 就按转录名反查（与 latex 重画那条路用同一套成品名规则），否则按 md+img 拼。
//
// 与 /api/retry 一样，预览页不干活，只改 `progress_items` 里的几个字段：
//   - min_overlap：这张图的提交门槛覆盖值（0 = 用 latex.figure_check 的全局值，
//     负数 = 这张图不卡门槛）；
//   - accept_as_is + note：粘性"尽力还原"标记（只写标记不写成绩——成绩是提交
//     那一刻量的，这里没有成品可量）；clear_accept 撤销它；
//   - clear_best：忘了上次成绩，让这张图下次重跑时重新量。
// "重画这张"仍然走 role=vector 的 retry 队列，本入口不碰产物。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mineru-tools/internal/retryq"
	"mineru-tools/internal/reviewpage"
)

type figureOverrideRequest struct {
	Project string `json:"project"`
	MD      string `json:"md,omitempty"`
	Img     string `json:"img,omitempty"`
	// Path 是作图转录名（vector_<成品名>.jsonl），会话页用它定位。
	Path string `json:"path,omitempty"`
	// MinOverlap 为空 = 不改；有值才写（0 = 用全局，负数 = 不卡）。
	MinOverlap  *float64 `json:"min_overlap,omitempty"`
	AcceptAsIs  *bool    `json:"accept_as_is,omitempty"`
	Note        string   `json:"note,omitempty"`
	ClearAccept bool     `json:"clear_accept,omitempty"`
	ClearBest   bool     `json:"clear_best,omitempty"`
}

func (v *viewerServer) serveFigureOverride(w http.ResponseWriter, r *http.Request) {
	if !v.allowRetry {
		http.Error(w, "改写入口已关闭（docvision sessions 用 --no-retry 启动的）", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "只接受 POST", http.StatusMethodNotAllowed)
		return
	}
	var req figureOverrideRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "请求体不是合法 JSON", http.StatusBadRequest)
		return
	}
	proj, ok := v.projectWorkspace(req.Project)
	if !ok {
		http.Error(w, "项目名不合法或不存在", http.StatusBadRequest)
		return
	}
	progDir := filepath.Join(proj, "source", "progress_items")
	mdName, imgName := strings.TrimSpace(req.MD), strings.TrimSpace(req.Img)
	if p := strings.TrimSpace(req.Path); p != "" {
		base := filepath.Base(p)
		if !strings.HasPrefix(base, "vector_") || !strings.HasSuffix(base, ".jsonl") {
			http.Error(w, "path 只认 vector_<成品名>.jsonl 这条转录", http.StatusBadRequest)
			return
		}
		texBase := strings.TrimSuffix(strings.TrimPrefix(base, "vector_"), ".jsonl")
		found := false
		for _, it := range loadFigureProgress(progDir) {
			if vectorTexBaseOf(it.MD, it.Img, it.Label) == texBase {
				mdName, imgName, found = it.MD, it.Img, true
				break
			}
		}
		if !found {
			http.Error(w, "找不到这张图对应的进度记录（成品名 "+texBase+"）", http.StatusNotFound)
			return
		}
	}
	if mdName == "" || imgName == "" {
		http.Error(w, "要定位一张图：给 (md 与 img)，或者给作图转录名 path", http.StatusBadRequest)
		return
	}
	file := filepath.Join(progDir, reviewpage.SanitizeName(mdName), reviewpage.SanitizeName(imgName)+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, "读不到这张图的进度记录："+err.Error(), http.StatusNotFound)
		return
	}
	var rec map[string]any
	if err := json.Unmarshal(data, &rec); err != nil {
		http.Error(w, "进度记录不是合法 JSON："+err.Error(), http.StatusInternalServerError)
		return
	}
	changed := 0
	if req.MinOverlap != nil {
		if *req.MinOverlap > 1 {
			http.Error(w, "min_overlap 是 0-1 的比例（例如 0.17 = 17%），也可以是负数表示不卡", http.StatusBadRequest)
			return
		}
		if *req.MinOverlap == 0 {
			delete(rec, "fig_min_overlap") // 0 = 用全局：不写字段
		} else {
			rec["fig_min_overlap"] = *req.MinOverlap
		}
		changed++
	}
	if req.ClearAccept {
		delete(rec, "fig_accept_as_is")
		delete(rec, "fig_accept_note")
		changed++
	}
	if req.ClearBest {
		delete(rec, "fig_best_overlap")
		changed++
	}
	if req.AcceptAsIs != nil {
		if *req.AcceptAsIs {
			note := strings.TrimSpace(req.Note)
			if note == "" {
				http.Error(w, "按尽力还原接受要写一句 note：差在哪里、为什么到不了", http.StatusBadRequest)
				return
			}
			rec["fig_accept_as_is"] = true
			rec["fig_accept_note"] = note
		} else {
			delete(rec, "fig_accept_as_is")
			delete(rec, "fig_accept_note")
		}
		changed++
	}
	if changed == 0 {
		http.Error(w, "没有要改的东西", http.StatusBadRequest)
		return
	}
	// T315/T316：门槛改动当场生效，两个方向都算。
	//  · 抬到高于这张图上次成绩 → 标回待处理，并在进度里留记号 fig_requeued；
	//  · 再放宽到不高于成绩（或设负数）→ 把那条排队撤回（状态回 done），
	//    免得一直在重复的图又被白画一遍。
	// 只动"完成态/我们自己排的队 + 有成绩 + 没被尽力还原接受过"的图；改成用全局
	// （0）读不到全局值，不动状态。成品字段一律不动：门槛排队不等于成品作废，
	// 留着它们，撤回时不必凭空重建。
	flipped, cancelled := false, false
	note := "已保存；这张图下次运行（或点\"重画这张图\"）时按新设置判定"
	if req.MinOverlap != nil {
		limit := *req.MinOverlap
		best, hasBest := rec["fig_best_overlap"].(float64)
		accepted, _ := rec["fig_accept_as_is"].(bool)
		status, _ := rec["status"].(string)
		ourQueue, _ := rec["fig_requeued"].(string)
		passes := hasBest && best > 0 && (limit < 0 || best >= limit)
		switch {
		case limit > 0 && hasBest && best > 0 && !accepted && status == "done" && best < limit:
			rec["status"] = "classified"
			rec["fig_requeued"] = "threshold"
			flipped = true
			note = fmt.Sprintf("已保存并当场标回待处理（上次重叠 %.2f%% < 新门槛 %.2f%%）",
				best*100, limit*100)
		case ourQueue == "threshold" && passes && !accepted && status == "classified":
			rec["status"] = "done"
			delete(rec, "fig_requeued")
			cancelled = true
			note = fmt.Sprintf("已保存并撤回重画（上次重叠 %.2f%% 已不低于新门槛）：这张图不会再画一遍", best*100)
		}
	}
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		http.Error(w, "序列化失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(file, out, 0o644); err != nil {
		http.Error(w, "写进度记录失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	// T317：抬门槛不只是"下次运行才画"——顺手排一条重画请求（与「重画这张图」
	// 同一条队列）。正在跑的 latex 会把它接进并发槽（图片阶段一结束、或别的阶段
	// 里都能接），没在跑就留到下次启动。抬门槛本来就是"这张图要重画"，让人再手
	// 点一遍重画或干脆重启一次，没有意义。排请求放在写盘之后：watcher 可能立刻
	// 看到它，那时进度里必须已经写着 classified。
	queued := false
	label, _ := rec["label"].(string)
	trName := "vector_" + vectorTexBaseOf(mdName, imgName, label) + ".jsonl"
	switch {
	case flipped:
		if _, qerr := retryq.Queue(proj, retryq.Request{
			Project: req.Project, Role: "vector", Reason: "门槛抬高，重画", Source: "web", Path: trName,
		}); qerr != nil {
			note += "；但排进重画队列失败：" + qerr.Error()
		} else {
			queued = true
			note += "；已排进重画队列：正在跑的 latex 会接着画这张图（没在跑就留到下次启动）"
		}
	case cancelled:
		// 撤回排队时顺手把还没被接走的那一条撤掉，否则放宽门槛之后它照样会把
		// 这张图重画一遍——白画一次正是这次要避免的。
		if n := dropVectorRequest(proj, trName); n > 0 {
			note += "；已从重画队列里撤下这条请求"
		}
	}
	writeJSON(w, map[string]any{
		"ok": true, "md": mdName, "img": imgName,
		"min_overlap":    rec["fig_min_overlap"],
		"best_overlap":   rec["fig_best_overlap"],
		"accept_as_is":   rec["fig_accept_as_is"],
		"accept_note":    rec["fig_accept_note"],
		"note":           note,
		"requeued":       flipped,
		"queued":         queued,
		"cancelled":      cancelled,
		"cleared_best":   req.ClearBest,
		"cleared_accept": req.ClearAccept || (req.AcceptAsIs != nil && !*req.AcceptAsIs),
	})
}

// dropVectorRequest 删掉队列里还没被接走的这条重画请求，返回删掉的条数。
// 已经被接走的请求不在 Pending 里（那边会改名成 .done.json），这里不动它。
func dropVectorRequest(proj, trName string) int {
	n := 0
	for _, f := range retryq.Pending(proj) {
		if f.Request.Role == "vector" && f.Request.Path == trName {
			if err := os.Remove(f.Path); err == nil {
				n++
			}
		}
	}
	return n
}

// progressLite 是反查只要的三个字段（其余不读，免得把 schema 抄两遍）。
type progressLite struct {
	MD    string `json:"md"`
	Img   string `json:"img"`
	Label string `json:"label"`
}

// loadFigureProgress 读一遍 progress_items（每个 md 一个子目录、每图一个 json）。
func loadFigureProgress(dir string) []progressLite {
	var out []progressLite
	mds, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, md := range mds {
		if !md.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(dir, md.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, md.Name(), f.Name()))
			if err != nil {
				continue
			}
			var it progressLite
			if json.Unmarshal(data, &it) == nil && it.MD != "" && it.Img != "" {
				out = append(out, it)
			}
		}
	}
	return out
}

// vectorTexBaseOf 与 latex 里 vectorTexBase 的成品名规则必须一致（这里不能
// import latex——latex 已经 import 了本包）。两边都由 SanitizeName 收口，
// 规则本身只有"图名__类别"再拼书名的三段式。
func vectorTexBaseOf(mdName, imgPath, label string) string {
	imgBase := reviewpage.SanitizeName(strings.TrimSuffix(filepath.Base(imgPath), filepath.Ext(imgPath)))
	base := imgBase + "__" + reviewpage.SanitizeName(label)
	if base == "__" || len(base) < 3 {
		base = imgBase
	}
	if base == "" {
		return ""
	}
	return reviewpage.SanitizeName(strings.TrimSuffix(mdName, filepath.Ext(mdName))) + "__" + base
}
