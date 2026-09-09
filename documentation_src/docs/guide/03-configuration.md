# 3. Server configuration reference

The server is configured with a single YAML file, passed via
`cloudbackup server start -c <file>`. This chapter documents **every** option,
its default, validation rules and when you would use it.

Three commands you will use constantly while editing configuration:

```bash
cloudbackup server config example              # print a fully commented example
cloudbackup server config validate -c FILE     # syntax + semantic validation
cloudbackup server config dump -c FILE         # print the parsed result (JSON, secrets masked)
```

`dump` matters more than it sounds: YAML indentation errors often *parse fine*
but land a setting in the wrong block. The dump shows the structure the server
will actually run with, defaults included.

Configuration is read once at startup — after editing the file, restart the
service (`sudo systemctl restart cloudbackup` / `Restart-Service cloudbackup`).
(The configuration can also be changed at runtime through the HTTP API's
`POST /api/v1/config`, which persists to the same file — see
[chapter 8](08-http-api.md).)

## 3.1 Top-level layout

```yaml
data_dir: /var/lib/cloudbackup      # required
restore_dir: /srv/restores          # optional
html_dir: /usr/share/cloudbackup/webstatic   # optional
user: [ ... ]                       # API users
http: { ... }                       # plain-HTTP listener
https: { ... }                      # TLS listener (disables http when enabled)
backup: [ ... ]                     # one or more backup definitions
notification: { ... }               # optional email / script notifications
```

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `data_dir` | **yes** | — | Directory for the server's internal SQLite databases (one per backup job) and job reports. Must exist and be writable by the user running the daemon. Protect it: it contains the metadata that powers diffing, reports and restores. |
| `restore_dir` | no | `<data_dir>/restores` | Base directory into which restored files are written. Each restore job creates `<restore_dir>/<backup-name>/<restore-job-id>/` underneath it. A restore request can override this per job. |
| `html_dir` | no | `webstatic` (relative to the working directory) | Where the web UI, documentation and Swagger assets live. The Linux packages set this to `/usr/share/cloudbackup/webstatic`; the Windows installer's sample config points at the install directory. |

## 3.2 API users (`user`)

```yaml
user:
  - name: admin
    pass: $2a$05$Ug1eUCXbSYU...   # bcrypt hash — cloudbackup misc hash-password
    access: write
  - name: monitoring
    pass: $2a$05$Pgdwe14mHjO...
    access: read
```

| Option | Required | Default | Description |
|--------|----------|---------|-------------|
| `name` | yes | — | Username for HTTP Basic Auth. |
| `pass` | yes | — | **bcrypt hash** of the password. Generate with `cloudbackup misc hash-password`. |
| `access` | no | `read` | `read` or `write`. `write` grants the whole API. `read` permits only a fixed read-only subset: listing backups and restores, dry-runs, watching progress, listing backup/restore reports, and reading the config. |

**Scenarios:** give `write` to admins and to the credentials used by the web UI
if operators start/stop jobs from it; give `read` to dashboards or monitoring
scripts that only list jobs and reports.

## 3.3 Choosing what the daemon can read

