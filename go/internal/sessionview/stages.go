package sessionview

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Stage metadata for the sidebar's second level (project → stage → sessions)
// and for finding a level-2 per-image session by image name, PDF page, order or
// image type instead of by its opaque transcript file name.

// StageOrder is the pipeline order used to sort stage groups; a stage that is
// not listed sorts last (alphabetically), so an unknown session file can never
// push itself to the top of a book's sidebar.
var StageOrder = []string{"vector", "style", "chapters", "convert", "checker", "style-fix", "figure-check"}

// StageTitles maps a stage key to its short Chinese display name.
var StageTitles = map[string]string{
	"vector":       "矢量图",
	"style":        "样式",
	"chapters":     "章节划分",
	"convert":      "章节转换",
	"checker":      "章节核对",
	"style-fix":    "样式修复",
	"figure-check": "逐图校验",
	"session":      "其他会话",
}

// StageTitle renders a stage key for the sidebar.
func StageTitle(stage string) string {
	if t, ok := StageTitles[stage]; ok {
		return t
	}
	if stage == "" {
		return StageTitles["session"]
	}
	return stage
}

// StageRank gives the sort position of a stage (unknown stages last).
func StageRank(stage string) int {
	for i, s := range StageOrder {
		if s == stage {
			return i
		}
	}
	return len(StageOrder)
}

// StageSummary is one sidebar stage group.
type StageSummary struct {
	Stage    string `json:"stage"`
	Title    string `json:"title"`
	Sessions int    `json:"sessions"`
	Live     int    `json:"live"`
	// Index is 1-based position in the pipeline order (0 = unknown stage).
	Index int `json:"index,omitempty"`
}

// SummarizeStages counts the sessions of each stage for one project, in
// pipeline order.
func SummarizeStages(sessions []SessionInfo, project string) []StageSummary {
	byStage := map[string]*StageSummary{}
	for _, s := range sessions {
		if s.Project != project {
			continue
		}
		stage := StageOf(s)
		row, ok := byStage[stage]
		if !ok {
			row = &StageSummary{Stage: stage, Title: StageTitle(stage)}
			byStage[stage] = row
		}
		row.Sessions++
		if s.Live {
			row.Live++
		}
	}
	out := make([]StageSummary, 0, len(byStage))
	for _, row := range byStage {
		row.Index = StageRank(row.Stage) + 1
		out = append(out, *row)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := StageRank(out[i].Stage), StageRank(out[j].Stage)
		if ri != rj {
			return ri < rj
		}
		return out[i].Stage < out[j].Stage
	})
	return out
}

// docIndexImage is the slice of doc_index.json this package needs: one entry
// per image block with its page, caption and the original image file.
type docIndexImage struct {
	Page  int    `json:"page"`
	Type  string `json:"type"`
	Text  string `json:"text"`
	Img   string `json:"img"`
	Label string `json:"label"`
}

// projectFacts is everything the sidebar can say about a project beyond the
// transcripts themselves: how far the pipeline got (progress.json) and which
// image each per-image session belongs to (doc_index.json).
type projectFacts struct {
	// Stages is progress.json rendered as stage → status ("done", "running",
	// "…"), which is what "显示当前进展/完成情况" means at book level.
	Stages map[string]string
	// Images maps an image file's base name (sans extension — MinerU names
	// extracted images by content hash) to its doc_index entry.
	Images map[string]docIndexImage
	// Order maps an image base name to its 1-based position in the book.
	Order map[string]int
}

