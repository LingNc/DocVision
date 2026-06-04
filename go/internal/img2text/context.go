package img2text

import (
	"fmt"
	"strings"
)

// GetContextLines returns the lines surrounding the image at imgIdx,
// prefixed with line markers. The image line itself is marked with
// ">>>[IMG] " and other lines with "[L<i>] ". Output mirrors the Python
// reference exactly:
//
//	for i in range(start, end):
//	    prefix = ">>>[IMG] " if i == img_idx else f"[L{i}] "
//	    parts.append(prefix + lines[i])
func GetContextLines(lines []string, imgIdx, up, down int) []string {
	start := imgIdx - up
	if start < 0 {
		start = 0
	}
	end := imgIdx + down + 1
	if end > len(lines) {
		end = len(lines)
	}
	parts := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		var prefix string
		if i == imgIdx {
			prefix = ">>>[IMG] "
		} else {
			prefix = fmt.Sprintf("[L%d] ", i)
		}
		parts = append(parts, prefix+lines[i])
	}
	return parts
}

// GetDeltaLines returns ONLY the new lines that fall in the expansion from
// (oldUp, oldDown) to (newUp, newDown). Lines added above use "[L<i>] "
// prefixes; lines added below use the same. Empty result means the new
// window did not actually grow (e.g. bounded by the document edges).
func GetDeltaLines(lines []string, imgIdx, oldUp, oldDown, newUp, newDown int) []string {
	oldStart := imgIdx - oldUp
	if oldStart < 0 {
		oldStart = 0
	}
	oldEnd := imgIdx + oldDown + 1
	if oldEnd > len(lines) {
		oldEnd = len(lines)
	}
	newStart := imgIdx - newUp
	if newStart < 0 {
		newStart = 0
	}
	newEnd := imgIdx + newDown + 1
	if newEnd > len(lines) {
		newEnd = len(lines)
	}

	var parts []string
	// Lines added above (in document order).
	if newStart < oldStart {
		for i := newStart; i < oldStart; i++ {
			parts = append(parts, fmt.Sprintf("[L%d] %s", i, lines[i]))
		}
	}
	// Lines added below (in document order).
	if newEnd > oldEnd {
		for i := oldEnd; i < newEnd; i++ {
			parts = append(parts, fmt.Sprintf("[L%d] %s", i, lines[i]))
		}
	}
	return parts
}

// joinLines is a small helper used elsewhere in the package to render
// context slices with "\n" separators. Kept here for proximity to the
// other context helpers.
func joinLines(parts []string) string {
	return strings.Join(parts, "\n")
}
