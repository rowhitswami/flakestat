package report

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/rowhitswami/flakestat/internal/score"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func sample() []score.Result {
	return []score.Result{
		{TestID: "1", Suite: "pytest", Class: "tests.api", Name: "test_login",
			Verdict: score.ClassFlaky, Score: 0.62, FlipRate: 0.58, Runs: 20, Passes: 14, Fails: 6},
		{TestID: "2", Suite: "pytest", Class: "tests.billing", Name: "test_charge",
			Verdict: score.ClassConsistentlyFail, Runs: 20, Passes: 0, Fails: 20},
		{TestID: "3", Suite: "pytest", Class: "tests.api", Name: "test_health",
			Verdict: score.ClassStable, Runs: 20, Passes: 20},
		{TestID: "4", Suite: "pytest", Class: "tests.api", Name: "test_new",
			Verdict: score.ClassInsufficient, Runs: 1, Passes: 1},
	}
}

func TestSummarizeCountsEveryVerdict(t *testing.T) {
	s := Summarize(sample())

	if s.Total != 4 {
		t.Errorf("Total = %d, want 4", s.Total)
	}
	if s.Flaky != 1 || s.ConsistentlyFailing != 1 || s.Stable != 1 || s.InsufficientData != 1 {
		t.Errorf("unexpected summary: %+v", s)
	}
}

// Stable and unscored tests are noise in the default view.
func TestTableHidesStableByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), Options{NoColor: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "test_health") {
		t.Error("stable test should be hidden by default")
	}
	if !strings.Contains(out, "test_login") {
		t.Error("flaky test should be shown")
	}
	// A consistently failing test must never be silently hidden.
	if !strings.Contains(out, "test_charge") {
		t.Error("consistently failing test should be shown")
	}
}

func TestTableAllIncludesStable(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), Options{NoColor: true, All: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	if !strings.Contains(buf.String(), "test_health") {
		t.Error("--all should include stable tests")
	}
}

