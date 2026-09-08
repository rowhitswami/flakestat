#!/usr/bin/env bash
# Build, verify, and publish the site to the gh-pages branch.
#
# The check is not optional. A link that 404s is visible to every reader and to
# nobody who wrote it; if check.py fails, nothing is published.
set -euo pipefail

cd "$(dirname "$0")"
ROOT=$(git rev-parse --show-toplevel)
OUT=$(mktemp -d)
WORK=$(mktemp -d)/gh-pages

cleanup() {
  git -C "$ROOT" worktree remove --force "$WORK" 2>/dev/null || true
  git -C "$ROOT" worktree prune
  rm -rf "$OUT" "$(dirname "$WORK")"
}
trap cleanup EXIT

python3 build.py "$OUT"
python3 check.py "$OUT"

git -C "$ROOT" fetch --quiet origin gh-pages
git -C "$ROOT" worktree add --quiet --detach "$WORK" origin/gh-pages
find "$WORK" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "$OUT"/. "$WORK"/

git -C "$WORK" add -A
if git -C "$WORK" diff --cached --quiet; then
  echo "no change to publish"
  exit 0
fi
git -C "$WORK" commit -q -m "${1:-docs: rebuild the site}"
git -C "$WORK" push -q origin HEAD:gh-pages
echo "published $(git -C "$WORK" rev-parse --short HEAD) to gh-pages"
