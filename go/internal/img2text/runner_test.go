package img2text

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// collectTasksForTest reuses the same regex as Run to scan a markdown
// blob and return the deduped tasks. We keep this logic inline (no
// shared helper in production code) because the task-grouping invariants
// are exactly what we want to test; the production helper builds an
// internal map and we mirror that shape here.
func collectTasksForTest(content string, name string) []imageTask {
	lines := strings.Split(content, "\n")
	starts := make([]int, len(lines)+1)
	starts[0] = 0
	for i, line := range lines {
		starts[i+1] = starts[i] + len(line) + 1
	}
	tasksByKey := map[string]*imageTask{}
	for _, m := range imageRefRe.FindAllStringSubmatchIndex(content, -1) {
		imgPath := content[m[2]:m[3]]
		off := OffsetPair{Start: m[0], End: m[1]}
		key := name + "::" + imgPath
		if t, ok := tasksByKey[key]; ok {
			t.offsets = append(t.offsets, off)
			continue
		}
		il := findLineIndex(starts, m[0])
		tasksByKey[key] = &imageTask{
			key:     key,
			mdName:  name,
			imgPath: imgPath,
			lineIdx: il,
			offsets: []OffsetPair{off},
		}
	}
	out := make([]imageTask, 0, len(tasksByKey))
	for _, t := range tasksByKey {
		sort.Slice(t.offsets, func(i, j int) bool { return t.offsets[i].Start < t.offsets[j].Start })
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// TestTaskDedupSameMD asserts that two references to the same image
// inside the same markdown collapse to one task with two offsets, and
// that two different markdowns each get their own task (no cross-md
// dedup).
func TestTaskDedupSameMD(t *testing.T) {
	doc := "# A\nfirst ![alt](images/a.jpg)\nsecond ![alt](images/a.jpg)\n"
	tasks := collectTasksForTest(doc, "doc.md")
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1 (same image twice in same md)", len(tasks))
	}
	if len(tasks[0].offsets) != 2 {
		t.Fatalf("got %d offsets, want 2", len(tasks[0].offsets))
	}
	if tasks[0].offsets[0].Start >= tasks[0].offsets[1].Start {
		t.Fatalf("offsets not sorted ascending: %+v", tasks[0].offsets)
	}

	// Different markdowns: the same image path should produce two tasks.
	twoDocs := map[string]string{
		"a.md": "![x](images/shared.jpg)\n",
		"b.md": "![y](images/shared.jpg)\n",
	}
	all := map[string]imageTask{}
	for name, content := range twoDocs {
		for _, ts := range collectTasksForTest(content, name) {
			all[ts.key] = ts
		}
	}
	if len(all) != 2 {
		t.Fatalf("cross-md tasks = %d, want 2", len(all))
	}
	for k := range all {
		if !strings.HasSuffix(k, "::images/shared.jpg") {
			t.Fatalf("unexpected key %q", k)
		}
	}
}

// TestValidateOffsetsAcceptsAndRejects covers the conservative
// rebuild-time filter: in-range offsets whose substring is still an
// image reference are kept; offsets that point outside the content,
// point at non-image text, or contain a different image path are dropped.
func TestValidateOffsetsAcceptsAndRejects(t *testing.T) {
	content := "intro ![a](images/a.jpg) middle ![b](images/b.jpg) end"
	matches := imageRefRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Both raw offsets should validate.
	all := []OffsetPair{
		{Start: matches[0][0], End: matches[0][1]},
		{Start: matches[1][0], End: matches[1][1]},
	}
	got := validateOffsets(content, all)
	if len(got) != 2 {
		t.Fatalf("expected 2 valid offsets, got %d", len(got))
	}

	// Out-of-range start.
	got = validateOffsets(content, []OffsetPair{{Start: -1, End: 10}})
	if len(got) != 0 {
		t.Fatalf("expected 0 valid offsets for out-of-range, got %d", len(got))
	}
	// End > len.
	got = validateOffsets(content, []OffsetPair{{Start: 0, End: len(content) + 5}})
	if len(got) != 0 {
		t.Fatalf("expected 0 valid offsets for end>len, got %d", len(got))
	}
	// Token mismatch: the user replaced what used to be the image
	// reference with plain text, but the recorded offsets still fall
	// inside content. The validator must drop these rather than rewriting
	// unrelated text.
	tampered := "intro this used to be an image but is now prose end"
	// Offset range covers plain text (still in bounds, still fits the
	// pre/post checks below, but not an image reference).
	plain := OffsetPair{Start: strings.Index(tampered, "this used"), End: strings.Index(tampered, "prose") + len("prose")}
	if !strings.HasPrefix(tampered[plain.Start:plain.End], "![") {
		// The substring does not start with "![", so the validator will
		// reject it via the prefix check; force the prefix by trimming.
		tampered = "intro " + tampered[strings.Index(tampered, "this used"):]
		plain.Start = strings.Index(tampered, "this used")
		plain.End = plain.Start + len("this used to be an image but is now prose")
		if !strings.HasSuffix(tampered[plain.Start:plain.End], ")") {
			tampered = strings.TrimRight(tampered, " ") + ")"
			plain.End = len(tampered)
		}
	}
	got = validateOffsets(tampered, []OffsetPair{plain})
	if len(got) != 0 {
		t.Fatalf("expected 0 valid offsets for token mismatch, got %d", len(got))
	}
	// Substring in content but NOT an image reference (regular text).
	plain2 := OffsetPair{Start: strings.Index(content, "intro"), End: strings.Index(content, "intro") + len("intro")}
	got = validateOffsets(content, []OffsetPair{plain2})
	if len(got) != 0 {
		t.Fatalf("expected 0 valid offsets for plain text, got %d", len(got))
	}
}

// TestRebuildFromHistoryAndRunResult is the integration-style test for
// the final markdown write. It builds a markdown with the same image
// appearing twice, plants an existing progress_items/<md>/<img>.json
// entry for it, and then calls the rebuild helper to confirm both
// references are replaced using ONLY the historical record.
func TestRebuildFromHistoryAndRunResult(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc.md"
	imgPath := "images/a.jpg"
	content := "intro ![a](images/a.jpg) middle ![a](images/a.jpg) end"
	mdPath := filepath.Join(finallyDir, mdName)
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plant a historical progress record with both offsets.
	historical := ProgressItem{
		Key:     mdName + "::" + imgPath,
		Result:  "[IMG_TYPE: chart]\nanswer",
		ImgPath: imgPath,
	}
	historical.SetOffsets([]OffsetPair{
		{Start: strings.Index(content, "![a]"), End: strings.Index(content, "![a]") + len("![a](images/a.jpg)")},
		{Start: strings.LastIndex(content, "![a]"), End: strings.LastIndex(content, "![a]") + len("![a](images/a.jpg)")},
	})
	if err := SaveProgressItem(progressRoot, mdName, imgPath, historical); err != nil {
		t.Fatal(err)
	}

	// Mirror the rebuild logic from Run(): history-only, no current run.
	items := LoadProgressItems(progressRoot)
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}

	// Same way the runner rebuilds: load content, validate offsets, write
	// replacements right-to-left.
	out, applied := applyHistory(mdPath, items, imgPath)
	if applied != 2 {
		t.Fatalf("expected 2 replacements, got %d", applied)
	}
	if !strings.Contains(out, "<!-- IMG: "+imgPath+" -->") {
		t.Fatalf("rebuilt output missing IMG comment:\n%s", out)
	}
	// Both occurrences should be replaced with the same block.
	if c := strings.Count(out, "[IMG_TYPE: chart]"); c != 2 {
		t.Fatalf("expected 2 AI blocks in output, got %d:\n%s", c, out)
	}
	// Original image references must be gone.
	if strings.Contains(out, "![a](images/a.jpg)") {
		t.Fatalf("output still contains original image ref:\n%s", out)
	}
}

