package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/rowhitswami/flakestat/internal/junit"
	"github.com/rowhitswami/flakestat/internal/store"
)

const ingestHelp = `Load JUnit XML reports from CI into the observation history.

Ingested runs carry their commit SHA, which is what lets scoring tell genuine
flakiness (same code, different result) from a regression that was later fixed.

USAGE
  flakestat ingest [flags] <file-or-glob>...

EXAMPLE
  flakestat ingest 'reports/**/*.xml'

FLAGS
`

func runIngest(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, ingestHelp)
		fs.PrintDefaults()
	}

	var (
		dir    = fs.String("dir", store.DefaultDir, "state directory")
		commit = fs.String("commit", "", "commit SHA for these results (default: current git HEAD)")
		branch = fs.String("branch", "", "branch name (default: current git branch)")
		runID  = fs.String("run-id", "", "identifier for this CI run (default: generated)")
	)

	// Flags may follow the file patterns; normalize before parsing.
	if err := fs.Parse(permute(fs, args)); err != nil {
		return err
	}

	patterns := fs.Args()
	if err := rejectStrayFlags(patterns); err != nil {
		return err
	}
	if len(patterns) == 0 {
		fs.Usage()
		return fmt.Errorf("no report files given")
	}

	gitCommit, gitBranch := gitInfo()
	if *commit == "" {
		*commit = gitCommit
	}
	if *branch == "" {
		*branch = gitBranch
	}
	if *runID == "" {
		*runID = store.NewRunID()
	}

	if *commit == "" {
		fmt.Fprintln(stderr,
			"warning: no commit SHA recorded; pass --commit to sharpen scoring.\n"+
				"         Without it, same-commit disagreement cannot be distinguished from a regression.")
	}

	st, err := openStore(*dir)
	if err != nil {
		return err
	}

	var cases []junit.Case
	for _, p := range patterns {
		rep, errs := junit.ParseGlob(p)
		warnAll(stderr, errs)
		if rep != nil {
			cases = append(cases, rep.Cases...)
		}
	}

	if len(cases) == 0 {
		return fmt.Errorf("no test cases found in %v", patterns)
	}

	obs := store.FromCases(cases, store.Meta{
		RunID:  *runID,
		Commit: *commit,
		Branch: *branch,
		Source: store.SourceCI,
	})
	if err := st.Append(obs); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Ingested %d test result(s) as run %s into %s\n", len(obs), *runID, st.Path())
	return nil
}
