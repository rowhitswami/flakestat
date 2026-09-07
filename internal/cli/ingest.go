package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/rowhitswami/flakestat/internal/dimension"
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
		noHost = fs.Bool("no-host", false,
			"never record this machine's platform, even in CI (use when aggregating reports produced elsewhere)")
	)
	var dims dimensionFlag
	registerDimensionFlag(fs, &dims)

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

	// Reports are read one file at a time rather than merged, because a case's
	// position within its own report is part of its execution identity. Merge
	// first and re-ingesting the same files renumbers everything.
	paths, errs := junit.Expand(patterns)
	warnAll(stderr, errs)

	type parsed struct {
		report *junit.Report
		digest string
		path   string
	}
	var (
		reports []parsed
		total   int
	)
	props := map[string]string{}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			warnAll(stderr, []error{err})
			continue
		}
		rep, err := junit.ParseBytes(data)
		if err != nil {
			warnAll(stderr, []error{fmt.Errorf("%s: %w", path, err)})
			continue
		}
		reports = append(reports, parsed{report: rep, digest: store.Digest(data), path: path})
		total += len(rep.Cases)
		for k, v := range rep.Properties {
			if _, seen := props[k]; !seen {
				props[k] = v
			}
		}
	}

	if total == 0 {
		return fmt.Errorf("no test cases found in %v", patterns)
	}

	// Being inside CI normally means this machine ran the tests. It does not
	// when a job downloads JUnit artifacts produced by other matrix jobs and
	// ingests them centrally: that aggregator's platform is not where anything
	// ran, and recording it would let analysis conclude the opposite of the
	// truth. --no-host disables the assumption; merging the NDJSON each job
	// produced avoids the situation altogether.
	inCI := dimension.InCI() && !*noHost
	dimensions, err := resolveDimensions(dims, props, inCI)
	if err != nil {
		return err
	}
	if !inCI && dimensions[dimension.OS] == "" {
		fmt.Fprintln(stderr,
			"note: no execution platform recorded. flakestat only assumes this machine ran the\n"+
				"      tests when it detects a CI provider. Pass --dimension os=... --dimension arch=...\n"+
				"      if you know where these results came from.")
	}

	// What is already recorded decides what is worth appending. Re-ingesting
	// the same artifact must not add evidence, because the tests did not run
	// again -- and to every downstream calculation a second copy is
	// indistinguishable from a second execution that happened to agree.
	known, errs := st.Keys()
	warnAll(stderr, errs)

	var (
		obs       []store.Observation
		duplicate int
	)
	for _, r := range reports {
		batch := store.FromCases(r.report.Cases, store.Meta{
			RunID:      *runID,
			Commit:     *commit,
			Branch:     *branch,
			Source:     store.SourceCI,
			Dimensions: dimensions,
			Scope: store.ExecutionScope{
				Context: dimensions,
				Report:  r.path,
				Digest:  r.digest,
			},
		})
		for _, o := range batch {
			if o.ExecKey != "" {
				if _, seen := known[o.ExecKey]; seen {
					duplicate++
					continue
				}
				known[o.ExecKey] = struct{}{}
			}
			obs = append(obs, o)
		}
	}

	if len(obs) > 0 {
		if err := st.Append(obs); err != nil {
			return err
		}
	}

	fmt.Fprintf(stdout, "Ingested %d test result(s) as run %s into %s\n", len(obs), *runID, st.Path())
	if duplicate > 0 {
		fmt.Fprintf(stdout,
			"Skipped %d result(s) already recorded; these executions were ingested before.\n", duplicate)
	}
	return nil
}