// loadProjectFacts reads the two files the sidebar enrichment needs. Both are
// optional: a project that never reached those stages simply has no facts.
func loadProjectFacts(projDir string) projectFacts {
	facts := projectFacts{Stages: map[string]string{}, Images: map[string]docIndexImage{}, Order: map[string]int{}}
	// progress.json: values are free-form (stage → status string, or nested
	// objects for the per-image level). Only strings are surfaced; anything
	// else is rendered as its JSON so a status is never silently dropped.
	if data, err := os.ReadFile(filepath.Join(projDir, "progress.json")); err == nil {
		var raw map[string]any
		if json.Unmarshal(data, &raw) == nil {
			for k, v := range raw {
				switch tv := v.(type) {
				case string:
					facts.Stages[k] = tv
				case nil:
				default:
					if b, err := json.Marshal(tv); err == nil {
						facts.Stages[k] = string(b)
					}
				}
			}
		}
	}
	// doc_index.json: one entry per block; the image blocks carry img/page/type.
	for _, rel := range []string{filepath.Join("doc_index", "doc_index.json"), "doc_index.json"} {
		data, err := os.ReadFile(filepath.Join(projDir, rel))
		if err != nil {
			continue
		}
		var idx struct {
			Entries []docIndexImage `json:"entries"`
		}
		if json.Unmarshal(data, &idx) != nil {
			continue
		}
		n := 0
		for _, e := range idx.Entries {
			if strings.TrimSpace(e.Img) == "" {
				continue
			}
			base := imageBase(e.Img)
			if base == "" {
				continue
			}
			n++
			if _, seen := facts.Images[base]; !seen {
				facts.Images[base] = e
				facts.Order[base] = n
			}
		}
		break
	}
	return facts
}

// imageBase strips the directory and extension from a doc_index img path
// ("images/<book>/<sha>.jpg" → "<sha>").
func imageBase(img string) string {
	base := path.Base(strings.ReplaceAll(img, "\\", "/"))
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return base
}

// projectDirFor returns the directory that holds a session's project (the
// workspace with work/ and source/ inside it), relative to the scan root.
// "." means the session sits directly under the root.
func projectDirFor(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i, p := range parts {
		if p == "work" || p == "source" || p == "doc_index" {
			if i == 0 {
				return "."
			}
			return strings.Join(parts[:i], "/")
		}
	}
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "/")
	}
	return "."
}

// enrichSessions fills the sidebar fields that are not in the transcript's own
// name: the pipeline stage (grouping), the book's progress and — for level-2
// per-image sessions — which image the session drew (name, PDF page, order,
// type). A vector transcript is named
// vector_<book>__<imagebase>__<label>, so the image base name is right there.
func (s *scanner) enrichSessions(root string, sessions []SessionInfo) {
	factsByDir := map[string]projectFacts{}
	facts := func(dir string) projectFacts {
		if f, ok := factsByDir[dir]; ok {
			return f
		}
		f := loadProjectFacts(filepath.Join(root, filepath.FromSlash(dir)))
		factsByDir[dir] = f
		return f
	}
	for i := range sessions {
		stage := StageOf(sessions[i])
		sessions[i].Stage = stage
		sessions[i].StageTitle = StageTitle(stage)
		dir := projectDirFor(sessions[i].ID)
		f := facts(dir)
		if len(f.Stages) > 0 {
			sessions[i].ProjectStages = f.Stages
		}
		if stage != "vector" && stage != "figure-check" {
			continue
		}
		base := vectorImageBase(sessions[i].ID)
		if base == "" {
			continue
		}
		sessions[i].ImageName = base
		if e, ok := f.Images[base]; ok {
			sessions[i].Page = e.Page
			sessions[i].ImageType = imageTypeOf(e)
			sessions[i].ImageOrder = f.Order[base]
			if cap := strings.TrimSpace(e.Text); cap != "" {
				sessions[i].ImageCaption = cap
			}
		}
	}
}

// imageTypeOf names the image kind: the DOCVISION marker when the markdown
// carried one (styled-text / vector / image), else the block type or "image".
func imageTypeOf(e docIndexImage) string {
	if e.Label != "" && e.Type == "" {
		return "image"
	}
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "image", "":
		return "image"
	case "table", "chart":
		return e.Type
	default:
		return e.Type
	}
}

// vectorImageBase extracts the image base name from a per-image transcript path:
// <proj>/source/sessions/vector_<book>__<imagebase>__<label>.jsonl, or the
// figure-check transcript that names the same image
// (work/sessions/figure_check_<book>__<imagebase>__<label>.jsonl).
func vectorImageBase(rel string) string {
	stem := transcriptStem(rel)
	stem = strings.TrimPrefix(stem, "vector_")
	stem = strings.TrimPrefix(stem, "figure_check_")
	parts := strings.Split(stem, "__")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-2]
}
