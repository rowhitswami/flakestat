# flakestat — Design

A language-agnostic flaky test detector. Single static Go binary, no SaaS, no account,
no data leaving your machine.

---

## 1. Problem

A flaky test passes and fails nondeterministically **on the same code**. Teams need to:

1. **Find** them — which tests are unreliable?
2. **Quantify** them — how unreliable, and is it getting worse?
3. **Quarantine** them — stop them blocking merges without deleting coverage.
4. **Track** them — did that fix actually work?

Today the good tooling is all hosted (Trunk, BuildPulse, Mergify, Qualflare). The
open-source side is fragmented per-language plugins (`pytest-flakefinder`,
`pytest-rerunfailures`, Maven/Gradle retry) and dormant research code. There is no
polished, universal, local binary. That is the gap.

## 2. Design principles

1. **Measure, don't guess.** Rerunning a test until it fails proves flakiness exists;
   it doesn't quantify it. Scores come from pass/fail records across many runs.
2. **Language-agnostic via JUnit XML.** Don't integrate with N frameworks. Integrate
   with the one format they all already emit.
3. **Local-first.** History is a file in your repo, not a vendor's database.
4. **Degrade gracefully.** No JUnit XML? Fall back to exit-code-only suite-level
   flakiness rather than failing.
5. **Single static binary.** No cgo, no runtime, drops into any CI image.

## 3. Why JUnit XML

It's the universal interchange format. Every major framework emits it:

| Ecosystem | How |
|---|---|
| Python | `pytest --junitxml=out.xml` |
| JS/TS | `jest-junit`, `vitest --reporter=junit` |
| Go | `gotestsum --junitfile out.xml` |
| Rust | `cargo nextest run --profile ci` |
| Java | Surefire/Gradle native |
| Ruby | `rspec_junit_formatter` |
| PHP | `phpunit --log-junit` |
| .NET | `dotnet test --logger junit` |

**Critical caveat: JUnit XML is not a real standard.** There is no official schema, and
vendor dialects differ. The parser must be deliberately tolerant of:

- `<testsuites>` root wrapper being present *or* absent (pytest omits it sometimes)
- Nested `<testsuite>` elements
- `classname` present, empty, or duplicated into `name`
- `<failure>` vs `<error>` vs `<skipped>` vs bare pass
- Surefire's `<flakyFailure>` / `<flakyError>` rerun markers — these are *already*
  flakiness signals and should be ingested as such
- Missing `time`, `file`, `line` attributes
- Invalid XML characters in failure messages (common with binary output in stack traces)

This parser is the single highest-value component in the project. It's also where most
competing tools are brittle.

## 4. Test identity

A score is meaningless without a stable ID across runs.

```
test_id = sha256(suite + "\x00" + classname + "\x00" + name)[:16]
```

Human-readable fields are stored alongside for display.

**Parameterized tests** (`test_foo[param1]`, jest `test.each`) get two IDs: the exact ID
and a **normalized ID** with bracketed/parenthesized parameters stripped. This lets us
report both "this one case is flaky" and "this whole parameterized family is flaky,"
which are different bugs.

Test renames breaking history is a known v1 limitation.

## 5. Two operating modes

This split is the core architecture. Both write to the same store.

### Burst mode — `flakestat hunt`

Run the test command N times locally, right now. Code is constant, so *any*
disagreement between runs is flakiness by definition. This is the strong signal.

Use for: "is this test actually flaky?", pre-merge gates, verifying a fix.

### History mode — `flakestat ingest` + `report`

Ingest JUnit XML from CI over time and score by flip rate. This is measurement across
real-world conditions and catches environment-dependent flakes that a local burst
never will.

Use for: continuous tracking, trend detection, quarantine lists.

### Same-commit vs cross-commit weighting

A subtlety most naive tools get wrong: in history mode, a test failing on commit A and
passing on commit B may be a **genuine regression that got fixed** — not flakiness.

So every observation records its commit SHA, and transitions are weighted:

- **Same SHA, different result** → definitive flakiness. Weight high (default `3.0`).
- **Different SHA, different result** → ambiguous. Weight `1.0`.

