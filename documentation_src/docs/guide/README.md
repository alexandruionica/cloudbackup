# CloudBackup User Guide

CloudBackup is an open-source backup tool that backs up files from Linux, FreeBSD and
Microsoft Windows systems into cloud object stores — **AWS S3**, **Azure Blob Storage**
and **GCP Cloud Storage** — with file-level granularity.

A single binary provides everything:

* a **server** (daemon) that scans, uploads, restores, schedules and reports,
* a **CLI client** that talks to the server over its HTTP API,
* a built-in **web UI** and **Swagger-documented HTTP API**, both served by the daemon.

## How to read this guide

The chapters are ordered the way a new installation proceeds. If you are setting up
CloudBackup for the first time, read chapters 1–2 in order; the rest are reference
chapters you can dip into as needed.

| # | Chapter | What it covers |
|---|---------|----------------|
| 1 | [Installation](01-installation.md) | Installing pre-built packages on Linux (`.deb`/`.rpm`) and Windows (`.msi`, portable `.zip`); upgrading; uninstalling |
| 2 | [Getting started](02-getting-started.md) | Core concepts, first-run configuration, starting the server, your first backup |
| 3 | [Server configuration reference](03-configuration.md) | Every configuration option, defaults, validation rules and usage scenarios |
| 4 | [Client-side encryption](04-encryption.md) | How encryption works, the keystore sidecar, operational do's and don'ts |
| 5 | [CLI client setup](05-cli-setup.md) | The client configuration file, environment variables and connection options |
| 6 | [CLI usage](06-cli-usage.md) | Day-to-day tasks: starting, watching and stopping backups; reports; restores |
| 7 | [Web UI](07-web-ui.md) | Using the built-in browser UI |
| 8 | [HTTP API](08-http-api.md) | Authentication, response format, common calls with `curl`, live watch streams |

## Other formats

* **On GitHub** — you are probably reading it there right now; all chapters are plain
  Markdown and cross-linked.
* **Served by the daemon** — every CloudBackup server serves this guide at
  `http(s)://<server>/docs/` and the interactive Swagger API reference at
  `http(s)://<server>/docs_api/`. No authentication is required for the documentation
  pages.
* **Offline / single file** — the documentation build also produces a single
  self-contained HTML file, `cloudbackup-user-guide.html` (in `webstatic/docs/` in the
  repository, also served at `http(s)://<server>/docs/cloudbackup-user-guide.html`).
  It works from a local disk without a web server and prints cleanly, so you can use
  your browser's *Print → Save as PDF* to produce a PDF copy.

## Quick demos

* CLI usage and documentation access: <https://www.youtube.com/watch?v=wyoO3pm_fmY>
* Web UI tour: <https://youtu.be/EFjg5-VDSu8>
