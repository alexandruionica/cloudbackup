---
name: add-object-store
description: Implement a new cloud object-store backend (e.g. Backblaze, Wasabi, MinIO) by satisfying the ObjectStore interface in objectstore/common.go. Use when the user asks to add support for a new storage provider.
---

# Add a new object store

Each backend is a single file under `objectstore/` named `store_<provider>.go`.
The contract is the `ObjectStore` interface in `objectstore/common.go` —
nothing else outside the package needs to know about the new provider.

## Inputs to ask for if not given
- Provider name (used in config as `type:`, e.g. `backblaze_b2`)
- SDK / client library to use (vendored, please)
- Required credentials and any provider-specific knobs (region, endpoint, bucket-versioning)

## Steps

1. **Add the type constant**. Edit `config/config.go` and append to
   `BackupTargetTypes` (or `HiddenBackupTargetTypes` for test-only backends).
   Without this, config validation rejects the new `type:`.

2. **Implement the store**. Create `objectstore/store_<provider>.go`. Mirror
   the structure of `store_aws_s3.go` — embed an unexported client struct,
   implement every method on the `ObjectStore` interface (`Validate`,
   `GetStoreDetails`, `Upload`, `Download`, `Delete`, `List`, etc.). All
   methods take a `context.Context` first; honour cancellation.

3. **Validate provider parameters**. Add `ValidateBackupTargetParametersForX`
   in `config/config.go` following the S3 / GCP / Azure templates. Wire it
   into the dispatch in `ValidateBackupTargetParameters`.

4. **Plug it into `GetObjectStores`**. Edit `objectstore/common.go` (or
   wherever the type-switch lives) so the new `type:` constructs your store.

5. **Tests**.
   - Unit tests next to `store_<provider>.go` (no network).
   - Live integration tests guarded behind an env-var check matching the
     `CLD_*` pattern in `store_aws_s3_test.go` so they no-op without
     credentials.
   - A Python integration test in `integration_tests/` that uploads,
     lists, and deletes if you have CI credentials available.

6. **Documentation**. Add a section to `documentation_src/docs/configuration.md`
   listing the new target type and its parameters. Run `make docs` to
   regenerate the static docs.

## Validation
- `go build -mod=vendor ./...`
- `go test -mod=vendor -race ./objectstore/...` (skipping live tests)
- `make testcp` — gosec catches credentials being logged; ensure none are.

## Pitfalls specific to this repo
- Backups can stream large files; do not buffer entire objects in memory.
  Match the `MaxBufferSize = 20 MiB` chunking convention from `common.go`.
- The `Validate()` method must do a real round-trip (head + small put/delete)
  so a misconfigured target fails fast at backup start, not midway through.
- `GetStoreDetails()` is used in many log lines; never include the secret
  there — only the store name and type.
