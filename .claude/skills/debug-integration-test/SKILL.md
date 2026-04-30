---
name: debug-integration-test
description: Triage and debug a single failing Python integration test (under integration_tests/) without rerunning the full 145-second suite — builds the binary, runs the targeted test, and surfaces the daemon log. Use when the user reports an inttest failure or pastes a Python traceback from the integration_tests/ folder.
---

# Debug a single integration test

The Python suite under `integration_tests/` boots a real `./cloudbackup`
daemon per test, so reproducing a failure requires the binary to exist and
the test to be runnable in isolation.

## Inputs to ask for if not given
- The exact test name (file + class + method, or the `module.Class.method`
  form printed by the failure traceback).

## Steps

1. **Build the binary**.
   ```
   make build
   ```
   Most "FileNotFoundError: ./cloudbackup" failures come from a stale build
   after Go changes.

2. **Run only the targeted test**, from the repo root, with `PYTHONPATH`
   pointed at `integration_tests/` so the test module is importable:
   ```
   PYTHONIOENCODING=utf-8 PYTHONPATH=integration_tests \
     ./integration_tests/.venv/bin/python -m unittest \
     <module>.<Class>.<method> -v
   ```
   Example: `cli_advanced.TestCliAdvanced.test_cmd_client_backup_status2`.

3. **Find the daemon log** if the test asserted on server behaviour. The
   harness writes daemon stdout/stderr to a tempfile whose path is logged
   at INFO at test start, e.g.
   `/tmp/integration_test_log_xxxx`. `cat` it to see what the server
   reported during the run. The log lives until the OS reaps `/tmp`, so
   read it before re-running.

4. **Re-run with extra logging if needed**. Add `--debug` to the daemon
   invocation by editing `BackupDaemon` in `integration_tests/common.py`
   for the iteration. Revert before committing.

5. **Run the whole module** as a final check before declaring victory:
   ```
   PYTHONIOENCODING=utf-8 PYTHONPATH=integration_tests \
     ./integration_tests/.venv/bin/python -m unittest <module> -v
   ```

## Pitfalls specific to this repo
- The Python venv lives at `integration_tests/.venv/`. If it's missing, run
  `./integration_tests.sh` once — it sets up the venv and installs deps.
- Tests assume the binary is at `./cloudbackup` in the *current working
  directory* (see `cmd_default = "./cloudbackup"` in `common.py`). Running
  from `integration_tests/` will fail with FileNotFoundError unless you
  copy or symlink the binary.
- Tests that allocate ports (`base_url` with random ports) can race in
  parallel. Always run a single test with `-v`; do not parallelise here.
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
