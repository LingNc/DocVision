// Package splitlog splits an img2text_*.log file into per-thread files
// by parsing the leading "[HH:MM:SS][Txx]" prefix on each line. Lines that
// do not match the prefix are grouped into "T00". Output files are written
// to a directory named after the input log (without the .log extension).
package splitlog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mineru-tools/internal/config"
)

// threadPat matches the leading "[HH:MM:SS][Txx]" prefix on a log line.
var threadPat = regexp.MustCompile(`^\[.*?\]\[(T\d+)\]`)

// FindLatestLog returns the path of the newest img2text_*.log file in
// logDir. Files are sorted by name in descending order, so the lexicographically
// largest filename (which embeds a timestamp) is returned.
func FindLatestLog(logDir string) (string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return "", fmt.Errorf("read log dir %s: %w", logDir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "img2text_") && strings.HasSuffix(name, ".log") {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no img2text_*.log found in %s", logDir)
	}

	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return filepath.Join(logDir, names[0]), nil
}

// SplitLogByThread reads the log file at logPath, groups lines by their
// leading thread ID, and writes one file per thread. If outputDir is
// non-empty, files are written there; otherwise a sibling directory named
// after the log (without the .log extension) is used.
func SplitLogByThread(logPath, outputDir string) error {
	f, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open log %s: %w", logPath, err)
	}
	defer f.Close()

	threadLines := make(map[string][]string)
	scanner := bufio.NewScanner(f)
	// Allow long log lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m := threadPat.FindStringSubmatch(line)
		tid := "T00"
		if m != nil {
			tid = m[1]
		}
		threadLines[tid] = append(threadLines[tid], line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read log %s: %w", logPath, err)
	}

	// Output directory: explicit override, else default to a sibling
	// directory named after the log's stem.
	if outputDir == "" {
		dir := filepath.Dir(logPath)
		base := strings.TrimSuffix(filepath.Base(logPath), ".log")
		outputDir = filepath.Join(dir, base)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create out dir %s: %w", outputDir, err)
	}

	tids := make([]string, 0, len(threadLines))
	for tid := range threadLines {
		tids = append(tids, tid)
	}
	sort.Strings(tids)

	base := strings.TrimSuffix(filepath.Base(logPath), ".log")
	for _, tid := range tids {
		outFile := filepath.Join(outputDir, fmt.Sprintf("%s_%s.log", tid, base))
		if err := writeLines(outFile, threadLines[tid]); err != nil {
			return err
		}
		fmt.Printf("  Wrote %d lines to %s\n", len(threadLines[tid]), filepath.Base(outFile))
	}

	fmt.Printf("\nSplit into %d thread files in %s/\n", len(threadLines), outputDir)
	return nil
}

// Run is the entry point used by the CLI: if logFile is empty, the latest
// img2text_*.log in cfg.Paths.FinallyDir is used. If outputDir is non-empty,
// it overrides the default sibling-directory placement. It prints a summary
// of the created files via SplitLogByThread.
func Run(cfg *config.Config, logFile, outputDir string) error {
	if logFile == "" {
		if cfg == nil {
			return fmt.Errorf("config and logFile are both empty")
		}
		latest, err := FindLatestLog(cfg.Paths.FinallyDir)
		if err != nil {
			return err
		}
		logFile = latest
		fmt.Printf("Using latest log: %s\n", filepath.Base(logFile))
	}

	if _, err := os.Stat(logFile); err != nil {
		return fmt.Errorf("log file not found: %s", logFile)
	}

	fmt.Printf("Splitting %s\n", logFile)
	return SplitLogByThread(logFile, outputDir)
}

func writeLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, l := range lines {
		if _, err := w.WriteString(l); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if !strings.HasSuffix(l, "\n") {
			if _, err := w.WriteString("\n"); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}
	}
	return w.Flush()
}
