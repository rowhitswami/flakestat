// Package baseline records the flakiness a repository has already accepted.
//
// This is what makes a CI gate usable. Failing on any flaky test turns a repo
// with existing flakiness permanently red on the day it adopts the tool, so
// teams delete the gate. Failing only on tests that are *newly* flaky, or that
// have measurably worsened, turns it into a ratchet: today's mess is
// grandfathered, tomorrow's is not.
package baseline

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/rowhitswami/flakestat/internal/score"
)

// FileName is the baseline's default location inside the state directory.
const FileName = "baseline.json"

// Entry is one accepted flaky test.
type Entry struct {
	Name    string  `json:"name"`
	Suite   string  `json:"suite,omitempty"`
	Class   string  `json:"class,omitempty"`
	Verdict string  `json:"verdict"`
	Score   float64 `json:"score"`
}

// Baseline is the accepted set, keyed by test ID.
type Baseline struct {
	Version int              `json:"version"`
	Updated time.Time        `json:"updated"`
	Tests   map[string]Entry `json:"tests"`
}

// New returns an empty baseline.
func New() *Baseline {
	return &Baseline{Version: 1, Tests: map[string]Entry{}}
}

// Path returns the baseline location within a state directory.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Load reads a baseline. A missing file is an empty baseline, not an error:
// the first run of `check` legitimately has nothing recorded yet.
func Load(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("baseline: read %s: %w", path, err)
	}

	b := New()
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("baseline: parse %s: %w", path, err)
	}
	if b.Tests == nil {
		b.Tests = map[string]Entry{}
	}
	return b, nil
}

// Save writes the baseline, creating the directory if needed.
func (b *Baseline) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("baseline: create dir: %w", err)
	}

	b.Version = 1
	b.Updated = time.Now().UTC().Truncate(time.Second)

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("baseline: encode: %w", err)
	}
	data = append(data, '\n')

	// Write and rename so an interrupted run cannot truncate a good baseline.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("baseline: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("baseline: replace: %w", err)
	}
	return nil
}

// FromResults builds a baseline holding every currently flaky test.
func FromResults(results []score.Result) *Baseline {
	b := New()
	for _, r := range results {
		if r.Verdict == score.ClassFlaky {
			b.Tests[r.TestID] = Entry{
				Name:    r.Name,
				Suite:   r.Suite,
				Class:   r.Class,
				Verdict: string(r.Verdict),
				Score:   r.Score,
			}
		}
	}
	return b
}

// Diff is the comparison of current results against the baseline.
type Diff struct {
	New       []score.Result // flaky now, absent from the baseline
	Regressed []score.Result // in the baseline, materially worse
	Fixed     []Entry        // in the baseline, no longer flaky
	Accepted  []score.Result // in the baseline, unchanged
}

// Compare classifies current results against the baseline.
//
// delta is how much a score must rise before an already-accepted test counts
// as regressed; it keeps ordinary scoring jitter from failing builds.
func Compare(b *Baseline, results []score.Result, delta float64) Diff {
	var d Diff
	seen := make(map[string]bool, len(results))

	for _, r := range results {
		if r.Verdict != score.ClassFlaky {
			continue
		}
		seen[r.TestID] = true

		prev, known := b.Tests[r.TestID]
		switch {
		case !known:
			d.New = append(d.New, r)
		case r.Score > prev.Score+delta:
			d.Regressed = append(d.Regressed, r)
		default:
			d.Accepted = append(d.Accepted, r)
		}
	}

	// Anything recorded but no longer flaky has been fixed. Reporting it is
	// what prompts someone to refresh the baseline and tighten the ratchet.
	ids := make([]string, 0, len(b.Tests))
	for id := range b.Tests {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		d.Fixed = append(d.Fixed, b.Tests[id])
	}

	return d
}
