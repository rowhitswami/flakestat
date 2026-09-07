// Package association reports where a test's failures concentrate.
//
// It answers "these failures cluster on Windows", never "Windows causes this".
// Observational data cannot distinguish a cause from anything perfectly
// correlated with it, and this package is careful not to imply otherwise.
//
// Three guards keep it from producing confident nonsense, which is the natural
// failure mode of correlation over sparse data:
//
//   - An evidence floor. Two failures out of five is not a finding, however
//     dramatic the percentage looks next to one out of forty.
//   - An effect-size floor. With enough observations a two point difference
//     becomes statistically detectable while remaining useless to act on.
//   - A multiple-comparison correction. Testing os, arch, runtime, browser,
//     database and every user dimension will eventually turn up something that
//     looks significant purely by chance.
//
// Nothing here influences the flake score or the classification. A test is
// flaky because its outcomes demonstrate flakiness; associations only say
// where that flakiness concentrates.
package association

import (
	"math"
	"sort"

	"github.com/rowhitswami/flakestat/internal/dimension"
	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

// Config holds the thresholds. These are calibrated against the corpus in this
// package rather than chosen by intuition, and are expected to move as that
// corpus grows.
type Config struct {
	// MinGroup is the observations required in the value being examined, and
	// MinOthers the same for everything it is compared against.
	MinGroup  int
	MinOthers int

	// CandidateDiff is the failure-rate gap, in proportion, below which a
	// comparison is not worth testing at all.
	CandidateDiff float64

	// StrongDiff is the gap required to call an association strong, on top of
	// statistical significance.
	StrongDiff float64

	// MaxQ is the adjusted significance level. Adjusted, not raw: with many
	// dimensions a raw p-value stops meaning what it appears to.
	MaxQ float64
}

func Defaults() Config {
	return Config{
		MinGroup:      10,
		MinOthers:     10,
		CandidateDiff: 0.10,
		StrongDiff:    0.15,
		MaxQ:          0.01,
	}
}

func (c Config) withDefaults() Config {
	d := Defaults()
	if c.MinGroup <= 0 {
		c.MinGroup = d.MinGroup
	}
	if c.MinOthers <= 0 {
		c.MinOthers = d.MinOthers
	}
	if c.CandidateDiff <= 0 {
		c.CandidateDiff = d.CandidateDiff
	}
	if c.StrongDiff <= 0 {
		c.StrongDiff = d.StrongDiff
	}
	if c.MaxQ <= 0 {
		c.MaxQ = d.MaxQ
	}
	return c
}

// Association is one dimension value compared against every other observed
// value of the same dimension.
type Association struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`

	Fail  int `json:"failures"`
	Total int `json:"observations"`

	OtherFail  int `json:"other_failures"`
	OtherTotal int `json:"other_observations"`

	Rate      float64 `json:"failure_rate"`
	OtherRate float64 `json:"other_failure_rate"`
	// Difference is in proportion; multiply by 100 for percentage points.
	Difference float64 `json:"difference"`

	// P is the one-sided Fisher exact probability of a split this extreme or
	// more, and Q the same value after correcting for how many comparisons
	// were made. Reporting decisions use Q.
	P float64 `json:"p"`
	Q float64 `json:"q"`
}

// contextDimensions are recorded for provenance but never analysed.
//
// A run id partitions the data perfectly by construction, since every failure
// happened during some run. Analysing it yields findings that are true and
// worthless: "failures are associated with CI run 9381732".
//
// Attempt is worse than worthless. Jobs are retried precisely because the
// first attempt failed, so failures concentrate in attempt 1 by definition and
// the analysis would confidently report the reason retries exist.
var contextDimensions = map[string]bool{
	dimension.CIRunID:   true,
	dimension.CIJobID:   true,
	dimension.CIAttempt: true,
}

// Analyzable reports whether a dimension is a population worth grouping by
// rather than an identifier.
func Analyzable(key string) bool { return !contextDimensions[key] }

// Analyze returns the associations that clear every threshold, strongest
// first. An empty result means the evidence does not support a claim, which is
// the common and correct outcome.
func Analyze(obs []store.Observation, cfg Config) []Association {
	cfg = cfg.withDefaults()

	// dimension -> value -> [failures, observations]
	counts := map[string]map[string]*[2]int{}
	for _, o := range obs {
		// Skips carry no pass/fail signal, exactly as in scoring.
		if o.Status == junit.StatusSkip {
			continue
		}
		failed := o.Status != junit.StatusPass

		for k, v := range o.Dimensions {
			if v == "" || !Analyzable(k) {
				continue
			}
			if counts[k] == nil {
				counts[k] = map[string]*[2]int{}
			}
			if counts[k][v] == nil {
				counts[k][v] = &[2]int{}
			}
			counts[k][v][1]++
			if failed {
				counts[k][v][0]++
			}
		}
	}

	var candidates []Association

	for dim, values := range counts {
		// A dimension with one observed value has nothing to compare against;
		// "all failures happened on the only platform we ran" is not a finding.
		if len(values) < 2 {
			continue
		}

		var totalFail, totalObs int
		for _, c := range values {
			totalFail += c[0]
			totalObs += c[1]
		}

		for value, c := range values {
			fail, total := c[0], c[1]
			otherFail, otherTotal := totalFail-fail, totalObs-total

			if total < cfg.MinGroup || otherTotal < cfg.MinOthers {
				continue
			}

			rate := float64(fail) / float64(total)
			otherRate := float64(otherFail) / float64(otherTotal)
			diff := rate - otherRate

			// Only excess failures are interesting. The value with fewer
			// failures is the same finding stated backwards, and reporting
			// both halves would double every result.
			if diff < cfg.CandidateDiff {
				continue
			}

			candidates = append(candidates, Association{
				Dimension: dim, Value: value,
				Fail: fail, Total: total,
				OtherFail: otherFail, OtherTotal: otherTotal,
				Rate: rate, OtherRate: otherRate, Difference: diff,
				P: fisherRightTail(fail, total-fail, otherFail, otherTotal-otherFail),
			})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	applyBenjaminiHochberg(candidates)

	var out []Association
	for _, a := range candidates {
		if a.Q <= cfg.MaxQ && a.Difference >= cfg.StrongDiff {
			out = append(out, a)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Difference != out[j].Difference {
			return out[i].Difference > out[j].Difference
		}
		return out[i].Dimension+out[i].Value < out[j].Dimension+out[j].Value
	})
	return out
}

// applyBenjaminiHochberg fills in Q, controlling the false discovery rate
// across every comparison made for this test.
//
// Without it, analysing a dozen dimensions makes a spurious "significant"
// result close to inevitable.
func applyBenjaminiHochberg(a []Association) {
	m := len(a)
	idx := make([]int, m)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool { return a[idx[x]].P < a[idx[y]].P })

	// Walk from the largest p downward, keeping the running minimum so the
	// adjusted values stay monotone.
	running := 1.0
	for rank := m; rank >= 1; rank-- {
		i := idx[rank-1]
		q := a[i].P * float64(m) / float64(rank)
		if q < running {
			running = q
		}
		if running > 1 {
			running = 1
		}
		a[i].Q = running
	}
}

// fisherRightTail is the one-sided Fisher exact probability for the 2x2 table
//
//	a  b
//	c  d
//
// of observing a or more failures in the first row, with the margins fixed.
//
// Exact rather than chi-squared because the interesting cases have small or
// zero cells, where the approximation is worst.
func fisherRightTail(a, b, c, d int) float64 {
	if a < 0 || b < 0 || c < 0 || d < 0 {
		return 1
	}

	rowTotal := a + b
	failTotal := a + c
	n := a + b + c + d
	if n == 0 {
		return 1
	}

	maxA := rowTotal
	if failTotal < maxA {
		maxA = failTotal
	}

	var p float64
	for x := a; x <= maxA; x++ {
		bx := rowTotal - x
		cx := failTotal - x
		dx := n - x - bx - cx
		if bx < 0 || cx < 0 || dx < 0 {
			continue
		}
		p += hypergeometric(x, bx, cx, dx)
	}

	if p > 1 {
		return 1
	}
	if p < 0 {
		return 0
	}
	return p
}

// hypergeometric is the probability of one specific 2x2 table given its
// margins, computed in log space so large factorials do not overflow.
func hypergeometric(a, b, c, d int) float64 {
	n := a + b + c + d
	return math.Exp(logChoose(a+b, a) + logChoose(c+d, c) - logChoose(n, a+c))
}

func logChoose(n, k int) float64 {
	if k < 0 || k > n {
		return math.Inf(-1)
	}
	ln, _ := math.Lgamma(float64(n) + 1)
	lk, _ := math.Lgamma(float64(k) + 1)
	lnk, _ := math.Lgamma(float64(n-k) + 1)
	return ln - lk - lnk
}
