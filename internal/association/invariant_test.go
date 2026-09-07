package association

import (
	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"

	"time"

	"math"
	"math/rand"
	"sort"
	"testing"
)

// Golden values are hand-maintained and were, on first writing, wrong. These
// invariants hold for any correct implementation, so they keep protecting the
// code as it changes and cannot rot the way a transcribed constant can.
//
// Note the test here is deliberately one-sided: only excess failures in the
// group under examination are interesting, and candidates are filtered by
// direction before any test runs. Invariants that hold only for a two-sided
// test (row or column swapping) therefore do not apply.

// The single strongest check: the hypergeometric distribution over all
// feasible tables with these margins must sum to exactly 1. A mismatched
// numerator and denominator -- the exact bug that appeared while verifying
// this code -- fails here immediately.
func TestHypergeometricSumsToOne(t *testing.T) {
	margins := [][4]int{
		{3, 1, 1, 3}, {12, 26, 0, 91}, {10, 90, 11, 89},
		{0, 50, 0, 50}, {20, 30, 5, 45}, {1, 1, 1, 1},
	}

	for _, m := range margins {
		row1, row2 := m[0]+m[1], m[2]+m[3]
		fails := m[0] + m[2]

		var total float64
		for x := 0; x <= row1; x++ {
			cx := fails - x
			if cx < 0 || cx > row2 {
				continue
			}
			total += hypergeometric(x, row1-x, cx, row2-cx)
		}
		if math.Abs(total-1) > 1e-9 {
			t.Errorf("margins %v: probabilities sum to %.12f, want 1", m, total)
		}
	}
}

// Transposing swaps the roles of the row and column margins, which leaves the
// hypergeometric unchanged. This is a genuine invariant of Fisher's test.
func TestFisherIsInvariantUnderTranspose(t *testing.T) {
	tables := [][4]int{{12, 26, 0, 91}, {5, 20, 3, 40}, {30, 70, 0, 100}, {2, 8, 1, 30}}

	for _, tc := range tables {
		a, b, c, d := tc[0], tc[1], tc[2], tc[3]
		got, transposed := fisherRightTail(a, b, c, d), fisherRightTail(a, c, b, d)
		if math.Abs(got-transposed) > 1e-12*math.Max(got, 1e-300) {
			t.Errorf("table %v: p=%.12g but transposed p=%.12g", tc, got, transposed)
		}
	}
}

// The right tail at a plus the left tail below a must account for everything.
func TestFisherTailsComplement(t *testing.T) {
	for _, tc := range [][4]int{{12, 26, 0, 91}, {10, 90, 11, 89}, {5, 5, 5, 5}} {
		a, b, c, d := tc[0], tc[1], tc[2], tc[3]
		row1, row2, fails := a+b, c+d, a+c

		var left float64
		for x := 0; x < a; x++ {
			cx := fails - x
			if cx < 0 || cx > row2 {
				continue
			}
			left += hypergeometric(x, row1-x, cx, row2-cx)
		}
		if sum := left + fisherRightTail(a, b, c, d); math.Abs(sum-1) > 1e-9 {
			t.Errorf("table %v: left+right = %.12f, want 1", tc, sum)
		}
	}
}

// Identical failure rates in both groups are the null hypothesis; the observed
// split sits at the centre of the distribution, so the tail must be large.
func TestIdenticalRatesGiveWeakEvidence(t *testing.T) {
	for _, tc := range [][4]int{{10, 90, 10, 90}, {50, 50, 50, 50}, {1, 9, 1, 9}} {
		if p := fisherRightTail(tc[0], tc[1], tc[2], tc[3]); p < 0.4 {
			t.Errorf("table %v: p=%.4f, want a large value when the rates match", tc, p)
		}
	}
}

// With no failures anywhere there is only one feasible table, so the observed
// split is certain.
func TestNoFailuresAnywhereIsCertain(t *testing.T) {
	for _, tc := range [][4]int{{0, 5, 0, 5}, {0, 50, 0, 91}, {0, 1000, 0, 3}} {
		if p := fisherRightTail(tc[0], tc[1], tc[2], tc[3]); math.Abs(p-1) > 1e-12 {
			t.Errorf("table %v: p=%.12f, want exactly 1", tc, p)
		}
	}
}

// Holding both group sizes and the total failure count fixed, moving failures
// into the first group can only strengthen the evidence.
func TestEvidenceStrengthensWithSeparation(t *testing.T) {
	const n1, n2, fails = 50, 50, 20

	prev := math.Inf(1)
	for a := 10; a <= fails; a++ { // 10 is the even split
		c := fails - a
		if c < 0 || c > n2 {
			continue
		}
		p := fisherRightTail(a, n1-a, c, n2-c)
		if p > prev+1e-12 {
			t.Errorf("a=%d: p=%.6g rose above the previous %.6g; separation must not weaken evidence", a, p, prev)
		}
		prev = p
	}
}