This is a meaningful accuracy edge over "count the failures."

## 6. Scoring

**Failure rate is the wrong metric.** A test that fails 100% of the time scores 1.0 on
failure rate but is not flaky — it's broken. Flakiness is *inconsistency*.

For a test with ordered observations `o_1..o_n` (pass=1, fail=0; skips excluded):

```
t_i = 1 if o_i != o_{i+1} else 0          # state transition
w_i = same_commit_weight if sha_i == sha_{i+1} else 1.0

flip_rate = Σ(w_i · t_i) / Σ(w_i)
```

Then an exponential recency weight with `alpha` (default `0.3`) so recent behavior
dominates and fixed tests decay back to stable. **Age is counted in commits, not in
observations** — see the validation section; aging per observation capped effective
sample size and made extra runs worthless.

**Confidence:** below `min_runs` (default 5) a test is `insufficient-data`, never
scored. Above it, classification uses a **Wilson lower bound** on the score rather
than the point estimate, so a verdict requires evidence proportional to the claim.

**Classification** (configurable thresholds; `0.10` is measured, not chosen):

| Score | Class |
|---|---|
| `< 0.05` | stable |
| `0.05 – 0.10` | suspect |
| `> 0.10` | flaky |

Tests that always fail produce zero transitions → flip rate 0 → correctly classified as
**not flaky**. They're surfaced separately as `consistently-failing`, since silently
hiding a broken test would be worse than the flake.

## 7. Storage

Constraint: no cgo, keep the binary static. Rules out `mattn/go-sqlite3`.

**Decision: append-only NDJSON event log** in `.flakestat/runs.ndjson`.

- Zero dependencies, git-friendly, diffable, trivially mergeable across CI shards
- Teams can commit it for shared history or ship it as a CI artifact
- Derived state is recomputed on read (fast enough well past 10^6 observations)
- `flakestat compact` collapses old observations into summaries when it grows

If scale ever demands it, `modernc.org/sqlite` (pure Go) is the migration path.

One observation per line:

```json
{"ts":"2026-08-17T10:00:00Z","run_id":"01J...","commit":"abc123","branch":"main",
 "source":"hunt","test_id":"9f2a...","suite":"tests.api","class":"TestAuth",
 "name":"test_login[oauth]","status":"fail","duration_ms":412,
 "message":"timeout after 5s","attempt":0}
```

## 8. CLI surface

```
flakestat init                    # detect project, scaffold .flakestat.json
flakestat hunt   [flags] -- <cmd> # run N times, detect disagreement
flakestat ingest <junit.xml...>   # load CI results into history
flakestat report [flags]          # scored table / json / markdown
flakestat quarantine [flags]      # emit framework-native skip list
flakestat check  [flags]          # CI gate, meaningful exit codes
flakestat compact                 # collapse old history
```

### `hunt` — capturing output across parallel runs

Parallel runs need unique report paths. Two mechanisms, both supported:

- A `{run}` placeholder in `--junit`
- A `FLAKESTAT_RUN` env var injected into the child process

```bash
flakestat hunt --runs 20 --parallel 4 \
  --junit '.flakestat/reports/junit-{run}.xml' \
  -- pytest --junitxml='.flakestat/reports/junit-{run}.xml'
```

Key flags: `--runs N`, `--parallel P`, `--until-fail` (stop at first disagreement,
for fast reproduction), `--timeout`, `--filter`.

**Fallback:** if no JUnit XML is produced, record the process exit code and report
suite-level flakiness with a clear warning that per-test granularity is unavailable.

### `check` — the CI gate

```
exit 0  clean
exit 1  new flaky tests above threshold  (--fail-on-new)
exit 2  existing flakes worsened         (--fail-on-regression)
```

Also emits GitHub Actions annotations and a markdown summary suitable for a PR comment.

### `quarantine` — framework-native output

Emitting a generic list nobody can consume is a common failure mode. Emit what each
runner actually accepts:

