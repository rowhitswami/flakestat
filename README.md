# flakestat

**Find flaky tests in any language** — pytest, Jest, go test, JUnit, RSpec,
PHPUnit, and anything else that writes a JUnit XML report. One static binary.
No SaaS, no account, no test data leaving your machine.

```
VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST
flaky                  0.62  high    20       14/6  test_demo::test_flaky_race
consistently-failing   0.00  high    20       0/20  test_demo::test_broken

1 flaky, 0 suspect, 1 consistently failing, 1 stable, 1 unscored (of 4 tests)
```

Every verdict carries its own confidence, because a score of `0.62` from three
runs and the same score from three hundred are very different claims. A small
sample can never reach `high`, no matter how flaky the test looks.

## Is this what you're looking for?

- **"Which of my tests are actually flaky?"** — flakestat ranks them worst
  first with a score, so you fix the top three instead of guessing.
- **"My CI fails randomly and I can't tell which test."** — `flakestat hunt`
  reruns your suite N times on unchanged code and shows exactly what disagreed.
- **"How do I detect flaky tests in CI?"** — `flakestat ingest` records every
  run; `flakestat check` fails the build only when flakiness gets *worse*.
- **"How do I find flaky tests in pytest / Jest / go test?"** — all of them
  already emit JUnit XML, which is all flakestat needs. No plugin to install,
  no framework lock-in.
- **"Is there a free, self-hosted alternative to BuildPulse or Trunk?"** — this
  one. It runs locally and your test results never leave your machine.
- **"How do I stop flaky tests blocking merges today?"** — `flakestat
  quarantine` emits a skip list your runner accepts, so you're unblocked while
  you work the list down.

## Why

A flaky test passes and fails nondeterministically on the same code. The good
tooling for this is all hosted — Trunk, BuildPulse, Mergify, Qualflare — and the
open-source side is a scatter of per-language plugins. flakestat is a local
binary that works with whatever you already use.

Two ideas make it accurate:

**Flakiness is inconsistency, not failure.** A test that fails 100% of the time
scores 1.0 on failure rate but isn't flaky — it's broken. flakestat scores
*state transitions* (pass→fail→pass), so an always-failing test correctly scores
zero and is reported separately as `consistently-failing` rather than being
mixed in with real flakes or silently hidden.

**Same-commit disagreement is proof; cross-commit is a hint.** A test that
failed on commit A and passed on commit B may just be a regression someone
fixed. flakestat records the commit SHA with every observation and discounts
cross-commit disagreement to a third of the weight.

**Branches are scored separately.** A test that passes on `main`, fails on an
in-progress feature branch, then passes on `main` again is not flaky — but read
as one chronological series it looks like two flips. flakestat only compares
observations *within* a branch, so that phantom signal can't arise, and
disagreement off the default branch counts for less because failures there are
often just unfinished work.

## Install

Pick whichever fits the project you're in — it's the same binary either way.

```bash
# Homebrew
brew install rowhitswami/tap/flakestat

# npm  (JS/TS projects)
npm install --save-dev flakestat     # or: npx flakestat

# pip  (Python projects)
pip install flakestat

# Go
go install github.com/rowhitswami/flakestat/cmd/flakestat@latest

# Script  (CI, or no package manager)
curl -sSfL https://raw.githubusercontent.com/rowhitswami/flakestat/main/scripts/install.sh | sh -s -- -b /usr/local/bin

# Docker
docker run --rm -v "$PWD:/workspace" ghcr.io/rowhitswami/flakestat report
```

