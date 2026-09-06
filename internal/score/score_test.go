package score

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

// series builds an observation history from a pass/fail pattern.
// 'p' = pass, 'f' = fail, 's' = skip. Each observation gets a distinct commit
// unless commit is non-empty, in which case all share it.
func series(pattern string, commit string) []store.Observation {
	base := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	obs := make([]store.Observation, 0, len(pattern))

	for i, ch := range pattern {
		var st junit.Status
		switch ch {
		case 'p':
			st = junit.StatusPass
		case 'f':
			st = junit.StatusFail
		case 's':
			st = junit.StatusSkip
		}

		sha := commit
		if sha == "" {
			sha = string(rune('a'+i)) + "0000000"
		}

		obs = append(obs, store.Observation{
			TS:     base.Add(time.Duration(i) * time.Minute),
			TestID: "t1",
			Name:   "test_example",
			Suite:  "suite",
			Class:  "Class",
			Status: st,
			Commit: sha,
		})
	}
	return obs
}

func TestAlwaysPassingIsStable(t *testing.T) {
	r := Score(series("pppppppp", ""), Defaults())

	if r.Verdict != ClassStable {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassStable)
	}
	if r.Score != 0 {
		t.Errorf("Score = %v, want 0", r.Score)
	}
	if r.Passes != 8 || r.Fails != 0 {
		t.Errorf("passes/fails = %d/%d, want 8/0", r.Passes, r.Fails)
	}
}

// The central correctness claim: a test that always fails is broken, not
// flaky. Failure-rate-based tools get this wrong.
func TestAlwaysFailingIsNotFlaky(t *testing.T) {
	r := Score(series("ffffffff", ""), Defaults())

	if r.Verdict != ClassConsistentlyFail {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassConsistentlyFail)
	}
	if r.Score != 0 {
		t.Errorf("Score = %v, want 0 (no transitions)", r.Score)
	}
	if r.Flaky() {
		t.Error("Flaky() = true for a consistently failing test")
	}
}

func TestAlternatingIsFlaky(t *testing.T) {
	// Alternating on one commit is the strongest possible signal.
	r := Score(series("pfpfpfpf", "deadbeef"), Defaults())

	if r.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassFlaky)
	}
	if r.FlipRate != 1.0 {
		t.Errorf("FlipRate = %v, want 1.0", r.FlipRate)
	}
	if r.Score <= 0.9 {
		t.Errorf("Score = %v, want near 1.0", r.Score)
	}

	// The same pattern spread across commits is still flaky, but the evidence
	// is discounted, so it must not reach the same score.
	across := Score(series("pfpfpfpf", ""), Defaults())
	if across.Verdict != ClassFlaky {
		t.Errorf("cross-commit Verdict = %q, want %q", across.Verdict, ClassFlaky)
	}
	if across.Score >= r.Score {
		t.Errorf("cross-commit score %v should be below same-commit score %v", across.Score, r.Score)
	}
}

func TestBelowMinRunsIsInsufficientData(t *testing.T) {
	r := Score(series("pf", ""), Defaults())

	if r.Verdict != ClassInsufficient {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassInsufficient)
	}
	if r.Flaky() {
		t.Error("two runs should not be enough to call a test flaky")
	}
}

// Same-commit disagreement is definitive flakiness; cross-commit disagreement
// may just be a regression that was later fixed. The same pattern must score
// higher when it happened on one commit.
func TestSameCommitDisagreementScoresHigher(t *testing.T) {
	pattern := "ppfppfpp"

	same := Score(series(pattern, "deadbeef"), Defaults())
	across := Score(series(pattern, ""), Defaults())

	if same.FlipRate != across.FlipRate {
		t.Fatalf("unweighted flip rates should match: %v vs %v", same.FlipRate, across.FlipRate)
	}
	if same.Score <= across.Score {
		t.Errorf("same-commit score %v should exceed cross-commit score %v", same.Score, across.Score)
	}
	if same.Commits != 1 {
		t.Errorf("Commits = %d, want 1", same.Commits)
	}
}

