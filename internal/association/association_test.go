package association

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

// The corpus is the contract. Each case names what must be concluded,
// including the ones where the honest answer is silence.
func TestCorpus(t *testing.T) {
	for _, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			got := Analyze(tc.observations(), Defaults())

			var signals []string
			for _, a := range got {
				signals = append(signals, fmt.Sprintf("%s=%s", a.Dimension, a.Value))
			}
			sort.Strings(signals)

			want := append([]string(nil), tc.wantSignals...)
			sort.Strings(want)

			if strings.Join(signals, ",") != strings.Join(want, ",") {
				t.Errorf("signals = %v, want %v\n  (%s)", signals, want, tc.why)
				for _, a := range got {
					t.Logf("    %s=%s  %d/%d vs %d/%d  diff=%.3f p=%.5f q=%.5f",
						a.Dimension, a.Value, a.Fail, a.Total, a.OtherFail, a.OtherTotal,
						a.Difference, a.P, a.Q)
				}
			}
		})
	}
}

// Confounded dimensions are indistinguishable from observations alone, so both
// must be reported and neither may be singled out.
func TestConfoundedDimensionsBothReported(t *testing.T) {
	var tc corpusCase
	for _, c := range corpus {
		if c.name == "confounded dimensions" {
			tc = c
		}
	}

	got := Analyze(tc.observations(), Defaults())
	if len(got) != 2 {
		t.Fatalf("associations = %d, want both dimensions reported", len(got))
	}
	if math.Abs(got[0].Difference-got[1].Difference) > 1e-9 {
		t.Error("perfectly correlated dimensions should carry identical evidence")
	}
}

// --- Fisher's exact test ------------------------------------------------

// Golden values computed independently with exact integer arithmetic
// (Python math.comb), not by this implementation and not by estimate.
func TestFisherRightTailGolden(t *testing.T) {
	tests := []struct {
		a, b, c, d int
		want       float64
	}{
		// Fisher's own tea-tasting table.
		{3, 1, 1, 3, 0.242857143},
		// A clean split on a tiny sample: suggestive, nowhere near decisive.
		{3, 0, 0, 3, 0.05},
		// The uneven-sample case from the corpus.
		{12, 26, 0, 91, 1.0349879e-07},
		// No association: 10/100 against 11/100 is slightly below the mean
		// split, so most of the distribution lies at or above it.
		{10, 90, 11, 89, 0.677261198},
		// Degenerate: nothing failed anywhere, so the observed split is certain.
		{0, 50, 0, 50, 1.0},
		// Large counts, where a naive factorial implementation would overflow.
		{600, 400, 400, 600, 2.14997621e-19},
	}

	for _, tc := range tests {
		got := fisherRightTail(tc.a, tc.b, tc.c, tc.d)
		// Relative tolerance: these span twenty orders of magnitude.
		if rel := math.Abs(got-tc.want) / math.Max(tc.want, 1e-300); rel > 1e-6 {
			t.Errorf("fisherRightTail(%d,%d,%d,%d) = %.9g, want %.9g (rel err %.2g)",
				tc.a, tc.b, tc.c, tc.d, got, tc.want, rel)
		}
	}
}

func TestFisherIsBounded(t *testing.T) {
	for _, tc := range [][4]int{{0, 0, 0, 0}, {5, 5, 5, 5}, {100, 0, 0, 100}, {1, 0, 0, 1}} {
		p := fisherRightTail(tc[0], tc[1], tc[2], tc[3])
		if p < 0 || p > 1 || math.IsNaN(p) {
			t.Errorf("fisherRightTail%v = %v, want a probability", tc, p)
		}
	}
}

// Large tables must not overflow; the log-space computation exists for this.
func TestFisherHandlesLargeCounts(t *testing.T) {
	p := fisherRightTail(600, 400, 400, 600)
	if math.IsNaN(p) || math.IsInf(p, 0) {
		t.Fatalf("large table produced %v", p)
	}
	if p > 1e-15 {
		t.Errorf("p = %g, want a decisively small value for a large clean effect", p)
	}
}

// --- Benjamini-Hochberg -------------------------------------------------

func TestBenjaminiHochbergAdjustsForComparisonCount(t *testing.T) {
	a := []Association{{P: 0.001}, {P: 0.008}, {P: 0.03}, {P: 0.5}}
	applyBenjaminiHochberg(a)

	// q_i = min over j>=i of p_j * m / j, made monotone.
	want := []float64{0.004, 0.016, 0.04, 0.5}
	for i := range a {
		if math.Abs(a[i].Q-want[i]) > 1e-9 {
			t.Errorf("q[%d] = %.6f, want %.6f", i, a[i].Q, want[i])
		}
	}
}

func TestBenjaminiHochbergIsMonotoneAndBounded(t *testing.T) {
	a := []Association{{P: 0.9}, {P: 0.8}, {P: 0.02}, {P: 0.7}}
	applyBenjaminiHochberg(a)

	for i := range a {
		if a[i].Q < a[i].P-1e-12 {
			t.Errorf("q must never fall below p: q=%g p=%g", a[i].Q, a[i].P)
		}
		if a[i].Q > 1 {
			t.Errorf("q = %g, want it capped at 1", a[i].Q)
		}
	}
}

// A single comparison needs no correction.
func TestBenjaminiHochbergSingleComparison(t *testing.T) {
	a := []Association{{P: 0.004}}
	applyBenjaminiHochberg(a)
	if math.Abs(a[0].Q-0.004) > 1e-12 {
		t.Errorf("q = %g, want it unchanged at 0.004", a[0].Q)
	}
}

// --- guards -------------------------------------------------------------

func TestIdentifierDimensionsAreNotAnalysed(t *testing.T) {
	if Analyzable("ci.run_id") || Analyzable("ci.job_id") {
		t.Error("provenance identifiers must not be analysis axes")
	}
	for _, k := range []string{"os", "arch", "runtime.version", "ci.worker", "ci.shard", "browser", "database"} {
		if !Analyzable(k) {
			t.Errorf("%q should be analysable", k)
		}
	}
}

// A dimension with one observed value has nothing to compare against.
func TestSingleValuedDimensionYieldsNothing(t *testing.T) {
	tc := corpusCase{dims: map[string]map[string]group{"os": {"linux": {40, 100}}}}
	if got := Analyze(tc.observations(), Defaults()); got != nil {
		t.Errorf("expected no associations, got %v", got)
	}
}

func TestNoObservationsYieldsNothing(t *testing.T) {
	if got := Analyze(nil, Defaults()); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// Thresholds are configurable, and loosening them must actually loosen.
func TestThresholdsAreRespected(t *testing.T) {
	tc := corpusCase{dims: map[string]map[string]group{
		"os": {"windows": {2, 5}, "linux": {1, 40}},
	}}

	if got := Analyze(tc.observations(), Defaults()); got != nil {
		t.Errorf("the sparse case must report nothing by default, got %v", got)
	}

	loose := Defaults()
	loose.MinGroup, loose.MinOthers, loose.MaxQ, loose.StrongDiff = 1, 1, 1.0, 0.01
	if got := Analyze(tc.observations(), loose); len(got) == 0 {
		t.Error("with the floors removed the same data should surface a candidate")
	}
}
