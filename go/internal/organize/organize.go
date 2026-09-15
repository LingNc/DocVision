// Package organize copies full.md files from the MinerU output directory,
// merges split parts by subject, and collects referenced images into per-
// subject subdirectories. It mirrors the archived Python reference
// (legacy/python/python/organize_files.py) while adding incremental
// behaviour: reruns preserve existing output Markdown, images, and manually
// created files; only the temp directory is rebuilt each run.
package organize

import (
	"sync"
	"sync/atomic"
	"time"

	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mineru-tools/internal/config"
	"mineru-tools/pkg/util"
)

// partRe captures "subject_partN" directory names.
// Group 1 = subject, group 2 = part number.
var partRe = regexp.MustCompile(`^(.+?)_part(\d+)$`)

// imgRe matches markdown image references of the form
// ![alt](images/foo.jpg) where the extension is jpg/jpeg/png/gif/webp.
// Group 1 captures "images/foo.ext".
var imgRe = regexp.MustCompile(`(?i)(?:!\[.*?\]\(|<img[^>]*?src=["'])(images/.+?\.(?:jpg|jpeg|png|gif|webp))(?:\)|["'][^>]*>)`)

// supportedExts is the set of image extensions recognised by step3.
var supportedExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
}

const mdSeparator = "\n\n---\n\n"

// fingerprintExt marks the sidecar file used to remember which source
// full.md files contributed to a given output subject. Keeping a sidecar
// alongside the merged Markdown lets later runs skip regeneration when
// the source content is unchanged.
const fingerprintExt = ".fp"

// indexScanCounter is a package-level counter incremented every time
// step3CollectImages actually builds the source image index in a given
// run. It exists purely for tests (see TestOrganizeFilesIndexBuiltOnce)
// to prove that the index is built at most once per OrganizeFiles call
// even when several subjects require missing-image lookups.
//
// The production pipeline does not read this value and does not rely on
// it for correctness. OrganizeFiles is invoked sequentially on a single
// goroutine, so the counter is only ever written by that goroutine and
// the race detector should stay quiet as long as tests are not run with
// t.Parallel(). Do not extend the public API for this variable; if a
// future feature needs to count builds, replace it with an explicit
// return value from the relevant helper instead of growing this global.
var indexScanCounter int

// OrganizeFiles runs the four-step file organisation pipeline:
//  1. copy full.md -> output/temp/{name}.md
//  2. merge split parts -> output/{subject}.md
//  3. collect referenced images -> images/{subject}/
//  4. print summary
//
// The function is incremental: reruns preserve the existing output/*.md,
// output/images and any manually created files. Only output/temp is
// rebuilt on every invocation.
func OrganizeFiles(cfg *config.Config, subjects ...string) error {
	mineruOutput := cfg.Paths.MineruOutput
	outputDir := cfg.Paths.OutputDir
	imagesDir := cfg.Paths.ImagesDir
	tempDir := filepath.Join(outputDir, "temp")

	if !util.DirExists(mineruOutput) {
		return fmt.Errorf("MinerU 输出目录不存在: %s", mineruOutput)
	}

	// Ensure output/images/temp exist. We deliberately do NOT RemoveAll
	// outputDir: reruns must keep existing Markdown, images and any
	// manually created files.
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return fmt.Errorf("创建 images 目录失败: %w", err)
	}
	// temp holds per-run part copies; safe and cheap to wipe.
	if util.DirExists(tempDir) {
		if err := os.RemoveAll(tempDir); err != nil {
			return fmt.Errorf("清理 temp 目录失败: %w", err)
		}
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("创建 temp 目录失败: %w", err)
	}

	// Reset the per-run scan counter used by tests to verify index reuse.
	indexScanCounter = 0

	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println("  MinerU 文件整理工具")
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println()

	// groups[subject] = sorted list of part numbers (empty for single-file subjects).
	// Optional subjects filter: when latex preflow names a specific book,
	// only that subject's parts are copied/merged (no full-tree scan).
	var filter map[string]bool
	if len(subjects) > 0 {
		filter = map[string]bool{}
		for _, sub := range subjects {
			filter[sub] = true
		}
	}
	groups, allDirs, err := step1CopyMarkdown(mineruOutput, tempDir, filter)
	if err != nil {
		return err
	}

	if err := step2MergeParts(groups, mineruOutput, outputDir); err != nil {
		return err
	}

	if err := step3CollectImages(allDirs, outputDir, imagesDir); err != nil {
		return err
	}

	step4Summary(outputDir, tempDir, imagesDir, subjects)
	return nil
}