// A test that was flaky and got fixed should decay back toward stable, while
// one that just started flaking should score high on the same flip count.
func TestRecentFlakinessOutweighsOldFlakiness(t *testing.T) {
	// Mirrored patterns: same length, same flip count, opposite ends.
	fixed := Score(series("pfpfpppppp", ""), Defaults())
	regressed := Score(series("ppppppfpfp", ""), Defaults())

	if fixed.FlipRate != regressed.FlipRate {
		t.Fatalf("unweighted flip rates should match: %v vs %v", fixed.FlipRate, regressed.FlipRate)
	}
	if regressed.Score <= fixed.Score {
		t.Errorf("recent flakiness (%v) should outweigh old flakiness (%v)", regressed.Score, fixed.Score)
	}
	if fixed.Verdict == ClassFlaky {
		t.Errorf("a long-fixed test should decay out of %q", ClassFlaky)
	}
}

// Skips carry no pass/fail signal and must not manufacture transitions.
func TestSkipsAreExcluded(t *testing.T) {
	withSkips := Score(series("psspsspssp", ""), Defaults())

	if withSkips.Runs != 4 {
		t.Errorf("Runs = %d, want 4 (skips excluded)", withSkips.Runs)
	}
	if withSkips.Score != 0 {
		t.Errorf("Score = %v, want 0: skips must not create transitions", withSkips.Score)
	}
}

// Surefire's <flakyFailure> is proof of flakiness, not a hint, so it bypasses
// the minimum-runs threshold.
func TestKnownFlakyBypassesMinRuns(t *testing.T) {
	obs := series("p", "")
	obs[0].KnownFlaky = true

	r := Score(obs, Defaults())

	if r.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassFlaky)
	}
	if !r.KnownFlaky {
		t.Error("KnownFlaky = false, want true")
	}
	if r.Score < Defaults().FlakyThreshold {
		t.Errorf("Score = %v, want at least the flaky threshold", r.Score)
	}
}

func TestSingleObservationDoesNotPanic(t *testing.T) {
	r := Score(series("p", ""), Defaults())
	if r.Verdict != ClassInsufficient {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassInsufficient)
	}
}

func TestEmptyHistory(t *testing.T) {
	r := Score(nil, Defaults())
	if r.Verdict != ClassInsufficient {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassInsufficient)
	}
	if r.Runs != 0 {
		t.Errorf("Runs = %d, want 0", r.Runs)
	}
}

func TestScoreIsBounded(t *testing.T) {
	patterns := []string{"pppppppp", "ffffffff", "pfpfpfpf", "ppffppff", "pffpfppf"}

	for _, p := range patterns {
		r := Score(series(p, "deadbeef"), Defaults())
		if r.Score < 0 || r.Score > 1 || math.IsNaN(r.Score) {
			t.Errorf("pattern %q: Score = %v, want within [0,1]", p, r.Score)
		}
		if r.FlipRate < 0 || r.FlipRate > 1 {
			t.Errorf("pattern %q: FlipRate = %v, want within [0,1]", p, r.FlipRate)
		}
	}
}

func TestZeroConfigUsesDefaults(t *testing.T) {
	withZero := Score(series("pfpfpfpf", ""), Config{})
	withDefaults := Score(series("pfpfpfpf", ""), Defaults())

	if withZero.Score != withDefaults.Score {
		t.Errorf("zero config scored %v, defaults scored %v", withZero.Score, withDefaults.Score)
	}
}

func TestAllSortsWorstFirst(t *testing.T) {
	grouped := map[string][]store.Observation{
		"stable": series("pppppppp", ""),
		"flaky":  series("pfpfpfpf", ""),
		"broken": series("ffffffff", ""),
		"thin":   series("pf", ""),
	}
	for id, obs := range grouped {
		for i := range obs {
			obs[i].TestID = id
			obs[i].Name = id
		}
	}

	results := All(grouped, Defaults())

	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}
	if results[0].Verdict != ClassFlaky {
		t.Errorf("first result = %q, want %q first", results[0].Verdict, ClassFlaky)
	}
	if results[len(results)-1].Verdict != ClassStable {
		t.Errorf("last result = %q, want %q last", results[len(results)-1].Verdict, ClassStable)
	}
}

// A test that is skipped in every run will never accumulate a pass or fail, so
// reporting "needs more runs" is wrong -- more runs cannot help.
func TestAlwaysSkippedIsItsOwnVerdict(t *testing.T) {
	r := Score(series("ssssssss", ""), Defaults())

	if r.Verdict != ClassAlwaysSkipped {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassAlwaysSkipped)
	}
	if r.Runs != 0 {
		t.Errorf("Runs = %d, want 0", r.Runs)
	}
	if r.Flaky() {
		t.Error("an always-skipped test must not be flaky")
	}
}

