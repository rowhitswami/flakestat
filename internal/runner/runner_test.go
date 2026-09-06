package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
)

// writeReportCmd returns a shell command that writes a minimal JUnit report to
// path, with the given case status.
func writeReportCmd(path, status string) []string {
	body := `<testcase classname="C" name="t"/>`
	if status == "fail" {
		body = `<testcase classname="C" name="t"><failure message="boom"/></testcase>`
	}
	xml := `<testsuite name="s" tests="1">` + body + `</testsuite>`
	return []string{"sh", "-c", fmt.Sprintf("printf '%%s' '%s' > '%s'", xml, path)}
}

func TestRunsExactlyNTimes(t *testing.T) {
	res, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "exit 0"},
		Runs:    5,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res) != 5 {
		t.Errorf("results = %d, want 5", len(res))
	}
}

// Each run must write to its own report file, and all must be parsed.
func TestRunPlaceholderExpansion(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "junit-"+RunPlaceholder+".xml")

	res, err := Run(context.Background(), Options{
		Command:      writeReportCmd(pattern, "pass"),
		Runs:         3,
		Parallel:     3,
		JUnitPattern: pattern,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, r := range res {
		if r.ReportMissing {
			t.Errorf("run %d: report missing at %s", r.Index, r.ReportPath)
		}
		if len(r.Cases) != 1 {
			t.Errorf("run %d: cases = %d, want 1", r.Index, len(r.Cases))
		}
	}

	for i := 1; i <= 3; i++ {
		p := filepath.Join(dir, fmt.Sprintf("junit-%d.xml", i))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected per-run report at %s: %v", p, err)
		}
	}
}

// Parallel runs sharing one report path would overwrite each other, so this
// must be rejected up front rather than silently producing wrong data.
func TestParallelRequiresRunPlaceholder(t *testing.T) {
	err := Options{
		Command:      []string{"true"},
		Runs:         4,
		Parallel:     4,
		JUnitPattern: "reports/junit.xml",
	}.Validate()

	if err == nil {
		t.Fatal("expected an error for parallel runs without {run}")
	}
	if !strings.Contains(err.Error(), RunPlaceholder) {
		t.Errorf("error should name the %s placeholder, got: %v", RunPlaceholder, err)
	}
}

func TestValidateRejectsEmptyCommand(t *testing.T) {
	if err := (Options{Runs: 1}).Validate(); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

// With no JUnit report, granularity is lost but the exit code still tells us
// whether the suite passed.
func TestMissingReportFallsBackToExitCode(t *testing.T) {
	dir := t.TempDir()

	res, err := Run(context.Background(), Options{
		Command:      []string{"sh", "-c", "exit 3"},
		Runs:         1,
		JUnitPattern: filepath.Join(dir, "never-written.xml"),
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !res[0].ReportMissing {
		t.Error("ReportMissing = false, want true")
	}
	if res[0].ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res[0].ExitCode)
	}
	if !res[0].Failed() {
		t.Error("Failed() = false, want true for a non-zero exit with no report")
	}

	c := SyntheticCase(res[0])
	if c.Status != junit.StatusFail {
		t.Errorf("SyntheticCase status = %q, want fail", c.Status)
	}
}

func TestUntilFailStopsEarly(t *testing.T) {
	res, err := Run(context.Background(), Options{
		Command:   []string{"sh", "-c", "exit 1"},
		Runs:      20,
		Parallel:  1,
		UntilFail: true,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res) >= 20 {
		t.Errorf("results = %d, want fewer than 20 with --until-fail", len(res))
	}
	if len(res) == 0 {
		t.Fatal("expected at least one run")
	}
}

// A stale report from a previous run must not be scored against this one.
func TestStaleReportIsRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "junit.xml")

	if err := os.WriteFile(path, []byte(`<testsuite name="old"><testcase classname="C" name="stale"/></testsuite>`), 0o644); err != nil {
		t.Fatalf("seed stale report: %v", err)
	}

	res, err := Run(context.Background(), Options{
		Command:      []string{"sh", "-c", "exit 0"},
		Runs:         1,
		JUnitPattern: path,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res[0].ReportMissing {
		t.Error("stale report was reused instead of being cleared")
	}
}

func TestRunNumberIsExportedToCommand(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "runs.txt")

	_, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "echo $FLAKESTAT_RUN >> " + out},
		Runs:    3,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, want := range []string{"1", "2", "3"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("FLAKESTAT_RUN=%s was not exported; got %q", want, data)
		}
	}
}

func TestTimeoutIsReportedAsError(t *testing.T) {
	res, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "sleep 5"},
		Runs:    1,
		Timeout: 100 * time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res[0].Err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.Contains(res[0].Err.Error(), "timed out") {
		t.Errorf("error = %v, want a timeout", res[0].Err)
	}
}

func TestFailedDetectsFailingCase(t *testing.T) {
	r := Result{Cases: []junit.Case{
		{Name: "a", Status: junit.StatusPass},
		{Name: "b", Status: junit.StatusFail},
	}}
	if !r.Failed() {
		t.Error("Failed() = false, want true")
	}

	ok := Result{Cases: []junit.Case{{Name: "a", Status: junit.StatusPass}}}
	if ok.Failed() {
		t.Error("Failed() = true, want false")
	}
}

func TestProgressCallbackFiresPerRun(t *testing.T) {
	var count int
	var mu = make(chan struct{}, 1)
	mu <- struct{}{}

	_, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "exit 0"},
		Runs:    4,
	}, func(Result) {
		<-mu
		count++
		mu <- struct{}{}
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 4 {
		t.Errorf("progress callbacks = %d, want 4", count)
	}
}