// step1CopyMarkdown walks the MinerU output, copying every full.md into
// temp/ and grouping part directories by subject. The temp directory is
// always rebuilt so this step does no incremental bookkeeping.
func step1CopyMarkdown(mineruOutput, tempDir string, filter map[string]bool) (map[string][]string, []string, error) {
	fmt.Printf("[1/4] 复制 full.md -> %s/ ...\n", tempDir)

	entries, err := os.ReadDir(mineruOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("读取 mineru_output 失败: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	groups := make(map[string][]string)
	var allDirs []string
	mdCount := 0

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if filter != nil {
			subject := e.Name()
			if m := partRe.FindStringSubmatch(e.Name()); m != nil {
				subject = m[1]
			}
			if !filter[subject] {
				continue
			}
		}
		dirPath := filepath.Join(mineruOutput, e.Name())
		fullMD := filepath.Join(dirPath, "full.md")
		if !util.FileExists(fullMD) {
			continue
		}
		allDirs = append(allDirs, dirPath)

		if m := partRe.FindStringSubmatch(e.Name()); m != nil {
			subject, partNum := m[1], m[2]
			groups[subject] = append(groups[subject], partNum)
			fmt.Printf("  %s (part%s)\n", subject, partNum)
			dst := filepath.Join(tempDir, subject+"_"+partNum+".md")
			if err := util.CopyFile(fullMD, dst); err != nil {
				return nil, nil, fmt.Errorf("复制 %s -> %s 失败: %w", fullMD, dst, err)
			}
			mdCount++
		} else {
			subject := e.Name()
			groups[subject] = []string{}
			fmt.Printf("  %s (单文件)\n", subject)
			dst := filepath.Join(tempDir, subject+".md")
			if err := util.CopyFile(fullMD, dst); err != nil {
				return nil, nil, fmt.Errorf("复制 %s -> %s 失败: %w", fullMD, dst, err)
			}
			mdCount++
		}
	}

	fmt.Printf("  MD: %d\n", mdCount)
	fmt.Println()
	return groups, allDirs, nil
}

// step2MergeParts writes the merged/standalone markdown for each subject.
// Subjects whose source fingerprints match the stored sidecar and whose
// merged Markdown is present are skipped, so reruns preserve existing
// content while still picking up new or modified sources.
func step2MergeParts(groups map[string][]string, mineruOutput, outputDir string) error {
	fmt.Printf("[2/4] 合并分片 -> %s/ ...\n", outputDir)

	// Process subjects in a deterministic order.
	subjects := make([]string, 0, len(groups))
	for s := range groups {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	mergeCount := 0
	skipCount := 0

	for _, subject := range subjects {
		parts := groups[subject]
		srcFingerprint, err := sourceFingerprint(mineruOutput, subject, parts)
		if err != nil {
			return err
		}

		dstFile := filepath.Join(outputDir, subject+".md")
		fpFile := fingerprintPath(dstFile)

		// Skip regeneration only when the stored fingerprint still matches
		// the source AND the merged file is on disk. Skipped subjects are
		// not counted in mergeCount below.
		if util.FileExists(dstFile) && util.FileExists(fpFile) {
			stored, err := os.ReadFile(fpFile)
			if err != nil {
				return fmt.Errorf("读取指纹 %s 失败: %w", fpFile, err)
			}
			if string(stored) == srcFingerprint {
				skipCount++
				continue
			}
		}

		var content string
		if len(parts) == 0 {
			// Single-file (non-split) subject: copy the temp file as-is.
			src := filepath.Join(outputDir, "temp", subject+".md")
			if !util.FileExists(src) {
				continue
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return fmt.Errorf("读取 %s 失败: %w", src, err)
			}
			content = string(data)
			fmt.Printf("  %s.md (单文件)\n", subject)
		} else {
			// Sort parts numerically.
			sort.Slice(parts, func(i, j int) bool {
				ai, _ := strconv.Atoi(parts[i])
				aj, _ := strconv.Atoi(parts[j])
				return ai < aj
			})

			if len(parts) == 1 {
				src := filepath.Join(outputDir, "temp", subject+"_"+parts[0]+".md")
				if !util.FileExists(src) {
					continue
				}
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("读取 %s 失败: %w", src, err)
				}
				content = string(data)
				fmt.Printf("  %s.md (单分片)\n", subject)
			} else {
				fmt.Printf("  合并: %s (parts %v)\n", subject, parts)
				tempBase := filepath.Join(outputDir, "temp")
				var contents []string
				for _, p := range parts {
					partFile := filepath.Join(tempBase, subject+"_"+p+".md")
					if !util.FileExists(partFile) {
						continue
					}
					data, err := os.ReadFile(partFile)
					if err != nil {
						return fmt.Errorf("读取 %s 失败: %w", partFile, err)
					}
					text := strings.TrimRight(string(data), "\r\n\t ")
					if text != "" {
						contents = append(contents, text)
					}
				}
				content = strings.Join(contents, mdSeparator)
			}
		}

		if err := os.WriteFile(dstFile, []byte(content), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", dstFile, err)
		}
		if err := os.WriteFile(fpFile, []byte(srcFingerprint), 0o644); err != nil {
			return fmt.Errorf("写入指纹 %s 失败: %w", fpFile, err)
		}
		// Every subject whose merged/standalone Markdown is actually
		// written to outputDir counts as one "merge" for the summary,
		// regardless of whether it was a single file, single part or a
		// multi-part merge. Skipped (unchanged) subjects are not counted.
		mergeCount++
	}

	fmt.Printf("  合并: %d, 跳过: %d\n", mergeCount, skipCount)
	fmt.Println()
	return nil
}

// step3CollectImages rewrites markdown image paths to point at
// images/{subject}/ and copies each referenced file from any mineru
// source directory into that location. The source image index is built
// at most once per run and only when a missing target is detected.
func step3CollectImages(allDirs []string, outputDir, imagesDir string) error {
	fmt.Printf("[3/4] 收集引用的图片 -> %s/ ...\n", imagesDir)

	imgCount := 0
	skipSubject := 0
	missing := 0

	mdFiles, err := filepath.Glob(filepath.Join(outputDir, "*.md"))
	if err != nil {
		return fmt.Errorf("查找 %s/*.md 失败: %w", outputDir, err)
	}
	sort.Strings(mdFiles)

	// Index is built lazily on first missing target and reused for every
	// subsequent subject in this run.
	var imgSourceMap map[string]string
	indexBuilt := false

	for _, mdFile := range mdFiles {
		subject := strings.TrimSuffix(filepath.Base(mdFile), filepath.Ext(mdFile))
		subjectImgDir := filepath.Join(imagesDir, subject)

		data, err := os.ReadFile(mdFile)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", mdFile, err)
		}
		content := string(data)

		refs := uniqueImageRefs(imgRe.FindAllStringSubmatch(content, -1))
		if len(refs) == 0 {
			continue
		}

		// Determine which refs are still missing on disk. Already-present
		// images don't trigger an index build or rewrite work.
		missingTargets := missingImageTargets(refs, imagesDir, subject)
		needsIndex := false
		for _, t := range missingTargets {
			if !util.FileExists(t) {
				needsIndex = true
				break
			}
		}

		if !needsIndex {
			// All referenced images are already present locally, so no
			// copy work is needed. Still apply the idempotent rewrite: a
			// rerun may have regenerated this markdown from the MinerU
			// source (which carries bare "images/foo.ext" paths), and if
			// we skip the rewrite those bare references survive and make
			// img2text fail with IMG_MISSING. rewriteImagePaths is strict
			// and idempotent: it only prefixes bare filenames and leaves
			// already-prefixed paths alone, so it cannot double-prefix
			// (including subjects that contain spaces/parentheses).
			if err := rewriteImagePaths(mdFile, content, subject); err != nil {
				return err
			}
			skipSubject++
			continue
		}

		if !indexBuilt {
			fmt.Printf("    有 %d 张图片缺失，正在扫描 %d 个 mineru 目录建索引（并行）...\n",
				len(missingTargets), len(allDirs))
			imgSourceMap = buildImageSourceIndex(allDirs)
			indexBuilt = true
			indexScanCounter++
		}

		if err := os.MkdirAll(subjectImgDir, 0o755); err != nil {
			return fmt.Errorf("创建 %s 失败: %w", subjectImgDir, err)
		}
		collected := 0

		for _, ref := range refs {
			// Extract the basename after the last "/" so that already
			// rewritten paths (images/<subject>/foo.jpg) and bare
			// paths (images/foo.jpg) both resolve to foo.jpg.
			imgName := ref
			if idx := strings.LastIndex(ref, "/"); idx >= 0 {
				imgName = ref[idx+1:]
			}
			dstImg := filepath.Join(subjectImgDir, imgName)
			if util.FileExists(dstImg) {
				collected++
				continue
			}
			srcImg, ok := imgSourceMap[imgName]
			if !ok {
				missing++
				// Report the original bare reference (images/<imgName>)
				// rather than the rewritten "images/<subject>/<imgName>"
				// form. On a rerun the on-disk Markdown may already be
				// subject-prefixed, which would mislead users looking
				// for the source image.
				fmt.Printf("  警告: [%s] 找不到图片源 images/%s\n", subject, imgName)
				continue
			}
			if err := util.CopyFile(srcImg, dstImg); err != nil {
				return fmt.Errorf("复制 %s -> %s 失败: %w", srcImg, dstImg, err)
			}
			imgCount++
			collected++
		}

		// Rewrite paths idempotently: only touch bare "images/foo.ext"
		// references that have not yet been prefixed with the subject.
		if err := rewriteImagePaths(mdFile, content, subject); err != nil {
			return err
		}

		fmt.Printf("  %s: %d 张图片\n", subject, collected)
	}

	fmt.Printf("  收集: %d 张, 跳过: %d 个文件\n", imgCount, skipSubject)
	if missing > 0 {
		fmt.Printf("  缺失: %d 张\n", missing)
	}
	fmt.Println()
	return nil
}

