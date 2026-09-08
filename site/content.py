#!/usr/bin/env python3
"""Page content for the flakestat documentation site."""
import html


def cb(code, title=None, lang="bash"):
    """One code block with a copy button."""
    head = ""
    if title:
        head = f'<div class="cblock-head"><span class="cblock-title">{html.escape(title)}</span>' \
               f'<button class="copy" type="button">Copy</button></div>'
    body = f'<pre><code>{code if "<span" in code else html.escape(code)}</code></pre>'
    if head:
        return f'<div class="cblock">{head}{body}</div>'
    return f'<div class="cblock"><button class="copy" type="button">Copy</button>{body}</div>'


def tabs(items, title=None):
    """Tabbed code block: [(label, lang, code), ...]."""
    ts, ps = [], []
    for i, (label, lang, code) in enumerate(items):
        sel = "true" if i == 0 else "false"
        ts.append(f'<button class="tab" role="tab" aria-selected="{sel}" data-lang="{lang}">{html.escape(label)}</button>')
        ps.append(f'<div class="cpane"{"" if i == 0 else " hidden"}><pre><code>{html.escape(code)}</code></pre></div>')
    t = f'<span class="cblock-title">{html.escape(title)}</span>' if title else ""
    return ('<div class="cblock" data-tabs><div class="cblock-head">' + t +
            '<div role="tablist">' + "".join(ts) + '</div>'
            '<button class="copy" type="button">Copy</button></div>' + "".join(ps) + "</div>")


def tbl(headers, rows, wrap=True):
    h = "".join(f"<th>{c}</th>" for c in headers)
    b = "".join("<tr>" + "".join(f"<td>{c}</td>" for c in r) + "</tr>" for r in rows)
    t = f"<table><thead><tr>{h}</tr></thead><tbody>{b}</tbody></table>"
    return f'<div class="tablewrap">{t}</div>' if wrap else t


INSTALL_TABS = tabs([
    ("Homebrew", "brew", "brew install rowhitswami/tap/flakestat"),
    ("npm", "npm", "npm install --save-dev flakestat\n# or, without installing:\nnpx flakestat report"),
    ("pip", "pip", "pip install flakestat"),
    ("Go", "go", "go install github.com/rowhitswami/flakestat/cmd/flakestat@latest"),
    ("Script", "sh", "curl -sSfL https://raw.githubusercontent.com/rowhitswami/flakestat/main/scripts/install.sh \\\n  | sh -s -- -b /usr/local/bin"),
    ("Docker", "docker", 'docker run --rm -v "$PWD:/workspace" ghcr.io/rowhitswami/flakestat report'),
], title="install")

HUNT_TABS = tabs([
    ("pytest", "pytest", "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- pytest --junitxml='{junit}'"),
    ("Jest", "jest", "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- npx jest --reporters=jest-junit"),
    ("go test", "go", "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- gotestsum --junitfile='{junit}' -- ./..."),
    ("Maven", "maven", "flakestat hunt --runs 20 --junit 'target/surefire-reports/*.xml' \\\n  -- mvn -q test"),
], title="hunt for flaky tests")