| Format | Output |
|---|---|
| `pytest` | node IDs for `--deselect` |
| `go` | regex for `go test -skip` |
| `jest` | `testPathIgnorePatterns` / name patterns |
| `yaml` | generic `flakestat.quarantine.yaml` |

## 9. Config — `.flakestat.yaml`

```yaml
version: 1

command: pytest --junitxml={junit}
junit: .flakestat/reports/junit-{run}.xml
runs: 20
parallel: 4

scoring:
  min_runs: 5
  ewma_alpha: 0.3
  same_commit_weight: 3.0
  thresholds:
    suspect: 0.05
    flaky: 0.15

quarantine:
  format: pytest
  path: .flakestat/quarantine.txt

ignore:
  - "tests/integration/**"
```

Precedence: CLI flags > env vars > config file > defaults.

## 10. Package layout

```
cmd/flakestat/main.go
internal/cli/         command wiring, flag/config precedence
internal/junit/       tolerant XML parser  ← highest-value component
internal/runner/      parallel execution, timeouts, output capture
internal/store/       NDJSON append log, query, compaction
internal/score/       flip rate, EWMA, weighting, classification
internal/report/      table / json / markdown / gha renderers
internal/quarantine/  per-framework emitters
testdata/junit/       real-world XML dialects from every framework above
```

`testdata/junit/` matters more than it looks: a corpus of genuine XML from each
framework is what keeps the parser honest, and it's the natural place for outside
contributors to land a fix ("my framework's output breaks it, here's the file").

## 11. Build order

1. ✅ `internal/junit` + the dialect corpus — everything depends on it
2. ✅ `internal/store` — append/read NDJSON
3. ✅ `internal/score` — pure functions, heavily unit-tested
4. ✅ `hunt` + `internal/runner` — first end-to-end user value
5. ✅ `report` — table / JSON / markdown renderers
6. ✅ `ingest` + `check` — the CI story
7. ✅ `quarantine` emitters
8. ◐ `init` and the config file done; `compact` outstanding
9. ✅ Distribution: GoReleaser, Homebrew, ghcr.io, GitHub Action, npm, PyPI
10. ✅ Branch-aware scoring

### Measured validation

Two defects surfaced only once ground truth existed -- neither was reachable by
unit tests or by the 21-repo sweep, because both required knowing the real flake
rate in advance.

**Recency decayed per observation, so more runs bought no more confidence.**
Weights summed to at most `1/alpha`, capping effective sample size at ~3.3
transitions. On `p=0.25`, standard deviation across trials was 0.246 at 100 runs
versus 0.255 at 10 -- ninety extra runs bought nothing. Recency now ages in
**commits**: a burst shares one commit and therefore uses all of its evidence
(sd 0.052 at 100 runs). Cross-commit decay is unchanged.

**Verdicts ignored sample size.** A `p=0.02` test was called flaky 25% of the
time at 10 runs and 0% at 20 -- a verdict that improved with *less* data.
Classification now requires a Wilson lower bound (with Kish effective sample
size) to clear the threshold, so a verdict needs evidence.

**The flaky threshold is now measured.** At 100 runs, `0.15` detected a
`p=0.10` test in 45% of trials; `0.10` detects it in 80%, with stable and broken
tests still flagged 0% of the time. Default lowered to `0.10`. `0.08` was
rejected for reintroducing lottery verdicts on `p=0.05`.

Still uncalibrated: `same_commit_weight` and `branch_weight`, since every
validation run so far has been single-commit and single-branch.

### Two orthogonal questions

Observations answer two questions that must not be collapsed into one:

```
Observations
   |
   +-- temporal evidence  -> classification + score + confidence
   |
   +-- contextual evidence -> associations + effect size + significance
                                    |
                                    v
                               Explanation
```

Neither branch may rewrite the other's conclusion. A test is flaky because its
outcomes demonstrate flakiness; an association only says where that behaviour
concentrates.

Keeping them separate preserves information that a single label would destroy.
`stable` with a strong `os=windows` association means the test is deterministic
within each environment but behaves differently between them -- a different
phenomenon from `flaky` with the same association, where one platform appears
to increase nondeterminism. One combined class could not express both.

