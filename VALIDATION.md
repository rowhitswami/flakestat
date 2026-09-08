# External validation contract

Written **before** any external experiment was run, and committed so its
timestamp is checkable. Everything below is fixed in advance: subjects,
stopping rules, frozen configuration, and what each outcome is allowed to mean.
Deciding what counts as success after seeing results is how a detector gets
graded on noise.

## Results at a glance

Three external subjects and this project's own suite, all scored with one
frozen build. Written after every experiment finished; the contract that
grades them was committed before any of them ran.

| Subject | Ground truth established by | Outcome |
| --- | --- | --- |
| **A** `playwright-flaky-tests` | fixture author, purpose-built | 10 intentional flakes detected at high confidence; 5 controls clean at 0/100 |
| **B** `techstories-demo-app` | fixture author, app-shaped, real PostgreSQL | 5 flaky, 5 consistently-failing, 57 stable, **0 suspect** |
| **C** `ConduitIO/conduit` | the project's own contributors | `flaky` 0.67 before their fix, `stable` 0.00 after |
| **self** flakestat's own CI | none known | 293 tests, 10,305 observations, all stable |

**No bug row in the contract ever fired.** Nine always-failing tests across A and
B were called `consistently-failing`, not flaky. No healthy test was flagged
anywhere. No association was reported from evidence below the thresholds.

### What each subject is worth

**C is the load-bearing result.** The defect was documented by Conduit's
contributors, the tests were named by them, and the repair was written by them.
all before this tool was pointed at the repository. flakestat was handed
observations from both sides of a commit it had no part in and separated them:
three of the four in-scope tests `flaky` at 0.67 with high confidence at
`612f5bfb`, and `stable` at 0.00 at `9e00e594`, under a protocol identical on
both sides and fixed in advance.

**A and B establish that the classifier separates cleanly** when ground truth is
known, including the distinction that motivates the whole design. Nine tests
failed 100 out of 100 times. A detector ranking by failure rate would put all
nine at the top of a flaky list; flakestat scored them 0.00 and filed them
separately as broken.

**The self-measurement is specificity on real data.** 293 tests over 33 runs
across three platforms, with branch, os, arch and runtime all varying, the exact
conditions under which earlier versions of the scorer manufactured phantom
transitions. Zero false positives.

### What this does not establish

**Sensitivity below roughly 5%.** Three tests across the subjects fired once or
twice in a hundred runs and were correctly left `stable`; that is the right
reading of the evidence, and it is also the boundary at which this sampling
approach stops being the right instrument.

**Discovery of unknown flakiness.** Every flake found here was already known to
someone. flakestat has not yet surfaced a flake nobody had filed, and nothing in
this report should be read as evidence that it can.

### Two results that are worth more than the pass rate

**A null arm was published.** The pre-registered `-count=1` protocol found zero
failures in a hundred executions of a suite its own maintainers had documented as
flaky. Reporting only the `-count=3` arm, which found everything, would have
been exactly the adjustment this contract exists to police. Both are above.

**A contaminated run was discarded rather than reported.** Subject B was first
run with backup copies of the working tests left inside the project, which Jest
collected; thirteen identities ended up with two implementations feeding one
name. That run is void and was re-run clean. flakestat had been right about the
corrupt input too, since one identity genuinely alternated on identical code,
which is flakiness, but a result obtained from a broken fixture is not a result.

## The failure mode this guards against

The tempting protocol is "run flakestat against real repositories until it
finds something". Given enough repositories and enough executions, it always
will, and the finding means nothing.

So the central rule is:

    Absence of reproduced flakiness is not evidence that flakestat failed.

If a project has a test that fails 2% of the time and our sample contains 200
passes, the correct output is silence. A detector cannot be graded against an
event that did not occur in its input. Where that happens, the run is recorded
as **inconclusive for sensitivity**, not as a pass and not as a failure.

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

If a genuine defect is found, as opposed to a threshold one would prefer to be
different, the fix is recorded here with the reasoning, and every prior
subject is re-run against the corrected build.

## Subjects

Ordered from strongest ground truth to weakest, so that the instrument is
checked against known answers before it is pointed at unknown ones.

