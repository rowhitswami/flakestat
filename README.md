<p align="center">
  <img src="https://flakestat.com/assets/logo-lockup.png" alt="flakestat" width="460">
</p>

<p align="center">
  Find flaky tests in any language. One static binary, no SaaS, no account,
  and your test results never leave your machine.
</p>

<p align="center">
  <a href="https://flakestat.com/"><b>Documentation</b></a> ·
  <a href="https://flakestat.com/docs/#quickstart">Quickstart</a> ·
  <a href="https://flakestat.com/docs/#commands">Commands</a> ·
  <a href="https://github.com/rowhitswami/flakestat/releases">Releases</a>
</p>

---

A flaky test passes and fails on the same code. flakestat reads the JUnit XML
your test runner already writes, and tells you which tests are actually flaky:
ranked, with a confidence level, and with the evidence behind every verdict.

```
VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST
flaky                  0.62  high    20       14/6  test_demo::test_flaky_race
consistently-failing   0.00  high    20       0/20  test_demo::test_broken

1 flaky, 0 suspect, 1 consistently failing, 1 stable, 1 unscored (of 4 tests)
```

## Install

```bash
brew install rowhitswami/tap/flakestat     # macOS / Linux
npm install --save-dev flakestat           # JS / TS
pip install flakestat                      # Python
go install github.com/rowhitswami/flakestat/cmd/flakestat@latest
```

Also available as a [container](https://github.com/rowhitswami/flakestat/pkgs/container/flakestat),
a [GitHub Action](https://flakestat.com/docs/#github-actions), and
[prebuilt binaries](https://github.com/rowhitswami/flakestat/releases) for macOS,
Linux and Windows.

## Try it

Run your suite a few times and see what disagrees:

```bash
flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \
  -- pytest --junitxml='{junit}'
```

Or record CI runs over time, which costs no extra compute:

```bash
flakestat ingest 'reports/**/*.xml'   # after your tests, in CI
flakestat report --top 20             # any time
```

Then ask why a particular test was flagged:

```bash
flakestat explain test_checkout_flow
```

## What makes it accurate

**Flakiness is inconsistency, not failure.** A test failing 100% of the time
isn't flaky, it's broken. flakestat scores pass→fail→pass transitions, so a
broken test scores zero and is reported separately.

**Evidence is weighed, not just counted.** Disagreement on the same commit is
proof; across commits it may be a regression someone already fixed, so it
counts for less. Every verdict carries a confidence level, and a small sample
can never reach `high` no matter how flaky the test looks.

**Nothing is compared that isn't comparable.** Two outcomes are evidence of
nondeterminism only if everything that could legitimately change the result
was held constant: branch, platform and runtime.

## Don't take my word for it

flakestat's strongest validation isn't a fixture I wrote. ConduitIO's own
contributors reported four flaky tests, named them, and later fixed them.
This runs the identical protocol either side of *their* fix:

```bash
git clone https://github.com/rowhitswami/flakestat && cd flakestat
./scripts/reproduce-validation.sh
```

```
test                       before their fix     at their fix
TestClient_NotFound        flaky 0.66  50/75    stable 0.00  0/75
TestClient_CacheMiss       flaky 0.66  50/75    stable 0.00  0/75
TestClient_CacheHit        flaky 0.66  50/75    stable 0.00  0/75
```

About two minutes. I did not create the bug, label the bug, or repair the bug.
flakestat is handed observations from both sides of a commit it had no part in,
and separates them. The full methodology, including the protocol arm that found
nothing and a contaminated run that was discarded, is in
[VALIDATION.md](VALIDATION.md).

## Works with

pytest · Jest · Vitest · go test · JUnit · TestNG · Maven Surefire · RSpec ·
PHPUnit · Mocha · Playwright · Cypress · xUnit · NUnit · and anything else that
writes JUnit XML.

No plugin to install and no framework lock-in. If your runner can emit a JUnit
report, flakestat can read it.

## Documentation

The [documentation site](https://flakestat.com/) covers
everything: every command and flag, CI recipes, how scoring works, how to keep
a shared history, and how flakestat avoids claiming more than the evidence
supports.

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) covers
what a useful bug report contains and the extra bar a scoring change has to
clear. [DESIGN.md](DESIGN.md) explains why the tool works the way it does,
including the mistakes that shaped it, and is worth reading before proposing a
change to scoring.

```bash
git clone https://github.com/rowhitswami/flakestat
cd flakestat && go test ./...
```

flakestat has no dependencies. Please keep it that way.

## Security

Found something exploitable? Please report it privately through
[GitHub's private vulnerability reporting](https://github.com/rowhitswami/flakestat/security/advisories/new)
rather than opening an issue. [SECURITY.md](SECURITY.md) sets out the threat
model: flakestat parses XML it does not control, its installers download and
verify a binary, and it runs the test command you give it.

## License

[MIT](LICENSE). See also the [code of conduct](CODE_OF_CONDUCT.md).
