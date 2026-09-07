package score

import (
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

// series builds observations from a P/F/S string, all on one commit, branch
// and platform, so nothing but the outcomes can affect the result.
func skipSeries(pattern string) []store.Observation {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	obs := make([]store.Observation, 0, len(pattern))
	i := 0
	for _, ch := range pattern {
		var st junit.Status
		switch ch {
		case ' ':
			continue
		case 'P':
			st = junit.StatusPass
		case 'F':
			st = junit.StatusFail
		case 'E':
			st = junit.StatusError
		case 'S':
			st = junit.StatusSkip
		default:
			panic("bad pattern character")
		}
		obs = append(obs, store.Observation{
			TS: base.Add(time.Duration(i) * time.Second), TestID: "t1",
			Name: "test_subject", Status: st, Commit: "c1", Branch: "main",
			Dimensions: map[string]string{"os": "linux"},
		})
		i++
	}
	return obs
}

// A skipped run is not evidence either way. It must not become an outcome that
// a neighbour can disagree with, and it must not enter the failure rate.
//
//	pass    -> pass evidence
//	failure -> fail evidence
//	error   -> fail evidence
//	skip    -> neither
func TestSkipsAreNotOutcomes(t *testing.T) {
	withSkips := Score(skipSeries("P S P F S P"), Defaults())
	without := Score(skipSeries("P P F P"), Defaults())

	if withSkips.Transitions != without.Transitions {
		t.Errorf("transitions = %d with skips, %d without; skips created outcomes",
			withSkips.Transitions, without.Transitions)
	}
	if withSkips.Score != without.Score {
		t.Errorf("score = %.4f with skips, %.4f without", withSkips.Score, without.Score)
	}
	if withSkips.Runs != without.Runs {
		t.Errorf("scored runs = %d with skips, %d without; a skip was counted as a run",
			withSkips.Runs, without.Runs)
	}
	if withSkips.Fails != without.Fails {
		t.Errorf("failures = %d with skips, %d without", withSkips.Fails, without.Fails)
	}
	if withSkips.Skips != 2 {
		t.Errorf("skips = %d, want 2 recorded separately", withSkips.Skips)
	}
}

// The specific shape that would betray a skip leaking in: a skip between two
// identical outcomes must not manufacture P->S->P as two disagreements.
func TestSkipBetweenIdenticalOutcomesIsNotADisagreement(t *testing.T) {
	r := Score(skipSeries("P S P"), Defaults())
	if r.Transitions != 1 {
		t.Errorf("transitions = %d, want 1 comparison between the two passes", r.Transitions)
	}
	if r.Score != 0 {
		t.Errorf("score = %.4f, want 0; a skip was treated as a differing outcome", r.Score)
	}
}

// An error is a failure. A skip is not.
func TestErrorCountsAsFailureAndSkipDoesNot(t *testing.T) {
	errors := Score(skipSeries("P E P E"), Defaults())
	fails := Score(skipSeries("P F P F"), Defaults())

	if errors.Score != fails.Score || errors.Fails != fails.Fails {
		t.Errorf("error series scored %.4f/%d, failure series %.4f/%d; they must agree",
			errors.Score, errors.Fails, fails.Score, fails.Fails)
	}
	if skipped := Score(skipSeries("P S P S"), Defaults()); skipped.Fails != 0 {
		t.Errorf("failures = %d for a pass/skip series, want 0", skipped.Fails)
	}
}

// A test that only ever skips has no evidence at all, and saying "needs more
// runs" would be false at any number of runs.
func TestOnlySkipsIsItsOwnVerdict(t *testing.T) {
	r := Score(skipSeries("S S S S S S"), Defaults())
	if r.Verdict != ClassAlwaysSkipped {
		t.Errorf("verdict = %q, want %q", r.Verdict, ClassAlwaysSkipped)
	}
	if r.Runs != 0 || r.Fails != 0 {
		t.Errorf("scored runs = %d, failures = %d; want no evidence at all", r.Runs, r.Fails)
	}
}
