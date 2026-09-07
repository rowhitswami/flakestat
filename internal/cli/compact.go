package cli

import (
	"flag"
	"fmt"
	"io"

	"github.com/rowhitswami/flakestat/internal/store"
)

const compactHelp = `Remove observations that record an execution already in the history.

The log is append-only and merged with cat, so the same execution can reach it
more than once -- an artifact collected twice, a job whose aggregation step was
re-run. Reading already ignores those copies; compact removes them from the
file, which matters when the file itself is the durable record.

Duplication is not harmless. A copy carries its original's timestamp and always
agrees with itself, so uncounted duplicates make a flaky test look stable.

Nothing else is discarded. Observations recorded before execution identity
existed, and results from 'hunt' -- which watched every execution happen and so
cannot hold duplicates -- are always kept.

USAGE
  flakestat compact [flags]

FLAGS
`

func runCompact(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("compact", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, compactHelp)
		fs.PrintDefaults()
	}

	var (
		dir    = fs.String("dir", store.DefaultDir, "state directory")
		dryRun = fs.Bool("dry-run", false, "report what would be removed without changing the log")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := rejectStrayFlags(fs.Args()); err != nil {
		return err
	}

	st, err := openStore(*dir)
	if err != nil {
		return err
	}

	raw, errs := st.Raw()
	warnAll(stderr, errs)

	kept := store.Dedup(raw)
	removed := len(raw) - len(kept)

	if removed == 0 {
		fmt.Fprintf(stdout, "Nothing to remove; %d observation(s) in %s\n", len(raw), st.Path())
		return nil
	}
	if *dryRun {
		fmt.Fprintf(stdout, "Would remove %d duplicate observation(s), leaving %d in %s\n",
			removed, len(kept), st.Path())
		return nil
	}

	// Corrupt lines are dropped by reading, so rewriting would delete evidence
	// that a human might still be able to recover. Refuse rather than decide
	// that for them.
	if len(errs) > 0 {
		return fmt.Errorf("refusing to rewrite a log with %d unreadable line(s); "+
			"fix or remove them first, or run with --dry-run", len(errs))
	}

	if err := st.Rewrite(kept); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed %d duplicate observation(s), leaving %d in %s\n",
		removed, len(kept), st.Path())
	return nil
}
