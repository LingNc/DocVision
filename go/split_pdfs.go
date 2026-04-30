package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const (
	defaultMaxPages = 200
	defaultOutputDir = "split_parts"
)

func splitPDF(pdfPath string, maxPages int, outputDir string) error {
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return fmt.Errorf("文件不存在: %s", pdfPath)
	}

	os.MkdirAll(outputDir, 0755)

	conf := model.NewDefaultConfiguration()

	// 获取 PDF 页数
	ctx, err := api.ReadContextFile(pdfPath)
	if err != nil {
		return fmt.Errorf("读取 PDF 失败: %w", err)
	}
	totalPages := ctx.PageCount

	baseName := strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))
	fmt.Printf("[信息] %s: 共 %d 页\n", filepath.Base(pdfPath), totalPages)

	part := 1
	startPage := 1

	for startPage <= totalPages {
		endPage := startPage + maxPages - 1
		if endPage > totalPages {
			endPage = totalPages
		}

		pageRange := fmt.Sprintf("%d-%d", startPage, endPage)
		partName := fmt.Sprintf("%s_part%d.pdf", baseName, part)
		partPath := filepath.Join(outputDir, partName)

		// 打开源文件
		f, err := os.Open(pdfPath)
		if err != nil {
			return fmt.Errorf("打开 PDF 失败: %w", err)
		}

		// 创建输出文件
		out, err := os.Create(partPath)
		if err != nil {
			f.Close()
			return fmt.Errorf("创建输出文件失败: %w", err)
		}

		// 使用 api.Collect 提取指定页范围并写入输出文件
		err = api.Collect(f, out, []string{pageRange}, conf)
		f.Close()
		out.Close()

		if err != nil {
			return fmt.Errorf("分割失败: %w", err)
		}

		fmt.Printf("  → %s  (页 %d–%d, 共 %d 页)\n", partName, startPage, endPage, endPage-startPage+1)

		startPage = endPage + 1
		part++
	}

	fmt.Printf("[完成] 共分割为 %d 个部分，输出到 %s/\n\n", part-1, outputDir)
	return nil
}

func main() {
	allFlag := flag.Bool("all", false, "分割当前目录下所有 PDF")
	maxPages := flag.Int("max-pages", defaultMaxPages, fmt.Sprintf("每部分最大页数（默认 %d）", defaultMaxPages))
	outputDir := flag.String("output-dir", defaultOutputDir, fmt.Sprintf("输出目录（默认 %s/）", defaultOutputDir))
	flag.Parse()

	cwd, _ := os.Getwd()

	var pdfFiles []string

	if *allFlag {
		entries, err := os.ReadDir(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[错误] 读取目录失败: %v\n", err)
			os.Exit(1)
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".pdf") && !strings.HasPrefix(entry.Name(), ".") {
				pdfFiles = append(pdfFiles, filepath.Join(cwd, entry.Name()))
			}
		}

		sort.Strings(pdfFiles)

		if len(pdfFiles) == 0 {
			fmt.Fprintln(os.Stderr, "[错误] 当前目录下没有找到 PDF 文件")
			os.Exit(1)
		}

		fmt.Printf("找到 %d 个 PDF 文件\n\n", len(pdfFiles))
	} else {
		args := flag.Args()
		if len(args) == 0 {
			fmt.Println("PDF 分割脚本 —— 将大 PDF 按不超过指定页数分割为多个部分。")
			fmt.Println()
			fmt.Println("用法:")
			fmt.Println("    # 分割单个 PDF")
			fmt.Println("    ./split_pdfs 2027数据结构_高清带书签版.pdf")
			fmt.Println()
			fmt.Println("    # 分割目录下所有 PDF")
			fmt.Println("    ./split_pdfs --all")
			fmt.Println()
			fmt.Println("    # 指定最大页数（默认 200）")
			fmt.Println("    ./split_pdfs 2027数据结构_高清带书签版.pdf --max-pages 150")
			fmt.Println()
			fmt.Println("输出目录: ./split_parts/")
			os.Exit(1)
		}

		pdfPath := args[0]
		if !filepath.IsAbs(pdfPath) {
			pdfPath = filepath.Join(cwd, pdfPath)
		}
		pdfFiles = append(pdfFiles, pdfPath)
	}

	for _, pdfPath := range pdfFiles {
		if err := splitPDF(pdfPath, *maxPages, *outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "[错误] %v\n", err)
		}
	}
}
