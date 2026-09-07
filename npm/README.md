<p align="center">
  <img src="https://rowhitswami.github.io/flakestat/assets/logo-lockup.png" alt="flakestat" width="420">
</p>

<p align="center"><a href="https://rowhitswami.github.io/flakestat/"><b>Documentation</b></a></p>

---

Find flaky tests in any language. No SaaS, no account, no data leaving your machine.

A flaky test passes and fails randomly **on the same code**. flakestat tells you
exactly which ones, ranked worst first.

```bash
npx flakestat hunt --runs 20 \
  --junit 'reports/junit-{run}.xml' \
  -- npx jest --reporters=jest-junit
```

```
VERDICT               SCORE  RUNS  PASS/FAIL  TEST
flaky                  0.62    20       14/6  auth.test.js::refreshes an expired token
consistently-failing   0.00    20       0/20  billing.test.js::applies a discount

1 flaky, 0 suspect, 1 consistently failing, 1 stable, 1 unscored (of 4 tests)
```

## Install

```bash
npm install --save-dev flakestat   # or: npx flakestat
```

This package downloads the prebuilt `flakestat` binary for your platform
(macOS, Linux, Windows on x64/arm64) from GitHub Releases.

## Why the verdicts matter

**Flakiness is inconsistency, not failure.** A test that fails 100% of the time
scores 1.0 on failure rate but isn't flaky — it's broken. flakestat scores
*state transitions* (pass→fail→pass), so an always-failing test correctly scores
zero and is reported separately as `consistently-failing` instead of being mixed
in with real flakes.

## Jest and Vitest setup

Both emit the JUnit XML flakestat reads.

**Jest** — `npm i -D jest-junit`:

```bash
npx flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \
  -- npx jest --reporters=jest-junit
```

with `JEST_JUNIT_OUTPUT_NAME` pointed at the same `{run}` path, or configure
`jest-junit` in `package.json`.

**Vitest**:

```bash
npx flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \
  -- npx vitest run --reporter=junit --outputFile='reports/junit-{run}.xml'
```

`{run}` is replaced with the run number so parallel runs don't overwrite each
other. It's also exported to your command as `$FLAKESTAT_RUN`.

## Tracking over time

A burst proves flakiness exists. History *measures* it, and catches
environment-dependent flakes a local burst never will.

```bash
npx flakestat ingest 'reports/**/*.xml'   # in CI, after tests
npx flakestat report --top 20
npx flakestat report --fail-on-flaky      # exit 1 to gate a pipeline
```

## Working out why a test is flaky

```bash
npx flakestat explain test_checkout_timeout
```

Shows the evidence behind a verdict: pass/fail counts, how often the outcome
changed, how much of that happened on identical code, and a history strip
(`P P F P P F`) marking each flip. `--json` emits the same for tooling.

Every verdict also carries a confidence level, because `0.41` from three runs
and `0.41` from three hundred are different claims.

## Environment variables

| Variable | Effect |
|---|---|
| `FLAKESTAT_BINARY` | Use this binary instead of downloading |
| `FLAKESTAT_SKIP_DOWNLOAD` | Fail instead of downloading |

## Full documentation

<https://github.com/rowhitswami/flakestat>

MIT
