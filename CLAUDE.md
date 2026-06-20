# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build        # Build the binary (go build -v -mod=vendor)
make test         # Run unit tests + race detection
make gotest       # Run unit tests only
make gotestrace   # Run unit tests with race detection
make testcp       # Run go fmt + golangci-lint (with gosec, without ineffassign)
make alltest      # Run all tests + integration tests + build
make inttest      # Run Python-based integration tests
make cover        # Show HTML coverage report
make deps         # go mod tidy + go mod vendor
make run          # Build and run the binary
make docs         # Regenerate Swagger API docs
```

To run a single Go test:
```bash
go test -v -run TestName ./path/to/package/...
```

Integration tests require cloud credentials as environment variables (AWS, GCP, Azure) — see README.md for the full list.

**Prerequisites:** Go 1.22+, golangci-lint v1.64.4, Python 3.12.3+, virtualenv, pip.

## Architecture

CloudBackup is a server/client backup tool with cloud blob store support (AWS S3, Azure Blob Storage, GCP Storage). It runs as a daemon exposing a REST API, with a CLI client for on-demand operations.

### Request Flow

```
CLI (cliargs) → Client packages (client/*) → HTTP → Daemon (daemon) → Config + SQLite DB
                                                          ↓
                                          HTTP Server (httpd) ←→ Scheduler (scheduler)
                                                          ↓
                                          Backup / Restore core (backup, restore)
                                                          ↓
                                          File scanning (backup/scan)
                                                          ↓
                                          Object store (objectstore)
                                          [AWS S3 / Azure / GCP / test_null]
                                                          ↓
                                          DB tracking + Notifications
```

### Key Packages

- **cliargs** — CLI argument parsing and routing; dispatches server start, client commands, and misc operations.
- **daemon** — Server lifecycle: initializes config, DB, HTTP server, and scheduler; wires them together via channels.
- **httpd** — REST API handlers (`api_rest_backup.go`, `api_rest_config.go`, `api_rest_report.go`). Uses `julienschmidt/httprouter`. Authenticated via HTTP Basic Auth with role-based permissions.
- **scheduler** — Listens on a channel for backup commands from HTTP handlers; manages concurrent backup/restore goroutines.
- **backup** — Core backup orchestration: diff calculation, upload, restore. Delegates scanning to `backup/scan/` and metadata to `backup/fileproperties/`. Per-(file,target) gating helpers in `preupload.go` (size limit, reserved-namespace check).
- **objectstore** — Cloud storage abstraction. Each provider (`store_aws_s3.go`, `store_azure_blob.go`, `store_gcp_storage.go`) implements the common interface defined in `common.go`. Per-target client-side-encryption lifecycle lives in `encryption.go` (`InitEncryption`, sidecar fetch/bootstrap with conditional PUT).
- **cbcrypto** — Client-side encryption: streaming AES-256-GCM (`EncryptingReader`, `DecryptingReader`), file header (incl. `keystore_uuid`), argon2id KEK derivation, `EncryptedSize` helper.
- **cbcrypto/keystore** — YAML sidecar (`<storePrefix>/.cbcrypt/keystore.v1.yaml`) holding salt, KDF params, keystore UUID, and password verifier.
- **database** — SQLite-backed metadata store using WAL mode. Tracks backup jobs, remote files, and failures. DB operations are in `dbops/`.
- **config** — Thread-safe YAML config with `GetCopyWithLock()` pattern for safe concurrent reads.
- **shared** — Shared structs used across packages: config types (`ConfigBackup`, `ConfigBackupTarget`, `ConfigUser`), job state (`BackupJobsState`), and file properties.
- **client** — Client-side logic for CLI commands (backup, config, notifications).
- **watcher** — Real-time file progress multiplexer for streaming backup status.
- **notifications** — Post-job notifications via email or custom scripts.

### Concurrency Patterns

- `sync.RWMutex` + `GetCopyWithLock()` for shared config/state — always use copies, never hold the lock across I/O.
- Channel-based communication between daemon, HTTP handlers, and scheduler.
- Race detection is part of standard testing (`make gotestrace`).

### Database

SQLite with WAL mode (3× faster than default), foreign keys enabled, 5s busy timeout. Schema lives in `database/`.

### Logging

Structured logging via `logrus`. Each package creates a context logger with `log.WithFields()`. JSON format by default; plaintext optional via config.
