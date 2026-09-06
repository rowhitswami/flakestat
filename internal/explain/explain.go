// Package explain turns stored observations into an inspectable account of
// why a test received its verdict.
//
// This is the single evidence model. The CLI, JSON output and the CI adapters
// all render an Explanation; none of them derives classifications, confidence
// or reasoning of their own. Keeping that boundary is what stops each new
// consumer from growing a slightly different interpretation of the same data.
package explain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

// DefaultHistory is how many recent observations an explanation carries.
const DefaultHistory = 40

// Outcome is a single recorded result, in a form safe to render anywhere.
type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
	OutcomeSkip Outcome = "skip"
)

// Symbol is the single-character form used in a history strip. Deliberately
// letters rather than colour, so the strip survives a plain terminal, a log
// file and a markdown table.
func (o Outcome) Symbol() string {
	switch o {
	case OutcomePass:
		return "P"
	case OutcomeFail:
		return "F"
	default:
		return "-"
	}
}

// Event is one observation with the context needed to read it in sequence.
type Event struct {
	Outcome Outcome `json:"outcome"`
	Commit  string  `json:"commit,omitempty"`
	Branch  string  `json:"branch,omitempty"`

	// Flip marks a disagreement with the previous scored observation on the
	// same branch, and SameCommit narrows that to identical code.
	Flip       bool `json:"flip,omitempty"`
	SameCommit bool `json:"same_commit_flip,omitempty"`
}

// Explanation is the complete evidence behind one verdict.
type Explanation struct {
	TestID      string `json:"test_id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Suite       string `json:"suite,omitempty"`
	Class       string `json:"class,omitempty"`
	File        string `json:"file,omitempty"`

	Verdict    string  `json:"classification"`
	Score      float64 `json:"score"`
	FlipRate   float64 `json:"flip_rate"`
	Confidence string  `json:"confidence"`
	// LowerBound is the numeric bound behind the confidence label.
	LowerBound float64 `json:"lower_bound"`

	Observations int     `json:"observations"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Skipped      int     `json:"skipped"`
	FailureRate  float64 `json:"failure_rate"`

	Transitions     int `json:"transitions"`
	SameCommitFlips int `json:"same_commit_disagreements"`
	Commits         int `json:"commits"`
	Branches        int `json:"branches"`

	KnownFlaky  bool   `json:"framework_reported_flaky,omitempty"`
	LastFailure string `json:"last_failure,omitempty"`

	// History is the most recent events, oldest first.
	History      []Event `json:"history"`
	HistoryTotal int     `json:"history_total"`

	// Reason states in plain language why this verdict follows from the
	// evidence above. Generated from the observations, never a canned
	// description of the class.
	Reason string `json:"reason"`
}

// Build assembles an explanation from a test's observations, which must be
// ordered oldest first.
func Build(obs []store.Observation, cfg score.Config, historyLimit int) Explanation {
	res := score.Score(obs, cfg)
	cfg = withDefaults(cfg)

	e := Explanation{
		TestID:      res.TestID,
		Name:        res.Name,
		DisplayName: res.DisplayName(),
		Suite:       res.Suite,
		Class:       res.Class,
		File:        res.File,

		Verdict:    string(res.Verdict),
		Score:      res.Score,
		FlipRate:   res.FlipRate,
		Confidence: string(res.Level),
		LowerBound: res.Confidence,

		Observations: res.Runs,
		Passed:       res.Passes,
		Failed:       res.Fails,
		Skipped:      res.Skips,

		Transitions:     res.Transitions,
		SameCommitFlips: res.SameCommitFlips,
		Commits:         res.Commits,
		Branches:        res.Branches,
		KnownFlaky:      res.KnownFlaky,

		HistoryTotal: len(obs),
	}
	if res.Runs > 0 {
		e.FailureRate = float64(res.Fails) / float64(res.Runs)
	}

	e.History = buildHistory(obs, historyLimit)
	e.LastFailure = lastFailure(obs)
	e.Reason = reason(res, cfg)
	return e
}

// buildHistory converts observations to events, marking transitions the same
// way scoring does: within a branch, never across one.
func buildHistory(obs []store.Observation, limit int) []Event {
	type prev struct {
		passed bool
		commit string
		seen   bool
	}
	last := map[string]prev{}

	events := make([]Event, 0, len(obs))
	for _, o := range obs {
		ev := Event{Commit: o.Commit, Branch: o.Branch}

		if o.Status == junit.StatusSkip {
			ev.Outcome = OutcomeSkip
			events = append(events, ev)
			continue
		}

		passed := o.Status == junit.StatusPass
		if passed {
			ev.Outcome = OutcomePass
		} else {
			ev.Outcome = OutcomeFail
		}

		p := last[o.Branch]
		if p.seen && p.passed != passed {
			ev.Flip = true
			ev.SameCommit = p.commit == o.Commit
		}
		last[o.Branch] = prev{passed: passed, commit: o.Commit, seen: true}

		events = append(events, ev)
	}

	if limit > 0 && len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events
}

// Strip renders the history as aligned symbol and marker lines:
//
//	P P P F P P F F P P
//	      ^     ^ ^
func (e Explanation) Strip() (symbols, markers string) {
	var s, m strings.Builder
	for i, ev := range e.History {
		if i > 0 {
			s.WriteByte(' ')
			m.WriteByte(' ')
		}
		s.WriteString(ev.Outcome.Symbol())
		switch {
		case ev.Flip && ev.SameCommit:
			m.WriteByte('!') // disagreement on identical code
		case ev.Flip:
			m.WriteByte('^')
		default:
			m.WriteByte(' ')
		}
	}
	return s.String(), strings.TrimRight(m.String(), " ")
}

