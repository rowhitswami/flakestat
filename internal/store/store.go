// Package store persists test observations as an append-only NDJSON log.
//
// NDJSON keeps the binary free of cgo, keeps history diffable in git, and lets
// results from parallel CI shards be concatenated without a merge step.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
)

// DefaultDir is the per-repo state directory.
const DefaultDir = ".flakestat"

// LogName is the append-only observation log within the state directory.
const LogName = "runs.ndjson"

// Source records how an observation was collected.
const (
	SourceHunt = "hunt" // local burst run; code is constant across runs
	SourceCI   = "ci"   // ingested from a CI artifact
)

// Observation is a single recorded test result.
type Observation struct {
	TS      time.Time `json:"ts"`
	RunID   string    `json:"run_id"`
	Commit  string    `json:"commit,omitempty"`
	Branch  string    `json:"branch,omitempty"`
	Source  string    `json:"source"`
	Attempt int       `json:"attempt,omitempty"`

	TestID string       `json:"test_id"`
	Suite  string       `json:"suite,omitempty"`
	Class  string       `json:"class,omitempty"`
	Name   string       `json:"name"`
	Status junit.Status `json:"status"`

	// File is the source file when the framework reports one. Quarantine
	// emitters prefer it over a path derived from the class name.
	File string `json:"file,omitempty"`

	DurationMS int64  `json:"duration_ms,omitempty"`
	Message    string `json:"message,omitempty"`

	// KnownFlaky is carried through from frameworks that report flakiness
	// themselves (Surefire <flakyFailure>).
	KnownFlaky bool `json:"known_flaky,omitempty"`
}

// Meta describes the run an observation batch belongs to.
type Meta struct {
	RunID   string
	Commit  string
	Branch  string
	Source  string
	Attempt int
}

// Store is an append-only observation log on disk.
type Store struct {
	path string
	mu   sync.Mutex
}

// Open prepares the log at dir/runs.ndjson, creating dir if needed.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = DefaultDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create %s: %w", dir, err)
	}
	return &Store{path: filepath.Join(dir, LogName)}, nil
}

// Path returns the log file location.
func (s *Store) Path() string { return s.path }

// NewRunID returns a sortable, collision-resistant run identifier.
func NewRunID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b[:])
}

// FromCases converts parsed JUnit cases into observations for one run.
func FromCases(cases []junit.Case, meta Meta) []Observation {
	now := time.Now().UTC()
	out := make([]Observation, 0, len(cases))

	for _, c := range cases {
		out = append(out, Observation{
			TS:         now,
			RunID:      meta.RunID,
			Commit:     meta.Commit,
			Branch:     meta.Branch,
			Source:     meta.Source,
			Attempt:    meta.Attempt,
			TestID:     c.ID(),
			Suite:      c.Suite,
			Class:      c.Class,
			Name:       c.Name,
			Status:     c.Status,
			File:       c.File,
			DurationMS: c.Duration.Milliseconds(),
			Message:    c.Message,
			KnownFlaky: c.KnownFlaky,
		})
	}
	return out
}

// Append writes observations to the log. Each batch is serialized in memory
// and written with a single O_APPEND call, so concurrent shards interleave
// cleanly rather than tearing individual lines.
func (s *Store) Append(obs []Observation) error {
	if len(obs) == 0 {
		return nil
	}

	buf := make([]byte, 0, len(obs)*256)
	for _, o := range obs {
		line, err := json.Marshal(o)
		if err != nil {
			return fmt.Errorf("store: encode observation: %w", err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("store: open log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(buf); err != nil {
		return fmt.Errorf("store: write log: %w", err)
	}
	return nil
}

// All reads every observation in write order. Corrupt lines are skipped and
// returned as errors rather than aborting the read: a truncated line from a
// killed CI job should not make the whole history unreadable.
func (s *Store) All() ([]Observation, []error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []error{fmt.Errorf("store: open log: %w", err)}
	}
	defer f.Close()

	var (
		obs  []Observation
		errs []error
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for line := 1; sc.Scan(); line++ {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var o Observation
		if err := json.Unmarshal(raw, &o); err != nil {
			errs = append(errs, fmt.Errorf("store: %s:%d: %w", s.path, line, err))
			continue
		}
		obs = append(obs, o)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		errs = append(errs, fmt.Errorf("store: read log: %w", err))
	}
	return obs, errs
}

// ByTest groups observations by test ID, each group ordered oldest first so
// scoring can walk the series chronologically.
func (s *Store) ByTest() (map[string][]Observation, []error) {
	obs, errs := s.All()

	grouped := make(map[string][]Observation)
	for _, o := range obs {
		grouped[o.TestID] = append(grouped[o.TestID], o)
	}
	for id := range grouped {
		g := grouped[id]
		sort.SliceStable(g, func(i, j int) bool { return g[i].TS.Before(g[j].TS) })
		grouped[id] = g
	}
	return grouped, errs
}
