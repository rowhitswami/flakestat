#!/usr/bin/env python3
"""Static generator for the flakestat documentation site.

Emits one directory per page so URLs stay clean and stable, plus the machine
readable surfaces: sitemap.xml, robots.txt, a search index, and llms.txt /
llms-full.txt for agents that would rather read the whole corpus at one URL
than crawl twenty.

Everything absolute is built from BASE and ORIGIN, so moving to a custom domain
is a two-line change here plus a CNAME file.
"""
import html
import json
import os
import re
import shutil
import sys
from datetime import date

ORIGIN = "https://flakestat.com"
BASE = "/"
SITE = ORIGIN + BASE.rstrip("/")  # no trailing slash
REPO = "https://github.com/rowhitswami/flakestat"
# The action is pinned explicitly rather than to a floating major tag.
# Pre-1.0 means no compatibility promise, and this runs inside other
# people's CI - they should choose when to move.
ACTION_REF = "rowhitswami/flakestat@v0.2.0"
TODAY = date.today().isoformat()

OUT = sys.argv[1] if len(sys.argv) > 1 else "out"
HERE = os.path.dirname(os.path.abspath(__file__))


def url(path: str) -> str:
    """Site-absolute URL for a page slug."""
    return BASE + (path + "/" if path else "")


def canonical(path: str) -> str:
    return ORIGIN + url(path)


# --------------------------------------------------------------- navigation
NAV = [
    ("Docs", "docs"),
    ("Writing", "writing"),
    ("Changelog", "changelog"),
    ("Compare", "compare/trunk"),
]

# Every long-form page shares one sidebar, so a reader who arrives on the
# validation record can see the rest of the writing without going back up.
WRITING_NAV = [
    ("Writing", [
        ("All writing", "writing"),
        ("Validating a flaky-test detector", "writing/validating-a-flaky-test-detector"),
        ("How flakestat was validated", "validation"),
        ("Hunting flaky tests in open source", "findings"),
        ("Design notes", "design"),
        ("Changelog", "changelog"),
    ]),
    ("Documentation", [
        ("Full documentation", "docs"),
        ("Quickstart", "docs#quickstart"),
        ("How scoring works", "docs#scoring"),
    ]),
]

# The core documentation is one page. Splitting it made every move between
# sections a page load, which is the wrong trade for reference material people
# scan, search and Cmd-F through. The framework and comparison pages stay
# separate because they answer different questions for different searches, not
# because they are chapters of the same document.
DOCS_SLUG = "docs"

SIDEBAR = [
    ("Getting started", [
        ("Install", "#install"),
        ("Quickstart", "#quickstart"),
        ("Reading a report", "#reading-a-report"),
    ]),
    ("Guides", [
        ("Hunting locally", "#hunting"),
        ("History over time", "#history"),
        ("Recording context", "#dimensions"),
        ("Gating CI", "#ci-gate"),
        ("Unblocking the pipeline", "#quarantine"),
        ("Explaining a verdict", "#explain"),
    ]),
    ("Continuous integration", [
        ("GitHub Actions", "#github-actions"),
        ("GitLab and others", "#gitlab"),
    ]),
    ("Reference", [
        ("Commands", "#commands"),
        ("Configuration", "#config"),
        ("How scoring works", "#scoring"),
        ("Supported runners", "#runners"),
        ("FAQ", "#faq"),
    ]),
    ("By framework", [
        ("pytest", "flaky-tests/pytest"),
        ("Jest", "flaky-tests/jest"),
        ("go test", "flaky-tests/go"),
    ]),
    ("Alternatives", [
        ("vs Trunk", "compare/trunk"),
        ("vs BuildPulse", "compare/buildpulse"),
    ]),
]

# docs/<slug> in content.py maps onto an anchor on the one docs page.
def anchor_for(slug):
    return "#" + slug.split("/", 1)[1].replace("/", "-").replace("ci-", "", 1) \
        if slug.startswith("docs/ci/") else "#" + slug.split("/", 1)[1]


