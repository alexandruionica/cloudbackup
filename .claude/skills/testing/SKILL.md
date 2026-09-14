---
name: testing
description: How cloudbackup is tested and how to add or change tests — the Go unit tests, the tiered Python suite under integration_tests/ (acceptance, api, ui, cloud), the fixtures and helpers in integration_tests/lib and conftest.py, the previous-release data fixture, the package install smoke tests, and the conventions each tier follows. Use when the user asks to add, extend, review or run tests, asks "is X covered", wants a new user-acceptance scenario, touches integration_tests/, or when a code change needs a matching test. For a single failing Python test use debug-integration-test instead.
---

# Testing — layers, tiers and how to add to them

Development on this project is prompt-driven; the suite is the regression net. Every
behaviour a user can reach should be covered at the level that would catch a
language model getting it wrong. The full reference lives in
`developer_documentation/docs/testing.md`; this skill is the working summary.

## The layers

| Layer | Where | Run with | Needs |
|---|---|---|---|
| Go unit | `*_test.go` next to the code | `make gotest`, `make gotestrace` | Go |
| Web UI unit | `webstatic/ui/tests/` | `make uitest` | Node >= 18 (`nvm use 20`) |
| Python tiers | `integration_tests/<tier>/` | `make inttest`, `make uitest-browser`, `make inttest-cloud` | Python 3.12+, built binary |
| Package smoke | `packaging/smoke-test.sh` and per-OS siblings | `make packages` (Linux), release workflow (others) | Docker / target OS |

`make test` = lint + Go + web UI unit. `make alltest` = everything, cloud tier included
(it skips per provider without `CLD_*` credentials, so it passes without them but proves
less). The pre-push hook runs `make alltest`.

## The Python tiers (integration_tests/)

- **`acceptance/`** — user acceptance. Drives the shipped `./cloudbackup` binary the way
  the user guide tells users to: `cli_*.py` per command group, `scenario_*.py` per guide
  chapter (getting started, incremental runs, HTTPS), `fixture_realism.py` for symlinks /
  empty / large / hard-linked / long-named files.
- **`api/`** — REST contract: `rest_api_*.py`, the schemathesis Swagger run, encryption
  lifecycle, `scenario_*.py` for things only the API exposes (resume, scheduled run,
  config persistence), `compat_previous_release.py`.
- **`ui/`** — Playwright driving the web UI in headless Chromium (user guide chapter 7).
- **`cloud/`** — real AWS S3 / Azure Blob / GCP Storage. Every module is decorated with
  `@requires_env(...)` and skips without credentials. The object-store cleanup runs after
  this tier only.
- **`lib/common.py`** — shared helpers. **`conftest.py`** — pytest fixtures. **`fixtures/`** —
  previous-release data plus the generator script.

Run one module or test from the repository root after `make build`:

```bash
integration_tests/.venv_linux/bin/python -m pytest integration_tests/acceptance/cli_restore.py -v
integration_tests/.venv_linux/bin/python -m pytest \
    "integration_tests/api/rest_api_restore.py::TestRestAPIRestore::test_restore_full_lifecycle" -v
integration_tests/.venv_linux/bin/python -m pytest integration_tests/acceptance -m "not slow" -q
```

The venv is per OS (`.venv_linux`, `.venv_macos`, `.venv_freebsd`, `.venv_windows`);
`./integration_tests.sh` creates it. pytest must stay >= 9 (schemathesis requires it).

## Writing a new Python test

Use the function style with the conftest fixtures, not a copied `setUp()`:

- `source_tree` — temp tree with Unicode / metacharacter names (`.root`, `.filelist`).
  For more shapes call `setup_dir_with_tmp_files(symlinks=True, empty_file=True,
  large_file=True, hard_link=True, long_name=True)` directly.
- `server_config` — temp server config whose `first_backup` backs up `source_tree`
  through a **persistent** `test_null` target; `server_config.patch(fn)` edits it before
  the daemon starts.
- `daemon` — running daemon on a free port with `--logfile`; `daemon.base_url`.
- `client_config` — client config for `testuser1` (write access) pointing at `daemon`.
- `api` / `api_readonly` — `ApiClient`: `run_backup_and_wait()`, `backup_report()`,
  `start_restore()`, `wait_restore_finishes()`, `restore_report()`,
  `set_target_ratelimit()` (make instant test_null transfers observable), `raw_post()`.