// step4Summary prints sizes for the output, temp and images directories.
// When subjects are named (latex preflow for one book), the listing is
// scoped to those subjects instead of dumping the whole shared tree.
func step4Summary(outputDir, tempDir, imagesDir string, subjects []string) {
	var filter map[string]bool
	scoped := len(subjects) > 0
	if scoped {
		filter = map[string]bool{}
		for _, sub := range subjects {
			filter[sub] = true
		}
	}
	fmt.Println("[4/4] 汇总")
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println()

	fmt.Printf("  %s/ (合并后):\n", outputDir)
	for _, md := range listOutputMarkdown(outputDir) {
		base := filepath.Base(md)
		if scoped && !filter[strings.TrimSuffix(base, ".md")] {
			continue
		}
		fmt.Printf("    %s  (%.1f KB)\n", base, float64(fileSize(md))/1024)
	}

	fmt.Println()
	fmt.Printf("  %s/ (分片):\n", tempDir)
	for _, md := range listMarkdown(tempDir) {
		fmt.Printf("    %s  (%.1f KB)\n", filepath.Base(md), float64(fileSize(md))/1024)
	}

	imgTotal := 0
	fmt.Println()
	fmt.Printf("  %s/ :\n", imagesDir)
	if util.DirExists(imagesDir) {
		entries, err := os.ReadDir(imagesDir)
		if err == nil {
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if scoped && !filter[e.Name()] {
					continue
				}
				count, _ := countFiles(filepath.Join(imagesDir, e.Name()))
				imgTotal += count
				fmt.Printf("    %s/ (%d 张)\n", e.Name(), count)
			}
		}
	}
	if scoped {
		fmt.Printf("    共 %d 张图片（仅选中书目）\n", imgTotal)
	} else {
		fmt.Printf("    共 %d 张图片\n", imgTotal)
	}
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println("完成!")
}

