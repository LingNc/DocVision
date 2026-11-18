package ids

import (
	"path/filepath"
	"testing"
)

func TestRegisterAndIdempotent(t *testing.T) {
	root := t.TempDir()
	tbl, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := tbl.Register(KindImg, "shaA", "source/images/a.png", 12, [4]float64{1, 2, 3, 4}, "cls_a")
	if err != nil {
		t.Fatal(err)
	}
	if r1.ID != "img001" {
		t.Fatalf("首个 img 编号应为 img001: %s", r1.ID)
	}
	// 同 SHA 幂等
	r2, err := tbl.Register(KindImg, "shaA", "source/images/a.png", 12, [4]float64{}, "cls_a")
	if err != nil {
		t.Fatal(err)
	}
	if r2.ID != "img001" || tbl.Seq[KindImg] != 1 {
		t.Fatalf("同 SHA 应共享编号不递增: %s seq=%d", r2.ID, tbl.Seq[KindImg])
	}
	// 不同 SHA 递增
	r3, _ := tbl.Register(KindImg, "shaB", "source/images/b.png", 20, [4]float64{}, "cls_b")
	if r3.ID != "img002" {
		t.Fatalf("应递增 img002: %s", r3.ID)
	}
	// 类别独立序列
	v1, _ := tbl.Register(KindVec, "shaA", "figures/a.tex", 12, [4]float64{}, "drw_a")
	if v1.ID != "vec001" {
		t.Fatalf("vec 序列独立: %s", v1.ID)
	}
	// 重载持久
	tbl2, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := tbl2.Get("img001"); !ok || got.SHA != "shaA" || got.Page != 12 {
		t.Fatalf("重载后映射应在: %+v %v", got, ok)
	}
	if tbl2.Seq[KindImg] != 2 {
		t.Fatalf("重载后序列应保持: %d", tbl2.Seq[KindImg])
	}
}

func TestPageIDAndBySHA(t *testing.T) {
	root := t.TempDir()
	tbl, _ := Load(root)
	tbl.Register(KindImg, "shaX", "p.png", 5, [4]float64{}, "")
	if PageID(137) != "pdfp137" {
		t.Fatalf("页码编号: %s", PageID(137))
	}
	if r := tbl.BySHA("shaX"); r == nil || r.ID != "img001" {
		t.Fatalf("反查失败: %+v", r)
	}
	_ = filepath.Join
}
