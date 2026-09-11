package split

import (
	"os"
	"path/filepath"
	"testing"
)

// 同一个文件在 Windows 上是 "files\book.pdf"、在 Linux 上是
// "files/book.pdf"：manifest 的缓存键必须两边一致，否则用另一个平台
// 跑同一份目录时整库重新切分（真实事故：Windows 写下的 manifest 在
// Linux 全都 miss，反之亦然）。
func TestManifestMatchesAcrossPathSeparators(t *testing.T) {
	base := MatchParams{SourceSize: 100, SourceMTime: 42, MaxPages: 200, MaxSizeMB: 0}

	cases := []struct {
		name     string
		recorded string
		asked    string
		want     bool
	}{
		{"windows 记录 / POSIX 查询", `files\book.pdf`, "files/book.pdf", true},
		{"POSIX 记录 / windows 查询", "files/book.pdf", `files\book.pdf`, true},
		{"带 ./ 前缀", "files/book.pdf", "./files/book.pdf", true},
		{"重复斜杠", "files/book.pdf", "files//book.pdf", true},
		{"不同文件", "files/book.pdf", "files/other.pdf", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &Manifest{
				SchemaVersion: ManifestSchemaVersion,
				Kind:          KindPDF,
				Mode:          ModeSplit,
				SourcePath:    tc.recorded,
				SourceSize:    base.SourceSize,
				SourceMTimeNS: base.SourceMTime,
				MaxPages:      base.MaxPages,
				Status:        "ok",
			}
			p := base
			p.SourcePath = tc.asked
			if got := m.Matches(p); got != tc.want {
				t.Fatalf("Matches(%q vs %q) = %v, want %v", tc.recorded, tc.asked, got, tc.want)
			}
		})
	}
}

// 归一化后的键才是落盘内容：Windows 侧以后再写的 manifest 也带 "/"，
// 于是 Linux 侧不用靠比较期兜底也能命中。
func TestManifestNormalizesSourcePathOnWrite(t *testing.T) {
	got := normalizeSourcePath(`files\27考研 红宝书[解析].pdf`)
	want := "files/27考研 红宝书[解析].pdf"
	if got != want {
		t.Fatalf("normalizeSourcePath = %q, want %q", got, want)
	}
	if got := normalizeSourcePath("./files//a.pdf"); got != "files/a.pdf" {
		t.Fatalf("./ 与重复斜杠未折叠: %q", got)
	}
}

// 真实场景：Windows 侧写下的 manifest 文件（source_path 里是反斜杠）
// 被 Linux 侧读到，必须算缓存命中——部署目录里就有这样的文件。
func TestWindowsWrittenManifestHitsOnPOSIX(t *testing.T) {
	dir := t.TempDir()
	base := "2010-26年数一真题套卷[解析]"
	part := base + "_part1.pdf"
	if err := os.WriteFile(filepath.Join(dir, part), []byte("part-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := &Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Kind:          KindPDF,
		Mode:          ModeSplit,
		SourcePath:    `files\` + base + ".pdf", // Windows 侧的原样记录
		SourceSize:    184496581,
		SourceMTimeNS: 1788440981906804900,
		MaxPages:      200,
		Status:        "ok",
		Parts:         []Part{{Filename: part, Index: 1, PageStart: 1, PageEnd: 200, Size: 6}},
	}
	path := ManifestPath(dir, base)
	if err := WriteManifest(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	p := MatchParams{
		SourcePath:  "files/" + base + ".pdf", // Linux 侧的写法
		SourceSize:  184496581,
		SourceMTime: 1788440981906804900,
		MaxPages:    200,
	}
	if !loaded.Matches(p) {
		t.Fatal("Windows 写下的 manifest 在 POSIX 侧未命中（跨平台缓存仍然失效）")
	}
	if ok, err := loaded.VerifyAgainstDisk(dir); err != nil || !ok {
		t.Fatalf("VerifyAgainstDisk = %v, %v", ok, err)
	}
}
