# 6. CLI usage

This chapter covers day-to-day operations with `cloudbackup client`. All
commands assume the client is configured as per [chapter 5](05-cli-setup.md);
all accept `--json` for machine-readable output.

Command map:

```
cloudbackup client
├── backup    start | stop | list | status | watch | dryrun
│   ├── target   test
│   └── report   list | show
├── restore   start | stop | list | watch
│   └── report   list | show
├── notification  test
├── config    validate | dump | example
├── version
└── server-version
```

## 6.1 Backups

### Start a backup

```bash
cloudbackup client backup start documents
cloudbackup client backup start documents --watch    # start and follow progress live
```

The response includes the run's **job id** (a UUID) — you'll use it with
`stop`, `watch`, `report show` and `restore start`.

### Monitor running backups

```console
$ cloudbackup client backup list
NAME        STATE     JOB ID                                START TIME            NEXT RUN
documents   running   1b2b7f96-...                          2026-07-10T02:30:00Z  ...
http_logs   stopped   -                                     -                     2026-08-01T08:00:00Z
```

```bash
# Detailed live counters for one job: examined/uploaded/failed files,
# transfer rates (1/5/15 min), current file, per-target rates ...
cloudbackup client backup status documents

# Live per-file event stream (what is being examined/uploaded right now)
cloudbackup client backup watch documents
cloudbackup client backup watch documents -i <job-id>   # only if this exact run is active
```

`watch` is best-effort: if the server produces events faster than the client
consumes them, events are dropped rather than slowing the backup down. Watching
requires only `read` access, so it's safe to hand to operators.

### Stop a running backup

```bash
cloudbackup client backup stop documents
cloudbackup client backup stop documents -i <job-id>   # guard: only stop this exact run
```

The `-i` form protects scripts against races — if the id doesn't match the
currently running instance of that job, nothing is stopped.

### Dry run

```bash
cloudbackup client backup dryrun documents
```

Walks the configured paths and prints what would be examined, uploaded and
excluded — without contacting the object store. Use it after every change to
`paths` or `exclusions`.

### Test targets

```bash
cloudbackup client backup target test documents
```

Verifies that every target of the job is reachable and the credentials allow
the needed operations. Run it after configuring a new target or rotating
credentials.

## 6.2 Backup reports

Every run leaves a report on the server.

```bash
# Runs of the last 30 days (the default window)
cloudbackup client backup report list documents

# Custom window (RFC3339 timestamps)
cloudbackup client backup report list documents \
    --from-start-time 2026-06-01T00:00:00Z \
    --until-start-time 2026-07-01T00:00:00Z

# Full report of one run: state, timings, counters (uploaded / up-to-date /
# failed / excluded ...), transfer totals, per-target details, script results
cloudbackup client backup report show documents -i <job-id>
```

**Scenario — nightly verification script:**

```bash
state=$(cloudbackup client backup report show documents -i "$id" --json | jq -r .result.state)
[ "$state" = finished ] || alert "backup $id ended in state $state"
```

## 6.3 Restores

### Start a restore

Restores pull files from a **specific past backup run**, so first find its job
id:

```bash
cloudbackup client backup report list documents
```

Then choose one of three selection modes:

```bash
# 1. Interactive: no --file/--all-files opens a terminal file browser
#    in which you tick files/directories to restore
cloudbackup client restore start documents -i <backup-job-id>

# 2. Explicit files (repeat --file as needed)
cloudbackup client restore start documents -i <backup-job-id> \
    --file /srv/documents/report.odt --file /srv/documents/2026/

# 3. Everything from that run
cloudbackup client restore start documents -i <backup-job-id> --all-files
```

Options:

| Flag | Description |
|------|-------------|
| `-i, --job-id` | **Required.** The backup run to restore from. |
| `--file PATH` | Absolute source path to restore; repeatable. Mutually exclusive with `--all-files`. |
| `--all-files` | Restore every file, directory and symlink from the run. |
| `--exclusion GLOB` | Glob pattern of files to skip (same syntax as backup exclusions); repeatable. |
| `--target NAME` | Which of the job's targets to fetch from (default: the first one). |
| `--restore-dir DIR` | Server-side destination override. |
| `-N, --non-interactive` | Fail instead of opening the file browser when no files were specified (for scripts). |
| `-w, --watch` | Watch progress immediately after starting. |

Files are written **on the server**, under
`<restore_dir>/<backup-name>/<restore-job-id>/` (default `restore_dir` is
`<data_dir>/restores`) — restores never overwrite the original files in place.
Copy or move them from there once the restore finishes.

> Restoring from an encrypted backup requires the keystore sidecar in the
> bucket and the correct `encrypt_pass` in the server config — see
> [chapter 4](04-encryption.md).

### Monitor and stop restores

```bash
cloudbackup client restore list                       # running restore jobs
cloudbackup client restore watch documents            # live per-file progress
cloudbackup client restore watch documents -i <restore-job-id>
cloudbackup client restore stop documents
cloudbackup client restore stop documents -i <restore-job-id>
```

### Restore reports

```bash
cloudbackup client restore report list documents
cloudbackup client restore report show documents -i <restore-job-id>
```

Same shape as backup reports: state, timings, per-file counters and errors.

> A failed or stopped restore can be **resumed** (skipping already-restored
> files) from the web UI or via the API's `/restore/resume` endpoint — see
> [chapter 7](07-web-ui.md) and [chapter 8](08-http-api.md). There is no CLI
> resume command at present; re-running `restore start` starts a fresh restore
> into a new directory.

## 6.4 Notifications

```bash
cloudbackup client notification test
```

Triggers every configured notification (each email block, each script) with a
test event, so you validate SMTP credentials and script plumbing without
waiting for a real failure.

## 6.5 Versions

```bash
cloudbackup client version          # version of the local binary
cloudbackup client server-version   # version info of the server, via the API
```

## 6.6 Server-side commands (not part of `client`)

These run on the backup server machine itself, against the server config file:

| Command | Purpose |
|---------|---------|
| `cloudbackup server start -c FILE` | Run the daemon ([chapter 2](02-getting-started.md#25-start-the-server)) |
| `cloudbackup server version` | Version + build info of the binary |
| `cloudbackup server config validate -c FILE` | Validate a server config |
| `cloudbackup server config dump -c FILE` | Print the parsed config (secrets masked) |
| `cloudbackup server config example` | Print a fully commented example config |
| `cloudbackup server reset-keystore -c FILE JOB` | Reset a job's encryption state ([chapter 4, §4.4](04-encryption.md#44-starting-over-after-sidecar-loss-reset-keystore)) |
| `cloudbackup misc hash-password` | bcrypt-hash a password for the `user:` section |

---

Next: [7. Web UI](07-web-ui.md)
