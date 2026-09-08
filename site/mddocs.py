#!/usr/bin/env python3
"""Render the repository's long-form Markdown as pages on the site.

These four documents are the project's evidence and its record of intent. They
have to exist in the repository, because that is where they are reviewable and
where their commit timestamps mean something — VALIDATION.md is only worth
reading because git can prove when it was written. But a raw .md on GitHub is a
poor way to *read* 600 lines, and the site had been linking out to files that
readers then had to squint at, or that went missing.

So: one source, two surfaces. The Markdown in the repository is authoritative
and these pages are generated from it at build time. Nothing is retyped, which
means the site cannot drift from the record the way hand-copied prose does.
"""
import html
import os
import re

import markdown

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)

# slug -> (source file, page metadata). Order is the order they appear in the
# sidebar and in llms.txt.
DOCS = [
    ("validation", "VALIDATION.md", dict(
        title="Validation",
        title_tag="How flakestat was validated — protocol, subjects and results",
        og_title="How flakestat was validated",
        description="The complete validation record for flakestat: a protocol registered "
                    "before any experiment ran, three third-party subjects, every outcome "
                    "including the arm that found nothing and a contaminated run that was "
                    "thrown away.",
        keywords=["flaky test detector validation", "flaky test detection accuracy",
                  "how to validate a flaky test tool", "flakestat validation"],
        lede="A detector that only reports what you hoped to find is not a detector. "
             "This is the contract that was written before the experiments, and what "
             "they returned.",
    )),
    ("findings", "HUNT-FINDINGS.md", dict(
        title="Flake hunt findings",
        title_tag="Hunting flaky tests in five open-source Go projects — flakestat",
        og_title="Hunting flaky tests in five open-source projects",
        description="What happened when flakestat was pointed at five active open-source Go "
                    "projects overnight: nine real flaky tests, seven already filed by their "
                    "maintainers, and zero previously-unknown flakes.",
        keywords=["flaky tests in open source", "hashicorp raft flaky test",
                  "litestream flaky test", "go test count shuffle flaky"],
        lede="Nine real flaky tests across five projects. Seven were already filed. "
             "The two that were not turned out not to be flaky tests at all.",
    )),
    ("design", "DESIGN.md", dict(
        title="Design notes",
        title_tag="flakestat design notes — how the scoring actually works",
        og_title="flakestat design notes",
        description="Why flakestat scores inconsistency rather than failure rate, how "
                    "execution context is compared, what the confidence level is derived "
                    "from, and the questions still open.",
        keywords=["flaky test scoring algorithm", "wilson score flaky test",
                  "flaky test detection design", "junit xml parsing"],
        lede="The reasoning behind the scoring, the storage format and the CLI — "
             "including the parts that are still guesses.",
    )),
    ("changelog", "CHANGELOG.md", dict(
        title="Changelog",
        title_tag="flakestat changelog — every release",
        og_title="flakestat changelog",
        description="Every flakestat release and what changed in it, including behaviour "
                    "changes that affect existing scores.",
        keywords=["flakestat changelog", "flakestat releases", "flakestat versions"],
        lede="What changed in each release, and which changes move existing scores.",
    )),
]

# Cross-document links inside the Markdown resolve to these pages instead of to
# the raw files, so a reader who arrives on the site stays on it.
_INTERNAL = {src: "/" + slug + "/" for slug, src, _ in DOCS}


def _rewrite_links(body: str, base: str, repo: str) -> str:
    """Point relative links at the right surface.

    A link to another of these documents becomes a site link. Anything else
    relative — a script, a workflow, a package — only exists in the repository,
    so it becomes a GitHub link rather than a 404.
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
    """Wide tables must scroll inside themselves, not push the page sideways."""
    return re.sub(r"(<table>.*?</table>)", r'<div class="tablewrap">\1</div>',
                  body, flags=re.S)


def _version_anchors(body: str) -> str:
    """Give each release a linkable id.

    "## [0.2.0] - 2026-09-07" would otherwise slugify to "020-2026-09-07",
    which nobody would paste into a message. #v0-2-0 is the link people want.
    """
    def repl(m):
        inner = m.group(1)
        # The version is a reference link to the GitHub release, so look past
        # the anchor tag for the number itself.
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
        path = os.path.join(ROOT, src)
        with open(path, encoding="utf-8") as f:
            text = _strip_h1(f.read())
        md.reset()
        body = _rewrite_links(md.convert(text), base, repo)
        if slug == "changelog":
            body = _version_anchors(body)
        body = _wrap_tables(body)
        source = (
            '<p class="src-note">Generated from '
            f'<a href="{repo}/blob/main/{src}"><code>{src}</code></a> in the repository, '
            'which is the copy of record — its history and timestamps are checkable there.'
            "</p>"
        )
        page = dict(meta)
        page.update(slug=slug, section="Evidence", layout="doc",
                    body=source + body, source=src)
        out.append(page)
    return out
