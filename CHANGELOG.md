# Changelog

Notable changes to flakestat. Versions follow [semantic versioning](https://semver.org),
with the usual caveat that 0.x makes no compatibility promise.

## [0.2.1] - 2026-09-08

A pre-launch audit. One of these silently discarded most of your evidence,
which for a tool that reports statistics is the worst kind of bug it could
have had.

### Fixed

**`**` in an ingest pattern matched almost nothing.** `flakestat ingest
'reports/**/*.xml'` is the command in the README, in the npm and PyPI readmes,
in `action.yml`, and in every CI recipe in the documentation. Go's
`filepath.Glob` does not implement `**`; it reads the two stars as one, so the
pattern meant "reports/*exactly one directory*/\*.xml". Reports written straight
into `reports/`, which is what `hunt --junit 'reports/junit-{run}.xml'` writes,
never matched. Against eight flat reports plus one in a shard directory, 0.2.0
ingested 2 test results and reported confident verdicts on them; 0.2.1 ingests
16. There was no error either time. `**` now means this directory and below.
Patterns without `**` are unchanged.

**`go install` produced a binary that reported `dev`.** The version is stamped
by GoReleaser through ldflags, which `go install` does not apply, so anyone
using the Go path could not say which build they had. It now reads the module
version from the build info, which for that path is the tag you asked for. A
working-tree build reports its commit rather than a pseudo-version naming a
release that was never cut.

### Security

**The installers verified checksums and then installed anyway when they could
not.** A missing `checksums.txt`, or an archive not listed in it, printed a
warning and continued. Anyone able to block a single request had silently
downgraded the install to no verification, on the path whose front door is
`curl ... | sh`. All three now refuse. `FLAKESTAT_SKIP_CHECKSUM=1` opts out and
has to be set deliberately.

**Release binaries now carry build provenance.** Verify one with
`gh attestation verify <archive> -R rowhitswami/flakestat`. npm already
published with `--provenance` and PyPI over OIDC; the GitHub release artifacts
had only an unsigned `checksums.txt`.

**Minimum Go raised to 1.25.13.** `go.mod` declared 1.25.4, which is the floor
for anyone building with `go install`, and GO-2026-6088 in `encoding/xml` is
reachable from the parser below 1.25.13. Released binaries were never affected;
they are built with a newer toolchain, and flakestat's own parser depth guard
rejects the nesting bomb regardless.

Also: a security policy with the threat model stated, a contributing guide,
issue templates, and private vulnerability reporting enabled on the repository.

## [0.2.0] - 2026-09-07

Everything the documentation describes now exists in the binary. v0.1.1 shipped
six commands; this ships ten, plus the analysis that makes the verdicts worth
reading.

### Added

**`flakestat explain <test>`** shows the full evidence behind one verdict: outcome
history as a strip, how many disagreements happened on identical code, which
commits and branches it was seen on, and where its failures concentrate.
`--json` emits the same evidence for tooling.

**Confidence on every verdict**, either `low`, `medium` or `high`, derived from how
much evidence exists. A score of 0.62 from three runs and the same score from
three hundred are different claims, and a small sample can never reach `high`.
Classification uses a lower bound on the score rather than the score itself, so
a verdict needs evidence rather than a lucky flip.

**Dimensions and association analysis**. `--dimension key=value`, repeatable,
records the platform, runtime and CI context an observation was made in.
flakestat then reports where failures concentrate:

```
Where the failures concentrate
  os=windows
    failures here:  12 / 12 (100.0%)
    elsewhere:      0 / 24 (0.0%)
    difference:     +100.0 pp
```

Findings must clear an evidence floor, an effect-size floor and a
Benjamini–Hochberg correction, because testing several dimensions will otherwise
turn up something significant by chance. Associations never influence the score,
and the wording stays correlational: failures *cluster* on Windows, not Windows
*causes* them. Where dimensions vary together, it says the observations cannot
tell which one matters.

Only a whitelist of known CI variables is read, and only JUnit properties
flakestat recognizes. The process environment is never scraped, so secrets
cannot reach your history.

**Execution identity**. Every observation carries the identity of the execution
it describes, so an artifact uploaded twice, a re-run aggregation step or a shard
collected by two jobs counts once, while a genuine retry still counts.

This mattered more than it sounds. A duplicate carries its original's timestamp,
sorts beside it and always agrees with itself, so uncounted duplicates make a
flaky test look **stable**. Measured on a test failing 4 of 12 runs, ingesting
the same reports twice moved the score from 0.64 to 0.30. The error ran towards
false negatives.

**`flakestat compact`** removes duplicate executions from the log file itself,
which matters when the file is the record you keep. Refuses to rewrite a log
containing unreadable lines, since reading skips those and rewriting would
discard evidence you might still recover.

**`flakestat ci-report`** renders job summaries and pull request comments. The
GitHub Action gained `comment`, `annotations` and `max-rows` inputs; a re-run
updates the same comment rather than adding another.

**`--no-host`** on `ingest`, for jobs that aggregate artifacts produced
elsewhere. Without it the aggregator's platform is stamped onto results from
every other machine.

### Fixed

**Outcomes are only compared when they are comparable.** A test that always
passes on Linux and always fails on Windows is deterministic, but read as one
chronological series it looked like constant disagreement, and it scored 0.45 with
an explanation asserting direct evidence of nondeterminism. Transitions are now
counted within one execution context: branch, os, arch and runtime held
constant.

**The history strip agrees with the verdict.** It grouped by branch while
scoring grouped by execution context, so it printed "flip on identical code" at
every platform boundary, at precisely the points the verdict had ruled out as
incomparable.

**Execution identity uses the whole recorded context.** GitHub reports the same
`GITHUB_JOB` for every leg of a matrix, so three platforms can ingest under one
provider, run, job and attempt. Keying on the CI fields alone would have merged
them and silently discarded two platforms' evidence.

### Validation

Validated against three external projects under a contract committed before any
experiment ran, and scored with a single frozen build. Full report in
[VALIDATION.md](VALIDATION.md).

The load-bearing result: at the commit before ConduitIO's own deflaking fix,
three of the four tests their issue named scored `flaky` at 0.67 with high
confidence; at the fix itself, all four were `stable` at 0.00, under an identical
protocol. Their contributors documented the defect, named the tests and wrote the
repair. flakestat was handed observations from both sides of a commit it had no
part in and separated them.

Also recorded there: the protocol arm that found nothing, and a contaminated run
that was discarded rather than reported.

## [0.1.1] - 2026-09-06

Fixed the Python wrapper reporting `0.0.0` regardless of the installed version,
which made it build a download URL for a release that does not exist. The
release workflow now verifies the built wheel reports the tagged version before
anything is published.

## [0.1.0] - 2026-09-06

First release. `init`, `hunt`, `ingest`, `report`, `check` and `quarantine`,
distributed via Homebrew, npm, PyPI, a container image, an install script and a
GitHub Action.

Yanked from PyPI: the wrapper shipped with a hardcoded `0.0.0` version and could
not download its binary. Superseded by 0.1.1.

[0.2.1]: https://github.com/rowhitswami/flakestat/releases/tag/v0.2.1
[0.2.0]: https://github.com/rowhitswami/flakestat/releases/tag/v0.2.0
[0.1.1]: https://github.com/rowhitswami/flakestat/releases/tag/v0.1.1
[0.1.0]: https://github.com/rowhitswami/flakestat/releases/tag/v0.1.0
