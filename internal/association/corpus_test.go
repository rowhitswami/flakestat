package association

import (
	"fmt"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

// This corpus is the ground truth for dimension analysis, written before the
// engine that consumes it.
//
// Correlation is far easier to get convincingly wrong than scoring was: sparse
// groups produce perfect-looking splits by chance, and a naive rule that only
// compares failure rates will confidently announce them. Every case below
// names what the engine must conclude, including the cases where the correct
// answer is "not enough evidence".

// group is an observed population: how many of these runs failed.
type group struct {
	fail  int
	total int
}

// corpusCase is one scenario with its expected outcome.
type corpusCase struct {
	name string

	// dims maps a dimension key to its values and their observed outcomes,
	// e.g. {"os": {"windows": {12, 38}, "linux": {0, 91}}}.
	dims map[string]map[string]group

	// wantSignals are the "key=value" associations that must be reported.
	// Empty means the engine must report nothing at all.
	wantSignals []string

	why string
}

// corpus is deliberately weighted towards cases where the honest answer is
// silence, since that is where a correlation engine fails.
var corpus = []corpusCase{
	{
		name: "strong association",
		dims: map[string]map[string]group{
			"os": {"windows": {30, 100}, "linux": {0, 100}},
		},
		wantSignals: []string{"os=windows"},
		why:         "large samples, a 30 point gap and a clean split",
	},
	{
		name: "strong association on an uneven sample",
		dims: map[string]map[string]group{
			"os": {"windows": {12, 38}, "linux": {0, 91}},
		},
		wantSignals: []string{"os=windows"},
		why:         "unequal group sizes are fine when both clear the floor",
	},
	{
		name: "no association",
		dims: map[string]map[string]group{
			"os": {"linux": {10, 100}, "windows": {11, 100}},
		},
		wantSignals: nil,
		why:         "the test is flaky everywhere; the platform explains nothing",
	},
	{
		name: "sparse lottery",
		dims: map[string]map[string]group{
			"os": {"windows": {2, 5}, "linux": {1, 40}},
		},
		wantSignals: nil,
		why:         "40% versus 2.5% looks dramatic, but five observations prove nothing",
	},
	{
		name: "tiny perfect split",
		dims: map[string]map[string]group{
			"os": {"windows": {3, 3}, "linux": {0, 3}},
		},
		wantSignals: nil,
		why:         "a perfect split over three runs each is exactly what chance produces",
	},
	{
		name: "large but weak difference",
		dims: map[string]map[string]group{
			"os": {"windows": {60, 1000}, "linux": {40, 1000}},
		},
		wantSignals: nil,
		why:         "detectable with enough data, but a 2 point gap is not actionable",
	},
	{
		name: "reversed association",
		dims: map[string]map[string]group{
			"os": {"linux": {25, 100}, "windows": {0, 100}},
		},
		wantSignals: []string{"os=linux"},
		why:         "nothing about the analysis may privilege one platform",
	},
	{
		name: "three values, one culprit",
		dims: map[string]map[string]group{
			"os": {"linux": {0, 80}, "windows": {20, 80}, "darwin": {1, 80}},
		},
		wantSignals: []string{"os=windows"},
		why:         "each value is compared against the rest, not pairwise",
	},
	{
		name: "runtime version signal",
		dims: map[string]map[string]group{
			"runtime.version": {"3.12": {1, 100}, "3.13": {25, 100}},
		},
		wantSignals: []string{"runtime.version=3.13"},
		why:         "any analysable dimension works, not just os",
	},
	{
		name: "all passing",
		dims: map[string]map[string]group{
			"os": {"linux": {0, 100}, "windows": {0, 100}},
		},
		wantSignals: nil,
		why:         "no failures anywhere means nothing to associate",
	},
	{
		name: "all failing",
		dims: map[string]map[string]group{
			"os": {"linux": {100, 100}, "windows": {100, 100}},
		},
		wantSignals: nil,
		why:         "a consistently broken test has no environment signal",
	},
	{
		name: "confounded dimensions",
		dims: map[string]map[string]group{
			"os":              {"windows": {20, 50}, "linux": {0, 50}},
			"runtime.version": {"3.13": {20, 50}, "3.12": {0, 50}},
		},
		// Both are real associations in the data. The engine must report both
		// and must not claim either is the cause: the observations cannot
		// distinguish them.
		wantSignals: []string{"os=windows", "runtime.version=3.13"},
		why:         "perfectly correlated dimensions are indistinguishable from the data alone",
	},
	{
		name: "identifiers are not populations",
		dims: map[string]map[string]group{
			// A run id splits perfectly by construction: every failure happened
			// in some run. Treating it as an axis yields true but useless
			// findings like "failures are associated with CI run 9381732".
			"ci.run_id": {"9381732": {20, 40}, "9381733": {0, 40}},
		},
		wantSignals: nil,
		why:         "provenance identifiers are excluded from analysis by default",
	},
	{
		name: "retries are not a population either",
		dims: map[string]map[string]group{
			// Jobs get retried because they failed, so the first attempt
			// always holds the failures. Reporting it would dress up the
			// definition of a retry as a discovery.
			"ci.attempt": {"1": {20, 40}, "2": {0, 40}},
		},
		wantSignals: nil,
		why:         "the attempt number is downstream of failure, not upstream of it",
	},
}

// observations renders a case into a history the engine can consume.
//
// Outcomes are interleaved rather than blocked so that the sequence does not
// accidentally encode the grouping; dimension analysis must not depend on the
// order observations happen to arrive in.
func (c corpusCase) observations() []store.Observation {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	// Every dimension in a case describes the same underlying runs, so the
	// case is built from whichever dimension has the most observations and the
	// others are layered on in the same order.
	var keys []string
	for k := range c.dims {
		keys = append(keys, k)
	}

	type slot struct {
		dims   map[string]string
		failed bool
	}
	var slots []slot

	for _, k := range keys {
		values := c.dims[k]
		var vnames []string
		for v := range values {
			vnames = append(vnames, v)
		}
		// Stable order so the fixture is deterministic.
		sortStrings(vnames)

		idx := 0
		for _, v := range vnames {
			g := values[v]
			for i := 0; i < g.total; i++ {
				failed := i < g.fail
				if idx < len(slots) {
					slots[idx].dims[k] = v
					idx++
					continue
				}
				slots = append(slots, slot{
					dims:   map[string]string{k: v},
					failed: failed,
				})
				idx++
			}
		}
	}

	obs := make([]store.Observation, 0, len(slots))
	for i, s := range slots {
		status := junit.StatusPass
		if s.failed {
			status = junit.StatusFail
		}
		obs = append(obs, store.Observation{
			TS:         base.Add(time.Duration(i) * time.Second),
			TestID:     "t1",
			Name:       "test_subject",
			Class:      "suite",
			Suite:      "s",
			Status:     status,
			Commit:     fmt.Sprintf("sha%d", i/10),
			Branch:     "main",
			Dimensions: s.dims,
		})
	}
	return obs
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