If a vocabulary for that is ever added it should be a *secondary* descriptor
("context pattern"), not a sixth primary class.

### Comparability: the transition invariant

    A transition is meaningful only between observations from the same
    execution context.

Execution context is currently:

    branch + os + arch + runtime.name + runtime.version

and explicitly excludes provenance identifiers -- `ci.run_id`, `ci.job_id` and
anything similarly high-cardinality. Including them would place every
observation alone in its own context, leaving no transitions to measure
anywhere.

This invariant has now been violated twice, in the same way, at two different
levels:

  - Interleaved *branches* made a test that passed on main and failed on an
    in-progress feature branch read as repeated disagreement.
  - Interleaved *platforms* made a test that always fails on Windows and always
    passes elsewhere read as constant disagreement, scoring 0.45 with an
    explanation asserting "direct evidence of nondeterminism". The test is
    perfectly deterministic on every platform.

Both were found only with real data; no synthetic fixture written beforehand
produced either. The second surfaced within minutes of pointing flakestat at
its own CI matrix.

The general shape is worth remembering when adding any new axis: two outcomes
are evidence of nondeterminism only if everything that could legitimately
change the outcome was held constant.

### Counting: the execution identity invariant

    Re-recording an execution must not create evidence.

An observation describes one execution of one test. The log is append-only and
designed to be merged with `cat`, so the same execution can reach it more than
once: an artifact uploaded twice, a job re-run, a shard collected by two
aggregators. Nothing in the record previously distinguished that from a second
execution that happened to agree.

Identity is derived from the evidence, never from ingestion:

    provider + run + job + shard + attempt + report path + report digest
      + test id + repetition index

Each part earns its place. `attempt` is what separates a legitimate retry --
which really did run the tests again -- from a duplicate. The report digest
separates a file from the file that later replaced it; the report path
separates two shards that emitted byte-identical XML. The repetition index is
not defensive: `-count=12` puts twelve executions of one test in one document,
and without it eleven results would vanish.

Nothing about *when* ingestion happened may enter the key, or re-ingesting
would mint a fresh one and restore the problem.

**The harm was the opposite of the one anticipated.** The expectation was that
a duplicate would inflate confidence -- more agreeing observations, a tighter
bound, a smaller p-value. Measured, it does the reverse. A copy carries its
original's timestamp, so it sorts adjacent to it, and a duplicate always agrees
with itself. Duplication therefore injects artificial *agreements*:

    a test failing 4 of 12 runs, ingested twice
    score        0.64 -> 0.30
    lower bound  0.510 -> 0.230
    transitions  11 -> 23

The evidence appears to double while the measured flip rate halves. The error
runs towards **false negatives** -- a flaky test made to look stable, with more
apparent support for the wrong answer. Duplication is not a bookkeeping
nuisance; it is a way of losing flaky tests quietly.

Dedup runs on read as well as on write, because `cat` never passes through the
write path. Observations carrying no key -- `hunt` results, or records written
before identity existed -- are always kept: `hunt` watched every execution
happen, so its results cannot be duplicates, and discarding evidence that
merely might be duplicated would be the worse error.

### Diagnosis is deliberately deferred

Categorising causes -- timing, race condition, network, shared state -- is
**not** unfinished work. It is deferred until observed signals justify each
category.

Nothing currently captured distinguishes a race from a slow network. Inferring
it anyway would be the one failure this project exists to avoid: presenting a
guess with the confidence of a measurement. The correlation layer states where
failures concentrate and explicitly declines to say why, including when two
dimensions are perfectly confounded and the data cannot separate them.

Not making a diagnosis is a feature of the evidence model, not a gap in it.

### Branch awareness

Transitions are computed **within a branch, never across one**. The original
flat chronological series manufactured flips: a test passing on `main`, failing
on an in-progress feature branch, then passing on `main` again reads as two
flips when nothing about `main` changed. Grouping by branch removes that class
of phantom signal outright — the same structural mistake as the cross-commit
problem, one level up.

