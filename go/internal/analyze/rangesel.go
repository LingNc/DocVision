package analyze

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	logNameRe = regexp.MustCompile(`(?:img2text|latex)_(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})\.log`)
	relSpecRe = regexp.MustCompile(`^(\d+[YMWDdhms])+$`)
	relTokRe  = regexp.MustCompile(`(\d+)([YMWDdhms])`)
)

// logNameTime extracts the run start time encoded in an img2text log
// filename like img2text_20260902_144733.log. Returns ok=false when the
// name does not carry a parseable timestamp.
func logNameTime(path string) (time.Time, bool) {
	base := filepath.Base(path)
	m := logNameRe.FindStringSubmatch(base)
	if m == nil {
		return time.Time{}, false
	}
	y, _ := strconv.Atoi(m[1])
	var v [6]int
	for i := 2; i <= 6; i++ {
		v[i-2], _ = strconv.Atoi(m[i])
	}
	t := time.Date(y, time.Month(v[0]), v[1], v[2], v[3], v[4], 0, time.Local)
	return t, true
}

// parseDurationToken parses a relative offset like "+2d", "+1d12h", "48h".
// Units: Y year, M month, D/d day, W week, H/h hour, m minute, s/S second.
// Returns the total duration and true when the whole string consists of
// duration tokens (a leading "+" is allowed).
func parseDurationToken(s string) (time.Duration, bool, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "+")
	if s == "" {
		return 0, false, nil
	}
	if !relSpecRe.MatchString(s) {
		return 0, false, nil
	}
	var d time.Duration
	now := time.Now()
	for _, m := range relTokRe.FindAllStringSubmatch(s, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, true, fmt.Errorf("数字无效: %q", m[1])
		}
		switch m[2] {
		case "Y":
			d += now.AddDate(n, 0, 0).Sub(now)
		case "M":
			d += now.AddDate(0, n, 0).Sub(now)
		case "W":
			d += time.Duration(n) * 7 * 24 * time.Hour
		case "D", "d":
			d += time.Duration(n) * 24 * time.Hour
		case "H", "h":
			d += time.Duration(n) * time.Hour
		case "m":
			d += time.Duration(n) * time.Minute
		case "S", "s":
			d += time.Duration(n) * time.Second
		}
	}
	return d, true, nil
}

// parseTimePoint parses one endpoint of a time range. Supported forms:
//   - absolute date: 2026Y9M2D / 2026-09-02 / 20260902 (time defaults 00:00:00)
//   - absolute date+time: 2026Y9M2D15H30m, 20260902_1530 / 20260902-153000,
//     2026-09-02 15:30:12
//   - relative to now: +2d, +1d12h, 48h
func parseTimePoint(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("时间点为空")
	}
	absRe := regexp.MustCompile(`^(\d{4})Y(\d{1,2})M(\d{1,2})D(?:[_ ]?(\d{1,2})H(?:(\d{1,2})m)?(?:(\d{1,2})s)?)?$`)
	if m := absRe.FindStringSubmatch(s); m != nil {
		return buildTime(m[1], m[2], m[3], m[4], m[5], m[6])
	}
	digits := regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(?:[_-](\d{2})(\d{2})(\d{2})?)?$`)
	if m := digits.FindStringSubmatch(s); m != nil {
		return buildTime(m[1], m[2], m[3], m[4], m[5], m[6])
	}
	dash := regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})(?:[ _](\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?$`)
	if m := dash.FindStringSubmatch(s); m != nil {
		return buildTime(m[1], m[2], m[3], m[4], m[5], m[6])
	}
	if d, ok, err := parseDurationToken(s); err != nil {
		return time.Time{}, err
	} else if ok {
		return now.Add(-d), nil
	}

	return time.Time{}, fmt.Errorf("无法解析时间点 %q（示例: 2026Y9M2D / 2026-09-02 / 20260902_1530 / +2d）", s)
}

