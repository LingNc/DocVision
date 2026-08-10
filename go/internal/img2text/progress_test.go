package img2text

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// progressItemRoundTrip verifies that ProgressItem JSON round-trips both
// for legacy single-offset records (only Start/End) and for new
// multi-offset records. Legacy readers see at least the first pair via
// EffectiveOffsets.
func TestProgressItemRoundTrip(t *testing.T) {
	t.Run("legacy single offset", func(t *testing.T) {
		raw := `{
			"key": "doc.md::images/a.jpg",
			"result": "[IMG_TYPE: table]\nfoo",
			"start": 10,
			"end": 35,
			"img_path": "images/a.jpg"
		}`
		var item ProgressItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			t.Fatalf("unmarshal legacy: %v", err)
		}
		eff := item.EffectiveOffsets()
		if len(eff) != 1 || eff[0].Start != 10 || eff[0].End != 35 {
			t.Fatalf("legacy effective offsets = %+v, want one pair {10,35}", eff)
		}
		// Re-marshal and confirm we can read it back unchanged.
		out, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal legacy: %v", err)
		}
		var round ProgressItem
		if err := json.Unmarshal(out, &round); err != nil {
			t.Fatalf("round unmarshal legacy: %v", err)
		}
		if round.Start != 10 || round.End != 35 {
			t.Fatalf("round Start/End = %d/%d, want 10/35", round.Start, round.End)
		}
		if len(round.EffectiveOffsets()) != 1 {
			t.Fatalf("round offsets length = %d, want 1", len(round.EffectiveOffsets()))
		}
	})

	t.Run("multi offset", func(t *testing.T) {
		item := ProgressItem{
			Key:     "doc.md::images/a.jpg",
			Result:  "[IMG_TYPE: table]\nfoo",
			ImgPath: "images/a.jpg",
		}
		item.SetOffsets([]OffsetPair{{Start: 5, End: 30}, {Start: 100, End: 125}})
		out, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !strings.Contains(string(out), `"offsets":`) {
			t.Fatalf("expected offsets key in JSON, got %s", out)
		}
		var round ProgressItem
		if err := json.Unmarshal(out, &round); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		eff := round.EffectiveOffsets()
		if len(eff) != 2 {
			t.Fatalf("round offsets length = %d, want 2", len(eff))
		}
		if eff[0].Start != 5 || eff[1].Start != 100 {
			t.Fatalf("round offsets = %+v", eff)
		}
		// Legacy Start/End should be refreshed to the first pair.
		if round.Start != 5 || round.End != 30 {
			t.Fatalf("legacy fields not refreshed: Start=%d End=%d", round.Start, round.End)
		}
	})

	t.Run("set offsets refreshes legacy", func(t *testing.T) {
		item := ProgressItem{Start: 999, End: 1000}
		item.SetOffsets([]OffsetPair{{Start: 7, End: 9}, {Start: 50, End: 60}})
		if item.Start != 7 || item.End != 9 {
			t.Fatalf("legacy not refreshed to first offset: %+v", item)
		}
		eff := item.EffectiveOffsets()
		if len(eff) != 2 {
			t.Fatalf("expected 2 offsets, got %d", len(eff))
		}
	})
}

// TestLoadProgressItemsFiltering seeds the progress dir with a mix of
// successful, sentinel, mismatched-directory, and invalid records and
// checks that LoadProgressItems returns only the successful ones whose
// directory matches their key.
func TestLoadProgressItemsFiltering(t *testing.T) {
	root := t.TempDir()

	write := func(mdDir, file string, body string) {
		dir := filepath.Join(root, mdDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Good record.
	write("doc", "good.json", `{
		"key": "doc.md::images/a.jpg",
		"result": "[IMG_TYPE: table]\nfoo",
		"start": 5, "end": 30, "img_path": "images/a.jpg",
		"offsets": [{"start":5,"end":30},{"start":100,"end":125}]
	}`)
	// Legacy single offset record - must still be kept.
	write("doc", "legacy.json", `{
		"key": "doc.md::images/legacy.jpg",
		"result": "[IMG_TYPE: table]\nbar",
		"start": 7, "end": 9, "img_path": "images/legacy.jpg"
	}`)
	// Error sentinel - should be skipped.
	write("doc", "bad.json", `{
		"key": "doc.md::images/bad.jpg",
		"result": "[IMG_API_ERROR: 500]",
		"start": 1, "end": 2, "img_path": "images/bad.jpg"
	}`)
	// Mismatched directory (sits under doc/ but the key claims other.md).
	write("doc", "wrongkey.json", `{
		"key": "other.md::images/x.jpg",
		"result": "[IMG_TYPE: chart]\nzzz",
		"start": 1, "end": 2, "img_path": "images/x.jpg"
	}`)
	// Invalid JSON.
	write("doc", "broken.json", `not json`)
	// Empty result.
	write("doc", "empty.json", `{
		"key": "doc.md::images/empty.jpg",
		"result": "",
		"start": 1, "end": 2, "img_path": "images/empty.jpg"
	}`)
	// Successful item under other/ that does match its key.
	write("other", "ok.json", `{
		"key": "other.md::images/o.jpg",
		"result": "[IMG_TYPE: chart]\nok",
		"start": 1, "end": 2, "img_path": "images/o.jpg"
	}`)

	items := LoadProgressItems(root)
	if len(items) != 3 {
		t.Fatalf("LoadProgressItems returned %d items, want 3: %+v", len(items), keys(items))
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	wantKeys := []string{
		"doc.md::images/a.jpg",
		"doc.md::images/legacy.jpg",
		"other.md::images/o.jpg",
	}
	for i, it := range items {
		if it.Key != wantKeys[i] {
			t.Fatalf("items[%d].Key = %q, want %q", i, it.Key, wantKeys[i])
		}
		if !strings.Contains(it.Result, "[IMG_TYPE:") {
			t.Fatalf("items[%d].Result missing [IMG_TYPE: : %q", i, it.Result)
		}
	}
	// The legacy record should yield a single offset via EffectiveOffsets.
	for _, it := range items {
		if it.Key == "doc.md::images/legacy.jpg" {
			if len(it.EffectiveOffsets()) != 1 {
				t.Fatalf("legacy offsets = %+v, want 1 pair", it.EffectiveOffsets())
			}
			if it.EffectiveOffsets()[0].Start != 7 || it.EffectiveOffsets()[0].End != 9 {
				t.Fatalf("legacy effective offset = %+v", it.EffectiveOffsets()[0])
			}
		}
		if it.Key == "doc.md::images/a.jpg" {
			if len(it.EffectiveOffsets()) != 2 {
				t.Fatalf("multi offsets = %+v, want 2 pairs", it.EffectiveOffsets())
			}
		}
	}
}

// TestLoadProgressKeysReuse confirms the bool API still skips sentinels
// so callers that only need the "already done" set keep working.
func TestLoadProgressKeysReuse(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "doc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	must := func(body string) {
		f, err := os.CreateTemp(dir, "*.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(body); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	must(`{"key":"doc.md::images/a.jpg","result":"[IMG_TYPE: t]\nfoo"}`)
	must(`{"key":"doc.md::images/b.jpg","result":"[IMG_API_ERROR]"}`)

	keys := LoadProgress(root)
	if !keys["doc.md::images/a.jpg"] {
		t.Fatalf("expected successful key, got %v", keys)
	}
	if keys["doc.md::images/b.jpg"] {
		t.Fatalf("expected sentinel key to be skipped, got %v", keys)
	}
}

func keys(items []ProgressItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Key)
	}
	return out
}
