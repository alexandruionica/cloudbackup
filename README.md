# README #
This project was an attempt to build an opensource backup solution which provides file level granularity of backups and is able to store resulting backups in a cloud provider's blob store, like AWS S3, Azure Blob Storage or Google's GCP Cloud Storage.
The project was a continuation of a tool(https://bitbucket.org/alexandru_ionica/s3backuptool/) a built many years ago(in 2016), in Python. This new attempt was in GO and was setup from the get go to use modern coding practices.

It had a server which would take care of backups, restores and reporting and a command line client capable to connect to the server and request on demand backups, restores and reporting.
The client could also connect and see in realtime the progress of a backup. Additionally an HTTP API (used by the client) was documented using SWAGGER.
Supported platforms were Linux, FreeBSD and MS Windows. While MacOS was not integrated into the CI/CD pipeline, it would most likely has minor issues to fix before a build would be possible.

The project never completed the restore capability while the on-demand backup one was implemented, for the three mentioned object stores.
If you wish to continue then to get started you need to make a build

The CI/CD setup is [in a separate repository](https://bitbucket.org/alexandru_ionica/cloudbackup_infrastructure/src/master/). Jenkins for the actual CI/CD and some tooling for building the AWS AMIs for Linux/FreeBSD/Windows to be used by Jenkins. 

To do initial setup:

*  install Golang 1.22 and golangci-lint v1.64.4 https://github.com/golangci/golangci-lint/releases/tag/v1.63.4
*  set $GOPATH
*  install Make, Python3 >=3.12.3 , Python3 Virtualenv, Python3 pip
*  Clone repo

```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
```

To do a build, run:
```
cd $GOPATH/srv/cloudbackup
make
```

## LLM usage ##

Code submitted before April 2026 was produced in "classical ways" and represents the vast majority of the codebase. 

Submissions starting with April 2026 represent code produced with agentic LLMs and have allowed to:

* add functionality in areas where my expertise is very limited like Javascript/Typescript web UIs.

* work to progress in areas where I didn't have any more significant time to invest 

## Quick demo ##

Watch https://www.youtube.com/watch?v=wyoO3pm_fmY for a quick demo of the project in action, showing CLI usage and access to documentation.

For a view of the web UI see https://youtu.be/EFjg5-VDSu8 .

# Required for running tests

Various credentials are needed for the tests which use object stores, like AWS S3.


```
# AWS S3 credentials for integration tests
export CLD_AWS_ACCESS_KEY_ID="REPLACE_WITH_KEY_ID"
export CLD_AWS_SECRET_ACCESS_KEY="REPLACE_WITH_SECRET"
export CLD_S3_BUCKET="aionica-tests"
export CLD_S3_REGION="us-east-1"

# GCP Storage credentials for integration tests
export CLD_GCP_TYPE="service_account"
export CLD_GCP_PROJECT_ID="FILL_IN"
export CLD_GCP_PRIVATE_KEY_ID="FILL_IN"
export CLD_GCP_PRIVATE_KEY='FILL_IN'
export CLD_GCP_CLIENT_EMAIL="FILL_IN"
export CLD_GCP_CLIENT_ID="FILL_IN"
export CLD_GCP_AUTH_URI="FILL_IN"
export CLD_GCP_TOKEN_URI="FILL_IN"
export CLD_GCP_AUTH_PROVIDER_X509_CERT_URL="FILL_IN"
export CLD_GCP_CLIENT_X509_CERT_URL="FILL_IN"
export CLD_GCP_STORAGE_BUCKET="aionica-tests"

# Azure Blobs credentials for integration tests
export CLD_AZURE_STORAGE_ACCOUNT='FILL_IN'
export CLD_AZURE_STORAGE_ACCESS_KEY='FILL_IN'
export CLD_AZURE_STORAGE_CONTAINER='FILL_IN'
```

It's generally recommended to add them to a `.creds` file at the root of the repo as this filename is blackliested 
(via `.gitignore`) and then just execute once `source .creds` after starting a new terminal session (in which the tests
 will eventually be ran). You will most likely also have to copy said values into your IDE so it can run tests and give
  you the coverage report (integrated with your IDE, assuming it has such functionality).

# Re-Generating Documentation

Run on Linux/Unixes only:

```
make docs
```
The documentation is in the `documentation_src` folder but once the server is launched  (`./cloudbackup server start -c config.yaml`) the the documentation can be accessed at http://127.0.0.1:8080/docs or the network reachable IP + port of the server (if one was configured)

# Client-side encryption

cloudbackup can encrypt file contents on the client before uploading to any of the supported cloud backends (AWS S3, Azure Blob, GCP Cloud Storage). When enabled, the bytes that hit the bucket are AES-256-GCM ciphertext keyed by a password the operator controls. The cloud provider never sees plaintext.

## Enabling it

In the server config, set `encrypt: true` and `encrypt_pass: <your-password>` on a backup job:

```yaml
backup:
  - name: documents
    encrypt: true
    encrypt_pass: "use-a-long-randomly-generated-passphrase"
    source:
      - "/home/me/Documents"
    target:
      - name: s3-prod
        type: aws_s3
        bucket: my-backups
        prefix: documents/
```

Validation refuses a config with `encrypt: true` but an empty `encrypt_pass`.

## How it works

- A 32-byte key-encryption-key (KEK) is derived from `encrypt_pass` with **argon2id** (memory=64 MiB, time=3, threads=4) using a random per-target salt.
- Each file gets a fresh random 32-byte content-encryption-key (CEK), wrapped with the KEK using AES-256-GCM and embedded in the file's encrypted header.
- The file body is chunked into 64 KiB plaintext blocks, each AES-256-GCM encrypted with a unique nonce; truncation is detected via a last-chunk flag in the nonce.
- The KEK lives in daemon memory across backup runs; argon2id is invoked once per target lifetime.

## The keystore sidecar

Each encryption-enabled target stores a small YAML object at:

```
<storePrefix>/.cbcrypt/keystore.v1.yaml
```

It holds the salt, argon2id parameters, a unique keystore UUID (stamped into every encrypted file's header), and a verifier that lets the daemon detect a wrong password at startup instead of mid-backup.

- All sidecar fields are non-sensitive (the salt is random, the verifier is a known plaintext sealed with the KEK). It can be cleartext.
- On first encrypted backup, the daemon creates the sidecar using a conditional PUT (`If-None-Match: *` on S3/Azure, `IfGenerationMatch=0` on GCS). If two daemons race, the loser adopts the winner's sidecar.
- The path segment `.cbcrypt/` is reserved: any local file whose remote-mapped path would land there is skipped and the `skipped_reserved_path` counter is bumped.

## Per-backend trade-offs

- **AWS S3 — single PUT only when encrypted.** Chunked AEAD must be processed as one sequential stream, so the encryption path uses `PutObject` instead of the multipart `Uploader`. The hard limit is therefore **5 GiB per encrypted file**. Files larger than that are skipped per target with the `skipped_too_large_for_target` counter; other targets in the same job can still succeed (e.g., the same file ships fine to Azure or GCP). Because the body is not seekable, the SDK cannot retry mid-upload on transient errors — a failed upload returns an error and the file will be retried on the next backup run.
- **GCP CRC32C — disabled when encrypting.** GCS computes CRC32C over the on-the-wire bytes; we cannot pre-compute it from the plaintext file once the body is encrypted, so the checksum is disabled when encryption is on for that target. GCS still preserves data integrity at the storage layer.
- **Azure — no change.** UploadStream reads the body sequentially into per-block buffers before dispatching parallel PUTs, so it's safe with the non-seekable `EncryptingReader`.

## What stays plaintext

The SQLite DB copy and the sanitised config copy that cloudbackup uploads at the end of every backup run are kept **plaintext** in the bucket, even when encryption is on. Rationale: their contents don't add meaningful disclosure beyond the directory tree already visible from the bucket layout, and operators can read them directly during disaster recovery without an extra decryption step. To restore from a fresh machine you only need `[password + bucket access]`.

## New counters in the backup report

| Counter | When |
|---|---|
| `skipped_reserved_path` | File's remote path collides with `<storePrefix>/.cbcrypt/` |
| `skipped_too_large_for_target` | Predicted ciphertext size exceeds the target's `MaxObjectSize(encrypted=true)` |
| `keystore_inconsistent` | Sidecar missing on the bucket but the local DB has rows marked encrypted — refuses to silently re-bootstrap (would orphan that data) |
| `decrypt_keystore_mismatch` | File header's `keystore_uuid` doesn't match the sidecar's, surfaced per file at restore time |

## Recovery from a lost sidecar

If the sidecar object is somehow deleted from the bucket while the local DB still references encrypted files, the next backup will fail target init with `keystore_inconsistent`. Recovery options:

1. **Restore the sidecar** from a bucket-side backup if you have one (e.g., S3 object-version restore, GCS soft-delete).
2. **Accept data loss** — the previously-encrypted objects in the bucket cannot be decrypted without the original salt. Clear the local DB's encrypted flags and start fresh:

```bash
cloudbackup server reset-keystore -c /etc/cloudbackup/server.yaml <backup-job-name>
```

This clears `encrypted=1` on every row in that job's local DB. On the next backup, the daemon sees an empty encrypted-files set in the DB, allows bootstrap, generates a brand new sidecar, and re-uploads all files. Previously-encrypted objects already in the bucket are left in place (they will eventually be cleaned up via your bucket lifecycle policy) but cannot be recovered.

## Migration: plaintext → encrypted

Just set `encrypt: true` and `encrypt_pass: ...` on an existing backup job. The next backup run sees the flag mismatch between DB (`encrypted=0` per file) and config (`encrypt=true`) and re-uploads every file under the new keystore. Old plaintext versions remain in version history until the bucket lifecycle policy retires them.

## Known limitations

- **No KEK rotation in v1.** The header carries a `keystore_uuid` field that is forward-compatible with a future scheme that tracks multiple historical UUIDs, but a current "rotate the password" workflow does not exist.
- **File path tree is visible.** The on-bucket layout mirrors local paths under `<storePrefix>/data/...`, so an observer of the bucket sees what paths were backed up even with encryption on. Encrypting filenames is out of scope for this iteration.
- **Cloud-cred integration tests for encryption are not yet in `integration_tests/`.** End-to-end coverage of the encryption code paths is provided at the Go unit-test level using the TestNull backend (`objectstore/encryption_e2e_test.go`, `objectstore/store_test_null_encryption_test.go`). Adding Python integration tests under `integration_tests/encrypted_<backend>.py` to validate real-cloud behaviour (conditional PUT race, 5 GiB cap actually firing on S3, etc.) is a follow-up.
