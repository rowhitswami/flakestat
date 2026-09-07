package explain

import (
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

// Deterministic per platform: always passes on linux, always fails on windows.
// Scoring groups transitions by execution context, so it sees no disagreement.
func TestMarkersMatchScoringOnPlatformBoundaries(t *testing.T) {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var obs []store.Observation
	i := 0
	for _, p := range []struct {
		os string
		st junit.Status
	}{{"linux", junit.StatusPass}, {"windows", junit.StatusFail}} {
		for n := 0; n < 6; n++ {
			obs = append(obs, store.Observation{
				TS: base.Add(time.Duration(i) * time.Second), TestID: "t1", Name: "t",
				Status: p.st, Commit: "c1", Branch: "main",
				Dimensions: map[string]string{"os": p.os},
			})
			i++
		}
	}

	e := Build(obs, score.Defaults(), 20)
	strip, markers := e.Strip()
	r := score.Score(obs, score.Defaults())

	t.Logf("strip    %s", strip)
	t.Logf("markers  %s", markers)
	t.Logf("explain: transitions=%d sameCommitFlips=%d", e.Transitions, e.SameCommitFlips)
	t.Logf("score:   transitions=%d sameCommitFlips=%d score=%.2f verdict=%s",
		r.Transitions, r.SameCommitFlips, r.Score, r.Verdict)

	marks := 0
	for _, c := range markers {
		if c == '^' || c == '!' {
			marks++
		}
	}
	if marks != r.SameCommitFlips {
		t.Errorf("strip shows %d flip marker(s) but scoring counted %d same-commit flip(s)",
			marks, r.SameCommitFlips)
	}
}

// The counterpart: within one context, a genuine same-commit flip must still
// be marked. Grouping must not become a way to hide real disagreement.
func TestGenuineFlipsAreStillMarked(t *testing.T) {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var obs []store.Observation
	for i, st := range []junit.Status{
		junit.StatusPass, junit.StatusFail, junit.StatusPass, junit.StatusFail,
	} {
		obs = append(obs, store.Observation{
			TS: base.Add(time.Duration(i) * time.Second), TestID: "t1", Name: "t",
			Status: st, Commit: "c1", Branch: "main",
			Dimensions: map[string]string{"os": "linux"},
		})
	}

	e := Build(obs, score.Defaults(), 20)
	_, markers := e.Strip()
	r := score.Score(obs, score.Defaults())

	marks := 0
	for _, c := range markers {
		if c == '^' || c == '!' {
			marks++
		}
	}
	if marks != 3 || r.SameCommitFlips != 3 {
		t.Errorf("markers=%d scoring=%d, want 3 same-commit flips marked", marks, r.SameCommitFlips)
	}
}
