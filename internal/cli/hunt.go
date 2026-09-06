package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/rowhitswami/flakestat/internal/config"
	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/report"
	"github.com/rowhitswami/flakestat/internal/runner"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

const huntHelp = `Run a test command repeatedly and detect disagreement between runs.

A burst holds the code constant, so any test that both passes and fails across
the runs is flaky by definition -- no history or threshold required.

USAGE
  flakestat hunt [flags] -- <test command>

EXAMPLE
  flakestat hunt --runs 20 \
    --junit '.flakestat/reports/junit-{run}.xml' \
    -- pytest --junitxml='.flakestat/reports/junit-{run}.xml'

CHASING ONE TEST
  Hunting a whole suite 100 times is expensive; hunting one suspect test is
  cheap and far more conclusive. Pass your runner's own filter:

    flakestat hunt --runs 100 -- pytest -k test_login --junitxml='{junit}'
    flakestat hunt --runs 100 -- gotestsum --junitfile '{junit}' -- -run '^TestFoo$' ./...

The literal {run} is replaced with the run number in both --junit and the test
command, so parallel runs write to separate report files. The run number is
also exported to the command as $FLAKESTAT_RUN.

ON --parallel
  Parallel runs execute copies of your test command in the same working
  directory. A suite that uses fixed paths, ports, or a shared database will
  collide with itself and look flaky when it is not. Candidates found under
  --parallel are therefore re-run sequentially before being reported as flaky;
  see --verify.

FLAGS
`