# ------------------------------------------------------------------- markup
def heading_ids(body: str, reserved=()):
    """Add stable ids and hover anchors to h2/h3, and collect a TOC.

    Ids must be unique or the fragment links silently land on the wrong place.
    Section ids are reserved and always win, because those are what the sidebar
    and every external link point at; a heading that would collide with one, or
    with an earlier heading, gets a numeric suffix.
    """
    toc = []
    taken = set(reserved)

    def slugify(text):
        s = re.sub(r"<[^>]+>", "", text)
        s = re.sub(r"[^\w\s-]", "", html.unescape(s)).strip().lower()
        return re.sub(r"[\s_]+", "-", s)

    def unique(base):
        if base not in taken:
            taken.add(base)
            return base
        n = 2
        while f"{base}-{n}" in taken:
            n += 1
        taken.add(f"{base}-{n}")
        return f"{base}-{n}"

    def repl(m):
        level, attrs, text = m.group(1), m.group(2) or "", m.group(3)
        if "id=" in attrs:
            hid = re.search(r'id="([^"]+)"', attrs).group(1)
            taken.add(hid)
        else:
            hid = unique(slugify(text))
            attrs += f' id="{hid}"'
        toc.append((level, hid, re.sub(r"<[^>]+>", "", text)))
        anchor = f'<a class="anchor" href="#{hid}" aria-label="Link to this section">#</a>'
        return f"<h{level}{attrs}>{text}{anchor}</h{level}>"

    body = re.sub(r"<h([23])([^>]*)>(.*?)</h\1>", repl, body, flags=re.S)
    return body, toc


def plain_text(body: str) -> str:
    t = re.sub(r"<(script|style)[^>]*>.*?</\1>", " ", body, flags=re.S)
    t = re.sub(r"<[^>]+>", " ", t)
    t = html.unescape(t)
    return re.sub(r"\s+", " ", t).strip()


def jsonld(page) -> str:
    graph = [{
        "@type": "SoftwareApplication",
        "@id": SITE + "/#software",
        "name": "flakestat",
        "applicationCategory": "DeveloperApplication",
        "applicationSubCategory": "Test automation",
        "operatingSystem": "macOS, Linux, Windows",
        "description": "Open-source CLI that detects flaky tests from JUnit XML in any language. Runs locally with no account and no data leaving your machine.",
        "url": SITE + "/",
        "downloadUrl": REPO + "/releases",
        "softwareVersion": "0.2.0",
        "license": "https://opensource.org/licenses/MIT",
        "isAccessibleForFree": True,
        "offers": {"@type": "Offer", "price": "0", "priceCurrency": "USD"},
        "author": {"@type": "Person", "name": "Rohit Swami", "url": "https://github.com/rowhitswami"},
        "programmingLanguage": "Go",
        "featureList": [
            "Detects flaky tests from JUnit XML produced by any test runner",
            "Scores state transitions rather than failure rate, so broken tests are not mistaken for flaky ones",
            "Reports a confidence level with every verdict",
            "Runs entirely locally with no account and no data egress",
            "Fails CI only when flakiness gets worse, via a committed baseline",
            "Emits quarantine skip lists for pytest, Jest and go test",
        ],
    }, {
        "@type": "WebSite",
        "@id": SITE + "/#website",
        "url": SITE + "/",
        "name": "flakestat",
        "description": "Documentation for flakestat, an open-source flaky test detector.",
        "publisher": {"@id": SITE + "/#person"},
    }, {
        "@type": "Person",
        "@id": SITE + "/#person",
        "name": "Rohit Swami",
        "url": "https://github.com/rowhitswami",
    }]

    if page["slug"]:
        crumbs = [{"@type": "ListItem", "position": 1, "name": "flakestat", "item": SITE + "/"}]
        parts = page["slug"].split("/")
        for i, part in enumerate(parts, start=2):
            crumbs.append({
                "@type": "ListItem", "position": i,
                "name": page["title"] if i == len(parts) + 1 else part.replace("-", " ").title(),
                "item": canonical("/".join(parts[: i - 1])),
            })
        graph.append({"@type": "BreadcrumbList", "@id": canonical(page["slug"]) + "#crumbs",
                      "itemListElement": crumbs})
        graph.append({
            "@type": "TechArticle",
            "@id": canonical(page["slug"]) + "#article",
            "headline": page["title"],
            "description": page["description"],
            "url": canonical(page["slug"]),
            "datePublished": "2026-09-06",
            "dateModified": TODAY,
            "author": {"@id": SITE + "/#person"},
            "about": {"@id": SITE + "/#software"},
            "inLanguage": "en",
        })

    if page.get("faq"):
        graph.append({
            "@type": "FAQPage",
            "@id": canonical(page["slug"]) + "#faq",
            "mainEntity": [
                {"@type": "Question", "name": q,
                 "acceptedAnswer": {"@type": "Answer", "text": a}}
                for q, a in page["faq"]
            ],
        })

    return json.dumps({"@context": "https://schema.org", "@graph": graph},
                      indent=None, separators=(",", ":"))


