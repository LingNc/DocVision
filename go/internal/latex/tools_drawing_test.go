package latex

import "testing"

func TestMatchImageRefAcceptsBareNames(t *testing.T) {
	refs, _ := scanImageRefs("text\n\n![a](images/测试-概率论/fig1.jpg)\n\n![b](images/其他/fig2.png)\n")
	cases := []struct {
		target string
		want   int
	}{
		{"images/测试-概率论/fig1.jpg", 0},
		{"测试-概率论/fig1.jpg", 0},
		{"fig1.jpg", 0},
		{"images/fig1.jpg", 0},
		{"fig2.png", 1},
		{"其他/fig2.png", 1},
		{"missing.jpg", -1},
		{"", -1},
	}
	for _, c := range cases {
		if got := matchImageRef(refs, c.target); got != c.want {
			t.Errorf("matchImageRef(%q) = %d, want %d", c.target, got, c.want)
		}
	}
}

func TestMatchImageRefRejectsAmbiguousBareName(t *testing.T) {
	refs, _ := scanImageRefs("![a](images/s1/fig.jpg)\n\n![b](images/s2/fig.jpg)\n")
	if got := matchImageRef(refs, "fig.jpg"); got != -1 {
		t.Errorf("ambiguous bare name must not match, got %d", got)
	}
	// The full ref still resolves unambiguously.
	if got := matchImageRef(refs, "images/s2/fig.jpg"); got != 1 {
		t.Errorf("full ref = %d, want 1", got)
	}
}

func TestImageContextBareNameAndDeltas(t *testing.T) {
	content := "line1\n![a](images/sub/a.jpg)\nline3\nline4\n![b](images/sub/b.jpg)\n"
	tool := &ImageContextTool{Content: content, CurrentImg: "images/sub/b.jpg"}
	res, err := tool.Execute(`{"image":"a.jpg","up":1,"down":1}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"image 1 of 2", "images/sub/a.jpg", "+3 lines from this image", "## NEXT image ref"} {
		if !containsStr(res.Text, want) {
			t.Errorf("result missing %q:\n%s", want, res.Text)
		}
	}
}

func TestImageSubject(t *testing.T) {
	if got := imageSubject("images/测试-概率论/fig.jpg"); got != "测试-概率论" {
		t.Errorf("subject = %q", got)
	}
	if got := imageSubject("images/fig.jpg"); got != "" {
		t.Errorf("subject = %q, want empty", got)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
