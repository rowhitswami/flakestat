// Package score quantifies test flakiness from observation history.
//
// Flakiness is inconsistency, not failure. A test that fails every single run
// is broken, not flaky, and scores zero here -- it is surfaced separately as
// ConsistentlyFailing so it is never silently hidden.
//
// The score is a recency-weighted rate of state transitions (pass->fail or
// fail->pass) across a test's history:
//
//	score = sum(r_i * e_i * t_i) / sum(r_i)
//
//	t_i = 1 if observation i and i+1 disagree, else 0
//	r_i = (1-alpha)^age        recency weight
//	e_i = evidence discount    1.0 same commit, 1/SameCommitWeight otherwise
//
// Note the normalization is by recency alone. Folding the commit factor into
// the denominator as well would cancel it out whenever a history is uniformly
// same-commit or uniformly cross-commit -- exactly the comparison that matters
// -- so cross-commit disagreement is modelled as *discounted evidence*
// instead: on identical code, disagreement is proof of flakiness, whereas
// across commits it may just be a regression someone later fixed. This keeps
// the score a bounded rate in [0,1].
//
// The recency weight means recent behavior dominates, so a test that has been
// fixed decays back to stable instead of being punished forever.
package score

import (
	"math"
	"sort"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

// Class is the verdict for a test.
type Class string

const (
	ClassStable           Class = "stable"
	ClassSuspect          Class = "suspect"
	ClassFlaky            Class = "flaky"
	ClassConsistentlyFail Class = "consistently-failing"
	ClassInsufficient     Class = "insufficient-data"

	// ClassAlwaysSkipped is for tests that were observed but never actually
	// ran. Reporting them as insufficient-data claims more runs would help,
	// which is false: a platform-gated or unconditionally skipped test will
	// never accumulate a single pass or fail no matter how often it is run.
	ClassAlwaysSkipped Class = "always-skipped"
)

// Config tunes scoring. Zero values are replaced by Defaults.
type Config struct {
	MinRuns          int     `json:"min_runs,omitempty"`
	EWMAAlpha        float64 `json:"ewma_alpha,omitempty"`
	SameCommitWeight float64 `json:"same_commit_weight,omitempty"`
	SuspectThreshold float64 `json:"suspect_threshold,omitempty"`
	FlakyThreshold   float64 `json:"flaky_threshold,omitempty"`

	// DefaultBranch is the branch whose results are trusted, typically main.
	// Empty disables branch weighting entirely; transitions are still never
	// computed across a branch boundary.
	DefaultBranch string `json:"default_branch,omitempty"`

	// BranchWeight scales disagreement seen on any other branch. Failures
	// there are often just work in progress rather than flakiness.
	BranchWeight float64 `json:"branch_weight,omitempty"`
}

// Defaults are the shipped scoring parameters.
//
// FlakyThreshold of 0.10 is measured, not guessed. Against a ground-truth
// suite with known flake rates it detects a test failing 10% of the time in
// 80% of 100-run trials (0.15 managed only 45%), while never once flagging a
// stable, broken or skipped test across every configuration tried. See
// VALIDATION.md.
//
// SameCommitWeight of 3.0 remains a judgment call: every validation run so far
// has been single-commit, so the cross-commit path is still unmeasured.
func Defaults() Config {
	return Config{
		MinRuns:          5,
		EWMAAlpha:        0.3,
		SameCommitWeight: 3.0,
		SuspectThreshold: 0.05,
		FlakyThreshold:   0.10,
		BranchWeight:     0.5,
	}
}

func (c Config) withDefaults() Config {
	d := Defaults()
	if c.MinRuns <= 0 {
		c.MinRuns = d.MinRuns
	}
	if c.EWMAAlpha <= 0 || c.EWMAAlpha > 1 {
		c.EWMAAlpha = d.EWMAAlpha
	}
	if c.SameCommitWeight <= 0 {
		c.SameCommitWeight = d.SameCommitWeight
	}
	if c.SuspectThreshold <= 0 {
		c.SuspectThreshold = d.SuspectThreshold
	}
	if c.FlakyThreshold <= 0 {
		c.FlakyThreshold = d.FlakyThreshold
	}
	if c.BranchWeight <= 0 || c.BranchWeight > 1 {
		c.BranchWeight = d.BranchWeight
	}
	return c
}

// Result is the verdict for one test.
type Result struct {
	TestID string
	Suite  string
	Class  string
	Name   string
	File   string

	Runs   int // scored observations (skips excluded)
	Passes int
	Fails  int

	// FlipRate is the unweighted transition rate, for display.
	FlipRate float64
	// Score is the weighted transition rate: the best point estimate of how
	// often this test disagrees with itself.
	Score float64
	// Confidence is a lower bound on Score given how much evidence exists.
	// Classification uses this, not Score, so a verdict requires evidence
	// rather than a lucky flip in a short run.
	Confidence float64

	Verdict Class

	// KnownFlaky means a framework reported flakiness directly (Surefire
	// <flakyFailure>), which is proof rather than inference.
	KnownFlaky bool

	LastStatus junit.Status
	Commits    int // distinct commits observed
	Branches   int // distinct branches observed
}

// DisplayName renders a qualified test name.
func (r Result) DisplayName() string {
	c := junit.Case{Suite: r.Suite, Class: r.Class, Name: r.Name}
	return c.DisplayName()
}

// Flaky reports whether this test should be treated as flaky.
func (r Result) Flaky() bool {
	return r.Verdict == ClassFlaky
}

// Score evaluates one test's observation history, which must be ordered
// oldest first (as store.ByTest returns it).
func Score(obs []store.Observation, cfg Config) Result {
	cfg = cfg.withDefaults()

	res := Result{}
	if len(obs) > 0 {
		last := obs[len(obs)-1]
		res.TestID = last.TestID
		res.Suite = last.Suite
		res.Class = last.Class
		res.Name = last.Name
		res.File = last.File
		res.LastStatus = last.Status
	}

	// Skips carry no pass/fail signal and must not create phantom transitions.
	points := make([]point, 0, len(obs))
	seenCommits := map[string]struct{}{}
	seenBranches := map[string]struct{}{}
	gen, lastCommit := 0, ""

	for _, o := range obs {
		if o.KnownFlaky {
			res.KnownFlaky = true
		}
		if o.Status == junit.StatusSkip {
			continue
		}
		passed := o.Status == junit.StatusPass

		// Recency is measured in generations -- distinct commits in
		// chronological order -- not in observations. Every run of a single
		// burst shares one commit and so shares one generation, which is what
		// lets a burst use all of its evidence instead of only the tail.
		if o.Commit != lastCommit || len(points) == 0 {
			if len(points) > 0 {
				gen++
			}
			lastCommit = o.Commit
		}

		points = append(points, point{
			passed: passed,
			commit: o.Commit,
			branch: o.Branch,
			gen:    gen,
		})
		if o.Commit != "" {
			seenCommits[o.Commit] = struct{}{}
		}
		if o.Branch != "" {
			seenBranches[o.Branch] = struct{}{}
		}
		if passed {
			res.Passes++
		} else {
			res.Fails++
		}
	}

	res.Runs = len(points)
	res.Commits = len(seenCommits)
	res.Branches = len(seenBranches)

	// A framework that observed fail-then-pass inside a single run has already
	// proven flakiness; no history threshold applies.
	if res.KnownFlaky {
		var effN float64
		res.FlipRate, res.Score, effN = flipRates(points, cfg)
		res.Confidence = wilsonLowerBound(res.Score, effN)
		if res.Score < cfg.FlakyThreshold {
			res.Score = cfg.FlakyThreshold
		}
		res.Verdict = ClassFlaky
		return res
	}

	// Observed, but never actually executed: more runs will not change this.
	if res.Runs == 0 && len(obs) > 0 {
		res.Verdict = ClassAlwaysSkipped
		return res
	}

	if res.Runs < cfg.MinRuns {
		res.Verdict = ClassInsufficient
		return res
	}

	var effN float64
	res.FlipRate, res.Score, effN = flipRates(points, cfg)
	res.Confidence = wilsonLowerBound(res.Score, effN)

	switch {
	case res.Fails == res.Runs:
		// Never passed: broken, not flaky. Surfaced so it can't hide.
		res.Verdict = ClassConsistentlyFail
	case res.Confidence >= cfg.FlakyThreshold:
		res.Verdict = ClassFlaky
	case res.Score >= cfg.SuspectThreshold:
		// Suspect deliberately uses the point estimate: it is the "worth a
		// look" bucket, where being early matters more than being certain.
		res.Verdict = ClassSuspect
	default:
		res.Verdict = ClassStable
	}
	return res
}

// point is one scored observation, keeping the provenance that scoring needs.
type point struct {
	passed bool
	commit string
	branch string
	gen    int // generation: how many distinct commits precede this one
}

// flipRates returns the unweighted and weighted transition rates.
//
// Transitions are computed *within* a branch, never across one. Interleaving
// branches chronologically manufactures flips that never happened: a test that
// passes on main, fails on an in-progress feature branch, then passes on main
// again reads as two flips in a flat series, when nothing about main changed.
// Grouping first removes that class of phantom signal entirely.
func flipRates(points []point, cfg Config) (flat, weighted, effN float64) {
	if len(points) < 2 {
		return 0, 0, 0
	}

	// Preserve first-seen branch order so results are deterministic.
	groups := map[string][]point{}
	var order []string
	for _, p := range points {
		if _, seen := groups[p.branch]; !seen {
			order = append(order, p.branch)
		}
		groups[p.branch] = append(groups[p.branch], p)
	}

	// Age is counted in generations, so a 100-run burst on one commit keeps
	// full statistical weight instead of decaying to its last few runs.
	newest := float64(points[len(points)-1].gen)

	var flips, transitions int
	var num, den, sumSqW float64

	for _, branch := range order {
		g := groups[branch]
		if len(g) < 2 {
			continue // a single observation on a branch proves nothing
		}

		// Work on the default branch is expected to be sound, so disagreement
		// there is the trustworthy signal. Failures on a feature branch are
		// frequently just work in progress, so they carry less weight.
		branchFactor := 1.0
		if cfg.DefaultBranch != "" && branch != "" && branch != cfg.DefaultBranch {
			branchFactor = cfg.BranchWeight
		}

		for i := 0; i < len(g)-1; i++ {
			transitions++

			t := 0.0
			if g[i].passed != g[i+1].passed {
				t = 1.0
				flips++
			}

			// Same-commit disagreement is proof; cross-commit is ambiguous and
			// so counts as partial evidence. Two observations with no recorded
			// SHA (a local hunt outside a git repo) compare equal and are
			// treated as same-commit, which is correct: a burst never changes
			// the code.
			evidence := 1.0
			if g[i].commit != g[i+1].commit {
				evidence = 1.0 / cfg.SameCommitWeight
			}

			// Recency uses the newer observation's generation, so a transition
			// is judged by how many commits ago it happened -- not by how many
			// individual runs have been recorded since.
			age := newest - float64(g[i+1].gen)
			recency := math.Pow(1-cfg.EWMAAlpha, age)

			num += recency * evidence * branchFactor * t
			den += recency
			sumSqW += recency * recency
		}
	}

	if transitions == 0 {
		return 0, 0, 0
	}
	flat = float64(flips) / float64(transitions)
	if den > 0 {
		weighted = num / den
		// Kish effective sample size: unequal weights carry less information
		// than the same number of equally weighted observations.
		effN = (den * den) / sumSqW
	}
	return flat, weighted, effN
}

// wilsonLowerBound returns the lower end of a one-sided Wilson score interval
// for a proportion.
//
// Flagging on the point estimate alone made verdicts incoherent: with only a
// handful of transitions, one unlucky flip produced a large estimate, so a
// barely-flaky test was called flaky 25% of the time at 10 runs and 0% at 20.
// Requiring the lower bound to clear the threshold means a verdict needs
// evidence, and collecting more data can only sharpen it.
func wilsonLowerBound(p, n float64) float64 {
	if n <= 0 {
		return 0
	}
	// z for a one-sided 80% bound: enough to suppress small-sample noise
	// without demanding impractical run counts.
	const z = 0.8416
	z2 := z * z

	centre := (p + z2/(2*n)) / (1 + z2/n)
	margin := (z / (1 + z2/n)) * math.Sqrt(p*(1-p)/n+z2/(4*n*n))

	if lb := centre - margin; lb > 0 {
		return lb
	}
	return 0
}

// All scores every test and returns results sorted worst first.
func All(grouped map[string][]store.Observation, cfg Config) []Result {
	out := make([]Result, 0, len(grouped))
	for _, obs := range grouped {
		out = append(out, Score(obs, cfg))
	}
	Sort(out)
	return out
}

// Sort orders results worst first: by verdict severity, then score, then name.
// Exported so callers that adjust a verdict after scoring can restore order.
func Sort(out []Result) {
	rank := map[Class]int{
		ClassFlaky:            0,
		ClassSuspect:          1,
		ClassConsistentlyFail: 2,
		ClassInsufficient:     3,
		ClassAlwaysSkipped:    4,
		ClassStable:           5,
	}

	sort.SliceStable(out, func(i, j int) bool {
		if rank[out[i].Verdict] != rank[out[j].Verdict] {
			return rank[out[i].Verdict] < rank[out[j].Verdict]
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].DisplayName() < out[j].DisplayName()
	})
}
