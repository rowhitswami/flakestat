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

#### Amended before running: a before/after design

Inspecting the repository first — as the contract intends, since the protocol
must be fixed before results exist — turned up something better than the
original plan.

The issue is still open, but the named tests have since been repaired by the
maintainers in an identifiable series of commits, and the project added its own
`flake-hunt.yml` job that repeats the suite nightly. At `HEAD` those tests are
expected to be clean, so pointing flakestat there would produce silence that
means nothing.

So subject C is run at two commits instead of one:

    C1  612f5bfb  the parent of "test: deflake known-flaky tests from #2534"
                  (#2537, 2026-07-05). Ground truth: documented flaky.
    C2  104f91a9  current HEAD, after that fix and the follow-up backlog
                  commit (#2542). Ground truth: documented fixed.

Both use the identical protocol. That converts a one-sided test into a
controlled comparison whose ground truth — both the defect and the repair —
was established by the project's own contributors, with no involvement from
this tool:

    detected at C1, silent at C2   -> strong positive validation
    silent at both                 -> inconclusive for sensitivity
    flagged at C2                  -> false positive, a bug
    silent at C1 but flagged at C2 -> a bug, and a serious one

Scope is limited to the packages that commit touched, which hold four of the
seven named tests:

    pkg/plugin/processor/builtin/internal/diff/lcs   TestRandOld
    pkg/schemaregistry                               TestClient_NotFound
                                                     TestClient_CacheMiss
                                                     TestClient_CacheHit

**Executions use `-shuffle=on`, one fresh process each.** These are
order-dependent failures rooted in process-global state — shared `math/rand`,
a shared in-memory registry — so Go's default declaration order would produce
the same outcome every run and no run-to-run variation to observe. Shuffling
samples the orderings real CI and real developers actually encounter, and it is
what the project's own flake-hunt job does. This is sampling the subject's
behaviour, not provoking it: no test is modified, and the same protocol runs at
both commits, so C2 has every opportunity to flake that C1 does.

This is the only subject where catching a real flake is genuinely attempted. It
is a bonus result, not a requirement.

## Protocol amendments

Recorded as additions with their reasoning. Earlier entries are left standing,
because a preregistration that gets edited is not one.

### 2026-09-07 — C2 tightened to the exact repair commit

Originally C2 was `HEAD`. That answers a weaker question than intended: if C1
flakes and HEAD does not, the honest conclusion is only *something between
these commits removed the behaviour*, not *the documented repair removed it*.
Hundreds of unrelated commits sit in between.

The comparison is now against the repair itself, with `HEAD` retained
separately as a durability check rather than as the contrast:

    C1  612f5bfb  parent of the repair          documented flaky
    C2  9e00e594  the repair, #2537             documented fixed
    C3  104f91a9  HEAD, optional                is the repair still holding?

C1 -> C2 differ only by that commit, so a difference in behaviour is
attributable to it. C1 -> C3 answers a separate and also useful question.

No Conduit execution had been run when this was written.

### 2026-09-07 — Conduit invocation stated explicitly

    go test -count=1 -shuffle=on <packages>

`-shuffle=on` alone already defeats the test cache in this invocation, but
`-count=1` states the experimental requirement rather than relying on that:
**execute the tests, never reuse a cached result.**

`go test` prints the shuffle seed it used. That seed is recorded per execution,
so any failure carries

    commit + runtime + test + shuffle seed + failure message

and is reproducible by a third party, rather than being "run 47 failed".

### 2026-09-07 — a defect found before the freeze, and how it was handled

Checking the skip invariant surfaced an unrelated bug: the history strip in
`explain` grouped transitions by branch while scoring grouped them by execution
context, so the rendered evidence printed "flip on identical code" at every
platform boundary. Verdicts were correct; the evidence displayed beneath them
was not.

It cannot affect any external subject, all of which run on a single machine
with one platform, one runtime and one branch — branch grouping and execution
context coincide there, so the strip and the verdict agree by construction. It
was fixed before the freeze rather than during the experiments.

Subject A was already executing against the pre-fix build when this was found.
Rather than restart it, note that execution and measurement are separable here:
the raw evidence is the JUnit XML on disk, and scoring happens afterwards. Every
subject's XML is **re-ingested and re-scored with the frozen build** before any
result is reported, so all three subjects are measured by one implementation
regardless of which build produced the XML.

## Frozen implementation

Fixed for the duration of A, B and C. If a genuine correctness bug is found
mid-experiment, affected results are invalidated, the build gets a new SHA
recorded here, and every affected subject is re-scored from its retained XML.
Nothing is quietly patched and continued.

    flakestat commit   465faaec
    flakestat version  dev (built from source)
    go                 go1.25.4
    host               Darwin arm64

    subject A          uppadhyayraj/playwright-flaky-tests @ dd4738a4
                       npx playwright test --workers=1 --retries=0 --reporter=junit
                       CI unset, so the repo's own config yields retries=0

    subject B          DataDog/techstories-demo-app @ (recorded when run)

    subject C          ConduitIO/conduit @ 612f5bfb / 9e00e594 / 104f91a9
                       go test -count=1 -shuffle=on <in-scope packages>
                       repo targets go 1.25.8

## Descriptive metrics

Captured for every subject and reported alongside the verdict. These describe
behaviour; they are **not** success criteria and must not be used to move the
bar set above.

    observations until first contradictory outcome
    observations until suspect
    observations until flaky
    final pass/fail counts
    final classification, score and confidence

Detection latency is the interesting one, and is a far more meaningful property
of a detector than whether a flake score resembles an advertised failure
probability. Reported per test:

    test_random_failure
      first failure             run 7
      first contradictory pair  run 7
      reached suspect           run 11
      reached flaky             run 24
      final                     74 pass / 26 fail, flaky, high confidence

A verdict snapshot is written after every execution, so these are recovered
from the record rather than estimated afterwards.

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
| A. playwright-flaky-tests | running — 100 executions, retries=0, workers=1 |
| B. techstories-demo-app | not started |
| C1. conduit @ 612f5bfb (pre-fix) | not started |
| C2. conduit @ 104f91a9 (post-fix) | not started |

Subject A is pinned at `dd4738a4`. Its "stable" control navigates to the live
`playwright.dev`, so a control failure there is ambiguous between a tool error
and a genuine network event; raw failure messages are kept so the two can be
told apart rather than assumed.
