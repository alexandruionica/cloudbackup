---
name: encryption
description: Edit, debug, or extend cloudbackup's client-side encryption (CSE) — the cbcrypto streaming readers, the per-target keystore sidecar lifecycle, the upload/download wiring on each ObjectStore backend, and the report/notification surfaces that show encryption counters. Use when the user asks about CSE, the keystore, KEK / CEK / argon2id, the .cbcrypt sidecar, encrypt_pass, or any of the four encryption counters (skipped_reserved_path, skipped_too_large_for_target, keystore_inconsistent, decrypt_keystore_mismatch).
---

# Client-side encryption (CSE) — editing guide

Design-level docs live in `README.md` under "Client-side encryption". This
skill captures the operational invariants, the per-surface checklist for
counter changes, and the recovery flows — things that are not obvious from
reading any single file.

## Where the code lives

| Concern | File |
|---|---|
| Streaming AES-256-GCM (Encrypting / Decrypting readers, header, KDF, EncryptedSize) | `cbcrypto/crypto.go`, `cbcrypto/reader.go` |
| Sidecar YAML format + verifier | `cbcrypto/keystore/keystore.go` |
| Shared per-target lifecycle helper (fetch / bootstrap / verify) and embedded `encryptionState` | `objectstore/encryption.go` |
| Per-backend `InitEncryption` + `sidecarIO` impl + Upload/Get encryption branches | `objectstore/store_aws_s3.go`, `store_azure_blob.go`, `store_gcp_storage.go`, `store_test_null.go` |
| Pre-upload gating (size cap, reserved-path) | `backup/preupload.go` |
| SkipEncryption setters for internal-state files | `backup/backup.go` — `UploadBackupDatabase`, `UploadBackupConfigCopy` |
| Caller wiring (after Validate, before any I/O) | `scheduler/scheduler.go`, `restore/restore.go`, `httpd/api_rest_backup.go` |
| DB helpers (lifecycle check + reset) | `database/dbops/encryption.go` |
| Operator CLI for recovery | `cliargs/cliargs.go` — `ArgsCommandServerResetKeystore` |

## Invariants — do not break

1. **Chunked AEAD must be processed as one sequential stream.** Never
   reintroduce parallel / ranged I/O for encrypted objects:
   - S3 upload: must use `awsS3Client.PutObject` (single PUT), NOT
     `manager.Uploader`. Hard 5 GiB cap is the trade.
   - S3 download: must use `awsS3Client.GetObject`, NOT
     `manager.NewDownloader` (which uses `WriterAt` + parallel ranges).
   - Body is non-seekable, so the SDK cannot auto-retry mid-stream. Document
     this if you touch the path.

2. **`metadata=true` + non-zero rate limit short-circuits in TestNull** —
   see `store_test_null.go` Upload. Removing this stalls the rate-limited
   integration tests because the DB-copy upload at backup-stop drains
   through the limiter (e.g. 100 B/s × ~50 KiB = ~8 minutes). Production
   uses `SkipEncryption=true` for those uploads, so no encryption
   read-path coverage is lost.

3. **DB copy and config copy MUST keep `SkipEncryption=true`** in
   `UploadBackupDatabase` and `UploadBackupConfigCopy`. The recovery story
   depends on these being plaintext in the bucket — operators restore with
   just `[password + bucket access]`, no extra decrypt step.

4. **`keystore_uuid` is the canonical "this came from this sidecar" link.**
   Every encrypted file header stamps it; every decrypt verifies it. If you
   add a sidecar-rotation path, plan for multiple historical UUIDs in the
   sidecar — don't drop the header field.

5. **GCP CRC32C is disabled when encrypting.** GCS validates the CRC over
   on-the-wire bytes; we cannot pre-compute it from the plaintext file
   once an EncryptingReader sits between. See `store_gcp_storage.go` Upload.

6. **Per-target salt + per-file CEK is the security model.** Don't add
   per-file argon2id derivations — that'd put a ~100 ms KDF on every file
   for no security benefit. The CEK randomness already gives per-file
   isolation.

## Adding a new stat counter — surfaces to touch

When you add a counter via `backupJobsState.IncrementCounter(...)`, it
**won't appear in any report** until you also update:

1. **Init map** in `shared/structs_scheduler.go` —
   `NewBackupJobsRunning` for backup counters, `MarkRestoreRunning` for
   restore counters. Without this the counter is missing from reports
   when it never fires.
2. **CLI `report show`** in `client/common/common.go` —
   `PrintBackupStatus` and/or `PrintRestoreStatus`. Add a Printf line.
