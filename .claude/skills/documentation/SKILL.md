---
name: documentation
description: How to edit, build and verify cloudbackup's documentation — the user guide (documentation_src/), the developer docs (developer_documentation/), and the hand-maintained Swagger API spec (webstatic/docs_api/). Use whenever a task adds or changes user-facing docs, adds a guide chapter, updates the API reference, or when a config/CLI/API change needs a docs follow-up.
---

# Managing documentation in this repository

## The three documentation sets

| Set | Source | Built output (tracked in git) | Served at |
|-----|--------|-------------------------------|-----------|
| **User guide** | `documentation_src/docs/` | `webstatic/docs/` | `/docs/` |
| **Developer docs** | `developer_documentation/docs/` | `developer_documentation/site/` | not served |
| **API reference** | `webstatic/docs_api/swagger.yaml` + `swagger.json` | — (hand-maintained) | `/docs_api/` |

The user guide proper lives in `documentation_src/docs/guide/` as nine Markdown
files (`README.md` + `01-`…`08-`). They are written to render three ways from
one source: on GitHub, as the mkdocs site the daemon serves, and as a single
offline file `webstatic/docs/cloudbackup-user-guide.html`.

## Build commands

```bash
./generate_docs.sh          # or: make docs — builds mkdocs site + offline HTML
./serve_live_docs.sh        # live-reloading preview on http://127.0.0.1:8000
```

`generate_docs.sh` runs mkdocs, then `documentation_src/build_offline_guide.py`
to produce the single-file HTML. Both write into `webstatic/docs/`.

`make docs` runs **only** `generate_docs.sh`. Despite what CLAUDE.md's command
list implies, it does **not** touch the Swagger spec — there is no Swagger
codegen in this repo at all.

## Rules that will bite you

**Built output is tracked in git.** `webstatic/docs/` is committed, not
generated at install time — the packages ship it verbatim. Any source edit must
be followed by `./generate_docs.sh` and the regenerated files committed in the
same change, or the served docs silently disagree with the Markdown.

**Heading anchors must slugify identically on GitHub and in mkdocs.** They do
not agree on punctuation that sits *between two spaces*: Python-Markdown drops
it and collapses to one hyphen, GitHub drops it and leaves two.

| Heading | mkdocs id | GitHub id |
|---------|-----------|-----------|
| ``3.2 `user` — API users`` | `32-user-api-users` | `32-user--api-users` |
| ``3.2 API users (`user`)`` | `32-api-users-user` | `32-api-users-user` ✓ |

So never put ` — `, ` & `, or ` / ` in a heading that anything links to. Move
the punctuation against a word (`(user)`) or drop it. A cross-chapter link with
the wrong slug still *builds*; mkdocs only emits an `INFO` line, so the broken
anchor ships unless you read the build output.

**Three files must agree when adding or renaming a chapter.** Miss one and the
chapter is invisible in that output:
1. `documentation_src/docs/guide/NN-name.md` — the source
2. `documentation_src/mkdocs.yml` — the `nav:` entry
3. `documentation_src/build_offline_guide.py` — the `CHAPTERS` list (ordered;
   each entry maps a filename to its in-document anchor)

Also add it to the tables in `guide/README.md` and `documentation_src/docs/index.md`.

**Cross-chapter links are relative `.md` links** (`[chapter 3](03-configuration.md)`,
optionally `#anchor`). That form works on GitHub, mkdocs rewrites it, and
`build_offline_guide.py` rewrites it to an in-document anchor. Do not use
absolute paths or bare `.html`.

**`webstatic/docs/cloudbackup-user-guide.html` triggers a build WARNING** —
mkdocs does not know about a file produced after it runs. Expected; ignore it.
Any *other* warning is real.

## The Swagger spec is hand-maintained

`swagger.yaml` and `swagger.json` are both edited by hand and must be kept in
sync — the Swagger UI loads **`swagger.json`**, so a yaml-only edit changes
nothing that a user sees. Verify equivalence after editing:

```bash
documentation_src/.venv/bin/python -c "
import json, yaml
y = yaml.safe_load(open('webstatic/docs_api/swagger.yaml'))
j = json.load(open('webstatic/docs_api/swagger.json'))
print('in sync' if y == j else 'DIVERGED')"
```

One benign false positive: an unquoted ISO date in the yaml is parsed as a
`datetime` but is a string in the json. Only `BuildDate.example` is affected.

`shared/structs_config.go` carries comments saying any struct change *requires*
a matching Swagger update — honour those when changing config structs.

## Document what the code does, not what it looks like it does

Config fields can exist, validate, and do nothing. Verify against the
implementation before describing behaviour — `grep` for the field outside
`structs_config.go` and the sample configs; if nothing reads it, say so.
Known live example: `versions_max_num` / `versions_max_age` are parsed and
validated but **no retention is enforced**, and the guide says exactly that.

## The venv breaks when the OS upgrades Python

`documentation_src/.venv` is pinned to the Python minor version it was built
with. After an OS Python upgrade, `.venv/bin/python` resolves to the new
interpreter, which looks in `lib/python3.<new>/site-packages` while the
packages sit in `lib/python3.<old>/` — everything reports as missing
(`No module named 'mkdocs'` / `'pip'`). `generate_docs.sh` only bootstraps when
`.venv/bin/python` is *absent*, so a version-mismatched venv sails past the
check and fails confusingly later.

Fix: `rm -rf documentation_src/.venv && ./generate_docs.sh` (it recreates it).
Diagnose with `.venv/bin/python --version` against `.venv/pyvenv.cfg`'s
`version_info`.

## Verify before reporting done

```bash
./generate_docs.sh 2>&1 | grep -iE "warning|error"   # only the known one above
grep -rn "](.*\.md" documentation_src/docs/guide/    # links resolve
git status --short webstatic/docs/                   # regenerated output staged
```

For a user-visible claim you are unsure of, read the implementation rather than
the sample config — the sample configs contain aspirational settings.
