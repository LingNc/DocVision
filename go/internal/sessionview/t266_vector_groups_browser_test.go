package sessionview

import (
	"path/filepath"
	"strings"
	"testing"
)

// T266：矢量图阶段同一张图的多次尝试（try1/try2/…）以前平铺成一串同号方块，
// 46 条会话里二十多个序号各出现两次，看不出哪些是重画。这里钉住三件事：
// ① 侧栏按"这张图"叠成一层（父行带尝试次数，子行是 tryN + 结局）；
// ② 方块视图一图一块，多次尝试带次数角标；
// ③ 搜索时不分组——历史尝试也必须能被搜到。
const (
	vecHashA = "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbb"
	vecHashB = "bbbb2222cccc3333dddd4444eeee5555ffff6666aaaaaaaabbbbccccdddd0000"
)

func vectorAttemptFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	body := transcript(`{"t":"msg","role":"user","text":"把这张图重画。"}`)
	for _, name := range []string{
		"vector_book__" + vecHashA + "__label.jsonl",
		"vector_book__" + vecHashA + "__label.try2.jsonl",
		"vector_book__" + vecHashA + "__label.try3.jsonl",
		"vector_book__" + vecHashB + "__label.jsonl",
	} {
		writeFile(t, jsonl(root, "proj", "source", "sessions", name), body)
	}
	writeFile(t, filepath.Join(root, "proj", "doc_index", "doc_index.json"),
		`{"entries":[`+
			`{"seq":1,"page":3,"type":"image","img":"images/book/`+vecHashA+`.jpg","text":"第一张图"},`+
			`{"seq":2,"page":5,"type":"image","img":"images/book/`+vecHashB+`.jpg","text":"第二张图"}]}`)
	return root
}

// vectorGroupsProbeJS 依次量：分组行数、父行文案、子行 try 号、方块视图、
// 点开方块视图后再搜一次（应当退回平铺）。
const vectorGroupsProbeJS = `
      var out = [];
      var push = function (k, v) { out.push(k + '=' + String(v).replace(/\s+/g, ' ').trim()); };
      var text = function (el) { return el ? el.textContent.replace(/\s+/g, ' ').trim() : ''; };
      var groups = document.querySelectorAll('.img-group');
      push('groupCount', groups.length);
      push('plainRows', document.querySelectorAll('.session-row[data-id$="@PLAINIMG@"]').length);
      push('firstGroupHead', text(groups[0] ? groups[0].querySelector('summary') : null));
      var tags = [];
      if (groups[0]) {
        Array.prototype.forEach.call(groups[0].querySelectorAll('.try-tag'), function (el) { tags.push(text(el)); });
      }
      push('firstGroupTries', tags.join(','));
      var states = [];
      if (groups[0]) {
        Array.prototype.forEach.call(groups[0].querySelectorAll('.try-state'), function (el) { states.push(text(el)); });
      }
      push('firstGroupStates', states.join(','));
      // 子行平时是收起的：父行 details 的 open 属性应为 false。
      push('firstGroupOpen', groups[0] ? groups[0].hasAttribute('open') : 'none');
      var toggle = document.querySelector('[title^="切到方块视图"]');
      push('hasBlockToggle', !!toggle);
      if (toggle) toggle.click();
      setTimeout(function () {
        push('blockCount', document.querySelectorAll('.session-block').length);
        push('blockBadges', document.querySelectorAll('.block-count').length);
        var back = document.querySelector('[title^="切回列表视图"]');
        if (back) back.click();
        setTimeout(function () {
          var input = document.getElementById('search');
          input.value = 'vector_book';
          input.dispatchEvent(new Event('input', { bubbles: true }));
          setTimeout(function () {
            push('searchGroups', document.querySelectorAll('.img-group').length);
            push('searchRows', document.querySelectorAll('.session-row').length);
            var o = document.createElement('div');
            o.id = 'name-probe';
            o.textContent = out.join(' ;; ');
            document.body.appendChild(o);
          }, 120);
        }, 120);
      }, 120);
`

func TestViewerVectorAttemptGroupsInChromium(t *testing.T) {
	root := vectorAttemptFixture(t)
	js := strings.NewReplacer("@PLAINIMG@", "vector_book__"+vecHashB+"__label.jsonl").Replace(vectorGroupsProbeJS)
	dom := renderViewerDOM(t, root, "vector_book__"+vecHashA+"__label.try3.jsonl", js)

	// 试过三次的那张叠成一层；只试过一次的那张不套壳，仍是普通会话行。
	if got := nameProbeValue(t, dom, "groupCount"); got != "1" {
		t.Errorf("分组行数 = %s，期望 1（只有那张试过三次的图叠层）", got)
	}
	if got := nameProbeValue(t, dom, "plainRows"); got != "1" {
		t.Errorf("只试过一次的图占 %s 行普通行，期望 1（不该多套一层展开点）", got)
	}
	head := nameProbeValue(t, dom, "firstGroupHead")
	if !strings.Contains(head, "3 次") {
		t.Errorf("父行没写尝试次数；实际：%s", head)
	}
	if got := nameProbeValue(t, dom, "firstGroupTries"); got != "try3,try2,try1" {
		t.Errorf("子行尝试号 = %q，期望 try3,try2,try1（新的在前）", got)
	}
	// 当前选中的正是 try3，它所在的那一组应自动展开。
	if got := nameProbeValue(t, dom, "firstGroupOpen"); got != "true" {
		t.Errorf("当前会话所在分组没有自动展开（open=%s）", got)
	}
	if got := nameProbeValue(t, dom, "hasBlockToggle"); got != "true" {
		t.Fatalf("页面里没有方块视图开关，探针点不到")
	}
	if got := nameProbeValue(t, dom, "blockCount"); got != "2" {
		t.Errorf("方块数 = %s，期望 2（一图一块，不再按尝试平铺）", got)
	}
	if got := nameProbeValue(t, dom, "blockBadges"); got != "1" {
		t.Errorf("次数角标 = %s 个，期望 1（只有那张试过三次的图挂角标）", got)
	}
	if got := nameProbeValue(t, dom, "searchGroups"); got != "0" {
		t.Errorf("搜索时仍在分组（img-group=%s），历史尝试会被藏起来", got)
	}
	if got := nameProbeValue(t, dom, "searchRows"); got != "4" {
		t.Errorf("搜索命中的行数 = %s，期望 4（四次尝试都要能搜到）", got)
	}
}
