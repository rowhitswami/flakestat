package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/rowhitswami/flakestat/internal/report"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

const reportHelp = `Score recorded history and print a report.

USAGE
  flakestat report [flags]

EXIT CODES
  0  no flaky tests above the threshold
  1  flaky tests found (only with --fail-on-flaky)

FLAGS
`

func runReport(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, reportHelp)
		fs.PrintDefaults()
	}

	var (
		dir         = fs.String("dir", store.DefaultDir, "state directory")
		format      = fs.String("format", "table", "output format: table, json, markdown")
		top         = fs.Int("top", 0, "show only the worst N tests (0 = all)")
		all         = fs.Bool("all", false, "include stable and unscored tests")
		noColor     = fs.Bool("no-color", false, "disable colored output")
		failOnFlaky = fs.Bool("fail-on-flaky", false, "exit non-zero if any test is flaky")
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

	grouped, errs := st.ByTest()
	warnAll(stderr, errs)

	if len(grouped) == 0 {
		fmt.Fprintf(stdout, "No history recorded yet in %s.\nRun \"flakestat hunt\" or \"flakestat ingest\" first.\n", st.Path())
		return nil
	}

	results := score.All(grouped, cfg)
	ropts := report.Options{Top: *top, All: *all, NoColor: *noColor}

	switch *format {
	case "json":
		err = report.JSON(stdout, results, ropts)
	case "markdown", "md":
		err = report.Markdown(stdout, results, ropts)
	case "table":
		err = report.Table(stdout, results, ropts)
	default:
		return fmt.Errorf("unknown --format %q (want table, json or markdown)", *format)
	}
	if err != nil {
		return err
	}

	if *failOnFlaky {
		if n := report.Summarize(results).Flaky; n > 0 {
			return exitCoder{err: fmt.Errorf("%d flaky test(s) found", n), code: 1}
		}
	}
	return nil
}
