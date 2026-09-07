package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

func compact(t *testing.T, stateDir string, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runCompact(append([]string{"--dir", stateDir}, args...), &out, &errOut)
	return out.String() + errOut.String(), err
}

func doubleLog(t *testing.T, stateDir string) {
	t.Helper()
	log := filepath.Join(stateDir, store.LogName)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, append(append([]byte{}, data...), data...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func lineCount(t *testing.T, stateDir string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(stateDir, store.LogName))
	if err != nil {
		t.Fatal(err)
	}
	return len(bytes.Split(bytes.TrimSpace(data), []byte("\n")))
}

func TestCompactRemovesDuplicatesFromTheFile(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	ingest(t, state, writeReport(t, dir, "junit.xml", repeatedReport))
	doubleLog(t, state)

	if got := lineCount(t, state); got != 6 {
		t.Fatalf("setup: %d lines, want 6", got)
	}
	out, err := compact(t, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Removed 3 duplicate observation(s), leaving 3") {
		t.Errorf("unexpected output: %q", out)
	}
	if got := lineCount(t, state); got != 3 {
		t.Errorf("%d lines on disk after compact, want 3", got)
	}
	if got := len(recorded(t, state)); got != 3 {
		t.Errorf("%d observations read back, want 3", got)
	}
}

func TestCompactIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	ingest(t, state, writeReport(t, dir, "junit.xml", repeatedReport))
	doubleLog(t, state)

	if _, err := compact(t, state); err != nil {
		t.Fatal(err)
	}
	out, err := compact(t, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nothing to remove") {
		t.Errorf("second compact should be a no-op, got %q", out)
	}
}

func TestCompactDryRunChangesNothing(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	ingest(t, state, writeReport(t, dir, "junit.xml", repeatedReport))
	doubleLog(t, state)

	out, err := compact(t, state, "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Would remove 3") {
		t.Errorf("unexpected output: %q", out)
	}
	if got := lineCount(t, state); got != 6 {
		t.Errorf("--dry-run rewrote the log: %d lines, want 6", got)
	}
}

// hunt watched every execution happen, so identical results are real repeats
// and compact must not touch them.
func TestCompactKeepsObservationsWithoutIdentity(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")

	st, err := store.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	obs := store.FromCases(huntCases(), store.Meta{Source: store.SourceHunt})
	if err := st.Append(append(obs, obs...)); err != nil {
		t.Fatal(err)
	}

	out, err := compact(t, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Nothing to remove") {
		t.Errorf("keyless observations were removed: %q", out)
	}
	if got := lineCount(t, state); got != 8 {
		t.Errorf("%d lines after compact, want all 8 kept", got)
	}
}

// A rewrite would silently delete lines that reading skipped, which may be the
// only remaining copy of that evidence.
func TestCompactRefusesToRewriteAnUnreadableLog(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	ingest(t, state, writeReport(t, dir, "junit.xml", repeatedReport))
	doubleLog(t, state)

	log := filepath.Join(state, store.LogName)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, append(data, []byte("{not json\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	before := lineCount(t, state)
	if _, err := compact(t, state); err == nil {
		t.Error("compact should refuse a log it cannot fully read")
	}
	if got := lineCount(t, state); got != before {
		t.Errorf("the log was rewritten anyway: %d lines, want %d", got, before)
	}
}

func huntCases() []junit.Case {
	out := make([]junit.Case, 0, 4)
	for i := 0; i < 4; i++ {
		out = append(out, junit.Case{Suite: "s", Class: "C", Name: "test_a", Status: junit.StatusPass})
	}
	return out
}