3. **Web UI** in `webstatic/ui/src/app.ts` — add to `ReportDetail`
   sections array. Compiled output `webstatic/ui/js/app.js` is tracked,
   so run `cd webstatic/ui && npm run typecheck && npx tsc` and commit
   the regenerated JS too.
4. **TypeScript types** in `webstatic/ui/src/api.ts` — add to the
   `StatsCounters` interface. The catch-all index signature makes this
   technically optional, but explicit typing matches the surrounding code.
5. **Email notifications** in `notifications/notifications.go` — add an
   HTML table row with the orange-highlight-on-nonzero pattern used by
   existing failure counters.
6. **Test expected maps** in `backup/scan/scan_test.go` — 16 hardcoded
   `expectedStats := map[string]uint64{...}` literals use
   `reflect.DeepEqual` so any missing key fails the test. Append the new
   key with a 0 value in each map; `gofmt -w` afterwards to fix indent.

The above checklist is enforced indirectly by `make gotest` (the scan
tests catch missing init entries) but the CLI/UI/notifications surfaces
have no automated coverage — you can ship and they'll silently drop the
counter from reports.

## Adding a new ObjectStore backend with encryption support

In addition to satisfying the `ObjectStore` interface (see the
`add-object-store` skill):

1. **Embed `encryptionState`** in the new store struct.
2. In the constructor, populate `encryptionState{enabled: backupConfig.Encrypt, password: []byte(backupConfig.EncryptPass)}`.
3. Implement `InitEncryption(opts EncryptionInitOptions) error` that
   delegates to `objStore.initEncryption(&yourSidecarIO{store: objStore}, opts, bump)`.
4. Implement a `sidecarIO` with `Fetch()` and `PutIfNotExists()` methods
   mapping to your SDK. Translate "not found" to `errSidecarNotFound` and
   conditional-create conflicts to `errSidecarConflict`.
5. In `Upload`, wrap with `cbcrypto.NewEncryptingReader(reader, kek, uuid)`
   when `EncryptionEnabled() && !DbRecord.SkipEncryption`. If your SDK
   pre-reads the file for a checksum / hash, disable that on encrypted
   uploads (see GCP CRC32C precedent).
6. In `Get`, when `existingDbRecord.Encrypted`, wrap with
   `cbcrypto.NewDecryptingReader(body, kek)`, call `PeekHeader()`, compare
   `hdr.KeystoreUUID` against `KeystoreUUID()`, bump
   `decrypt_keystore_mismatch` on mismatch.
7. Implement `MaxObjectSize(encrypted bool)` returning the SDK's hard cap
   for whichever upload mode you use when encrypted vs not.

## Debugging recipes

**`keystore_inconsistent` at backup-job start.** Sidecar missing on the
bucket but local DB has `encrypted=1` rows for the job. Either:
- Restore the sidecar from a bucket-side backup (S3 version restore,
  GCS soft-delete), or
- Accept data loss and run
  `cloudbackup server reset-keystore -c <cfg> <jobname>` (clears DB flags,
  next run bootstraps a fresh sidecar — orphaned encrypted objects in the
  bucket stay until your lifecycle policy retires them).

**`decrypt_keystore_mismatch` at restore.** File header's `keystore_uuid`
doesn't match the current sidecar — the sidecar was replaced (possibly
with the same password). Recover by restoring the previous sidecar; the
file's wrapped CEK can only be unwrapped with the KEK derived from the
original salt.

**"Backup did not complete stopping after 6 seconds" in integration test
with `ratelimit=100`.** Suspect the TestNull `metadata=true` short-circuit
(invariant 2). Confirm with `git log -p store_test_null.go` and search
for "rateLimit > 0".

## Testing without cloud creds

TestNull has a working in-memory `sidecarIO` and an upload-byte capture.
Use it for full encrypt → decrypt roundtrip tests; see
`objectstore/store_test_null_encryption_test.go` and
`encryption_e2e_test.go`. Pass `"0"` as the ratelimit so the metadata
short-circuit doesn't fire.

## Useful greps

- All four counter names: `git grep -nE 'skipped_reserved_path|skipped_too_large_for_target|keystore_inconsistent|decrypt_keystore_mismatch'`
- All `EncryptingReader` / `DecryptingReader` wrap sites:
  `git grep -nE 'NewEncryptingReader|NewDecryptingReader'`
- Anywhere SkipEncryption is set: `git grep -n 'SkipEncryption = '`
- The sidecar's well-known path: `git grep -n SidecarRelativePath`
