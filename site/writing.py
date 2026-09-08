#!/usr/bin/env python3
"""Long-form pages. Kept apart from the docs because they age differently."""


def post(BASE, REPO):
    return dict(
        slug="writing/validating-a-flaky-test-detector",
        section="Writing",
        layout="post",
        kicker="Essay",
        title="Validating a flaky-test detector",
        title_tag="Validating a flaky-test detector",
        og_title="Validating a flaky-test detector",
        description="Building a flaky-test detector took a week. Working out whether it actually worked took considerably longer, and produced better material.",
        keywords=["flaky test detection", "test validation", "flaky test scoring",
                  "junit xml", "go testing"],
        lede="",
        updated="8 September 2026",
        lastmod="2026-09-08",
        summary="A week to build it. Rather longer to work out whether it did "
                "anything, and three bugs that all had the same shape.",
        body=f"""
<article class="post">
<p class="post-meta">8 September 2026 · Rohit Swami</p>
<h1>Validating a flaky-test detector</h1>

<p class="post-standfirst">Building it took about a week. Working out whether it
actually worked took considerably longer, and produced better material.</p>

<p>I once spent an afternoon on a flaky test that wasn't flaky.</p>

<p>It sat at the top of a list ranked by failure rate, which seemed like a
sensible way to rank flaky tests until I noticed it had failed on every run for
three weeks. It wasn't nondeterministic. It was broken, and had been broken so
consistently that the ranking put it exactly where the most nondeterministic
test in the suite should have been.</p>

<p>That is the whole design argument for the tool I ended up writing, so I'll
state it once and move on: <strong>flakiness is inconsistency, not
failure</strong>. A test that fails every time tells you nothing about
nondeterminism. What you want to know is how often a test disagrees with
<em>itself</em>. Score that instead, and the always-failing test drops to zero
and gets filed under a different heading, where it belongs.</p>

<p>flakestat does that. It reads the JUnit XML your runner already emits, scores
transitions rather than failures, and attaches a confidence level to every
verdict, because 0.62 from three runs and 0.62 from three hundred are not the
same claim. One Go binary, no account, nothing leaves the machine.</p>

<p>That part was a week. This post is about the rest.</p>

<h2>The awkward question</h2>

<p>How do you know a flaky-test detector works?</p>

<p>It's a genuinely annoying question. You can write fixtures with planted
flakes, and I did. A test that fails on a coin flip is easy to detect and
proves almost nothing. Real flakiness is rarer, weirder, and entangled with the
environment. And the failure mode you care about is silent: a detector that
misses things looks identical to a codebase that isn't flaky.</p>

<p>What I wanted was a case where somebody else had established the ground
truth. Not my fixture, not my judgement.</p>

<p>ConduitIO had one. Their contributors filed an issue documenting several
flaky tests, named them, and later merged a commit that fixed them. That gives
two commits, one either side of a repair I had nothing to do with, and a
detector that works should say different things about them.</p>

<div class="fig" id="fig-ba">
  <div class="fig-head">Same protocol, two commits</div>
  <div class="fig-body">
    <div class="ba">
      <div class="ba-col pending">
        <h4>612f5bfb, before their fix</h4>
        <div class="ba-row"><span>TestClient_NotFound</span><b class="v-f">flaky 0.66</b></div>
        <div class="ba-row"><span>TestClient_CacheMiss</span><b class="v-f">flaky 0.66</b></div>
        <div class="ba-row"><span>TestClient_CacheHit</span><b class="v-f">flaky 0.66</b></div>
      </div>
      <div class="ba-arrow" aria-hidden="true">&rarr;</div>
      <div class="ba-col pending">
        <h4>9e00e594, their fix</h4>
        <div class="ba-row"><span>TestClient_NotFound</span><b class="v-s">stable 0.00</b></div>
        <div class="ba-row"><span>TestClient_CacheMiss</span><b class="v-s">stable 0.00</b></div>
        <div class="ba-row"><span>TestClient_CacheHit</span><b class="v-s">stable 0.00</b></div>
      </div>
    </div>
  </div>
  <div class="fig-note"><p>One commit apart. 60 of 90 observations failing on the
  left, none on the right.</p></div>
</div>

<p>You can run that yourself:</p>

<pre><code>git clone {REPO} &amp;&amp; cd flakestat
./scripts/reproduce-validation.sh</code></pre>

<p>Two minutes. It fetches their repository at both commits, runs the same
sampling protocol at each, and prints the table. It also runs in CI on every
build, so if their commits move or the script rots, it breaks on my end before
it breaks on yours.</p>

<p>I'd rather be judged on that than on anything I assert. The protocol was
<a href="{BASE}validation/">written down before any of it ran</a>, which is the
only way the results mean anything.</p>

<h2>Three things I got wrong</h2>

<p>The bugs were more instructive than the features, and they share a shape.</p>

<h3>The measurement was the signal</h3>

<p>I added a <code>--parallel</code> mode to run the suite several times at
once. Pointed at <code>spf13/cobra</code>, it reported
<code>TestDeadcodeElimination</code> as flaky at 0.60.</p>

<p>It isn't. The test builds a binary at a fixed path, and concurrent copies
were deleting each other's build. The tool had manufactured the exact phenomenon
it was built to detect, then reported it with a straight face.</p>

<p>Candidates found under <code>--parallel</code> are now re-run sequentially
and demoted if they don't reproduce. I kept the mode rather than deleting it,
because failing to reproduce in five runs is weak evidence of innocence, not
proof. But the default is sequential, and the default is trustworthy.</p>

<h3>Duplicates suppress flakiness</h3>

<p>CI artifacts get collected twice. A job re-runs, two aggregators pick up the
same shard, someone re-uploads. I assumed the risk was inflated confidence:
more observations agreeing, tighter bound, smaller p-value, all resting on
evidence that only existed once.</p>

<p>It does the opposite, and it took a measurement to notice.</p>

<p>A duplicate carries its original's timestamp. So it sorts <em>next to</em>
its original, and a duplicate always agrees with itself. Duplication doesn't
add noise, it adds artificial <em>agreement</em>. On a test failing 4 of 12
runs, ingesting the same reports twice moved the score from 0.64 to 0.30 while
the apparent evidence doubled.</p>

<div class="fig" id="fig-dup">
  <div class="fig-head">Drag to duplicate the same observations</div>
  <div class="fig-body">
    <div class="dup-controls">
      <label for="dup-range">Copies of each observation</label>
      <input id="dup-range" type="range" min="1" max="4" value="1" step="1">
      <span class="dup-count">none</span>
    </div>
    <div class="strip"></div>
  </div>
  <div class="gauge">
    <div><span>Observations</span><b id="dup-obs">12</b></div>
    <div><span>Score</span><b id="dup-score">0.64</b></div>
    <div><span>Verdict</span><b id="dup-verdict" class="flaky">flaky</b></div>
  </div>
  <div class="fig-note"><p id="dup-note"></p></div>
</div>

<p>The error runs towards false negatives. Duplication makes flaky tests look
stable, which is the direction that loses them quietly. Every observation now
carries the identity of the execution it describes, so a copy counts once. A
genuine retry, which really did run the tests again, still counts.</p>

<h3>The same invariant, three times</h3>

<p>Two outcomes are evidence of nondeterminism only if everything that could
legitimately change the result was held constant. Obvious once written down. I
broke it three times.</p>

<p>First branches: a test passing on <code>main</code> and failing on an
in-progress feature branch, read as one chronological series, looks like
repeated disagreement.</p>

<p>Then platforms. A test that always fails on Windows and always passes
everywhere else is perfectly deterministic on every platform. It scored 0.45,
with an explanation asserting "direct evidence of nondeterminism". That one
surfaced within minutes of pointing the tool at its own CI matrix.</p>

<p>Then the display layer, which kept grouping by branch after scoring had moved
on to execution context. The verdict was right; the evidence printed underneath
it was wrong, marking "flip on identical code" at exactly the boundaries the
verdict had ruled out. Arguably worse than being wrong outright, since the
number a careful reader checks against was the one still misbehaving.</p>

<p>There is now one definition of comparability and everything that displays
transition evidence calls it. Three implementations that agreed by convention
was the actual bug.</p>

<h2>A night spent failing to find something</h2>

<p>One hole I couldn't argue away: every flaky test the tool had found was
already known to somebody. Conduit's were filed. So were the ones in the
regression corpus. A detector that only rediscovers documented bugs is a
plausible detector, not a demonstrated one.</p>

<p>So I pointed it at six active Go projects overnight. Five finished the
protocol: 335 executions, each a fresh process, shuffled, with a recorded seed
so anything found would be reproducible by its maintainer rather than "run 47
failed". The sixth had a ten-minute baseline and would have taken fifty
hours.</p>

<p>It found nine real flaky tests.</p>

<p><strong>Seven were already filed.</strong> One had a fix open and unmerged
since March. One had been caught by litestream's nightly race-detector sweep the
week before. flakestat hit it in ten runs, which is a reasonable argument for
concentrated repetition over nightly sampling, and not an argument for
discovery.</p>

<p>The two that weren't known looked promising for about an hour. One was an
unreported intermittent failure in a 9.1k-star consensus library, clean against
four separate cross-checks: no issue, no pull request, no commit, and absent
from the project's own community-maintained list of flaky tests.</p>

<p>Then I ran the experiment that mattered.</p>

<pre><code>test alone,  -count=1                  25/25 pass
test alone,  -count=3                  25/25 pass
full suite,  -count=1, no shuffle      12/12 pass
full suite,  -count=1, shuffled        12/12 pass
full suite,  ./... -count=1            10/10 pass
full suite,  ./... -count=3, shuffled   2/31 FAIL</code></pre>

<p>It only failed under the exact conditions my harness imposed. Their CI runs
<code>-count=1</code>. The other unreported finding turned out to be the same
category: a package that can't be run with <code>-count&gt;1</code> at all,
which is a real problem worth telling them about and is not flakiness.</p>

<p>So: <strong>zero previously-unknown flaky tests.</strong></p>

<p>Stopping there was tempting. It was four in the morning and the result was
exactly what I had been hoping for. Had I stopped, I would have filed a bug
report that misattributed its own cause, and written a post claiming the tool
found something nobody knew about.</p>

<p>That's the <code>--parallel</code> mistake again. Same shape, different
layer: the apparatus producing the signal. I've now made it twice in one
project, which suggests it's less a bug than a standing hazard of measuring
anything.</p>

<h2>What I'll claim</h2>

<p>Two things the evidence supports.</p>

<p><strong>Speed.</strong> Concentrated repetition beats nightly sampling.
Litestream's sweep needed a week of nightly runs to surface a flake; ten runs on
a laptop found it.</p>

<p><strong>Restraint.</strong> Across two clean repositories, 202 executions
and 231 distinct tests, it reported nothing at all. It refused to classify a
dramatic ten-minute hang on two observations, correctly, because two
observations aren't a verdict. It called nine always-failing tests broken rather
than flaky. A detector that finds something everywhere you point it is just a
mirror.</p>

<p>What I won't claim is discovery. It has never surfaced a flaky test nobody
had already filed, and <a href="{BASE}findings/">the write-up</a> says so in the
same words I'd use if it had.</p>

<p>I think that's the more useful post anyway. There is a lot of tooling that
tells you what it found. There is less that tells you what it looked for and
missed.</p>

<hr>

<p class="post-foot">
flakestat is MIT-licensed and reads the JUnit XML your test runner already
writes.
<code>brew install rowhitswami/tap/flakestat</code>,
<code>npm install --save-dev flakestat</code> or
<code>pip install flakestat</code>.
<a href="{BASE}docs/">Documentation</a> ·
<a href="{REPO}">Source</a> ·
<a href="{BASE}validation/">Full validation record</a> ·
<a href="{BASE}findings/">The overnight hunt</a>
</p>
</article>
""")