- `restore_dir` — empty temp directory.

Helpers in `common.py`: `run_cli("client backup list --json", client_config)` returns a
`CliResult` (`.returncode`, `.stdout`, `.lines`, `.json()`), asserting exit code 0 unless
`expect=` says otherwise; `check_backup_report()`, `check_restore_report()`,
`assert_counters(api, stats, {...})`, `verify_restored_tree()` (md5 of every file, symlink
targets, counts), `verify_restored_subset()`, `decode_json_stream()` for CLI `--json`
output that mixes pretty-printed and single-line documents, `count_files_folders_links()`
(mirrors the server's dereference handling), `make_self_signed_cert()`.

Conventions:

- Put the test in the tier that matches what it proves. A user-visible journey is
  acceptance; a status code or JSON shape is api.
- The docstring names the user-guide section the scenario mirrors.
- Assert on `--json` output and parsed structure, never on line counts or exact
  human-readable text.
- Never hardcode a port; never use a bare `return` to skip — `self.skipTest()` or
  `pytest.skip()` so the summary shows it.
- Anything that restores, runs a job twice or checks encryption end to end needs the
  persistent store (the `server_config` fixture, or
  `setup_tmp_config_file_and_tmp_dirs(..., persistent_null_store=True)`).
- Mark tests that wait for a cron tick `@pytest.mark.slow`.
- When a scenario exposes a product bug, fix the product in its own commit and keep the
  test as written. Pin current behaviour in a test only when it is a gap rather than a
  defect, and say so in the docstring (example: an unknown source job id on restore
  start is accepted, then vanishes without a report row).

## test_null, the credential-free backend

Hidden target type used by unit and integration tests. By default it drains uploads into
an in-process map. With the target parameter `persist_dir` (absolute path) every object
and the encryption keystore sidecar are written to disk, keyed by remote path **and
version**, so restores, incremental runs and point-in-time restores behave like a
versioned bucket. A `ratelimit` on the target slows its instant transfers down so a
running job can be watched or stopped.

## Previous-release data

`integration_tests/fixtures/release_<tag>/` holds databases, config and `meta.json`
written by that release's binary; `api/compat_previous_release.py` runs the current
binary on it. Regenerate or add a release with the old binary built from its tag:

```bash
git worktree add /tmp/cb-v0.0.3 v0.0.3
(cd /tmp/cb-v0.0.3 && bash generate_version.sh && go build -mod=vendor -o cloudbackup .)
integration_tests/.venv_linux/bin/python integration_tests/fixtures/make_release_fixture.py \
    --binary /tmp/cb-v0.0.3/cloudbackup --version v0.0.3
```

The databases store absolute paths, so the fixture is tied to the OS family that made it.
When the database schema changes, this test is what forces a migration path.

## Package install smoke

Each installer family has a smoke test that installs the package, checks files /
ownership / service, edits the sample config, starts the daemon through the service
mechanism, drives it with the CLI client and removes it. Linux runs in Docker after every
`make packages` build (`SMOKE=0` skips); FreeBSD, macOS and Windows run in the release
workflow. A packaging change to a path, owner, mode or service name needs the matching
smoke script updated (see the packaging skill).

## Go unit tests

Table tests next to the code. Weak spots that have no other net and where new code needs
tests first: `restore` (restoreOne / Resume paths), `scheduler` (lifecycle and dispatch),
`daemon`, and the `objectstore` helpers. The cloud backends in `objectstore` are only
covered by the cloud tier. Use `objectstore.InitialiseStoreTestNull` with a `persist_dir`
parameter for anything that must read back what it uploaded, and `shared.NewJobsState()`
plus `MarkRestoreRunning()` when counters must be observable.

## What to run before declaring a test change done

1. `integration_tests/.venv_linux/bin/flake8 --ignore E501,F401,F403,F405,W504,W605 integration_tests/ --exclude='.venv*'`
2. The module(s) you touched, then the tier they live in.
3. `make testcp` and the Go package(s) touched, with `-race` for anything concurrent.
4. If docs changed: `make docs` and `(cd developer_documentation && ./generate_docs.sh)`.
