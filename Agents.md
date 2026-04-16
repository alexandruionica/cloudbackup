# Agents.md

Instructions and context for AI coding agents working on this repository.

## Project Overview

CloudBackup is a server/client backup tool written in Go. It backs up files to cloud blob stores (AWS S3, Azure Blob Storage, GCP Storage). The server runs as a daemon exposing a REST API; the CLI client issues commands against it.

## Build and Test Commands

```bash
make build        # Build the binary (go build -v -mod=vendor)
make test         # Run go fmt + golangci-lint + unit tests + race detection + UI tests
make gotest       # Run unit tests only
make gotestrace   # Run unit tests with race detection
make testcp       # Run go fmt + golangci-lint (with gosec, without ineffassign)
make alltest      # Run all tests + integration tests + build
make inttest      # Run Python-based integration tests
make cover        # Show HTML coverage report
make deps         # go mod tidy + go mod vendor
```

To run a single Go test:
```bash
go test -v -run TestName ./path/to/package/...
```

After making changes, always run `make testcp` (formatting + lint) and `make gotest` (unit tests) before considering the work done.

## Architecture

### Request Flow

```
CLI (cliargs) --> Client packages (client/*) --> HTTP --> Daemon (daemon)
                                                            |
                                                   HTTP Server (httpd)
                                                      |          |
                                                Scheduler    Report handlers
                                                   |
                                            Backup / Restore
                                                   |
                                         Object Store (objectstore)
                                        [AWS S3 / Azure / GCP / test_null]
                                                   |
                                         SQLite DB (database/dbops)
                                                   |
                                           Notifications
```

### Key Packages

- **cliargs** -- CLI argument parsing using `jessevdk/go-flags`. Defines command structs with `Execute()` methods that load client config and call into `client/*` packages. Nested command hierarchy: `cloudbackup server|client`, then `client backup|restore|config|notification`, then sub-commands like `start|stop|list|watch|report`.
- **daemon** -- Server lifecycle: initializes config, DB, HTTP server, and scheduler; wires them together via channels.
- **httpd** -- REST API handlers using `julienschmidt/httprouter`. Authenticated via HTTP Basic Auth with role-based permissions (full access vs read-only). Key files:
  - `api_rest_backup.go` -- backup start/stop/list/watch/dryrun handlers
  - `api_rest_restore.go` -- restore start/stop/list/watch handlers
  - `api_rest_report.go` -- report handlers for backup and restore history, file listing
  - `api_rest_config.go` -- configuration read/write handlers
  - `init.go` -- route registration and read-access control list
