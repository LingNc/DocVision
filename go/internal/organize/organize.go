// Package organize copies full.md files from the MinerU output directory,
// merges split parts by subject, and collects referenced images into per-
// subject subdirectories. It mirrors python/organize_files.py exactly.
package organize

import (
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
var imgRe = regexp.MustCompile(`!\[.*?\]\((images/.+?\.(?:jpg|jpeg|png|gif|webp))\)`)

const mdSeparator = "\n\n---\n\n"

// OrganizeFiles runs the four-step file organisation pipeline:
//  1. copy full.md -> output/temp/{name}.md
//  2. merge split parts -> output/{subject}.md
//  3. collect referenced images -> images/{subject}/
//  4. print summary
func OrganizeFiles(cfg *config.Config) error {
	mineruOutput := cfg.Paths.MineruOutput
	outputDir := cfg.Paths.OutputDir
	imagesDir := cfg.Paths.ImagesDir
	tempDir := filepath.Join(outputDir, "temp")

	if !util.DirExists(mineruOutput) {
		return fmt.Errorf("MinerU 输出目录不存在: %s", mineruOutput)
	}

	// Clean the entire output directory on each run, then recreate.
	if util.DirExists(outputDir) {
		if err := os.RemoveAll(outputDir); err != nil {
			return fmt.Errorf("清理输出目录失败: %w", err)
		}
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("创建 temp 目录失败: %w", err)
	}
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return fmt.Errorf("创建 images 目录失败: %w", err)
	}

	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println("  MinerU 文件整理工具")
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println()

	// groups[subject] = sorted list of part numbers (empty for single-file subjects).
	groups, allDirs, err := step1CopyMarkdown(mineruOutput, tempDir)
	if err != nil {
		return err
	}

	if err := step2MergeParts(groups, tempDir, outputDir); err != nil {
		return err
	}

	if err := step3CollectImages(allDirs, outputDir, imagesDir); err != nil {
		return err
	}

	step4Summary(outputDir, tempDir, imagesDir)
	return nil
}