def index(BASE, REPO, entries):
    """The one page that lists everything long-form.

    Separate nav entries for each record turned the header into a filing
    cabinet. One list, ordered newest first, does the same job.
    """
    items = []
    for e in entries:
        meta = [f'<span>{e["kicker"]}</span>']
        if e.get("updated"):
            meta.append(f'<span>{e["updated"]}</span>')
        items.append(
            f'<li class="wr-item"><a href="{BASE}{e["slug"]}/">'
            f'<p class="wr-kicker">{"".join(meta)}</p>'
            f'<h2>{e["title"]}</h2>'
            f'<p class="wr-sum">{e["summary"]}</p>'
            f'<span class="wr-go">Read<i></i></span>'
            f"</a></li>"
        )

    return dict(
        slug="writing",
        section="Writing",
        layout="index",
        # The index is only as fresh as the newest thing on it.
        lastmod=max((e.get("lastmod") or "") for e in entries),
        title="Writing",
        title_tag="Writing about flaky tests and flakestat",
        og_title="Writing",
        description="Essays, validation records and design notes from building "
                    "flakestat, an open-source flaky test detector.",
        keywords=["flaky test blog", "flaky test detection writing",
                  "flakestat validation", "flakestat design notes"],
        lede="",
        body=f"""
<div class="wr-wrap">
  <header class="wr-head">
    <p class="wr-eyebrow">Writing</p>
    <h1>How it was built,<br>and what it failed to find.</h1>
    <p>Essays and records from building flakestat.</p>
  </header>
  <ol class="wr-list">{"".join(items)}</ol>
</div>
""")
