package sessionview

import (
	"regexp"
	"strings"
	"testing"
)

// unitProbeJS 收集"显示单位开关"两个状态下的计数文案，全部写进 #unit-probe
// （headless 只回一份 DOM，所以先把两个状态都采下来）。
const unitProbeJS = `
      var probe = [];
      var push = function (k, v) { probe.push(k + '=' + String(v).replace(/\s+/g, ' ').trim()); };
      var firstText = function (sel) {
        var n = document.querySelector(sel);
        return n ? n.textContent : 'none';
      };
      var tile = function (label) {
        var hit = 'none';
        Array.prototype.forEach.call(document.querySelectorAll('.tile'), function (t) {
          var l = t.querySelector('.tile-label');
          if (l && l.textContent === label) { hit = t.querySelector('.tile-value').textContent; }
        });
        return hit;
      };
      var imgTile = function () {
        var hit = 'none';
        Array.prototype.forEach.call(document.querySelectorAll('.tile'), function (t) {
          var l = t.querySelector('.tile-label');
          if (l && l.textContent.indexOf('图片') === 0) {
            hit = l.textContent + ' / ' + t.querySelector('.tile-value').textContent;
          }
        });
        return hit;
      };
      var trajHead = function () {
        var ths = document.querySelectorAll('.traj-table th');
        return ths.length > 5 ? ths[5].textContent : 'none';
      };
      var trajCell = function () {
        var c = document.querySelector('.traj-table tbody tr td.traj-num-cell');
        return c ? c.textContent : 'none';
      };
      push('toggle', firstText('#unit-toggle'));
      push('tail', firstText('.disclosure-tool .line-tail'));
      push('fold', firstText('.text-toggle'));
      push('thinking', firstText('.disclosure-thinking .line-tail'));
      push('inTile', tile('输入 tokens'));
      push('imgTile', imgTile());
      document.getElementById('tab-traj').click();
      push('trajHead', trajHead());
      push('trajCell', trajCell());
      document.getElementById('tab-chat').click();
      document.getElementById('unit-toggle').click();
      push('toggle2', firstText('#unit-toggle'));
      push('tail2', firstText('.disclosure-tool .line-tail'));
      push('fold2', firstText('.text-toggle'));
      push('thinking2', firstText('.disclosure-thinking .line-tail'));
      push('inTile2', tile('输入 tokens'));
      document.getElementById('tab-traj').click();
      push('trajHead2', trajHead());
      push('trajCell2', trajCell());
      document.getElementById('tab-chat').click();
      var out = document.createElement('div');
      out.id = 'unit-probe';
      out.textContent = probe.join(' ;; ');
      document.body.appendChild(out);
`

// unitProbe 把一个探针字段从渲染后的 DOM 里取出来。
func unitProbe(t *testing.T, dom, key string) string {
	t.Helper()
	i := strings.Index(dom, `id="unit-probe"`)
	if i < 0 {
		t.Fatalf("页面里没有探针输出（渲染可能失败）；DOM 片段：%s", head(dom, 400))
	}
	probe := dom[i:]
	if gt := strings.Index(probe, ">"); gt >= 0 {
		probe = probe[gt+1:]
	}
	if end := strings.Index(probe, "</div>"); end > 0 {
		probe = probe[:end]
	}
	for _, part := range strings.Split(probe, ";;") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(part, key+"="))
		}
	}
	t.Fatalf("探针里没有 %s：%s", key, probe)
	return ""
}