// sourceFingerprint computes a deterministic fingerprint string over all
// source full.md files contributing to a given subject. The fingerprint
// is based on file size and mtime (size:mtime pairs sorted by part
// number) and is independent of the per-run temp directory so reruns do
// not consider temp file regeneration as a content change.
func sourceFingerprint(mineruOutput, subject string, parts []string) (string, error) {
	if len(parts) == 0 {
		srcMD := filepath.Join(mineruOutput, subject, "full.md")
		fpStr, err := singleFingerprint(srcMD)
		if err != nil {
			return "", err
		}
		return fpStr, nil
	}

	sortedParts := append([]string(nil), parts...)
	sort.Slice(sortedParts, func(i, j int) bool {
		ai, _ := strconv.Atoi(sortedParts[i])
		aj, _ := strconv.Atoi(sortedParts[j])
		return ai < aj
	})

	var lines []string
	for _, p := range sortedParts {
		srcMD := filepath.Join(mineruOutput, subject+"_part"+p, "full.md")
		sig, err := singleFingerprint(srcMD)
		if err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf("%s=%s", p, sig))
	}
	return strings.Join(lines, ";"), nil
}

// singleFingerprint returns a "size:mtime" signature for a single file.
func singleFingerprint(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("读取 %s 信息失败: %w", path, err)
	}
	return fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano()), nil
}

