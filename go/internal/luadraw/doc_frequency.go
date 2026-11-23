package luadraw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type frequencyEntry struct {
	ID      string `json:"id"`
	Doc     string `json:"doc"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Text    string `json:"text"`
	Hits    int    `json:"hits"`
	Source  string `json:"source,omitempty"`
	Updated int64  `json:"updated"`
}
type frequencyStore struct {
	Fingerprint string           `json:"fingerprint"`
	Entries     []frequencyEntry `json:"entries"`
}

func frequencyID(doc string, start, end int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", doc, start, end)))
	return hex.EncodeToString(h[:])
}

func (d *Docs) frequencyPath() string {
	return filepath.Join(d.cache, "luadraw-doc-frequency-v1.json")
}
func (d *Docs) frequencyLock() string { return d.frequencyPath() + ".lock" }
func (d *Docs) acquireFrequencyLock() (func(), error) {
	path := d.frequencyLock()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.Mkdir(path, 0700)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if st, e := os.Stat(path); e == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("frequency cache lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func (d *Docs) withFrequencyLock(fn func(*frequencyStore) error) error {
	d.freqMu.Lock()
	defer d.freqMu.Unlock()
	release, err := d.acquireFrequencyLock()
	if err != nil {
		return err
	}
	defer release()
	var st frequencyStore
	if b, e := os.ReadFile(d.frequencyPath()); e == nil {
		_ = json.Unmarshal(b, &st)
	}
	if err := fn(&st); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(d.cache, "luadraw-frequency-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	return os.Rename(name, d.frequencyPath())
}
func (d *Docs) recordFrequency(fingerprint, doc string, start, end int, text, source string, hits int) {
	text = truncate(strings.TrimSpace(text), 2048)
	if text == "" {
		return
	}
	if hits < 1 {
		hits = 1
	}
	_ = d.withFrequencyLock(func(st *frequencyStore) error {
		if st.Fingerprint != fingerprint {
			st.Fingerprint = fingerprint
			st.Entries = nil
		}
		for i := range st.Entries {
			e := &st.Entries[i]
			if e.ID == "" {
				e.ID = frequencyID(e.Doc, e.Start, e.End)
			}
			if e.ID == frequencyID(doc, start, end) || (!strings.HasSuffix(strings.ToLower(e.Doc), ".pdf") && e.Doc == doc && e.Start <= end && start <= e.End) {
				e.Hits += hits
				e.Updated = time.Now().Unix()
				return nil
			}
		}
		st.Entries = append(st.Entries, frequencyEntry{ID: frequencyID(doc, start, end), Doc: doc, Start: start, End: end, Text: text, Hits: hits, Source: source, Updated: time.Now().Unix()})
		if len(st.Entries) > 256 {
			sort.SliceStable(st.Entries, func(i, j int) bool {
				if st.Entries[i].Hits != st.Entries[j].Hits {
					return st.Entries[i].Hits > st.Entries[j].Hits
				}
				return st.Entries[i].Updated > st.Entries[j].Updated
			})
			st.Entries = st.Entries[:256]
		}
		return nil
	})
}
func (d *Docs) FrequentSnippet(ctx context.Context, limit, lineChars, maxChars int) (string, error) {
	if _, err := os.Stat(d.frequencyPath()); err != nil && os.IsNotExist(err) {
		return "", nil
	}
	idx, err := d.get(ctx)
	if err != nil {
		return "", err
	}
	entries, err := d.frequencySnapshot(idx.Key)
	if err != nil {
		return "", err
	}
	out, _ := formatFrequency(entries, limit, lineChars, maxChars)
	return out, nil
}

func (d *Docs) frequencySnapshot(fingerprint string) ([]frequencyEntry, error) {
	var st frequencyStore
	b, err := os.ReadFile(d.frequencyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if json.Unmarshal(b, &st) != nil || st.Fingerprint != fingerprint {
		return nil, nil
	}
	for i := range st.Entries {
		if st.Entries[i].ID == "" {
			st.Entries[i].ID = frequencyID(st.Entries[i].Doc, st.Entries[i].Start, st.Entries[i].End)
		}
	}
	sort.SliceStable(st.Entries, func(i, j int) bool {
		if st.Entries[i].Hits != st.Entries[j].Hits {
			return st.Entries[i].Hits > st.Entries[j].Hits
		}
		return st.Entries[i].ID < st.Entries[j].ID
	})
	return st.Entries, nil
}
func formatFrequency(entries []frequencyEntry, limit, lineChars, maxChars int) (string, []frequencyEntry) {
	if limit < 1 {
		limit = 8
	}
	if limit > 256 {
		limit = 256
	}
	if lineChars < 64 {
		lineChars = 64
	}
	if lineChars > 8000 {
		lineChars = 8000
	}
	if maxChars < 64 {
		maxChars = 64
	}
	if maxChars > 32000 {
		maxChars = 32000
	}
	used := 0
	var rows []string
	var emitted []frequencyEntry
	for i, e := range entries {
		if i >= limit {
			break
		}
		text := strings.Join(strings.Fields(e.Text), " ")
		id := e.ID
		if len(id) > 12 {
			id = id[:12]
		}
		prefix := fmt.Sprintf("%d. [%s] %s:%d-%d hits=%d source=%s ", len(rows)+1, id, e.Doc, e.Start, e.End, e.Hits, e.Source)
		avail := maxChars - used - len([]rune(prefix)) - 1
		if avail <= 0 {
			break
		}
		if avail > lineChars-len([]rune(prefix)) {
			avail = lineChars - len([]rune(prefix))
		}
		if avail < 1 {
			continue
		}
		text = truncate(text, avail)
		row := prefix + text
		if used+len([]rune(row))+1 > maxChars {
			break
		}
		rows = append(rows, row)
		emitted = append(emitted, e)
		used += len([]rune(row)) + 1
	}
	return strings.Join(rows, "\n"), emitted
}
func (d *Docs) frequent(ctx context.Context, idx *docIndex, args map[string]any) (string, error) {
	entries, err := d.frequencySnapshot(idx.Key)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	selected := entries
	id := strOpt(args, "entry_id", "")
	doc := strOpt(args, "doc", "")
	start, end := intArg(args, "start_line", 0), intArg(args, "end_line", 0)
	lim := intArg(args, "limit", 8)
	if lim < 1 {
		lim = 8
	}
	if lim > 256 {
		lim = 256
	}
	if id != "" {
		var matches []frequencyEntry
		for _, e := range entries {
			if strings.HasPrefix(e.ID, id) {
				matches = append(matches, e)
			}
		}
		if len(matches) == 0 {
			return "", fmt.Errorf("frequency entry_id not found: %s", id)
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("frequency entry_id is ambiguous: %s", id)
		}
		selected = matches
	} else if doc != "" {
		selected = nil
		for _, e := range entries {
			if e.Doc != doc {
				continue
			}
			if start > 0 && e.End < start {
				continue
			}
			if end > 0 && e.Start > end {
				continue
			}
			selected = append(selected, e)
		}
	} else if start > 0 || end > 0 {
		if start < 1 {
			start = 1
		}
		if end < start {
			end = start
		}
		if start > len(entries) {
			selected = nil
		} else {
			if end > len(entries) {
				end = len(entries)
			}
			selected = entries[start-1 : end]
		}
	} else if lim < len(selected) {
		selected = selected[:lim]
	}
	out, emitted := formatFrequency(selected, lim, intArg(args, "line_chars", 256), intArg(args, "max_chars", 4096))
	if out == "" {
		return "", nil
	}
	for _, e := range emitted {
		d.recordFrequency(idx.Key, e.Doc, e.Start, e.End, e.Text, "frequent", 1)
	}
	return out, nil
}
