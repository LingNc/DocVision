package latex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mineru-tools/internal/config"
	"mineru-tools/internal/session"
)

// pilotTestProject 造一个能进风格试点的最小项目：source/ 下有一份正文 md。
func pilotTestProject(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	src := filepath.Join(proj, "source")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "book.md"), []byte("# 一\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj
}

// pilotRunner 把选材模型指向一个假服务端（非流式），返回 Runner 与项目目录。
func pilotRunner(t *testing.T, handler http.HandlerFunc) (*Runner, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	off := false
	r := keepTestRunner(t)
	r.cfg.Latex.ChapterModel = "chapters"
	r.cfg.Latex.PilotSelectModel = ""
	r.cfg.Models = map[string]config.ModelConfig{
		"chapters": {BaseURL: srv.URL, APIKey: "k", Model: "m", Stream: &off},
	}
	r.clients = map[string]*session.Client{}
	r.models = map[string]config.ModelConfig{}
	proj := pilotTestProject(t)
	r.projDir = proj
	return r, proj
}

// T288：试点会话被接口掐断（上游流式错误）时必须**中止整轮**，不能像以前那样
// 只打一句"跳过试点"就进划分与转换——真实事故里那一轮接着把全下游跑成了废产物。
func TestPilotPhaseAbortsOnSessionError(t *testing.T) {
	r, proj := pilotRunner(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Upstream request failed, please try again later", "type": "upstream_error"},
		})
	})
	err := r.stylePilotPhase(proj)
	if err == nil {
		t.Fatal("接口级失败必须中止整轮，而不是只跳过试点")
	}
	if !strings.Contains(err.Error(), "样式试点") {
		t.Fatalf("错误要说明是哪一步中止的: %v", err)
	}
	if fileExists(pilotMarkerPath(proj)) {
		t.Fatal("中止时不写 marker：下次运行要重新试点")
	}
}

// 内容级失败（会话跑完却没交出选材结果）仍旧只跳过试点：那不挡后续阶段。
func TestPilotPhaseSkipsOnContentFailure(t *testing.T) {
	r, proj := pilotRunner(t, func(w http.ResponseWriter, req *http.Request) {
		jsonReply(w, "I looked at the book but found nothing worth sampling.")
	})
	if err := r.stylePilotPhase(proj); err != nil {
		t.Fatalf("内容级失败不该中止整轮: %v", err)
	}
	if !fileExists(pilotMarkerPath(proj)) {
		t.Fatal("跳过试点要写 marker，免得下次又白跑一遍")
	}
}
