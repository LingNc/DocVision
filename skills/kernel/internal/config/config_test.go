package config

import (
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndRoundtrip(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	// 用户指定的默认：矢量图重合度 17%
	if cfg.Figure.MinOverlap != 0.17 || cfg.Figure.AcceptMinSubmits != 3 || cfg.Split.PagesPerPart != 100 {
		t.Fatalf("默认值错: %+v", cfg.Figure)
	}
	cfg.Figure.MinOverlap = 0.55
	cfg.Figure.AcceptMinSubmits = 5
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	cfg2, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Figure.MinOverlap != 0.55 || cfg2.Figure.AcceptMinSubmits != 5 {
		t.Fatalf("回写后读回应一致: %+v", cfg2.Figure)
	}
	if _, err := filepath.EvalSymlinks(Path(root)); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptConfigFallsBack(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, &Defaults); err != nil {
		t.Fatal(err)
	}
	if err := writeCorrupt(Path(root)); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err == nil {
		t.Fatal("损坏配置应报错")
	}
	if cfg.Figure.MinOverlap != 0.17 {
		t.Fatal("损坏配置必须回落默认值（坏文件不能绕过门槛）")
	}
}
