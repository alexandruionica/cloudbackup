# 4. Client-side encryption

With `encrypt: true` on a backup definition, file contents are encrypted **on
the backup server, before upload** — the cloud provider only ever sees
ciphertext. This chapter explains how it works and, more importantly, how to
operate it safely.

```yaml
backup:
  - name: documents
    encrypt: true
    encrypt_pass: 'a-long-random-passphrase'
    ...
```

## 4.1 How it works

* Each file is encrypted with streaming **AES-256-GCM**; data is never staged
  unencrypted on disk and files of any size are handled in constant memory.
* The encryption key is derived from `encrypt_pass` with **argon2id**, a
  memory-hard key-derivation function that makes brute-forcing the passphrase
  expensive.
* Every encrypted target keeps a small **keystore sidecar** object in the
  bucket at `<prefix>/.cbcrypt/keystore.v1.yaml`. It stores the KDF salt and
  parameters, a keystore UUID and a password verifier — **not** the key itself.
  The sidecar plus your passphrase are what make decryption possible.
* On the first encrypted backup to a target, the sidecar is created
  automatically (with a conditional PUT, so two servers racing to create it
  cannot corrupt each other).
* Encrypted objects carry a small header (including the keystore UUID), so the
  on-the-wire size is slightly larger than the plaintext.

The `.cbcrypt/` path segment is **reserved**: local files whose path contains a
`.cbcrypt` component are skipped (counted as `skipped_reserved_path` in
reports) to protect the metadata namespace in the bucket.

## 4.2 The rules that keep your data recoverable

1. **The passphrase is unrecoverable by design.** If you lose `encrypt_pass`,
   every encrypted object becomes permanent noise. Store the passphrase in a
   password manager / escrow independent from the backup server.
2. **Don't delete the keystore sidecar.** Without
   `<prefix>/.cbcrypt/keystore.v1.yaml`, existing ciphertext cannot be
   decrypted even with the right passphrase (the KDF salt lives there).
   Bucket lifecycle or cleanup scripts must leave `.cbcrypt/` alone.
3. **Don't change `encrypt_pass` on an existing job.** The password is verified
   against the keystore; a different password will fail verification rather
   than silently writing a mix of keys.
4. **Restores need both**: a reachable sidecar and the same `encrypt_pass` in
   the server config. Restore operations never create or modify the sidecar.

## 4.3 Failure modes you might see

| Symptom (report counter / error) | Meaning | Recovery |
|-----------------------------------|---------|----------|
| `keystore sidecar missing; cannot decrypt without it` (on restore) | Sidecar was deleted from the bucket. | Restore the sidecar object (e.g. from bucket versioning — one more reason to enable it). |
| `keystore_inconsistent` counter / backup refuses to encrypt | Sidecar missing from the bucket, but the local database says encrypted files exist for this job. Bootstrapping a fresh keystore would silently orphan them. | Either restore the sidecar, or accept the loss and run `reset-keystore` (below). |
| `decrypt_keystore_mismatch` counter | An object was encrypted under a different keystore than the current sidecar (e.g. the sidecar was re-created at some point). | Those objects need the original sidecar to decrypt. |
| `skipped_too_large_for_target` counter | The file's encrypted size exceeds the target's maximum object size, so it was skipped (other targets in the same job may still receive it). | Store that file on a target with a larger object-size limit, or exclude it. |

## 4.4 Starting over after sidecar loss (`reset-keystore`)

If the sidecar is gone for good and you accept that previously-encrypted
objects are lost, reset the job's local encryption state:

```console
$ cloudbackup server reset-keystore -c /etc/cloudbackup/config.yaml documents
This will clear the 'encrypted' flag for all files in the local DB for backup job "documents".
After this, the next backup run will treat all previously-encrypted files as needing re-upload.
Any previously-encrypted objects already in the bucket will be unrecoverable unless their keystore sidecar is restored.
Type the job name ("documents") to confirm:
```

This is a **server-side** command (run it on the machine holding `data_dir`,
with the server config). Add `-y` to skip the confirmation prompt in scripts.
Also delete the stale `.cbcrypt/` object from the bucket if one is present, so
the next backup run bootstraps a fresh keystore and re-uploads everything.

## 4.5 What encryption does *not* cover

* **File paths and metadata** are visible to the object store as key names —
  only file *contents* are encrypted.
* Enabling `encrypt: true` on a job that already has unencrypted backups does
  not retroactively encrypt what is already in the bucket; files are encrypted
  as they are (re-)uploaded.

---

Next: [5. CLI client setup](05-cli-setup.md)
