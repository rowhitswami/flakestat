package ci

import (
	"strings"
	"testing"

	"github.com/rowhitswami/flakestat/internal/baseline"
	"github.com/rowhitswami/flakestat/internal/score"
)

func res(name string, s float64, lvl score.Level, file string) score.Result {
	return score.Result{
		TestID: name, Name: name, Class: "suite", Suite: "s",
		Verdict: score.ClassFlaky, Score: s, Level: lvl, File: file,
		Runs: 30, Passes: 20, Fails: 10,
	}
}

func sample() (Report, baseline.Diff) {
	diff := baseline.Diff{
		New:       []score.Result{res("test_checkout_timeout", 0.81, score.LevelHigh, "tests/checkout.py")},
		Regressed: []score.Result{res("test_websocket_reconnect", 0.67, score.LevelMedium, "")},
		Fixed:     []baseline.Entry{{Name: "test_old_flake"}},
		Accepted:  []score.Result{res("test_known", 0.30, score.LevelHigh, "")},
	}
	base := baseline.New()
	base.Tests["test_websocket_reconnect"] = baseline.Entry{Name: "test_websocket_reconnect", Score: 0.40}

	all := append([]score.Result{}, diff.New...)
	all = append(all, diff.Regressed...)
	all = append(all, diff.Accepted...)
	return Build(all, base, diff, true, "1 newly flaky test(s)"), diff
}

func TestMarkdownReportsCountsAndChanges(t *testing.T) {
	r, _ := sample()
	md := r.Markdown(DefaultMaxRows)

	for _, want := range []string{
		Marker, "## flakestat", "Tests analyzed", "Flaky", "Consistently failing",
		"Always skipped", "Changes since the baseline", "New", "Regressed", "Fixed", "Accepted",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report should mention %q", want)
		}
	}
}

// The gate verdict must be reported, not recomputed here.
func TestMarkdownReportsGateOutcome(t *testing.T) {
	r, _ := sample()
	if md := r.Markdown(0); !strings.Contains(md, "FAILED") || !strings.Contains(md, "1 newly flaky") {
		t.Errorf("report should carry the gate result verbatim:\n%s", md)
	}

	r.GateFailed, r.GateReason = false, ""
	if md := r.Markdown(0); !strings.Contains(md, "passed") {
		t.Errorf("a passing gate should say so:\n%s", md)
	}
}

func TestHighlightsAreWorstFirstAndLabelled(t *testing.T) {
	r, _ := sample()
	if len(r.Highlights) != 2 {
		t.Fatalf("highlights = %d, want 2", len(r.Highlights))
	}
	if r.Highlights[0].Score < r.Highlights[1].Score {
		t.Error("highlights should be ordered worst first")
	}
	if r.Highlights[0].Status != StatusNew {
		t.Errorf("status = %q, want %q", r.Highlights[0].Status, StatusNew)
	}
	if !strings.Contains(r.Markdown(0), "high") {
		t.Error("the highlight table should carry confidence")
	}
}

// A suite with hundreds of new flakes must summarize, not flood.
func TestHighlightTableIsBounded(t *testing.T) {
	var many []score.Result
	for i := 0; i < 50; i++ {
		many = append(many, res(strings.Repeat("t", i+1), 0.5, score.LevelHigh, ""))
	}
	r := Build(many, baseline.New(), baseline.Diff{New: many}, false, "")

	md := r.Markdown(5)
	if rows := strings.Count(md, "| New flaky |"); rows != 5 {
		t.Errorf("rendered %d highlight rows, want 5", rows)
	}
	if !strings.Contains(md, "and 45 more") {
		t.Errorf("truncation should be stated, got:\n%s", md)
	}
}

// Inventing a source location would point reviewers at the wrong line.
func TestAnnotationsOnlyWhereFileIsKnown(t *testing.T) {
	r, _ := sample()
	got := r.Annotations(DefaultMaxRows)

	if len(got) != 1 {
		t.Fatalf("annotations = %d, want 1 (only the test with a file)", len(got))
	}
	if !strings.Contains(got[0], "file=tests/checkout.py") {
		t.Errorf("annotation should carry the reported file: %s", got[0])
	}
	if !strings.HasPrefix(got[0], "::warning ") {
		t.Errorf("annotation should be a workflow command: %s", got[0])
	}
}

// A test lacking file metadata still belongs in the written report.
func TestTestsWithoutFilesStillAppearInMarkdown(t *testing.T) {
	r, _ := sample()
	if !strings.Contains(r.Markdown(0), "test_websocket_reconnect") {
		t.Error("a test without a source file must still be reported")
	}
}

func TestAnnotationsEscapeWorkflowDelimiters(t *testing.T) {
	r := Build(nil, baseline.New(), baseline.Diff{
		New: []score.Result{res("weird::name,with:delims", 0.5, score.LevelHigh, "a.py")},
	}, false, "")

	got := r.Annotations(10)
	if len(got) != 1 {
		t.Fatalf("annotations = %d, want 1", len(got))
	}
	// Raw ':' or ',' in the message would truncate the workflow command.
	msg := got[0][strings.Index(got[0], "::")+2:]
	if strings.Contains(msg, "::name") || strings.Contains(msg, ",with") {
		t.Errorf("delimiters should be escaped: %s", got[0])
	}
}

func TestAnnotationCountIsBounded(t *testing.T) {
	var many []score.Result
	for i := 0; i < 40; i++ {
		many = append(many, res(strings.Repeat("t", i+1), 0.5, score.LevelHigh, "a.py"))
	}
	r := Build(many, baseline.New(), baseline.Diff{New: many}, false, "")

	if got := len(r.Annotations(10)); got != 10 {
		t.Errorf("annotations = %d, want them capped at 10", got)
	}
}

// The marker is what lets a re-run update one comment instead of adding more.
func TestReportCarriesAStableMarker(t *testing.T) {
	r, _ := sample()
	if !strings.HasPrefix(r.Markdown(0), Marker) {
		t.Error("the report must start with the marker used to find an existing comment")
	}
}
