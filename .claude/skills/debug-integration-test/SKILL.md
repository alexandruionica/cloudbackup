---
name: debug-integration-test
description: Triage and debug a single failing Python integration test (under integration_tests/<tier>/) without rerunning the full suite — builds the binary, runs the targeted test with pytest, and surfaces the daemon log. Use when the user reports an inttest failure or pastes a Python traceback from the integration_tests/ folder.
---

# Debug a single integration test

The Python suite under `integration_tests/` boots a real `./cloudbackup`
daemon per test, so reproducing a failure requires the binary to exist and
the test to be runnable in isolation. Modules live in tier directories:
`acceptance/` (CLI journeys), `api/` (REST contract), `cloud/` (real object
stores, skipped without `CLD_*` credentials). Shared helpers are in
`lib/common.py`; pytest fixtures in `conftest.py`.

For writing or extending tests, the tiers, fixtures and conventions are in
the `testing` skill; this one is only about reproducing one failure.

## Inputs to ask for if not given
- The exact test name (file + class + method, or the
  `path::Class::method` form printed by pytest).

## Steps

1. **Build the binary**.
   ```
   make build
   ```
   Most "FileNotFoundError: ./cloudbackup" failures come from a stale build
   after Go changes.

2. **Run only the targeted test**, from the repo root, with the per-OS venv
   (`.venv_linux`, `.venv_macos`, `.venv_freebsd`, `.venv_windows`):
   ```
   PYTHONIOENCODING=utf-8 ./integration_tests/.venv_linux/bin/python -m pytest \
     "integration_tests/<tier>/<module>.py::<Class>::<method>" -v
   ```
   Example:
   `integration_tests/acceptance/cli_advanced.py::TestCliAdvanced::test_cmd_client_backup_status2`.

3. **Find the daemon log** if the test asserted on server behaviour. Tests
   that pass `--logfile` log its path at INFO at test start, e.g.
   `/tmp/integration_test_log_xxxx`; otherwise the daemon's stdout/stderr
   are captured by `BackupDaemon` and printed on failure. `cat` the log to
   see what the server reported. It lives until the OS reaps `/tmp`, so
   read it before re-running.

4. **Re-run with extra logging if needed**. Pass `extra_options="-d"` to the
   `BackupDaemon(...)` call in the test (or the `daemon` fixture in
   `conftest.py`) for the iteration. Revert before committing.

5. **Run the whole module** as a final check before declaring victory:
   ```
   ./integration_tests/.venv_linux/bin/python -m pytest integration_tests/<tier>/<module>.py -v
   ```

## Pitfalls specific to this repo
- If the venv is missing, run `./integration_tests.sh` once — it sets up
  the venv and installs deps (pytest included).
- Tests assume the binary is at `./cloudbackup` in the *current working
  directory* (see `cmd_default = "./cloudbackup"` in `lib/common.py`).
  Running from `integration_tests/` will fail with FileNotFoundError.
- Ports are dynamic: `BackupDaemon` picks a free port and rewrites
  `http.bind_address` in the temp config; `daemon.base_url` is the address
  to use. Never hardcode 8080 in a new test.
- Restore tests need `persistent_null_store=True` (or the `server_config`
  fixture) so the `test_null` store keeps objects across jobs.
- A test that checks JSON output with a strict equality (e.g.
  `assertEqual(decoded, expected_result, ...)`) will break on any new
  field added to the response. Treat new-field failures as "update the
  expected dict", not as a server bug — but verify the field's value is
  reasonable first.

## What to report back
- Whether the failure reproduces.
- The first failing assertion or stack frame from the test output.
- The relevant 10-20 lines of the daemon log around the failure timestamp.
- A proposed fix or, if more investigation is needed, a specific next
  question.