def doc_search_rows(page):
    """One search row per h2 of a long document.

    A single row for a 600-line record is close to useless: the query matches,
    the reader lands at the top, and then has to find the bit that matched.
    Splitting on h2 means a hit opens at the section that actually answers it.
    """
    body, _ = heading_ids(page["body"])
    heads = list(re.finditer(r'<h2[^>]*\bid="([^"]+)"[^>]*>(.*?)</h2>', body, flags=re.S))
    rows = []
    for i, m in enumerate(heads):
        end = heads[i + 1].start() if i + 1 < len(heads) else len(body)
        text = re.sub(r'<a class="anchor".*?</a>', "", m.group(2), flags=re.S)
        title = re.sub(r"<[^>]+>", "", text).strip()
        title = re.sub(r"^\d+\.\s+", "", title)   # "6. Scoring" -> "Scoring"
        rows.append({
            "url": f'{page["slug"]}/#{m.group(1)}',
            "title": title or page["title"],
            "section": page["title"],
            "text": plain_text(body[m.end():end])[:1200],
            "keywords": " ".join(page.get("keywords", [])).lower(),
        })
    return rows


def sidebar_html(active: str, tree=None, label="Documentation") -> str:
    out = [f'<nav class="sidebar" aria-label="{label}">']
    for group, items in (tree if tree is not None else SIDEBAR):
        out.append(f'<div class="sb-group"><h5>{group}</h5>')
        for text, target in items:
            label = text
            if target.startswith("#"):
                href = (target if active == DOCS_SLUG else url(DOCS_SLUG) + target)
                out.append(f'<a href="{href}" data-anchor="{target[1:]}">{label}</a>')
            else:
                slug, _, frag = target.partition("#")
                cur = ' aria-current="page"' if slug == active and not frag else ""
                href = url(slug) + ("#" + frag if frag else "")
                out.append(f'<a href="{href}"{cur}>{label}</a>')
        out.append("</div>")
    out.append("</nav>")
    return "".join(out)


def toc_html(toc) -> str:
    if len(toc) < 2:
        return '<div class="toc"></div>'
    items = "".join(
        f'<a class="lvl{lvl}" href="#{hid}">{html.escape(text)}</a>'
        for lvl, hid, text in toc
    )
    return f'<div class="toc"><h6>On this page</h6>{items}</div>'


def pager_html(slug: str) -> str:
    """Cross-links out of the standalone pages, back into the docs."""
    if slug.startswith(("flaky-tests/", "compare/")):
        return ('<nav class="pager">'
                f'<a href="{url(DOCS_SLUG)}"><span>Reference</span><b>Full documentation</b></a>'
                f'<a class="next" href="{url(DOCS_SLUG)}#quickstart"><span>Get going</span>'
                f'<b>Quickstart</b></a></nav>')
    return ""


