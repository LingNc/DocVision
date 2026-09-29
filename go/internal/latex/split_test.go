package latex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// split 工具：逐条校验 + 批量 + 排序落盘 + edit_file 直改可见 + submit 读同一份文件。
func TestSplitToolOps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "split.json")
	sp := NewSplitTool(path)

	// add 非法字段
	res, _ := sp.Execute(`{"op":"add","title":"","start_line":1,"end_line":5}`)
	if !strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("空标题必须拒: %s", res.Text)
	}
	// add 正常两条（乱序插入也要排序落盘）
	if _, err := sp.Execute(`{"op":"add","title":"b","start_line":11,"end_line":20}`); err != nil {
		t.Fatal(err)
	}
	res, _ = sp.Execute(`{"op":"add","title":"a","start_line":1,"end_line":10}`)
	if strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("乱序 add 不该拒: %s", res.Text)
	}
	// 排序：list 里 a 在 b 前
	res, _ = sp.Execute(`{"op":"list"}`)
	if !strings.Contains(res.Text, "1. [1-10] a") {
		t.Fatalf("必须按 start_line 排序: %s", res.Text)
	}
	// 重叠当场拒
	res, _ = sp.Execute(`{"op":"add","title":"x","start_line":5,"end_line":15}`)
	if !strings.Contains(res.Text, "REJECTED") || !strings.Contains(res.Text, "重叠") {
		t.Fatalf("重叠必须当场拒: %s", res.Text)
	}
	// update 改边界
	if res, _ := sp.Execute(`{"op":"update","n":2,"end_line":25}`); strings.Contains(res.Text, "REJECTED") {
		t.Fatalf("合法 update 不该拒: %s", res.Text)
	}
	// submit 读到同一份文件
	sub := &SubmitSplitTool{Root: dir}
	if _, err := sub.Execute(`{}`); err != nil || !sub.Set || len(sub.Chapters) != 2 || sub.Chapters[1].EndLine != 25 {
		t.Fatalf("submit 必须读到工具维护的文件: err=%v set=%v %+v", err, sub.Set, sub.Chapters)
	}
}

// add 批量形（{chapters:[...]}）：一把加（50 章不用 50 次调用）；任一非法整批拒绝。
func TestSplitToolAddMany(t *testing.T) {
	dir := t.TempDir()
	sp := NewSplitTool(filepath.Join(dir, "split.json"))

	bulk := `{"op":"add","chapters":[
		{"title":"a","start_line":1,"end_line":10},
		{"title":"b","start_line":11,"end_line":20},
		{"title":"c","start_line":21,"end_line":30}]}`
	res, err := sp.Execute(bulk)
	if err != nil || strings.Contains(res.Text, "REJECTED") || !strings.Contains(res.Text, "3 章") {
		t.Fatalf("批量加失败: err=%v res=%s", err, res.Text)
	}
	// 批内自重叠 → 整批未动
	bad := `{"op":"add","chapters":[
		{"title":"d","start_line":31,"end_line":40},
		{"title":"e","start_line":35,"end_line":45}]}`
	res, _ = sp.Execute(bad)
	if !strings.Contains(res.Text, "整批未动") {
		t.Fatalf("批内重叠必须整批拒: %s", res.Text)
	}
	res, _ = sp.Execute(`{"op":"list"}`)
	if !strings.Contains(res.Text, "3 章") {
		t.Fatalf("整批拒绝后必须还是 3 条: %s", res.Text)
	}
	// update_many
	res, _ = sp.Execute(`{"op":"update","chapters":[{"n":1,"title":"A2"},{"n":3,"title":"C"}]}`)
	if strings.Contains(res.Text, "REJECTED") || !strings.Contains(res.Text, "1. [1-10] A2") || !strings.Contains(res.Text, "3. [21-30] C") {
		t.Fatalf("批量改失败: %s", res.Text)
	}
}

// edit_file 裸改 split.json 是逃生舱：split 工具每次操作前重读文件，
// 外部改动立刻可见；乱写的 JSON 由 submit 终审兜底拒绝。
func TestSplitToolSeesDirectEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "split.json")
	sp := NewSplitTool(path)
	if _, err := sp.Execute(`{"op":"add","title":"a","start_line":1,"end_line":10}`); err != nil {
		t.Fatal(err)
	}
	// 外部直改（模拟 edit_file/bash 绕过工具）
	if err := os.WriteFile(path, []byte(`{"chapters":[{"title":"z","start_line":100,"end_line":200}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	res, _ := sp.Execute(`{"op":"list"}`)
	if !strings.Contains(res.Text, "z") || strings.Contains(res.Text, "[1-10]") {
		t.Fatalf("工具必须看到外部直改: %s", res.Text)
	}
	// 乱写 JSON → submit 拒
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := &SubmitSplitTool{Root: dir}
	res2, _ := sub.Execute(`{}`)
	if !strings.Contains(res2.Text, "REJECTED") {
		t.Fatalf("坏 JSON 必须被 submit 拒: %s", res2.Text)
	}
}

// delete 批量：{ns:[...]} 一次删多条，越界整批不动。
func TestSplitToolDeleteMany(t *testing.T) {
	dir := t.TempDir()
	sp := NewSplitTool(filepath.Join(dir, "split.json"))
	if _, err := sp.Execute(`{"op":"add","chapters":[{"title":"a","start_line":1,"end_line":10},{"title":"b","start_line":11,"end_line":20},{"title":"c","start_line":21,"end_line":30}]}`); err != nil {
		t.Fatal(err)
	}
	res, _ := sp.Execute(`{"op":"delete","ns":[1,3]}`)
	if !strings.Contains(res.Text, "2 章") && strings.Contains(res.Text, "1. [11-20] b") == false {
		t.Fatalf("批量删失败: %s", res.Text)
	}
	if res, _ := sp.Execute(`{"op":"delete","ns":[9]}`); !strings.Contains(res.Text, "整批未动") {
		t.Fatalf("越界必须整批拒: %s", res.Text)
	}
}
