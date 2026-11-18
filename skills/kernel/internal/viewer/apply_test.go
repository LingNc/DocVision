package viewer

import (
	"os"
	"testing"

	"github.com/LingNc/DocVision/skills/kernel/internal/ids"
)

func TestApplyGroupEdit(t *testing.T) {
	root := t.TempDir()
	tbl, _ := ids.Load(root)
	tbl.Register(ids.KindImg, "sha1", "p/a.png", 1, [4]float64{}, "cls_a")
	tbl.Register(ids.KindImg, "sha2", "p/b.png", 1, [4]float64{}, "cls_b")
	if err := tbl.Save(); err != nil {
		t.Fatal(err)
	}
	// 合法：用户把两张图并成一组
	ov, err := ApplyRegroup(root, RegroupRequest{Type: "group_edit", Groups: []GroupCard{
		{Rep: "img001", Members: []string{"img001", "img002"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Groups) != 1 || ov.Groups[0].Rep != "img001" || len(ov.Groups[0].Members) != 2 {
		t.Fatalf("group_edit 应直接落覆盖: %+v", ov.Groups)
	}
	if _, err := os.Stat(OverridePath(root)); err != nil {
		t.Fatal("覆盖文件应已写出")
	}
	// 非法：代表不在组内
	if _, err := ApplyRegroup(root, RegroupRequest{Type: "group_edit", Groups: []GroupCard{
		{Rep: "img999", Members: []string{"img001", "img002"}},
	}}); err == nil {
		t.Fatal("代表不在组内应拒")
	}
	// 非法：编号不存在
	if _, err := ApplyRegroup(root, RegroupRequest{Type: "group_edit", Groups: []GroupCard{
		{Rep: "img001", Members: []string{"img001", "img777"}},
	}}); err == nil {
		t.Fatal("未知编号应拒")
	}
	// LoadOverride 回读
	ov2, err := LoadOverride(root)
	if err != nil || ov2 == nil || len(ov2.Groups) != 1 {
		t.Fatalf("覆盖文件应可读回: %+v %v", ov2, err)
	}
}