// fingerprintPath returns the sidecar path used to remember a subject's
// source fingerprint, given the path of the merged Markdown.
func fingerprintPath(mdPath string) string {
	return mdPath + fingerprintExt
}

// missingImageTargets returns the on-disk destination paths for each
// referenced image under imagesDir/<subject>/, deduped by basename.
func missingImageTargets(refs []string, imagesDir, subject string) []string {
	seen := make(map[string]bool, len(refs))
	var out []string
	for _, ref := range refs {
		name := ref
		if idx := strings.LastIndex(ref, "/"); idx >= 0 {
			name = ref[idx+1:]
		}
		dst := filepath.Join(imagesDir, subject, name)
		if seen[dst] {
			continue
		}
		seen[dst] = true
		out = append(out, dst)
	}
	return out
}

// rewriteImagePaths rewrites bare "images/foo.ext" occurrences in the
// Markdown content to point at images/<subject>/foo.ext, leaving any
// already-prefixed path untouched. The change is written back to disk
// only if the content actually changed.
//
// "Bare" means the segment after "images/" has no further "/" — i.e.
// it is exactly a filename. Already-prefixed paths are recognised by
// an exact byte-level prefix match against the subject, with no trim,
// case folding or other normalisation, so subjects that contain
// spaces, parentheses, commas, trailing whitespace or non-ASCII
// characters round-trip safely across runs.
//
// Tokenisation reuses imgRe — the same regex step3 uses to identify
// image references. Anchoring on the Markdown image syntax
// `![alt](images/...)` is the only reliable way to bound a path
// token, because subject names and even bare filenames may contain
// whitespace, parentheses, quotes or other characters that ad-hoc
// scanners would mistake for delimiters.
func rewriteImagePaths(mdFile, content, subject string) error {
	prefix := "images/" + subject + "/"
	subjectWithSlash := subject + "/"
	// imgRe matches `![...](images/<path>)`. m[0]:m[1] is the whole
	// match (including the surrounding `](` and trailing `)`); m[2]:m[3]
	// is the captured URL ("images/..."). Use the captured URL as the
	// token — that is exactly the Markdown-link-bounded path we want,
	// regardless of what characters appear inside the subject or the
	// filename.
	matches := imgRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}
	var b strings.Builder
	last := 0
	changed := false
	for _, m := range matches {
		urlStart, urlEnd := m[2], m[3]
		url := content[urlStart:urlEnd]
		rest := url[len("images/"):]
		switch {
		case strings.HasPrefix(rest, subjectWithSlash):
			// Exact subject prefix: already normalised, leave alone.
			b.WriteString(content[last:urlStart])
			b.WriteString(url)
			last = urlEnd
		case !strings.Contains(rest, "/"):
			// Single bare filename: prefix with the subject directory.
			b.WriteString(content[last:urlStart])
			b.WriteString(prefix)
			b.WriteString(rest)
			last = urlEnd
			changed = true
		default:
			// Some other relative path under images/: not ours to touch.
			b.WriteString(content[last:urlStart])
			b.WriteString(url)
			last = urlEnd
		}
	}
	b.WriteString(content[last:])
	newContent := b.String()
	if !changed {
		return nil
	}
	return os.WriteFile(mdFile, []byte(newContent), 0o644)
}

