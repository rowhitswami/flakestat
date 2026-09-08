#!/usr/bin/env bash
#
# Reproduce flakestat's strongest validation result on your own machine.
#
# The claim being tested is not about a fixture we wrote. ConduitIO's own
# contributors reported four flaky tests (issue #2534), named them, and later
# fixed them (#2537). This script runs the identical protocol at the commit
# before that fix and at the fix itself, and shows that flakestat calls the
# tests flaky on one side and stable on the other.
#
# We did not create the bug, label the bug, or repair the bug. flakestat is
# handed observations from both sides of a commit it had no part in.
#
#   ./scripts/reproduce-validation.sh          ~2 minutes, 30 executions per side
#   ./scripts/reproduce-validation.sh --full   ~4 minutes, 100 per side
#
# Requirements: git, go (1.25+), and network access. Nothing is installed
# system-wide; everything lands in a temporary directory that is removed on
# exit unless you pass --keep.

set -uo pipefail

REPO_URL="https://github.com/conduitio/conduit.git"
BEFORE="612f5bfb4e1e2a0bab3e1e717c23fb6a8917d591"   # parent of the deflaking fix
AFTER="9e00e5948590a7f715d25fe617855fe4aacc9563"    # "test: deflake known-flaky tests from #2534" (#2537)

# The packages that commit touched. Scoping to them keeps this to minutes
# rather than the half-hour the whole conduit suite would cost.
PKGS=(./pkg/plugin/processor/builtin/internal/diff/lcs/... ./pkg/schemaregistry/...)

# Named in conduit's issue #2534 and repaired by #2537.
WATCH="TestClient_NotFound TestClient_CacheMiss TestClient_CacheHit TestRandOld"

RUNS=30
KEEP=0
for arg in "$@"; do
  case "$arg" in
    --full) RUNS=100 ;;
    --keep) KEEP=1 ;;
    --runs=*) RUNS="${arg#--runs=}" ;;
    -h|--help) sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

say()  { printf '\n\033[1m%s\033[0m\n' "$*"; }
note() { printf '  %s\n' "$*"; }
die()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

command -v git >/dev/null || die "git is required"
command -v go  >/dev/null || die "go is required (1.25+)"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/flakestat-repro.XXXXXX") || die "cannot create a work directory"
cleanup() { [ "$KEEP" -eq 1 ] && { echo; note "kept: $WORK"; return; }; rm -rf "$WORK"; }
trap cleanup EXIT

ROOT=$(cd "$(dirname "$0")/.." && pwd)

say "flakestat: reproducing the ConduitIO before/after result"
note "runs per side : $RUNS  (each is a fresh process, -count=3 -shuffle=<seed>)"
note "work directory: $WORK"

# ---------------------------------------------------------------- flakestat
say "1/4  Building flakestat"
if [ -f "$ROOT/go.mod" ] && grep -q "module github.com/rowhitswami/flakestat" "$ROOT/go.mod" 2>/dev/null; then
  (cd "$ROOT" && go build -o "$WORK/flakestat" ./cmd/flakestat) || die "could not build flakestat"
  note "built from this checkout: $(cd "$ROOT" && git rev-parse --short HEAD 2>/dev/null || echo 'unknown')"
elif command -v flakestat >/dev/null; then
  cp "$(command -v flakestat)" "$WORK/flakestat"
  note "using the installed binary"
else
  die "run this from a flakestat checkout, or install flakestat first"
fi
note "$("$WORK/flakestat" version)"

say "2/4  Fetching gotestsum (converts go test output to JUnit XML)"
if command -v gotestsum >/dev/null; then
  cp "$(command -v gotestsum)" "$WORK/gotestsum"
  note "using the installed gotestsum"
else
  GOBIN="$WORK" go install gotest.tools/gotestsum@latest >/dev/null 2>&1 \
    || die "could not install gotestsum"
  note "installed into the work directory"
fi

# ------------------------------------------------------------------ conduit
say "3/4  Fetching ConduitIO/conduit at both commits"
SRC="$WORK/conduit"
mkdir -p "$SRC" && (cd "$SRC" && git init -q . && git remote add origin "$REPO_URL") || die "git init failed"
for sha in "$BEFORE" "$AFTER"; do
  (cd "$SRC" && git fetch -q --depth 1 origin "$sha") || die "could not fetch $sha"
  note "fetched ${sha:0:8}"
done

