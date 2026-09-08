# Flake hunt across third-party repositories

**Complete. Both unreported findings have since been filed with their
maintainers: [hashicorp/raft#711](https://github.com/hashicorp/raft/issues/711)
and [panjf2000/ants#401](https://github.com/panjf2000/ants/issues/401).**

Ran flakestat's validation protocol against active open-source Go projects to
answer one question the validation report leaves open: *can it surface a flaky
test nobody has already filed?*

## Protocol

Identical to validation subject C, so results are comparable:

    go test -count=3 -shuffle=<recorded seed>    fresh process per execution

`-count=3` because process-global-state flakes only surface with in-process
repetition, which is what Conduit's own flake-hunt job uses. `-shuffle` with a
seed chosen and recorded by the harness, so any failure handed to a maintainer
is reproducible rather than "run 47 failed". Scored with the frozen build
`a4672ece`. No repository was modified.

Every candidate was screened first for existing flake-hunting: all 33 workflow
files across the five repos were checked for `shuffle`, `count>1`, scheduled
test jobs and flake-named jobs.

## Results

| Repo | Executions | Distinct tests | Failing tests | Notes |
| --- | --- | --- | --- | --- |
| `benbjohnson/litestream` | 100 | 1,438 | 1 | rate 2/301 observations |
| `sourcegraph/conc` | 101 | 162 | 0 | clean |
| `charmbracelet/bubbletea` | 101 | 69 | 0 | clean |
| `hashicorp/raft` | 31 * | ~500 | 7 | flakiest suite by far |
| `panjf2000/ants` | 2 † | 65 | 1 | deterministic hang, see below |
| `etcd-io/bbolt` | not run | n/a | n/a | 606s baseline; the protocol would cost ~50h |

\* reduced from 100: baseline 131s, so `-count=3` costs ~6.5min per execution
and 100 would have consumed the whole window and starved the other repos.
† full-suite run abandoned at 2 executions, because each hang costs the
10-minute Go test timeout, and replaced with a targeted diagnostic.

## Every real flake found, and whether it was already known

| Finding | Rate | Already known? |
| --- | --- | --- |
| raft `TestRaft_HasExistingState` | 2/31 runs | **no, clean on four checks** |
| ants `TestAntsPool` hangs under `-count>1` | 15/15 | **no, clean on three checks** |
| raft `TestRaft_FollowerRemovalNoElection` | 4/31 | yes, open PR #669, unmerged since 2026-03-19 |
| raft `TestRaft_SendSnapshotFollower` | 2/31 | yes, issue #311, and listed in #372 |
| raft `TestRaft_ProtocolVersion_Upgrade_2_3` | 2/31 | yes, listed in #372 |
| raft `TestRaft_RecoverCluster` | 2/31 | yes, listed in #372 |
| raft `TestRaft_ProtocolVersion_Upgrade_1_2` | 1/31 | yes, listed in #372 |
| raft `TestNetworkTransport_AppendEntriesPipeline_CloseStreams` | 1/31 | yes, has prior issues |
| litestream `TestResumableReader_ContextCancelAbortsBackoff` | 2/301 | yes, open issue #1502 |

**Seven of nine were already filed.** That is the honest headline of this
exercise: projects with active maintainers usually already know. It is not a
failure of the tool, it is what the world looks like.

## The two unreported candidates

### `hashicorp/raft`: `TestRaft_HasExistingState`

    testing.go:705: peer mismatch
      expected  2 servers  [b3bfde62, c21909b3]
      observed  3 servers  [b3bfde62, c21909b3, 1a02a2fe]

Failed on runs 003 and 011 under independent seeds (`758137489003`,
`837114709011`). A third node appears in a two-node configuration.

Cross-checked at the time against four sources, all clean: issue search, PR
search, commit search, and raft's own community-collected flaky-test inventory
(issue #372, whose comments name twenty tests, and this was not among them).
The only issue that mentions it now is #711, which is mine.

**Diagnostic results, and two corrections to my own reasoning.**

    test alone,  -count=1                  25/25 pass
    test alone,  -count=3                  25/25 pass
    full suite,  -count=1, no shuffle      12/12 pass
    full suite,  -count=1, shuffled        12/12 pass
    full suite,  ./... -count=1            10/10 pass for this test
    full suite,  ./... -count=3, shuffled   2/31 FAIL

The failure needs the full suite **and** `-count=3`. Neither alone reproduces
it in 74 attempts.

I was wrong twice getting here. First I assumed repetition of the test itself
was leaking state, and tested it in isolation, where it is clean. Then I
assumed the trigger might be shuffle ordering, which arm B rules out.

**hashicorp/raft runs `-count=1` in CI, so their CI cannot hit this.** It is not
a flaky test they experience. It is the same category as the ants finding: the
package does not survive in-process repetition of the whole suite.


### `panjf2000/ants`: `TestAntsPool`

    -count=1    15 passed,  0 hung
    -count=2     0 passed,  5 hung   (all rc=124)
    -count=3     0 passed, 15 hung   (all rc=124)

Deterministic, and the trigger is repetition itself rather than anything
specific to three iterations: a single extra run of the test in the same
process is enough. Thirty-five attempts, no ambiguity.

The test binary times out at 10 minutes with `TestAntsPool` blocked; the
goroutine dump shows `testing.(*T).Run` waiting while several pools'
`purgeStaleWorkers` and `ticktock` janitors remain alive across iterations.

Cross-check clean at the time: no issue, no PR and no commit mentioned it.
The only one that does now is #401, which is mine.

**How to characterise it:** a test-hygiene bug, not a flaky test and not a
proven pool deadlock. It blocks `go test -count=N`, which is the standard way
to hunt real flakes, so the practical impact is that this project cannot use
the technique that finds genuine flakes. Same class as Conduit's *"make
entrypoint SIGTERM handling testable under `-count>1`"* fix.

## What flakestat itself did, precisely

Worth separating, because it is easy to overclaim:

1. Its **protocol** of repeated fresh processes, `-count=3`, shuffled, surfaced
   every finding here.
2. Its **scoring** correctly declined to classify the ants hang on two captured
   runs (`insufficient-data`, not `flaky`), which is the right answer on that
   evidence. A naive detector would have shouted about a 10-minute hang.
3. The **characterisation** of both candidates came from targeted follow-up
   experiments, not from the tool.

The tool contributed the sampling discipline and the restraint. It did not, on
its own, diagnose anything.

## Does this close the gap?

**No.** And the way it fails to is worth more than a weak yes.

Nine real flaky tests were found. Seven were already filed by their maintainers. The
two that were not, ants `TestAntsPool` and raft `TestRaft_HasExistingState`,
are **both** artefacts of running a suite with `-count>1`. Neither is a flaky
test that the project's own CI can encounter.

So the count of previously-unknown flaky tests found in the wild is **zero**.

### The protocol produced its own signal

The raft finding failed only under the exact conditions the hunt imposed:
`-count=3` across the full suite. That is not a bug their CI can hit; it is a
consequence of how the hunt was run.

This project has been here before. From DESIGN.md, on `--parallel`:

> The deeper lesson: the measurement apparatus was itself a source of signal,
> which is precisely the failure this project exists to prevent.

It happened again, in the hunt rather than the tool. Had I stopped after the
first cross-check came back clean, with an unreported test, a real intermittent
failure and a 9.1k-star library, I would have filed a bug report that misattributed the
cause, and told you the gap was closed.

The four experiments that prevented that cost about an hour.

### What can be said

- flakestat reliably surfaces real flakes in third-party code,
  including one with an unmerged fix open since March and one their nightly
  race sweep needed a week to catch.
- It stays quiet when it should: two repositories clean across 202 executions
  and 231 distinct tests, and it correctly refused to classify a dramatic
  10-minute hang on two observations.
- **Every genuinely flaky test it found was already known to its maintainers.**
  It has still never surfaced one that nobody had filed.

The honest claim for a launch post is the speed and the restraint, not
discovery. Anyone writing "found bugs in real projects" from this data would be
overstating it, and the third sentence of the FAQ would have to walk it back.

### Told the maintainers

Both `-count>1` findings are real, and both have the same practical
consequence: **the project cannot use `go test -count=N`**, which is the
standard way to find genuine flakes. Each was filed as what it is, a
test-hygiene limitation rather than a product bug, with the reproduction
attached and no claim about their CI.

| Finding | Filed as |
| --- | --- |
| raft `TestRaft_HasExistingState` under `-count>1` | [hashicorp/raft#711](https://github.com/hashicorp/raft/issues/711) |
| ants `TestAntsPool` hangs under `-count>1` | [panjf2000/ants#401](https://github.com/panjf2000/ants/issues/401) |

Neither changes the count above. They were unreported when found, and they
are still not flaky tests either project's CI can encounter.
