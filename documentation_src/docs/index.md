# CloudBackup Documentation

CloudBackup backs up files from Linux, FreeBSD and Windows systems into cloud
object stores (AWS S3, Azure Blob Storage, GCP Cloud Storage), with a server
daemon, a CLI client, a web UI and a Swagger-documented HTTP API.

**Start here → [User Guide](guide/README.md)**

| Chapter | Contents |
|---------|----------|
| [1. Installation](guide/01-installation.md) | Pre-built packages: Linux `.deb`/`.rpm`, Windows `.msi` and portable zip |
| [2. Getting started](guide/02-getting-started.md) | First-run configuration and your first backup |
| [3. Configuration reference](guide/03-configuration.md) | Every server configuration option explained |
| [4. Client-side encryption](guide/04-encryption.md) | How encryption works and how to operate it safely |
| [5. CLI client setup](guide/05-cli-setup.md) | Client config file, environment variables, flags |
| [6. CLI usage](guide/06-cli-usage.md) | Backups, monitoring, reports, restores from the command line |
| [7. Web UI](guide/07-web-ui.md) | Using the built-in browser interface |
| [8. HTTP API](guide/08-http-api.md) | Scripting the server with `curl` |

Other resources:

* **API (Swagger)** — the *API* item in the navigation opens the interactive
  Swagger UI served by this same server.
* **[Architecture](architecture.md)** — internals overview for the curious.
* **Offline copy** — [cloudbackup-user-guide.html](cloudbackup-user-guide.html)
  is this entire user guide as a single self-contained HTML file; save it
  anywhere, read it without a network, or print it to PDF from your browser.
