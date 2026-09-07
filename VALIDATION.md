# External validation contract

Written **before** any external experiment was run, and committed so its
timestamp is checkable. Everything below — subjects, stopping rules, frozen
configuration, and what each outcome is allowed to mean — is fixed in advance.
Deciding what counts as success after seeing results is how a detector gets
graded on noise.

## The failure mode this guards against

The tempting protocol is "run flakestat against real repositories until it
finds something". Given enough repositories and enough executions, it always
will, and the finding means nothing.

So the central rule is:

    Absence of reproduced flakiness is not evidence that flakestat failed.

If a project has a test that fails 2% of the time and our sample contains 200
passes, the correct output is silence. A detector cannot be graded against an
event that did not occur in its input. Where that happens, the run is recorded
as **inconclusive for sensitivity** — not as a pass and not as a failure.

## Frozen configuration

Defaults are frozen as of this commit and **must not be tuned in response to
any external result**. Tuning after seeing whether the tool detects a project's
named tests would make the validation no longer external.

    scoring
      min_runs             5
      ewma_alpha           0.3
      same_commit_weight   3.0
      suspect_threshold    0.05
      flaky_threshold      0.10
      branch_weight        0.5
      medium_evidence      10 transitions
      high_evidence        30 transitions

    association
      min_group            10
      min_others           10
      candidate_diff       0.10
      strong_diff          0.15
      max_q                0.01

If a genuine defect is found — as opposed to a threshold one would prefer to be
different — the fix is recorded here with the reasoning, and every prior
subject is re-run against the corrected build.

## Subjects

Ordered from strongest ground truth to weakest, so that the instrument is
checked against known answers before it is pointed at unknown ones.

### A. Known positive control — `uppadhyayraj/playwright-flaky-tests`

Deliberately contains a stable control alongside random, race,
network/timing and shared-state flaky tests.

Protocol: independent executions with **framework retries disabled**. Playwright
retries would collapse a fail-then-pass into a single reported outcome, which
is the framework's own flakiness handling standing in for the run-to-run
history flakestat is supposed to measure.

Expected:

    stable control            -> stable
    intentional flaky tests   -> suspect or flaky
    no healthy test           -> flaky

**Not** expected, and not checked: that a flakestat score equals the repository's
advertised failure probability. A flake score measures inconsistency between
consecutive runs; an advertised rate is a marginal failure probability. For a
Bernoulli process the first is roughly `2p(1-p)` of the second — they are
different quantities and agreement between them would be a coincidence.

### B. Realistic controlled positive — `DataDog/techstories-demo-app`

An actual application with Jest and Cypress, with deliberately flaky
integration scenarios — concurrent registration, database timeout simulation,
network latency, session-state races — kept separate from the normal suite.

This adds a second ecosystem and application-shaped behaviour.

Expected:

- intentionally flaky cases accumulate temporal evidence;
- ordinary tests stay quiet;
- differing failure mechanisms need no special handling;
- JUnit dialect and test identity stay correct across two runners.

### C. Wild documented flakiness — `ConduitIO/conduit`

Ground truth documented by the project's own contributors before flakestat
touched it: an open issue (July 2026) reporting three consecutive CI reruns
failing in unrelated suites before a fourth passed, naming

    TestServiceLifecycle_Stop/user_stop:_forceful
    TestClient_NotFound
    TestClient_CacheMiss
    TestClient_CacheHit
    TestEncodeDecode_ExtractAndUploadSchemaStrategy
    TestEncodeDecode_DownloadStrategy_Avro
    TestRandOld

attributed to timing, shared registry/cache state, parallel contention and
shared `math/rand` state.

Being Go, `gotestsum --junitfile` produces JUnit XML with no modification to
the repository.

This is the only subject where catching a wild flake is genuinely attempted.
It is a bonus result, not a requirement.

## Stopping rule

For each subject, stop at whichever comes first:

    A. enough contradictory outcomes to cross the evidence threshold, or
    B. 100 independent executions

100 is chosen so that a missed detection is meaningful rather than
unremarkable. Prior measured sensitivity was 100% recall at p >= 0.25 and
roughly 80% at p = 0.10 with 50+ runs, so a test flaky at those rates that goes
undetected in 100 executions is a real finding. Below p = 0.05 detection was
already known to be unreliable, and silence there says nothing.

## What each outcome means

| Outcome | Interpretation |
| --- | --- |
| Known flaky test reproduces and flakestat detects it | positive validation |
| Known flaky test never fails in our sample, flakestat silent | correct, but **inconclusive for sensitivity** |
| Stable test stays stable | negative validation |
| Environment-specific deterministic behaviour becomes `stable` + association | positive validation |
| An always-failing test called flaky | **bug** |
| A healthy test called flaky from interleaving or context effects | **bug** |
| A correlation reported from sparse evidence below thresholds | **bug** |
| Wild undocumented flake found and reproducible | major result, not required |

## Reporting format

Raw evidence is recorded next to the judgment, never replaced by it. No row
states an expectation as though it were an observation:

    Test                    External evidence    Observed   flakestat
    stable-control          stable               0/100      stable
    random-flake            intentional          27/100     flaky
    TestRandOld             documented wild      3/80       suspect
    TestClient_CacheHit     documented wild      0/80       stable *

    * no contradictory outcome observed in this experiment; historical
      flakiness was not supplied to flakestat as input

## Status

| Subject | State |
| --- | --- |
| A. playwright-flaky-tests | not started |
| B. techstories-demo-app | not started |
| C. ConduitIO/conduit | not started |
