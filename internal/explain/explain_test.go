package explain

import (
	"strings"
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

// obs builds a history from a pattern: p=pass, f=fail, s=skip. When commit is
// empty each observation gets its own commit, so cross-commit behaviour can be
// exercised too.
func obs(pattern, commit string) []store.Observation {
	base := time.Date(2026, 9, 7, 10, 0, 0, 0, time.UTC)
	out := make([]store.Observation, 0, len(pattern))

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
		msg := ""
		if st == junit.StatusFail {
			msg = "timed out waiting for gateway"
		}
		out = append(out, store.Observation{
			TS:     base.Add(time.Duration(i) * time.Minute),
			TestID: "t1", Name: "test_checkout", Suite: "suite", Class: "checkout",
			Status: st, Commit: sha, Branch: "main", Message: msg,
		})
	}
	return out
}

func burstCfg() score.Config {
	c := score.Defaults()
	c.MinRuns = 2
	return c
}

func TestBuildReportsObservedCounts(t *testing.T) {
	e := Build(obs("ppfpsfp", "abc"), burstCfg(), DefaultHistory)

	if e.Observations != 6 {
		t.Errorf("Observations = %d, want 6 (skips excluded)", e.Observations)
	}
	if e.Passed != 4 || e.Failed != 2 {
		t.Errorf("passed/failed = %d/%d, want 4/2", e.Passed, e.Failed)
	}
	if e.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", e.Skipped)
	}
	if got := e.FailureRate; got < 0.33 || got > 0.34 {
		t.Errorf("FailureRate = %.3f, want ~0.333", got)
	}
	if e.DisplayName != "checkout::test_checkout" {
		t.Errorf("DisplayName = %q", e.DisplayName)
	}
}

// The explanation must come from the observations, not from a canned
// description of the verdict.
func TestReasonQuotesRealEvidence(t *testing.T) {
	e := Build(obs("pfpfpfpfpfpf", "onecommit"), burstCfg(), DefaultHistory)

	if e.Verdict != string(score.ClassFlaky) {
		t.Fatalf("Verdict = %q, want flaky", e.Verdict)
	}
	for _, want := range []string{"11", "identical code"} {
		if !strings.Contains(e.Reason, want) {
			t.Errorf("reason should mention %q, got:\n%s", want, e.Reason)
		}
	}
}

// A consistently failing test must explain why it is broken, not flaky.
func TestBrokenExplainsWhyNotFlaky(t *testing.T) {
	e := Build(obs("ffffffff", "abc"), burstCfg(), DefaultHistory)

	if e.Verdict != string(score.ClassConsistentlyFail) {
		t.Fatalf("Verdict = %q, want consistently-failing", e.Verdict)
	}
	low := strings.ToLower(e.Reason)
	if !strings.Contains(low, "broken") || !strings.Contains(low, "flaky") {
		t.Errorf("reason must contrast broken with flaky, got:\n%s", e.Reason)
	}
	if !strings.Contains(low, "rerunning") {
		t.Errorf("reason should say rerunning will not help, got:\n%s", e.Reason)
	}
}

func TestInsufficientExplainsWhy(t *testing.T) {
	e := Build(obs("pf", ""), score.Defaults(), DefaultHistory)

	if e.Verdict != string(score.ClassInsufficient) {
		t.Fatalf("Verdict = %q, want insufficient-data", e.Verdict)
	}
	if !strings.Contains(e.Reason, "below") {
		t.Errorf("reason should say the sample is below the minimum, got:\n%s", e.Reason)
	}
	// Even without a verdict, evidence level must be reported.
	if e.Confidence == "" {
		t.Error("confidence level must be present even when unclassified")
	}
}

func TestAlwaysSkippedExplainsWhy(t *testing.T) {
	e := Build(obs("ssss", "abc"), burstCfg(), DefaultHistory)

	if e.Verdict != string(score.ClassAlwaysSkipped) {
		t.Fatalf("Verdict = %q, want always-skipped", e.Verdict)
	}
	if !strings.Contains(strings.ToLower(e.Reason), "skip") {
		t.Errorf("reason should explain the skips, got:\n%s", e.Reason)
	}
}

// History is the raw evidence; it must be chronological and mark transitions.
func TestHistoryStripMarksTransitions(t *testing.T) {
	e := Build(obs("pppfppffpp", "onecommit"), burstCfg(), DefaultHistory)
	symbols, markers := e.Strip()

	if symbols != "P P P F P P F F P P" {
		t.Errorf("symbols = %q, want chronological P/F strip", symbols)
	}
	// Transitions at index 3 (P->F), 4 (F->P), 6 (P->F), 8 (F->P).
	for _, i := range []int{3, 4, 6, 8} {
		if pos := i * 2; pos >= len(markers) || markers[pos] == ' ' {
			t.Errorf("expected a transition marker at position %d in %q", i, markers)
		}
	}
	// Same-commit flips are marked distinctly from ordinary ones.
	if !strings.Contains(markers, "!") {
		t.Errorf("same-commit flips should be marked with '!', got %q", markers)
	}
}

