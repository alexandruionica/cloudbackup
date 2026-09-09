# 8. HTTP API

Everything the CLI and web UI do goes through the server's HTTP API — you can
script against it directly.

* **Base path:** `http(s)://<server>/api/v1`
* **Authoritative reference:** interactive Swagger UI at
  `http(s)://<server>/docs_api/`; raw spec at `/swagger.yaml` and
  `/swagger.json`. This chapter is a practical introduction, the Swagger spec
  documents every field of every model.

## 8.1 Authentication and access levels

All `/api/v1` endpoints use **HTTP Basic Auth** with the users from the server
config. Use HTTPS whenever requests cross a network — Basic Auth sends the
password with every request.

`access: write` users can call everything. `access: read` users are limited to:

| Method | Read-accessible endpoints |
|--------|---------------------------|
| GET | `/config`, `/backup/list`, `/restore/list` |
| POST | `/backup/dryrun`, `/backup/watch`, `/restore/watch`, `/report/backup/list`, `/report/restore/list` |

Everything else (starting/stopping jobs, report *details*, config changes,
notification tests) returns `403` for read users.

## 8.2 Response envelope

Every JSON endpoint answers with the same envelope:

```json
{
  "code": "success",          // or "error"
  "message": "...",           // human-readable, set on errors
  "result": ...,              // payload: object or array
  "next": "..."               // pagination token, on list endpoints
}
```

List endpoints (`/report/backup/list`, `/report/restore/list`,
`/report/backup/file/list`) paginate: pass the returned `next` token in the
next request's `next` field until it comes back empty. `max_results` bounds the
page size.

## 8.3 Endpoint overview

| Method & path | Purpose |
|---------------|---------|
| `GET /report/version` | Server version info |
| `GET /backup/list` | All backup jobs with live status |
| `POST /backup/start` | Start a job — body `{"name": "..."}` |
| `POST /backup/stop` | Stop — body `{"name": "...", "job_id": "optional-guard"}` |
| `POST /backup/dryrun` | Dry-run stream for a job |
| `POST /backup/watch` | Live progress event stream (SSE-style, see §8.5) |
| `POST /backup/target/test` | Test all targets of a job |
| `GET /restore/list` | Running restore jobs |
| `POST /restore/start` | Start a restore (see §8.4) |
| `POST /restore/stop` | Stop — body `{"name": "...", "restore_job_id": "optional"}` |
| `POST /restore/resume` | Resume a partial restore — body `{"name", "target_name", "restore_job_id"}` |
| `POST /restore/watch` | Live restore progress stream |
| `POST /report/backup/list` | Past backup runs — body `{"name", "from_start_time", "until_start_time", "max_results", "next"}` (times RFC3339) |
| `POST /report/backup/show` | Full report — body `{"name", "job_id"}` |
| `POST /report/backup/file/list` | Browse files recorded in a run — body `{"name", "job_id", "path", "descend", "next"}` |
| `POST /report/restore/list` / `POST /report/restore/show` | Same, for restores |
| `POST /report/notification/test` | Fire test notifications |
| `GET /config` | Current server config (secrets masked as `*****`) |
| `POST /config` | Replace the server config (persists to the YAML file) |
| `POST /config/backup` | Replace only the `backup` section |

## 8.4 Worked examples

```bash
BASE=http://127.0.0.1:8080/api/v1
AUTH=admin:secret

# Server version
curl -su $AUTH $BASE/report/version

# List jobs and their live state
curl -su $AUTH $BASE/backup/list

# Start a backup
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents"}' $BASE/backup/start
# → {"code":"success", "result":{"name":"documents","job_id":"1b2b7f96-..."}}

# Stop it (job_id optional but recommended as a guard)
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents","job_id":"1b2b7f96-..."}' $BASE/backup/stop

# Backup runs of June 2026
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents",
          "from_start_time":"2026-06-01T00:00:00Z",
          "until_start_time":"2026-07-01T00:00:00Z"}' \
     $BASE/report/backup/list

# Full report of one run
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents","job_id":"1b2b7f96-..."}' $BASE/report/backup/show

# Restore two paths from that run
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents",
          "source_backup_job_id":"1b2b7f96-...",
          "files":["/srv/documents/report.odt","/srv/documents/2026/"]}' \
     $BASE/restore/start
# → {"code":"success","result":{"name":"documents","restore_job_id":"..."}}

# ... or everything, to a chosen directory, from a chosen target
curl -su $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents","source_backup_job_id":"1b2b7f96-...",
          "all_files":true,"target_name":"s3-main",
          "restore_dir":"/srv/restore-here"}' \
     $BASE/restore/start
```

## 8.5 Live watch streams

`POST /backup/watch`, `POST /restore/watch` and `POST /backup/dryrun` respond
with a long-lived stream of `data: <json>` lines (Server-Sent-Events style,
each event terminated by a single newline). Body:
`{"name":"documents"}` — optionally with `"job_id"` to bind to one specific
run.

```bash
curl -sN -u $AUTH -X POST -H 'Content-Type: application/json' \
     -d '{"name":"documents"}' $BASE/backup/watch
```

Each event carries: `sequence`, `name` (the file), `type`
(file/directory/symlink), `operation_type` (`examine`, `upload`, `metadata`,
`up_to_date`, `excluded`, `mark_deleted`), `percent_done`, `rate`,
`store_name`/`store_type` and `error`. The stream is best-effort — under load,
events are dropped rather than slowing the job — and ends with a plain-text
completion message.

## 8.6 Changing configuration over the API

`GET /config` returns the full parsed configuration with every secret replaced
by `*****`. `POST /config` accepts the same structure back: fields still set
to `*****` mean "keep the existing secret", so the usual flow is *GET → modify
→ POST* without ever handling plaintext secrets. The new configuration is
validated like a config file at startup, applied live, and **persisted to the
server's YAML config file**. `POST /config/backup` does the same for just the
`backup:` list.

---

Back to the [guide index](README.md)