Or grab a binary straight from
[Releases](https://github.com/rowhitswami/flakestat/releases) — macOS, Linux and
Windows on x86_64 and arm64.

## Which command do you want?

| Your situation | Use | Cost |
|---|---|---|
| "Is *this one test* flaky?" | `hunt` with your runner's filter | seconds |
| "Which tests are flaky, over time?" | `ingest` in CI | **free** — reuses runs you already pay for |
| "Don't let it get worse" | `check --fail-on-new` | free |
| "Unblock the pipeline now" | `quarantine` | free |
| "Why did it call this test flaky?" | `explain <test>` | free |

A note on cost, because it decides which of these you actually want. `hunt`
costs `suite runtime x runs`, and detecting a test that fails 10% of the time
needs 50+ runs. On a fast suite that is a
minute. On `psf/requests` — 632 tests, ~70s per run — it is an hour.

Flakiness concentrates in slow integration suites, which is exactly where
hunting a whole suite is least affordable. So:

- **`hunt` one filtered test** to get a definitive answer fast.
- **`ingest` in CI** for the whole-suite picture, at zero extra compute.

## Quickstart

```bash
flakestat init    # detects pytest / Jest / vitest / go / cargo / maven / gradle / rspec / phpunit
flakestat hunt    # runs your suite 20 times and reports what disagreed
```

`init` writes a `.flakestat.json` describing your test command and where its
JUnit report lands, so `hunt` needs no arguments. Flags always override it.

Without a config file, spell it out:

```bash
flakestat hunt --runs 20 \
  --junit 'reports/junit-{run}.xml' \
  -- pytest -q --junitxml='{junit}'
```

`{junit}` expands to the resolved report path, so it's written once rather than
kept in sync in two places.

`{run}` is replaced with the run number in both `--junit` and your test command,
so parallel runs write to separate files. It's also exported as
`$FLAKESTAT_RUN`.

Within a burst the code never changes, so any test that both passes and fails is
flaky by definition — no history or threshold needed.

### Chasing one suspect test

The cheapest, most conclusive thing flakestat does. Pass your runner's own
filter and use plenty of runs:

```bash
flakestat hunt --runs 100 -- pytest -k test_login --junitxml='{junit}'
flakestat hunt --runs 100 -- gotestsum --junitfile '{junit}' -- -run '^TestFoo$' ./...
```

100 runs of one test usually costs less than 5 runs of the suite, and at that
sample size the answer is definitive rather than suggestive.

### If it finds nothing

`hunt` tells you what it could have found:

```
No flaky tests detected across 1,995 test(s).
With 12 run(s), this reliably finds tests that fail about a quarter of the time or more.
Rarer flakes need more evidence -- try --runs 50, or record CI
runs over time with "flakestat ingest", which costs nothing extra.
```

A clean result at 12 runs is not the same as a clean suite, and flakestat says
so rather than letting you assume otherwise.

### A warning about `--parallel`

`--parallel N` runs N copies of your test command **in the same working
directory**. A suite that writes to fixed paths, binds a fixed port, or shares a
database will collide with itself and look flaky when it isn't.

This is real, not theoretical: running flakestat against `spf13/cobra` with
`--parallel 4` reported `TestDeadcodeElimination` as flaky at 0.60. It isn't —
the test builds a binary at a fixed path, and concurrent copies deleted each
other's build. Sequentially, cobra is completely clean.

So flakestat now **verifies its own findings**. Candidates found under
`--parallel` are automatically re-run sequentially, and anything that doesn't
reproduce is demoted from `flaky` to `suspect` with an explanation:

```
1 candidate(s) did not reproduce in 5 sequential run(s) and were demoted to suspect:
  github.com/spf13/cobra::TestDeadcodeElimination
These most likely collided with themselves under --parallel rather than being flaky.
```

Sequential (the default) is always trustworthy. Use `--parallel` to hunt faster,
and let verification sort out the difference — or `--verify 0` to opt out.

## Works with your test runner

flakestat reads **JUnit XML**, which every major framework already emits:

| Ecosystem | Flag |
|---|---|
| Python | `pytest --junitxml=reports/junit-{run}.xml` |
| JS/TS | `jest --reporters=jest-junit` · `vitest --reporter=junit` |
| Go | `gotestsum --junitfile reports/junit-{run}.xml` |
| Rust | `cargo nextest run --profile ci` |
| Java | Surefire / Gradle, native |
| Ruby | `rspec --format RspecJunitFormatter --out reports/junit-{run}.xml` |
| PHP | `phpunit --log-junit reports/junit-{run}.xml` |
| .NET | `dotnet test --logger junit` |

No JUnit XML? flakestat falls back to exit codes and reports suite-level
flakiness — less precise, but it still works.

## Tracking flakiness over time

A burst proves flakiness exists. Scoring history *measures* it, and catches
environment-dependent flakes a local burst never will.

```bash
# In CI, after your tests run:
flakestat ingest 'reports/**/*.xml'

# Any time:
flakestat report --top 20
```

History lives in `.flakestat/runs.ndjson` — one JSON object per line. Commit it
for shared team history, or keep it as a CI artifact. Because it's append-only
NDJSON, results from parallel CI shards concatenate with no merge step.

Re-ingesting the same reports is safe. Each observation carries the identity of
the execution it describes, so an artifact uploaded twice, a job re-run, or a
shard collected by two aggregators is counted once — while a genuine retry,
which really did run the tests again, still counts. Duplication is not
harmless: a copy always agrees with itself, so uncounted duplicates make a
flaky test look stable.

Copies are ignored on read, so nothing is required of you. `flakestat compact`
removes them from the file as well, which is worth doing when the file itself
is the record you keep.

### Where to keep the history

For a durable, shared history, a **dedicated branch** works well and is what
flakestat uses for itself:

```text
matrix jobs → per-job NDJSON artifacts → aggregate job
  → fetch history branch → merge + compact → analyze
  → commit back to the history branch
```

It keeps telemetry commits out of your development history, avoids a bot commit
retriggering the same workflow, and gives one place to serialize concurrent
writers:

```yaml
concurrency:
  group: flakestat-history-writer
  cancel-in-progress: false
```

A CI cache is the wrong home for it. Cache eviction should cost you time, not
statistical history. See [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
for the full arrangement, including re-merging a push that lost a race.

### Recording where tests ran

Pass `--dimension key=value` (repeatable) to record the context a run happened
in. flakestat then reports where failures concentrate:

```bash
flakestat ingest 'reports/**/*.xml' \
  --dimension os=windows \
  --dimension runtime.version=3.13 \
  --dimension database=postgres-17
```

```
Where the failures concentrate

  os=windows
    failures here:  12 / 12 (100.0%)
    elsewhere:      0 / 24 (0.0%)
    difference:     +100.0 pp
```

Inside a recognized CI provider, `os`, `arch` and the run/job identity are
detected automatically. Two rules keep that honest:

- Only a whitelist of known CI variables is ever read, and only JUnit
  `<property>` names flakestat recognizes. The process environment is never
  scraped, so secrets and build ids cannot end up in your history.
- Host details are recorded only when flakestat ran the tests. If one job
  downloads other jobs' artifacts and ingests them centrally, pass `--no-host`
  — otherwise the aggregator's platform gets stamped onto results from
  everywhere else. Merging each job's NDJSON with `cat` avoids the problem
  entirely.

Associations are correlations, never causes. flakestat says failures *cluster*
on Windows; it will not claim Windows is why.

### Gating CI without turning the build permanently red

Most repos already have flaky tests when they adopt a tool like this. Failing on
*all* of them means a red build on day one, and the gate gets deleted by day
three. So `check` is a **ratchet** instead: accept today's flakiness, then fail
only on what's new.

```bash
flakestat check --update-baseline   # once; commit .flakestat/baseline.json
flakestat check --fail-on-new       # in CI from then on
```

```
NEWLY FLAKY (1)
  0.60  test_demo::test_newly_flaky

1 new, 0 regressed, 1 accepted, 0 fixed
```

Exit codes: `0` nothing new, `1` newly flaky (`--fail-on-new`), `2` an accepted
test measurably worsened (`--fail-on-regression`). Regressions are judged
against `--regression-delta` (default `0.10`) so ordinary scoring jitter doesn't
fail builds. Tests that stop being flaky are reported as `FIXED`, which is your
cue to re-run `--update-baseline` and tighten the ratchet.

`report --format markdown` produces output suited to a PR comment;
`--format json` is for scripting.

## Unblocking the pipeline

Knowing which tests are flaky doesn't help if you're still blocked by them.
`quarantine` emits a skip list **in the format your runner actually accepts**:

```bash
flakestat quarantine --format pytest -o quarantine.txt
pytest $(grep -v '^#' quarantine.txt | sed 's/^/--deselect /')
```

```bash
flakestat quarantine --format go -o skip.txt
go test -skip "$(grep -v '^#' skip.txt)" ./...
```

| Format | Output |
|---|---|
| `pytest` | node IDs for `--deselect` |
| `go` | an escaped regex for `go test -skip` |
| `jest` | `testPathIgnorePatterns` (file-level — jest can't deselect single tests) |
| `yaml` / `json` | structured, for your own tooling |

Consistently failing tests are **excluded by default**. They're broken rather
than flaky, and quietly skipping them would hide a real defect — pass
`--include-broken` if you really want them.

### GitHub Actions

```yaml
- name: Run tests
  run: pytest --junitxml=reports/junit.xml
  continue-on-error: true

- uses: rowhitswami/flakestat@v1
  with:
    args: ingest 'reports/**/*.xml'
    comment: true       # post/update a PR comment
    annotations: true   # annotate new flakes in the diff
```

That records the run, writes a report to the job summary, and posts it as a
pull request comment. A re-run **updates the same comment** rather than adding
another one.

Outputs: `flaky-count`, `new-flaky-count`, `report-file`.

| Input | Default | Purpose |
|---|---|---|
| `args` | — | Arguments passed to flakestat |
| `comment` | `false` | Post/update a PR comment (needs `pull-requests: write`) |
| `annotations` | `false` | Annotate new and regressed tests in the diff |
| `summary` | `true` | Append the report to the job summary |
| `max-rows` | `20` | Cap the highlight table on large suites |
| `version` | latest | Release tag to install |
| `install-only` | `false` | Put the binary on PATH without running it |
| `working-directory` | `.` | Directory to run in |

#### The intended CI sequence

Two commands, in this order, because they answer different questions:

```bash
flakestat ingest 'reports/**/*.xml'   # record what happened
flakestat check --fail-on-new         # should this build pass?
flakestat ci-report --step-summary    # present the result
```

**`check` decides. `ci-report` reports.** `ci-report` always exits `0`, on
purpose: if it shared an exit code with the gate, a failing build would suppress
the very report explaining why it failed. Run `check` for the exit code, and
`ci-report` with `if: always()` so the report is published either way.

#### Where it degrades, and how

Reporting is best-effort by design; none of these break your build or the
analysis:

| Situation | Behaviour |
|---|---|
| Not running in GitHub Actions | `--step-summary` is a no-op and says so |
| No `pull-requests: write` permission | Comment is skipped; report stays in the job summary |
| Pull request from a fork | Token is read-only, so commenting is skipped with a note |
| Not a pull request event | Comment step is skipped entirely |
| Test framework reports no source file | No annotation, but the test still appears in the summary and comment |

The last one matters more than it sounds: JUnit XML does not guarantee source
locations, and several popular runners emit none. Annotations are an
enhancement, never a prerequisite — flakestat will not invent a file or line to
produce one.

## Working out why a test is flaky

A score is not an argument. `explain` shows the evidence behind a verdict:

```bash
flakestat explain test_checkout_timeout
```

```
checkout::test_checkout_timeout

  Classification: flaky
  Score:          0.41
  Confidence:     high  (lower bound 0.352 over 41 transition(s))

  Observations:   42
  Passed:         23
  Failed:         19
  Failure rate:   45.2%

  Transitions:               41
  Same-commit disagreements: 17

  History  (last 20 of 42, oldest first)

  F P F F P P F P P P F P F F F F F P P P
  ! ! !   !   ! !     ! ! !         !

  P pass   F fail   - skip   ^ flip   ! flip on identical code

  Why flaky?

  The outcome changed 17 time(s) across 41 comparison(s) of the same test.
  Even the cautious lower bound on that rate (0.352) clears the 0.10 flaky
  threshold, so this is not an artefact of a small sample. 17 of those
  disagreements happened on identical code, which is direct evidence of
  nondeterminism rather than an inference.
```

The reasoning is generated from the observations, not from a stock description
of the class — a consistently failing test explains why it is *broken* rather
than flaky, and an unclassified one explains exactly what is missing.

`--json` emits the same evidence for tooling, and `--history N` widens the
strip.

## How scoring works

For a test's observations, ordered oldest first:

```
score = sum(r_i * e_i * b_i * t_i) / sum(r_i)

t_i = 1 if observation i and i+1 disagree, else 0
r_i = (1-alpha)^age        recency weight, aged in COMMITS, not runs
e_i = evidence discount    1.0 same commit, 1/same-commit-weight otherwise
b_i = branch weight        1.0 default branch, branch-weight otherwise
```

Recency is aged in *commits* rather than individual runs. That matters: aging
per run capped the effective sample size at about `1/alpha` transitions, so 100
runs carried no more evidence than 10. Every run of one burst shares a commit,
so a burst now uses all of its data.

A test is called flaky only when a **confidence lower bound** on the score
clears the threshold, not the point estimate alone. Otherwise one unlucky flip
in a short run produces a confident-looking verdict, and collecting more data
could make the answer worse.

| Score | Verdict |
|---|---|
| `< 0.05` | stable |
| `0.05 – 0.10` | suspect |
| `> 0.10` | flaky |

For a test failing randomly with probability `p`, consecutive runs disagree with
probability `2p(1-p)`, so the `0.10` threshold corresponds to roughly `p ≥ 5%`.

Measured against suites with known flake rates: tests failing 25% of the time or
more are caught essentially always; tests failing 10% of the time are caught
about 80% of the time at 50+ runs; below 5% is unreliable. Stable and
permanently broken tests were never once misreported as flaky.

Skipped tests are excluded — they carry no pass/fail signal and would otherwise
manufacture phantom transitions. Below `--min-runs` (default 5) a test is
reported as `insufficient-data` rather than guessed at.

One exception: Surefire's `<flakyFailure>` means a test failed and then passed
on rerun *within one run*. That's proof, not inference, so it's marked flaky
immediately regardless of history.

Tuning: `--min-runs`, `--alpha`, `--same-commit-weight`, `--threshold`,
`--suspect-threshold`.

## Commands

| Command | Purpose |
|---|---|
| `init` | Detect this project's test setup and write `.flakestat.json` |
| `hunt` | Run a test command N times and detect disagreement |
| `ingest` | Load JUnit XML from CI into the history |
| `compact` | Drop observations that duplicate an execution already recorded |
| `report` | Score recorded history and print a report |
| `explain` | Show why one test received its verdict |
| `check` | Fail CI when flakiness gets worse, not when it exists |
| `quarantine` | Emit a skip list your test runner accepts |
| `ci-report` | Render a CI-facing report for job summaries and PR comments |

Run `flakestat <command> -h` for flags.

## How it compares

**vs. hosted platforms** — Trunk, BuildPulse, Mergify and Qualflare are more
featureful, with dashboards, org-wide analytics and support. flakestat is free,
runs locally, and never transmits your test data. If you can't send test results
to a vendor, or don't want a per-seat bill to find out which tests are flaky,
that's the trade this makes.

**vs. retry plugins** — `pytest-rerunfailures`, `jest.retryTimes` and Maven
Surefire reruns retry a test until it goes green. That unblocks the build but
*hides* the problem: the flaky test stays flaky forever, and nobody ever sees a
list of them. flakestat measures which tests are unreliable and by how much, so
they can be fixed. The two compose well — keep retrying to stay moving, and use
flakestat to work the list down.

**vs. single-framework tools** — `pytest-flakefinder` and similar cover one
ecosystem. Because flakestat reads JUnit XML, one tool and one history cover
every suite in a polyglot repo.

**vs. doing nothing** — the common outcome. Everyone agrees the suite is flaky;
nobody can name the ten worst offenders, because that needs memory across
hundreds of CI runs. So it's tolerated for years. Naming and ranking them is the
whole point.

## Contributing

JUnit XML has no official schema and every framework's dialect differs slightly.
If flakestat mis-parses your framework's output, that's a bug worth reporting —
**drop the XML file into `testdata/junit/` and open a PR**. The parser is tested
directly against that corpus, so a new dialect file is the most useful
contribution you can make.

```bash
go test ./...
```

## License

MIT