- **scheduler** -- Channel-based message passing between HTTP handlers and backup/restore goroutines. Manages concurrent job state via `BackupJobsState`.
- **backup** -- Core backup orchestration: diff calculation, upload. Delegates scanning to `backup/scan/` and metadata to `backup/fileproperties/`.
- **restore** -- Core restore orchestration: downloads files from object store and writes to local filesystem.
- **objectstore** -- Cloud storage abstraction. Each provider implements a common interface (`common.go`). Providers: `store_aws_s3.go`, `store_azure_blob.go`, `store_gcp_storage.go`, `store_test_null.go` (in-memory, used by integration tests that don't need real cloud credentials).
- **database** -- SQLite-backed metadata store using WAL mode. Schema creation in `database.go`. Prepared statements and CRUD operations in `dbops/dbops.go`.
- **shared** -- Shared structs used across packages. Key files:
  - `structs_scheduler.go` -- `BackupJobsState`, `BackupJobStatus`, `CommWithSchedulerForBackup`, `CommWithSchedulerForRestore`, concurrency management methods
  - `structs_dbops.go` -- `DbData`, `DbPreparedStatements`
  - `structs_config.go` -- config types
- **config** -- Thread-safe YAML config with `GetCopyWithLock()` pattern for safe concurrent reads.
- **client/** -- Client-side packages that make HTTP calls to the server and render output:
  - `client/backup/` -- backup start/stop/list/watch
  - `client/backup/report/` -- backup report list/show (paginated table output)
  - `client/backup/target/` -- target test
  - `client/restore/` -- restore start/stop/list/watch, TUI file browser (`browse.go`), restore report list/show (`report.go`)
  - `client/config/` -- client configuration loading from YAML + env vars + CLI flags
  - `client/common/` -- shared utilities like `ValidateServerResponse()`, `PrintBackupStatus()`
  - `client/notification/` -- notification test
- **watcher** -- Real-time file progress multiplexer for streaming backup/restore status via SSE.
- **notifications** -- Post-job notifications via email or custom scripts.

### Database Schema

SQLite with WAL mode, foreign keys enabled, 5s busy timeout. Core tables:
- `jobs` -- tracks all backup and restore job runs. Has `type` column (`'backup'` or `'restore'`), `state` column (`started`, `finished`, `failed`, `cancelled`, `crashed`), and `report` column (JSON-serialized `BackupJobStatus`).
- `files` -- local file metadata (path, size, mtime, permissions).
- `remote_files` -- records of files uploaded to object stores.
- `backup_collections` -- links job runs to their uploaded files.
- `top_items` -- top-level paths being processed.

Each backup definition gets its own SQLite database file (named after the backup definition). Restore jobs write to the same database as the backup they restore from.

### Concurrency Patterns

- `sync.RWMutex` + `GetCopyWithLock()` for shared config/state -- always use copies, never hold the lock across I/O.
- Channel-based communication between daemon, HTTP handlers, and scheduler (`CommWithSchedulerForBackup`, `CommWithSchedulerForRestore`).
- `context.Context` for cancellation signalling to running backup/restore jobs.
- Race detection is part of standard testing (`make gotestrace`).

### Authentication and Access Control

- HTTP Basic Auth on all API endpoints.
- Users defined in server config with `access` field: `"full"` or `"read-only"`.
- Read-only access list defined in `httpd/init.go` (`ReadAccess` map) -- specifies which method+path combinations read-only users may access.

### Logging

Structured logging via `logrus`. Each package creates a context logger with `log.WithFields(log.Fields{"context": "package.name"})`. JSON format by default; plaintext optional via config.

## Requirements for Code Changes

### Quality and Testing

- All new code must be accompanied by **unit tests** (Go `*_test.go` files in the same package).
- New or changed API endpoints must also have **integration tests** in the `integration_tests/` directory (Python, using `unittest.TestCase`).
- Run `make testcp` to verify formatting and lint compliance before finishing. Zero lint issues is required.
- Run `make gotest` to verify all unit tests pass.
- Use `make gotestrace` periodically to check for race conditions.

### Unit Test Patterns

- **httpd handlers**: Create a minimal `SrvData{Mutex: &sync.RWMutex{}}`, use `httptest.NewRequest()` and `httptest.NewRecorder()` to drive handlers directly. Test input validation (missing fields, invalid JSON, bad dates, etc.). See `httpd/api_rest_restore_test.go` for examples.
- **client packages**: Use `httptest.NewServer()` with a recorder handler to mock the server. Test the pure `doXxx()` functions that return values/errors (not the `Xxx()` wrappers that call `os.Exit()`). See `client/restore/restore_test.go` and `client/restore/report_test.go`.
- **cliargs Execute methods**: Use the subprocess pattern with `TEST_RUNNING=1` environment variable to test exit codes. See `cliargs/cligargs_test.go`.

### Integration Test Patterns

- Located in `integration_tests/`, written in Python using `unittest.TestCase`.
- Each test class: `setUp()` creates temp config, starts a `BackupDaemon`, `tearDown()` kills it and cleans up.
- Use `common.py` utilities: `setup_tmp_config_file_and_tmp_dirs()`, `setup_dir_with_tmp_files()`, `BackupDaemon`, `check_api_server_ready()`.
- Use `test_null` object store backend (no cloud credentials needed) for tests that don't exercise real cloud storage.
- Helper method `ValidatedAndDecodeResponse()` checks Content-Type, decodes JSON, verifies `code` and `message` keys.
- Use `requests` library with `auth=(username, password)` for Basic Auth.
- See `integration_tests/rest_api_restore.py` and `integration_tests/rest_api_report_restore.py` for examples.

### API Documentation

Any additions or changes to REST API endpoints **must** be reflected in both:
- `webstatic/docs_api/swagger.yaml`
- `webstatic/docs_api/swagger.json`

Add path entries, request/response definitions, and ensure the JSON file is valid (`python3 -m json.tool swagger.json > /dev/null`).

### Cross-Platform Compatibility

Code must work on **Linux** and **Microsoft Windows** (compilable and runnable). Ideally it also works on **macOS**.

Key considerations:
- Use `path/filepath` (not `path`) for filesystem operations -- it handles OS-specific separators.
- Use `filepath.Join()`, `filepath.Clean()`, `filepath.Dir()` instead of string concatenation with `/`.
- The Makefile has `ifeq ($(OS),Windows_NT)` blocks for Windows-specific commands.
- Integration tests in `common.py` handle Windows path differences (drive letters, backslashes).
- TUI components use `charmbracelet/bubbletea` which supports Windows via ConPTY.
- Avoid Unix-specific syscalls; use Go's standard library abstractions where possible.
- Test on both platforms when making filesystem or path-related changes.

### Dependency Management

- Dependencies are vendored (`vendor/` directory). Use `make deps` (`go mod tidy && go mod vendor`) after adding or updating dependencies.
- Build with `-mod=vendor` flag (already configured in Makefile).

### Code Style

- Go standard formatting enforced by `go fmt` (run as part of `make testcp`).
- `golangci-lint` with `gosec` enabled, `ineffassign` disabled.
- Each package defines its own `logger` and `loggingContext` for structured logging.
- HTTP handlers follow the pattern: increment stats counter, validate input, get DB access, query, respond with `JSONSuccess*` or `JSONError`.
- Client-side code separates pure testable functions (`doXxx()` returning values/errors) from CLI wrappers (`Xxx()` that print and call `os.Exit()`).

### Adding New CLI Commands

1. Define the command struct in `cliargs/cliargs.go` with `go-flags` tags.
2. Add it as a field on the parent command struct (e.g., `ArgsCommandClientRestore`).
3. Implement the `Execute(args []string) error` method following the existing pattern: setup logging, load client config, call into the appropriate `client/*` function.
4. Add unit tests using the subprocess pattern in `cliargs/`.

### Adding New API Endpoints

1. Define request/response types in the appropriate `httpd/api_rest_*.go` file.
2. Implement the handler function on `SrvData`.
3. Register the route in `httpd/init.go`.
4. If read-only users should have access, add the path to the `ReadAccess` map in `httpd/init.go`.
5. Add SQL prepared statements if needed in `database/dbops/dbops.go` and corresponding fields in `shared/structs_dbops.go`.
6. Update `webstatic/docs_api/swagger.yaml` and `webstatic/docs_api/swagger.json`.
7. Add unit tests in `httpd/` and integration tests in `integration_tests/`.
