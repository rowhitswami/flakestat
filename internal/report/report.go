// Package report renders scored results for humans and machines.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/rowhitswami/flakestat/internal/score"
)

// Options controls rendering.
type Options struct {
	Top      int  // 0 means no limit
	All      bool // include stable tests
	NoColor  bool
	Markdown bool
}

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

func verdictColor(v score.Class) string {
	switch v {
	case score.ClassFlaky:
		return colorRed
	case score.ClassSuspect:
		return colorYellow
	case score.ClassConsistentlyFail:
		return colorRed
	case score.ClassStable:
		return colorGreen
	default:
		return colorGray
	}
}

// filter drops rows that need no action unless All is set, then applies the
// Top limit. Consistently-failing rows are never dropped: hiding a broken test
// would be worse than the noise.
func filter(results []score.Result, opts Options) []score.Result {
	out := make([]score.Result, 0, len(results))
	for _, r := range results {
		if !opts.All {
			switch r.Verdict {
			case score.ClassStable, score.ClassInsufficient, score.ClassAlwaysSkipped:
				continue
			}
		}
		out = append(out, r)
	}
	if opts.Top > 0 && len(out) > opts.Top {
		out = out[:opts.Top]
	}
	return out
}

// Summary counts results by verdict.
type Summary struct {
	Total               int `json:"total"`
	Flaky               int `json:"flaky"`
	Suspect             int `json:"suspect"`
	Stable              int `json:"stable"`
	ConsistentlyFailing int `json:"consistently_failing"`
	InsufficientData    int `json:"insufficient_data"`
	AlwaysSkipped       int `json:"always_skipped"`
}

// Line renders the one-line tally. Categories that would read as zero noise
// are omitted rather than padding the line with "0 always skipped".
func (s Summary) Line() string {
	parts := []string{
		fmt.Sprintf("%d flaky", s.Flaky),
		fmt.Sprintf("%d suspect", s.Suspect),
		fmt.Sprintf("%d consistently failing", s.ConsistentlyFailing),
		fmt.Sprintf("%d stable", s.Stable),
	}
	if s.InsufficientData > 0 {
		parts = append(parts, fmt.Sprintf("%d unscored", s.InsufficientData))
	}
	if s.AlwaysSkipped > 0 {
		parts = append(parts, fmt.Sprintf("%d always skipped", s.AlwaysSkipped))
	}
	return fmt.Sprintf("%s (of %d tests)", strings.Join(parts, ", "), s.Total)
}

// Summarize counts every verdict across all results.
func Summarize(results []score.Result) Summary {
	s := Summary{Total: len(results)}
	for _, r := range results {
		switch r.Verdict {
		case score.ClassFlaky:
			s.Flaky++
		case score.ClassSuspect:
			s.Suspect++
		case score.ClassStable:
			s.Stable++
		case score.ClassConsistentlyFail:
			s.ConsistentlyFailing++
		case score.ClassInsufficient:
			s.InsufficientData++
		case score.ClassAlwaysSkipped:
			s.AlwaysSkipped++
		}
	}
	return s
}

// medianRuns is the typical number of scored runs behind these results, used to
// describe what the data could actually have revealed.
func medianRuns(results []score.Result) int {
	var runs []int
	for _, r := range results {
		if r.Runs > 0 {
			runs = append(runs, r.Runs)
		}
	}
	if len(runs) == 0 {
		return 0
	}
	sort.Ints(runs)
	return runs[len(runs)/2]
}

// detectionFloor states, in plain terms, the flake rate a given number of runs
// can reliably surface.
//
// Reporting a bare "no flaky tests detected" overclaims: it reads as "your
// suite is clean" when it often means "not at this sample size". The bands come
// from measured detection rates in VALIDATION.md.
func detectionFloor(runs int) (floor string, suggestMore bool) {
	switch {
	case runs < 10:
		return "about half the time", true
	case runs < 50:
		return "about a quarter of the time or more", true
	default:
		return "about 10% of the time or more", false
	}
}

// noFindings explains a clean result honestly, including what it could not have
// seen and how to look harder.
func noFindings(w io.Writer, results []score.Result, summary Summary) {
	fmt.Fprintf(w, "No flaky tests detected across %d test(s).\n", summary.Total)

	if n := medianRuns(results); n > 0 {
		floor, more := detectionFloor(n)
		fmt.Fprintf(w, "With %d run(s), this reliably finds tests that fail %s.\n", n, floor)
		if more {
			fmt.Fprintln(w, "Rarer flakes need more evidence -- try --runs 50, or record CI")
			fmt.Fprintln(w, "runs over time with \"flakestat ingest\", which costs nothing extra.")
		}
	}

	if summary.InsufficientData > 0 {
		fmt.Fprintf(w, "%d test(s) need more runs before they can be scored.\n", summary.InsufficientData)
	}
	if summary.AlwaysSkipped > 0 {
		fmt.Fprintf(w, "%d test(s) were skipped in every run and never scored.\n", summary.AlwaysSkipped)
	}
}

// alignment controls per-column padding.
type alignment int

const (
	alignLeft alignment = iota
	alignRight
)