func runHunt(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("hunt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, huntHelp)
		fs.PrintDefaults()
	}

	var (
		runs      = fs.Int("runs", 20, "number of times to run the test command")
		parallel  = fs.Int("parallel", 1, "how many runs to execute concurrently (see ON --parallel)")
		junitPath = fs.String("junit", "", "path to the JUnit XML each run produces; supports {run}")
		timeout   = fs.Duration("timeout", 0, "per-run timeout, e.g. 90s (0 disables)")
		untilFail = fs.Bool("until-fail", false, "stop at the first failing run")
		dir       = fs.String("dir", store.DefaultDir, "state directory")
		history   = fs.Bool("history", false, "score all recorded history, not just this burst")
		quiet     = fs.Bool("quiet", false, "suppress per-run progress")
		noColor   = fs.Bool("no-color", false, "disable colored output")
		format    = fs.String("format", "table", "output format: table, json, markdown")
		all       = fs.Bool("all", false, "include stable tests in the report")
		top       = fs.Int("top", 0, "show only the worst N tests (0 = all)")
		verify    = fs.Int("verify", -1,
			"sequential runs used to confirm candidates found under --parallel (-1 auto, 0 off)")
	)
	var dims dimensionFlag
	registerDimensionFlag(fs, &dims)

	var cfg score.Config
	scoringFlags(fs, &cfg)

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Config supplies defaults; anything given on the command line wins.
	conf, err := config.Load(config.Find("."))
	if err != nil {
		return err
	}
	set := setFlags(fs)

	command := fs.Args()
	if len(command) == 0 {
		command = conf.Command
	}
	if len(command) == 0 {
		fs.Usage()
		if conf.Path() == "" {
			return fmt.Errorf("no test command given: put it after --, or run \"flakestat init\" to create %s", config.FileName)
		}
		return fmt.Errorf("no test command given and %s has no \"command\"", conf.Path())
	}

	if !set["junit"] && conf.JUnit != "" {
		*junitPath = conf.JUnit
	}
	if !set["runs"] && conf.Runs > 0 {
		*runs = conf.Runs
	}
	if !set["parallel"] && conf.Parallel > 0 {
		*parallel = conf.Parallel
	}
	if !set["dir"] && conf.Dir != "" {
		*dir = conf.Dir
	}
	if !set["timeout"] && conf.Timeout != "" {
		d, err := time.ParseDuration(conf.Timeout)
		if err != nil {
			return fmt.Errorf("%s: invalid timeout %q: %w", conf.Path(), conf.Timeout, err)
		}
		*timeout = d
	}
	mergeScoring(&cfg, conf.Scoring, set)

	if conf.Path() != "" {
		fmt.Fprintf(stderr, "Using config %s\n", conf.Path())
	}

	opts := runner.Options{
		Command:      command,
		Runs:         *runs,
		Parallel:     *parallel,
		JUnitPattern: *junitPath,
		Timeout:      *timeout,
		UntilFail:    *untilFail,
	}
	if err := opts.Validate(); err != nil {
		return err
	}

	if *junitPath == "" {
		fmt.Fprintln(stderr,
			"warning: no --junit path given; falling back to exit codes only.\n"+
				"         Suite-level flakiness will still be detected, but not which test is at fault.")
	}

	// Resolve the verification budget: on by default only where it is needed.
	verifyRuns := *verify
	if verifyRuns < 0 {
		verifyRuns = 0
		if *parallel > 1 {
			verifyRuns = 5
		}
	}

	if *parallel > 1 {
		fmt.Fprintln(stderr,
			"note: --parallel runs copies of your test command in the same working directory.\n"+
				"      Suites using fixed paths, ports or a shared database can collide with\n"+
				"      themselves and appear flaky when they are not.")
		if verifyRuns > 0 {
			fmt.Fprintf(stderr,
				"      Candidates will be re-run %d times sequentially to confirm (--verify 0 to skip).\n", verifyRuns)
		} else {
			fmt.Fprintln(stderr,
				"      Verification is disabled, so findings are unconfirmed.")
		}
	}

	st, err := openStore(*dir)
	if err != nil {
		return err
	}

	commit, branch := gitInfo()
	burstID := store.NewRunID()

	// flakestat spawns the test command here, so this machine really is the
	// execution host.
	dimensions, err := resolveDimensions(dims, nil, true)
	if err != nil {
		return err
	}

	// With a machine-readable format, stdout must contain only the document.
	// Progress goes to stderr so "flakestat hunt --format json | jq" works.
	progressOut := stdout
	if *format != "table" {
		progressOut = stderr
	}

	fmt.Fprintf(progressOut, "Hunting flakes: %d run(s), %d at a time\n", *runs, max(*parallel, 1))

	start := time.Now()
	results, err := executeAndRecord(st, opts, store.Meta{
		RunID: burstID, Commit: commit, Branch: branch, Source: store.SourceHunt,
		Dimensions: dimensions,
	}, progressOut, stderr, *quiet, *runs)
	if err != nil {
		return err
	}

	fmt.Fprintf(progressOut, "\nCompleted %d run(s) in %s\n\n", len(results), time.Since(start).Round(time.Millisecond))

	// Score this burst by default. Within a burst the code is constant, so two
	// runs are enough to prove flakiness -- the history minimum does not apply.
	burstCfg := cfg
	if !*history {
		burstCfg.MinRuns = 2
	}

	grouped, errs := st.ByTest()
	warnAll(stderr, errs)
	if !*history {
		grouped = filterByRun(grouped, burstID)
	}
	scored := score.All(grouped, burstCfg)

	// Confirm parallel findings sequentially before reporting them as fact.
	var demoted []score.Result
	if verifyRuns > 0 && *parallel > 1 {
		candidates := flakyOf(scored)
		if len(candidates) > 0 {
			fmt.Fprintf(progressOut, "Verifying %d candidate(s) with %d sequential run(s)...\n",
				len(candidates), verifyRuns)

			verifyOpts := opts
			verifyOpts.Parallel = 1
			verifyOpts.Runs = verifyRuns
			verifyOpts.UntilFail = false

			verifyID := store.NewRunID()
			if _, err := executeAndRecord(st, verifyOpts, store.Meta{
				RunID: verifyID, Commit: commit, Branch: branch, Source: store.SourceHunt,
				Dimensions: dimensions,
			}, progressOut, stderr, true, verifyRuns); err != nil {
				return err
			}

			vGrouped, vErrs := st.ByTest()
			warnAll(stderr, vErrs)
			vCfg := cfg
			vCfg.MinRuns = 2
			confirmed := flakySet(score.All(filterByRun(vGrouped, verifyID), vCfg))

			for i := range scored {
				if scored[i].Verdict != score.ClassFlaky || confirmed[scored[i].TestID] {
					continue
				}
				// Did not reproduce without parallel contention. It may still be
				// flaky, so demote to suspect rather than discarding it.
				scored[i].Verdict = score.ClassSuspect
				demoted = append(demoted, scored[i])
			}
			score.Sort(scored)
			fmt.Fprintln(progressOut)
		}
	}

	ropts := report.Options{Top: *top, All: *all, NoColor: *noColor}

	switch *format {
	case "json":
		if err := report.JSON(stdout, scored, ropts); err != nil {
			return err
		}
	case "markdown", "md":
		if err := report.Markdown(stdout, scored, ropts); err != nil {
			return err
		}
	case "table":
		if err := report.Table(stdout, scored, ropts); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown --format %q (want table, json or markdown)", *format)
	}

	if len(demoted) > 0 && *format == "table" {
		fmt.Fprintf(stdout,
			"\n%d candidate(s) did not reproduce in %d sequential run(s) and were demoted to suspect:\n",
			len(demoted), verifyRuns)
		for _, r := range demoted {
			fmt.Fprintf(stdout, "  %s\n", r.DisplayName())
		}
		fmt.Fprintln(stdout,
			"These most likely collided with themselves under --parallel rather than being flaky.\n"+
				"Re-run with --parallel 1 to be certain.")
	}
	return nil
}