// TestValidateOffsetsWithPath covers the audit fix for the "user
// renamed the image file at the same offset" scenario: when the recorded
// offset still wraps an image reference but the path it points at no
// longer matches the path this ProgressItem was generated for, the
// rebuild pass must drop it so the current run can produce a fresh
// result. Offsets whose path still matches are kept; an empty expected
// path conservatively disables the path-equality guard.
func TestValidateOffsetsWithPath(t *testing.T) {
	oldRef := "![a](images/old.jpg)"
	newRef := "![b](images/new.jpg)"
	content := "intro " + oldRef + " middle " + newRef + " end"

	// Offset for the first reference (originally images/old.jpg).
	oldStart := strings.Index(content, oldRef)
	oldEnd := oldStart + len(oldRef)
	// Offset for the second reference (images/new.jpg).
	newStart := strings.Index(content, newRef)
	newEnd := newStart + len(newRef)

	// 1. Path matches: both offsets survive when asked for old.jpg at
	//    oldStart and new.jpg at newStart respectively.
	if got := validateOffsetsWithPath(content, []OffsetPair{{Start: oldStart, End: oldEnd}}, "images/old.jpg"); len(got) != 1 {
		t.Fatalf("expected 1 offset for matching old path, got %d: %+v", len(got), got)
	}
	if got := validateOffsetsWithPath(content, []OffsetPair{{Start: newStart, End: newEnd}}, "images/new.jpg"); len(got) != 1 {
		t.Fatalf("expected 1 offset for matching new path, got %d: %+v", len(got), got)
	}

	// 2. Audit fix: same offset as old.jpg, but the user replaced the
	//    image with new.jpg at that byte range. The historical record
	//    was generated for old.jpg; the path-equality guard must drop
	//    it so the current run reprocesses images/new.jpg.
	if got := validateOffsetsWithPath(content, []OffsetPair{{Start: oldStart, End: oldEnd}}, "images/new.jpg"); len(got) != 0 {
		t.Fatalf("expected renamed-image offset to be dropped, got %+v", got)
	}

	// 3. Mixed offsets: keep the matching one, drop the mismatched one.
	mixed := []OffsetPair{
		{Start: oldStart, End: oldEnd},
		{Start: newStart, End: newEnd},
	}
	got := validateOffsetsWithPath(content, mixed, "images/new.jpg")
	if len(got) != 1 || got[0].Start != newStart || got[0].End != newEnd {
		t.Fatalf("expected only the new.jpg offset to survive, got %+v", got)
	}

	// 4. Conservative skip: an empty expectedPath preserves the legacy
	//    "offset still matches an image reference" behaviour.
	got = validateOffsetsWithPath(content, mixed, "")
	if len(got) != 2 {
		t.Fatalf("expected both offsets to survive when expectedPath is empty, got %+v", got)
	}
}

