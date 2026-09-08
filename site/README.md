# flakestat.com

The generator for <https://flakestat.com>. Plain Python, one dependency, output
committed to the `gh-pages` branch.

```sh
pip install -r requirements.txt
python3 build.py out       # render the site into ./out
python3 check.py out       # every internal link and #fragment must resolve
./deploy.sh                # build, check, publish to gh-pages
```

## What is where

| File | Contains |
| --- | --- |
| `build.py` | Page shell, navigation, JSON-LD, sitemap, `robots.txt`, `llms.txt`, search index |
| `content.py` | The reference documentation, framework pages and comparisons |
| `writing.py` | Long-form posts |
| `mddocs.py` | Renders `VALIDATION.md`, `HUNT-FINDINGS.md`, `DESIGN.md` and `CHANGELOG.md` from the repository root |
| `check.py` | Link checker. Run before every deploy; CI runs it too |
| `theme.css`, `app.js`, `assets/` | Everything the browser gets |

## Two rules worth keeping

**The long-form pages are generated, never retyped.** `/validation/`,
`/findings/`, `/design/` and `/changelog/` come from the Markdown files in the
repository root at build time. Edit the Markdown; the page follows.

**Nothing ships with a broken link.** `check.py` resolves every internal `href`
and every `#fragment`, in both quote styles. A fragment that points at nothing
still returns 200 and lands the reader somewhere else, so checking that the page
exists is not enough. It also fails on duplicate element ids, which is how a
heading called "Results" once collided with the search palette.
