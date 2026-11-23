package luadraw

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

func (d *Docs) rootPath(name string) (string, string, error) {
	root := d.root
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = abs(real)
	}
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		name = "."
	}
	native := filepath.FromSlash(name)
	windowsDrive := len(name) >= 2 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && name[1] == ':'
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" || strings.HasPrefix(name, "\\") || strings.HasPrefix(name, "/") || windowsDrive {
		return "", "", fmt.Errorf("document path must be relative to LuaDraw root")
	}
	for _, segment := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return "", "", fmt.Errorf("document path must not contain '..' segments")
		}
	}
	p := abs(filepath.Join(root, native))
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("document path must stay inside LuaDraw root")
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		real = abs(real)
		r, e := filepath.Rel(root, real)
		if e != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("document path must stay inside LuaDraw root")
		}
		rel = r
		p = real
	}
	return p, filepath.ToSlash(rel), nil
}

type sourceParagraph struct {
	Start int
	End   int
	Text  string
}

func sourceParagraphs(text string) []sourceParagraph {
	lines := strings.Split(text, "\n")
	const maxParagraphLines = 80
	const maxParagraphBytes = 12000
	var out []sourceParagraph
	for i := 0; i < len(lines); {
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
		if i >= len(lines) {
			break
		}
		start := i
		size := 0
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			lineSize := len(lines[i]) + 1
			if i > start && (i-start >= maxParagraphLines || size+lineSize > maxParagraphBytes) {
				break
			}
			size += lineSize
			i++
		}
		out = append(out, sourceParagraph{Start: start + 1, End: i, Text: strings.Join(lines[start:i], "\n")})
	}
	return out
}

func isTextFile(path string, data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func (d *Docs) sourcePath(name string) (string, string, []byte, error) {
	p, rel, err := d.rootPath(name)
	if err != nil {
		return "", "", nil, err
	}
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return "", "", nil, d.pathSuggestion(rel)
	}
	if st.Size() > 16<<20 {
		return "", "", nil, fmt.Errorf("source file exceeds the 16 MiB read limit: %s", rel)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", "", nil, err
	}
	if !isTextFile(p, data) {
		return "", "", nil, fmt.Errorf("document is binary or invalid UTF-8: %s", rel)
	}
	return p, rel, data, nil
}

