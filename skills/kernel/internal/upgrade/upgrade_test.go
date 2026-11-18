package upgrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestCheckUpgradeAvailable(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/repos/LingNc/DocVision/commits/feat/skills", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"sha": "aaaabbbbccccdddd111122223333444455556666"})
	})
	mux.HandleFunc("/repos/LingNc/DocVision/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("path") == "skills" {
			json.NewEncoder(w).Encode([]any{map[string]any{"sha": "dddd1111222233334444555566667777888899990000"}})
		}
	})
	mux.HandleFunc("/repos/LingNc/DocVision/compare/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "behind", "behind_by": 3})
	})

	u := Upstream{Repo: "LingNc/DocVision", Branch: "feat/skills", SkillsPath: "skills", APITemplate: srv.URL}
	res, err := Check(u, "", "", "1111222233334444555566667777888899990000")
	if err != nil {
		t.Fatal(err)
	}
	if !res.UpgradeNeeded || res.Behind != 3 || !res.SkillsChanged {
		t.Fatalf("应检出升级: %+v", res)
	}
}

func TestCheckUpToDate(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	const sha = "aaaabbbbccccdddd111122223333444455556666"
	mux.HandleFunc("/repos/LingNc/DocVision/commits/feat/skills", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"sha": sha})
	})
	mux.HandleFunc("/repos/LingNc/DocVision/commits", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]any{map[string]any{"sha": sha}})
	})
	mux.HandleFunc("/repos/LingNc/DocVision/compare/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "identical", "behind_by": 0})
	})
	u := Upstream{APITemplate: srv.URL}
	res, _ := Check(u, "", "", "aaaabbbbcccc")
	if res.UpgradeNeeded {
		t.Fatalf("应判定最新: %+v", res)
	}
}

func TestVersionFileFallback(t *testing.T) {
	dir := t.TempDir()
	vf := VersionFilePath(dir)
	if err := writeTestFile(vf, "abc123def456\n"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	sux := httptest.NewServer(mux)
	defer sux.Close()
	mux.HandleFunc("/repos/LingNc/DocVision/commits/feat/skills", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"sha": "abc123def456aaaabbbbcccc"})
	})
	u := Upstream{APITemplate: sux.URL}
	res, _ := Check(u, "", vf, "")
	if res.LocalRef != "abc123def456" || res.Source != "version-file" {
		t.Fatalf("安装包模式应读 .skill-version: %+v", res)
	}
}

func writeTestFile(p, body string) error {
	return os.WriteFile(p, []byte(body), 0o644)
}
