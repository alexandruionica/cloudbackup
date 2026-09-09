# 1. Installation

Pre-built packages are published on the project's GitHub Releases page:

> <https://github.com/alexandruionica/cloudbackup/releases>

Each release (tagged `v<version>`, e.g. `v0.0.2`) carries:

| Asset | Platform | Example file name |
|-------|----------|-------------------|
| Debian package | Debian 12, Ubuntu 24.04/26.04 — `amd64` and `arm64` | `cloudbackup_0.0.2-1~deb12_amd64.deb` |
| RPM package | RHEL/Alma/Rocky 8 and 9 — `x86_64` and `aarch64` | `cloudbackup-0.0.2-1.el9.x86_64.rpm` |
| Windows installer | Windows x64 and ARM64 | `cloudbackup_0.0.2_amd64.msi` |
| Windows portable zip | Windows x64 and ARM64 | `cloudbackup_0.0.2_windows_amd64.zip` |

For platforms without a pre-built package (FreeBSD, macOS, other Linux
distributions) build from source — see the
[README in the repository root](https://github.com/alexandruionica/cloudbackup#readme).

The same binary contains the server, the CLI client and all miscellaneous
commands; installing the package on a machine gives you all of them.

---

## 1.1 Linux (`.deb` / `.rpm`)

Download the package that matches your distribution and architecture, then:

```bash
# Debian / Ubuntu
sudo apt install ./cloudbackup_0.0.2-1~deb12_amd64.deb

# RHEL / Alma / Rocky
sudo dnf install ./cloudbackup-0.0.2-1.el9.x86_64.rpm
```

The package installs:

| Path | Purpose |
|------|---------|
| `/usr/bin/cloudbackup` | The binary (server + CLI client) |
| `/etc/cloudbackup/config.yaml` | Sample server configuration (preserved on upgrade — marked as a config file) |
| `/usr/share/cloudbackup/webstatic/` | Web UI, this documentation and the Swagger API reference |
| `/usr/lib/systemd/system/cloudbackup.service` | systemd unit |
| `/var/lib/cloudbackup/` | Data directory (SQLite databases, reports), mode `0750` |

It also creates a dedicated system user and group named `cloudbackup`, and
**enables — but does not start** — the `cloudbackup` systemd service. The service
stays stopped on purpose: the shipped config contains a placeholder password hash
that you must replace before the first start. Chapter
[2 — Getting started](02-getting-started.md) walks through that.

When you are ready:

```bash
sudo systemctl start cloudbackup
sudo systemctl status cloudbackup
journalctl -u cloudbackup -f        # follow the (JSON-formatted) logs
```

### Note on the hardened service unit

The shipped unit runs the daemon as the unprivileged `cloudbackup` user with
systemd hardening (`ProtectSystem=full`, `ProtectHome=true`,
`ReadWritePaths=/var/lib/cloudbackup`). Two practical consequences:

1. **The daemon can only back up files the `cloudbackup` user can read.** System
   paths are readable, but anything with restrictive permissions is not.
2. **`/home` is invisible to the service** because of `ProtectHome=true`.

If you need to back up home directories or root-only files, override the unit
rather than editing the packaged file:

```bash
sudo systemctl edit cloudbackup
```

and, for example, relax the home protection and/or change the user:

```ini
[Service]
ProtectHome=read-only
# or, to run as root (weigh this against your threat model):
#User=root
#Group=root
```

Then `sudo systemctl daemon-reload && sudo systemctl restart cloudbackup`.

### Upgrading and uninstalling

```bash
# Upgrade: install the newer package the same way; your edited
# /etc/cloudbackup/config.yaml and /var/lib/cloudbackup are preserved.
sudo apt install ./cloudbackup_<newver>_amd64.deb

# Uninstall
sudo apt remove cloudbackup       # dnf remove cloudbackup
```

---

## 1.2 Windows — MSI installer

Download the `.msi` matching your architecture (`amd64` for Intel/AMD,
`arm64` for Windows-on-ARM) and either double-click it or install silently:

```powershell
msiexec /i cloudbackup_0.0.2_amd64.msi /qn /l*v install.log
```

The installer:

* installs `cloudbackup.exe` and `webstatic\` to `C:\Program Files\cloudbackup\`,
* places a sample config at `C:\ProgramData\cloudbackup\config.yaml` and creates
  `C:\ProgramData\cloudbackup\data\` — **both are preserved across upgrades and
  uninstall**, so an edited config and backup metadata are never clobbered,
* registers a Windows **service named `cloudbackup`** (runs as `LocalSystem`,
  start type *Manual* — installed but not auto-started, matching the Linux packages),
* adds an *Add/Remove Programs* entry.

After editing the config (see [chapter 2](02-getting-started.md)):

```powershell
notepad C:\ProgramData\cloudbackup\config.yaml
Start-Service cloudbackup
Get-Service cloudbackup
```

When running as a service, log output goes to the **Windows Event Log**
(Event Viewer → Windows Logs → Application).

Uninstall via *Add/Remove Programs* or:

```powershell
msiexec /x cloudbackup_0.0.2_amd64.msi /qn
```

Config and data under `C:\ProgramData\cloudbackup\` are intentionally retained.

---

## 1.3 Windows — portable zip

The zip contains a `cloudbackup\` folder with `cloudbackup.exe`, the `webstatic\`
assets and a sample config. Use it when you don't want (or aren't allowed) to
install an MSI:

```powershell
Expand-Archive cloudbackup_0.0.2_windows_amd64.zip -DestinationPath C:\Tools
cd C:\Tools\cloudbackup
.\cloudbackup.exe server version
```

Run the server in a console (see chapter 2 for the config file):

```powershell
.\cloudbackup.exe server start --configfile .\config.yaml
```

There is no service registration with the portable zip; the process runs in the
foreground until stopped.

---

## 1.4 Verifying an installation

```console
$ cloudbackup server version
Server version: 0.0.2
Build date: ...
OS: linux
Arch: amd64
...
```

`cloudbackup client version` prints the (same) client version. Once a server is
running, `cloudbackup client server-version` fetches the version over the API —
a handy end-to-end connectivity check.

---

Next: [2. Getting started](02-getting-started.md)
