---
name: add-config-field
description: Add a new field to one of the YAML config structs (CfgTemplate, ConfigBackup, ConfigBackupTarget, ConfigUser, etc.) — including yaml tag, deep-copy, validator, default, Swagger entry, and password sanitisation if it holds a secret. Use when the user asks to expose a new configurable knob.
---

# Add a config field

Config in this repo is parsed by `configor` from `config.yaml`, lives in
`shared/structs_config.go`, and is concurrency-protected by the
`GetCopyWithLock(...)` pattern. Missing any one of the steps below will leave
the field silently broken — usually the field will be parsed but not survive
a config copy, or it will be ignored on reload.

## Inputs to ask for if not given
- Which struct does the field belong to? (`CfgTemplate`, `ConfigBackup`,
  `ConfigBackupTarget`, `ConfigUser`, `ConfigHttp`/`ConfigHttps`,
  `ConfigNotification*`)
- Field name, type, default value, whether it's required.
- Is it a secret? (Affects `ParemtersWithSecrets` and `SaveSanitizedCfgToTmpFile`.)
- Is it user-facing in the API / Web UI?

## Steps

1. **Add the field** in `shared/structs_config.go`. Use existing tags as
   templates: `default:"..."`, `required:"true"`, `yaml:"..."`, `json:"..."`.

2. **Deep-copy it**. If the field is a slice, map, or pointer, add a copy line
   to `CopyConfigBackupStruct` (for `ConfigBackup`) or to `GetCopyWithLock`
   directly (for `CfgTemplate`). Scalar types are copied by the assignment
   `cfgCopy := cfg.Config` already; slices / maps / pointers are NOT.

3. **Validate**. Add validation to the matching `Validate*` function in
   `config/config.go`. Validators are called both at startup and on
   `SIGHUP` reload, so reject bad values explicitly with
   `errors.New(...)`.

4. **Default it**. If the field has a sensible default not expressible via
   the `default:` tag (e.g. derived from another field), set it in `Load`
   in `config/config.go` after `configor.Load` returns.

5. **Sanitise if secret**. Add the field name to
   `config/config.go: ParemtersWithSecrets` if it goes through
   `Parameters[]`, OR scrub it manually in `SaveSanitizedCfgToTmpFile`. The
   sanitised copy is uploaded to remote object stores after every backup —
   leaking a secret here ships it offsite.

6. **Plumb into the API / Swagger** if user-facing. Update
   `webstatic/docs_api/swagger.yaml` with the field on the relevant schema,
   then `make docs`. Update the `httpd/api_rest_config.go` payload struct if
   it diverges from the YAML struct (some endpoints use trimmed views).

7. **Document** in `documentation_src/docs/configuration.md` and
   `config.yaml` (the example file at repo root).

## Validation
- `go test -mod=vendor -race ./config/... ./shared/...`
- `make testcp`
- Sanity-check by editing a copy of `config.yaml`, starting the daemon, and
  hitting `GET /api/config` — confirm the field round-trips.

## Pitfalls specific to this repo
- `shared.RuntimeConfig.Mutex` protects the *outer* struct; `CfgTemplate.Mutex`
  protects copies. Forgetting to use copies in goroutines is the most common
  bug — see CLAUDE.md for the convention.
- `configor` evaluates the `default:` tag only when the field is the zero
  value. `default:"false"` on a `bool` does nothing useful.
- If the field affects how a backup runs (encryption, paths, exclusions),
  add a copy line to `MakeCopyOfBackupJobDefinition` too — that's the entry
  point each backup goroutine uses.