// executeAndRecord runs a burst and appends its observations to the store.
func executeAndRecord(
	st *store.Store, opts runner.Options, meta store.Meta,
	stdout, stderr io.Writer, quiet bool, total int,
) ([]runner.Result, error) {
	var mu sync.Mutex
	var done int

	progress := func(r runner.Result) {
		if quiet {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		done++

		status := "ok"
		if r.Failed() {
			status = "FAIL"
		}
		fmt.Fprintf(stdout, "  run %d/%d  %-4s  %s\n",
			done, total, status, r.Duration.Round(time.Millisecond))
	}

	results, err := runner.Run(context.Background(), opts, progress)
	if err != nil {
		return nil, err
	}

	var (
		obs        []store.Observation
		missing    int
		firstBroke = -1
	)
	for i, r := range results {
		if r.Err != nil {
			fmt.Fprintf(stderr, "warning: %v\n", r.Err)
		}

		cases := r.Cases
		if r.ReportMissing {
			missing++
			if firstBroke < 0 && (r.ExitCode != 0 || r.Err != nil) {
				firstBroke = i
			}
			cases = []junit.Case{runner.SyntheticCase(r)}
		}

		batch := meta
		batch.Attempt = r.Index
		obs = append(obs, store.FromCases(cases, batch)...)
	}

	if err := st.Append(obs); err != nil {
		return nil, err
	}

	if missing > 0 && opts.JUnitPattern != "" {
		fmt.Fprintf(stderr,
			"warning: %d of %d run(s) produced no JUnit XML at %q; those runs were scored by exit code only\n",
			missing, len(results), opts.JUnitPattern)
	}

	// Without this the user sees "consistently-failing <whole suite>" and no
	// reason for it, when the command's own output says exactly what broke.
	if firstBroke >= 0 {
		if out := tailLines(results[firstBroke].Output, 20); out != "" {
			fmt.Fprintf(stderr, "\n--- output from failing run %d (last 20 lines) ---\n%s\n--- end ---\n\n",
				results[firstBroke].Index, out)
		}
	}

	return results, nil
}

func flakyOf(results []score.Result) []score.Result {
	var out []score.Result
	for _, r := range results {
		if r.Verdict == score.ClassFlaky {
			out = append(out, r)
		}
	}
	return out
}

func flakySet(results []score.Result) map[string]bool {
	out := make(map[string]bool)
	for _, r := range results {
		if r.Verdict == score.ClassFlaky {
			out[r.TestID] = true
		}
	}
	return out
}

// tailLines returns at most the last n lines of s.
func tailLines(s string, n int) string {
	s = strings.TrimRight(s, "\n \t")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// filterByRun narrows history to a single burst.
func filterByRun(grouped map[string][]store.Observation, runID string) map[string][]store.Observation {
	out := make(map[string][]store.Observation, len(grouped))
	for id, obs := range grouped {
		kept := make([]store.Observation, 0, len(obs))
		for _, o := range obs {
			if o.RunID == runID {
				kept = append(kept, o)
			}
		}
		if len(kept) > 0 {
			out[id] = kept
		}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
