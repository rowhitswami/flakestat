package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rowhitswami/flakestat/internal/quarantine"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

var quarantineHelp = `Emit a skip list your test runner actually accepts.

Unblocks a pipeline now, so flaky tests can be fixed on their own schedule
rather than under the pressure of a red build.

USAGE
  flakestat quarantine --format <` + strings.Join(quarantine.Formats(), "|") + `> [flags]

EXAMPLES
  flakestat quarantine --format pytest -o quarantine.txt
  pytest $(grep -v '^#' quarantine.txt | sed 's/^/--deselect /')

  flakestat quarantine --format go -o skip.txt
  go test -skip "$(grep -v '^#' skip.txt)" ./...

Consistently failing tests are excluded by default: those are broken rather
than flaky, and skipping them would hide a real defect.

FLAGS
`

func runQuarantine(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("quarantine", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, quarantineHelp)
		fs.PrintDefaults()
	}

	var (
		dir      = fs.String("dir", store.DefaultDir, "state directory")
		format   = fs.String("format", "yaml", "output format: "+strings.Join(quarantine.Formats(), ", "))
		output   = fs.String("o", "", "write to this file instead of stdout")
		broken   = fs.Bool("include-broken", false, "also include consistently failing tests")
		quietSum = fs.Bool("quiet", false, "suppress the summary line on stderr")
	)
	var cfg score.Config
	scoringFlags(fs, &cfg)

	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := loadConfigDefaults(fs, &cfg, dir, stderr); err != nil {
		return err
	}

	f, err := quarantine.ParseFormat(*format)
	if err != nil {
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
	opts := quarantine.Options{Format: f, IncludeBroken: *broken}

	w := stdout
	if *output != "" {
		file, err := os.Create(*output)
		if err != nil {
			return fmt.Errorf("quarantine: create %s: %w", *output, err)
		}
		defer file.Close()
		w = file
	}

	if err := quarantine.Render(w, results, opts); err != nil {
		return err
	}

	if !*quietSum {
		n := len(quarantine.Select(results, opts))
		dest := "stdout"
		if *output != "" {
			dest = *output
		}
		fmt.Fprintf(stderr, "Quarantined %d test(s) in %s format -> %s\n", n, f, dest)
	}
	return nil
}
