#!/usr/bin/env python3
"""Build the CloudBackup user guide as ONE self-contained HTML file.

Concatenates the guide chapters (docs/guide/*.md, in reading order) into a
single HTML document with embedded CSS, usable offline from a local disk and
printable to PDF from any browser. Cross-chapter Markdown links are rewritten
to in-document anchors.

Run by generate_docs.sh after the mkdocs build; output goes into the mkdocs
site dir so the daemon serves it at /docs/cloudbackup-user-guide.html.
Requires only the "markdown" package (a dependency of mkdocs itself).
"""

import datetime
import html
import pathlib
import re
import sys

import markdown

DOCS = pathlib.Path(__file__).parent / "docs" / "guide"
OUT = pathlib.Path(__file__).parent.parent / "webstatic" / "docs" / "cloudbackup-user-guide.html"

# Reading order; each file becomes a section with a stable anchor id.
CHAPTERS = [
    ("README.md", "guide-overview"),
    ("01-installation.md", "chapter-1"),
    ("02-getting-started.md", "chapter-2"),
    ("03-configuration.md", "chapter-3"),
    ("04-encryption.md", "chapter-4"),
    ("05-cli-setup.md", "chapter-5"),
    ("06-cli-usage.md", "chapter-6"),
    ("07-web-ui.md", "chapter-7"),
    ("08-http-api.md", "chapter-8"),
]

FILE_TO_ANCHOR = {fname: anchor for fname, anchor in CHAPTERS}

CSS = """
:root { --fg: #1a1a1a; --muted: #555; --border: #d0d0d0; --accent: #0b5d8a; }
* { box-sizing: border-box; }
body { color: var(--fg); font: 16px/1.6 Georgia, 'Times New Roman', serif;
       max-width: 54rem; margin: 0 auto; padding: 2rem 1.25rem 4rem; }
h1, h2, h3, h4 { font-family: Helvetica, Arial, sans-serif; line-height: 1.25; }
h1 { border-bottom: 3px solid var(--accent); padding-bottom: .3rem; margin-top: 3rem; }
h2 { border-bottom: 1px solid var(--border); padding-bottom: .2rem; margin-top: 2.2rem; }
a { color: var(--accent); }
code, pre { font-family: 'SF Mono', Consolas, Menlo, monospace; font-size: .85em; }
code { background: #f2f2f2; padding: .1em .3em; border-radius: 3px; }
pre { background: #f7f7f7; border: 1px solid var(--border); border-radius: 4px;
      padding: .8rem 1rem; overflow-x: auto; }
pre code { background: none; padding: 0; }
table { border-collapse: collapse; width: 100%; margin: 1rem 0; font-size: .92em; }
th, td { border: 1px solid var(--border); padding: .4rem .6rem; text-align: left;
         vertical-align: top; }
th { background: #f0f4f7; font-family: Helvetica, Arial, sans-serif; }
blockquote { border-left: 4px solid var(--accent); margin: 1rem 0; padding: .2rem 1rem;
             color: var(--muted); background: #f7fafc; }
.cover { text-align: center; margin: 4rem 0 5rem; }
.cover h1 { border: none; font-size: 2.4rem; }
.cover p { color: var(--muted); }
.toc { background: #f7fafc; border: 1px solid var(--border); border-radius: 6px;
       padding: 1rem 2rem; }
.toc ol { margin: .4rem 0; }
section.chapter { page-break-before: always; }
@media print {
  body { max-width: none; font-size: 11pt; }
  pre, blockquote, table, img { break-inside: avoid; }
  a { color: inherit; }
}
"""


def rewrite_links(text: str) -> str:
    """Turn cross-chapter file links into in-document anchor links."""

    def repl(m: re.Match) -> str:
        fname, frag = m.group(1), m.group(2)
        if fname not in FILE_TO_ANCHOR:
            return m.group(0)
        if frag:  # heading anchors survive concatenation (attr ids from TOC ext)
            return "](#%s)" % frag
        return "](#%s)" % FILE_TO_ANCHOR[fname]

    return re.sub(r"\]\((?!https?://)([\w.\-]+\.md)(?:#([\w\-]+))?\)", repl, text)


def main() -> int:
    md = markdown.Markdown(extensions=["tables", "fenced_code", "toc"])
    sections = []
    for fname, anchor in CHAPTERS:
        path = DOCS / fname
        body = md.reset().convert(rewrite_links(path.read_text(encoding="utf-8")))
        sections.append('<section class="chapter" id="%s">\n%s\n</section>' % (anchor, body))

    today = datetime.date.today().isoformat()
    doc = """<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>CloudBackup User Guide</title>
<style>%s</style>
</head>
<body>
<div class="cover">
  <h1>CloudBackup User Guide</h1>
  <p>Single-file offline edition &mdash; generated %s.<br>
  Latest version: <a href="https://github.com/alexandruionica/cloudbackup">github.com/alexandruionica/cloudbackup</a></p>
</div>
%s
</body>
</html>
""" % (CSS, html.escape(today), "\n".join(sections))

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(doc, encoding="utf-8")
    print("Wrote %s (%d KB)" % (OUT, OUT.stat().st_size // 1024))
    return 0


if __name__ == "__main__":
    sys.exit(main())
