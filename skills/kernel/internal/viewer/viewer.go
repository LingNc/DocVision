// Package viewer 是看图台（SK-P30 v1）：静态 HTML 报告生成器。
// 只读观察者+请求文件机制（沿用旧线 watcher 思想）：页面是纯静态产物，
// 用户/agent 调阈值或并拆组后**导出请求 JSON**，丢进 work/retry/ 由
// dvagent viewer-apply 消费（内核仍是唯一写通道）。
package viewer

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/LingNc/DocVision/skills/kernel/internal/dedupe"
	"github.com/LingNc/DocVision/skills/kernel/internal/figcheck"
	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
	"github.com/LingNc/DocVision/skills/kernel/internal/queue"
	"github.com/LingNc/DocVision/skills/kernel/internal/task"
)

// Report 是看图台数据源（也是测试断言面）。
type Report struct {
	Images  []ImageCard `json:"images"`
	Groups  []GroupCard `json:"groups"`
	Progress Progress   `json:"progress"`
	Viewer  string      `json:"viewer"` // index.html 路径
}

// ImageCard 是一张图的信息。
type ImageCard struct {
	ID      string `json:"id"`       // imgxxx（人类编号）
	Path    string `json:"path"`     // 相对项目根
	Verdict string `json:"verdict"`  // vector/styled_text/raster/未分类
	TaskID  string `json:"task_id"`
}

// GroupCard 是一个相似组。
type GroupCard struct {
	Rep      string   `json:"rep"`      // 代表图 id（imgxxx）
	Members  []string `json:"members"`  // 成员 id（含代表，排序）
	Exact    bool     `json:"exact"`
	Similar  float64  `json:"similar"`  // 与代表的最小重合度（组内闸门口径）
}

// Progress 是阶段进度汇总。
type Progress struct {
	Classified int `json:"classified"`
	Vector     int `json:"vector"`
	StyledText int `json:"styled_text"`
	Raster     int `json:"raster"`
	Grouped    int `json:"grouped"` // 入组图片数
}