SHELL = """<!doctype html>
<html lang="en" data-base="{base}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title_tag}</title>
<meta name="description" content="{description}">
<link rel="canonical" href="{canonical}">
<meta name="robots" content="index, follow, max-image-preview:large, max-snippet:-1">
<meta name="author" content="Rohit Swami">
{keywords}
<meta property="og:type" content="{og_type}">
<meta property="og:site_name" content="flakestat">
<meta property="og:title" content="{og_title}">
<meta property="og:description" content="{description}">
<meta property="og:url" content="{canonical}">
<meta property="og:image" content="{origin}{base}assets/social-card.png">
<meta property="og:image:width" content="1280">
<meta property="og:image:height" content="640">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="{og_title}">
<meta name="twitter:description" content="{description}">
<meta name="twitter:image" content="{origin}{base}assets/social-card.png">
<meta name="theme-color" content="#08142A" media="(prefers-color-scheme: dark)">
<meta name="theme-color" content="#ffffff" media="(prefers-color-scheme: light)">
<link rel="icon" href="{base}assets/favicon.png">
<link rel="apple-touch-icon" href="{base}assets/logo-mark.png">
<link rel="alternate" type="text/plain" href="{origin}{base}llms.txt" title="llms.txt">
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link rel="preload" as="style" href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap">
<link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap">
<link rel="stylesheet" href="{base}theme.css">
<script>(function(){{try{{var t=localStorage.getItem('flakestat-theme');if(t)document.documentElement.setAttribute('data-theme',t);}}catch(e){{}}}})();</script>
<script type="application/ld+json">{jsonld}</script>
</head>
<body>
<div class="progress" id="fs-progress"></div>

<header class="nav">
  <div class="nav-in">
    <button class="ibtn menu-btn" id="fs-menu" aria-label="Toggle navigation" aria-expanded="false">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M3 12h18M3 18h18"/></svg>
    </button>
    <a class="brand" href="{base}" aria-label="flakestat home">
      <img src="{base}assets/logo-text.png" alt="flakestat" width="186" height="30"
           data-light="{base}assets/logo-text.png" data-dark="{base}assets/logo-text-dark.png">
    </a>
    <nav class="nav-links" aria-label="Main">{navlinks}</nav>
    <div class="nav-right">
      <button class="searchbtn" data-search aria-label="Search documentation">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" width="15" height="15"><circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/></svg>
        <span>Search docs</span><kbd>⌘K</kbd>
      </button>
      <a class="ibtn" href="{repo}" aria-label="GitHub repository">
        <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 .5a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2c-3.3.7-4-1.6-4-1.6-.6-1.4-1.4-1.8-1.4-1.8-1-.7.1-.7.1-.7 1.2.1 1.8 1.2 1.8 1.2 1 1.8 2.8 1.3 3.5 1 .1-.8.4-1.3.7-1.6-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2-.1-.3-.5-1.5.1-3.2 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0C17.3 4.7 18.3 5 18.3 5c.6 1.7.2 2.9.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.4.4.8 1.1.8 2.2v3.3c0 .3.2.7.8.6A12 12 0 0 0 12 .5Z"/></svg>
      </a>
      <button class="ibtn" id="fs-theme" aria-label="Toggle dark mode">◐</button>
    </div>
  </div>
</header>

{body}

<footer>
  <div class="foot-in">
    <div>
      <img class="foot-lockup" src="{base}assets/logo-lockup.png" width="247" height="62" alt="flakestat, find flaky tests in any language"
           data-light="{base}assets/logo-lockup.png" data-dark="{base}assets/logo-lockup-dark.png">
      <p>An open-source flaky test detector. One static binary, no SaaS, no account, and your test results never leave your machine.</p>
    </div>
    <div class="foot-col"><h6>Docs</h6>
      <a href="{base}docs/#install">Install</a>
      <a href="{base}docs/#quickstart">Quickstart</a>
      <a href="{base}docs/#commands">Commands</a>
      <a href="{base}docs/#scoring">How scoring works</a>
      <a href="{base}docs/#runners">Supported runners</a>
    </div>
    <div class="foot-col"><h6>Guides</h6>
      <a href="{base}docs/#github-actions">GitHub Actions</a>
      <a href="{base}docs/#history">Tracking over time</a>
      <a href="{base}docs/#ci-gate">Gating CI</a>
      <a href="{base}flaky-tests/pytest/">Flaky pytest tests</a>
      <a href="{base}flaky-tests/jest/">Flaky Jest tests</a>
      <a href="{base}flaky-tests/go/">Flaky Go tests</a>
    </div>
    <div class="foot-col"><h6>Writing</h6>
      <a href="{base}writing/">All writing</a>
      <a href="{base}validation/">Validation record</a>
      <a href="{base}findings/">Flake hunt findings</a>
      <a href="{base}design/">Design notes</a>
      <a href="{base}changelog/">Changelog</a>
    </div>
    <div class="foot-col"><h6>Project</h6>
      <a href="{repo}">GitHub</a>
      <a href="{repo}/releases">Releases</a>
      <a href="{repo}/issues">Issues</a>
      <a href="{base}docs/#faq">FAQ</a>
      <a href="{origin}{base}llms.txt">llms.txt</a>
    </div>
  </div>
  <div class="foot-btm"><div>MIT licensed · Built by <a href="https://github.com/rowhitswami">Rohit Swami</a></div></div>
</footer>

<div class="scrim" id="fs-scrim" hidden role="dialog" aria-modal="true" aria-label="Search documentation">
  <div class="palette">
    <input id="fs-q" type="search" placeholder="Search the docs…" autocomplete="off" spellcheck="false" aria-label="Search query">
    <div class="results" id="fs-results"></div>
    <div class="p-foot"><span><kbd>↑</kbd><kbd>↓</kbd> navigate</span><span><kbd>↵</kbd> open</span><span><kbd>esc</kbd> close</span></div>
  </div>
</div>

<script src="{base}app.js" defer></script>
</body>
</html>
"""