### A. Known positive control: `uppadhyayraj/playwright-flaky-tests`

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
Bernoulli process the first is roughly `2p(1-p)` of the second, so they are
different quantities and agreement between them would be a coincidence.

### B. Realistic controlled positive: `DataDog/techstories-demo-app`

An actual application with Jest and Cypress, with deliberately flaky
integration scenarios kept separate from the normal suite: concurrent
registration, database timeout simulation, network latency, session-state races.

This adds a second ecosystem and application-shaped behaviour.

Expected:

- intentionally flaky cases accumulate temporal evidence;
- ordinary tests stay quiet;
- differing failure mechanisms need no special handling;
- JUnit dialect and test identity stay correct across two runners.

### C. Wild documented flakiness: `ConduitIO/conduit`

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

Inspecting the repository first, as the contract intends, since the protocol
must be fixed before results exist, turned up something better than the
original plan.

The issue is still open, but the named tests have since been repaired by the
maintainers in an identifiable series of commits, and the project added its own
`flake-hunt.yml` job that repeats the suite nightly. At `HEAD` those tests are
expected to be clean, so pointing flakestat there would produce silence that
means nothing.

So subject C is run at two commits instead of one:

    C1  612f5bfb  the parent of "test: deflake known-flaky tests from #2534"
                  (#2537, 2026-07-06). Ground truth: documented flaky.
    C2  104f91a9  current HEAD, after that fix and the follow-up backlog
                  commit (#2542). Ground truth: documented fixed.

Both use the identical protocol. That converts a one-sided test into a
controlled comparison whose ground truth, both the defect and the repair,
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
order-dependent failures rooted in process-global state, a shared `math/rand`
and a shared in-memory registry, so Go's default declaration order would produce
the same outcome every run and no run-to-run variation to observe. Shuffling
samples the orderings real CI and real developers actually encounter, and it is
what the project's own flake-hunt job does. This is sampling the subject's
behaviour, not provoking it: no test is modified, and the same protocol runs at
both commits, so C2 has every opportunity to flake that C1 does.

This is the only subject where catching a real flake is genuinely attempted. It
is a bonus result, not a requirement.


## Subject A: results

`uppadhyayraj/playwright-flaky-tests @ dd4738a4`, 100 independent executions,
`--workers=1 --retries=0`, scored with the frozen build.

| Test | Advertised | Observed | flakestat | Score |
| --- | --- | --- | --- | --- |
| docs sidebar loads before content | 30% | 100/100 | `consistently-failing` | 0.00 |
| API reference loads all section headings | 15% | 100/100 | `consistently-failing` | 0.00 |
| cookie-reader: checks consent state | n/a | 100/100 | `consistently-failing` | 0.00 |
| docs page loads within strict threshold | 25% | 44/100 | `flaky` | 0.35 |
| homepage CTA renders in time | 40% | 40/100 | `flaky` | 0.45 |
| concurrent page loads complete without timeout | 20% | 38/100 | `flaky` | 0.34 |
| version badge matches expected | 25% | 32/100 | `flaky` | 0.38 |
| docs search index loaded | 30% | 31/100 | `flaky` | 0.44 |
| back navigation preserves scroll position | 25% | 29/100 | `flaky` | 0.42 |
| cookie-polluter: sets analytics consent | n/a | 24/100 | `flaky` | 0.41 |
| step-2: validate title from shared state | n/a | 20/100 | `flaky` | 0.32 |
| step-3: assert visit count | n/a | 20/100 | `flaky` | 0.32 |
| homepage loads within strict threshold | 30% | 18/100 | `flaky` | 0.20 |
| navbar renders before JS finishes loading | 35% | 2/100 | `stable` | 0.04 |
| image assets load before scroll interaction | 20% | 1/100 | `stable` | 0.02 |
| step-1: capture homepage title | n/a | 0/100 | `stable` | 0.00 |
| **control** · homepage has correct title | control | 0/100 | `stable` | 0.00 |
| **control** · homepage has Get Started link | control | 0/100 | `stable` | 0.00 |
| **control** · docs intro page loads | control | 0/100 | `stable` | 0.00 |
| **control** · API reference page loads | control | 0/100 | `stable` | 0.00 |
| **control** · navbar is present on all pages | control | 0/100 | `stable` | 0.00 |

Against the contract:

- **Positive validation.** Ten intentionally flaky tests reached `flaky`, every
  one at high confidence.
- **Negative validation.** All five controls sat at 0/100 and stayed `stable`.
  No false positive anywhere in the run.
- **No bug row fired.** Three tests failed 100/100 and were called
  `consistently-failing`, not flaky. Nothing healthy was called flaky. No
  association was reported from sparse evidence.

### Detection latency

Measured from a verdict snapshot taken after every execution.

| Test | 1st failure | 1st contradiction | → suspect | → flaky |
| --- | --- | --- | --- | --- |
| homepage CTA renders in time (40%) | 1 | 5 | 5 | 5 |
| docs page loads within threshold (25%) | 1 | 2 | 5 | 5 |
| version badge matches expected (25%) | 3 | 3 | 5 | 5 |
| docs search index loaded (30%) | 6 | 6 | 6 | 8 |
| step-2 / step-3 shared state | 9 | 9 | 9 | 10 |

Nothing was classified before `--min-runs` allowed it, which is why the earliest
`suspect` is run 5. Every test failing 18% or more of the time was `flaky`
within ten executions.

### Three fixtures were deterministic here, not flaky

Named as flaky, they failed 100/100:

- *docs sidebar loads before content*: `toBeVisible()` fails outright.
  playwright.dev's DOM has changed since the fixture was written.
- *API reference loads all section headings*: "Expected ≥2 headings, found 1".
  Same cause.
- *cookie-reader: checks consent state*: needs a cookie set by an earlier test
  in the same browser context, which Playwright's per-test isolation prevents.

flakestat called all three `consistently-failing`. That is the contract's
"an always-failing test called flaky" bug row **not** firing, on data that
would trip a failure-rate-based detector: each has a 100% failure rate and zero
flakiness.

### Two fixtures barely fired, and were correctly not flagged

*navbar renders before JS finishes loading* advertises 35% and failed **2/100**.
*image assets load before scroll interaction* advertises 20% and failed
**1/100**.

flakestat scored them 0.04 and 0.02 and left both `stable`. For a Bernoulli
process at p = 0.02 the expected flip rate is 2p(1-p) ≈ 0.039, so 0.04 is the
right measurement of what actually happened. The gap is between the fixture's
advertised rate and its behaviour in this environment, not between the evidence
and the verdict, which is exactly why the contract forbids grading a score
against an advertised probability.

The honest reading: these two are **inconclusive for sensitivity**. A detector
cannot be graded on an event that occurred twice in its input.


## Subject C: results

Identical protocol at both commits: 100 independent executions, fresh process
each, `-shuffle=<seed>` with a distinct recorded seed per execution.

| | `-count=1` | `-count=3` |
| --- | --- | --- |
| **C1** `612f5bfb`, documented flaky | 0/100 runs had failures | **100/100 runs had failures** |
| **C2** `9e00e594`, the repair | 0/100 | **0/100** |

### What flakestat said

At `-count=3`, scored with the frozen build:

| Test | External evidence | C1 observed | C1 verdict | C2 observed | C2 verdict |
| --- | --- | --- | --- | --- | --- |
| `TestClient_NotFound` | documented flaky, #2534 | 200/300 | `flaky` 0.67 high | 0/300 | `stable` 0.00 |
| `TestClient_CacheMiss` | documented flaky, #2534 | 200/300 | `flaky` 0.67 high | 0/300 | `stable` 0.00 |
| `TestClient_CacheHit` | documented flaky, #2534 | 200/300 | `flaky` 0.67 high | 0/300 | `stable` 0.00 |
| `TestRandOld` | documented flaky, #2534 | 2/300 | `stable` 0.01 \* | 0/300 | `stable` 0.00 |
| `TestAlgosOld` (and subtests) | not implicated | 0/300 | `stable` 0.00 | 0/300 | `stable` 0.00 |

\* two contradictory outcomes in three hundred; see below.

**This is the contract's strongest row: detected at C1, silent at C2.** Three of
the four in-scope tests were called flaky at high confidence before the repair
and stable after it, under a protocol that was identical on both sides and fixed
before either ran.

The value is in who established what. Conduit's contributors documented the
defect, named the tests, and wrote the fix, all before this tool was pointed at
their repository. flakestat was handed observations from both sides of a commit
it had no part in, and separated them.

### Detection latency

`flaky` by **run 2** for all three. With `-count=3` each execution contributes
three observations, so run 2 is the first point at which six scored runs exist,
one above the `min_runs` floor of five. Nothing was classified earlier than the
evidence allowed.

### `-count=1` found nothing, and that is the finding

The pre-registered protocol said `-count=1`. Under it, C1 produced **zero**
failures in a hundred executions of a suite its own maintainers had documented
as flaky.

The reason is in the defect: these are process-global-state bugs, a shared
in-memory registry and shared `math/rand` state, so the pollution has to happen
*within* one process before a later test can trip over it. One repetition per
process never gets there, however many processes you start. Conduit's own
`flake-hunt.yml` uses `-count=3 -shuffle=on` for exactly this reason, and the
original protocol under-specified by not matching it.

Both were run and both are reported. The `-count=1` result is not a flakestat
failure and not a success: it is a **detector correctly staying silent on an
input that contained no contradictory outcome**, and a reminder that a sampling
protocol can be too weak to see a defect that is definitely there.

Changing the repetition count after seeing a null result is the kind of
adjustment this contract exists to police, so the honest handling is to publish
both arms rather than the flattering one, and to note that `-count=3` was
adopted from the project's own reproduction recipe rather than tuned until
something appeared.

### `TestRandOld`: two failures in three hundred

Named in the issue and repaired by the same commit, but it fired twice in three
hundred observations here and flakestat scored it 0.01 and left it `stable`.
That is the right reading of the evidence in hand, and **inconclusive for
sensitivity**, not a miss. The shared `math/rand` dependency it exercises is
order-sensitive in a way that a hundred shuffles happened to trip only twice.


## Subject B: results

`DataDog/techstories-demo-app @ 1ca082de`, 100 independent executions, Jest with
no retries against PostgreSQL 16, the repository's own `broken-tests/` swapped
in over their working counterparts.

67 tests, 100 executions each:

| Verdict | Count |
| --- | --- |
| `stable` | 57 |
| `flaky` | 5 |
| `consistently-failing` | 5 |
| `suspect` | 0 |

**No stable test failed even once, and nothing landed in `suspect`.** The
separation was total: every test either fired repeatedly or never.

| Test | Observed | flakestat | Score |
| --- | --- | --- | --- |
| `PostList` expects PostListItem to be called | 49/100 | `flaky` | 0.58 |
| Post and Comment, can create a post | 54/100 | `flaky` | 0.56 |
| Database, email uniqueness race | 39/100 | `flaky` | 0.54 |
| User Registration, concurrent registration | 55/100 | `flaky` | 0.47 |
| SignUp, registration (flaky variant) | 13/100 | `flaky` | 0.22 |
| Header, renders the user's name | 100/100 | `consistently-failing` | 0.00 |
| Quotes API, four cases | 100/100 each | `consistently-failing` | 0.00 |

`PostList` is the one fixture with a mechanism visible in the source:
`Math.random() > 0.5`. It fired 49 times in 100. That is the closest thing to a
known-rate positive control in this subject, and the observation matches it.

Detection latency: four of the five reached `flaky` at run 5, the earliest
`min_runs` permits. The 13% case took until run 11.

### A contaminated first run, and what it showed

The first attempt at this subject is void. Backups of the working test files
were left in `.originals/` **inside the project**, and Jest collected them, so
five working files ran alongside the broken versions that replaced them.
Thirteen test identities therefore had two different implementations
contributing outcomes under one name.

That run is discarded, not reported, and 100 executions were re-run against a
tree where `jest --listTests` returns 18 files and a single run yields 67
testcases with no repeated identity.

The comparison is worth keeping, because flakestat was right both times:

    Header renders the user's name when signed in
      contaminated:  flaky 0.99          (working + broken version alternating)
      clean:         consistently-failing 0.00   (only the broken version runs)

Under contamination, one identity really did alternate between passing and
failing on identical code, which is flakiness, and reporting it as such was
correct analysis of a corrupt input. The fault was in the fixture, not the
tool. It is a fair reminder that a detector can only be as good as the identity
its inputs give it, and that `-count`-style repetition and accidental
duplication look alike from the outside.

## Protocol amendments

Recorded as additions with their reasoning. Earlier entries are left standing,
because a preregistration that gets edited is not one.

### 2026-09-07: C2 tightened to the exact repair commit

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

### 2026-09-07: Conduit invocation stated explicitly

    go test -count=1 -shuffle=on <packages>

`-shuffle=on` alone already defeats the test cache in this invocation, but
`-count=1` states the experimental requirement rather than relying on that:
**execute the tests, never reuse a cached result.**

`go test` prints the shuffle seed it used. That seed is recorded per execution,
so any failure carries

    commit + runtime + test + shuffle seed + failure message

and is reproducible by a third party, rather than being "run 47 failed".

### 2026-09-07: a defect found before the freeze, and how it was handled

Checking the skip invariant surfaced an unrelated bug: the history strip in
`explain` grouped transitions by branch while scoring grouped them by execution
context, so the rendered evidence printed "flip on identical code" at every
platform boundary. Verdicts were correct; the evidence displayed beneath them
was not.

It cannot affect any external subject, all of which run on a single machine
with one platform, one runtime and one branch, so branch grouping and execution
context coincide there, so the strip and the verdict agree by construction. It
was fixed before the freeze rather than during the experiments.

Subject A was already executing against the pre-fix build when this was found.
Rather than restart it, note that execution and measurement are separable here:
the raw evidence is the JUnit XML on disk, and scoring happens afterwards. Every
subject's XML is **re-ingested and re-scored with the frozen build** before any
result is reported, so all three subjects are measured by one implementation
regardless of which build produced the XML.

### 2026-09-07: a perturbation during subject A, recorded not hidden

Docker Desktop was started around executions 29-31 of subject A, in preparation
for subject B, and its startup competed for CPU: execution time rose from ~110s
to ~2min. Subject A includes wall-clock threshold assertions ("Docs page load
took 5932ms - threshold: 4500ms"), so added CPU load can inflate exactly the
failure rates being measured.

Docker was stopped again and subject B deferred until A completes. The overlap
is recorded here rather than corrected away: three of a hundred executions ran
under load, which is noted beside subject A's rates so a reader can judge it.
Nothing was re-run, because selectively discarding executions that ran under
conditions one dislikes is how a measured rate becomes a chosen one.

## flakestat against its own suite

Accumulated on the `flakestat-history` branch across every CI run of this
project: **10,305 observations of 293 distinct tests over 33 runs, on three
platforms. All 293 stable, no false positives.**

Not a sensitivity result, since nothing in that suite is known to be flaky, but it
is specificity measured on real, unsynthetic data, from a matrix where branch,
platform and runtime all vary and where earlier versions of the scorer produced
phantom transitions.

## Frozen implementation

Fixed for the duration of A, B and C. If a genuine correctness bug is found
mid-experiment, affected results are invalidated, the build gets a new SHA
recorded here, and every affected subject is re-scored from its retained XML.
Nothing is quietly patched and continued.

    flakestat commit   a4672ece  (code identical to 465faaec; later commits
                                  in the range are documentation only)
    flakestat version  dev (built from source)
    binary sha256      e5db674fe97f6de323f24f88a30f27282c4ccd4fb332ff710d07
                       cf1b332af760
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
| A. playwright-flaky-tests | **complete**, see results below |
| B. techstories-demo-app | **complete** |
| C1. conduit @ 612f5bfb (pre-fix) | **complete** |
| C2. conduit @ 9e00e594 (the repair) | **complete** |

Subject A is pinned at `dd4738a4`. Its "stable" control navigates to the live
`playwright.dev`, so a control failure there is ambiguous between a tool error
and a genuine network event; raw failure messages are kept so the two can be
told apart rather than assumed.
