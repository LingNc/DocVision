// T37: scanning extra directories alongside the main transcript root.
//
// The img2text pipeline writes its sessions OUTSIDE the latex scan root:
// upgrade-fix sessions live in <finally>/progress_items/mermaid_fix/<书>_<图>/
// and (debug mode) per-image transcripts in <finally>/progress_items/sessions/
// <md>/<图>.jsonl. When preview.img2text is enabled the viewer scans that
// progress_items directory as an extra root and groups what it finds per
// book, numbered and searchable like every other session.
package sessionview

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ScanExtra is one additional directory scanned next to the main root,
// with its own grouping rules.
type ScanExtra struct {
	// Dir is scanned recursively for *.jsonl. Missing directories are
	// skipped silently (the img2text pipeline may not have run yet).
	Dir string
	// Kind selects the grouping rules. "img2text" understands
	// progress_items/{mermaid_fix,sessions}/... layouts.
	Kind string
}

// img2TextGroup tags a scanned session as coming from an img2text extra
// root so the numbering pass can find it.
type img2TextGroup struct {
	key string // stable per-image sort key (full image file name)
}

// scanExtras walks every extra directory and appends matching sessions.
// Unreadable directories are skipped (mirrors the main walk's policy).
func (s *scanner) scanExtras(out []SessionInfo, now time.Time) []SessionInfo {
	for _, ex := range s.extras {
		if ex.Kind != "img2text" {
			continue
		}
		base, err := filepath.Abs(ex.Dir)
		if err != nil {
			continue
		}
		st, err := os.Stat(base)
		if err != nil || !st.IsDir() {
			continue
		}
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if p != base && skipScanDir(p, d.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
				return nil
			}
			info, ierr := d.Info()
			if ierr != nil {
				return nil
			}
			rel, rerr := filepath.Rel(base, p)
			if rerr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			si, g, ok := img2TextSessionInfo(p, rel, d.Name(), info, s.fileStat(p, info.Size(), info.ModTime()), now)
			if !ok {
				return nil
			}
			s.img2Text = append(s.img2Text, img2TextEntry{id: si.ID, key: g})
			out = append(out, si)
			return nil
		})
	}
	return out
}

// img2TextEntry couples an img2text session's ID with its per-image sort
// key. It deliberately holds NO pointer into the sessions slice: appends
// reallocate, and any pointer taken mid-scan would dangle (T37 bug).
type img2TextEntry struct {
	id  string
	key string
}

// img2TextSessionInfo maps one transcript under progress_items/ to a
// SessionInfo with book-level project grouping:
//
//	mermaid_fix/<书>_<图>/session-stageN.jsonl -> 项目 "img2text · <书>"，阶段 升级修复
//	sessions/<md>/<图>.jsonl                   -> 项目 "img2text · <md去扩展名>"，阶段 逐图分析
//
// ok=false means the layout is not one we recognise; such files are
// ignored (never mis-filed into a wrong group).
func img2TextSessionInfo(p, rel, name string, info fs.FileInfo, stat transcriptStat, now time.Time) (SessionInfo, string, bool) {
	segs := strings.Split(rel, "/")
	var project, stage, stageTitle, label, title, imgKey string
	switch {
	case len(segs) == 3 && segs[0] == "mermaid_fix":
		book, img := splitFixDirName(segs[1])
		if book == "" || img == "" {
			return SessionInfo{}, "", false
		}
		short := shortImgName(img)
		project = "img2text · " + book
		// 阶段键与 Label 前缀一致：enrichSessions 会按 Label 重算 Stage，
		// 两边必须说的是同一个键（stages.go StageTitles 有对应中文名）。
		stage, stageTitle = "mermaid-fix", "升级修复"
		label = "mermaid-fix:" + short
		title = "升级修复 · " + short
		imgKey = img
	case len(segs) == 3 && segs[0] == "sessions":
		md := strings.TrimSuffix(segs[1], filepath.Ext(segs[1]))
		if md == "" {
			return SessionInfo{}, "", false
		}
		img := transcriptStem(segs[2])
		short := shortImgName(img)
		project = "img2text · " + md
		stage, stageTitle = "img2text", "逐图分析"
		label = "img2text:" + short
		title = "逐图 · " + short
		imgKey = img
	default:
		return SessionInfo{}, "", false
	}
	return SessionInfo{
		// Prefix the ID so it can never collide with a main-root rel path.
		ID:           "img2text:" + rel,
		Board:        "img2text",
		Label:        label,
		Title:        title,
		Name:         name,
		Path:         p,
		Project:      project,
		Stage:        stage,
		StageTitle:   stageTitle,
		Messages:     stat.Messages,
		Meta:         stat.Meta,
		Stats:        stat.Usage,
		Estimate:     estimateInfoFor(stat),
		Bytes:        info.Size(),
		ModTime:      info.ModTime(),
		SHA:          stat.SHA,
		Live:         liveNow(stat, info.ModTime(), now),
		EndState:     liveEndState(endStateOf(stat), now.Sub(info.ModTime()) < LiveWindow),
		ChapterOrder: -1, // set by the numbering pass below
	}, imgKey, true
}