// TestRebuildDropsRenamedImagePath is the end-to-end version of the
// scenario above: when the user edits a markdown so that the byte range
// previously occupied by images/old.jpg now contains images/new.jpg,
// the rebuild pass must leave the original image reference untouched
// (i.e. skip writing the cached [AI] block at that offset) so the next
// worker run can produce a result for images/new.jpg.
func TestRebuildDropsRenamedImagePath(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc.md"
	// Current markdown: the user replaced images/old.jpg with
	// images/new.jpg in place.
	content := "intro ![b](images/new.jpg) end"
	mdPath := filepath.Join(finallyDir, mdName)
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Historical record for the OLD image path, with offsets that
	// still wrap an image reference at the same byte range (now
	// pointing at images/new.jpg).
	oldStart := strings.Index(content, "![b](images/new.jpg)")
	oldEnd := oldStart + len("![b](images/new.jpg)")
	historical := ProgressItem{
		Key:     mdName + "::images/old.jpg",
		Result:  "[IMG_TYPE: chart]\nstale answer for old.jpg",
		ImgPath: "images/old.jpg",
	}
	historical.SetOffsets([]OffsetPair{{Start: oldStart, End: oldEnd}})
	if err := SaveProgressItem(progressRoot, mdName, "images/old.jpg", historical); err != nil {
		t.Fatal(err)
	}

	items := LoadProgressItems(progressRoot)
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}

	out, applied := applyHistoryWithExpected(mdPath, items)
	if applied != 0 {
		t.Fatalf("expected 0 replacements for renamed image, got %d:\n%s", applied, out)
	}
	// Output must remain the current markdown (no [AI] block inserted)
	// and must still contain the new.jpg reference.
	if strings.TrimSpace(out) != strings.TrimSpace(content) {
		t.Fatalf("output should be unchanged for renamed image:\nwant: %q\ngot:  %q", content, out)
	}
	if !strings.Contains(out, "![b](images/new.jpg)") {
		t.Fatalf("output lost the current image reference:\n%s", out)
	}
	if strings.Contains(out, "stale answer") {
		t.Fatalf("output leaked the stale AI result for old.jpg:\n%s", out)
	}
}