type pathCandidate struct {
	path  string
	score float64
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func min3(a, b, c int) int { return minInt(minInt(a, b), c) }

func editDistance(a, b string) int {
	x, y := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	prev := make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ca := range x {
		curr := make([]int, len(y)+1)
		curr[0] = i + 1
		for j, cb := range y {
			cost := 0
			if ca != cb {
				cost = 1
			}
			curr[j+1] = min3(curr[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = curr
	}
	return prev[len(y)]
}

func commonPrefixRunes(a, b string) int {
	x, y := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	n := minInt(len(x), len(y))
	for i := 0; i < n; i++ {
		if x[i] != y[i] {
			return i
		}
	}
	return n
}

func (d *Docs) pathSuggestion(rel string) error {
	requestedBase := strings.ToLower(filepath.Base(rel))
	requestedDir := filepath.ToSlash(filepath.Dir(rel))
	var candidates []pathCandidate
	visited := 0
	_ = filepath.WalkDir(d.root, func(path string, ent fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ent.IsDir() {
			return nil
		}
		if ent.Type()&os.ModeSymlink != 0 {
			return nil
		}
		visited++
		if visited > 10000 {
			return fs.SkipAll
		}
		base := ent.Name()
		r, e := filepath.Rel(d.root, path)
		if e != nil {
			return nil
		}
		relPath := filepath.ToSlash(r)
		maxLen := maxInt(len([]rune(requestedBase)), len([]rune(base)))
		if maxLen == 0 {
			return nil
		}
		similarity := 1 - float64(editDistance(requestedBase, base))/float64(maxLen)
		prefix := float64(commonPrefixRunes(requestedBase, base)) / float64(maxLen)
		score := similarity + prefix*.35
		if strings.EqualFold(requestedBase, base) {
			score += 2
		}
		if strings.EqualFold(requestedDir, filepath.ToSlash(filepath.Dir(relPath))) {
			score += .8
		}
		candidates = append(candidates, pathCandidate{path: relPath, score: score})
		return nil
	})
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].path < candidates[j].path
	})
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	paths := make([]string, len(candidates))
	for i := range candidates {
		paths[i] = candidates[i].path
	}
	parent := requestedDir
	if parent == "." {
		parent = ""
	}
	if len(paths) > 0 {
		return fmt.Errorf("document not found under LuaDraw root: %s; closest actual paths: %s; use mode=list with doc=%q to inspect that directory", rel, strings.Join(paths, ", "), parent)
	}
	return fmt.Errorf("document not found under LuaDraw root: %s; use mode=list with doc=%q or mode=find to locate an indexed path", rel, parent)
}

func (d *Docs) listFiles(name string, recursive bool) ([]string, error) {
	p, rel, err := d.rootPath(name)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("document directory not found under root: %s", rel)
	}
	var out []string
	add := func(path string, ent fs.DirEntry) error {
		if ent.Type()&os.ModeSymlink != 0 {
			return nil
		}
		r, err := filepath.Rel(d.root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(r)
		if ent.IsDir() {
			name += "/"
		}
		out = append(out, name)
		return nil
	}
	if !recursive {
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		for _, ent := range entries {
			if err := add(filepath.Join(p, ent.Name()), ent); err != nil {
				return nil, err
			}
		}
	} else {
		visited := 0
		err = filepath.WalkDir(p, func(path string, ent fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if path == p {
				return nil
			}
			visited++
			if visited > 10000 {
				return fs.SkipAll
			}
			if ent.Type()&os.ModeSymlink != 0 {
				return nil
			}
			return add(path, ent)
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func (d *Docs) findFiles(scope, pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("find requires query")
	}
	p, _, err := d.rootPath(scope)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("find scope does not exist under root: %s", scope)
	}
	var out []string
	visit := 0
	match := func(path string, ent fs.DirEntry) error {
		if ent.IsDir() || ent.Type()&os.ModeSymlink != 0 {
			return nil
		}
		visit++
		if visit > 10000 {
			return fs.SkipAll
		}
		rel, e := filepath.Rel(d.root, path)
		if e != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		matched, e := filepath.Match(pattern, filepath.Base(path))
		if e != nil {
			return e
		}
		if !matched && strings.ContainsAny(pattern, "/\\\\") {
			matched, e = filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel))
			if e != nil {
				return e
			}
		}
		if matched || strings.Contains(strings.ToLower(rel), strings.ToLower(pattern)) {
			out = append(out, rel)
		}
		return nil
	}
	if st.IsDir() {
		err = filepath.WalkDir(p, func(path string, ent fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			return match(path, ent)
		})
	} else {
		ent, e := os.Lstat(p)
		if e == nil {
			err = match(p, fs.FileInfoToDirEntry(ent))
		}
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func (d *Docs) grepFiles(scope, query string, regex bool, limit, offset, maxChars int) (string, error) {
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("grep requires query")
	}
	p, _, err := d.rootPath(scope)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("grep scope does not exist under root: %s", scope)
	}
	var re *regexp.Regexp
	if regex {
		re, err = regexp.Compile(query)
		if err != nil {
			return "", fmt.Errorf("invalid grep regex: %w", err)
		}
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	rows := []string{}
	const maxFiles, maxFileBytes, maxTotalBytes, maxMatches = 1000, 4 << 20, 32 << 20, 10000
	files, totalBytes, stopped := 0, int64(0), false
	check := func(path string, ent fs.DirEntry) error {
		if ent.IsDir() || ent.Type()&os.ModeSymlink != 0 {
			return nil
		}
		files++
		if files > maxFiles {
			stopped = true
			return fs.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".pdf" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			return nil
		}
		info, e := ent.Info()
		if e != nil || info.Size() < 0 || info.Size() > maxFileBytes {
			return nil
		}
		if totalBytes+info.Size() > maxTotalBytes {
			stopped = true
			return fs.SkipAll
		}
		data, e := os.ReadFile(path)
		if e != nil || !isTextFile(path, data) {
			return nil
		}
		totalBytes += int64(len(data))
		rel, _ := filepath.Rel(d.root, path)
		for n, line := range strings.Split(string(data), "\n") {
			ok := (re != nil && re.MatchString(line)) || (re == nil && strings.Contains(line, query))
			if !ok {
				continue
			}
			rows = append(rows, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), n+1, truncate(strings.TrimSpace(line), 800)))
			if len(rows) >= maxMatches {
				stopped = true
				return fs.SkipAll
			}
		}
		return nil
	}
	if st.IsDir() {
		err = filepath.WalkDir(p, func(path string, ent fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			return check(path, ent)
		})
	} else {
		ent, e := os.Lstat(p)
		if e == nil {
			err = check(p, fs.FileInfoToDirEntry(ent))
		}
	}
	if err != nil {
		return "", err
	}
	sort.Strings(rows)
	if stopped {
		rows = append(rows, "[scan budget reached; narrow doc scope]")
	}
	if len(rows) == 0 {
		return "No matches.", nil
	}
	return navigationRows(rows, limit, offset, maxChars), nil
}

func navigationRows(rows []string, limit, offset, maxChars int) string {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	if maxChars <= 0 {
		maxChars = 6000
	}
	if maxChars < 64 {
		maxChars = 64
	}
	if maxChars > 32000 {
		maxChars = 32000
	}
	if offset >= len(rows) {
		return "No matches."
	}
	end := minInt(len(rows), offset+limit)
	var out strings.Builder
	used := 0
	for _, row := range rows[offset:end] {
		line := row
		if out.Len() > 0 {
			line = "\n" + line
		}
		remaining := maxChars - used
		if remaining <= 0 {
			break
		}
		runes := []rune(line)
		if len(runes) > remaining {
			if remaining > 1 {
				line = string(runes[:remaining-1]) + "…"
			} else {
				line = string(runes[:remaining])
			}
			out.WriteString(line)
			used = maxChars
			break
		}
		out.WriteString(line)
		used += len(runes)
	}
	if end < len(rows) && used < maxChars {
		more := fmt.Sprintf("\nMore results; continue with offset=%d.", end)
		remaining := maxChars - used
		if len([]rune(more)) <= remaining {
			out.WriteString(more)
		}
	}
	return out.String()
}
