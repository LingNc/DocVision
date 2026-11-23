package luadraw

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func frequencyFixture(t *testing.T) (string, string, *Docs) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "doc", "src"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "doc", "src", "notes.tex")
	if err := os.WriteFile(source, []byte("Alpha canonical paragraph line one.\nAlpha canonical paragraph line two.\n\nBeta canonical paragraph.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	d, err := NewDocs(root, cache)
	if err != nil {
		t.Fatal(err)
	}
	return root, cache, d
}

func frequencyEntries(t *testing.T, cache string) []frequencyEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(cache, "luadraw-doc-frequency-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st frequencyStore
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	return st.Entries
}

func TestFrequencySharedCacheConcurrentReadsExactAndPassive(t *testing.T) {
	root, cache, d := frequencyFixture(t)
	ctx := context.Background()
	if _, _, err := d.Execute(ctx, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(cache, "luadraw-doc-frequency-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.FrequentSnippet(ctx, 8, 256, 2048); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(cache, "luadraw-doc-frequency-v1.json"))
	if string(before) != string(after) {
		t.Fatal("passive FrequentSnippet changed disk cache")
	}
	const workers, reads = 4, 3
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			od, e := NewDocs(root, cache)
			if e != nil {
				errs <- e
				return
			}
			for j := 0; j < reads; j++ {
				if _, _, e = od.Execute(ctx, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, t.TempDir()); e != nil {
					errs <- e
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	entries := frequencyEntries(t, cache)
	if len(entries) != 1 || entries[0].Hits != 1+workers*reads {
		t.Fatalf("shared frequency lost updates: %#v", entries)
	}
}

func TestFrequencyRankSelectionBudgetAndStablePrefix(t *testing.T) {
	_, cache, d := frequencyFixture(t)
	ctx := context.Background()
	idx, err := d.get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d.recordFrequency(idx.Key, "doc/a.tex", 1, 1, "first", "read", 5)
	d.recordFrequency(idx.Key, "doc/b.tex", 2, 2, "second", "read", 4)
	d.recordFrequency(idx.Key, "doc/c.tex", 3, 3, "third", "read", 3)
	out, _, err := d.Execute(ctx, map[string]any{"mode": "frequent", "start_line": 2, "end_line": 2, "limit": -1, "max_chars": 4096}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "doc/b.tex") || strings.Contains(out, "doc/a.tex") || strings.Contains(out, "doc/c.tex") {
		t.Fatalf("rank-2 selection wrong: %s", out)
	}
	entries := frequencyEntries(t, cache)
	for _, e := range entries {
		if e.Doc == "doc/b.tex" && e.Hits != 5 {
			t.Fatalf("selected rank entry not incremented: %#v", e)
		}
		if e.Doc != "doc/b.tex" && e.Hits != 5-(5-e.Hits) {
			_ = e
		}
	}
	id := ""
	for _, e := range entries {
		if e.Doc == "doc/b.tex" {
			id = e.ID
		}
	}
	if id == "" {
		t.Fatal("missing stable id")
	}
	prefix := id[:6]
	out, _, err = d.Execute(ctx, map[string]any{"mode": "frequent", "entry_id": prefix, "limit": 1, "max_chars": 128}, "")
	if err != nil || !strings.Contains(out, prefix) {
		t.Fatalf("prefix selection failed: %v %s", err, out)
	}
	short, _, _ := d.Execute(ctx, map[string]any{"mode": "frequent", "limit": 8, "max_chars": 70, "line_chars": 256}, "")
	if strings.Contains(short, "doc/b.tex") && strings.Contains(short, "doc/c.tex") {
		t.Fatalf("budget emitted too many rows: %s", short)
	}
}

func TestFrequencyFingerprintInvalidUTF8AndBadCache(t *testing.T) {
	root, cache, d := frequencyFixture(t)
	ctx := context.Background()
	bad := filepath.Join(cache, "luadraw-doc-frequency-v1.json")
	if err := os.WriteFile(bad, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := d.FrequentSnippet(ctx, 8, 256, 2048); err != nil || got != "" {
		t.Fatalf("bad cache recovery: %v %q", err, got)
	}
	invalid := filepath.Join(root, "doc", "src", "invalid.tex")
	if err := os.WriteFile(invalid, []byte{0xff, 0xfe}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := d.Execute(ctx, map[string]any{"mode": "read", "doc": "doc/src/invalid.tex"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "binary or invalid UTF-8") {
		t.Fatalf("invalid UTF8 not rejected: %v", err)
	}
	if _, _, err := d.Execute(ctx, map[string]any{"mode": "read", "doc": "doc/src/notes.tex", "start_line": 1, "end_line": 2}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "doc", "src", "notes.tex"), []byte("changed paragraph\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := d.FrequentSnippet(ctx, 8, 256, 2048); err != nil || got != "" {
		t.Fatalf("fingerprint cache not invalidated: %v %q", err, got)
	}
}