// reason explains the verdict from the evidence actually observed.
func reason(r score.Result, cfg score.Config) string {
	flips := r.Transitions - int(r.FlipRate*float64(r.Transitions)+0.5)
	flips = r.Transitions - flips // number that did flip

	switch r.Verdict {
	case score.ClassFlaky:
		b := fmt.Sprintf("The outcome changed %d time(s) across %d comparison(s) of the same test. "+
			"Even the cautious lower bound on that rate (%.3f) clears the %.2f flaky threshold, "+
			"so this is not an artefact of a small sample.",
			flips, r.Transitions, r.Confidence, cfg.FlakyThreshold)
		if r.SameCommitFlips > 0 {
			b += fmt.Sprintf(" %d of those disagreements happened on identical code, "+
				"which is direct evidence of nondeterminism rather than an inference.", r.SameCommitFlips)
		}
		if r.KnownFlaky {
			b += " The test framework also reported a rerun that passed after failing within a single run."
		}
		return b

	case score.ClassConsistentlyFail:
		return fmt.Sprintf("It failed all %d run(s) and never disagreed with itself, so there is no "+
			"inconsistency to measure. This is a broken test rather than a flaky one, and rerunning "+
			"it will not help.", r.Runs)

	case score.ClassSuspect:
		return fmt.Sprintf("The estimated flip rate (%.3f) is above the %.2f suspect threshold, but with "+
			"only %d comparison(s) the lower bound (%.3f) does not reach %.2f. More runs would settle "+
			"whether this is real.",
			r.Score, cfg.SuspectThreshold, r.Transitions, r.Confidence, cfg.FlakyThreshold)

	case score.ClassInsufficient:
		return fmt.Sprintf("Only %d scored run(s) are recorded, below the %d needed before a verdict is "+
			"claimed. Nothing here is evidence either way yet.", r.Runs, cfg.MinRuns)

	case score.ClassAlwaysSkipped:
		return fmt.Sprintf("All %d recorded observation(s) were skips, so the test has never produced a "+
			"pass or a fail. More runs cannot change that; the skip condition has to.", r.Skips)

	default:
		if r.Transitions == 0 {
			return fmt.Sprintf("All %d run(s) agreed, with no opportunity for disagreement to show up.", r.Runs)
		}
		return fmt.Sprintf("The outcome changed %d time(s) across %d comparison(s), which is below the "+
			"%.2f suspect threshold once sample size is accounted for.",
			flips, r.Transitions, cfg.SuspectThreshold)
	}
}

func lastFailure(obs []store.Observation) string {
	for i := len(obs) - 1; i >= 0; i-- {
		if obs[i].Status == junit.StatusSkip {
			continue
		}
		if m := strings.TrimSpace(obs[i].Message); m != "" {
			return m
		}
	}
	return ""
}

func withDefaults(c score.Config) score.Config {
	d := score.Defaults()
	if c.MinRuns <= 0 {
		c.MinRuns = d.MinRuns
	}
	if c.FlakyThreshold <= 0 {
		c.FlakyThreshold = d.FlakyThreshold
	}
	if c.SuspectThreshold <= 0 {
		c.SuspectThreshold = d.SuspectThreshold
	}
	return c
}

// AmbiguousError reports that a query matched several tests. Choosing one
// silently would make the explanation quietly describe the wrong test.
type AmbiguousError struct {
	Query   string
	Matches []string // display names, sorted
}

func (e *AmbiguousError) Error() string {
	shown := e.Matches
	extra := 0
	if len(shown) > 10 {
		extra = len(shown) - 10
		shown = shown[:10]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%q matches %d tests:\n", e.Query, len(e.Matches))
	for _, m := range shown {
		fmt.Fprintf(&b, "\n  %s", m)
	}
	if extra > 0 {
		fmt.Fprintf(&b, "\n  ... and %d more", extra)
	}
	b.WriteString("\n\nUse the full test identifier.")
	return b.String()
}

// NotFoundError reports that nothing matched.
type NotFoundError struct{ Query string }

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no test matching %q; run \"flakestat report --all\" to list what is recorded", e.Query)
}

// Find resolves a user's query to exactly one test id, preferring exact
// matches over substrings and refusing to guess between several.
func Find(grouped map[string][]store.Observation, query string) (string, error) {
	if _, ok := grouped[query]; ok {
		return query, nil
	}

	lower := strings.ToLower(strings.TrimSpace(query))
	var exact, partial []string

	for id, obs := range grouped {
		if len(obs) == 0 {
			continue
		}
		last := obs[len(obs)-1]
		display := junit.Case{Suite: last.Suite, Class: last.Class, Name: last.Name}.DisplayName()

		switch {
		case last.Name == query || display == query:
			exact = append(exact, id)
		case strings.Contains(strings.ToLower(last.Name), lower),
			strings.Contains(strings.ToLower(display), lower):
			partial = append(partial, id)
		}
	}

	candidates := exact
	if len(candidates) == 0 {
		candidates = partial
	}

	switch len(candidates) {
	case 0:
		return "", &NotFoundError{Query: query}
	case 1:
		return candidates[0], nil
	default:
		names := make([]string, 0, len(candidates))
		for _, id := range candidates {
			last := grouped[id][len(grouped[id])-1]
			names = append(names, junit.Case{Suite: last.Suite, Class: last.Class, Name: last.Name}.DisplayName())
		}
		sort.Strings(names)
		return "", &AmbiguousError{Query: query, Matches: names}
	}
}