// A test with some real runs must not be mistaken for always-skipped.
func TestPartiallySkippedIsNotAlwaysSkipped(t *testing.T) {
	r := Score(series("sspsspss", ""), Defaults())

	if r.Verdict == ClassAlwaysSkipped {
		t.Error("a test with real runs must not be classed always-skipped")
	}
	if r.Runs != 2 {
		t.Errorf("Runs = %d, want 2", r.Runs)
	}
}

// No observations at all is still insufficient-data, not always-skipped.
func TestEmptyIsNotAlwaysSkipped(t *testing.T) {
	if got := Score(nil, Defaults()).Verdict; got != ClassInsufficient {
		t.Errorf("Verdict = %q, want %q", got, ClassInsufficient)
	}
}

func TestSortIsStableAndWorstFirst(t *testing.T) {
	in := []Result{
		{Name: "c", Verdict: ClassStable},
		{Name: "a", Verdict: ClassFlaky, Score: 0.2},
		{Name: "d", Verdict: ClassAlwaysSkipped},
		{Name: "b", Verdict: ClassFlaky, Score: 0.9},
	}
	Sort(in)

	if in[0].Name != "b" || in[1].Name != "a" {
		t.Errorf("flaky tests should lead, worst first; got %s, %s", in[0].Name, in[1].Name)
	}
	if in[len(in)-1].Name != "c" {
		t.Errorf("stable should sort last, got %s", in[len(in)-1].Name)
	}
}

// branchSeries builds a history where each observation carries a branch.
// pattern and branches must be the same length.
func branchSeries(pattern string, branches []string, commitPerObs bool) []store.Observation {
	base := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	obs := make([]store.Observation, 0, len(pattern))

	for i, ch := range pattern {
		var st junit.Status
		switch ch {
		case 'p':
			st = junit.StatusPass
		case 'f':
			st = junit.StatusFail
		case 's':
			st = junit.StatusSkip
		}

		sha := "fixedsha"
		if commitPerObs {
			sha = string(rune('a'+i)) + "0000000"
		}

		obs = append(obs, store.Observation{
			TS:     base.Add(time.Duration(i) * time.Minute),
			TestID: "t1",
			Name:   "test_example",
			Suite:  "suite",
			Class:  "Class",
			Status: st,
			Commit: sha,
			Branch: branches[i],
		})
	}
	return obs
}

// The core branch-awareness bug: main passes consistently, but an in-progress
// feature branch fails. Interleaved chronologically that reads as repeated
// flips, when nothing about main ever changed.
func TestBranchInterleavingDoesNotManufactureFlips(t *testing.T) {
	// main:p feature:f main:p feature:f main:p feature:f ...
	pattern := "pfpfpfpf"
	branches := []string{"main", "feature", "main", "feature", "main", "feature", "main", "feature"}

	cfg := Defaults()
	cfg.DefaultBranch = "main"

	r := Score(branchSeries(pattern, branches, true), cfg)

	// Within main every observation passed; within feature every one failed.
	// Neither branch disagrees with itself, so there are zero real flips.
	if r.FlipRate != 0 {
		t.Errorf("FlipRate = %v, want 0: no branch disagrees with itself", r.FlipRate)
	}
	if r.Score != 0 {
		t.Errorf("Score = %v, want 0", r.Score)
	}
	if r.Verdict == ClassFlaky {
		t.Error("interleaved branches must not produce a flaky verdict")
	}
	if r.Branches != 2 {
		t.Errorf("Branches = %d, want 2", r.Branches)
	}
}

// Genuine disagreement inside one branch must still be caught.
func TestDisagreementWithinABranchIsStillFlaky(t *testing.T) {
	pattern := "pfpfpfpf"
	branches := []string{"main", "main", "main", "main", "main", "main", "main", "main"}

	cfg := Defaults()
	cfg.DefaultBranch = "main"

	r := Score(branchSeries(pattern, branches, false), cfg)

	if r.FlipRate != 1.0 {
		t.Errorf("FlipRate = %v, want 1.0", r.FlipRate)
	}
	if r.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassFlaky)
	}
}

