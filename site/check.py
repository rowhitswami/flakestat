#!/usr/bin/env python3
"""Fail the build on a link that does not resolve.

Written after three links in the README and one in the site footer pointed at
pages that had been renamed or never committed. Every one of them was visible
to anybody who clicked, and none was visible to me. A fragment is the worst
case: the URL returns 200 and simply lands in the wrong place, so checking the
page exists is not enough — the anchor has to exist too.

Usage: python3 check.py <build output dir>
"""
import html
import os
import re
import sys

ORIGIN = "https://flakestat.com"


def pages(root):
    for dirpath, _, names in os.walk(root):
        for n in names:
            if n.endswith(".html"):
                path = os.path.join(dirpath, n)
                rel = "/" + os.path.relpath(path, root).replace(os.sep, "/")
                yield rel, path, open(path, encoding="utf-8").read()


def main():
    root = sys.argv[1] if len(sys.argv) > 1 else "out"
    docs, ids, problems = {}, {}, []

    for rel, path, text in pages(root):
        docs[rel] = text
        found = re.findall(r'\bid="([^"]+)"', text)
        dupes = {i for i in found if found.count(i) > 1}
        if dupes:
            problems.append(f"{rel}: duplicate id(s) {sorted(dupes)}")
        ids[rel] = set(found)

    def resolve(url):
        """Map a site URL to the file that serves it, or None."""
        url = url.split("?")[0]
        if url.startswith(ORIGIN):
            url = url[len(ORIGIN):] or "/"
        if not url.startswith("/"):
            return None
        cand = url if url.endswith(".html") else url.rstrip("/") + "/index.html"
        if cand == "/index.html" or cand in docs:
            return cand if cand in docs else None
        return None

    for rel, text in docs.items():
        refs = re.findall(r'(?:href|src)="([^"]+)"', text)
        for raw in refs:
            ref = html.unescape(raw)
            if ref.startswith(("mailto:", "data:", "javascript:")):
                continue
            if ref.startswith("http") and not ref.startswith(ORIGIN):
                continue  # off-site; not this script's business

            if ref.startswith(ORIGIN):
                ref = ref[len(ORIGIN):] or "/"
            target, _, frag = ref.partition("#")
            if not target:                       # same-page fragment
                if frag and frag not in ids[rel]:
                    problems.append(f"{rel}: #{frag} does not exist on this page")
                continue

            path = resolve(target)
            if path is None:
                # Assets and other non-HTML files are checked on disk.
                on_disk = os.path.join(root, target.lstrip("/"))
                if target.startswith("/") and os.path.isfile(on_disk):
                    continue
                problems.append(f"{rel}: -> {ref}  (no such page)")
                continue
            if frag and frag not in ids[path]:
                problems.append(f"{rel}: -> {ref}  (page exists, #{frag} does not)")

    print(f"checked {len(docs)} pages")
    if problems:
        for p in sorted(set(problems)):
            print("  BROKEN  " + p)
        print(f"\n{len(set(problems))} problem(s)")
        return 1
    print("  all internal links and fragments resolve")
    return 0


if __name__ == "__main__":
    sys.exit(main())
