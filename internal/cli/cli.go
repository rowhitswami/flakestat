// Package cli implements the flakestat command line.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/rowhitswami/flakestat/internal/config"
	"github.com/rowhitswami/flakestat/internal/dimension"
	"github.com/rowhitswami/flakestat/internal/score"
	"github.com/rowhitswami/flakestat/internal/store"
)

// Version is the build version, overridden at release time via -ldflags.
var Version = "dev"

const usage = `flakestat - find flaky tests, without a SaaS account

USAGE
  flakestat <command> [flags]

COMMANDS
  init        Detect this project and write .flakestat.json
  hunt        Run a test command N times and detect disagreement
  ingest      Load JUnit XML from CI into the history
  report      Score recorded history and print a report
  explain     Show why one test received its verdict
  check       Fail CI when flakiness gets worse, not when it exists
  quarantine  Emit a skip list your test runner accepts
  ci-report   Render a CI-facing report for job summaries and PR comments
  version     Print the version

Run "flakestat <command> -h" for command flags.

EXAMPLES
  # Detect the project, then hunt for flakes locally.
  flakestat init
  flakestat hunt

  # Or without a config file:
  flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \
    -- pytest --junitxml='{junit}'

  # Record a CI run, then review the trend.
  flakestat ingest 'reports/*.xml'
  flakestat report --top 20

  # Accept today's flakiness, then gate on anything new.
  flakestat check --update-baseline
  flakestat check --fail-on-new

  # Unblock the pipeline while the flakes get fixed.
  flakestat quarantine --format pytest -o quarantine.txt
`

// Main runs the CLI and returns a process exit code.
func Main(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	cmd, rest := args[1], args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(rest, stdout, stderr)
	case "hunt":
		err = runHunt(rest, stdout, stderr)
	case "ingest":
		err = runIngest(rest, stdout, stderr)
	case "explain":
		err = runExplain(rest, stdout, stderr)
	case "report":
		err = runReport(rest, stdout, stderr)
	case "check":
		err = runCheck(rest, stdout, stderr)
	case "ci-report":
		err = runCIReport(rest, stdout, stderr)
	case "quarantine":
		err = runQuarantine(rest, stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "flakestat %s\n", Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "flakestat: unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		var ec exitCoder
		if asExitCoder(err, &ec) {
			fmt.Fprintf(stderr, "flakestat: %v\n", ec.err)
			return ec.code
		}
		fmt.Fprintf(stderr, "flakestat: %v\n", err)
		return 1
	}
	return 0
}

// exitCoder carries a specific process exit code out of a command.
type exitCoder struct {
	err  error
	code int
}

func (e exitCoder) Error() string { return e.err.Error() }

func asExitCoder(err error, target *exitCoder) bool {
	if ec, ok := err.(exitCoder); ok {
		*target = ec
		return true
	}
	return false
}

// scoringFlags registers the shared scoring knobs on a flag set.
func scoringFlags(fs *flag.FlagSet, cfg *score.Config) {
	d := score.Defaults()
	fs.IntVar(&cfg.MinRuns, "min-runs", d.MinRuns,
		"minimum scored runs before a test is classified")
	fs.Float64Var(&cfg.EWMAAlpha, "alpha", d.EWMAAlpha,
		"recency decay: higher weights recent runs more heavily")
	fs.Float64Var(&cfg.SameCommitWeight, "same-commit-weight", d.SameCommitWeight,
		"how much more a same-commit disagreement counts than a cross-commit one")
	fs.Float64Var(&cfg.FlakyThreshold, "threshold", d.FlakyThreshold,
		"score at or above which a test is called flaky")
	fs.Float64Var(&cfg.SuspectThreshold, "suspect-threshold", d.SuspectThreshold,
		"score at or above which a test is called suspect")
	fs.StringVar(&cfg.DefaultBranch, "default-branch", gitDefaultBranch(),
		"branch whose results are trusted; disagreement elsewhere counts for less")
	fs.Float64Var(&cfg.BranchWeight, "branch-weight", d.BranchWeight,
		"weight for disagreement seen off the default branch")
}

// setFlags reports which flags the user actually passed, so config values can
// fill in the rest without ever overriding an explicit choice.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