def pages(BASE, REPO, ACTION_REF='rowhitswami/flakestat@v0.2.0'):
    P = []

    # ══════════════════════════════════════════════════════════════ landing
    P.append(dict(
        slug="", section="Home", layout="wide",
        title="flakestat",
        title_tag="flakestat: find flaky tests in any language",
        og_title="flakestat, a flaky test detector",
        description="Open-source flaky test detector for pytest, Jest, go test, JUnit and anything that writes JUnit XML. One static binary, no SaaS, no account, no data egress.",
        keywords=["flaky test detector", "find flaky tests", "flaky test detection",
                  "open source flaky tests", "pytest flaky tests", "jest flaky tests",
                  "go test flaky", "junit xml flaky tests", "ci flaky tests"],
        lede="",
        body=f"""
<section class="hero">
  <svg class="mesh" aria-hidden="true" width="100%" height="100%">
    <defs><pattern id="n" width="54" height="54" patternUnits="userSpaceOnUse">
      <circle cx="27" cy="27" r="2.4" fill="#4D8DFF" opacity=".30"/></pattern></defs>
    <rect width="100%" height="100%" fill="url(#n)"/>
  </svg>
  <div class="hero-in">
    <div>
      <img class="hero-lockup" src="{BASE}assets/logo-lockup-dark.png" alt="flakestat" width="400" height="126">
      <h1>Some tests disagree<br>with <span class="odd">themselves</span>.</h1>
      <p class="sub">flakestat reads the JUnit XML your test runner already writes and tells you
      which tests are actually flaky, ranked, with a confidence level, and with the evidence
      behind every verdict.</p>
      <div class="hero-cta">
        <a class="btn btn-pri" href="{BASE}docs/#quickstart">Get started</a>
        <a class="btn btn-ghost" href="{REPO}">View on GitHub</a>
      </div>
    </div>
    <div class="cblock">
      <div class="cblock-head"><span class="cblock-title">flakestat report</span>
      <button class="copy" type="button">Copy</button></div>
<pre><code><span class="t-dim">VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST</span>
<span class="t-flaky">flaky                  0.62  high    20       14/6  test_demo::test_flaky_race</span>
<span class="t-ok">consistently-failing   0.00  high    20       0/20  test_demo::test_broken</span>

<span class="t-dim">1 flaky, 0 suspect, 1 consistently failing,
1 stable, 1 unscored (of 4 tests)</span></code></pre>
    </div>
  </div>
</section>

<div class="band"><div class="band-in">
  <div><b>Local first</b><p>One static binary. No account, no server, no test data leaving your machine.</p></div>
  <div><b>Any language</b><p>If your runner writes JUnit XML, and they all do, flakestat reads it.</p></div>
  <div><b>Free at any volume</b><p>Recording CI runs costs no extra compute. No per-seat or per-run pricing.</p></div>
  <div><b>Zero dependencies</b><p>The Go module has no <code>require</code> block. Nothing to audit but the tool.</p></div>
</div></div>

<div class="wide narrow">
  <h2>Install and run in two commands</h2>
  {INSTALL_TABS}
  {HUNT_TABS}
  <p>That runs your suite 20 times on unchanged code and reports what disagreed.
  Already have CI? <a href="{BASE}docs/#history">Record the runs you already pay for</a> instead:
  it catches environment-dependent flakes a local burst never will, at no extra compute.</p>

  <h2>Why the verdicts can be trusted</h2>
  <div class="cards">
    <div class="card"><h4>Inconsistency, not failure</h4>
      <p>A test failing 100% of the time isn't flaky, it's broken. flakestat scores
      pass→fail→pass transitions, so a broken test scores zero and is reported separately.</p></div>
    <div class="card"><h4>Evidence is weighed</h4>
      <p>Disagreement on the same commit is proof; across commits it may be a regression
      someone already fixed, so it counts for less.</p></div>
    <div class="card"><h4>Confidence, not just a number</h4>
      <p>A score of 0.62 from three runs and from three hundred are different claims. A small
      sample can never reach <code>high</code>.</p></div>
    <div class="card"><h4>Only comparable things compared</h4>
      <p>Two outcomes are evidence of nondeterminism only if branch, platform and runtime were
      held constant.</p></div>
  </div>
  <p><a href="{BASE}docs/#scoring">See exactly how scoring works</a>, including a live demo you
  can poke at.</p>

  <h2>Start where you are</h2>
  <div class="cards">
    <a class="card" href="{BASE}flaky-tests/pytest/"><h4>pytest →</h4><p>Find flaky tests in a Python suite.</p></a>
    <a class="card" href="{BASE}flaky-tests/jest/"><h4>Jest →</h4><p>Find flaky tests in a JS or TS suite.</p></a>
    <a class="card" href="{BASE}flaky-tests/go/"><h4>go test →</h4><p>Find flaky tests in a Go suite.</p></a>
    <a class="card" href="{BASE}docs/#github-actions"><h4>GitHub Actions →</h4><p>Record every CI run and gate on regressions.</p></a>
  </div>
</div>
"""))

    # ══════════════════════════════════════════════════════════════ install
    P.append(dict(
        slug="docs/install", section="Getting started",
        title="Install flakestat",
        title_tag="Install flakestat with Homebrew, npm, pip, Go or Docker",
        description="Install flakestat via Homebrew, npm, pip, Go, a shell script or Docker. Same static binary on macOS, Linux and Windows, x86_64 and arm64.",
        keywords=["install flakestat", "flakestat homebrew", "flakestat npm", "flakestat pip"],
        lede="Same binary whichever route you pick. No runtime to install alongside it.",
        body=f"""
{INSTALL_TABS}
<p>Or download a binary directly from <a href="{REPO}/releases">Releases</a>. macOS, Linux and
Windows, on x86_64 and arm64. The binary is statically linked and has no runtime dependencies.</p>

<h2>Verify the install</h2>
{cb("flakestat version\nflakestat --help")}

<h2>Which install should I use?</h2>
{tbl(["Route", "Best when"], [
  ["<code>brew</code>", "You work on macOS or Linux and want it on your PATH globally."],
  ["<code>npm</code>", "The project is JS or TS. Pins the version in <code>package.json</code> so CI and laptops match."],
  ["<code>pip</code>", "The project is Python. Same benefit: the version is pinned with your other dev tooling."],
  ["<code>go install</code>", "You have Go and want to build from source."],
  ["Script", "CI images without a package manager. Pin with <code>-b</code> and a version tag."],
  ["Docker", "You'd rather not install anything. Mount the repo at <code>/workspace</code>."],
])}

<div class="callout"><b>Pin the version in CI</b>
A floating <code>latest</code> means a scoring change can move your verdicts between builds.
The <a href="{BASE}docs/#github-actions">GitHub Action</a> takes a <code>version</code> input,
and the install script takes a tag.</div>

<h2>Next</h2>
<p>Head to the <a href="{BASE}docs/#quickstart">quickstart</a>. Two commands, and you'll know
whether your suite is flaky.</p>
"""))

    # ═══════════════════════════════════════════════════════════ quickstart
    P.append(dict(
        slug="docs/quickstart", section="Getting started",
        title="Quickstart",
        title_tag="How to find flaky tests: the flakestat quickstart",
        description="Two ways to detect flaky tests: run your suite N times locally with hunt, or record CI runs over time with ingest. Working examples for pytest, Jest and go test.",
        keywords=["how to find flaky tests", "detect flaky tests", "flaky test tutorial",
                  "flakestat quickstart"],
        lede="Two ways to use flakestat. They answer different questions, and most projects end up doing both.",
        body=f"""
<h2>1. Hunt: is this suite flaky right now?</h2>
<p>Runs your test command N times on unchanged code and reports what disagreed. Use it when you
already suspect a suite, or before opening a pull request.</p>
{HUNT_TABS}
<p><code>{{run}}</code> becomes the run number so each run writes its own report, and
<code>{{junit}}</code> becomes that path inside your command. Everything after <code>--</code> is
your command, passed through untouched.</p>

<h3>Let it detect your setup</h3>
{cb("flakestat init      # writes .flakestat.json\nflakestat hunt      # no flags needed")}
<p><code>init</code> looks at your project and writes the command and report path once, so the
day-to-day invocation is a single word. See <a href="{BASE}docs/#config">configuration</a>.</p>

<h2>2. Ingest: which tests are flaky over time?</h2>
<p>Records the CI runs you already pay for. This is the one that catches flakes that only appear
on one platform, one runtime, or under CI load, and it costs no extra compute.</p>
{cb("# in CI, after your tests run\nflakestat ingest 'reports/**/*.xml'\n\n# any time\nflakestat report --top 20")}
<p>See <a href="{BASE}docs/#history">tracking flakiness over time</a> for where to keep the
history, and <a href="{BASE}docs/#github-actions">GitHub Actions</a> for a complete workflow.</p>

<div class="callout"><b>Which should I use?</b>
Hunt when you need an answer in the next ten minutes. Ingest when you want a reliable picture of
the whole suite. They write to the same history, so using both just gives you more evidence.</div>

<h2>What you get back</h2>
{cb("""VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST
flaky                  0.62  high    20       14/6  test_demo::test_flaky_race
suspect                0.08   low     6        5/1  test_api::test_timeout
consistently-failing   0.00  high    20       0/20  test_demo::test_broken""", title="flakestat report")}
<p>Read the columns in <a href="{BASE}docs/#reading-a-report">reading a report</a>, or ask about
one test with <a href="{BASE}docs/#explain"><code>flakestat explain</code></a>.</p>

<h2>If it finds nothing</h2>
<p>That is a real result, not a failure. It means nothing disagreed in that many runs. A test that
fails 2% of the time will usually survive 20 runs untouched. Raise <code>--runs</code>, or record
CI history where the sample grows for free.</p>
"""))

    # ════════════════════════════════════════════════════ reading a report
    P.append(dict(
        slug="docs/reading-a-report", section="Getting started",
        title="Reading a report",
        title_tag="Reading a flakestat report: verdicts, score and confidence",
        description="What each flakestat verdict means, why score is not a failure rate, and how confidence is derived from the amount of evidence behind a verdict.",
        keywords=["flaky test score", "flaky test verdict", "flakestat report"],
        lede="Six verdicts, one score and one confidence level. Here is what each of them is claiming.",
        body=f"""
{cb("""VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST
flaky                  0.62  high    20       14/6  test_demo::test_flaky_race
suspect                0.08   low     6        5/1  test_api::test_timeout
consistently-failing   0.00  high    20       0/20  test_demo::test_broken""")}

<h2>Verdicts</h2>
<div class="verdict v-flaky"><b>flaky</b><span>Disagrees with itself often enough, with enough evidence, to act on.</span></div>
<div class="verdict v-suspect"><b>suspect</b><span>Some disagreement, below the flaky threshold. Worth watching, not worth a ticket yet.</span></div>
<div class="verdict v-stable"><b>stable</b><span>No meaningful disagreement.</span></div>
<div class="verdict v-other"><b>consistently-failing</b><span>Fails every time. Broken, not flaky, and a different problem with a different fix.</span></div>
<div class="verdict v-other"><b>insufficient-data</b><span>Fewer runs than needed for a verdict. More runs would help.</span></div>
<div class="verdict v-other"><b>always-skipped</b><span>Never actually ran. More runs would <em>not</em> help, so this is deliberately not the same as insufficient data.</span></div>

<h2>Score is not a failure rate</h2>
<p><strong>Score</strong> is how often a test disagrees with itself: 0.62 means roughly six of ten
consecutive runs changed their answer. A test that fails every single time scores
<code>0.00</code>, because its outcome never disagrees with itself. It is broken, and it shows up
as <code>consistently-failing</code> so you can fix it as the different problem it is.</p>

<div class="callout"><b>Why this matters</b>
A failure-rate ranking puts your most broken test at the top of the flaky list, where it wastes
the time of whoever is hunting nondeterminism. Ranking by inconsistency puts the genuinely
nondeterministic tests there instead.</div>

<h2>Confidence</h2>
<p><strong>Confidence</strong> is <code>low</code>, <code>medium</code> or <code>high</code>
depending on how much evidence sits behind the score. A score of 0.62 from three runs and the same
score from three hundred are very different claims, and a small sample can never reach
<code>high</code> no matter how flaky the test looks.</p>
<p>Classification uses a <em>lower bound</em> on the score rather than the score itself, so a
verdict requires evidence rather than a lucky flip in a short run. The mechanics are in
<a href="{BASE}docs/#scoring">how scoring works</a>.</p>

{tbl(["Column", "Meaning"], [
  ["<code>SCORE</code>", "Weighted rate at which consecutive runs disagreed."],
  ["<code>CONF</code>", "How much evidence supports that score."],
  ["<code>RUNS</code>", "Scored observations. Skips are excluded, because a skip is not evidence either way."],
  ["<code>PASS/FAIL</code>", "Counts among scored runs."],
  ["<code>TEST</code>", "Stable identity, from suite, class and name."],
])}

<h2>Other output formats</h2>
{cb("flakestat report --format json      # for tooling\nflakestat report --format markdown  # for a PR comment or wiki\nflakestat report --all              # include stable and unscored tests\nflakestat report --top 20           # worst 20 only")}
"""))

    # ═══════════════════════════════════════════════════════════════ hunting
    P.append(dict(
        slug="docs/hunting", section="Guides",
        title="Hunting flaky tests locally",
        title_tag="Reproduce a flaky test locally with flakestat hunt",
        description="Use flakestat hunt to run a test suite many times on unchanged code and find which tests disagree, including how many runs a given flake rate needs.",
        keywords=["reproduce flaky test", "rerun tests to find flaky", "flakestat hunt"],
        lede="Run the suite many times on unchanged code and see what disagrees.",
        body=f"""
{HUNT_TABS}

<h2>How many runs do I need?</h2>
<p>It depends entirely on how rare the flake is. These are measured against synthetic tests with
known failure probabilities:</p>
{tbl(["Flake rate", "Runs for reliable detection"], [
  ["25% or higher", "20 runs is plenty"],
  ["10%", "50 or more"],
  ["5%", "100+, and a local burst is starting to be the wrong instrument"],
  ["Below 5%", "Record <a href='" + BASE + "docs/#history'>CI history</a> instead"],
])}

<h2>Chasing one test</h2>
{cb("flakestat hunt --runs 50 --junit 'reports/junit-{run}.xml' \\\n  -- pytest tests/test_checkout.py::test_race --junitxml='{junit}'")}
<p>Narrowing to one test is usually much faster per run, so you can afford far more runs, which
is exactly what a rare flake needs.</p>

<h2>If it finds nothing</h2>
<p>That is a real result. It means nothing disagreed in that many runs, which is evidence the test
is not flaky <em>at a rate this sample could detect</em>. It is not evidence the test is clean.
Raise <code>--runs</code>, or let CI history accumulate.</p>

<div class="callout flag"><b>A warning about --parallel</b>
<p><code>--parallel N</code> runs N copies of your command <em>in the same working directory</em>.
A suite that writes fixed paths, binds a fixed port or shares a database will collide with itself
and look flaky when it isn't.</p>
<p>This is real, not theoretical. Running against <code>spf13/cobra</code> with
<code>--parallel 4</code> reported <code>TestDeadcodeElimination</code> as flaky at 0.60. It isn't.
the test builds a binary at a fixed path, and concurrent copies deleted each other's build.
Sequentially, cobra is completely clean.</p>
<p>So flakestat verifies its own findings. Candidates found under <code>--parallel</code> are
re-run sequentially and demoted from <code>flaky</code> to <code>suspect</code> if they don't
reproduce. Sequential (the default) is always trustworthy. Use <code>--parallel</code> to hunt
faster and let verification sort out the difference, or <code>--verify 0</code> to opt out.</p></div>

<h2>Scoring the whole history instead</h2>
<p>By default <code>hunt</code> scores only the burst it just ran. Add <code>--history</code> to
score everything recorded so far, burst and CI runs together.</p>
{cb("flakestat hunt --runs 20 --history")}
"""))

    # ═══════════════════════════════════════════════════════════════ history
    P.append(dict(
        slug="docs/history", section="Guides",
        title="Tracking flakiness over time",
        title_tag="Track flaky tests over time in CI with flakestat ingest",
        description="Record every CI run with flakestat ingest to measure flakiness over time, and choose where to keep the history: committed, an artifact, or a dedicated branch.",
        keywords=["track flaky tests over time", "flaky test history", "flakestat ingest",
                  "ci flaky test tracking"],
        lede="A burst proves flakiness exists. History measures it, and catches flakes a local run never will.",
        body=f"""
{cb("flakestat ingest 'reports/**/*.xml' \\\n  --commit \"$GIT_SHA\" --branch \"$BRANCH\"")}
<p>Commit and branch default to the current git checkout, so in most CI setups you can omit them.
Recording the commit is what lets flakestat tell genuine flakiness, same code and different result,
from a regression someone later fixed.</p>

<p>History lives in <code>.flakestat/runs.ndjson</code>, one JSON object per line. Because it is
append-only NDJSON, results from parallel CI shards concatenate with <code>cat</code> and no merge
step.</p>

<h2>Re-ingesting is safe</h2>
<p>Every observation carries the identity of the execution it describes, so an artifact uploaded
twice, a re-run aggregation step, or a shard collected by two jobs is counted once, while a
genuine retry, which really did run the tests again, still counts.</p>

<div class="callout flag"><b>Duplicates are not harmless</b>
A duplicate carries its original's timestamp, sorts next to it, and always agrees with itself, so
uncounted duplicates make a flaky test look <em>stable</em>. Measured on a test failing 4 of 12
runs, ingesting the same reports twice moved the score from 0.64 to 0.30. The error runs towards
false negatives, which is the direction that loses tests quietly.</div>

<p>Copies are ignored on read, so nothing is required of you.
<code>flakestat compact</code> removes them from the file as well, which is worth doing when the
file is the record you keep.</p>

<h2>Where to keep the history</h2>
<div class="cards">
  <div class="card"><h4>Commit it</h4><p>Simplest. The file goes in the repo and everyone shares
  one history. Downside: every CI run wants to write to it, so telemetry lands in your development
  history.</p></div>
  <div class="card"><h4>A CI artifact</h4><p>Zero setup, but nothing accumulates. Each run sees
  only itself, so you never build the long history that makes the tool accurate.</p></div>
  <div class="card pick"><span class="tag">Recommended</span><h4>A dedicated branch</h4>
  <p>What flakestat uses for itself. Keeps telemetry out of development history, avoids a bot
  commit retriggering your workflow, and gives one place to serialize writers.</p></div>
</div>

{cb("""matrix jobs → per-job NDJSON artifacts → aggregate job
  → fetch history branch → merge + compact → analyze
  → commit back to the history branch""", title="the arrangement")}

{cb("concurrency:\n  group: flakestat-history-writer\n  cancel-in-progress: false", title=".github/workflows/ci.yml")}

<div class="callout flag"><b>Not a CI cache</b>
Cache eviction should cost you time, not statistical history. Keep the record in durable storage
and let the cache stay an optimization.</div>

<p>flakestat's own <a href="{REPO}/blob/main/.github/workflows/ci.yml">CI workflow</a> implements
this end to end, including re-merging a push that lost a race against another run.</p>

<h2>Trimming the file</h2>
{cb("flakestat compact             # drop duplicate executions\nflakestat compact --dry-run   # report what would go, change nothing")}
<p><code>compact</code> refuses to rewrite a log containing unreadable lines, since reading skips
those and rewriting would delete evidence you might still recover.</p>
"""))

    # ════════════════════════════════════════════════════════════ dimensions
    P.append(dict(
        slug="docs/dimensions", section="Guides",
        title="Recording where tests ran",
        title_tag="Find where flaky tests concentrate: platform, runtime, shard",
        description="Attach platform, runtime and CI context to observations with --dimension, and see where failures concentrate, without flakestat ever claiming causation.",
        keywords=["flaky test only on windows", "flaky test by platform", "flaky test correlation"],
        lede="Attach context to observations and flakestat will tell you where failures concentrate.",
        body=f"""
{cb("flakestat ingest 'reports/**/*.xml' \\\n  --dimension os=windows \\\n  --dimension runtime.version=3.13 \\\n  --dimension database=postgres-17")}

{cb("""Where the failures concentrate

  os=windows
    failures here:  12 / 12 (100.0%)
    elsewhere:      0 / 24 (0.0%)
    difference:     +100.0 pp""", title="flakestat explain")}

<h2>Two rules that keep it honest</h2>
<ul>
  <li><strong>Nothing is scraped.</strong> Only a whitelist of known CI variables is read, and only
  JUnit <code>&lt;property&gt;</code> names flakestat recognizes. The process environment is never
  walked, so secrets, tokens and build ids cannot end up in your history.</li>
  <li><strong>Host details are recorded only when flakestat ran the tests.</strong> If one job
  downloads other jobs' artifacts and ingests them centrally, pass <code>--no-host</code>:
  otherwise the aggregator's platform is stamped onto results from everywhere else, and analysis
  can conclude the exact opposite of the truth.</li>
</ul>

<h2>Canonical keys</h2>
{tbl(["Key", "Meaning"], [
  ["<code>os</code>, <code>arch</code>", "Platform the tests ran on"],
  ["<code>runtime.name</code>, <code>runtime.version</code>", "Language runtime under test"],
  ["<code>ci.provider</code>", "github, gitlab, circleci, buildkite, jenkins, azure"],
  ["<code>ci.run_id</code>, <code>ci.job_id</code>", "Provenance. Recorded, never analysed, since every failure happened during <em>some</em> run"],
  ["<code>ci.attempt</code>", "Separates a retry from a duplicate. Never analysed: retries happen <em>because</em> of failures"],
  ["<code>ci.shard</code>, <code>ci.worker</code>", "Which shard or worker executed the run"],
])}
<p>Anything else you pass is a user dimension and is analysed normally: database version, browser,
feature flag, region.</p>

<h2>Correlation, never causation</h2>
<p>flakestat says failures <em>cluster</em> on Windows. It will not claim Windows is why.
Observational data cannot distinguish a cause from anything perfectly correlated with it, and when
several dimensions vary together, as <code>os</code> and <code>arch</code> usually do on a CI
matrix, it reports all of them and says the observations cannot tell which one matters.</p>

<p>Three guards stop it producing confident nonsense over sparse data: an evidence floor, an
effect-size floor, and a Benjamini–Hochberg correction for how many dimensions were tested. Testing
os, arch, runtime, browser and database will eventually turn up something that looks significant
purely by chance.</p>
"""))

    # ═══════════════════════════════════════════════════════════════ ci gate
    P.append(dict(
        slug="docs/ci-gate", section="Guides",
        title="Gating CI without a permanently red build",
        title_tag="Fail CI on new flaky tests only, with flakestat check",
        description="flakestat check is a ratchet: accept today's flakiness in a committed baseline, then fail the build only when a test becomes newly flaky or measurably worse.",
        keywords=["fail ci on flaky tests", "flaky test baseline", "flaky test gate"],
        lede="Accept today's flakiness, then fail only on what is new or measurably worse.",
        body=f"""
<p>Most repos already have flaky tests when they adopt a tool like this. Failing on <em>all</em> of
them means a red build on day one, and a deleted gate by day three. So <code>check</code> is a
ratchet rather than a threshold.</p>

{cb("flakestat check --update-baseline    # once; commit .flakestat/baseline.json\nflakestat check --fail-on-new        # in CI from then on")}

{tbl(["Exit code", "Meaning"], [
  ["<code>0</code>", "Nothing new"],
  ["<code>1</code>", "A test became flaky that wasn't in the baseline"],
  ["<code>2</code>", "An accepted test got measurably worse (<code>--fail-on-regression</code>)"],
])}

<h2>Keeping jitter out of your builds</h2>
<p>Scores move a little as evidence accumulates. <code>--regression-delta</code> (default
<code>0.1</code>) is how much an accepted test has to worsen before it counts as a regression, so
ordinary drift doesn't fail builds.</p>

<h2>Tightening the ratchet</h2>
<p>Tests that stop being flaky are reported as <code>FIXED</code>. That is your cue to re-run
<code>--update-baseline</code> so the newly clean test can never silently regress.</p>

{cb("""FIXED   test_api::test_timeout      was 0.31, now 0.00
NEW     test_cart::test_checkout    0.42 (high)

1 newly flaky test. Baseline has 4 accepted.""", title="flakestat check")}

<div class="callout"><b>Order matters in CI</b>
Ingest before you gate, and let the test step continue on error. A failed suite is exactly the run
you most want recorded. See <a href="{BASE}docs/#github-actions">GitHub Actions</a> for the
full sequence.</div>
"""))

    # ════════════════════════════════════════════════════════════ quarantine
    P.append(dict(
        slug="docs/quarantine", section="Guides",
        title="Unblocking the pipeline",
        title_tag="Quarantine flaky tests: skip lists for any runner",
        description="Generate a skip list your test runner already understands, so flaky tests stop blocking merges while you work the list down.",
        keywords=["quarantine flaky tests", "skip flaky tests", "pytest deselect flaky"],
        lede="Emit a skip list your runner already understands, so merges are unblocked while you fix things.",
        body=f"""
{tabs([
  ("pytest", "pytest", "flakestat quarantine --format pytest -o quarantine.txt\npytest @quarantine.txt"),
  ("Jest", "jest", "flakestat quarantine --format jest -o quarantine.json"),
  ("go test", "go", "flakestat quarantine --format go"),
  ("YAML", "yaml", "flakestat quarantine --format yaml -o quarantine.yaml"),
], title="quarantine")}

{tbl(["Flag", "Effect"], [
  ["<code>--format</code>", "<code>pytest</code>, <code>go</code>, <code>jest</code>, <code>yaml</code> (default) or <code>json</code>"],
  ["<code>--include-broken</code>", "Also list consistently failing tests"],
  ["<code>-o</code>", "Write to a file instead of stdout"],
  ["<code>--quiet</code>", "Suppress the summary line on stderr"],
])}

<div class="callout flag"><b>A tourniquet, not a cure</b>
Everything in the file is something to fix. Quarantine buys you a working pipeline while you work
the list down; it does not make the tests correct. Keep the file in review so it cannot grow
quietly.</div>

<h2>A workable rhythm</h2>
<ol>
  <li>Quarantine what is blocking merges today.</li>
  <li>Commit a <a href="{BASE}docs/#ci-gate">baseline</a> so nothing <em>new</em> gets in.</li>
  <li>Fix the worst-ranked test, remove it from quarantine, update the baseline.</li>
  <li>Repeat. The list only shrinks, because the gate stops it growing.</li>
</ol>
"""))

    # ═══════════════════════════════════════════════════════════════ explain
    P.append(dict(
        slug="docs/explain", section="Guides",
        title="Explaining a verdict",
        title_tag="Why is this test flaky? flakestat explain",
        description="flakestat explain shows the full evidence behind one test's verdict: its outcome history, same-commit disagreements, and where its failures concentrate.",
        keywords=["why is my test flaky", "flaky test evidence", "flakestat explain"],
        lede="Every verdict can be interrogated. Nothing is asserted that the evidence does not show.",
        body=f"""
{cb("flakestat explain test_checkout_flow")}

{cb("""tests/test_checkout.py::test_checkout_flow
  id 4c1f9a2e77b3d810

  Classification: flaky
  Score:          0.55
  Confidence:     high  (lower bound 0.402 over 41 transition(s))

  Observations:   42
  Passed:         27
  Failed:         15
  Failure rate:   35.7%

  Transitions:               41
  Same-commit disagreements: 9
  Seen across:               6 commit(s), 2 branch(es)

  History  (last 20 of 42, oldest first)

  P F P P F P F F P P P F P P F P P F P P
    ^ ^   ^ ^ ^   ^     ^ ^   ^ ^ ^   ^

  P pass   F fail   -  skip   ^ flip   ! flip on identical code""")}

<h2>Reading the strip</h2>
{tbl(["Symbol", "Meaning"], [
  ["<code>P</code> / <code>F</code>", "Passed / failed"],
  ["<code>-</code>", "Skipped. Carries no signal, so it is shown but never counted as an outcome"],
  ["<code>^</code>", "The outcome changed from the previous comparable run"],
  ["<code>!</code>", "It changed on <em>identical code</em>, the strongest single piece of evidence a test is flaky"],
])}

<p>Markers appear only between observations that are actually comparable: same branch, same
platform, same runtime. A test that always passes on Linux and always fails on Windows is
deterministic, so its strip carries no flip markers even though the outcomes differ.</p>

<h2>Options</h2>
{cb("flakestat explain test_name --json        # the same evidence, for tooling\nflakestat explain test_name --history 80  # widen the strip\nflakestat explain 4c1f9a2e77b3d810        # by id, when names are ambiguous")}

<p>If a name matches several tests, flakestat lists the candidates with their ids rather than
guessing which you meant.</p>
"""))

    # ═════════════════════════════════════════════════════════ github actions
    P.append(dict(
        slug="docs/ci/github-actions", section="Continuous integration",
        title="GitHub Actions",
        title_tag="Detect flaky tests in GitHub Actions with the flakestat action",
        description="Add flaky test detection to GitHub Actions: record every run, post a pull request comment, annotate new flakes in the diff and fail only on regressions.",
        keywords=["github actions flaky tests", "flaky test github action", "detect flaky tests ci"],
        lede="Record every run, comment on the pull request, and gate on regressions rather than on flakiness itself.",
        body=f"""
{cb("""- name: Run tests
  run: pytest --junitxml=reports/junit.xml
  continue-on-error: true

- uses: @@ACTION@@
  with:
    args: ingest 'reports/**/*.xml'
    comment: true          # post/update a PR comment
    annotations: true      # annotate new flakes in the diff""", title=".github/workflows/ci.yml", lang="yaml")}

<p>That records the run, writes a report to the job summary, and posts it as a pull request
comment. A re-run <strong>updates the same comment</strong> rather than adding another.</p>

<h2>The intended sequence</h2>
{cb("""- run: pytest --junitxml=reports/junit.xml
  continue-on-error: true          # record the run even when it fails

- uses: @@ACTION@@
  with: { args: "ingest 'reports/**/*.xml'" }

- uses: @@ACTION@@
  with: { args: "check --fail-on-new", comment: true }""", lang="yaml")}

<div class="callout"><b>continue-on-error matters</b>
A failed suite is exactly the run you most want in the history. Without it, the job stops before
ingest and you record only the runs that passed, which is the one sample guaranteed to hide
flakiness.</div>

<h2>Inputs</h2>
{tbl(["Input", "Default", "Description"], [
  ["<code>args</code>", "None", "Arguments passed to flakestat"],
  ["<code>version</code>", "latest", "Release tag to install, e.g. <code>v0.2.0</code>"],
  ["<code>install-only</code>", "<code>false</code>", "Put the binary on PATH without running it"],
  ["<code>summary</code>", "<code>true</code>", "Append the report to the job summary"],
  ["<code>comment</code>", "<code>false</code>", "Post/update a PR comment (needs <code>pull-requests: write</code>)"],
  ["<code>annotations</code>", "<code>false</code>", "Annotate new and regressed tests in the diff"],
  ["<code>max-rows</code>", "<code>20</code>", "Cap the highlight table on large suites"],
  ["<code>working-directory</code>", "<code>.</code>", "Directory to run in"],
])}
<p>Outputs: <code>flaky-count</code>, <code>new-flaky-count</code>, <code>report-file</code>.</p>

<h2>Permissions</h2>
{cb("permissions:\n  contents: read\n  pull-requests: write   # only if comment: true", lang="yaml")}

<h2>Matrix builds</h2>
<p>Let each matrix job ingest its own results, then merge the NDJSON in an aggregate job. That way
every observation records the platform it actually ran on, rather than the aggregator's.</p>
{cb("""- uses: @@ACTION@@
  with:
    args: >-
      ingest 'reports/**/*.xml'
      --dimension os=${{ matrix.os }}

- uses: actions/upload-artifact@v4
  with:
    name: observations-${{ matrix.os }}
    path: .flakestat/runs.ndjson""", lang="yaml")}
<p>See <a href="{BASE}docs/#dimensions">recording where tests ran</a> for why this matters, and
<a href="{BASE}docs/#history">tracking over time</a> for where to keep the merged history.</p>
"""))

    # ════════════════════════════════════════════════════════════════ gitlab
    P.append(dict(
        slug="docs/ci/gitlab", section="Continuous integration",
        title="GitLab, CircleCI and others",
        title_tag="Flaky tests in GitLab CI, CircleCI and Jenkins",
        description="Use flakestat in GitLab CI, CircleCI, Buildkite, Jenkins or Azure Pipelines. Provider context is detected automatically; nothing is GitHub-specific.",
        keywords=["gitlab ci flaky tests", "circleci flaky tests", "jenkins flaky tests",
                  "buildkite flaky tests"],
        lede="Nothing is GitHub-specific. Install the binary and call it.",
        body=f"""
<p>Provider context (run id, job id, attempt and shard) is detected automatically for GitLab CI,
CircleCI, Buildkite, Jenkins and Azure Pipelines.</p>

{tabs([
  ("GitLab CI", "gitlab", """test:
  script:
    - pytest --junitxml=reports/junit.xml || true
    - curl -sSfL https://raw.githubusercontent.com/rowhitswami/flakestat/main/scripts/install.sh
        | sh -s -- -b /usr/local/bin
    - flakestat ingest 'reports/**/*.xml'
    - flakestat check --fail-on-new
  artifacts:
    paths: [.flakestat/runs.ndjson]
    when: always"""),
  ("CircleCI", "circleci", """- run:
    name: Tests
    command: pytest --junitxml=reports/junit.xml
    when: always
- run:
    name: Record flakiness
    command: |
      curl -sSfL https://raw.githubusercontent.com/rowhitswami/flakestat/main/scripts/install.sh \\
        | sh -s -- -b /usr/local/bin
      flakestat ingest 'reports/**/*.xml'
      flakestat check --fail-on-new
    when: always"""),
  ("Jenkins", "jenkins", """stage('Test') {
  steps {
    sh 'pytest --junitxml=reports/junit.xml || true'
    sh 'flakestat ingest "reports/**/*.xml"'
    sh 'flakestat check --fail-on-new'
  }
}"""),
  ("Buildkite", "buildkite", """steps:
  - command:
      - "pytest --junitxml=reports/junit.xml || true"
      - "flakestat ingest 'reports/**/*.xml'"
      - "flakestat check --fail-on-new"
    artifact_paths: ".flakestat/runs.ndjson\""""),
])}

<div class="callout"><b>Always record, even on failure</b>
Every example above lets the test step fail without stopping the job. Recording only successful
runs is the one sample guaranteed to hide flakiness.</div>

<h2>Sharded pipelines</h2>
<p>Let each shard ingest its own results and concatenate the NDJSON afterwards. Each shard then
records the context it actually ran in, rather than the aggregator's.</p>
{cb("# in each shard\nflakestat ingest 'reports/**/*.xml' --dimension ci.shard=\"$CI_NODE_INDEX\"\n\n# in the aggregate job\ncat shard-*/runs.ndjson > .flakestat/runs.ndjson\nflakestat compact\nflakestat report --top 20")}

<h2>No CI at all</h2>
<p>flakestat is just a binary reading local files. Point it at any directory of JUnit XML you have,
from any source.</p>
{cb("flakestat ingest 'archive/2026-*/junit.xml'\nflakestat report")}
"""))

    # ══════════════════════════════════════════════════════════════ commands
    P.append(dict(
        slug="docs/commands", section="Reference",
        title="Command reference",
        title_tag="flakestat CLI reference: every command and flag",
        description="Full reference for every flakestat command: init, hunt, ingest, report, explain, check, quarantine, ci-report and compact, with all flags and defaults.",
        keywords=["flakestat cli", "flakestat commands", "flakestat flags"],
        lede="Nine commands. Run <code>flakestat &lt;command&gt; -h</code> for the full flag list of any of them.",
        body=f"""
{tbl(["Command", "What it does"], [
  [f'<a href="{BASE}docs/#config"><code>init</code></a>', "Detect the project and write <code>.flakestat.json</code>"],
  [f'<a href="{BASE}docs/#hunting"><code>hunt</code></a>', "Run a test command N times and detect disagreement"],
  [f'<a href="{BASE}docs/#history"><code>ingest</code></a>', "Load JUnit XML from CI into the history"],
  [f'<a href="{BASE}docs/#reading-a-report"><code>report</code></a>', "Score recorded history and print a report"],
  [f'<a href="{BASE}docs/#explain"><code>explain</code></a>', "Show why one test received its verdict"],
  [f'<a href="{BASE}docs/#ci-gate"><code>check</code></a>', "Fail CI when flakiness gets worse, not when it exists"],
  [f'<a href="{BASE}docs/#quarantine"><code>quarantine</code></a>', "Emit a skip list your test runner accepts"],
  ["<code>ci-report</code>", "Render a report for job summaries and PR comments"],
  [f'<a href="{BASE}docs/#history"><code>compact</code></a>', "Drop observations duplicating an already-recorded execution"],
])}

<h2>Scoring flags</h2>
<p>Accepted by <code>hunt</code>, <code>report</code>, <code>explain</code>, <code>check</code> and
<code>quarantine</code>. Defaults are calibrated against a ground-truth corpus, not chosen by
intuition. See <a href="{BASE}docs/#scoring">how scoring works</a> before changing them.</p>
{tbl(["Flag", "Default", "Effect"], [
  ["<code>--threshold</code>", "<code>0.10</code>", "Score at or above which a test is called flaky"],
  ["<code>--suspect-threshold</code>", "<code>0.05</code>", "Score at or above which a test is called suspect"],
  ["<code>--min-runs</code>", "<code>5</code>", "Scored runs required before any verdict"],
  ["<code>--same-commit-weight</code>", "<code>3</code>", "How much more a same-commit disagreement counts"],
  ["<code>--branch-weight</code>", "<code>0.5</code>", "Weight for disagreement off the default branch"],
  ["<code>--default-branch</code>", "<code>main</code>", "Branch whose results are trusted fully"],
  ["<code>--alpha</code>", "<code>0.3</code>", "Recency decay; higher weights recent runs more"],
])}

<h2>Output flags</h2>
{tbl(["Flag", "Default", "Effect"], [
  ["<code>--format</code>", "<code>table</code>", "<code>table</code>, <code>json</code> or <code>markdown</code>"],
  ["<code>--all</code>", "<code>false</code>", "Include stable and unscored tests"],
  ["<code>--top N</code>", "<code>0</code>", "Show only the worst N tests (0 = all)"],
  ["<code>--no-color</code>", "<code>false</code>", "Disable colour, for logs and pipes"],
  ["<code>--dir</code>", "<code>.flakestat</code>", "State directory"],
])}

<h2>Command-specific flags</h2>
<h3>hunt</h3>
{tbl(["Flag", "Effect"], [
  ["<code>--runs N</code>", "How many times to run the command"],
  ["<code>--junit PATH</code>", "Where each run writes its report; supports <code>{run}</code>"],
  ["<code>--parallel N</code>", "Run N copies concurrently. Read the warning in <a href='" + BASE + "docs/#hunting'>hunting</a>"],
  ["<code>--history</code>", "Score all recorded history, not just this burst"],
  ["<code>--dimension k=v</code>", "Attach context; repeatable"],
])}
<h3>ingest</h3>
{tbl(["Flag", "Effect"], [
  ["<code>--commit SHA</code>", "Commit these results belong to (default: current git HEAD)"],
  ["<code>--branch NAME</code>", "Branch name (default: current git branch)"],
  ["<code>--dimension k=v</code>", "Attach context; repeatable"],
  ["<code>--no-host</code>", "Never record this machine's platform, even in CI"],
  ["<code>--run-id ID</code>", "Identifier for this run (default: generated)"],
])}
<h3>check</h3>
{tbl(["Flag", "Effect"], [
  ["<code>--update-baseline</code>", "Accept current flakiness as the baseline"],
  ["<code>--fail-on-new</code>", "Exit 1 when a test is newly flaky (default true)"],
  ["<code>--fail-on-regression</code>", "Exit 2 when an accepted test worsens"],
  ["<code>--regression-delta</code>", "Score increase before that counts (default 0.1)"],
  ["<code>--baseline PATH</code>", "Baseline file (default <code>&lt;dir&gt;/baseline.json</code>)"],
])}
"""))

    # ════════════════════════════════════════════════════════════════ config
    P.append(dict(
        slug="docs/config", section="Reference",
        title="Configuration",
        title_tag="flakestat configuration with .flakestat.json",
        description="Configure flakestat once with .flakestat.json so the day-to-day command is a single word. Flags always override the file.",
        keywords=["flakestat config", "flakestat.json"],
        lede="Write the command and report path once, then run a single word.",
        body=f"""
{cb("flakestat init            # detects your project\nflakestat init --project pytest   # or skip detection\nflakestat init --force    # overwrite an existing config")}

{cb("""{{
  "command": ["pytest", "--junitxml={junit}"],
  "junit": "reports/junit-{run}.xml",
  "runs": 20,
  "threshold": 0.10
}}""", title=".flakestat.json", lang="json")}

<p>With that file in place, the whole invocation becomes:</p>
{cb("flakestat hunt")}

<h2>Precedence</h2>
<p>Flags always win over the file, so a one-off <code>flakestat hunt --runs 50</code> works
without editing anything. Nothing in the file is required, and every key has a default.</p>

{tbl(["Key", "Meaning"], [
  ["<code>command</code>", "Your test command as an argv array. <code>{junit}</code> is substituted."],
  ["<code>junit</code>", "Where each run writes its report. <code>{run}</code> is substituted."],
  ["<code>runs</code>", "Default run count for <code>hunt</code>"],
  ["<code>threshold</code>", "Flaky threshold"],
  ["<code>suspect_threshold</code>", "Suspect threshold"],
  ["<code>min_runs</code>", "Scored runs before a verdict"],
  ["<code>default_branch</code>", "Branch whose results are trusted fully"],
])}

<div class="callout"><b>Commit it</b>
The config belongs in the repo. It is how everyone on the team, and CI, runs the same thing.</div>

<h2>What gets written where</h2>
{tbl(["Path", "What it is", "Commit it?"], [
  ["<code>.flakestat.json</code>", "Configuration", "Yes"],
  ["<code>.flakestat/runs.ndjson</code>", "Observation history", f'<a href="{BASE}docs/#history">Depends</a>'],
  ["<code>.flakestat/baseline.json</code>", "Accepted flakiness for <code>check</code>", "Yes"],
])}
"""))

    # ═══════════════════════════════════════════════════════════════ scoring
    P.append(dict(
        slug="docs/scoring", section="Reference",
        title="How scoring works",
        title_tag="How flaky test scoring works in flakestat",
        description="flakestat scores state transitions rather than failure rate and classifies on a lower bound, so a verdict needs evidence rather than a lucky flip.",
        keywords=["flaky test score", "flaky test algorithm", "flaky test detection algorithm"],
        lede="Five steps, and a live demo you can poke at to see each of them.",
        body=f"""
<h2>Try it</h2>
<p>Click any run to cycle it between pass, fail and skip. The verdict below is computed with the
same rules the binary uses.</p>

<div class="demo" id="demo">
  <div class="demo-top">
    <p>Outcome history, oldest first, in one execution context on one commit.</p>
    <div class="seq"></div>
    <div class="demo-actions">
      <button class="mini" data-demo="add">+ run</button>
      <button class="mini" data-demo="remove">− run</button>
      <button class="mini" data-demo="PPFPPFPPFPPF">Flaky</button>
      <button class="mini" data-demo="PPPPPPPPPPPP">Stable</button>
      <button class="mini" data-demo="FFFFFFFFFFFF">Always fails</button>
      <button class="mini" data-demo="PPF">Too few runs</button>
      <button class="mini" data-demo="SSSSSS">Always skipped</button>
    </div>
  </div>
  <div class="demo-out">
    <div class="stat"><span>Verdict</span><b id="d-verdict">&middot;</b></div>
    <div class="stat"><span>Score</span><b id="d-score">&middot;</b></div>
    <div class="stat"><span>Lower bound</span><b id="d-lower">&middot;</b></div>
    <div class="stat"><span>Pass / fail</span><b id="d-runs">&middot;</b></div>
    <div class="stat"><span>Comparisons</span><b id="d-trans">&middot;</b></div>
  </div>
  <div class="demo-why" id="d-why"></div>
</div>

<p><small>The demo covers one execution context on one commit, which is where the interesting
behaviour is. The binary additionally weights same-commit and cross-branch evidence, and decays by
age, in steps 3 and 4 below.</small></p>

<h2>The five steps</h2>
<ol>
  <li><strong>Group by execution context.</strong> Two outcomes are only comparable if branch, os,
  arch and runtime were the same. Without this, interleaving platforms makes a test that always
  fails on Windows and always passes elsewhere read as constant disagreement, a phantom signal
  that has caught this project twice.</li>
  <li><strong>Count transitions.</strong> Every adjacent pair within a context that disagreed is a
  flip. Failure rate is deliberately not used, which is why an always-failing test scores zero.</li>
  <li><strong>Weight them.</strong> A flip on the same commit counts three times a cross-commit one,
  because the latter may be a regression someone fixed. Disagreement off the default branch counts
  for half, because failures there are expected.</li>
  <li><strong>Decay by age.</strong> Age is measured in <em>commits</em>, not observations, so a
  200-run burst on one commit uses all of its evidence instead of only the tail.</li>
  <li><strong>Take a lower bound.</strong> Classification uses a Wilson score lower bound with an
  effective sample size, so a verdict requires evidence rather than a lucky flip in a short run.</li>
</ol>

<h2>Skips are not outcomes</h2>
{cb("""pass    -> pass evidence
failure -> fail evidence
error   -> fail evidence
skip    -> neither""")}
<p>A skipped run is not evidence either way, so it never becomes an outcome a neighbour can
disagree with, and never enters the failure rate. <code>P S P</code> is one comparison between two
passes, not two disagreements.</p>

<h2>Where failures concentrate</h2>
<p>Association analysis is separate and never touches the score. A test is flaky because its
outcomes demonstrate flakiness; associations only say <em>where</em> that flakiness concentrates.
Findings must clear an evidence floor, an effect-size floor, and a Benjamini–Hochberg correction
for how many dimensions were tested. See
<a href="{BASE}docs/#dimensions">recording where tests ran</a>.</p>

<h2>Going deeper</h2>
<p>The <a href="{BASE}design/">design notes</a> record the reasoning in full, including the
mistakes that shaped it: a recency weight that made 100 runs no better than 10, a parallel mode
that manufactured the flakiness it reported, and three separate occasions where incomparable
observations were compared. The <a href="{BASE}validation/">validation record</a> covers whether
any of it holds up against somebody else&rsquo;s bugs.</p>
"""))

    # ═══════════════════════════════════════════════════════════════ runners
    P.append(dict(
        slug="docs/runners", section="Reference",
        title="Supported test runners",
        title_tag="Supported test runners: pytest, Jest, go test, JUnit",
        description="flakestat works with any runner that writes JUnit XML: pytest, Jest, Vitest, go test, JUnit 5, TestNG, RSpec, PHPUnit, Playwright, Cypress, xUnit, NUnit and more.",
        keywords=["junit xml test runners", "pytest junit xml", "jest junit xml", "gotestsum"],
        lede="Anything that writes JUnit XML, which in practice is everything.",
        body=f"""
<p><span class="pill">pytest</span><span class="pill">Jest</span><span class="pill">Vitest</span>
<span class="pill">go test</span><span class="pill">JUnit 5</span><span class="pill">TestNG</span>
<span class="pill">Surefire</span><span class="pill">RSpec</span><span class="pill">PHPUnit</span>
<span class="pill">Mocha</span><span class="pill">Playwright</span><span class="pill">Cypress</span>
<span class="pill">xUnit</span><span class="pill">NUnit</span><span class="pill">cargo test</span></p>

{tbl(["Language", "Runner", "How to emit JUnit XML"], [
  ["Python", "pytest", "<code>pytest --junitxml=reports/junit.xml</code>"],
  ["Python", "unittest", "<code>unittest-xml-reporting</code>"],
  ["JS / TS", "Jest", "<code>jest-junit</code> reporter"],
  ["JS / TS", "Vitest", "<code>--reporter=junit</code>"],
  ["JS / TS", "Mocha", "<code>mocha-junit-reporter</code>"],
  ["JS / TS", "Playwright", "<code>--reporter=junit</code>"],
  ["JS / TS", "Cypress", "<code>cypress-multi-reporters</code>"],
  ["Go", "go test", "<code>gotestsum --junitfile reports/junit.xml</code>"],
  ["Java", "Maven Surefire", "Written by default to <code>target/surefire-reports/</code>"],
  ["Java", "Gradle / JUnit 5", "Written by default to <code>build/test-results/</code>"],
  ["Ruby", "RSpec", "<code>rspec_junit_formatter</code>"],
  ["PHP", "PHPUnit", "<code>--log-junit reports/junit.xml</code>"],
  [".NET", "xUnit / NUnit", "<code>dotnet test --logger junit</code>"],
  ["Rust", "cargo test", "<code>cargo2junit</code> or <code>cargo-nextest</code>"],
  ["Elixir", "ExUnit", "<code>junit_formatter</code>"],
])}

<h2>Dialect tolerance</h2>
<p>JUnit XML has no official schema, so every framework writes it slightly differently. The parser
handles both root elements, nested suites, locale-formatted durations like <code>4,521</code>,
bytes that are illegal in XML 1.0, and Surefire's <code>&lt;flakyFailure&gt;</code> markers, which
are a direct flakiness signal and are read as one.</p>
<p>Playwright emits <code>&lt;error&gt;</code> for some failures and <code>&lt;failure&gt;</code>
for others; both count as failures. A <code>&lt;skipped&gt;</code> test counts as neither.</p>

<h2>No JUnit XML?</h2>
<p>flakestat falls back to exit codes and reports suite-level flakiness. That is less precise,
because you learn the suite is flaky rather than which test, but it works with anything that
returns a status code.</p>
{cb("flakestat hunt --runs 20 -- ./run-tests.sh")}
"""))

    # ═══════════════════════════════════════════════════════════════════ faq
    P.append(dict(
        slug="docs/faq", section="Reference",
        title="Frequently asked questions",
        title_tag="flakestat FAQ: flaky test detection questions answered",
        description="Answers about flaky test detection with flakestat: data privacy, how many runs you need, monorepos and sharding, re-run double counting, and CI overhead.",
        keywords=["flaky test faq", "flaky test questions", "how many runs flaky test"],
        lede="Short answers. Each links to the longer explanation.",
        faq=[
            ("Does my test data leave my machine?",
             "No. flakestat is a binary that reads local files and writes a local file. There is no network call, no account and no telemetry."),
            ("My test failed 100% of the time. Why is the score zero?",
             "Because it is not flaky, it is broken. flakestat scores how often a test disagrees with itself, not how often it fails, so a consistently failing test scores zero and is reported as consistently-failing, which is a different problem with a different fix."),
            ("How many runs do I need to detect a flaky test?",
             "It depends on the flake rate. A test failing about 25% of the time is reliably caught within 20 runs. One failing 10% of the time usually needs 50 or more. Below about 5%, a local burst is the wrong instrument and recording CI history is better."),
            ("Can I use flakestat with a monorepo or sharded CI?",
             "Yes. Let each shard ingest its own results and concatenate the NDJSON. The history is append-only with one observation per line specifically so shards merge with cat and no merge step."),
            ("Does re-running a CI job double-count results?",
             "No. Each observation carries the identity of the execution it describes, so the same artifact ingested twice counts once, while a genuine retry that really did run the tests again counts as the second execution it is."),
            ("Will flakestat slow down my CI?",
             "Ingest parses XML files you already produce and takes milliseconds. Hunt runs your suite N times and costs exactly that, which is why it is a local tool rather than something you run on every build."),
            ("What if my test runner does not write JUnit XML?",
             "Almost all of them can with one flag or one reporter package. Failing that, flakestat falls back to exit codes and reports suite-level flakiness."),
            ("Is flakestat free?",
             "Yes. It is MIT licensed and free at any volume, with no per-seat or per-run pricing, because it runs on your own machines."),
            ("How is this different from just retrying failed tests?",
             "Retries hide flakiness; flakestat measures it. A retried test still costs you time and still fails sometimes in ways that matter. Retry to keep the pipeline moving, measure so the list actually shrinks."),
        ],
        body=f"""
<h3>Does my test data leave my machine?</h3>
<p>No. flakestat is a binary that reads local files and writes a local file. There is no network
call, no account and no telemetry.</p>

<h3>My test failed 100% of the time. Why is the score zero?</h3>
<p>Because it isn't flaky, it's broken. flakestat scores how often a test <em>disagrees with
itself</em>, not how often it fails, so a consistently failing test scores zero and appears as
<code>consistently-failing</code>, a different problem with a different fix. See
<a href="{BASE}docs/#reading-a-report">reading a report</a>.</p>

<h3>How many runs do I need?</h3>
<p>Depends on the flake rate. ~25% is reliably caught within 20 runs; 10% usually needs 50 or more;
below about 5% a local burst is the wrong instrument and
<a href="{BASE}docs/#history">CI history</a> is right. <code>insufficient-data</code> means exactly
that. There is not enough evidence yet.</p>

<h3>Can I use it with a monorepo or sharded CI?</h3>
<p>Yes. Let each shard ingest its own results and concatenate the NDJSON. It is append-only and one
observation per line specifically so that works with <code>cat</code> and no merge step.</p>

<h3>Does re-running a job double-count?</h3>
<p>No. Each observation carries the identity of the execution it describes, so the same artifact
ingested twice counts once, while a genuine retry, which really did run the tests again, counts as
the second execution it is.</p>

<h3>Will it slow down my CI?</h3>
<p><code>ingest</code> parses XML files you already produce; it is milliseconds. <code>hunt</code>
runs your suite N times and costs exactly that, which is why it is a local tool rather than
something on every build.</p>

<h3>What if my runner doesn't write JUnit XML?</h3>
<p>Almost all of them can, with one flag or one reporter package. See
<a href="{BASE}docs/#runners">supported runners</a>. Failing that, exit-code mode still gives
suite-level results.</p>

<h3>Is it free?</h3>
<p>Yes. MIT licensed, free at any volume, and no per-seat or per-run pricing, because it runs on your own
machines, so there is nothing to meter.</p>

<h3>How is this different from just retrying failed tests?</h3>
<p>Retries hide flakiness; flakestat measures it. A retried test still costs time and still fails
in ways that matter. Retry to keep the pipeline moving, and measure so the list actually shrinks.
<a href="{BASE}docs/#ci-gate">Gating on regressions</a> is how you stop it growing.</p>

<h3>Why not just use a hosted service?</h3>
<p>Use one if you want dashboards, org-wide rollups and alerting. They do that well.
flakestat is for the case where you want detection to be local, free and yours. See
<a href="{BASE}compare/trunk/">how it compares</a>.</p>
"""))

    # ══════════════════════════════════════════════════ per-framework pages
    fw = [
        ("pytest", "Python", "pytest",
         "pytest --junitxml=reports/junit.xml",
         "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- pytest --junitxml='{junit}'",
         "pytest",
         ["flaky tests pytest", "pytest flaky test detection", "pytest rerun failures",
          "pytest flaky test plugin", "find flaky tests python"],
         """<p>pytest writes JUnit XML natively, so there is no plugin to install. If you already use
<code>pytest-rerunfailures</code>, keep it. Retries keep the pipeline moving, but they hide the
flakiness rather than measuring it. flakestat measures it so the list actually shrinks.</p>"""),
        ("jest", "JavaScript and TypeScript", "Jest",
         "npx jest --reporters=default --reporters=jest-junit",
         "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- npx jest --reporters=jest-junit",
         "jest",
         ["flaky tests jest", "jest flaky test detection", "jest retry times",
          "find flaky tests javascript", "vitest flaky tests"],
         """<p>Install <code>jest-junit</code> and point it at a per-run path. Jest's
<code>--retryTimes</code> hides flakiness rather than measuring it. Useful to keep CI moving,
but it is why flaky suites stay flaky for years. Vitest works the same way with
<code>--reporter=junit</code>.</p>"""),
        ("go", "Go", "go test",
         "gotestsum --junitfile reports/junit.xml -- ./...",
         "flakestat hunt --runs 20 --junit 'reports/junit-{run}.xml' \\\n  -- gotestsum --junitfile='{junit}' -- ./... -count=1",
         "go",
         ["flaky tests go", "go test flaky", "gotestsum junit", "golang flaky tests",
          "go test shuffle flaky"],
         """<p><code>go test</code> does not write JUnit XML itself, so use
<a href="https://github.com/gotestyourself/gotestsum">gotestsum</a>, which does and is a drop-in
wrapper. Two flags matter: <code>-count=1</code> defeats the test cache so every run really runs,
and <code>-shuffle=on</code> varies test order, which is what surfaces order-dependent flakes
caused by shared package-level state.</p>"""),
    ]
    for slug, lang, runner, emit, hunt, tab, kw, note in fw:
        P.append(dict(
            slug=f"flaky-tests/{slug}", section="By framework",
            title=f"Find flaky tests in {runner}",
            title_tag=f"How to find flaky tests in {runner} ({lang})",
            description=f"Detect flaky tests in {runner}. Run the suite repeatedly or record CI runs over time, using the JUnit XML {runner} already writes. Free and open source.",
            keywords=kw,
            lede=f"{runner} already writes everything flakestat needs. Here is the shortest path from “CI is flaky” to a ranked list.",
            body=f"""
<h2>1. Emit JUnit XML</h2>
{cb(emit, title=f"{runner}")}
{note}

<h2>2. Hunt for disagreement</h2>
{cb(hunt, title="run the suite 20 times")}
<p>That runs your suite 20 times on unchanged code and reports which tests changed their answer.
Nothing is modified in your repo, and the whole thing is one binary.</p>

<h2>3. Read the result</h2>
{cb("""VERDICT               SCORE  CONF  RUNS  PASS/FAIL  TEST
flaky                  0.62  high    20       14/6  test_checkout::test_race
consistently-failing   0.00  high    20       0/20  test_api::test_broken""")}
<p>A test failing every time scores <code>0.00</code> and is reported separately, because it is broken,
not flaky, and mixing the two wastes the time of whoever is hunting nondeterminism.
<a href="{BASE}docs/#reading-a-report">Full explanation of the columns</a>.</p>

<h2>4. Record CI runs so it keeps working</h2>
<p>A local burst finds the obvious flakes. Recording the CI runs you already pay for finds the ones
that only appear on another platform, another runtime, or under load, at no extra compute.</p>
{cb("flakestat ingest 'reports/**/*.xml'\nflakestat report --top 20")}
<p>See <a href="{BASE}docs/#github-actions">GitHub Actions</a> or
<a href="{BASE}docs/#gitlab">GitLab and others</a> for a complete workflow, and
<a href="{BASE}docs/#ci-gate">gating CI</a> to stop new flaky tests getting in.</p>

<h2>Why not just retry?</h2>
<p>Retrying keeps the pipeline moving, and you should. But it hides the problem. A test that needs
a retry still fails sometimes in ways that matter, and a suite with retries on stays flaky for
years because nothing ever measures whether it is getting better. Retry to unblock; measure so the
list shrinks.</p>

<h2>Install</h2>
{INSTALL_TABS}
<p>Then <a href="{BASE}docs/#quickstart">the quickstart</a> takes about two minutes.</p>
"""))

    # ═══════════════════════════════════════════════════════════ comparisons
    for slug, name, blurb, kw in [
        ("trunk", "Trunk Flaky Tests",
         "Trunk is a hosted service with dashboards, org-wide rollups and alerting. It does same-commit detection too, and does the hosted part well.",
         ["trunk flaky tests alternative", "trunk io alternative", "open source trunk alternative"]),
        ("buildpulse", "BuildPulse",
         "BuildPulse is a hosted flaky test service that ingests your CI results and ranks flaky tests in a web dashboard.",
         ["buildpulse alternative", "buildpulse open source alternative", "free buildpulse alternative"]),
    ]:
        P.append(dict(
            slug=f"compare/{slug}", section="Alternatives",
            title=f"flakestat vs {name}",
            title_tag=f"{name} alternative: free and self-hosted",
            description=f"An honest comparison of flakestat and {name}: what each does well, what flakestat deliberately does not do, and which one fits your situation.",
            keywords=kw,
            lede=f"An honest comparison, including the cases where {name} is the better choice.",
            body=f"""
<p>{blurb} flakestat is a local binary that does the detection part, for free, without your test
data leaving your machines.</p>

{tbl(["", "flakestat", name], [
  ["Cost", "Free at any volume", "Per seat or per run"],
  ["Test data leaves your machine", "No", "Yes, results are uploaded"],
  ["Works before you push", "Yes, <code>hunt</code> runs locally", "No, it needs CI results"],
  ["Setup", "One binary", "Account plus CI integration"],
  ["Languages", "Any that writes JUnit XML", "Many"],
  ["Dashboards and history UI", "No", "Yes"],
  ["Org-wide rollups, alerting", "No", "Yes"],
  ["Auto-quarantine in the platform", "Emits a skip list you apply", "Yes, managed"],
  ["Source available", "MIT licensed", "Proprietary"],
])}

<h2>When {name} is the better choice</h2>
<ul>
  <li>You want a dashboard non-engineers can read.</li>
  <li>You need rollups across many repositories and teams.</li>
  <li>You want alerting, ownership routing and SLA tracking.</li>
  <li>You would rather buy the whole workflow than assemble it.</li>
</ul>
<p>flakestat does not try to do any of that, and pretending otherwise would waste your time.</p>

<h2>When flakestat is the better choice</h2>
<ul>
  <li>Test data cannot leave your infrastructure, whether from regulation, private code, or policy.</li>
  <li>You want to find a flake <em>before</em> pushing, not after CI reports it.</li>
  <li>The budget for this is zero, or the volume makes per-run pricing awkward.</li>
  <li>You want the detection logic to be readable and auditable rather than a black box.</li>
</ul>

<h2>What flakestat does differently</h2>
<p><strong>It ranks by inconsistency, not failure rate.</strong> A test that fails every time scores
zero and is reported as <code>consistently-failing</code>, so your most broken test does not sit at
the top of the flaky list wasting the time of whoever is hunting nondeterminism.</p>
<p><strong>Every verdict carries a confidence level.</strong> A score from three runs and the same
score from three hundred are different claims, and small samples can never reach high confidence.</p>
<p><strong>It refuses to claim causation.</strong> When failures cluster on Windows it says they
cluster on Windows, and when <code>os</code> and <code>arch</code> vary together it says the
observations cannot tell which one matters. <a href="{BASE}docs/#dimensions">More on that</a>.</p>

<h2>Can I use both?</h2>
<p>Yes, and it is a reasonable setup: the hosted service for org-wide visibility, flakestat locally
so engineers can reproduce and confirm a flake in ten minutes without pushing. They read the same
JUnit XML and neither interferes with the other.</p>

<div class="callout"><b>Try it in two minutes</b>
<code>brew install rowhitswami/tap/flakestat</code>, then
<a href="{BASE}docs/#quickstart">the quickstart</a>. Nothing to sign up for.</div>
"""))

    # Substituted here rather than interpolated: several of these sit inside
    # nested string literals, where an f-string placeholder stays literal.
    for page in P:
        page["body"] = page["body"].replace("@@ACTION@@", ACTION_REF)
    return P
