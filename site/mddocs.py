#!/usr/bin/env python3
"""Render the repository's long-form Markdown as pages on the site.

The Markdown in the repository is the source. These pages are generated from
it at build time so the two cannot drift apart.
"""
import html
import os
import re
import subprocess

import markdown

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

# Order here is the order they appear on the writing index and in llms.txt.
DOCS = [
    ("validation", "VALIDATION.md", dict(
        kicker="Validation record",
        title="How flakestat was validated",
        title_tag="How flakestat was validated: protocol, subjects and results",
        og_title="How flakestat was validated",
        description="The full validation record for flakestat. A protocol registered "
                    "before any experiment ran, three third-party subjects, and every "
                    "outcome, including the arm that found nothing and a contaminated "
                    "run that was thrown away.",
        keywords=["flaky test detector validation", "flaky test detection accuracy",
                  "how to validate a flaky test tool", "flakestat validation"],
        lede="The contract was written before the experiments. This is that contract, "
             "and what the experiments returned.",
        summary="Three third-party subjects, one frozen build, and a set of rules "
                "fixed in advance about what each result would be allowed to mean.",
    )),
    ("findings", "HUNT-FINDINGS.md", dict(
        kicker="Field notes",
        title="Hunting flaky tests in open source",
        title_tag="Hunting flaky tests in five open-source Go projects",
        og_title="Hunting flaky tests in five open-source projects",
        description="What happened when flakestat ran overnight against five active "
                    "open-source Go projects: nine real flaky tests, seven already "
                    "filed by their maintainers, and zero previously-unknown flakes.",
        keywords=["flaky tests in open source", "hashicorp raft flaky test",
                  "litestream flaky test", "go test count shuffle flaky"],
        lede="Nine real flaky tests across five projects. Seven were already filed, "
             "and the two that were not turned out not to be flaky tests.",
        summary="An overnight run against five active Go repositories, and why it "
                "closed nothing.",
    )),
    ("design", "DESIGN.md", dict(
        kicker="Design",
        title="Design notes",
        title_tag="flakestat design notes: how the scoring actually works",
        og_title="flakestat design notes",
        description="Why flakestat scores inconsistency rather than failure rate, how "
                    "execution context is compared, what the confidence level is "
                    "derived from, and which questions are still open.",
        keywords=["flaky test scoring algorithm", "wilson score flaky test",
                  "flaky test detection design", "junit xml parsing"],
        lede="The reasoning behind the scoring, the storage format and the command "
             "line, including the parts that are still guesses.",
        summary="Why flakiness is scored as inconsistency, and the three times that "
                "principle was implemented wrong.",
    )),
    ("changelog", "CHANGELOG.md", dict(
        kicker="Releases",
        title="Changelog",
        title_tag="flakestat changelog: every release",
        og_title="flakestat changelog",
        description="Every flakestat release and what changed in it, including the "
                    "behaviour changes that move existing scores.",
        keywords=["flakestat changelog", "flakestat releases", "flakestat versions"],
        lede="What changed in each release, and which changes move existing scores.",
        summary="Every release, and which changes move scores you have already "
                "recorded.",
    )),
]

_INTERNAL = {src: "/" + slug + "/" for slug, src, _ in DOCS}

MONTHS = ("January February March April May June July August September October "
          "November December").split()


def _updated(src: str) -> str:
    """Date of the last commit that touched the file."""
    try:
        out = subprocess.run(
            ["git", "-C", ROOT, "log", "-1", "--format=%cs", "--", src],
            capture_output=True, text=True, timeout=10).stdout.strip()
        y, m, d = out.split("-")
        return f"{int(d)} {MONTHS[int(m) - 1]} {y}"
    except Exception:
        return ""


def _rewrite_links(body: str, base: str, repo: str) -> str:
    """Point relative links at the right surface.

    A link to another of these documents becomes a site link. Anything else
    relative only exists in the repository, so it becomes a GitHub link rather
    than a 404.
    """
    def repl(m):
        href = html.unescape(m.group(1))
        if re.match(r"^(https?:|mailto:|#|/)", href):
            return m.group(0)
        path, _, frag = href.partition("#")
        if path in _INTERNAL:
            target = base.rstrip("/") + _INTERNAL[path]
            return 'href="%s%s"' % (target, "#" + frag if frag else "")
        return 'href="%s/blob/main/%s"' % (repo, href)

    return re.sub(r'href="([^"]+)"', repl, body)


def _wrap_tables(body: str) -> str:
    """Wide tables scroll inside themselves rather than pushing the page over."""
    return re.sub(r"(<table>.*?</table>)", r'<div class="tablewrap">\1</div>',
                  body, flags=re.S)


def _version_anchors(body: str) -> str:
    """Give each release a link somebody would want to paste.

    "## [0.2.0] - 2026-09-07" slugifies to "020-2026-09-07". #v0-2-0 is better.
    """
    def repl(m):
        inner = m.group(1)
        v = re.match(r"\s*(?:<a[^>]*>)?\s*v?(\d+\.\d+\.\d+)", inner)
        if not v:
            return m.group(0)
        return '<h2 id="v%s">%s</h2>' % (v.group(1).replace(".", "-"), inner)

    return re.sub(r"<h2>(.*?)</h2>", repl, body, flags=re.S)


def _strip_h1(text: str):
    """Drop the document's own title; the page shell renders one."""
    lines = text.splitlines()
    if lines and lines[0].startswith("# "):
        lines = lines[1:]
        while lines and not lines[0].strip():
            lines.pop(0)
    return "\n".join(lines)


def pages(base: str, repo: str):
    md = markdown.Markdown(extensions=["tables", "fenced_code", "sane_lists"])
    out = []
    for slug, src, meta in DOCS:
        with open(os.path.join(ROOT, src), encoding="utf-8") as f:
            text = _strip_h1(f.read())
        md.reset()
        body = _rewrite_links(md.convert(text), base, repo)
        if slug == "changelog":
            body = _version_anchors(body)
        body = _wrap_tables(body)

        updated = _updated(src)
        byline = (
            f'<p class="doc-meta"><span>{meta["kicker"]}</span>'
            + (f"<span>Updated {updated}</span>" if updated else "")
            + f'<a href="{repo}/blob/main/{src}">{src}</a></p>'
        )
        page = dict(meta)
        page.update(slug=slug, section="Writing", layout="doc",
                    body=byline + body, source=src, updated=updated)
        out.append(page)
    return out