// buildImageSourceIndex indexes the source images directories by filename
// so step3 can resolve each referenced image to a source path. First-seen
// wins, matching the Python implementation's `if img_file.name not in
// img_source_map` semantics. The supportedExts filter is applied here.
// buildImageSourceIndex maps image basename -> source path across every
// mineru output directory. On a network share (the usual deployment) the
// directory listings dominate: 167 dirs / ~75k files over SMB took minutes
// when listed serially, with no output at all (T10: "会卡很久"). Now the
// listings run in parallel and a progress line reports movement — a slow
// scan the user can see is not a hang.
func buildImageSourceIndex(allDirs []string) map[string]string {
	idx := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 12) // cap concurrent listings; network dirs like some parallelism
	var scanned atomic.Int64
	var drew atomic.Bool
	total := len(allDirs)
	progressDone := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-t.C:
				drew.Store(true)
				fmt.Printf("    已扫描 %d/%d 个目录...\r", scanned.Load(), total)
			}
		}
	}()
	for _, d := range allDirs {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			srcImages := filepath.Join(d, "images")
			if !util.DirExists(srcImages) {
				scanned.Add(1)
				return
			}
			entries, err := os.ReadDir(srcImages)
			if err != nil {
				scanned.Add(1)
				return
			}
			local := make(map[string]string, len(entries))
			for _, e := range entries {
				if !e.Type().IsRegular() {
					continue
				}
				if !supportedExts[strings.ToLower(filepath.Ext(e.Name()))] {
					continue
				}
				local[e.Name()] = filepath.Join(srcImages, e.Name())
			}
			mu.Lock()
			for k, v := range local {
				if _, ok := idx[k]; !ok {
					idx[k] = v
				}
			}
			mu.Unlock()
			scanned.Add(1)
		}(d)
	}
	wg.Wait()
	close(progressDone)
	if drew.Load() {
		fmt.Println()
	}
	return idx
}

// uniqueImageRefs returns the distinct image references (e.g. "images/x.jpg")
// in input order, preserving first-seen position.
func uniqueImageRefs(matches [][]string) []string {
	seen := make(map[string]bool, len(matches))
	var out []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		out = append(out, m[1])
	}
	return out
}

func listMarkdown(dir string) []string {
	if !util.DirExists(dir) {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	return matches
}

// listOutputMarkdown lists the merged Markdown files in outputDir,
// excluding the .fp fingerprint sidecars.
func listOutputMarkdown(dir string) []string {
	if !util.DirExists(dir) {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil
	}
	sort.Strings(matches)
	var out []string
	for _, m := range matches {
		if strings.HasSuffix(m, fingerprintExt) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func countFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.Type().IsRegular() {
			n++
		}
	}
	return n, nil
}