// Regression guard: tabwriter counted ANSI escape bytes toward column width,
// so colored and uncolored rows drifted apart. Stripping the color from the
// colored render must reproduce the uncolored render exactly.
func TestColorDoesNotDisturbAlignment(t *testing.T) {
	var plain, colored bytes.Buffer

	if err := Table(&plain, sample(), Options{NoColor: true, All: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	if err := Table(&colored, sample(), Options{NoColor: false, All: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}

	stripped := ansi.ReplaceAllString(colored.String(), "")
	if stripped != plain.String() {
		t.Errorf("colored output does not match plain once ANSI is stripped:\n--- plain ---\n%s\n--- stripped ---\n%s",
			plain.String(), stripped)
	}
}

func TestTableColumnsAlign(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), Options{NoColor: true, All: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Header plus four rows, before the blank line and summary.
	var rows []string
	for _, l := range lines {
		if l == "" {
			break
		}
		rows = append(rows, l)
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5 (header + 4)", len(rows))
	}

	want := strings.Index(rows[0], "TEST")
	if want < 0 {
		t.Fatal("no TEST column in header")
	}
	for _, r := range rows[1:] {
		if got := strings.Index(r, "tests."); got != want {
			t.Errorf("test column starts at %d, want %d\nrow: %q", got, want, r)
		}
	}
}

func TestTableEmptyResults(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, nil, Options{NoColor: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	if !strings.Contains(buf.String(), "No flaky tests detected") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestTopLimitsRows(t *testing.T) {
	var buf bytes.Buffer
	if err := Table(&buf, sample(), Options{NoColor: true, All: true, Top: 2}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	if strings.Contains(buf.String(), "test_health") {
		t.Error("--top 2 should have excluded the third row")
	}
}

func TestJSONShape(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sample(), Options{}); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got struct {
		Summary Summary `json:"summary"`
		Results []struct {
			Name    string  `json:"name"`
			Verdict string  `json:"verdict"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if got.Summary.Total != 4 {
		t.Errorf("summary.total = %d, want 4", got.Summary.Total)
	}
	if len(got.Results) != 2 {
		t.Fatalf("results = %d, want 2 (stable and unscored filtered)", len(got.Results))
	}
	if got.Results[0].Verdict != string(score.ClassFlaky) {
		t.Errorf("first verdict = %q, want %q", got.Results[0].Verdict, score.ClassFlaky)
	}
}

// The summary must always reflect every test, even when rows are filtered.
func TestJSONSummaryCountsUnfilteredTotal(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sample(), Options{Top: 1}); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var got struct {
		Summary Summary       `json:"summary"`
		Results []interface{} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Summary.Total != 4 {
		t.Errorf("summary.total = %d, want 4 despite --top", got.Summary.Total)
	}
	if len(got.Results) != 1 {
		t.Errorf("results = %d, want 1", len(got.Results))
	}
}

func TestMarkdownRendersTable(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, sample(), Options{}); err != nil {
		t.Fatalf("Markdown: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "## flakestat") {
		t.Error("missing heading")
	}
	if !strings.Contains(out, "| Verdict | Score | Runs | Pass/Fail | Test |") {
		t.Error("missing table header")
	}
	if !strings.Contains(out, "test_login") {
		t.Error("missing flaky test row")
	}
}

// A pipe in a test name would otherwise break the markdown table.
func TestMarkdownEscapesPipes(t *testing.T) {
	results := []score.Result{{
		Name: "test_a|b", Class: "C", Suite: "s", Verdict: score.ClassFlaky, Runs: 10, Fails: 3,
	}}

	var buf bytes.Buffer
	if err := Markdown(&buf, results, Options{}); err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.Contains(buf.String(), `test_a\|b`) {
		t.Errorf("pipe was not escaped: %q", buf.String())
	}
}

func TestMarkdownEmptyResults(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, nil, Options{}); err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if !strings.Contains(buf.String(), "No flaky tests detected") {
		t.Errorf("unexpected output: %q", buf.String())
	}
}

func TestKnownFlakyIsLabelled(t *testing.T) {
	results := []score.Result{{
		Name: "testConcurrent", Class: "OrderTest", Suite: "s",
		Verdict: score.ClassFlaky, Runs: 1, Passes: 1, KnownFlaky: true,
	}}

	var buf bytes.Buffer
	if err := Table(&buf, results, Options{NoColor: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}
	if !strings.Contains(buf.String(), "framework-reported") {
		t.Errorf("framework-reported flake was not labelled: %q", buf.String())
	}
}

// A bare "no flaky tests detected" reads as "your suite is clean" when it often
// means "not at this sample size". The message must state its own limits.
func TestNoFindingsStatesDetectionFloor(t *testing.T) {
	stable := []score.Result{
		{Name: "a", Verdict: score.ClassStable, Runs: 12, Passes: 12},
		{Name: "b", Verdict: score.ClassStable, Runs: 12, Passes: 12},
	}

	var buf bytes.Buffer
	if err := Table(&buf, stable, Options{NoColor: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "12 run(s)") {
		t.Errorf("should name the run count it was working from:\n%s", out)
	}
	if !strings.Contains(out, "quarter of the time") {
		t.Errorf("should state the detection floor for 12 runs:\n%s", out)
	}
	if !strings.Contains(out, "--runs 50") {
		t.Errorf("should suggest how to look harder:\n%s", out)
	}
}

// With plenty of runs there is nothing more to suggest, so don't nag.
func TestNoFindingsAtHighRunCountDoesNotNag(t *testing.T) {
	stable := []score.Result{{Name: "a", Verdict: score.ClassStable, Runs: 100, Passes: 100}}

	var buf bytes.Buffer
	if err := Table(&buf, stable, Options{NoColor: true}); err != nil {
		t.Fatalf("Table: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "10% of the time") {
		t.Errorf("should state the tighter floor at 100 runs:\n%s", out)
	}
	if strings.Contains(out, "--runs 50") {
		t.Errorf("should not suggest more runs when already at 100:\n%s", out)
	}
}

func TestMedianRuns(t *testing.T) {
	got := medianRuns([]score.Result{
		{Runs: 5}, {Runs: 20}, {Runs: 12}, {Runs: 0},
	})
	if got != 12 {
		t.Errorf("medianRuns = %d, want 12 (zero-run tests excluded)", got)
	}
	if medianRuns(nil) != 0 {
		t.Error("medianRuns(nil) should be 0")
	}
}
