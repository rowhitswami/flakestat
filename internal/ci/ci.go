// Package ci composes a CI-facing report from results the rest of flakestat
// already produced.
//
// It calculates no classifications, confidence or baseline differences of its
// own: those come from score and baseline. Keeping that direction of
// dependency means a second CI provider is a new renderer, not a second
// implementation of the analysis.
package ci

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/rowhitswami/flakestat/internal/baseline"
	"github.com/rowhitswami/flakestat/internal/report"
	"github.com/rowhitswami/flakestat/internal/score"
)

// DefaultMaxRows bounds the highlight table. A suite with hundreds of new
// flakes should produce a readable summary, not an unbounded wall.
const DefaultMaxRows = 20

// Status labels a highlighted test in terms the reader already knows from
// `check`, rather than inventing competing vocabulary.
type Status string

const (
	StatusNew       Status = "New flaky"
	StatusRegressed Status = "Regressed"
)

// Highlight is one test worth a reader's attention.
type Highlight struct {
	Name       string
	File       string
	Status     Status
	Score      float64
	Confidence string
	Previous   float64 // baseline score, for regressions
}

// Report is everything a CI surface needs, already reduced to values.
type Report struct {
	Summary    report.Summary
	Diff       baseline.Diff
	Highlights []Highlight

	// GateFailed mirrors what `check` decided; it is never recomputed here.
	GateFailed bool
	GateReason string
}

// Build assembles the report. The diff, baseline and gate state all come from
// the caller so that this package cannot disagree with `check`.
func Build(results []score.Result, base *baseline.Baseline, diff baseline.Diff, gateFailed bool, gateReason string) Report {
	r := Report{
		Summary:    report.Summarize(results),
		Diff:       diff,
		GateFailed: gateFailed,
		GateReason: gateReason,
	}

	prev := map[string]float64{}
	if base != nil {
		for id, e := range base.Tests {
			prev[id] = e.Score
		}
	}

	for _, res := range diff.New {
		r.Highlights = append(r.Highlights, Highlight{
			Name: res.DisplayName(), File: res.File, Status: StatusNew,
			Score: res.Score, Confidence: string(res.Level),
		})
	}
	for _, res := range diff.Regressed {
		r.Highlights = append(r.Highlights, Highlight{
			Name: res.DisplayName(), File: res.File, Status: StatusRegressed,
			Score: res.Score, Confidence: string(res.Level), Previous: prev[res.TestID],
		})
	}

	// Worst first, so a truncated table still shows what matters most.
	sort.SliceStable(r.Highlights, func(i, j int) bool {
		if r.Highlights[i].Score != r.Highlights[j].Score {
			return r.Highlights[i].Score > r.Highlights[j].Score
		}
		return r.Highlights[i].Name < r.Highlights[j].Name
	})
	return r
}

// Marker identifies flakestat's own comment so repeated runs update one
// comment instead of accumulating duplicates.
const Marker = "<!-- flakestat-report -->"

// Markdown renders the report for a job summary or a pull request comment.
func (r Report) Markdown(maxRows int) string {
	if maxRows <= 0 {
		maxRows = DefaultMaxRows
	}
	var b strings.Builder

	b.WriteString(Marker + "\n")
	b.WriteString("## flakestat\n\n")

	s := r.Summary
	fmt.Fprintf(&b, "Tests analyzed: **%d**\n\n", s.Total)
	fmt.Fprintf(&b, "| | |\n|---|--:|\n")
	fmt.Fprintf(&b, "| Stable | %d |\n", s.Stable)
	fmt.Fprintf(&b, "| Flaky | %d |\n", s.Flaky)
	fmt.Fprintf(&b, "| Suspect | %d |\n", s.Suspect)
	fmt.Fprintf(&b, "| Consistently failing | %d |\n", s.ConsistentlyFailing)
	fmt.Fprintf(&b, "| Insufficient data | %d |\n", s.InsufficientData)
	fmt.Fprintf(&b, "| Always skipped | %d |\n", s.AlwaysSkipped)

	b.WriteString("\n### Changes since the baseline\n\n")
	fmt.Fprintf(&b, "| | |\n|---|--:|\n")
	fmt.Fprintf(&b, "| New | %d |\n", len(r.Diff.New))
	fmt.Fprintf(&b, "| Regressed | %d |\n", len(r.Diff.Regressed))
	fmt.Fprintf(&b, "| Fixed | %d |\n", len(r.Diff.Fixed))
	fmt.Fprintf(&b, "| Accepted | %d |\n", len(r.Diff.Accepted))

	if len(r.Highlights) > 0 {
		b.WriteString("\n### New and regressed\n\n")
		b.WriteString("| Test | Status | Score | Confidence |\n|---|---|--:|---|\n")

		shown := r.Highlights
		if len(shown) > maxRows {
			shown = shown[:maxRows]
		}
		for _, h := range shown {
			conf := h.Confidence
			if conf == "" {
				conf = "-"
			}
			fmt.Fprintf(&b, "| `%s` | %s | %.2f | %s |\n",
				escapePipes(h.Name), h.Status, h.Score, conf)
		}
		if len(r.Highlights) > maxRows {
			fmt.Fprintf(&b, "\n_and %d more._\n", len(r.Highlights)-maxRows)
		}
	}

	if len(r.Diff.Fixed) > 0 {
		b.WriteString("\n<details><summary>Fixed since the baseline</summary>\n\n")
		for _, e := range r.Diff.Fixed {
			fmt.Fprintf(&b, "- `%s`\n", escapePipes(e.Name))
		}
		b.WriteString("\n</details>\n")
	}

	b.WriteString("\n")
	if r.GateFailed {
		fmt.Fprintf(&b, "`flakestat check`: **FAILED** — %s\n", r.GateReason)
	} else {
		b.WriteString("`flakestat check`: **passed**\n")
	}
	return b.String()
}

// Annotations renders GitHub workflow commands for tests worth flagging in the
// diff view.
//
// Only tests whose source file the framework actually reported are annotated:
// inventing a location would point reviewers at the wrong line. Everything
// else still appears in the summary and the comment, so a framework without
// file metadata loses nothing but the inline marker.
func (r Report) Annotations(maxCount int) []string {
	if maxCount <= 0 {
		maxCount = DefaultMaxRows
	}

	var out []string
	for _, h := range r.Highlights {
		if h.File == "" {
			continue
		}
		if len(out) >= maxCount {
			break
		}
		out = append(out, fmt.Sprintf("::warning file=%s,title=%s::%s (score %.2f, %s confidence)",
			h.File, escapeAnnotation(string(h.Status)), escapeAnnotation(h.Name), h.Score, h.Confidence))
	}
	return out
}

// IsGitHubActions reports whether this is running inside GitHub Actions, using
// the variable GitHub itself documents as always set.
func IsGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// StepSummaryPath returns the job-summary file, empty when not available.
func StepSummaryPath() string { return os.Getenv("GITHUB_STEP_SUMMARY") }

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

// escapeAnnotation applies the escaping GitHub requires inside workflow
// commands, where raw newlines and delimiters would truncate the message.
func escapeAnnotation(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
	return r.Replace(s)
}
