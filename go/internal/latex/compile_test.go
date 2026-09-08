package latex

import (
	"strings"
	"testing"
)

func TestExtractLatexWarnings(t *testing.T) {
	logText := strings.Join([]string{
		"This is XeTeX",
		"LaTeX Warning: Reference `fig:1' undefined on input line 5.",
		"Package hyperref Warning: Token not allowed in a PDF string.",
		"Overfull \\hbox (12.34567pt too wide) in paragraph at lines 20--22",
		"! Undefined control sequence.",
		"l.42 \\badcommand",
		"LaTeX Warning: Reference `fig:1' undefined on input line 5.", // duplicate
	}, "\n")
	warnings, total := extractLatexWarnings(logText, 20)
	if total != 3 {
		t.Fatalf("total = %d, want 3 (deduplicated)", total)
	}
	res := CompileResult{Warnings: warnings, WarningCount: total}
	summary := res.WarningSummary()
	for _, want := range []string{"WARNINGS (3)", "Reference `fig:1'", "Overfull"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "Undefined control sequence") {
		t.Errorf("errors must not be reported as warnings:\n%s", summary)
	}
	if (CompileResult{}).WarningSummary() != "" {
		t.Error("no warnings must render an empty summary")
	}
}

func TestExtractLatexWarningsBoundsList(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString("Overfull \\hbox (1.0pt too wide) in paragraph at lines ")
		b.WriteString(strings.Repeat("x", i+1))
		b.WriteString("\n")
	}
	warnings, total := extractLatexWarnings(b.String(), 5)
	if total != 30 || len(warnings) != 5 {
		t.Fatalf("total=%d shown=%d, want 30/5", total, len(warnings))
	}
	if !strings.Contains(CompileResult{Warnings: warnings, WarningCount: total}.WarningSummary(), "showing first 5") {
		t.Error("summary should state the truncation")
	}
}
