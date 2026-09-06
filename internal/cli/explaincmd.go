package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/rowhitswami/flakestat/internal/explain"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

const explainHelp = `Show why one test received its verdict.

A score on its own is not an argument. This shows the evidence behind it: the
run history, where the outcome flipped, how much of that happened on identical
code, and how far the verdict can be trusted at this sample size.

USAGE
  flakestat explain <test name or id>

The argument is matched against the test id, the exact name, and finally as a
case-insensitive substring. If several tests match, they are listed rather than
one being chosen for you.

FLAGS
`

func runExplain(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, explainHelp)
		fs.PrintDefaults()
	}

	var (
		dir     = fs.String("dir", store.DefaultDir, "state directory")
		limit   = fs.Int("history", explain.DefaultHistory, "how many recent observations to show")
		asJSON  = fs.Bool("json", false, "emit the explanation as JSON")
		noColor = fs.Bool("no-color", false, "disable colored output")
	)
	var cfg score.Config
	scoringFlags(fs, &cfg)

	if err := fs.Parse(permute(fs, args)); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if query == "" {
		fs.Usage()
		return fmt.Errorf("no test given")
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

	id, err := explain.Find(grouped, query)
	if err != nil {
		return err
	}

	// One evidence model; this command only renders it.
	e := explain.Build(grouped[id], cfg, *limit)

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(e)
	}
	renderExplanation(stdout, e, *noColor)
	return nil
}

func renderExplanation(w io.Writer, e explain.Explanation, noColor bool) {
	const (
		reset  = "\033[0m"
		red    = "\033[31m"
		yellow = "\033[33m"
		green  = "\033[32m"
		gray   = "\033[90m"
		bold   = "\033[1m"
	)
	paint := func(c, s string) string {
		if noColor {
			return s
		}
		return c + s + reset
	}

	color := gray
	switch e.Verdict {
	case "flaky", "consistently-failing":
		color = red
	case "suspect":
		color = yellow
	case "stable":
		color = green
	}

	fmt.Fprintf(w, "%s\n", paint(bold, e.DisplayName))
	if e.File != "" {
		fmt.Fprintf(w, "%s\n", paint(gray, "  "+e.File))
	}
	fmt.Fprintf(w, "%s\n\n", paint(gray, "  id "+e.TestID))

	fmt.Fprintf(w, "  Classification: %s\n", paint(color, e.Verdict))
	fmt.Fprintf(w, "  Score:          %.2f\n", e.Score)
	fmt.Fprintf(w, "  Confidence:     %s%s\n", e.Confidence,
		paint(gray, fmt.Sprintf("  (lower bound %.3f over %d transition(s))", e.LowerBound, e.Transitions)))
	fmt.Fprintln(w)

	fmt.Fprintf(w, "  Observations:   %d\n", e.Observations)
	fmt.Fprintf(w, "  Passed:         %d\n", e.Passed)
	fmt.Fprintf(w, "  Failed:         %d\n", e.Failed)
	if e.Skipped > 0 {
		fmt.Fprintf(w, "  Skipped:        %d\n", e.Skipped)
	}
	fmt.Fprintf(w, "  Failure rate:   %.1f%%\n\n", e.FailureRate*100)

	fmt.Fprintf(w, "  Transitions:               %d\n", e.Transitions)
	fmt.Fprintf(w, "  Same-commit disagreements: %d\n", e.SameCommitFlips)
	if e.Commits > 0 || e.Branches > 0 {
		fmt.Fprintf(w, "  Seen across:               %d commit(s), %d branch(es)\n", e.Commits, e.Branches)
	}

	if len(e.History) > 0 {
		symbols, markers := e.Strip()
		fmt.Fprintf(w, "\n  History%s\n", paint(gray,
			fmt.Sprintf("  (last %d of %d, oldest first)", len(e.History), e.HistoryTotal)))
		fmt.Fprintf(w, "\n  %s\n", symbols)
		if strings.TrimSpace(markers) != "" {
			fmt.Fprintf(w, "  %s\n", markers)
		}
		fmt.Fprintf(w, "\n  %s\n", paint(gray, "P pass   F fail   - skip   ^ flip   ! flip on identical code"))
	}

	fmt.Fprintf(w, "\n  Why %s?\n\n", e.Verdict)
	for _, line := range wrap(e.Reason, 72) {
		fmt.Fprintf(w, "  %s\n", line)
	}

	if e.LastFailure != "" {
		fmt.Fprintf(w, "\n  Last failure\n")
		for _, line := range wrap(oneLine(e.LastFailure), 72) {
			fmt.Fprintf(w, "  %s\n", paint(gray, line))
		}
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// wrap breaks text at word boundaries so explanations stay readable in a
// terminal without depending on the terminal's own wrapping.
func wrap(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) > width {
			lines = append(lines, cur)
			cur = w
			continue
		}
		cur += " " + w
	}
	return append(lines, cur)
}
