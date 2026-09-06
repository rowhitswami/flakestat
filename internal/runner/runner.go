// Package runner executes a test command repeatedly and collects its reports.
//
// A burst run holds the code constant, so any disagreement between runs is
// flakiness by definition -- the strongest signal available.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rowhitswami/flakestat/internal/junit"
)

// RunPlaceholder is substituted with the 1-based run index in the command and
// the report path, so parallel runs write to distinct files.
const RunPlaceholder = "{run}"

// JUnitPlaceholder is substituted in the command with the resolved report path.
// Without it the report path has to be written twice -- once in --junit and
// once inside the test command -- and the two silently drift apart.
const JUnitPlaceholder = "{junit}"

// Options configures a burst.
type Options struct {
	Command      []string // argv; required
	Runs         int
	Parallel     int
	JUnitPattern string // path to the report each run produces
	Timeout      time.Duration
	UntilFail    bool // stop early once a run fails
	Env          []string
}

// Result is the outcome of a single run.
type Result struct {
	Index      int
	ExitCode   int
	Duration   time.Duration
	Cases      []junit.Case
	ReportPath string

	// ReportMissing means no JUnit XML was produced, so only the process exit
	// code is known and per-test granularity is unavailable.
	ReportMissing bool

	// Err is set for failures to execute or time out, not for test failures.
	Err    error
	Output string
}

// Failed reports whether this run had any failing test, or a non-zero exit
// when no report was produced.
func (r Result) Failed() bool {
	if r.Err != nil {
		return true
	}
	for _, c := range r.Cases {
		if c.Status == junit.StatusFail || c.Status == junit.StatusError {
			return true
		}
	}
	return r.ReportMissing && r.ExitCode != 0
}

// Validate checks the options before any process is started.
func (o Options) Validate() error {
	if len(o.Command) == 0 {
		return errors.New("runner: no test command given")
	}
	if o.Runs < 1 {
		return errors.New("runner: --runs must be at least 1")
	}
	if o.Parallel > 1 && o.JUnitPattern != "" && !strings.Contains(o.JUnitPattern, RunPlaceholder) {
		return fmt.Errorf(
			"runner: --parallel %d needs %s in --junit so runs do not overwrite each other's reports (got %q)",
			o.Parallel, RunPlaceholder, o.JUnitPattern)
	}
	return nil
}

// Run executes the command Runs times and returns results in run order.
func Run(ctx context.Context, opts Options, progress func(Result)) ([]Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	parallel := opts.Parallel
	if parallel < 1 {
		parallel = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu      sync.Mutex
		results = make([]Result, opts.Runs)
		sem     = make(chan struct{}, parallel)
		wg      sync.WaitGroup
		stop    bool
	)

	for i := 0; i < opts.Runs; i++ {
		mu.Lock()
		halted := stop
		mu.Unlock()
		if halted || ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			res := runOnce(ctx, opts, idx)

			mu.Lock()
			results[idx] = res
			if opts.UntilFail && res.Failed() {
				stop = true
				cancel()
			}
			mu.Unlock()

			if progress != nil {
				progress(res)
			}
		}(i)
	}
	wg.Wait()

	// Drop slots for runs that were never started because of --until-fail.
	out := make([]Result, 0, opts.Runs)
	for _, r := range results {
		if r.Index == 0 && r.Duration == 0 && r.Cases == nil && r.Err == nil && r.ReportPath == "" {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func runOnce(ctx context.Context, opts Options, idx int) Result {
	run := idx + 1
	res := Result{Index: run}

	reportPath := expand(opts.JUnitPattern, run)
	res.ReportPath = reportPath

	args := make([]string, len(opts.Command))
	for i, a := range opts.Command {
		args[i] = expand(strings.ReplaceAll(a, JUnitPlaceholder, reportPath), run)
	}

	// Remove any stale report so a crashed run is not scored against the
	// previous run's results.
	if reportPath != "" {
		_ = os.Remove(reportPath)
	}

	runCtx := ctx
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)
	cmd.Env = append(os.Environ(), opts.Env...)
	cmd.Env = append(cmd.Env,
		"FLAKESTAT_RUN="+strconv.Itoa(run),
		"FLAKESTAT=1",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	err := cmd.Run()
	res.Duration = time.Since(start)
	res.Output = out.String()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		// Could not start the process at all: a real error, not a test failure.
		res.Err = fmt.Errorf("run %d: %w", run, err)
		return res
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.Err = fmt.Errorf("run %d: timed out after %s", run, opts.Timeout)
		return res
	}

	if reportPath == "" {
		res.ReportMissing = true
		return res
	}

	rep, errs := junit.ParseGlob(reportPath)
	if rep == nil || len(rep.Cases) == 0 {
		res.ReportMissing = true
		if len(errs) > 0 {
			res.Err = errs[0]
		}
		return res
	}
	res.Cases = rep.Cases
	return res
}

func expand(s string, run int) string {
	if s == "" {
		return s
	}
	return strings.ReplaceAll(s, RunPlaceholder, strconv.Itoa(run))
}

// SyntheticCase represents a whole suite when no JUnit report was produced.
// Granularity is lost, but suite-level flakiness is still measurable.
func SyntheticCase(r Result) junit.Case {
	status := junit.StatusPass
	if r.ExitCode != 0 || r.Err != nil {
		status = junit.StatusFail
	}
	msg := ""
	if r.Err != nil {
		msg = r.Err.Error()
	} else if status == junit.StatusFail {
		msg = fmt.Sprintf("exit code %d", r.ExitCode)
	}

	return junit.Case{
		Suite:    "flakestat",
		Class:    "suite",
		Name:     "<whole suite>",
		Status:   status,
		Duration: r.Duration,
		Message:  msg,
	}
}
