# Contributing

Bug reports, reproductions and patches are all welcome. This document is short
because the project is small.

## Before you start

For anything beyond a bug fix, open an issue first. flakestat has a deliberately
narrow scope, and [DESIGN.md](DESIGN.md) §12 lists what is out of it on purpose.
Saving you an afternoon is the point of asking.

## Reporting a bug

The useful ones include:

- `flakestat version`, your OS, and the test runner
- The exact command
- A JUnit XML file small enough to attach, or the smallest one that reproduces it

If flakestat scored something wrong, `flakestat explain <test> --json` prints the
evidence it used, which is usually enough to see where the disagreement is.

## Development

```sh
git clone https://github.com/rowhitswami/flakestat && cd flakestat
go test ./...
go build -o flakestat ./cmd/flakestat
```

No dependencies, so no module downloads and no build tags. `go.mod` has no
`require` block and pull requests that add one need to argue for it: being a
single static binary with nothing to audit is a feature people choose this over
a hosted service for.

Before pushing:

```sh
gofmt -l .          # must print nothing
go vet ./...
go test -race ./...
```

## Changing the scoring

Scoring changes move numbers in everybody's existing history, so they need more
than a passing test suite:

- Say what the change does to a test that was previously `flaky`, `suspect`,
  `stable` and `consistently-failing`.
- Run `./scripts/reproduce-validation.sh`. The three ConduitIO tests must still
  read flaky before their fix and stable at it. CI enforces this.
- If it changes a default, [VALIDATION.md](VALIDATION.md) records why the
  current one was chosen. Argue with that, not with intuition.

The one rule that is not negotiable: **flakiness is inconsistency, not failure
rate.** A test that fails every time scores zero and is reported separately. If
a change makes an always-failing test look flaky, it is wrong regardless of what
else it improves.

## The site

`site/` generates <https://flakestat.com>. `python3 site/build.py out` renders
it, `python3 site/check.py out` must pass, and both run in CI. The pages at
`/validation/`, `/findings/`, `/design/` and `/changelog/` are generated from
the Markdown in the repository root, so edit those files rather than the HTML.

## Commits and pull requests

Explain why in the commit message, not just what. The diff already says what.

One logical change per pull request. If you found three things, three pull
requests get reviewed faster than one.

## Licence

By contributing you agree your work is released under the MIT licence, the same
as the rest of the project.
