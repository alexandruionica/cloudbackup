# 2. Getting started

This chapter takes you from a freshly installed package to a first successful
backup and restore.

## 2.1 Concepts in one minute

* **One binary, two roles.** `cloudbackup server ...` runs the backup engine as a
  daemon; `cloudbackup client ...` is a thin CLI that talks to a (possibly remote)
  server over its HTTP API. `cloudbackup misc ...` holds helpers such as password
  hashing.
* **Backup definition (job).** A named entry in the server config: which `paths`
  to back up, what to exclude, when (`schedule`), and where to store the data
  (one or more `target`s).
* **Target.** An object store destination: a bucket + key `prefix` in AWS S3,
  Azure Blob Storage or GCP Cloud Storage. A job with several targets uploads
  every file to each of them.
* **Local state.** The server keeps one SQLite database per backup job under
  `data_dir` — it records what was uploaded, when and where, and powers diffing,
  reports and restores. Losing it doesn't lose your data, but protect it anyway.
* **Users.** API access uses HTTP Basic Auth against users defined in the server
  config; `access: read` users can look but not touch, `access: write` users can
  do everything.

## 2.2 Hash a password

Passwords are stored in the server config as bcrypt hashes, never in plaintext:

```console
$ cloudbackup misc hash-password
Enter password:
Re-enter password:
The hashed password is: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
```

Copy the hash — you'll paste it into the config next.

## 2.3 Write a minimal server configuration

The packages install a commented starting point (`/etc/cloudbackup/config.yaml`
on Linux, `C:\ProgramData\cloudbackup\config.yaml` on Windows). A minimal but
complete config looks like this:

```yaml
# Where the server keeps its SQLite databases and reports
data_dir: /var/lib/cloudbackup

# Linux packages install the web assets here; omit html_dir if you run
# from a source checkout (it defaults to "webstatic" relative to the cwd)
html_dir: /usr/share/cloudbackup/webstatic

http:
  bind_address: "127.0.0.1:8080"

user:
  - name: admin
    pass: $2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.
    access: write

backup:
  - name: documents
    paths:
      - /srv/documents
    target:
      - name: s3-main
        type: aws_s3
        bucket: 'my-backup-bucket'
        prefix: 'backups/server-01'
        parameters:
          - name: AWS_ACCESS_KEY_ID
            value: AKIAIOSFODNN7EXAMPLE
          - name: AWS_SECRET_ACCESS_KEY
            value: wJalrXUtnFEMI/K7MDENG/bPxRfiCEXAMPLEKEY
    schedule:
      - '30 02 * * *'      # every night at 02:30
```

Two commands print full annotated examples at any time:

```bash
cloudbackup server config example     # server config, with every option commented
cloudbackup client config example     # client config (chapter 5)
```

> **Provider prerequisites:** for AWS S3 and GCP Storage you must enable **bucket
> object versioning** — CloudBackup relies on it to keep multiple versions of a
> file. Details and per-provider credential options are in
> [chapter 3](03-configuration.md).

## 2.4 Validate before starting

YAML indentation mistakes are easy to make and can silently move a setting into
the wrong section. Two commands protect you:

```bash
# 1. Syntax + semantic validation (schedules, paths, target parameters, ...)
cloudbackup server config validate -c /etc/cloudbackup/config.yaml

# 2. Dump the *parsed* configuration (as JSON, secrets masked) and eyeball it —
#    this is what the server will actually use, defaults included
cloudbackup server config dump -c /etc/cloudbackup/config.yaml
```

## 2.5 Start the server

On Linux (package install):

```bash
sudo systemctl start cloudbackup
journalctl -u cloudbackup -f
```

On Windows (MSI install): `Start-Service cloudbackup`.

Or in the foreground on any platform:

```bash
cloudbackup server start --configfile /etc/cloudbackup/config.yaml
```

Useful `server start` flags:

| Flag | Effect |
|------|--------|
| `-c, --configfile` | Path to the YAML config (required) |
| `-t, --textlog` | Human-readable log lines instead of the default JSON |
| `-l, --logfile FILE` | Log to a file instead of stdout |
| `-q, --quiet` | Suppress log output |
| `-d, --debug` | Debug logging — **warning: prints secrets and passwords** |

## 2.6 Point the CLI client at the server

Create `~/.cloudbackup.yaml` (the client's default config location; full details
in [chapter 5](05-cli-setup.md)):

```yaml
---
username: admin
password: 'the-plaintext-password-you-hashed'
address: http://127.0.0.1:8080
```

Check the connection end to end:

```console
$ cloudbackup client server-version
Server version: 0.0.2
...
```

## 2.7 Test the target, dry-run, back up

```bash
# 1. Can the server actually reach the bucket with these credentials?
cloudbackup client backup target test documents

# 2. What would be backed up? (also verifies your exclusion rules)
cloudbackup client backup dryrun documents

# 3. Run it, watching per-file progress live
cloudbackup client backup start documents --watch
```

While a job runs you can follow it from another terminal:

```bash
cloudbackup client backup list             # all jobs + brief status
cloudbackup client backup status documents # detailed live counters
cloudbackup client backup watch documents  # per-file event stream
```

Afterwards, inspect the report:

```bash
cloudbackup client backup report list documents
cloudbackup client backup report show documents -i <job-uuid>
```

## 2.8 Restore something

```bash
# Find the backup run to restore from
cloudbackup client backup report list documents

# No --file/--all-files: an interactive file browser opens in the terminal
cloudbackup client restore start documents -i <backup-job-uuid> --watch
```

Restored files land on the **server**, under
`<data_dir>/restores/<backup-name>/<restore-job-id>/` by default (configurable —
see `restore_dir` in [chapter 3](03-configuration.md), or override per restore
with `--restore-dir`).

## 2.9 The web UI and API docs

Open `http://127.0.0.1:8080/` in a browser — it redirects to the web UI at
`/ui/`. The same daemon serves this guide at `/docs/` and the interactive
Swagger API reference at `/docs_api/`. See [chapter 7](07-web-ui.md) and
[chapter 8](08-http-api.md).

> The default `bind_address` of `127.0.0.1` only accepts local connections. To
> use the CLI or web UI from another machine, bind to `0.0.0.0` (or a specific
> interface address) — and at that point enable HTTPS; see
> [chapter 3, §3.4](03-configuration.md#34-http-and-https).

---

Next: [3. Server configuration reference](03-configuration.md)
