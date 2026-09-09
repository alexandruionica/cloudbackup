# 7. Web UI

The daemon ships a minimalistic single-page web UI, served at:

```
http(s)://<server-address>/ui/
```

(`http(s)://<server-address>/` redirects there.) The UI's static files are
served without authentication; every API call the UI makes carries your HTTP
Basic Auth credentials.

> **No internet access required.** The UI ships its entire runtime (Preact and
> htm) inside `webstatic/ui/vendor/`, served by the daemon itself, so it works
> unchanged on airgapped and DMZ networks. Nothing on the page is fetched from
> a third-party host.

## 7.1 Connecting

Click **Connection** (top right) and fill in:

* **Server URL** — e.g. `http://127.0.0.1:8080` (usually pre-filled with the
  address you loaded the UI from),
* **Username** / **Password** — a user from the server config
  ([chapter 3, §3.2](03-configuration.md#32-api-users-user)).

Settings are stored in the browser's local storage — on a shared machine,
prefer a `read`-access user. Users with `read` access can view jobs, watch
progress and browse report lists, but starting/stopping backups and restores
and reading full report details require `write` access.

The header shows the connected server's version, and links to this
**Documentation** and the **API (Swagger)** reference served by the same
daemon.

## 7.2 The job list

The main screen shows one card per backup definition, refreshed automatically
every few seconds. Each card shows the job's state (`running`, `stopped`,
`finished`, `failed`, ...), last/next run times and live counters while
running.

Per-card actions:

| Button | What it does |
|--------|--------------|
| **Start** / **Stop** | Start the job now / stop the currently running instance |
| **Watch** | Opens a live progress view — a streaming table of per-file events (examined, uploaded, up-to-date, excluded) with rates, as the backup runs |
| **Reports** | Past backup runs, newest first (paginated). Click a run for the full report: overview, transfer totals, examination/upload counters, deletion tracking, script results and client-side-encryption counters |
| **Restore** | Start a restore from this job (below) |
| **Restore reports** | Past restore runs, with details — and the **Resume** action |

When a restore is running for a job, the card also offers watch/stop for it.

## 7.3 Restoring from the UI

**Restore** opens a dialog where you:

1. pick the **backup run** (source job id) to restore from,
2. pick the **target** to fetch from (when the job has several),
3. select **what** to restore — either *all files*, or specific ones via the
   built-in file tree browser (directories expand lazily and large directories
   paginate),
4. optionally set a **destination directory** override and **exclusion**
   patterns.

Files are restored **onto the server**, under
`<restore_dir>/<backup-name>/<restore-job-id>/` unless overridden — same
semantics as the CLI ([chapter 6, §6.3](06-cli-usage.md#63-restores)).

## 7.4 Resuming a failed restore

In **Restore reports**, a restore that ended prematurely (failed, cancelled)
offers a **Resume** button (as long as no other restore is currently running
for that job). Resume continues into the *same* restore directory and skips
files that were already restored — useful for large restores interrupted by
network trouble.

---

Next: [8. HTTP API](08-http-api.md)
