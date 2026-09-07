package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigPathPrefersLocal(t *testing.T) {
	tmp := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("mineru: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, source, created, err := ResolveConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if source != "local" || created {
		t.Fatalf("source=%s created=%v path=%s", source, created, path)
	}
	if !strings.HasSuffix(path, "config.yaml") {
		t.Fatalf("path=%s", path)
	}
}

func TestResolveConfigPathCreatesGlobal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	path, source, created, err := ResolveConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if source != "global" || !created {
		t.Fatalf("source=%s created=%v", source, created)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("global config missing: %v", err)
	}
	// Second resolve must not re-create.
	_, _, created2, err := ResolveConfigPath("")
	if err != nil || created2 {
		t.Fatalf("created2=%v err=%v", created2, err)
	}
}

func TestValidateDataUnknownKey(t *testing.T) {
	data := []byte("mineru:\n  bogus_key: 1\n")
	problems := ValidateData(data)
	if len(problems) == 0 {
		t.Fatal("unknown key not reported")
	}
	found := false
	for _, p := range problems {
		if strings.Contains(p, "bogus_key") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems: %v", problems)
	}
}

func TestValidateDataBadType(t *testing.T) {
	data := []byte("options:\n  concurrency: not-a-number\n")
	problems := ValidateData(data)
	if len(problems) == 0 {
		t.Fatal("bad type not reported")
	}
}

func TestValidateDataSemantic(t *testing.T) {
	data := []byte("config_version: 2\nmodels:\n  text:\n    base_url: \"https://x/v1\"\n    api_key: \"YOUR-X\"\n    model: \"m\"\nmineru:\n  token: \"YOUR-X\"\n")
	problems := ValidateData(data)
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "占位符") || !strings.Contains(joined, "models.text.api_key") {
		t.Fatalf("semantic problems: %v", problems)
	}
}

func TestValidateDataSyntaxError(t *testing.T) {
	problems := ValidateData([]byte("bad: [\n"))
	if len(problems) != 1 || !strings.Contains(problems[0], "YAML 语法错误") {
		t.Fatalf("problems: %v", problems)
	}
}

func TestValidateDataEmptyConfigReportsRequired(t *testing.T) {
	problems := ValidateData([]byte("{}"))
	if len(problems) == 0 {
		t.Fatal("empty config should report required fields")
	}
}