// Randomised sweep: probabilities must stay well-defined for any table.
func TestFisherAlwaysReturnsAProbability(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 3000; i++ {
		a := rng.Intn(200)
		b := rng.Intn(200)
		c := rng.Intn(200)
		d := rng.Intn(200)

		p := fisherRightTail(a, b, c, d)
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			t.Fatalf("fisherRightTail(%d,%d,%d,%d) = %v", a, b, c, d, p)
		}
	}
}

// Counts large enough to overflow a naive factorial must stay stable.
func TestFisherStableAtExtremeCounts(t *testing.T) {
	for _, tc := range [][4]int{
		{5000, 5000, 5000, 5000},
		{9000, 1000, 1000, 9000},
		{1, 99999, 0, 100000},
	} {
		p := fisherRightTail(tc[0], tc[1], tc[2], tc[3])
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			t.Errorf("table %v produced %v", tc, p)
		}
	}
}

// --- Benjamini-Hochberg invariants --------------------------------------

func TestBHInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(11))

	for trial := 0; trial < 500; trial++ {
		n := 1 + rng.Intn(20)
		a := make([]Association, n)
		for i := range a {
			a[i].P = rng.Float64()
		}
		applyBenjaminiHochberg(a)

		for i := range a {
			if a[i].Q < a[i].P-1e-12 {
				t.Fatalf("q=%.9f fell below p=%.9f", a[i].Q, a[i].P)
			}
			if a[i].Q < 0 || a[i].Q > 1 {
				t.Fatalf("q=%v is not a probability", a[i].Q)
			}
		}

		// Ordering by p must also order by q: a stronger raw result can never
		// come out weaker after adjustment.
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(x, y int) bool { return a[idx[x]].P < a[idx[y]].P })
		for k := 1; k < n; k++ {
			if a[idx[k]].Q < a[idx[k-1]].Q-1e-12 {
				t.Fatalf("q is not monotone in p: %.9f then %.9f", a[idx[k-1]].Q, a[idx[k]].Q)
			}
		}
	}
}

// Correction must depend on how many comparisons were made: the same p-value
// is weaker evidence when it was one of many.
func TestBHPenalisesMoreComparisons(t *testing.T) {
	one := []Association{{P: 0.004}}
	applyBenjaminiHochberg(one)

	many := make([]Association, 20)
	many[0].P = 0.004
	for i := 1; i < 20; i++ {
		many[i].P = 0.9
	}
	applyBenjaminiHochberg(many)

	if many[0].Q <= one[0].Q {
		t.Errorf("q was %.6f among 20 comparisons and %.6f alone; it should be larger",
			many[0].Q, one[0].Q)
	}
}

// A skipped run carries no pass/fail signal, so it must not enter either cell
// of the 2x2 table. If skips landed in the denominator, a dimension whose runs
// were mostly skipped would show an artificially low failure rate and could be
// reported as protective, or mask a real concentration elsewhere.
func TestSkipsDoNotEnterTheTable(t *testing.T) {
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	var withSkips, without []store.Observation

	add := func(dst *[]store.Observation, os string, status junit.Status, i int) {
		*dst = append(*dst, store.Observation{
			TS: base.Add(time.Duration(i) * time.Second), TestID: "t1", Name: "t",
			Status: status, Commit: "c1", Branch: "main",
			Dimensions: map[string]string{"os": os},
		})
	}

	i := 0
	for n := 0; n < 20; n++ {
		st := junit.StatusPass
		if n < 8 {
			st = junit.StatusFail
		}
		add(&withSkips, "windows", st, i)
		add(&without, "windows", st, i)
		i++
		add(&withSkips, "linux", junit.StatusPass, i)
		add(&without, "linux", junit.StatusPass, i)
		i++
	}
	// Skips only in the group being examined, which is where they would do the
	// most damage to its measured rate.
	for n := 0; n < 40; n++ {
		add(&withSkips, "windows", junit.StatusSkip, i)
		i++
	}

	a := Analyze(withSkips, Defaults())
	b := Analyze(without, Defaults())

	if len(a) != len(b) {
		t.Fatalf("skips changed the number of associations: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Total != b[i].Total || a[i].Fail != b[i].Fail || a[i].Rate != b[i].Rate {
			t.Errorf("%s=%s: %d/%d (%.3f) with skips, %d/%d (%.3f) without",
				a[i].Dimension, a[i].Value, a[i].Fail, a[i].Total, a[i].Rate,
				b[i].Fail, b[i].Total, b[i].Rate)
		}
	}
}