// Failures during feature-branch development are weaker evidence than the same
// pattern on the trusted default branch.
func TestDefaultBranchOutweighsFeatureBranch(t *testing.T) {
	pattern := "pfpfpfpf"
	mainOnly := []string{"main", "main", "main", "main", "main", "main", "main", "main"}
	featOnly := []string{"feat", "feat", "feat", "feat", "feat", "feat", "feat", "feat"}

	cfg := Defaults()
	cfg.DefaultBranch = "main"

	onMain := Score(branchSeries(pattern, mainOnly, false), cfg)
	onFeat := Score(branchSeries(pattern, featOnly, false), cfg)

	if onMain.FlipRate != onFeat.FlipRate {
		t.Fatalf("raw flip rates should match: %v vs %v", onMain.FlipRate, onFeat.FlipRate)
	}
	if onFeat.Score >= onMain.Score {
		t.Errorf("feature-branch score %v should be below default-branch score %v",
			onFeat.Score, onMain.Score)
	}
}

// With no default branch configured, every branch is trusted equally, but
// transitions are still confined within a branch.
func TestEmptyDefaultBranchDisablesWeightingOnly(t *testing.T) {
	pattern := "pfpfpfpf"
	featOnly := []string{"feat", "feat", "feat", "feat", "feat", "feat", "feat", "feat"}

	cfg := Defaults() // DefaultBranch is ""
	r := Score(branchSeries(pattern, featOnly, false), cfg)

	if r.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q when no default branch is set", r.Verdict, ClassFlaky)
	}
}

// A branch contributing a single observation has nothing to compare against.
func TestSingleObservationBranchIsIgnored(t *testing.T) {
	pattern := "ppppf"
	branches := []string{"main", "main", "main", "main", "stray"}

	cfg := Defaults()
	cfg.DefaultBranch = "main"

	r := Score(branchSeries(pattern, branches, false), cfg)

	if r.FlipRate != 0 {
		t.Errorf("FlipRate = %v, want 0: the lone stray observation cannot flip", r.FlipRate)
	}
	if r.Runs != 5 {
		t.Errorf("Runs = %d, want 5: the observation still counts", r.Runs)
	}
}

// Observations with no branch recorded (a local hunt) must behave exactly as
// before, so existing histories keep scoring the same.
func TestEmptyBranchesBehaveAsOneSeries(t *testing.T) {
	withBranch := Score(series("pfpfpfpf", "deadbeef"), Defaults())
	if withBranch.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q for unbranded observations", withBranch.Verdict, ClassFlaky)
	}
	if withBranch.Branches != 0 {
		t.Errorf("Branches = %d, want 0", withBranch.Branches)
	}
}

// Recency must decay over commits, not observations. Measuring age in
// observations caps the effective sample size at ~1/alpha transitions, so a
// 100-run burst carried no more evidence than a 10-run one and the score never
// converged. Every run of a burst shares a commit and so must weigh equally.
func TestBurstUsesAllEvidenceNotJustTheTail(t *testing.T) {
	// Flips in the first half, quiet in the second, all on one commit.
	pattern := "pfpfpfpfpf" + strings.Repeat("p", 30)
	r := Score(series(pattern, "onecommit"), Defaults())

	// 10 flips across 39 transitions; uniform weighting puts this near 0.26.
	if r.Score < 0.15 {
		t.Errorf("Score = %.3f: early flips in a burst were decayed away", r.Score)
	}
	if r.Verdict != ClassFlaky {
		t.Errorf("Verdict = %q, want %q", r.Verdict, ClassFlaky)
	}
}

// Within a burst the decay constant must not change the answer at all.
func TestAlphaIsInertWithinASingleBurst(t *testing.T) {
	pattern := "pfpfpfpfpfpppppppppp"

	fast := Defaults()
	fast.EWMAAlpha = 0.9
	slow := Defaults()
	slow.EWMAAlpha = 0.01

	a := Score(series(pattern, "onecommit"), fast)
	b := Score(series(pattern, "onecommit"), slow)

	if math.Abs(a.Score-b.Score) > 1e-9 {
		t.Errorf("alpha changed a single-commit burst score: %.6f vs %.6f", a.Score, b.Score)
	}
}

// Across commits, recency must still work: old flakiness decays.
func TestRecencyStillDecaysAcrossCommits(t *testing.T) {
	// Distinct commit per observation (series with "" does that).
	old := Score(series("pfpfpppppp", ""), Defaults())
	recent := Score(series("ppppppfpfp", ""), Defaults())

	if recent.Score <= old.Score {
		t.Errorf("recent flakiness (%.3f) should outweigh old (%.3f)", recent.Score, old.Score)
	}
}