// splitFixDirName splits "<书>_<图file>" into book and image file. The
// image file is a content hash name without underscores, so the LAST
// underscore is the boundary; a directory without an underscore is not
// a fix workspace we recognise.
func splitFixDirName(dir string) (book, img string) {
	i := strings.LastIndex(dir, "_")
	if i <= 0 || i == len(dir)-1 {
		return "", ""
	}
	return dir[:i], dir[i+1:]
}

// shortImgName renders the image file the way the sidebar's vector
// sessions do (stages.go shortImageName): hash names collapse to the
// first 8 chars + extension; per-group collisions escalate to 12, then
// the full name (handled by numberImg2Text below).
func shortImgName(img string) string {
	base := img
	ext := ""
	if i := strings.LastIndex(img, "."); i > 0 {
		base, ext = img[:i], img[i:]
	}
	return shortImageName(base, ext, imageShortKeep)
}

// numberImg2Text assigns each img2text IMAGE its index inside its project
// (sorted by the full image file name — stable and searchable; the hash
// order is not the book's page order) and prefixes every session title with
// "#N ·". ChapterOrder feeds the block-view badge.
// T77：编号按**图片**走而不是按 项目+阶段——同一张图的逐图分析与升级修复
// 成员共用同一个书内序号。旧法给修复组单独从 #1 起算，而链的外层徽标取
// 最新成员（往往是修复会话）的编号，两张不同的链会同时显示 #1。
// 短名撞车升级（8→12 位→全名）同样按项目内的**去重图片**算——同图的
// 分析与修复成员本来就该显示同一个短名。
func (s *scanner) numberImg2Text(out []SessionInfo) {
	if len(s.img2Text) == 0 {
		return
	}
	idxByID := map[string]int{}
	for i := range out {
		idxByID[out[i].ID] = i
	}
	projOf := map[string]string{}          // 会话 ID → 项目
	keyOf := map[string]string{}           // 会话 ID → 图片键
	imgsOf := map[string]map[string]bool{} // 项目 → 去重图片键
	members := map[string][]string{}       // 项目\x00图片 → 会话 ID（保序）
	for _, e := range s.img2Text {
		i, ok := idxByID[e.id]
		if !ok {
			continue
		}
		proj := out[i].Project
		projOf[e.id] = proj
		keyOf[e.id] = e.key
		if imgsOf[proj] == nil {
			imgsOf[proj] = map[string]bool{}
		}
		imgsOf[proj][e.key] = true
		k := proj + "\x00" + e.key
		members[k] = append(members[k], e.id)
	}
	shortAt := func(key string, keep int) string {
		base := key
		ext := ""
		if i := strings.LastIndex(key, "."); i > 0 {
			base, ext = key[:i], key[i:]
		}
		return shortImageName(base, ext, keep)
	}
	for proj, set := range imgsOf {
		imgs := make([]string, 0, len(set))
		for k := range set {
			imgs = append(imgs, k)
		}
		sort.Strings(imgs)
		// 8 位短名撞车的升级 12 位、再撞写全名（与矢量图会话同一套
		// 词汇，见 stages.go assignShortImageNames）；粒度 = 项目内去重图片。
		keepOf := map[string]int{}
		for _, k := range imgs {
			keepOf[k] = imageShortKeep
		}
		for keep := imageShortKeep; keep <= imageShortFallbackKeep; keep += imageShortFallbackKeep - imageShortKeep {
			counts := map[string]int{}
			for _, k := range imgs {
				counts[shortAt(k, keep)]++
			}
			collided := false
			for _, k := range imgs {
				if counts[shortAt(k, keep)] > 1 {
					keepOf[k] = keep + (imageShortFallbackKeep - imageShortKeep)
					collided = true
				}
			}
			if !collided {
				break
			}
		}
		for n, k := range imgs {
			for _, id := range members[proj+"\x00"+k] {
				i := idxByID[id]
				out[i].ChapterOrder = n + 1
				keep := keepOf[k]
				if keep < imageShortKeep {
					keep = imageShortKeep
				}
				name := shortAt(k, keep)
				if out[i].Stage == "mermaid-fix" {
					name = "升级修复 · " + name
				} else {
					name = "逐图 · " + name
				}
				out[i].Title = fmt.Sprintf("#%d · %s", n+1, name)
				if keep > imageShortKeep {
					out[i].ImgShortKeep = keep
				}
			}
		}
	}
}