// applyHistoryWithExpected mirrors Run()'s final-pass loop using
// validateOffsetsWithPath so tests can verify path-equality is enforced.
func applyHistoryWithExpected(mdPath string, items []ProgressItem) (string, int) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		panic(err)
	}
	content := string(data)
	type rep struct {
		off  OffsetPair
		item ProgressItem
	}
	var reps []rep
	for _, it := range items {
		parts := strings.SplitN(it.Key, "::", 2)
		if len(parts) != 2 {
			continue
		}
		expected := expectedImgPath(it, parts[1])
		valid := validateOffsetsWithPath(content, it.EffectiveOffsets(), expected)
		for _, o := range valid {
			reps = append(reps, rep{off: o, item: it})
		}
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].off.Start > reps[j].off.Start })
	nc := content
	for _, r := range reps {
		block := "\n\n<!-- IMG: " + r.item.ImgPath + " -->\n[AI] " +
			r.item.Result + "\n\n<!-- /IMG -->\n\n"
		nc = nc[:r.off.Start] + block + nc[r.off.End:]
	}
	return nc, len(reps)
}

// TestRebuildKeepsMatchingPath is the positive companion to
// TestRebuildDropsRenamedImagePath: when the historical image path still
// matches the image at the recorded offset, the rebuild must apply the
// cached AI result.
func TestRebuildKeepsMatchingPath(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc.md"
	imgPath := "images/a.jpg"
	content := "intro ![a](" + imgPath + ") end"
	mdPath := filepath.Join(finallyDir, mdName)
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	start := strings.Index(content, "![a](")
	end := start + len("![a]("+imgPath+")")
	historical := ProgressItem{
		Key:     mdName + "::" + imgPath,
		Result:  "[IMG_TYPE: chart]\nanswer",
		ImgPath: imgPath,
	}
	historical.SetOffsets([]OffsetPair{{Start: start, End: end}})
	if err := SaveProgressItem(progressRoot, mdName, imgPath, historical); err != nil {
		t.Fatal(err)
	}

	items := LoadProgressItems(progressRoot)
	out, applied := applyHistoryWithExpected(mdPath, items)
	if applied != 1 {
		t.Fatalf("expected 1 replacement for matching path, got %d:\n%s", applied, out)
	}
	if !strings.Contains(out, "[IMG_TYPE: chart]") {
		t.Fatalf("output missing AI result for matching path:\n%s", out)
	}
	if strings.Contains(out, "![a](images/a.jpg)") {
		t.Fatalf("output still contains original image ref:\n%s", out)
	}
}