// step1CopyMarkdown walks the MinerU output, copying every full.md into
// temp/ and grouping part directories by subject.
func step1CopyMarkdown(mineruOutput, tempDir string) (map[string][]string, []string, error) {
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
func step2MergeParts(groups map[string][]string, tempDir, outputDir string) error {
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
		dstFile := filepath.Join(outputDir, subject+".md")

		// Final file already exists: skip (user can delete it to regenerate).
		if util.FileExists(dstFile) {
			skipCount++
			continue
		}

		if len(parts) == 0 {
			// Single-file (non-split) subject: copy the temp file as-is.
			src := filepath.Join(tempDir, subject+".md")
			if util.FileExists(src) {
				if err := util.CopyFile(src, dstFile); err != nil {
					return fmt.Errorf("复制 %s -> %s 失败: %w", src, dstFile, err)
				}
				fmt.Printf("  %s.md (单文件)\n", subject)
			}
			continue
		}

		// Sort parts numerically.
		sort.Slice(parts, func(i, j int) bool {
			ai, _ := strconv.Atoi(parts[i])
			aj, _ := strconv.Atoi(parts[j])
			return ai < aj
		})

		if len(parts) == 1 {
			// Single split: just copy through.
			src := filepath.Join(tempDir, subject+"_"+parts[0]+".md")
			if util.FileExists(src) {
				if err := util.CopyFile(src, dstFile); err != nil {
					return fmt.Errorf("复制 %s -> %s 失败: %w", src, dstFile, err)
				}
				fmt.Printf("  %s.md (单分片)\n", subject)
			}
			continue
		}

		fmt.Printf("  合并: %s (parts %v)\n", subject, parts)

		var contents []string
		for _, p := range parts {
			partFile := filepath.Join(tempDir, subject+"_"+p+".md")
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

		if err := os.WriteFile(dstFile, []byte(strings.Join(contents, mdSeparator)), 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", dstFile, err)
		}
		mergeCount++
	}

	fmt.Printf("  合并: %d, 跳过: %d\n", mergeCount, skipCount)
	fmt.Println()
	return nil
}

// step3CollectImages rewrites markdown image paths to point at
// images/{subject}/ and copies each referenced file from any mineru
// source directory into that location.
func step3CollectImages(allDirs []string, outputDir, imagesDir string) error {
	fmt.Printf("[3/4] 收集引用的图片 -> %s/ ...\n", imagesDir)

	// Index image sources by filename. First-seen wins, matching the
	// Python implementation's `if img_file.name not in img_source_map`.
	imgSourceMap := make(map[string]string)
	for _, d := range allDirs {
		srcImages := filepath.Join(d, "images")
		if !util.DirExists(srcImages) {
			continue
		}
		entries, err := os.ReadDir(srcImages)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", srcImages, err)
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			if _, ok := imgSourceMap[e.Name()]; ok {
				continue
			}
			imgSourceMap[e.Name()] = filepath.Join(srcImages, e.Name())
		}
	}

	imgCount := 0
	skipSubject := 0
	missing := 0

	mdFiles, err := filepath.Glob(filepath.Join(outputDir, "*.md"))
	if err != nil {
		return fmt.Errorf("查找 %s/*.md 失败: %w", outputDir, err)
	}
	sort.Strings(mdFiles)

	for _, mdFile := range mdFiles {
		subject := strings.TrimSuffix(filepath.Base(mdFile), filepath.Ext(mdFile))
		subjectImgDir := filepath.Join(imagesDir, subject)

		// Skip subjects whose image directory is already populated.
		if util.DirExists(subjectImgDir) {
			hasContent, err := dirHasContent(subjectImgDir)
			if err != nil {
				return err
			}
			if hasContent {
				skipSubject++
				continue
			}
		}

		data, err := os.ReadFile(mdFile)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", mdFile, err)
		}
		content := string(data)

		refs := uniqueImageRefs(imgRe.FindAllStringSubmatch(content, -1))
		if len(refs) == 0 {
			continue
		}

		if err := os.MkdirAll(subjectImgDir, 0o755); err != nil {
			return fmt.Errorf("创建 %s 失败: %w", subjectImgDir, err)
		}
		collected := 0

		for _, ref := range refs {
			imgName := ref
			if idx := strings.Index(ref, "/"); idx >= 0 {
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
				fmt.Printf("  警告: [%s] 找不到图片源 %s\n", subject, ref)
				continue
			}
			if err := util.CopyFile(srcImg, dstImg); err != nil {
				return fmt.Errorf("复制 %s -> %s 失败: %w", srcImg, dstImg, err)
			}
			imgCount++
			collected++
		}

		// Rewrite paths: images/xxx.jpg -> images/{subject}/xxx.jpg.
		newContent := strings.ReplaceAll(content, "images/", "images/"+subject+"/")
		if newContent != content {
			if err := os.WriteFile(mdFile, []byte(newContent), 0o644); err != nil {
				return fmt.Errorf("写入 %s 失败: %w", mdFile, err)
			}
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
func step4Summary(outputDir, tempDir, imagesDir string) {
	fmt.Println("[4/4] 汇总")
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println()

	fmt.Printf("  %s/ (合并后):\n", outputDir)
	for _, md := range listMarkdown(outputDir) {
		fmt.Printf("    %s  (%.1f KB)\n", filepath.Base(md), float64(fileSize(md))/1024)
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
				count, _ := countFiles(filepath.Join(imagesDir, e.Name()))
				imgTotal += count
				fmt.Printf("    %s/ (%d 张)\n", e.Name(), count)
			}
		}
	}
	fmt.Printf("    共 %d 张图片\n", imgTotal)
	fmt.Println("=" + strings.Repeat("=", 49))
	fmt.Println("完成!")
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

func dirHasContent(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(entries) > 0, nil
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
