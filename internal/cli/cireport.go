package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rowhitswami/flakestat/internal/baseline"
	"github.com/rowhitswami/flakestat/internal/ci"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

const ciReportHelp = `Render a CI-facing reliability report.

Combines the current classification counts with the same baseline comparison
"check" performs, so the two can never disagree. The report is plain markdown:
suitable for a GitHub job summary, a pull request comment, or a merge request
note on any other provider.

USAGE
  flakestat ci-report [flags]

INSIDE GITHUB ACTIONS
  flakestat ci-report --step-summary --annotations

  --step-summary appends to $GITHUB_STEP_SUMMARY when it is set, and does
  nothing when it is not, so the same command is safe to run locally.

EXIT CODE
  Always 0. This command reports; use "flakestat check" to gate a build.

FLAGS
`

func runCIReport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ci-report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, ciReportHelp)
		fs.PrintDefaults()
	}

	var (
		dir          = fs.String("dir", store.DefaultDir, "state directory")
		baselinePath = fs.String("baseline", "", "baseline file (default <dir>/baseline.json)")
		output       = fs.String("o", "", "write the report to this file as well as stdout")
		stepSummary  = fs.Bool("step-summary", false, "append to $GITHUB_STEP_SUMMARY when running in GitHub Actions")
		annotations  = fs.Bool("annotations", false, "emit GitHub workflow annotations for new and regressed tests")
		maxRows      = fs.Int("max-rows", ci.DefaultMaxRows, "cap the highlight table so large suites stay readable")
		delta        = fs.Float64("regression-delta", 0.10, "score increase before an accepted test counts as regressed")
		failOnNew    = fs.Bool("fail-on-new", true, "treat newly flaky tests as a gate failure in the report")
		failOnReg    = fs.Bool("fail-on-regression", false, "treat regressions as a gate failure in the report")
	)
	var cfg score.Config
	scoringFlags(fs, &cfg)

	if err := fs.Parse(permute(fs, args)); err != nil {
		return err
	}
	if err := loadConfigDefaults(fs, &cfg, dir, stderr); err != nil {
		return err
	}

	st, err := openStore(*dir)
	if err != nil {
		return err
	}

	grouped, errs := st.ByTest()
	warnAll(stderr, errs)
	if len(grouped) == 0 {
		return fmt.Errorf("no history recorded yet in %s; run \"flakestat hunt\" or \"flakestat ingest\" first", st.Path())
	}

	results := score.All(grouped, cfg)

	path := *baselinePath
	if path == "" {
		path = baseline.Path(*dir)
	}
	base, err := baseline.Load(path)
	if err != nil {
		return err
	}
	diff := baseline.Compare(base, results, *delta)

	// Mirror check's decision rather than re-deriving it.
	gateFailed, reason := gateOutcome(diff, *failOnNew, *failOnReg)

	rep := ci.Build(results, base, diff, gateFailed, reason)
	md := rep.Markdown(*maxRows)

	fmt.Fprint(stdout, md)

	if *output != "" {
		if err := os.WriteFile(*output, []byte(md), 0o644); err != nil {
			return fmt.Errorf("ci-report: write %s: %w", *output, err)
		}
	}

	// Appending is deliberate: a job may write several summaries, and
	// truncating would discard another step's output.
	if *stepSummary {
		if p := ci.StepSummaryPath(); p != "" {
			f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				fmt.Fprintf(stderr, "warning: could not write the job summary: %v\n", err)
			} else {
				defer f.Close()
				if _, err := f.WriteString(md); err != nil {
					fmt.Fprintf(stderr, "warning: could not write the job summary: %v\n", err)
				}
			}
		} else if !ci.IsGitHubActions() {
			fmt.Fprintln(stderr, "note: --step-summary had no effect; GITHUB_STEP_SUMMARY is not set")
		}
	}

	// Annotations go to stdout as workflow commands, which GitHub reads from
	// the log. Tests without a reported source file are skipped rather than
	// given an invented location.
	if *annotations {
		for _, a := range rep.Annotations(*maxRows) {
			fmt.Fprintln(stdout, a)
		}
	}
	return nil
}

// gateOutcome reproduces check's precedence: a regression is the more specific
// signal, so it is reported ahead of a new flake.
func gateOutcome(diff baseline.Diff, failOnNew, failOnReg bool) (bool, string) {
	if failOnReg && len(diff.Regressed) > 0 {
		return true, fmt.Sprintf("%d accepted test(s) got worse", len(diff.Regressed))
	}
	if failOnNew && len(diff.New) > 0 {
		return true, fmt.Sprintf("%d newly flaky test(s)", len(diff.New))
	}
	return false, ""
}