// mergeScoring layers config scoring values under the command-line flags.
func mergeScoring(dst *score.Config, src score.Config, set map[string]bool) {
	if !set["min-runs"] && src.MinRuns > 0 {
		dst.MinRuns = src.MinRuns
	}
	if !set["alpha"] && src.EWMAAlpha > 0 {
		dst.EWMAAlpha = src.EWMAAlpha
	}
	if !set["same-commit-weight"] && src.SameCommitWeight > 0 {
		dst.SameCommitWeight = src.SameCommitWeight
	}
	if !set["threshold"] && src.FlakyThreshold > 0 {
		dst.FlakyThreshold = src.FlakyThreshold
	}
	if !set["suspect-threshold"] && src.SuspectThreshold > 0 {
		dst.SuspectThreshold = src.SuspectThreshold
	}
	if !set["default-branch"] && src.DefaultBranch != "" {
		dst.DefaultBranch = src.DefaultBranch
	}
	if !set["branch-weight"] && src.BranchWeight > 0 {
		dst.BranchWeight = src.BranchWeight
	}
}

// loadConfigDefaults applies the project config to a command that only needs
// the state directory and scoring settings.
func loadConfigDefaults(fs *flag.FlagSet, cfg *score.Config, dir *string, stderr io.Writer) error {
	conf, err := config.Load(config.Find("."))
	if err != nil {
		return err
	}
	set := setFlags(fs)

	if !set["dir"] && conf.Dir != "" {
		*dir = conf.Dir
	}
	mergeScoring(cfg, conf.Scoring, set)
	return nil
}

// dimensionFlag collects repeated --dimension key=value flags.
type dimensionFlag []string

func (d *dimensionFlag) String() string { return strings.Join(*d, ",") }
func (d *dimensionFlag) Set(v string) error {
	*d = append(*d, v)
	return nil
}

// registerDimensionFlag makes --dimension available wherever observations are
// created. Offering it on only one command would leave users with metadata
// that silently differs between hunt and ingest.
func registerDimensionFlag(fs *flag.FlagSet, d *dimensionFlag) {
	fs.Var(d, "dimension",
		"attach `key=value` context to these observations; repeatable (e.g. runtime.version=3.13)")
}

// resolveDimensions layers context in precedence order: automatic detection,
// then recognized JUnit properties, then the user's explicit flags.
//
// hostIsExecutor must be true only when flakestat launched the tests itself.
// An ingest of a report produced elsewhere has no idea what ran it, and
// stamping it with this machine's platform would manufacture evidence that a
// later correlation would report as fact.
func resolveDimensions(explicit dimensionFlag, props map[string]string, hostIsExecutor bool) (dimension.Set, error) {
	user, err := dimension.Parse(explicit)
	if err != nil {
		return nil, err
	}

	var layers []dimension.Set
	if hostIsExecutor {
		layers = append(layers, dimension.Host())
	}
	layers = append(layers, dimension.DetectCI(), dimension.FromProperties(props), user)

	return dimension.Merge(layers...), nil
}

// gitDefaultBranch resolves the repository's default branch, falling back to
// the conventional names. An empty result simply disables branch weighting;
// transitions are never computed across branches either way.
func gitDefaultBranch() string {
	// The remote's HEAD is authoritative when the repo has an origin.
	if ref := gitOutput("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); ref != "" {
		if _, name, ok := strings.Cut(ref, "/"); ok && name != "" {
			return name
		}
		return ref
	}
	for _, name := range []string{"main", "master"} {
		if gitOutput("rev-parse", "--verify", "--quiet", name) != "" {
			return name
		}
	}
	return ""
}

// openStore opens the observation log, defaulting to .flakestat.
func openStore(dir string) (*store.Store, error) {
	return store.Open(dir)
}

// permute reorders args so every flag precedes the positional arguments.
//
// Go's flag package stops parsing at the first non-flag argument, so
// "ingest reports/*.xml --dir build" silently treats "--dir" and "build" as
// file patterns and ignores the flag entirely. That fails quietly and produces
// a confusing error about missing test cases, so the order is normalized
// first. Anything after a bare "--" is left untouched.
func permute(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			positional = append(positional, args[i:]...)
			break
		}

		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)

		// "--flag=value" already carries its value.
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}

		// A non-boolean flag consumes the next argument as its value.
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positional...)
}

// isBoolFlag reports whether a flag is used without a value.
func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

// rejectStrayFlags catches an unknown flag that survived parsing as a
// positional argument, rather than letting it be read as a file pattern.
func rejectStrayFlags(args []string) error {
	for _, a := range args {
		if len(a) > 1 && a[0] == '-' {
			return fmt.Errorf("unknown flag %q (run with -h to list the available flags)", a)
		}
	}
	return nil
}

// gitInfo returns the current commit and branch, or empty strings outside a
// git repository. Commit provenance sharpens scoring but is never required.
func gitInfo() (commit, branch string) {
	return gitOutput("rev-parse", "HEAD"), gitOutput("rev-parse", "--abbrev-ref", "HEAD")
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// warnAll prints non-fatal errors without failing the command.
func warnAll(w io.Writer, errs []error) {
	for _, err := range errs {
		fmt.Fprintf(w, "warning: %v\n", err)
	}
}
