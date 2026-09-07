package store

import (
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
)

func scope() ExecutionScope {
	return ExecutionScope{
		Provider: "github", Run: "9381732", Job: "test", Attempt: "1",
		Report: "reports/junit.xml", Digest: Digest([]byte("<testsuite/>")),
	}
}

func cases(n int) []junit.Case {
	out := make([]junit.Case, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, junit.Case{Suite: "s", Class: "C", Name: "test_a", Status: junit.StatusPass})
	}
	return out
}

func keys(obs []Observation) []string {
	out := make([]string, len(obs))
	for i, o := range obs {
		out[i] = o.ExecKey
	}
	return out
}

// The whole design rests on this: the key must be a function of the evidence
// alone. Anything that varies between two ingestions of the same artifact --
// the clock, a generated run id -- would produce a fresh key every time and
// silently restore the duplication the key exists to prevent.
func TestKeyIsIndependentOfWhenIngestionHappened(t *testing.T) {
	first := FromCases(cases(3), Meta{RunID: NewRunID(), Scope: scope()})
	time.Sleep(2 * time.Millisecond)
	second := FromCases(cases(3), Meta{RunID: NewRunID(), Scope: scope()})

	for i := range first {
		if first[i].ExecKey != second[i].ExecKey {
			t.Fatalf("key %d changed between ingestions: %s vs %s", i, first[i].ExecKey, second[i].ExecKey)
		}
		if first[i].ExecKey == "" {
			t.Fatal("no key was assigned")
		}
	}
	if first[0].TS.Equal(second[0].TS) {
		t.Skip("clock did not advance; the comparison above proved nothing")
	}
}

// Twelve entries for one test in one report are twelve executions. Collapsing
// them would discard eleven results, which is the opposite failure to the one
// dedup is for and just as wrong.
func TestRepetitionsWithinOneReportStayDistinct(t *testing.T) {
	obs := FromCases(cases(12), Meta{Scope: scope()})

	seen := map[string]bool{}
	for _, k := range keys(obs) {
		if seen[k] {
			t.Fatalf("repetition collapsed onto an existing key %s", k)
		}
		seen[k] = true
	}
	if got := len(Dedup(obs)); got != 12 {
		t.Errorf("Dedup kept %d of 12 repetitions", got)
	}
}

// ingest(X) twice must leave the same state as ingest(X) once.
func TestIngestingTheSameReportTwiceAddsNothing(t *testing.T) {
	once := FromCases(cases(5), Meta{Scope: scope()})
	twice := append(append([]Observation{}, once...), FromCases(cases(5), Meta{Scope: scope()})...)

	if got := len(Dedup(twice)); got != len(once) {
		t.Errorf("after ingesting twice: %d observations, want %d", got, len(once))
	}
}

// ingest(X + a legitimate retry) must contain both. A re-run really did
// execute the tests a second time and that result is evidence.
func TestARetryIsNotADuplicate(t *testing.T) {
	first := scope()
	retry := scope()
	retry.Attempt = "2"

	both := append(FromCases(cases(5), Meta{Scope: first}), FromCases(cases(5), Meta{Scope: retry})...)
	if got := len(Dedup(both)); got != 10 {
		t.Errorf("a retry contributed %d of 10 expected observations", got)
	}
}

// Two shards can run different tests and still emit byte-identical XML -- an
// empty suite, or a framework that writes a fixed header. The job identity is
// what separates them, so the digest alone must never be the whole key.
func TestIdenticalReportsFromDifferentJobsStayDistinct(t *testing.T) {
	a, b := scope(), scope()
	b.Job = "test-2"

	both := append(FromCases(cases(4), Meta{Scope: a}), FromCases(cases(4), Meta{Scope: b})...)
	if got := len(Dedup(both)); got != 8 {
		t.Errorf("two jobs produced %d of 8 expected observations", got)
	}
}

// Conversely, one job that emits two different reports must keep both, which
// the job identity alone cannot express.
func TestDifferentReportsFromOneJobStayDistinct(t *testing.T) {
	a, b := scope(), scope()
	b.Report, b.Digest = "reports/platform.xml", Digest([]byte("<testsuite name='other'/>"))

	both := append(FromCases(cases(4), Meta{Scope: a}), FromCases(cases(4), Meta{Scope: b})...)
	if got := len(Dedup(both)); got != 8 {
		t.Errorf("two reports produced %d of 8 expected observations", got)
	}
}

// hunt watches every execution happen, so its results cannot be duplicates and
// must never be discarded -- two runs of a deterministic suite legitimately
// produce identical evidence.
func TestObservationsWithoutAKeyAreAlwaysKept(t *testing.T) {
	obs := FromCases(cases(6), Meta{Source: SourceHunt})
	for _, o := range obs {
		if o.ExecKey != "" {
			t.Fatal("an observation with no scope should carry no key")
		}
	}
	if got := len(Dedup(append(obs, obs...))); got != 12 {
		t.Errorf("Dedup kept %d of 12 keyless observations", got)
	}
}

// Field values must not be able to slide between fields.
func TestKeyFieldsCannotBeRearranged(t *testing.T) {
	a := ExecutionScope{Run: "1", Job: "23"}
	b := ExecutionScope{Run: "12", Job: "3"}

	if ExecutionKey(a, "t", 0) == ExecutionKey(b, "t", 0) {
		t.Error("run/job boundaries are not encoded; keys collided")
	}
}

func TestEmptyScopeYieldsNoKey(t *testing.T) {
	if got := ExecutionKey(ExecutionScope{}, "t", 0); got != "" {
		t.Errorf("ExecutionKey with no scope = %q, want empty", got)
	}
}

// Dedup keeps the first of each execution and preserves the order of the rest,
// because scoring walks the series chronologically.
func TestDedupPreservesOrder(t *testing.T) {
	s := scope()
	obs := []Observation{
		{TestID: "a", ExecKey: ExecutionKey(s, "a", 0)},
		{TestID: "b", ExecKey: ExecutionKey(s, "b", 0)},
		{TestID: "a", ExecKey: ExecutionKey(s, "a", 0)},
		{TestID: "c", ExecKey: ExecutionKey(s, "c", 0)},
	}
	got := Dedup(obs)
	if len(got) != 3 || got[0].TestID != "a" || got[1].TestID != "b" || got[2].TestID != "c" {
		t.Errorf("Dedup = %v, want a b c in order", keys(got))
	}
}