The daemon backs up files *as the OS user it runs as*. The packaged Linux
service runs as `root` by default, so it can read any path you point it at. If
you would rather run it unprivileged, the
[installation chapter](01-installation.md#note-on-the-service-account) shows how
to override the unit — and what stops being readable once you do.

## 3.4 `http` and `https`

```yaml
http:
  bind_address: "127.0.0.1:8080"

https:
  enabled: true
  bind_address: "0.0.0.0:8443"
  ssl_cert_path: /etc/cloudbackup/cert.crt
  ssl_key_path: /etc/cloudbackup/cert.key
```

| Option | Default | Description |
|--------|---------|-------------|
| `http.bind_address` | `127.0.0.1:8080` | Listen address for plain HTTP. |
| `https.enabled` | `false` | When `true`, the HTTPS listener is started **and the HTTP listener is disabled** — only one of the two ever runs. |
| `https.bind_address` | `127.0.0.1:8443` | Listen address for HTTPS. |
| `https.ssl_cert_path` | — | PEM certificate (required when HTTPS is enabled; the file must exist). |
| `https.ssl_key_path` | — | PEM private key (required when HTTPS is enabled). |

**Scenarios:**

* *Single machine, local CLI only* — keep the default `127.0.0.1:8080`; nothing
  is exposed to the network.
* *Backup server managed from other machines* — enable `https` on
  `0.0.0.0:8443` with a certificate the client machines trust. Basic Auth
  credentials travel with every request, so avoid plain HTTP across a network.

## 3.5 `backup` — backup definitions

Each list entry is one independent job:

```yaml
backup:
  - name: documents
    paths:
      - /srv/documents
      - /etc
    exclusions:
      - "**/*.tmp"
      - /srv/documents/cache
    dereference: true
    checksum: false
    schedule:
      - '30 02 * * *'
    encrypt: false
    pre_run_script: /usr/local/bin/pre.sh
    post_run_script: /usr/local/bin/post.sh
    target:
      - ...
```

### `name` (required)

Unique across all backup definitions. Used in every CLI command, API call and
as the on-disk database filename, so it must be ASCII, must not contain `/` or
`\` or the reserved substring `__`, and cannot be `.` or `..`.

### `paths` (required)

List of **absolute** paths (files or directories) to back up. Directories are
walked recursively.

### `exclusions`

Glob patterns (Bash-like, with globstar) removing matches from the scan:

| Pattern element | Meaning |
|------|---------|
| `*` | any run of characters, except path separators |
| `**` | any run of characters, **including** path separators |
| `?` | any single character except a path separator |
| `[abc]`, `[a-z]` | single character from a set / range |
| `[^class]` | single character *not* in the class |
| `{alt1,alt2}` | either alternative |

Examples: `**/*.log` excludes every `.log` file anywhere; `/var/lib/*.db`
excludes `.db` files directly inside `/var/lib` but **not** in its
subdirectories.

> Always verify exclusion rules with `cloudbackup client backup dryrun <job>` —
> it lists exactly what would be examined, uploaded and excluded, without
> touching the object store.

### `dereference` (default: `true`)

How symbolic links are treated:

* `true` — links are followed; the *content they point to* is backed up.
* `false` — links are stored *as links* (their target path is recorded and
  recreated on restore).

Set `false` when backing up trees that contain internal symlinks (package
trees, `alternatives`-style layouts) to avoid duplicating content or escaping
the intended directory.

### `checksum` (default: `false`)

How "has this file changed?" is decided between runs:

* `false` — metadata comparison (size, timestamps). Fast; right for almost
  everyone.
* `true` — the file content is read and checksummed every run. Catches
  modifications that preserve size and timestamps, at the cost of reading every
  file on every run. Use for data where silent in-place changes are a real
  concern and the dataset is small enough to read each time.

### `schedule`

A list of standard **5-field cron expressions** (`minute hour day-of-month
month day-of-week`), plus the usual descriptors (`@daily`, `@hourly`,
`@weekly`, ...). Multiple entries are allowed. An empty/omitted list means the
job only runs when started manually (CLI, web UI or API).

```yaml
schedule:
  - '05 01 * * *'     # daily at 01:05
  - '00 13 * * 6'     # Saturdays at 13:00
```

Only one run of a given job can be active at a time.

### `encrypt` / `encrypt_pass`

Client-side encryption of file contents before upload. `encrypt: true`
requires a non-empty `encrypt_pass`. This feature has enough operational
implications to deserve its own chapter — read
[chapter 4 — Client-side encryption](04-encryption.md) **before** enabling it,
in particular what happens if the password is lost (unrecoverable data).

### `versions_max_num` / `versions_max_age`

```yaml
versions_max_num: 10    # keep at most 10 versions per file; 0 = unlimited
versions_max_age: 6w    # drop versions older than 6 weeks;  0 = unlimited
```

Intended retention limits for old versions of backed-up files.

> **Current status:** these two options are parsed and accepted, but retention
> enforcement is **not yet implemented** in this version — no versions are
> deleted regardless of the values. Manage retention with your bucket's
> lifecycle rules for now, and be careful that such rules don't delete versions
> you still need.

### `pre_run_script` / `post_run_script`

Absolute path to a script or executable to run before / after each run of this
job.

* On Unix-likes the file must be executable (`+x`); on Windows it must be a
  `.bat` or `.ps1` script (or an executable).
* The script receives **one argument: the job id** (a UUID).
* The file must exist at server start, otherwise the server refuses to start.
* **Pre-run:** a non-zero exit code cancels the whole backup run and marks it
  failed; the script's combined stdout+stderr is captured into the error log
  (large output increases server memory usage). Cancelling a job while the
  pre-run script is executing waits for the script to finish — it is not
  killed.
* **Post-run:** runs regardless of whether the backup finished, failed or was
  cancelled.

**Scenario:** snapshot a database before the run and release the snapshot after:

```yaml
pre_run_script: /usr/local/bin/take_db_snapshot.sh
post_run_script: /usr/local/bin/remove_db_snapshot.sh
```

## 3.6 `target` — where the data goes

Each backup definition needs at least one target; with several targets every
file is uploaded to each of them (e.g. S3 *and* Azure for provider
redundancy).

```yaml
target:
  - name: s3-main
    type: aws_s3
    bucket: 'my-backup-bucket'
    prefix: 'backups/server-01'
    ratelimit: 10 MB
    parameters:
      - name: AWS_ACCESS_KEY_ID
        value: AKIA...
      - name: AWS_SECRET_ACCESS_KEY
        value: ...
```

| Option | Required | Description |
|--------|----------|-------------|
| `name` | yes | Target name, referenced by restore commands (`--target`). Same character rules as backup names. |
| `type` | yes | `aws_s3`, `gcp_storage` or `azure_blob`. (`test_null` exists for the test suite; it discards data.) |
| `bucket` | yes | Bucket / container name. |
| `prefix` | yes | Key prefix under which this server's data is stored. |
| `ratelimit` | no (default `0`) | Upload rate limit for this target, in bytes/second; accepts human units (`500 KB`, `10 MB`). `0` or unset = unlimited. Useful to keep a backup from saturating an office uplink. |
| `parameters` | per type | List of `{name, value}` pairs, documented per provider below. Parameter *names* are case-insensitive; *values* are case-sensitive unless noted. |

**Shared advice for every provider:**

* Dedicate buckets to backups only, and give each server its own `prefix`.
  Anything else writing under the same keys can corrupt backups.
* Lock the bucket down so only the backup credentials can read/write it.
* After configuring a target, run
  `cloudbackup client backup target test <job>` to verify connectivity,
  credentials and permissions before relying on it.

### 3.6.1 `aws_s3`

**Required bucket setup:** enable
[S3 bucket versioning](https://docs.aws.amazon.com/AmazonS3/latest/dev/Versioning.html).
Also consider a lifecycle rule that aborts/purges incomplete multipart uploads
older than a few days.

| Parameter | Required | Description |
|-----------|----------|-------------|
| `AWS_ACCESS_KEY_ID` | no* | Access key id. *If set, `AWS_SECRET_ACCESS_KEY` must be set too (and vice-versa). If neither is set, the AWS SDK's [standard credential resolution](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#specifying-credentials) is used — environment, shared credentials file, or EC2/ECS instance roles. |
| `AWS_SECRET_ACCESS_KEY` | no* | Secret access key; see above. |
| `storage_class` | no | One of `STANDARD`, `REDUCED_REDUNDANCY`, `STANDARD_IA`, `ONEZONE_IA`, `INTELLIGENT_TIERING` (case-sensitive). |
| `region` | no | Lower-case AWS region (e.g. `us-east-1`). Only used as a fallback when the bucket's region cannot be auto-detected via the S3 API. |

**Scenario — EC2 instance with an IAM role:** omit both key parameters and
grant the instance role access to the bucket; credentials never live in the
config file.

### 3.6.2 `gcp_storage`

**Required bucket setup:** enable
[object versioning](https://cloud.google.com/storage/docs/using-object-versioning).

Credentials come from a
[service account key](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
JSON file. The credential parameters below are **all-or-nothing**: specify none
of them (then [Application Default Credentials](https://cloud.google.com/docs/authentication/production#finding_credentials_automatically)
are used — right choice on GCE/GKE) or all of them.

| Parameter | Description |
|-----------|-------------|
| `type`, `project_id`, `private_key_id`, `private_key`, `client_email`, `client_id`, `auth_uri`, `token_uri`, `auth_provider_x509_cert_url`, `client_x509_cert_url` | Copied field-for-field from the service account JSON key file. **`private_key` must be wrapped in double quotes** (not single) so the embedded `\n` escapes survive YAML parsing. |
| `storage_class` | Optional; one of `multi_regional`, `regional`, `nearline`, `coldline`. |
| `disable_crc32c_hash` | Optional, default `false`. When `false`, a CRC32C hash is computed for each upload and GCP verifies it server-side — at the cost of one extra local read per file. Set to `yes`/`true` only if that read is too expensive; keeping the check on is strongly advised. |

### 3.6.3 `azure_blob`

| Parameter | Required | Description |
|-----------|----------|-------------|
| `storage_account` | **yes** | Storage account name (must be a Blob Storage account). |
| `storage_access_key` | **yes** | Storage account access key. |
| `primary_blob_service_endpoint` | no | Override endpoint URL (HTTPS only), as shown in the Azure portal under *Storage accounts → your account → Properties → Primary Blob Service Endpoint*. Defaults to `https://<storage_account>.blob.core.windows.net/`. |

## 3.7 `notification`

Optional. Fires on backup/restore job lifecycle events. Two mechanisms, each a
list (so different recipients/scripts can subscribe to different events):

Event types: `started`, `finished`, `failed`, `cancelled`, `crashed`.
When `type` is omitted, a block defaults to `failed` + `crashed` — the "only
tell me when something is wrong" setting.

### 3.7.1 `notification.email`

```yaml
notification:
  email:
    - server: smtp.example.com
      port: 587                # default "25"
      user: backup-notify      # omit user/pass for localhost SMTP
      pass: 'secret'
      from: backup@example.com # optional
      to: ops@example.com      # exactly one recipient
      cc:                      # optional, multiple allowed
        - manager@example.com
      type: [failed, crashed]
```

| Option | Required | Default | Notes |
|--------|----------|---------|-------|
| `server` | yes | — | SMTP server address. Gmail caps direct sending (~99 mails/day/account). |
| `port` | no | `25` | |
| `user` / `pass` | no | — | SMTP auth. Can be omitted when `server` is `127.0.0.1`/`localhost`. |
| `from` | no | — | Sender address. |
| `to` | yes | — | Single recipient. |
| `cc` | no | — | List of additional recipients. |
| `type` | no | `[failed, crashed]` | Events this block fires on. |

### 3.7.2 `notification.script`

```yaml
notification:
  script:
    - path: /usr/local/bin/notify_hook.sh
      type: [started, finished, failed, cancelled, crashed]
```

The script (executable, or `.bat`/`.ps1` on Windows) is invoked with **six
arguments**:

```
JobType JobName JobId JobState JobError ReportFile
```

`ReportFile` is the path to a plain-text file containing the job's JSON report
(the `ResultBackupJobStatus` model in the [API documentation](08-http-api.md)).
Use it to feed Slack webhooks, ticketing systems, or metrics.

Test the whole notification setup at any time:

```bash
cloudbackup client notification test
```

---

Next: [4. Client-side encryption](04-encryption.md)
