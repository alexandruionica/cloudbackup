---
name: release-prep
description: Pre-release checklist for cloudbackup — runs the full test matrix, regenerates Swagger / docs, confirms the working tree is clean, and surfaces any remaining TODOs. Use when the user wants to cut a release or asks "is this branch ready to ship".
---

# Release prep checklist

Run these in order. Stop at the first failure and report it back; do not
proceed past a red step.

## 1. Working tree is clean
- `git status` — no uncommitted or untracked files (other than ignored).
- `git log <main>..HEAD` — verify the commit list matches what's intended
  for the release.

## 2. Build
- `make build` — must produce `./cloudbackup` with no warnings.
- `go build -mod=vendor ./...` — catches package-level breakage that
  the main binary alone may not.

## 3. Lint
- `make testcp` — `go fmt` is a no-op and golangci-lint reports `0 issues`.

## 4. Unit tests with race detection
- `make gotestrace` — all packages green except `objectstore` live tests
  that need `CLD_*` env vars (those failures are environmental, not
  release-blocking, but verify the message says exactly that).

## 5. Integration tests
- `./integration_tests.sh` (or `make inttest`) — all 118+ tests pass.
  If only `objectstore` cloud tests fail and the user has not provided
  `CLD_S3_BUCKET` / `CLD_AZURE_STORAGE_ACCOUNT` / `CLD_GCP_STORAGE_BUCKET`,
  surface that as a known gap before approving the release.

## 6. Docs are current
- `make docs` — rebuilds the static site under `webstatic/docs/` (mkdocs plus
  the single-file offline guide). It does **not** touch the Swagger spec.
  Diff the result; if files changed, the developer forgot to regenerate
  before committing — make a follow-up commit.
- Skim `documentation_src/docs/guide/03-configuration.md` against the current
  `shared/structs_config.go` — every yaml-tagged user-facing field should
  appear in the docs.
- `webstatic/docs_api/swagger.yaml` and `swagger.json` are hand-maintained in
  parallel and must stay equivalent; the `documentation` skill has the check.

## 7. Outstanding TODOs
- `grep -rn "TODO" --include='*.go' .` — list any TODOs touching code paths
  in this release. The author's discretion as to which are blockers.

## 8. Version bump
- Search for the version string in `httpd/misc_handlers.go` (the
  `/api/report/version` endpoint) and any package-level constants. Bump it
  in a single commit just before tagging.

## 9. Cut the tag
- Confirm with the user before doing this — tagging is a write to a shared
  remote and is hard to undo. Suggested: `git tag -a vX.Y.Z -m "..."` then
  `git push --tags` only after explicit user confirmation.

## What to report back
A one-screen punch list: each step ✓ or ✗, with the failing command + first
20 lines of output for any ✗. Do not paste full test logs.
