<p align="center">
  <img src="https://rowhitswami.github.io/flakestat/assets/logo-lockup.png" alt="flakestat" width="420">
</p>

<p align="center"><a href="https://rowhitswami.github.io/flakestat/"><b>Documentation</b></a></p>

---

Find flaky tests in any language. No SaaS, no account, no data leaving your machine.

A flaky test passes and fails randomly **on the same code**. flakestat tells you
exactly which ones, ranked worst first.

```bash
flakestat hunt --runs 20 \
  --junit 'reports/junit-{run}.xml' \
  -- pytest -q --junitxml='reports/junit-{run}.xml'
```

```
VERDICT               SCORE  RUNS  PASS/FAIL  TEST
flaky                  0.62    20       14/6  tests.test_auth::test_refresh_token
consistently-failing   0.00    20       0/20  tests.test_billing::test_discount

1 flaky, 0 suspect, 1 consistently failing, 1 stable, 1 unscored (of 4 tests)
```

## Install

```bash
pip install flakestat
```

This package downloads the prebuilt `flakestat` binary for your platform
(macOS, Linux, Windows on x86_64/arm64) from GitHub Releases on first use. It has
no Python dependencies, so it cannot interfere with your project's resolution.

## Why the verdicts matter

**Flakiness is inconsistency, not failure.** A test that fails 100% of the time
scores 1.0 on failure rate but isn't flaky — it's broken. flakestat scores
*state transitions* (pass→fail→pass), so an always-failing test correctly scores
zero and is reported separately as `consistently-failing` instead of being mixed
in with real flakes.

Skipped tests are excluded rather than treated as passes, so `@pytest.mark.skip`
never manufactures a phantom transition.

## How it compares

`pytest-rerunfailures` and `pytest-flakefinder` retry tests so a run goes green.
That hides the symptom. flakestat *measures* which tests are unreliable and by
how much, so they can be fixed — and it works across every suite in a polyglot
repo, not just the Python one.

## Tracking over time

A burst proves flakiness exists. History *measures* it, and catches
environment-dependent flakes a local burst never will.

```bash
flakestat ingest 'reports/**/*.xml'   # in CI, after tests
flakestat report --top 20
flakestat report --fail-on-flaky      # exit 1 to gate a pipeline
```

History lives in `.flakestat/runs.ndjson` — one JSON object per line. Commit it
for shared team history, or keep it as a CI artifact.

## Working out why a test is flaky

```bash
flakestat explain test_checkout_timeout
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
| `FLAKESTAT_CACHE_DIR` | Where the binary is cached |

## Full documentation

<https://github.com/rowhitswami/flakestat>

MIT