func head(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// TestViewerUnitToggleInChromium 钉住用户要求的那条开关：
//   - 默认按 token 显示，且 token 是本地估算 → 一律带 ≈；
//   - 点一下切成字符，行内计数、折叠文案、轨迹表头与计数列全部换口径；
//   - 详情栏的「输入 tokens」是厂商实测值，任何状态下都不带 ≈。
func TestViewerUnitToggleInChromium(t *testing.T) {
	root := t.TempDir()
	// media 引用按转录所在目录解析（与 viewer.js 的 mediaURL 同规则）。
	writePNG(t, jsonl(root, "proj", "work", "sessions", "media", "fig.png"), 1000, 800)

	body := strings.Repeat("这一行是任务提示的一部分。\n", 25)
	reasoning := "先看结构，再决定读哪个文件。\n"
	args := `{"path":"work/chapter_001.tex","start":1,"end":40,"note":"` +
		strings.Repeat("x", 600) + `"}`
	writeFile(t, jsonl(root, "proj", "work", "sessions", "unit_01.jsonl"), transcript(
		`{"t":"meta","kind":"system","session_label":"convert:01","model":"wire-A","sys_hash":"deadbeefcafe","text":`+jsonString(body)+`,"tools":[{"name":"read_file","description":"读一个文件","parameters":"{\"type\":\"object\"}"}]}`,
		`{"t":"usage","ts":"2025-01-02T03:00:00Z","model":"wire-A","round":1,"prompt_tokens":12345,"cached_tokens":10000,"completion_tokens":678,"duration_ms":4000,"ttft_ms":1000,"finish_reason":"tool_calls"}`,
		`{"t":"msg","role":"user","text":`+jsonString(body)+`,"ts":"2025-01-02T03:00:01Z"}`,
		`{"t":"msg","role":"assistant","text":`+jsonString(body)+`,"reasoning_content":`+jsonString(reasoning)+`,"ts":"2025-01-02T03:00:02Z","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":`+jsonString(args)+`}}]}`,
		`{"t":"msg","role":"tool","tool_call_id":"call_1","text":`+jsonString(strings.Repeat("结果的一行。\n", 18))+`,"ts":"2025-01-02T03:00:03Z"}`,
		`{"t":"msg","role":"user","text":"Tool image output (for your visual review):","images":["file://media/fig.png"],"ts":"2025-01-02T03:00:04Z"}`,
	))

	dom := renderViewerDOM(t, root, "unit_01.jsonl", unitProbeJS)

	// ① 默认 token：开关文案就是 token，行内与折叠文案都带 ≈ 且写 tokens。
	if got := unitProbe(t, dom, "toggle"); got != "token" {
		t.Errorf("默认开关文案 = %q，期望 token", got)
	}
	if got := unitProbe(t, dom, "tail"); !strings.Contains(got, "≈") || !strings.Contains(got, "tokens") {
		t.Errorf("默认工具行计数 = %q，期望带 ≈ 的 tokens 形态", got)
	}
	if got := unitProbe(t, dom, "tail"); !strings.Contains(got, "→") {
		t.Errorf("工具行没有同时给出输入与输出：%q", got)
	}
	if got := unitProbe(t, dom, "fold"); !strings.Contains(got, "≈") || !strings.Contains(got, "tokens") {
		t.Errorf("默认折叠文案 = %q，期望带 ≈ 的 tokens 形态", got)
	}
	if got := unitProbe(t, dom, "thinking"); !strings.Contains(got, "≈") {
		t.Errorf("思考行计数 = %q，期望带 ≈ 的 tokens 形态", got)
	}
	if got := unitProbe(t, dom, "trajHead"); got != "tokens" {
		t.Errorf("轨迹表计数列表头 = %q，期望 tokens", got)
	}
	if got := unitProbe(t, dom, "trajCell"); !strings.HasPrefix(got, "≈ ") {
		t.Errorf("轨迹表计数列 = %q，期望 ≈ 前缀", got)
	}

	// ② 图片瓦片：按尺寸折算（1000×800 → 800000/750 = 1066 → ≈ 1.1k）。
	if got := unitProbe(t, dom, "imgTile"); got != "图片 1 张 / ≈ 1.1k" {
		t.Errorf("图片瓦片 = %q，期望 \"图片 1 张 / ≈ 1.1k\"", got)
	}

	// ③ 厂商实测值不带 ≈（输入 tokens 是 prompt_tokens）。
	for _, key := range []string{"inTile", "inTile2"} {
		got := unitProbe(t, dom, key)
		if got == "none" || got == "" {
			t.Fatalf("%s 没有渲染出来（详情栏指标缺失）", key)
		}
		if strings.Contains(got, "≈") {
			t.Errorf("%s = %q：厂商实测值不该带 ≈", key, got)
		}
	}
	if got := unitProbe(t, dom, "inTile"); got != "12k" {
		t.Errorf("输入 tokens 瓦片 = %q，期望 12k（厂商 prompt_tokens 照抄）", got)
	}

	// ④ 切成字符：全部变成精确字符数，不再出现 ≈ / tokens。
	if got := unitProbe(t, dom, "toggle2"); got != "字符" {
		t.Errorf("切换后开关文案 = %q，期望 字符", got)
	}
	for _, key := range []string{"tail2", "fold2", "thinking2"} {
		got := unitProbe(t, dom, key)
		if !strings.Contains(got, "字符") {
			t.Errorf("%s = %q，期望字符口径", key, got)
		}
		if strings.Contains(got, "≈") || strings.Contains(got, "tokens") {
			t.Errorf("%s = %q：字符口径不该出现 ≈ / tokens", key, got)
		}
	}
	if got := unitProbe(t, dom, "trajHead2"); got != "字符" {
		t.Errorf("切换后轨迹表计数列表头 = %q，期望 字符", got)
	}
	if got := unitProbe(t, dom, "trajCell2"); !regexp.MustCompile(`^[0-9]+$`).MatchString(got) {
		t.Errorf("切换后轨迹表计数列 = %q，期望纯数字", got)
	}
}
