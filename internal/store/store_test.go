package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
)

func TestAppendAndReadRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	want := []Observation{
		{TS: time.Now().UTC(), RunID: "r1", TestID: "a", Name: "test_a", Status: junit.StatusPass},
		{TS: time.Now().UTC(), RunID: "r1", TestID: "b", Name: "test_b", Status: junit.StatusFail, Message: "boom"},
	}
	if err := s.Append(want); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, errs := s.All()
	if len(errs) > 0 {
		t.Fatalf("All: %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("observations = %d, want 2", len(got))
	}
	if got[1].Message != "boom" || got[1].Status != junit.StatusFail {
		t.Errorf("round trip lost data: %+v", got[1])
	}
}

func TestReadingMissingLogIsNotAnError(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	obs, errs := s.All()
	if len(errs) > 0 {
		t.Errorf("errors = %v, want none for a fresh store", errs)
	}
	if len(obs) != 0 {
		t.Errorf("observations = %d, want 0", len(obs))
	}
}

// A truncated line from a killed CI job must not make the rest unreadable.
func TestCorruptLineIsSkippedNotFatal(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := s.Append([]Observation{{TestID: "a", Name: "test_a", Status: junit.StatusPass}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Simulate a torn write followed by a healthy one.
	f, err := os.OpenFile(filepath.Join(dir, LogName), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := f.WriteString("{\"test_id\":\"b\",\"na\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	if err := s.Append([]Observation{{TestID: "c", Name: "test_c", Status: junit.StatusPass}}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	obs, errs := s.All()
	if len(errs) != 1 {
		t.Errorf("errors = %d, want 1 corrupt-line error", len(errs))
	}
	if len(obs) != 2 {
		t.Errorf("observations = %d, want 2 healthy records preserved", len(obs))
	}
}

func TestByTestGroupsChronologically(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	base := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	// Appended newest first, to prove ByTest reorders.
	err = s.Append([]Observation{
		{TS: base.Add(2 * time.Minute), TestID: "a", Name: "test_a", Status: junit.StatusFail},
		{TS: base, TestID: "a", Name: "test_a", Status: junit.StatusPass},
		{TS: base.Add(time.Minute), TestID: "b", Name: "test_b", Status: junit.StatusPass},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	grouped, errs := s.ByTest()
	if len(errs) > 0 {
		t.Fatalf("ByTest: %v", errs)
	}
	if len(grouped) != 2 {
		t.Fatalf("groups = %d, want 2", len(grouped))
	}
	if a := grouped["a"]; len(a) != 2 || a[0].Status != junit.StatusPass {
		t.Errorf("group 'a' not ordered oldest first: %+v", a)
	}
}

// Parallel CI shards append concurrently; no line may be torn.
func TestConcurrentAppendsDoNotTearLines(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			batch := make([]Observation, 8)
			for j := range batch {
				batch[j] = Observation{TestID: "t", Name: "test_t", Status: junit.StatusPass, RunID: "r"}
			}
			if err := s.Append(batch); err != nil {
				t.Errorf("Append: %v", err)
			}
		}(i)
	}
	wg.Wait()

	obs, errs := s.All()
	if len(errs) > 0 {
		t.Errorf("errors = %v, want none", errs)
	}
	if len(obs) != 128 {
		t.Errorf("observations = %d, want 128", len(obs))
	}
}

func TestFromCasesCarriesFlakyMarker(t *testing.T) {
	cases := []junit.Case{
		{Suite: "s", Class: "C", Name: "test_a", Status: junit.StatusPass, KnownFlaky: true, Duration: 1500 * time.Millisecond},
	}

	obs := FromCases(cases, Meta{RunID: "r1", Commit: "abc", Source: SourceHunt})
	if len(obs) != 1 {
		t.Fatalf("observations = %d, want 1", len(obs))
	}
	if !obs[0].KnownFlaky {
		t.Error("KnownFlaky was not carried through")
	}
	if obs[0].DurationMS != 1500 {
		t.Errorf("DurationMS = %d, want 1500", obs[0].DurationMS)
	}
	if obs[0].TestID != cases[0].ID() {
		t.Error("TestID does not match the case ID")
	}
}

func TestNewRunIDIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewRunID()
		if seen[id] {
			t.Fatalf("duplicate run ID: %s", id)
		}
		seen[id] = true
	}
}
