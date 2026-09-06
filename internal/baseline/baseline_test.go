package baseline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rowhitswami/flakestat/internal/score"
)

func flaky(id, name string, s float64) score.Result {
	return score.Result{TestID: id, Name: name, Class: "C", Suite: "s",
		Verdict: score.ClassFlaky, Score: s, Runs: 20}
}

func stable(id, name string) score.Result {
	return score.Result{TestID: id, Name: name, Class: "C", Suite: "s",
		Verdict: score.ClassStable, Runs: 20, Passes: 20}
}

// A missing baseline is the normal first-run state, not an error.
func TestLoadMissingFileIsEmpty(t *testing.T) {
	b, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Tests) != 0 {
		t.Errorf("tests = %d, want 0", len(b.Tests))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")

	in := FromResults([]score.Result{
		flaky("a", "test_a", 0.5),
		stable("b", "test_b"),
	})
	if err := in.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out.Tests) != 1 {
		t.Fatalf("tests = %d, want 1 (only flaky tests are recorded)", len(out.Tests))
	}
	if out.Tests["a"].Score != 0.5 {
		t.Errorf("score = %v, want 0.5", out.Tests["a"].Score)
	}
	if out.Updated.IsZero() {
		t.Error("Updated was not stamped")
	}
}

// The whole point of the ratchet: existing flakiness must not fail the build.
func TestAcceptedFlakinessDoesNotCountAsNew(t *testing.T) {
	b := FromResults([]score.Result{flaky("a", "test_a", 0.5)})

	d := Compare(b, []score.Result{flaky("a", "test_a", 0.5)}, 0.10)

	if len(d.New) != 0 {
		t.Errorf("new = %d, want 0: an accepted test must not be reported as new", len(d.New))
	}
	if len(d.Accepted) != 1 {
		t.Errorf("accepted = %d, want 1", len(d.Accepted))
	}
}

func TestNewlyFlakyIsDetected(t *testing.T) {
	b := FromResults([]score.Result{flaky("a", "test_a", 0.5)})

	d := Compare(b, []score.Result{
		flaky("a", "test_a", 0.5),
		flaky("b", "test_b", 0.4),
	}, 0.10)

	if len(d.New) != 1 {
		t.Fatalf("new = %d, want 1", len(d.New))
	}
	if d.New[0].Name != "test_b" {
		t.Errorf("new test = %q, want test_b", d.New[0].Name)
	}
}

// Ordinary scoring jitter must not fail builds; a real worsening must.
func TestRegressionRequiresExceedingDelta(t *testing.T) {
	b := FromResults([]score.Result{flaky("a", "test_a", 0.50)})

	within := Compare(b, []score.Result{flaky("a", "test_a", 0.58)}, 0.10)
	if len(within.Regressed) != 0 {
		t.Errorf("regressed = %d, want 0 for a move inside the delta", len(within.Regressed))
	}
	if len(within.Accepted) != 1 {
		t.Errorf("accepted = %d, want 1", len(within.Accepted))
	}

	beyond := Compare(b, []score.Result{flaky("a", "test_a", 0.75)}, 0.10)
	if len(beyond.Regressed) != 1 {
		t.Errorf("regressed = %d, want 1 for a move beyond the delta", len(beyond.Regressed))
	}
}

// An improving test must never be reported as a regression.
func TestImprovementIsNotARegression(t *testing.T) {
	b := FromResults([]score.Result{flaky("a", "test_a", 0.80)})

	d := Compare(b, []score.Result{flaky("a", "test_a", 0.20)}, 0.10)
	if len(d.Regressed) != 0 {
		t.Errorf("regressed = %d, want 0", len(d.Regressed))
	}
}

func TestFixedTestsAreReported(t *testing.T) {
	b := FromResults([]score.Result{
		flaky("a", "test_a", 0.5),
		flaky("b", "test_b", 0.6),
	})

	d := Compare(b, []score.Result{
		flaky("a", "test_a", 0.5),
		stable("b", "test_b"),
	}, 0.10)

	if len(d.Fixed) != 1 {
		t.Fatalf("fixed = %d, want 1", len(d.Fixed))
	}
	if d.Fixed[0].Name != "test_b" {
		t.Errorf("fixed = %q, want test_b", d.Fixed[0].Name)
	}
}

// A test absent from the current run entirely (deleted, or not executed) is
// treated as fixed rather than lingering in the baseline forever.
func TestVanishedTestCountsAsFixed(t *testing.T) {
	b := FromResults([]score.Result{flaky("a", "test_a", 0.5)})

	d := Compare(b, nil, 0.10)
	if len(d.Fixed) != 1 {
		t.Errorf("fixed = %d, want 1", len(d.Fixed))
	}
	if len(d.New) != 0 {
		t.Errorf("new = %d, want 0", len(d.New))
	}
}

// With no baseline, every flaky test is new -- which is exactly why
// --update-baseline exists as the first step.
func TestEmptyBaselineMakesEverythingNew(t *testing.T) {
	d := Compare(New(), []score.Result{
		flaky("a", "test_a", 0.5),
		flaky("b", "test_b", 0.6),
	}, 0.10)

	if len(d.New) != 2 {
		t.Errorf("new = %d, want 2", len(d.New))
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	if err := FromResults([]score.Result{flaky("a", "test_a", 0.5)}).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := FromResults([]score.Result{flaky("b", "test_b", 0.6)}).Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// No temporary file should survive a successful write.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}

	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := b.Tests["b"]; !ok {
		t.Error("second save did not take effect")
	}
}

func TestCorruptBaselineIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Error("expected an error for a corrupt baseline")
	}
}