Layered on top, disagreement off the default branch is scaled by
`branch_weight` (0.5), since failures on a feature branch are frequently just
unfinished work. The default branch is auto-detected from `origin/HEAD`, then
`main`/`master`.

### Config file: JSON, not YAML

§9 specified `.flakestat.yaml`. Shipped as `.flakestat.json` instead: YAML needs
a third-party parser, and a dependency-free binary is worth more to the
security-conscious audience that picks a local tool over a hosted one. The
command accepts both a list and a plain string, so hand-editing stays pleasant.

`{junit}` in the command expands to the resolved report path, so it is written
once instead of being kept in sync in two places — the single biggest source of
first-run friction.

### Revision from implementation

**The commit weighting was wrong as originally specified.** The design called
for `score = sum(w*t)/sum(w)` with `w = commit_weight * recency`. Because that
normalizes by the same weight it multiplies by, a uniform commit weight cancels
out entirely: an all-same-commit history and an all-cross-commit history scored
*identically*. The weighting only ever did anything on mixed histories, which is
not the comparison that matters.

Fixed by normalizing on recency alone and modelling cross-commit disagreement as
**discounted evidence** (`e = 1/same_commit_weight`) applied to the transition
value. Same-commit flips now count fully, cross-commit flips count a third, the
score stays bounded in `[0,1]`, and the absolute scores differ as intended.

Also learned: `text/tabwriter` counts ANSI escape bytes toward cell width even
with `StripEscape` set, so a column mixing colored and uncolored cells drifts.
The report renderer pads manually against uncolored widths instead.

**`--parallel` manufactured false positives, and the docs recommended it.**
Found by dogfooding against five public repos. Parallel runs execute copies of
the test command in one working directory, so any suite using fixed paths,
ports or a shared database collides with itself. Against `spf13/cobra` this
produced a wholly invented flaky verdict (score 0.60) that was indistinguishable
from a real finding; the same suite was clean sequentially, and the test passed
8/8 alone. Worse, the README's headline example used `--parallel 4`.

Not universal -- `jd/tenacity` stayed clean at `--parallel 4` -- so the fix is
not to remove the feature. Candidates found under `--parallel` are now re-run
sequentially and demoted from `flaky` to `suspect` if they do not reproduce.
Demotion rather than deletion is deliberate: failing to reproduce in five runs
is weak evidence of innocence, not proof.

The deeper lesson: the measurement apparatus was itself a source of signal,
which is precisely the failure this project exists to prevent.

**Always-skipped tests were reported as `insufficient-data`**, which claims more
runs would help. For a platform-gated test that is false at any number of runs.
They now get their own verdict (`always-skipped`); `pallets/click` has 26.

**The CI gate as originally shipped was unusable.** `report --fail-on-flaky`
fails on *any* flaky test, which turns a repo with existing flakiness red the
day it adopts flakestat — so the gate gets deleted. Replaced with `check` and a
committed baseline (§8's `--fail-on-new`, which should never have been folded
away): existing flakiness is grandfathered, and only new or measurably worsened
tests fail the build. A `--regression-delta` keeps scoring jitter from failing
builds, and `FIXED` tests are surfaced to prompt tightening the baseline.

## 12. Deliberately out of scope for v1

- Web UI or hosted dashboard
- Root-cause analysis (test ordering, shared state, timing)
- Auto-retry during a test run — that's the framework's job
- Rename/refactor tracking across history
- Direct CI-provider API integrations (ingest XML artifacts instead)

## 13. Open questions

- ✅ **Name availability** — 13 GitHub repos match "flakestat", all abandoned (max 4
  stars). No established project to collide with.
- ☐ **Module path** — currently `github.com/rowhitswami/flakestat`, a placeholder.
  Needs the real GitHub username before publishing.
- ☐ **Default `same_commit_weight` of 3.0** is a judgment call, not an empirical
  result. Worth revisiting once we have real corpus data.
- ☐ **Config file** — `.flakestat.yaml` (§9) is designed but not implemented; the CLI
  is flags-only so far, which keeps `go.mod` dependency-free. Adding YAML means taking
  on `gopkg.in/yaml.v3`.