// dateOnly reports whether s is a pure date with no time component.
func dateOnly(s string) bool {
	s = strings.TrimSpace(s)
	for _, re := range []*regexp.Regexp{regexp.MustCompile(`^\d{8}$`), regexp.MustCompile(`^\d{4}Y\d{1,2}M\d{1,2}D$`), regexp.MustCompile(`^\d{4}-\d{1,2}-\d{1,2}$`)} {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func buildTime(y, mo, d, h, mi, se string) (time.Time, error) {
	atoiOr := func(s string, def int) int {
		if s == "" {
			return def
		}
		v, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return v
	}
	return time.Date(atoiOr(y, 0), time.Month(atoiOr(mo, 1)), atoiOr(d, 1),
		atoiOr(h, 0), atoiOr(mi, 0), atoiOr(se, 0), 0, time.Local), nil
}

// ParseRoundSpec parses a round selection like "2", "1-3", "-2", "2-".
// Round 0 is the latest log, 1 the previous one, and so on. Returns the
// inclusive [lo, hi] round numbers; open ends become lo=0 or hi=count-1.
func ParseRoundSpec(spec string, count int) (lo, hi int, err error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return 0, 0, fmt.Errorf("轮次为空")
	}
	if !strings.Contains(spec, "-") {
		n, e := strconv.Atoi(spec)
		if e != nil || n < 0 {
			return 0, 0, fmt.Errorf("轮次无效: %q（0=最新一次, 1=上一次, ...）", spec)
		}
		return n, n, nil
	}
	parts := strings.SplitN(spec, "-", 2)
	loS, hiS := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	lo, hi = 0, count-1
	if loS != "" {
		lo, err = strconv.Atoi(loS)
		if err != nil || lo < 0 {
			return 0, 0, fmt.Errorf("起始轮次无效: %q", loS)
		}
	}
	if hiS != "" {
		hi, err = strconv.Atoi(hiS)
		if err != nil || hi < 0 {
			return 0, 0, fmt.Errorf("结束轮次无效: %q", hiS)
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi, nil
}

// selectLogsByRounds picks logs by round range (0=latest) from the
// newest-first list.
func selectLogsByRounds(paths []string, lo, hi int) []string {
	n := len(paths)
	if hi >= n {
		hi = n - 1
	}
	if lo < 0 {
		lo = 0
	}
	if lo > hi || n == 0 {
		return nil
	}
	return paths[lo : hi+1]
}

// selectLogsByTime filters logs whose filename timestamp falls in [from, to].
func selectLogsByTime(paths []string, from, to time.Time) []string {
	var out []string
	for _, p := range paths {
		t, ok := logNameTime(p)
		if !ok {
			continue
		}
		if !t.Before(from) && !t.After(to) {
			out = append(out, p)
		}
	}
	return out
}

// sortNewestFirst returns a newest-first copy of paths.
func sortNewestFirst(paths []string) []string {
	sorted := make([]string, len(paths))
	copy(sorted, paths)
	sort.Strings(sorted)
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}
	return sorted
}

// SelectLogs applies an optional round spec and time range spec to the
// newest-first list of candidate logs. Returns the selected paths (newest
// first) and a human-readable description of the applied filter.
func SelectLogs(paths []string, roundSpec, timeSpec string) ([]string, string, error) {
	if len(paths) == 0 {
		return nil, "", fmt.Errorf("没有可用日志")
	}
	sel := sortNewestFirst(paths)
	var desc string
	if roundSpec != "" {
		lo, hi, err := ParseRoundSpec(roundSpec, len(sel))
		if err != nil {
			return nil, "", err
		}
		sel = selectLogsByRounds(sel, lo, hi)
		if len(sel) == 0 {
			return nil, "", fmt.Errorf("轮次 %q 没有选中任何日志（共有 %d 个）", roundSpec, len(sel))
		}
		if lo == hi {
			desc = fmt.Sprintf("按轮次: 第 %d 次（0=最新）", lo)
		} else {
			desc = fmt.Sprintf("按轮次范围: 第 %d ~ %d 次（0=最新）", lo, hi)
		}
	}
	if timeSpec != "" {
		from, to, err := parseTimeRangeSpec(timeSpec)
		if err != nil {
			return nil, "", err
		}
		sel = selectLogsByTime(sel, from, to)
		if len(sel) == 0 {
			return nil, "", fmt.Errorf("时间范围 %q 没有选中任何日志", timeSpec)
		}
		d := fmt.Sprintf("按时间: %s ~ %s", from.Format("2006-01-02 15:04:05"), to.Format("2006-01-02 15:04:05"))
		if desc != "" {
			desc += "；" + d
		} else {
			desc = d
		}
	}
	return sel, desc, nil
}

// parseTimeRangeSpec parses "a-b", "a-", "-b", or a bare offset like "+2d"
// (from 2 days ago to now). Each endpoint goes through parseTimePoint.
func parseTimeRangeSpec(spec string) (from, to time.Time, err error) {
	spec = strings.TrimSpace(spec)
	now := time.Now()
	if d, ok, e := parseDurationToken(spec); e != nil {
		return time.Time{}, time.Time{}, e
	} else if ok {
		return now.Add(-d), now, nil
	}
	idx := strings.Index(spec, "-")
	if idx < 0 {
		t, e := parseTimePoint(spec, now)
		if e != nil {
			return time.Time{}, time.Time{}, e
		}
		return t, now, nil
	}
	a, b := strings.TrimSpace(spec[:idx]), strings.TrimSpace(spec[idx+1:])
	if a == "" && b == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("时间范围为空")
	}
	if a == "" {
		to, e := parseTimePoint(b, now)
		if e != nil {
			return time.Time{}, time.Time{}, e
		}
		return time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local), to, nil
	}
	from, e := parseTimePoint(a, now)
	if e != nil {
		return time.Time{}, time.Time{}, e
	}
	if b == "" {
		return from, now, nil
	}
	var e2 error
	to, e2 = parseTimePoint(b, now)
	if e2 != nil {
		return time.Time{}, time.Time{}, e2
	}
	// A date-only endpoint means the whole day (inclusive).
	if dateOnly(b) {
		to = to.Add(24*time.Hour - time.Second)
	}
	return from, to, nil
}