func TestHistoryRendersSkipsWithoutTransitions(t *testing.T) {
	e := Build(obs("psspss", "abc"), burstCfg(), DefaultHistory)
	symbols, markers := e.Strip()

	if !strings.Contains(symbols, "-") {
		t.Errorf("skips should render as '-', got %q", symbols)
	}
	if strings.Contains(markers, "^") || strings.Contains(markers, "!") {
		t.Errorf("skips must not create transitions, got markers %q", markers)
	}
}

func TestHistoryIsBoundedButReportsTotal(t *testing.T) {
	e := Build(obs(strings.Repeat("pf", 60), "abc"), burstCfg(), 10)

	if len(e.History) != 10 {
		t.Errorf("History = %d events, want 10", len(e.History))
	}
	if e.HistoryTotal != 120 {
		t.Errorf("HistoryTotal = %d, want 120", e.HistoryTotal)
	}
}

// Cross-branch observations must not be shown as transitions, matching how
// scoring treats them.
func TestHistoryDoesNotFlipAcrossBranches(t *testing.T) {
	o := obs("pfpf", "abc")
	o[1].Branch = "feature"
	o[3].Branch = "feature"

	e := Build(o, burstCfg(), DefaultHistory)
	_, markers := e.Strip()
	if strings.Contains(markers, "^") || strings.Contains(markers, "!") {
		t.Errorf("interleaved branches must not read as transitions, got %q", markers)
	}
}

func TestLastFailureIsSurfaced(t *testing.T) {
	e := Build(obs("pfp", "abc"), burstCfg(), DefaultHistory)
	if !strings.Contains(e.LastFailure, "gateway") {
		t.Errorf("LastFailure = %q, want the recorded message", e.LastFailure)
	}
}

// --- lookup -----------------------------------------------------------------

func grouped() map[string][]store.Observation {
	mk := func(id, class, name string) []store.Observation {
		o := obs("pfp", "abc")
		for i := range o {
			o[i].TestID, o[i].Class, o[i].Name = id, class, name
		}
		return o
	}
	return map[string][]store.Observation{
		"id-auth":  mk("id-auth", "auth", "test_login"),
		"id-oauth": mk("id-oauth", "oauth", "test_login"),
		"id-pay":   mk("id-pay", "pay", "test_checkout_timeout"),
	}
}

func TestFindByIDAndExactName(t *testing.T) {
	g := grouped()

	if got, err := Find(g, "id-pay"); err != nil || got != "id-pay" {
		t.Errorf("lookup by id = %q, %v", got, err)
	}
	if got, err := Find(g, "test_checkout_timeout"); err != nil || got != "id-pay" {
		t.Errorf("lookup by exact name = %q, %v", got, err)
	}
	if got, err := Find(g, "pay::test_checkout_timeout"); err != nil || got != "id-pay" {
		t.Errorf("lookup by display name = %q, %v", got, err)
	}
}

func TestFindBySubstring(t *testing.T) {
	got, err := Find(grouped(), "checkout")
	if err != nil || got != "id-pay" {
		t.Errorf("substring lookup = %q, %v", got, err)
	}
}

// Silently picking one of several matches would make the explanation quietly
// describe the wrong test.
func TestFindRefusesToGuessBetweenMatches(t *testing.T) {
	_, err := Find(grouped(), "test_login")

	amb, ok := err.(*AmbiguousError)
	if !ok {
		t.Fatalf("expected AmbiguousError, got %T: %v", err, err)
	}
	if len(amb.Matches) != 2 {
		t.Errorf("matches = %d, want 2", len(amb.Matches))
	}
	msg := amb.Error()
	for _, want := range []string{"auth::test_login", "oauth::test_login", "full test identifier"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should contain %q, got:\n%s", want, msg)
		}
	}
}

func TestFindUnknownIsActionable(t *testing.T) {
	_, err := Find(grouped(), "nope")
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatalf("expected NotFoundError, got %T", err)
	}
	if !strings.Contains(err.Error(), "flakestat report") {
		t.Errorf("error should suggest how to list tests, got: %v", err)
	}
}

func TestFindOnEmptyHistory(t *testing.T) {
	if _, err := Find(map[string][]store.Observation{}, "anything"); err == nil {
		t.Error("expected an error when nothing is recorded")
	}
}