# --------------------------------------------------------------------- runs
arm() {
  local sha="$1" label="$2" out="$WORK/$2"
  mkdir -p "$out/xml" "$out/state"
  (cd "$SRC" && git checkout -q "$sha") || die "could not check out $sha"
  (cd "$SRC" && go build "${PKGS[@]}" >/dev/null 2>&1) || die "build failed at $sha"

  local fails=0
  for i in $(seq 1 "$RUNS"); do
    local n seed
    n=$(printf '%03d' "$i")
    # A seed we choose and record, so any failure here is reproducible rather
    # than "run 47 failed".
    seed=$(( (RANDOM << 15 | RANDOM) * 1000 + i ))
    (cd "$SRC" && "$WORK/gotestsum" --junitfile "$out/xml/run-$n.xml" --format none -- \
        -count=3 -shuffle="$seed" "${PKGS[@]}") >/dev/null 2>&1
    grep -q "<failure\|<error" "$out/xml/run-$n.xml" 2>/dev/null && fails=$((fails+1))
    # Overwrite in place on a terminal; one tidy line per run when piped to a
    # file or a CI log, where \r would smear everything onto one line.
    if [ -t 1 ]; then
      printf '\r  %s: %d/%d executions, %d with failures' "$label" "$i" "$RUNS" "$fails"
    elif [ "$i" -eq "$RUNS" ]; then
      printf '  %s: %d executions, %d with failures\n' "$label" "$RUNS" "$fails"
    fi
  done
  [ -t 1 ] && printf '\n'

  (cd "$out/state" && for f in "$out"/xml/*.xml; do
      "$WORK/flakestat" ingest "$f" --commit "$sha" --branch main >/dev/null 2>&1
   done)
  # Explicit, because the last command above is a conditional and `set -u`
  # plus a non-zero return would abort the caller.
  return 0
}

say "4/4  Running the protocol at each commit"
note "before the fix, ${BEFORE:0:8}"
arm "$BEFORE" before
note "at the fix,     ${AFTER:0:8}  (identical protocol)"
arm "$AFTER" after

# ------------------------------------------------------------------ verdict
say "Result"

# Written to a file first: a heredoc inside a || fallback does not parse.
cat > "$WORK/summarise.py" <<'PY'
import json, subprocess, sys
work, watch = sys.argv[1], sys.argv[2].split()

def verdicts(arm):
    out = subprocess.run([f"{work}/flakestat", "report", "--format", "json", "--all"],
                         cwd=f"{work}/{arm}/state", capture_output=True, text=True).stdout
    try:
        return {r["name"]: r for r in json.loads(out).get("results", [])}
    except Exception:
        return {}

b, a = verdicts("before"), verdicts("after")
print(f"  {'test':<34}{'before the fix':<26}{'at the fix'}")
print("  " + "-" * 74)
flips = 0
for name in watch:
    rb, ra = b.get(name), a.get(name)
    if not rb or not ra:
        print(f"  {name:<34}{'not observed':<26}{'not observed'}")
        continue
    fb = f"{rb['verdict']} {rb['score']:.2f}  {rb['fails']}/{rb['runs']}"
    fa = f"{ra['verdict']} {ra['score']:.2f}  {ra['fails']}/{ra['runs']}"
    flip = rb["verdict"] in ("flaky", "suspect") and ra["verdict"] == "stable"
    flips += flip
    print(f"  {name:<34}{fb:<26}{fa}{'   <-- detected, then silent' if flip else ''}")

print()
if flips:
    print(f"  {flips} of {len(watch)} tests were flagged before ConduitIO's fix and stable at it.")
    print("  Neither commit is ours. The defect and the repair are both theirs.")
else:
    print("  No test flipped. On a fast machine the race may not reproduce at this")
    print("  sample size - try --full. A null result here is reported as a null")
    print("  result; see VALIDATION.md for why that distinction matters.")
PY

if command -v python3 >/dev/null; then
  python3 "$WORK/summarise.py" "$WORK" "$WATCH"
else
  note "(python3 unavailable - raw reports below)"
  for a in before after; do
    echo; note "--- $a ---"
    (cd "$WORK/$a/state" && "$WORK/flakestat" report --no-color --all)
  done
fi

say "Notes"
note "conduit issue #2534 reported these tests; PR #2537 fixed them."
note "Full methodology and the protocol amendments: VALIDATION.md"
note "TestRandOld fired only twice in 300 observations for us and is expected"
note "to read 'stable'. That is the detector declining on thin evidence."
