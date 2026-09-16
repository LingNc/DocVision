package latex

// 终审产物持久化（T39）：assemble 每次运行都清空重建 build/ 构建树，
// 而终审（finalReview）会话对构建树的整理——章节 .tex 修订、新增的
// frontmatter.tex、main.tex 里的 \input{frontmatter.tex} 钩子——只活在
// 构建树里。一旦进程死在终审完成后的收尾路上（progress.json 没落盘），
// 下一跑会把几十轮终审从头再烧一遍（真实事故：72 轮重烧成 63 轮）。
//
// 解法：终审成功（编译通过）后把产物回写工作区——
//
//	work/chapters/**.tex      ← 构建树里的修订版章节（覆盖转换产物）
//	work/final_review/*.tex   ← main.tex + 终审新增的顶层 .tex（frontmatter 等）
//
//	同时把「work/chapters + work/final_review」的内容指纹写进
//	work/final_review/state.sha256。续跑时指纹一致说明 convert 之后
//	没有任何变化，重建出的树必然与上次终审后的一致，直接沿用持久化
//	状态（applyFinalReviewState 用终审版 main.tex 覆盖生成的骨架），
//	整段跳过终审会话；指纹不一致（convert 重跑过/手改过）则照常重审。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// finalReviewStateDir is the persisted top-level of the final review,
// relative to the project workspace.
const finalReviewStateDir = "final_review"

// stateHashFile is the fingerprint file inside finalReviewStateDir.
const stateHashFile = "state.sha256"

// archivePrevTranscript renames a leftover transcript from a previous run to
// <base>_prev.jsonl so the new run starts a clean transcript while the old
// one stays visible in the preview UI (T39). A previous transcript must not
// be replayed into a rebuilt-from-scratch tree: its edits no longer exist.
func archivePrevTranscript(path string) {
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		return
	}
	prev := strings.TrimSuffix(path, ".jsonl") + "_prev.jsonl"
	_ = os.Remove(prev)
	_ = os.Rename(path, prev)
}

// hashFinalReviewInputs computes the content fingerprint of everything the
// final review could have touched: all chapter .tex files (recursive) plus
// the persisted top-level .tex files.
func hashFinalReviewInputs(proj string) (string, error) {
	h := sha256.New()
	roots := []struct {
		dir  string
		base string
	}{
		{filepath.Join(proj, "work", "chapters"), "chapters"},
		{filepath.Join(proj, "work", finalReviewStateDir), finalReviewStateDir},
	}
	for _, root := range roots {
		var files []string
		err := filepath.WalkDir(root.dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".tex") {
				return nil
			}
			files = append(files, p)
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(files)
		for _, f := range files {
			rel, err := filepath.Rel(root.dir, f)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			io.WriteString(h, root.base+"/"+filepath.ToSlash(rel))
			h.Write([]byte{0})
			h.Write(data)
			h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// persistFinalReviewState writes the final-review session's tree edits back
// into the workspace and records the content fingerprint. Called only after
// the post-review compile passed.
func persistFinalReviewState(proj, buildDir string) error {
	stateDir := filepath.Join(proj, "work", finalReviewStateDir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	// 1) chapter .tex revisions (recursive) → work/chapters/
	chapSrc := filepath.Join(buildDir, "chapters")
	chapDst := filepath.Join(proj, "work", "chapters")
	err := filepath.WalkDir(chapSrc, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".tex") {
			return nil
		}
		rel, err := filepath.Rel(chapSrc, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(chapDst, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return copyFile(p, dst)
	})
	if err != nil {
		return err
	}
	// 2) top-level .tex (main.tex + anything the review added, e.g.
	// frontmatter.tex) → work/final_review/
	entries, err := os.ReadDir(buildDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tex") {
			continue
		}
		if err := copyFile(filepath.Join(buildDir, e.Name()), filepath.Join(stateDir, e.Name())); err != nil {
			return err
		}
	}
	// 3) fingerprint of what we just wrote.
	sum, err := hashFinalReviewInputs(proj)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(stateDir, stateHashFile), []byte(sum+"\n"), 0o644)
}

// finalReviewStateCurrent reports whether the persisted final-review state
// still matches the workspace: the fingerprint stored at persist time equals
// the fingerprint of the current work/chapters + work/final_review. When
// true, a rebuilt tree equals the post-review tree and the review can be
// skipped entirely.
func finalReviewStateCurrent(proj string) bool {
	stored, err := os.ReadFile(filepath.Join(proj, "work", finalReviewStateDir, stateHashFile))
	if err != nil {
		return false
	}
	cur, err := hashFinalReviewInputs(proj)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(stored)) == cur
}

// applyFinalReviewState overlays the persisted final-review top-level files
// (main.tex and friends) onto a freshly built build dir. Chapter files need
// no action: persistFinalReviewState already wrote them into work/chapters,
// which the normal build copy brings along.
func applyFinalReviewState(proj, buildDir string) error {
	stateDir := filepath.Join(proj, "work", finalReviewStateDir)
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tex") {
			continue
		}
		if err := copyFile(filepath.Join(stateDir, e.Name()), filepath.Join(buildDir, e.Name())); err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("work/final_review 里没有可恢复的 .tex")
	}
	return nil
}