// Table renders a human-readable report.
//
// Columns are padded manually rather than with text/tabwriter: tabwriter
// counts ANSI escape bytes toward cell width even when stripping them, so a
// column mixing colored and uncolored cells drifts out of alignment. Widths
// here are computed from the uncolored text, and color is applied afterwards.
func Table(w io.Writer, results []score.Result, opts Options) error {
	summary := Summarize(results)
	rows := filter(results, opts)

	if len(rows) == 0 {
		noFindings(w, results, summary)
		return nil
	}

	paint := func(c, s string) string {
		if opts.NoColor {
			return s
		}
		return c + s + colorReset
	}

	headers := []string{"VERDICT", "SCORE", "RUNS", "PASS/FAIL", "TEST"}
	aligns := []alignment{alignLeft, alignRight, alignRight, alignRight, alignLeft}

	// Plain cell text, used for both width computation and rendering.
	cells := make([][]string, 0, len(rows))
	for _, r := range rows {
		name := r.DisplayName()
		if r.KnownFlaky {
			name += " (framework-reported)"
		}
		cells = append(cells, []string{
			string(r.Verdict),
			fmt.Sprintf("%.2f", r.Score),
			fmt.Sprintf("%d", r.Runs),
			fmt.Sprintf("%d/%d", r.Passes, r.Fails),
			name,
		})
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range cells {
		for i, c := range row {
			if n := utf8.RuneCountInString(c); n > widths[i] {
				widths[i] = n
			}
		}
	}

	const gap = "  "

	var line strings.Builder
	for i, h := range headers {
		line.WriteString(pad(paint(colorBold, h), utf8.RuneCountInString(h), widths[i], aligns[i]))
		if i < len(headers)-1 {
			line.WriteString(gap)
		}
	}
	fmt.Fprintln(w, strings.TrimRight(line.String(), " "))

	for ri, row := range cells {
		line.Reset()
		for i, c := range row {
			text := c
			switch i {
			case 0:
				text = paint(verdictColor(rows[ri].Verdict), c)
			case 4:
				if rows[ri].KnownFlaky {
					text = strings.TrimSuffix(c, " (framework-reported)") +
						paint(colorGray, " (framework-reported)")
				}
			}
			line.WriteString(pad(text, utf8.RuneCountInString(c), widths[i], aligns[i]))
			if i < len(row)-1 {
				line.WriteString(gap)
			}
		}
		fmt.Fprintln(w, strings.TrimRight(line.String(), " "))
	}

	fmt.Fprintf(w, "\n%s\n", summary.Line())
	return nil
}

// pad aligns text to width, measuring by plainLen so ANSI escapes in text do
// not distort the padding.
func pad(text string, plainLen, width int, a alignment) string {
	fill := width - plainLen
	if fill < 0 {
		fill = 0
	}
	if a == alignRight {
		return strings.Repeat(" ", fill) + text
	}
	return text + strings.Repeat(" ", fill)
}

// jsonResult is the stable wire shape, decoupled from internal field names.
type jsonResult struct {
	TestID     string  `json:"test_id"`
	Name       string  `json:"name"`
	Suite      string  `json:"suite,omitempty"`
	Class      string  `json:"class,omitempty"`
	Verdict    string  `json:"verdict"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`
	FlipRate   float64 `json:"flip_rate"`
	Runs       int     `json:"runs"`
	Passes     int     `json:"passes"`
	Fails      int     `json:"fails"`
	Commits    int     `json:"commits"`
	KnownFlaky bool    `json:"known_flaky,omitempty"`
}

// JSON renders machine-readable output.
func JSON(w io.Writer, results []score.Result, opts Options) error {
	rows := filter(results, opts)

	out := struct {
		Summary Summary      `json:"summary"`
		Results []jsonResult `json:"results"`
	}{
		Summary: Summarize(results),
		Results: make([]jsonResult, 0, len(rows)),
	}

	for _, r := range rows {
		out.Results = append(out.Results, jsonResult{
			TestID:     r.TestID,
			Name:       r.Name,
			Suite:      r.Suite,
			Class:      r.Class,
			Verdict:    string(r.Verdict),
			Score:      r.Score,
			Confidence: r.Confidence,
			FlipRate:   r.FlipRate,
			Runs:       r.Runs,
			Passes:     r.Passes,
			Fails:      r.Fails,
			Commits:    r.Commits,
			KnownFlaky: r.KnownFlaky,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// Markdown renders a report suitable for a PR comment.
func Markdown(w io.Writer, results []score.Result, opts Options) error {
	summary := Summarize(results)
	rows := filter(results, opts)

	fmt.Fprintln(w, "## flakestat")
	fmt.Fprintln(w)

	if len(rows) == 0 {
		noFindings(w, results, summary)
		return nil
	}

	fmt.Fprintf(w, "**%d flaky**, %d suspect, %d consistently failing (of %d tests)\n\n",
		summary.Flaky, summary.Suspect, summary.ConsistentlyFailing, summary.Total)

	fmt.Fprintln(w, "| Verdict | Score | Runs | Pass/Fail | Test |")
	fmt.Fprintln(w, "| --- | --: | --: | --: | --- |")
	for _, r := range rows {
		name := escapePipes(r.DisplayName())
		if r.KnownFlaky {
			name += " _(framework-reported)_"
		}
		fmt.Fprintf(w, "| %s | %.2f | %d | %d/%d | `%s` |\n",
			r.Verdict, r.Score, r.Runs, r.Passes, r.Fails, name)
	}
	return nil
}

func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
