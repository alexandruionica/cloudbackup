# 5. CLI client setup

Every `cloudbackup client ...` command needs three things: the server
**address**, a **username** and a **password** (a user from the server config —
see [chapter 3, §3.2](03-configuration.md#32-api-users-user)). You can supply
them in three ways; from highest to lowest precedence:

1. **Command-line flags** — `-a/--address`, `-u/--username`, `-p/--password`
2. **Environment variables** — `CLOUDBACKUP_CLIENT_ADDRESS`,
   `CLOUDBACKUP_CLIENT_USERNAME`, `CLOUDBACKUP_CLIENT_PASSWORD`
3. **Client configuration file**

## 5.1 The client configuration file

Default location:

| Platform | Path |
|----------|------|
| Linux / Unix | `$HOME/.cloudbackup.yaml` |
| Windows | `%HomeDrive%%HomePath%\.cloudbackup.yaml` (e.g. `C:\Users\you\.cloudbackup.yaml`) |

A different file can be passed to any command with `-c/--configfile`.

Format (print it any time with `cloudbackup client config example`):

```yaml
---
username: admin
password: 'the-plaintext-password'
address: http://127.0.0.1:8080
```

* `address` must include the scheme: `http://host:port` or `https://host:port`.
* `password` is the plaintext password (the *server* stores only the bcrypt
  hash). Restrict the file's permissions accordingly:
  `chmod 600 ~/.cloudbackup.yaml`.
* With `https://` addresses, the server's certificate must be trusted by the
  client machine (public CA or the CA/cert added to the system trust store).

## 5.2 Validating the client configuration

```bash
# Is the merged configuration (file + env vars + flags) complete and valid?
cloudbackup client config validate

# Show the merged result (password masked)
cloudbackup client config dump

# End-to-end test: fetch the server's version over the API
cloudbackup client server-version
```

## 5.3 Common flags on every client command

| Flag | Description |
|------|-------------|
| `-c, --configfile FILE` | Use this client config file instead of the default location |
| `-u, --username` / `-p, --password` / `-a, --address` | Override credentials/address for this invocation |
| `--json` | Print the raw JSON response from the server instead of formatted text/tables — ideal for scripting (pipe into `jq`) |
| `-d, --debug` | Debug logging — **prints secrets**; use only while troubleshooting |
| `--jsonlog` | Log messages as JSON instead of plaintext |

**Scenario — cron/automation without a config file:** pass credentials through
the environment so nothing is on the command line (visible in `ps`) and no
file needs distributing:

```bash
export CLOUDBACKUP_CLIENT_ADDRESS="https://backup.internal:8443"
export CLOUDBACKUP_CLIENT_USERNAME="automation"
export CLOUDBACKUP_CLIENT_PASSWORD="..."
cloudbackup client backup start documents --json
```

---

Next: [6. CLI usage](06-cli-usage.md)
