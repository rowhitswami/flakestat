package explain

import (
	"strings"
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

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

// A skip is shown, but it is not an outcome. It must not appear as a flip and
// must not enter the pass/fail counts.
func TestSkipsAreShownButNotCounted(t *testing.T) {
	e := Build(skipSeries("P S P F S P"), score.Defaults(), 20)

	// Observations is the scored count, with skips reported separately, so a
	// skip can never land in the denominator of a failure rate.
	if e.Observations != 4 {
		t.Errorf("observations = %d, want 4 scored runs", e.Observations)
	}
	if e.Passed != 3 || e.Failed != 1 || e.Skipped != 2 {
		t.Errorf("passed/failed/skipped = %d/%d/%d, want 3/1/2", e.Passed, e.Failed, e.Skipped)
	}
	if e.Passed+e.Failed != e.Observations {
		t.Errorf("passed+failed = %d but observations = %d", e.Passed+e.Failed, e.Observations)
	}

	plain := Build(skipSeries("P P F P"), score.Defaults(), 20)
	if e.Transitions != plain.Transitions {
		t.Errorf("transitions = %d with skips, %d without", e.Transitions, plain.Transitions)
	}
	if e.SameCommitFlips != plain.SameCommitFlips {
		t.Errorf("same-commit flips = %d with skips, %d without", e.SameCommitFlips, plain.SameCommitFlips)
	}
}

// The rendered strip must not put a flip marker under a skip.
func TestSkipCarriesNoFlipMarker(t *testing.T) {
	e := Build(skipSeries("P S P"), score.Defaults(), 20)
	strip, markers := e.Strip()

	if !strings.Contains(strip, "-") {
		t.Errorf("strip %q should show the skip", strip)
	}
	if strings.ContainsAny(markers, "^!") {
		t.Errorf("markers %q claim a flip across a skip", markers)
	}
}

// A skip between two identical outcomes is not two disagreements.
func TestSkipDoesNotSplitAgreement(t *testing.T) {
	if e := Build(skipSeries("P S P"), score.Defaults(), 20); e.Transitions != 1 || e.SameCommitFlips != 0 {
		t.Errorf("transitions = %d, same-commit flips = %d; want 1 and 0",
			e.Transitions, e.SameCommitFlips)
	}
}