# Ids used by the page chrome itself. Reserved so a document heading that
# slugifies to one of them gets a suffix instead of producing two elements with
# the same id - which is what put a #results anchor on the search palette.
CHROME_IDS = ["fs-progress", "fs-menu", "fs-scrim", "fs-q", "fs-results", "fs-theme"]


def render(page):
    # Section ids and the shell's own control ids are both navigation targets.
    reserved = re.findall(r'<section id="([^"]+)"', page["body"]) + CHROME_IDS
    if page.get("layout") == "index":
        body, toc = page["body"], []
    else:
        body, toc = heading_ids(page["body"], reserved)
    slug = page["slug"]

    two_col = slug == DOCS_SLUG

    if page.get("layout") == "post":
        inner = f'<div class="post-wrap">{body}</div>'
    elif page.get("layout") in ("wide", "index"):
        inner = body
    elif page.get("layout") == "doc":
        inner = (
            f'<div class="shell">{sidebar_html(slug, WRITING_NAV, "Writing")}'
            f'<article class="content content-doc">'
            f'<nav class="crumbs" aria-label="Breadcrumb"><a href="{url("")}">Home</a>'
            f'<span>/</span><a href="{url("writing")}">Writing</a><span>/</span>'
            f'<span>{html.escape(page["title"])}</span></nav>'
            f'<h1>{html.escape(page["title"])}</h1>'
            f'<p class="lede">{page["lede"]}</p>'
            f'{body}</article>{toc_html(toc)}</div>'
        )
    else:
        crumbs = ['<nav class="crumbs" aria-label="Breadcrumb">',
                  f'<a href="{url("")}">Home</a><span>/</span>']
        parts = slug.split("/")
        if len(parts) > 1:
            crumbs.append(f'<span>{parts[0].replace("-", " ").title()}</span><span>/</span>')
        crumbs.append(f'<span>{html.escape(page["title"])}</span></nav>')
        inner = (
            f'<div class="shell{" shell-2col" if two_col else ""}">{sidebar_html(slug)}'
            f'<article class="content{" content-wide" if two_col else ""}">{"".join(crumbs)}'
            f'<h1>{html.escape(page["title"])}</h1>'
            f'<p class="lede">{page["lede"]}</p>'
            f"{body}{pager_html(slug)}</article>"
            f"{'' if two_col else toc_html(toc)}</div>"
        )

    navlinks = "".join(f'<a href="{url(s)}">{t}</a>' for t, s in NAV)
    title_tag = page.get("title_tag") or f'{page["title"]} · flakestat'
    kw = page.get("keywords", [])
    keywords = f'<meta name="keywords" content="{html.escape(", ".join(kw))}">' if kw else ""

    return SHELL.format(
        base=BASE, origin=ORIGIN, repo=REPO,
        title_tag=html.escape(title_tag),
        description=html.escape(page["description"]),
        og_title=html.escape(page.get("og_title") or page["title"]),
        og_type="website" if not slug else "article",
        canonical=canonical(slug),
        keywords=keywords,
        jsonld=jsonld(page),
        navlinks=navlinks,
        body=inner,
    )