// TestRebuildFallsBackToKeyForLegacyImgPath covers the legacy JSON case
// where img_path is missing; the rebuild must derive the expected image
// path from the key suffix and still enforce the path-equality guard.
func TestRebuildFallsBackToKeyForLegacyImgPath(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc"
	imgPath := "images/legacy.jpg"
	content := "intro ![x](" + imgPath + ") end"
	mdPath := filepath.Join(finallyDir, mdName+".md")
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	start := strings.Index(content, "![x](")
	end := start + len("![x]("+imgPath+")")
	// Plant a legacy JSON record (img_path omitted) so expectedImgPath
	// must fall back to the key suffix.
	subdir := filepath.Join(progressRoot, mdName)
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"key":"doc.md::` + imgPath + `","result":"[IMG_TYPE: chart]\nlegacy","start":` +
		strconv.Itoa(start) + `,"end":` + strconv.Itoa(end) + `}`
	if err := os.WriteFile(filepath.Join(subdir, "legacy.jpg.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	items := LoadProgressItems(progressRoot)
	if len(items) != 1 {
		t.Fatalf("expected 1 history item, got %d", len(items))
	}
	if items[0].ImgPath != "" {
		t.Fatalf("legacy fixture should have empty ImgPath, got %q", items[0].ImgPath)
	}

	out, applied := applyHistoryWithExpected(mdPath, items)
	if applied != 1 {
		t.Fatalf("expected 1 replacement for legacy record with key-suffix fallback, got %d:\n%s", applied, out)
	}
	if !strings.Contains(out, "legacy") {
		t.Fatalf("output missing legacy AI result:\n%s", out)
	}
}

// TestRebuildDropsStaleOffsets covers the conservative case: when the
// recorded offsets no longer match the current markdown content (e.g.
// the user added text before the image), those offsets must be skipped
// rather than corrupting the file.
func TestRebuildDropsStaleOffsets(t *testing.T) {
	original := "![a](images/a.jpg)\n"
	historical := ProgressItem{
		Key:     "doc.md::images/a.jpg",
		Result:  "[IMG_TYPE: chart]\nanswer",
		ImgPath: "images/a.jpg",
	}
	// Recorded offset points at the start of the original markdown.
	historical.SetOffsets([]OffsetPair{{Start: 0, End: len("![a](images/a.jpg)\n") - 1}})

	// Current content has extra text prepended, so the recorded offsets
	// no longer cover an image reference.
	shifted := "preamble added by the user.\n" + original

	valid := validateOffsets(shifted, historical.EffectiveOffsets())
	if len(valid) != 0 {
		t.Fatalf("expected stale offsets to be rejected, got %+v", valid)
	}

	// Re-deriving from the current content gives the correct offsets.
	matches := imageRefRe.FindAllStringSubmatchIndex(shifted, -1)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match in current content, got %d", len(matches))
	}
	current := []OffsetPair{{Start: matches[0][0], End: matches[0][1]}}
	got := validateOffsets(shifted, current)
	if len(got) != 1 {
		t.Fatalf("current offsets should validate, got %+v", got)
	}
}

// TestRebuildMergesHistoryAndCurrent covers the case where one item
// comes from history and a different item comes from the in-memory
// progressData this run. Both should land in the output.
func TestRebuildMergesHistoryAndCurrent(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc.md"
	content := "first ![a](images/a.jpg) middle ![b](images/b.jpg) end"
	if err := os.WriteFile(filepath.Join(finallyDir, mdName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// History: only image a is done.
	histItem := ProgressItem{
		Key:     mdName + "::images/a.jpg",
		Result:  "[IMG_TYPE: chart]\nA-result",
		ImgPath: "images/a.jpg",
	}
	histItem.SetOffsets([]OffsetPair{
		{Start: strings.Index(content, "![a]"), End: strings.Index(content, "![a]") + len("![a](images/a.jpg)")},
	})
	if err := SaveProgressItem(progressRoot, mdName, "images/a.jpg", histItem); err != nil {
		t.Fatal(err)
	}

	// Current run produced image b's result.
	curItem := ProgressItem{
		Key:     mdName + "::images/b.jpg",
		Result:  "[IMG_TYPE: chart]\nB-result",
		ImgPath: "images/b.jpg",
	}
	curItem.SetOffsets([]OffsetPair{
		{Start: strings.Index(content, "![b]"), End: strings.Index(content, "![b]") + len("![b](images/b.jpg)")},
	})

	// Merge via the runner's algorithm: history + current successful run.
	history := LoadProgressItems(progressRoot)
	merged := mergeProgressItems(history, []ProgressItem{curItem})

	out, applied := applyHistoryWithExtras(mdName, filepath.Join(finallyDir, mdName), merged)
	if applied != 2 {
		t.Fatalf("expected 2 replacements, got %d", applied)
	}
	if !strings.Contains(out, "A-result") || !strings.Contains(out, "B-result") {
		t.Fatalf("output missing one of the results:\n%s", out)
	}
	if strings.Contains(out, "![a](images/a.jpg)") || strings.Contains(out, "![b](images/b.jpg)") {
		t.Fatalf("output still contains original references:\n%s", out)
	}
}

// TestRebuildRecoversAfterFinallyDeleted covers the "pending=0, finally
// file deleted" recovery case: progress_items on disk is the only
// source of AI results and must rebuild the finally file. We stage an
// empty placeholder so applyHistory can read it (the runner path will
// simply write a fresh file when none exists, so this is the realistic
// recovery contract).
func TestRebuildRecoversAfterFinallyDeleted(t *testing.T) {
	root := t.TempDir()
	finallyDir := filepath.Join(root, "finally")
	progressRoot := filepath.Join(finallyDir, "progress_items")
	if err := os.MkdirAll(progressRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	mdName := "doc.md"
	imgPath := "images/a.jpg"
	content := "![a](images/a.jpg)\n"
	offset := []OffsetPair{
		{Start: strings.Index(content, "![a]"), End: strings.Index(content, "![a]") + len("![a](images/a.jpg)")},
	}

	// The on-disk record only carries offsets relative to the original
	// content; we use that content here so the offset is still valid.
	mdPath := filepath.Join(finallyDir, mdName)
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	item := ProgressItem{
		Key:     mdName + "::" + imgPath,
		Result:  "[IMG_TYPE: chart]\nrec",
		ImgPath: imgPath,
	}
	item.SetOffsets(offset)
	if err := SaveProgressItem(progressRoot, mdName, imgPath, item); err != nil {
		t.Fatal(err)
	}

	items := LoadProgressItems(progressRoot)
	out, applied := applyHistory(mdPath, items, imgPath)
	if applied != 1 {
		t.Fatalf("expected 1 replacement, got %d", applied)
	}
	if !strings.Contains(out, "rec") {
		t.Fatalf("output missing result block:\n%s", out)
	}
}

// applyHistory mirrors the rebuild logic in Run() for a single markdown
// file: validate each historical item's offsets against the current
// content, then apply replacements right-to-left. Returns the rebuilt
// markdown and the number of replacements applied.
func applyHistory(mdPath string, items []ProgressItem, imgPath string) (string, int) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		panic(err)
	}
	content := string(data)
	type rep struct {
		off  OffsetPair
		item ProgressItem
	}
	var reps []rep
	for _, it := range items {
		valid := validateOffsets(content, it.EffectiveOffsets())
		for _, o := range valid {
			reps = append(reps, rep{off: o, item: it})
		}
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].off.Start > reps[j].off.Start })
	nc := content
	for _, r := range reps {
		block := "\n\n<!-- IMG: " + r.item.ImgPath + " -->\n[AI] " +
			r.item.Result + "\n\n<!-- /IMG -->\n\n"
		nc = nc[:r.off.Start] + block + nc[r.off.End:]
	}
	return nc, len(reps)
}

// applyHistoryWithExtras is like applyHistory but takes an already-merged
// item slice (history + current run) so callers can inject extras.
func applyHistoryWithExtras(mdName, mdPath string, items []ProgressItem) (string, int) {
	data, err := os.ReadFile(mdPath)
	if err != nil {
		panic(err)
	}
	content := string(data)
	type rep struct {
		off  OffsetPair
		item ProgressItem
	}
	var reps []rep
	for _, it := range items {
		if !strings.HasPrefix(it.Key, mdName+"::") {
			continue
		}
		valid := validateOffsets(content, it.EffectiveOffsets())
		for _, o := range valid {
			reps = append(reps, rep{off: o, item: it})
		}
	}
	sort.Slice(reps, func(i, j int) bool { return reps[i].off.Start > reps[j].off.Start })
	nc := content
	for _, r := range reps {
		block := "\n\n<!-- IMG: " + r.item.ImgPath + " -->\n[AI] " +
			r.item.Result + "\n\n<!-- /IMG -->\n\n"
		nc = nc[:r.off.Start] + block + nc[r.off.End:]
	}
	return nc, len(reps)
}
