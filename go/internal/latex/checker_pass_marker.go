package latex

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// T260：核对通过之后留个记号，用来识别"更旧的核对反馈"。
//
// 实测事故：这一章的核对已经交了 pass（结论写在自己那条转录里、`.tex.checker`
// 也删了），可另有一条**更早的核对会话**还留着 issues——它一被驱动起来，那串
// 旧问题就按"打回"喂回转换会话，于是刚通过的章节被重做一小时，重做完再开核对，
// 还撞上上游 400 整章判失败。产物一个字没变，问题全是上一次的。
//
// 记号 = 产物指纹 + 记号文件自己的 mtime。判断"旧反馈"要同时满足两条：
// 产物指纹仍与通过时一致（这一版没被改过），且记号比那条核对转录**更新**（通过
// 发生在这次核对之后）。两条都不满足就照旧按 issues 打回——人工「重新核对」在
// 同一份产物上真的看出问题，仍然应该能打回。
const passMarkerSuffix = ".passed"

// productFingerprint 对提交产物取指纹：主 .tex 的内容 + parts 下全部文件（按
// 相对路径排序后依次喂入，路径与内容都参与），这样补一个分片也算产物变了。
func productFingerprint(texPath, partsPath string) string {
	h := sha256.New()
	if b, err := os.ReadFile(texPath); err == nil {
		_, _ = h.Write(b)
	} else {
		return ""
	}
	names := []string{}
	if partsPath != "" {
		_ = filepath.Walk(partsPath, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(partsPath, p)
			if rerr != nil {
				return nil
			}
			names = append(names, filepath.ToSlash(rel))
			return nil
		})
	}
	sort.Strings(names)
	for _, rel := range names {
		_, _ = io.WriteString(h, "\x00"+rel+"\x00")
		if b, err := os.ReadFile(filepath.Join(partsPath, filepath.FromSlash(rel))); err == nil {
			_, _ = h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func passMarkerPath(texPath string) string { return texPath + passMarkerSuffix }

// writePassMarker 记下"这一版产物通过了"。写失败只是少了这层保护，不影响流水线。
func writePassMarker(texPath, fingerprint string) {
	if fingerprint == "" {
		return
	}
	_ = os.WriteFile(passMarkerPath(texPath), []byte(fingerprint+"\n"), 0o644)
}

// dropPassMarker 在产物被判 issues（或产物被替换）时清掉记号。
func dropPassMarker(texPath string) { _ = os.Remove(passMarkerPath(texPath)) }

// staleCheckerFeedback 判断这次拿到的 issues 是不是"更旧的核对反馈"：产物与
// 通过时一致，且通过记号比这条核对转录更新。
func staleCheckerFeedback(texPath, partsPath, checkerTranscript string) bool {
	b, err := os.ReadFile(passMarkerPath(texPath))
	if err != nil {
		return false
	}
	want := strings.TrimSpace(string(b))
	if want == "" || want != productFingerprint(texPath, partsPath) {
		return false
	}
	mi, merr := os.Stat(passMarkerPath(texPath))
	if merr != nil {
		return false
	}
	ti, terr := os.Stat(checkerTranscript)
	if terr != nil {
		// 转录文件认不出来时不敢下结论，按旧反馈处理会白白重做一小时，宁可
		// 相信记号（它本来就是在更晚的时间写下的）。
		return true
	}
	return mi.ModTime().After(ti.ModTime())
}