def write(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)


def main():
    sys.path.insert(0, HERE)
    import content

    pages = content.pages(BASE, REPO, ACTION_REF)
    import writing
    essay = writing.post(BASE, REPO)
    import mddocs
    md_pages = mddocs.pages(BASE, REPO)
    long_form = [essay] + md_pages
    pages += long_form
    pages.append(writing.index(BASE, REPO, long_form))
    if os.path.isdir(OUT):
        shutil.rmtree(OUT)
    os.makedirs(OUT)

    docs_parts = [p for p in pages if p["slug"].startswith("docs/")]
    others = [p for p in pages if not p["slug"].startswith("docs/")]

    # ---------------------------------------------- assemble the docs page
    order = [t[1:] for _, items in SIDEBAR for _, t in items if t.startswith("#")]
    by_anchor = {}
    for part in docs_parts:
        tail = part["slug"].split("/", 1)[1]
        by_anchor[tail.split("/")[-1] if tail.startswith("ci/") else tail] = part

    sections, index_rows, faq_pairs = [], [], []
    for anchor in order:
        part = by_anchor.get(anchor)
        if not part:
            continue
        faq_pairs += part.get("faq", [])
        sections.append(
            f'<section id="{anchor}">'
            f'<h2>{html.escape(part["title"])}</h2>'
            f'<p class="lede">{part["lede"]}</p>{part["body"]}</section>'
        )
        # One search row per section, so a hit lands on the right anchor rather
        # than the top of a very long page.
        index_rows.append({
            "url": f"{DOCS_SLUG}/#{anchor}",
            "title": part["title"],
            "section": part.get("section", "Docs"),
            "text": plain_text(part["body"])[:1200],
            "keywords": " ".join(part.get("keywords", [])).lower(),
        })

    docs_page = {
        "slug": DOCS_SLUG, "section": "Docs", "layout": "docs",
        "title": "Documentation",
        "title_tag": "flakestat documentation: find flaky tests in any language",
        "description": "Complete flakestat documentation: install, quickstart, CI recipes, "
                       "every command and flag, how scoring works, and supported test runners.",
        "keywords": ["flakestat documentation", "flaky test detection", "flaky test cli"],
        "lede": "Everything on one page. Use <kbd>⌘K</kbd> to search, or the contents on the left.",
        "body": "".join(sections),
        "faq": faq_pairs,
    }

    for page in [docs_page] + others:
        slug = page["slug"]
        out = os.path.join(OUT, slug, "index.html") if slug else os.path.join(OUT, "index.html")
        write(out, render(page))
        if page.get("layout") == "doc":
            index_rows.append({
                "url": slug + "/",
                "title": page["title"],
                "section": page.get("section", "Docs"),
                "text": plain_text(page["body"])[:1200],
                "keywords": " ".join(page.get("keywords", [])).lower(),
            })
            index_rows += doc_search_rows(page)
        elif slug != DOCS_SLUG:
            index_rows.append({
                "url": (slug + "/") if slug else "",
                "title": page["title"],
                "section": page.get("section", "Docs"),
                "text": plain_text(page["body"])[:1200],
                "keywords": " ".join(page.get("keywords", [])).lower(),
            })

    write(os.path.join(OUT, "search-index.json"), json.dumps(index_rows, separators=(",", ":")))

    # ------------------------------------------------------------- sitemap
    routes = [""] + [DOCS_SLUG] + [p["slug"] for p in others if p["slug"]]
    urls = []
    for r in routes:
        pri = "1.0" if not r else (
            "0.9" if r == DOCS_SLUG or r.startswith("flaky-tests")
            else "0.8" if r in ("validation", "findings", "design", "changelog", "writing")
            else "0.7")
        urls.append(f"<url><loc>{canonical(r)}</loc><lastmod>{TODAY}</lastmod>"
                    f"<changefreq>weekly</changefreq><priority>{pri}</priority></url>")
    write(os.path.join(OUT, "sitemap.xml"),
          '<?xml version="1.0" encoding="UTF-8"?>\n'
          '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n'
          + "\n".join(urls) + "\n</urlset>\n")

    write(os.path.join(OUT, "robots.txt"),
          "# flakestat documentation\n"
          "User-agent: *\nAllow: /\n\n"
          "# Agents are welcome. llms-full.txt is the whole corpus in one file.\n"
          + "".join(f"User-agent: {a}\nAllow: /\n" for a in [
              "GPTBot", "OAI-SearchBot", "ChatGPT-User", "ClaudeBot", "Claude-Web",
              "anthropic-ai", "PerplexityBot", "Google-Extended", "Applebot-Extended", "CCBot"])
          + f"\nSitemap: {SITE}/sitemap.xml\n")

    # ------------------------------------------------- llms.txt (the index)
    llms = [
        "# flakestat", "",
        "> Open-source command line tool that finds flaky tests in any language by "
        "reading the JUnit XML a test runner already writes. One static binary, no "
        "account, no server, and no test data leaves the machine it runs on. "
        "MIT licensed, written in Go, zero dependencies.", "",
        "flakestat scores how often a test disagrees with itself between runs rather "
        "than how often it fails, so a test that fails every time is reported as "
        "`consistently-failing` and scores zero instead of being ranked alongside real "
        "flakes. Every verdict carries a confidence level derived from how much "
        "evidence exists, and a small sample can never reach high confidence.", "",
        "Install: `brew install rowhitswami/tap/flakestat`, `npm install --save-dev flakestat`, "
        "`pip install flakestat`, or `go install github.com/rowhitswami/flakestat/cmd/flakestat@latest`.",
        "", "## Documentation", "",
        f"- [Full documentation]({canonical(DOCS_SLUG)}): every section on one page.",
    ]
    for anchor in order:
        part = by_anchor.get(anchor)
        if part:
            llms.append(f"  - [{part['title']}]({canonical(DOCS_SLUG)}#{anchor}): {part['description']}")
    llms += ["", "## By framework", ""]
    for p in others:
        if p["slug"].startswith(("flaky-tests/", "compare/")):
            llms.append(f"- [{p['title']}]({canonical(p['slug'])}): {p['description']}")
    llms += ["", "## Writing", "",
             f"- [Index of everything long-form]({canonical('writing')})"]
    for p in long_form:
        llms.append(f"- [{p['title']}]({canonical(p['slug'])}): {p['description']}")
    llms += ["", "## Optional", "",
             f"- [Full documentation as one plain-text file]({SITE}/llms-full.txt)",
             f"- [Source and issues]({REPO})", ""]
    write(os.path.join(OUT, "llms.txt"), "\n".join(llms))

    # -------------------------------------------- llms-full.txt (the corpus)
    full = ["# flakestat: complete documentation", "",
            f"Source: {SITE}/ · Generated {TODAY} · MIT licensed", "",
            "flakestat finds flaky tests in any language from JUnit XML. It runs locally "
            "as a single static binary with no account and no data egress.", "", "=" * 78, ""]
    for anchor in order:
        part = by_anchor.get(anchor)
        if part:
            full += [f"## {part['title']}", f"URL: {canonical(DOCS_SLUG)}#{anchor}", "",
                     plain_text(part["body"]), "", "-" * 78, ""]
    for p in others:
        if p["slug"]:
            full += [f"## {p['title']}", f"URL: {canonical(p['slug'])}", "",
                     plain_text(p["body"]), "", "-" * 78, ""]
    write(os.path.join(OUT, "llms-full.txt"), "\n".join(full))

    for asset in ("theme.css", "app.js", "CNAME", ".nojekyll"):
        shutil.copy(os.path.join(HERE, asset), os.path.join(OUT, asset))
    shutil.copytree(os.path.join(HERE, "assets"), os.path.join(OUT, "assets"))

    print(f"{len(others) + 1} pages → {OUT} (docs folded from {len(docs_parts)} sections)")
    print(f"  search-index.json  {len(index_rows)} entries")
    print(f"  llms-full.txt      {os.path.getsize(os.path.join(OUT,'llms-full.txt'))//1024}K")


if __name__ == "__main__":
    main()
