package mineru

import (
	"strings"
	"testing"
)

// dataIDForUpload derives the value sent to the MinerU API as
// "files[].data_id". The API requires this field to be at most
// 128 characters. Long Chinese filenames exceed that limit when used
// verbatim, so the value is shortened for stems above 128 bytes while
// keeping a readable prefix and a stable collision-resistant suffix.

// TestDataIDForUpload_ShortStemUnchanged ensures stems already within
// the 128-char limit are passed through verbatim, so existing task
// state files keyed by the same stem keep working.
func TestDataIDForUpload_ShortStemUnchanged(t *testing.T) {
	stem := "short_file"
	if got := dataIDForUpload(stem); got != stem {
		t.Fatalf("dataIDForUpload(short) = %q, want %q", got, stem)
	}
}

// TestDataIDForUpload_LongStemCapped is the regression test for the
// MinerU API error "field files.data_id cannot exceed 128 characters".
// With a 200-char Chinese stem the unfixed code passed the full stem
// to the API, which then rejected the request.
func TestDataIDForUpload_LongStemCapped(t *testing.T) {
	// 200-character Chinese-style stem, comfortably over the 128 limit.
	stem := strings.Repeat("中", 200)
	got := dataIDForUpload(stem)
	if len(got) > dataIDMaxLen {
		t.Fatalf("dataIDForUpload(long) length = %d, want <= %d (stem=%q)", len(got), dataIDMaxLen, stem)
	}
	if got == stem {
		t.Fatalf("dataIDForUpload(long) returned the full stem; expected a shortened value")
	}
}

// TestDataIDForUpload_Stable ensures the same stem always maps to the
// same data_id. Without stability, resume-by-batch-id would break for
// any client that ever inspects data_id.
func TestDataIDForUpload_Stable(t *testing.T) {
	stem := strings.Repeat("学", 180)
	first := dataIDForUpload(stem)
	second := dataIDForUpload(stem)
	if first != second {
		t.Fatalf("dataIDForUpload is not stable: %q vs %q", first, second)
	}
	if len(first) > dataIDMaxLen {
		t.Fatalf("dataIDForUpload stable output length = %d, want <= %d", len(first), dataIDMaxLen)
	}
}

// TestDataIDForUpload_Unique ensures distinct stems produce distinct
// data_ids. Collisions would let one file overwrite another's result
// inside the same batch.
func TestDataIDForUpload_Unique(t *testing.T) {
	stemA := strings.Repeat("中", 200)
	stemB := strings.Repeat("国", 200)
	stemC := "中文" + strings.Repeat("文", 200) // same length, different content
	if dataIDForUpload(stemA) == dataIDForUpload(stemB) {
		t.Fatalf("distinct long stems A and B produced the same data_id")
	}
	if dataIDForUpload(stemA) == dataIDForUpload(stemC) {
		t.Fatalf("distinct long stems A and C produced the same data_id")
	}
	if dataIDForUpload(stemB) == dataIDForUpload(stemC) {
		t.Fatalf("distinct long stems B and C produced the same data_id")
	}
}

// TestDataIDForUpload_RealisticBugCase mirrors the kind of filename
// from the original bug report (a long Chinese stem with a duplicated
// title and a "_part" suffix) to lock the fix to the user-visible
// scenario. The stem is repeated to push it well past the 128-char
// cap and exercise the shortening path.
func TestDataIDForUpload_RealisticBugCase(t *testing.T) {
	stem := strings.Repeat("新时代中国特色社会主义思想概论", 8) + "_part1"
	if len(stem) <= dataIDMaxLen {
		t.Fatalf("test stem is unexpectedly short (%d chars); pick a longer one", len(stem))
	}
	got := dataIDForUpload(stem)
	if len(got) > dataIDMaxLen {
		t.Fatalf("dataIDForUpload(realistic) length = %d, want <= %d", len(got), dataIDMaxLen)
	}
}
