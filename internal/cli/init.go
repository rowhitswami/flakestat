package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rowhitswami/flakestat/internal/config"
	"github.com/rowhitswami/flakestat/internal/store"
)

const initHelp = `Detect this project's test setup and write .flakestat.json.

Without a config file every invocation has to repeat the test command and the
report path, and the report path has to be written twice. With one, the whole
command is "flakestat hunt".

USAGE
  flakestat init [flags]

FLAGS
`

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, initHelp)
		fs.PrintDefaults()
	}

	var (
		force = fs.Bool("force", false, "overwrite an existing config")
		dir   = fs.String("C", ".", "project directory to inspect")
		pick  = fs.String("project", "", "skip detection and use this setup (e.g. pytest, jest, go)")
	)

	if err := fs.Parse(permute(fs, args)); err != nil {
		return err
	}

	path := *dir + "/" + config.FileName
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", path)
	}

	found := config.Detect(*dir)
	if len(found) == 0 {
		fmt.Fprintln(stderr, "Could not detect a test setup in this directory.")
		fmt.Fprintln(stderr, "flakestat works with anything that writes JUnit XML; configure it by hand:")
		fmt.Fprintln(stderr, "\n  "+config.FileName+":")
		fmt.Fprintln(stderr, `  {
    "version": 1,
    "command": ["your-test-command", "--junit-flag", "{junit}"],
    "junit": ".flakestat/reports/junit-{run}.xml",
    "runs": 20
  }`)
		return fmt.Errorf("no supported test setup detected")
	}

	chosen := found[0]
	if *pick != "" {
		var ok bool
		for _, p := range found {
			if strings.EqualFold(p.Name, *pick) {
				chosen, ok = p, true
				break
			}
		}
		if !ok {
			names := make([]string, len(found))
			for i, p := range found {
				names[i] = p.Name
			}
			return fmt.Errorf("no %q setup detected here (found: %s)", *pick, strings.Join(names, ", "))
		}
	}

	if len(found) > 1 {
		names := make([]string, 0, len(found)-1)
		for _, p := range found[1:] {
			names = append(names, p.Name)
		}
		fmt.Fprintf(stdout, "Detected several test setups; using %s.\n", chosen.Name)
		fmt.Fprintf(stdout, "Also present: %s (re-run with --project <name> to pick one).\n\n",
			strings.Join(names, ", "))
	} else {
		fmt.Fprintf(stdout, "Detected %s.\n\n", chosen.Name)
	}

	cfg := &config.File{
		Command: chosen.Args,
		JUnit:   chosen.JUnit,
		Runs:    20,
		Dir:     store.DefaultDir,
	}
	cfg.Scoring.DefaultBranch = gitDefaultBranch()

	if err := cfg.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)

	if chosen.Setup != "" {
		fmt.Fprintf(stdout, "\nThis setup needs one more thing first:\n  %s\n", chosen.Setup)
	}
	if chosen.Notes != "" {
		fmt.Fprintf(stdout, "\nNote: %s\n", chosen.Notes)
	}

	fmt.Fprintf(stdout, `
Next:
  flakestat hunt          # run the suite %d times and report what disagreed
`, cfg.Runs)

	// Hunting one suspect test is dramatically cheaper than hunting a whole
	// suite, and "is THIS test flaky?" is usually the real question.
	if chosen.Filter != "" {
		fmt.Fprintf(stdout, `
Chasing one specific test? Filter it and use more runs -- far faster, and far
more conclusive than hunting the whole suite:
  flakestat hunt --runs 100 -- %s %s
`, strings.Join(chosen.Args, " "), chosen.Filter)
	}

	fmt.Fprintf(stdout, `
For an ongoing signal, record the CI runs you already pay for -- this costs no
extra compute and accumulates across commits and branches:
  flakestat ingest '%s'
  flakestat check --update-baseline   # accept today's flakiness
  flakestat check --fail-on-new       # fail only on what is new
`, chosen.JUnit)

	return nil
}
