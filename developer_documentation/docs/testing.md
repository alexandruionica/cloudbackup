# Testing

Development on this project is prompt-driven; the test suite is the regression net that catches what a
language model gets wrong. There are four layers.

| Layer | Where | Runs with | Needs |
|---|---|---|---|
| Go unit tests | `*_test.go` next to the code | `make gotest`, `make gotestrace` | Go |
| Web UI unit tests | `webstatic/ui/tests/` | `make uitest` | Node >= 18 |
| Python integration tiers | `integration_tests/<tier>/` | `make inttest`, `make inttest-cloud` | Python 3.12+, a built binary |
| Package install smoke | `packaging/` | `make packages` | Docker |

`make test` runs lint plus the Go and web UI unit tests. `make alltest` adds every integration tier.

## Integration tiers

Every Python test boots a real `./cloudbackup` daemon from a temporary config and drives it through the
CLI binary or the REST API. The modules are grouped by what they prove:

- **`acceptance/`** — user acceptance. Drives the shipped binary the way the user guide tells users to:
  `cloudbackup client ...` and `cloudbackup server ...` commands, exit codes, `--json` output, overrides
  between config file, environment and flags, notifications delivered to a real SMTP listener.
- **`api/`** — REST contract. Validation errors, role checks, pagination tokens, server-sent event
  streams, the schemathesis run against the Swagger spec, and the encryption lifecycle.
- **`cloud/`** — real object stores. Backup and restore against AWS S3, Azure Blob Storage and GCP
  Storage. Each module is decorated with `requires_env(...)` and is **skipped**, not failed, when its
  `CLD_*` credentials are absent (see the README for the variable list). After this tier the runner
  calls `cloud/clean_object_stores_after_tests.py`, which likewise skips providers without credentials.
- **`lib/`** — `common.py`, the shared helpers. `conftest.py` puts it on `sys.path`, so modules keep
  their `from common import *`.

```bash
make inttest                       # acceptance + api (no credentials needed, ~90 s)
make inttest-cloud                 # cloud tier, then object store cleanup
./integration_tests.sh all         # every tier
./integration_tests.sh acceptance  # one tier
```

Windows uses `integration_tests.ps1` with the same arguments.

### Running one module or one test

The runner is pytest. The binary must exist at `./cloudbackup` in the repository root, so run from there:

```bash
make build
integration_tests/.venv_linux/bin/python -m pytest integration_tests/acceptance/cli_basics.py -v
integration_tests/.venv_linux/bin/python -m pytest \
    "integration_tests/api/rest_api_restore.py::TestRestAPIRestore::test_restore_full_lifecycle" -v
```

The virtualenv is per OS (`.venv_linux`, `.venv_macos`, `.venv_freebsd`, `.venv_windows`); running the
runner script once creates it.

### Ports

Nothing is hardcoded. `BackupDaemon(config_path)` picks a free port, rewrites `http.bind_address` in the
temporary config, and exposes it as `daemon.base_url`; the SMTP listener does the same through
`find_free_port()`. A developer's own daemon on 8080 cannot break the suite, and two test processes can run
side by side.

### The persistent `test_null` store

Most modules use the hidden `test_null` target type so no cloud account is needed. The daemon builds a new
object store per job, so by default a restore job cannot read what a backup job uploaded. Passing
`persistent_null_store=True` to `setup_tmp_config_file_and_tmp_dirs()` adds a `persist_dir` parameter to
every `test_null` target, which keeps objects (and the encryption keystore sidecar) on disk. Use it for any
test that restores, runs the same job twice, or checks encryption end to end. See the architecture chapter
for the implementation.

### Writing new tests

New modules should use the pytest fixtures from `integration_tests/conftest.py` instead of copying a
`setUp()`:

- `source_tree` — a temp directory populated with files and directories (Unicode names included).
- `server_config` — a temp server config whose `first_backup` job backs up `source_tree` through a
  persistent `test_null` target. `server_config.patch(fn)` edits it before the daemon starts.
- `daemon` — a running daemon on a free port with `--logfile` captured; `daemon.base_url`.
- `client_config` — a client config file for `testuser1` pointing at `daemon`.
- `api` / `api_readonly` — an `ApiClient` (write / read-only user) with `run_backup_and_wait()`,
  `start_restore()`, `wait_restore_finishes()`, `backup_report()`, `set_target_ratelimit()` and friends.
- `restore_dir` — an empty temp directory.

Assertion helpers in `common.py`: `check_backup_report()`, `check_restore_report()`,
`verify_restored_tree()` (md5 of every restored file plus counts), `decode_json_stream()` for CLI
`--json` output that mixes pretty-printed and single-line documents.

Prefer `--json` output over counting lines of human-readable text, and make every scenario docstring
name the user guide section it mirrors.

### Fixture trees

`setup_dir_with_tmp_files()` builds the default tree (Unicode and shell-metacharacter names). Flags add
the shapes that trip backup tools: `symlinks` (to a file, to a directory, dangling), `empty_file`,
`large_file` (6 MiB, above the S3 multipart part size and the Azure block size), `hard_link`,
`long_name`. `acceptance/fixture_realism.py` runs the full set through backup and restore with
`dereference` on and off; the cloud restore tests carry the empty and large files onto real buckets.
`count_files_folders_links(path, dereference)` mirrors what the server will examine, symlinks included.

### Previous-release data

`integration_tests/fixtures/release_<tag>/` holds a data directory (SQLite databases), the config and a
`meta.json` written by that release's binary; `api/compat_previous_release.py` starts the current binary
on it, reads the old run's report and file list, runs an incremental backup on top and restores. The
databases store absolute paths, so the tree is recreated at the recorded location and the test skips on
other OS families. Regenerate (or add a release) with the old binary built from its tag:

```bash
git worktree add /tmp/cb-v0.0.3 v0.0.3
(cd /tmp/cb-v0.0.3 && bash generate_version.sh && go build -mod=vendor -o cloudbackup .)
integration_tests/.venv_linux/bin/python integration_tests/fixtures/make_release_fixture.py \
    --binary /tmp/cb-v0.0.3/cloudbackup --version v0.0.3
```

The database package has no schema versioning yet; the day the schema changes, this test is what turns
a silent break into a failing build and forces a migration path.

### Skips

Use `self.skipTest(...)` or `@unittest.skipUnless(...)` for platform limitations, never a bare `return`:
a silent return counts as a pass and hides a hole in coverage. Skips are listed in the pytest summary.
