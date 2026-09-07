package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

// A report with one test recorded three times, as -count=3 or a parameterised
// suite would produce.
const repeatedReport = `<testsuite name="s" tests="3">
  <testcase classname="C" name="test_a"/>
  <testcase classname="C" name="test_a"><failure message="boom"/></testcase>
  <testcase classname="C" name="test_a"/>
</testsuite>`

func writeReport(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ingest(t *testing.T, stateDir string, args ...string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	full := append([]string{"--dir", stateDir, "--commit", "c0", "--branch", "main"}, args...)
	if err := runIngest(full, &out, &errOut); err != nil {
		t.Fatalf("ingest %v: %v (stderr: %s)", args, err, errOut.String())
	}
	return out.String()
}

func recorded(t *testing.T, stateDir string) []store.Observation {
	t.Helper()
	st, err := store.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	obs, errs := st.All()
	for _, e := range errs {
		t.Errorf("store: %v", e)
	}
	return obs
}

// The invariant a user meets first: a retried CI step that re-uploads the same
// artifact must not make flakestat more certain of anything.
func TestIngestingTheSameFileTwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	report := writeReport(t, dir, "junit.xml", repeatedReport)

	ingest(t, state, report)
	after := len(recorded(t, state))

	out := ingest(t, state, report)
	if got := len(recorded(t, state)); got != after {
		t.Errorf("second ingest changed the history from %d to %d observations", after, got)
	}
	if !strings.Contains(out, "Skipped 3 result(s) already recorded") {
		t.Errorf("the skip was not reported to the user: %q", out)
	}
	if after != 3 {
		t.Errorf("recorded %d observations, want the 3 repetitions in the report", after)
	}
}

// Ingesting a file whose contents changed is a genuinely new execution, even
// though the path is the same.
func TestReingestingAChangedReportAddsIt(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	report := writeReport(t, dir, "junit.xml", `<testsuite name="s"><testcase classname="C" name="test_a"/></testsuite>`)
	ingest(t, state, report)

	writeReport(t, dir, "junit.xml", `<testsuite name="s"><testcase classname="C" name="test_a"><failure message="boom"/></testcase></testsuite>`)
	ingest(t, state, report)

	obs := recorded(t, state)
	if len(obs) != 2 {
		t.Fatalf("recorded %d observations, want both the original and the rerun", len(obs))
	}
	if obs[0].Status == obs[1].Status {
		t.Error("the second execution's result was not recorded")
	}
}

// The merge path never goes through ingest, so it needs its own guarantee:
// concatenating one job's artifact twice must not double its evidence.
func TestConcatenatedLogsDedupOnRead(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	report := writeReport(t, dir, "junit.xml", repeatedReport)
	ingest(t, state, report)

	log := filepath.Join(state, store.LogName)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	// Exactly what `cat a b > merged` does when a job's artifact is collected
	// twice.
	if err := os.WriteFile(log, append(append([]byte{}, data...), data...), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := len(recorded(t, state)); got != 3 {
		t.Errorf("a doubled log read back as %d observations, want 3", got)
	}
}

// Two reports ingested together in one command are different executions even
// when they describe the same test.
func TestSeparateReportsInOneIngestBothCount(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	writeReport(t, dir, "a.xml", `<testsuite name="s"><testcase classname="C" name="test_a"/></testsuite>`)
	writeReport(t, dir, "b.xml", `<testsuite name="s"><testcase classname="C" name="test_a"><failure message="x"/></testcase></testsuite>`)

	ingest(t, state, filepath.Join(dir, "*.xml"))
	if got := len(recorded(t, state)); got != 2 {
		t.Errorf("recorded %d observations, want one per report", got)
	}
}

// The concrete harm dedup exists to prevent, pinned to numbers.
//
// A duplicate is not inert. It carries its original's timestamp, so it sorts
// beside it, and it always agrees with itself -- so duplication injects
// artificial agreements and drags a flaky test towards stable. Measured before
// this test existed: 0.64 -> 0.30.
func TestDuplicationWouldSuppressAFlakeScore(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")

	for i, ch := range "PPFPPFPPFPPF" {
		body := `<testcase classname="C" name="test_flaky"/>`
		if ch == 'F' {
			body = `<testcase classname="C" name="test_flaky"><failure message="boom"/></testcase>`
		}
		writeReport(t, dir, fmt.Sprintf("run-%02d.xml", i),
			`<testsuite name="s" tests="1">`+body+`</testsuite>`)
	}
	pattern := filepath.Join(dir, "run-*.xml")

	ingest(t, state, pattern)
	before := scoreOf(t, state, "test_flaky")

	// Ingesting the identical reports again must leave the verdict untouched.
	ingest(t, state, pattern)
	if after := scoreOf(t, state, "test_flaky"); after != before {
		t.Errorf("re-ingesting changed the score from %.2f to %.2f", before, after)
	}

	// And with identity removed, the damage must be visible -- otherwise this
	// test would keep passing if dedup silently stopped doing anything.
	log := filepath.Join(state, store.LogName)
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var stripped bytes.Buffer
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var o map[string]any
		if err := json.Unmarshal(line, &o); err != nil {
			t.Fatal(err)
		}
		delete(o, "exec_key")
		enc, _ := json.Marshal(o)
		// Written twice: the duplicate the key would have caught.
		stripped.Write(enc)
		stripped.WriteByte('\n')
		stripped.Write(enc)
		stripped.WriteByte('\n')
	}
	if err := os.WriteFile(log, stripped.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if damaged := scoreOf(t, state, "test_flaky"); damaged >= before {
		t.Errorf("unkeyed duplication scored %.2f, expected it to fall below %.2f", damaged, before)
	}
}

func scoreOf(t *testing.T, stateDir, name string) float64 {
	t.Helper()
	st, err := store.Open(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	grouped, errs := st.ByTest()
	for _, e := range errs {
		t.Errorf("store: %v", e)
	}
	for _, obs := range grouped {
		if len(obs) > 0 && obs[0].Name == name {
			return score.Score(obs, score.Defaults()).Score
		}
	}
	t.Fatalf("no observations for %q", name)
	return 0
}
