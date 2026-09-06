package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/rowhitswami/flakestat/internal/baseline"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

const checkHelp = `Gate CI on flakiness getting worse, not on flakiness existing.

A repo adopting flakestat usually already has flaky tests. Failing on all of
them turns the build permanently red and the gate gets deleted. Instead, record
today's flakiness as a baseline and fail only when something new appears or an
accepted test measurably worsens.

USAGE
  flakestat check [flags]
  flakestat check --update-baseline    # accept current flakiness

TYPICAL SETUP
  flakestat check --update-baseline    # once, then commit .flakestat/baseline.json
  flakestat check --fail-on-new        # in CI from then on

EXIT CODES
  0  nothing new
  1  newly flaky tests found      (with --fail-on-new)
  2  accepted tests got worse     (with --fail-on-regression)

FLAGS
`

func runCheck(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, checkHelp)
		fs.PrintDefaults()
	}

	var (
		dir          = fs.String("dir", store.DefaultDir, "state directory")
		baselinePath = fs.String("baseline", "", "baseline file (default <dir>/baseline.json)")
		update       = fs.Bool("update-baseline", false, "record current flakiness as accepted and exit")
		failOnNew    = fs.Bool("fail-on-new", true, "exit 1 when a test is newly flaky")
		failOnReg    = fs.Bool("fail-on-regression", false, "exit 2 when an accepted test worsens")
		delta        = fs.Float64("regression-delta", 0.10, "score increase before an accepted test counts as regressed")
		noColor      = fs.Bool("no-color", false, "disable colored output")
	)
	var cfg score.Config
	scoringFlags(fs, &cfg)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := loadConfigDefaults(fs, &cfg, dir, stderr); err != nil {
		return err
	}

	st, err := openStore(*dir)
	if err != nil {
		return err
	}

	path := *baselinePath
	if path == "" {
		path = baseline.Path(*dir)
	}

	grouped, errs := st.ByTest()
	warnAll(stderr, errs)

	if len(grouped) == 0 {
		fmt.Fprintf(stdout, "No history recorded yet in %s.\nRun \"flakestat hunt\" or \"flakestat ingest\" first.\n", st.Path())
		return nil
	}

	results := score.All(grouped, cfg)

	if *update {
		b := baseline.FromResults(results)
		if err := b.Save(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Recorded %d flaky test(s) as accepted in %s\n", len(b.Tests), path)
		if len(b.Tests) > 0 {
			fmt.Fprintln(stdout, "Commit this file so CI shares the same baseline.")
		}
		return nil
	}

	b, err := baseline.Load(path)
	if err != nil {
		return err
	}
	diff := baseline.Compare(b, results, *delta)

	paint := func(c, s string) string {
		if *noColor {
			return s
		}
		return c + s + "\033[0m"
	}

	const (
		red    = "\033[31m"
		yellow = "\033[33m"
		green  = "\033[32m"
	)

	if len(diff.New) > 0 {
		fmt.Fprintf(stdout, "%s\n", paint(red, fmt.Sprintf("NEWLY FLAKY (%d)", len(diff.New))))
		for _, r := range diff.New {
			fmt.Fprintf(stdout, "  %.2f  %s\n", r.Score, r.DisplayName())
		}
		fmt.Fprintln(stdout)
	}

	if len(diff.Regressed) > 0 {
		fmt.Fprintf(stdout, "%s\n", paint(yellow, fmt.Sprintf("REGRESSED (%d)", len(diff.Regressed))))
		for _, r := range diff.Regressed {
			prev := b.Tests[r.TestID]
			fmt.Fprintf(stdout, "  %.2f (was %.2f)  %s\n", r.Score, prev.Score, r.DisplayName())
		}
		fmt.Fprintln(stdout)
	}

	if len(diff.Fixed) > 0 {
		fmt.Fprintf(stdout, "%s\n", paint(green, fmt.Sprintf("FIXED (%d)", len(diff.Fixed))))
		for _, e := range diff.Fixed {
			fmt.Fprintf(stdout, "  %s\n", e.Name)
		}
		fmt.Fprintf(stdout, "  Run \"flakestat check --update-baseline\" to tighten the baseline.\n\n")
	}

	fmt.Fprintf(stdout, "%d new, %d regressed, %d accepted, %d fixed\n",
		len(diff.New), len(diff.Regressed), len(diff.Accepted), len(diff.Fixed))

	// Regression outranks new: exit 2 is the more specific signal.
	if *failOnReg && len(diff.Regressed) > 0 {
		return exitCoder{err: fmt.Errorf("%d accepted test(s) got worse", len(diff.Regressed)), code: 2}
	}
	if *failOnNew && len(diff.New) > 0 {
		return exitCoder{err: fmt.Errorf("%d newly flaky test(s)", len(diff.New)), code: 1}
	}
	return nil
}