// Build 扫描项目生成看图台：
//   - 图清单来自 ids 表（imgxxx）+ 队列里 classify 任务的 verdict；
//   - 分组用 dedupe 本地算法（FindCandidatesAligned，0.60 闸门——用户调阈值在页面上改）；
//   - 产物 work/viewer/index.html（引用相对路径的图，不内嵌）。
func Build(projectRoot string, q *queue.Queue) (*Report, error) {
	rep := &Report{}
	tbl, err := ids.Load(projectRoot)
	if err != nil {
		return rep, err
	}
	// 1. 图清单：ids 表里的 img 记录
	for _, r := range tbl.List(ids.KindImg) {
		rep.Images = append(rep.Images, ImageCard{ID: r.ID, Path: r.Path, TaskID: r.TaskID})
	}
	sort.Slice(rep.Images, func(i, j int) bool { return rep.Images[i].ID < rep.Images[j].ID })

	// 2. verdict 回填 + 进度
	verdictOf := map[string]string{}
	for _, tk := range q.Tasks {
		if tk.Type != task.TypeClassify || len(tk.Verdicts) == 0 {
			continue
		}
		last := tk.Verdicts[len(tk.Verdicts)-1]
		if !last.Pass {
			continue
		}
		b, err := os.ReadFile(filepath.Join(projectRoot, tk.Sandbox, "out", "verdict.json"))
		if err != nil {
			continue
		}
		var v struct {
			Class string `json:"class"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			continue
		}
		verdictOf[tk.ID] = v.Class
	}
	for i := range rep.Images {
		if vc, ok := verdictOf[rep.Images[i].TaskID]; ok {
			rep.Images[i].Verdict = vc
			rep.Progress.Classified++
			switch vc {
			case "vector":
				rep.Progress.Vector++
			case "styled_text":
				rep.Progress.StyledText++
			case "raster":
				rep.Progress.Raster++
			}
		}
	}

	// 3. 分组：dedupe 本地算法（只读图，不写任何项目文件）
	var samples []dedupe.Sample
	pathOf := map[string]string{}
	for _, img := range rep.Images {
		imgPath := filepath.Join(projectRoot, filepath.FromSlash(img.Path))
		imgData, err := figcheck.DecodeImage(imgPath)
		if err != nil {
			continue
		}
		samples = append(samples, dedupe.Sample{Key: img.ID, Image: imgData})
		pathOf[img.ID] = img.Path
	}
	if len(samples) >= 2 {
		groups, _ := dedupe.FindCandidatesAligned(samples, 0.60, 0.10)
		idOf := map[string]string{} // path→id 已含在 ImageCard
		idByPath := map[string]string{}
		for _, img := range rep.Images {
			idByPath[img.Path] = img.ID
		}
		for _, g := range groups {
			gc := GroupCard{Exact: g.Exact}
			for _, m := range g.Members {
				if id, ok := idByPath[pathOf[m]]; ok {
					gc.Members = append(gc.Members, id)
				}
			}
			sort.Strings(gc.Members)
			if len(gc.Members) >= 2 {
				if id, ok := idByPath[pathOf[g.Members[0]]]; ok {
					gc.Rep = id
				}
				rep.Groups = append(rep.Groups, gc)
				rep.Progress.Grouped += len(gc.Members)
			}
		}
		_ = idOf
	}
	sort.Slice(rep.Groups, func(i, j int) bool { return rep.Groups[i].Rep < rep.Groups[j].Rep })

	// 4. 渲染 HTML
	viewerDir := filepath.Join(projectRoot, "work", "viewer")
	if err := os.MkdirAll(viewerDir, 0o755); err != nil {
		return rep, err
	}
	htmlPath := filepath.Join(viewerDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(renderHTML(rep, projectRoot)), 0o644); err != nil {
		return rep, err
	}
	rep.Viewer = htmlPath
	return rep, nil
}

// renderHTML 生成自包含页面：图卡片（相对路径引用）、组卡片、进度条、
// 阈值输入+导出请求（下载 JSON，丢 work/retry/ 由 viewer-apply 消费）。
func renderHTML(rep *Report, projectRoot string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html lang="zh"><head><meta charset="utf-8"><title>DocVision 看图台</title>
<style>body{font-family:system-ui;margin:24px;background:#fafafa}h1{font-size:20px}.card{display:inline-block;margin:8px;padding:8px;border:1px solid #ddd;border-radius:8px;background:#fff;vertical-align:top;width:220px}
.card img{max-width:200px;max-height:150px;display:block}.id{font-family:monospace;color:#06c}.verd{font-size:12px;padding:2px 6px;border-radius:4px;background:#eef}
.verd.vector{background:#dfd}.verd.styled_text{background:#ffd}.verd.raster{background:#ddd}.grp{border:1px solid #bbb;border-radius:10px;margin:16px 0;padding:12px;background:#fff}
.progress{font-size:14px;margin:8px 0}button{padding:6px 14px}</style></head><body>
<h1>DocVision 看图台 <span style="font-size:12px;color:#888">` + time.Now().Format("01-02 15:04") + `</span></h1>`)
	fmt.Fprintf(&b, `<div class="progress">已分类 <b>%d</b>（vector %d / styled_text %d / raster %d）｜入组 %d 张｜组 %d 个</div>`,
		rep.Progress.Classified, rep.Progress.Vector, rep.Progress.StyledText, rep.Progress.Raster, rep.Progress.Grouped, len(rep.Groups))

	b.WriteString(`<h2>分组（相似图并组，代表=将被重绘的那张；勾选=保留在组内，去掉勾选=移出）</h2>`)
	for _, g := range rep.Groups {
		fmt.Fprintf(&b, `<div class="grp"><b>组 代表=<span class="id">%s</span></b> 成员 %d 张 %s <label style="font-size:12px;color:#888"><input type="checkbox" class="gck" data-rep="%s" checked>整组保留</label>`,
			html.EscapeString(g.Rep), len(g.Members), exactBadge(g.Exact), html.EscapeString(g.Rep))
		for _, m := range g.Members {
			img := findImg(rep, m)
			if img == nil {
				continue
			}
			fmt.Fprintf(&b, `<div class="card"><input type="checkbox" class="mck" data-rep="%s" data-id="%s" checked><span class="id">%s</span> %s<br><a href="../../%s" class="lb"><img src="../../%s" loading="lazy"></a></div>`,
				html.EscapeString(g.Rep), html.EscapeString(m), html.EscapeString(img.ID), verdictBadge(img.Verdict), html.EscapeString(img.Path), html.EscapeString(img.Path))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<h2>全部图片</h2>`)
	for i := range rep.Images {
		img := &rep.Images[i]
		fmt.Fprintf(&b, `<div class="card"><span class="id">%s</span> %s<br><a href="../../%s" target="_blank"><img src="../../%s" loading="lazy"></a><br><span style="font-size:11px;color:#888">%s</span></div>`,
			html.EscapeString(img.ID), verdictBadge(img.Verdict), html.EscapeString(img.Path), html.EscapeString(img.Path), html.EscapeString(img.Path))
	}

	b.WriteString(`<h2>调参与请求</h2>
<div>组闸门阈值 <input id="th" type="number" step="0.05" min="0.3" max="0.95" value="0.60" style="width:70px">
<button onclick="exportReq()">导出重分组请求</button>
<button onclick="exportEdit()">导出组调整请求</button>
（下载的 JSON 存到项目 <code>work/retry/</code>，然后 <code>dvagent viewer-apply &lt;项目&gt; work/retry/&lt;文件&gt;</code>）</div>
<div id="lb" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,.9);z-index:9;text-align:center">
  <img id="lbi" style="max-width:92%;max-height:88%;margin-top:3%">
  <div style="color:#fff;font-size:13px"><span id="lbt"></span>　←/→ 翻张，Esc 关闭</div>
</div>
<script>
const IMGS=[` + imgListJS(rep) + `];let lbi=0;
document.querySelectorAll('a.lb').forEach(a=>a.addEventListener('click',e=>{e.preventDefault();openLb(a.querySelector('img').src)}));
function openLb(src){lbi=IMGS.indexOf(src);if(lbi<0)lbi=0;showLb();document.getElementById('lb').style.display='block'}
function showLb(){document.getElementById('lbi').src=IMGS[lbi];document.getElementById('lbt').textContent=(lbi+1)+' / '+IMGS.length}
document.addEventListener('keydown',e=>{if(document.getElementById('lb').style.display!=='block')return;
  if(e.key==='Escape')document.getElementById('lb').style.display='none';
  if(e.key==='ArrowRight'&&lbi<IMGS.length-1){lbi++;showLb()}
  if(e.key==='ArrowLeft'&&lbi>0){lbi--;showLb()}});
document.getElementById('lb').addEventListener('click',()=>document.getElementById('lb').style.display='none');
function dl(obj,name){const b=new Blob([JSON.stringify(obj,null,2)],{type:'application/json'});
  const a=document.createElement('a');a.href=URL.createObjectURL(b);a.download=name;a.click()}
function exportReq(){
  const th=parseFloat(document.getElementById('th').value);
  dl({type:"regroup",align_min:th,shift_pct:0.10},'regroup.json')}
function exportEdit(){
  const byRep={};
  document.querySelectorAll('.mck').forEach(c=>{
    const rep=c.dataset.rep,id=c.dataset.id;
    byRep[rep]=byRep[rep]||{rep:rep,members:[]};
    if(c.checked)byRep[rep].members.push(id)});
  const groups=Object.values(byRep).filter(g=>g.members.length>=2);
  dl({type:"group_edit",groups:groups},'group_edit.json')}
</script></body></html>`)
	return b.String()
}

func verdictBadge(v string) string {
	if v == "" {
		v = "未分类"
	}
	return `<span class="verd ` + html.EscapeString(v) + `">` + html.EscapeString(v) + `</span>`
}

func exactBadge(exact bool) string {
	if exact {
		return `<span class="verd" style="background:#cfc">精确同图</span>`
	}
	return ""
}

func findImg(rep *Report, id string) *ImageCard {
	for i := range rep.Images {
		if rep.Images[i].ID == id {
			return &rep.Images[i]
		}
	}
	return nil
}

// imgListJS 输出图片路径数组（灯箱翻页用），JSON 安全。
func imgListJS(rep *Report) string {
	var parts []string
	for i := range rep.Images {
		parts = append(parts, `"../../`+jsEscape(rep.Images[i].Path)+`"`)
	}
	return strings.Join(parts, ",")
}

// jsEscape 转义 JSON 字符串值里的特殊字符（路径含引号/反斜杠时兜底）。
func jsEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return r.Replace(s)
}
