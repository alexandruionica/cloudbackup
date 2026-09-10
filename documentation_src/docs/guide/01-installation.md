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
| FreeBSD package | FreeBSD 14 and 15 — `amd64` | `cloudbackup-0.0.2_1.freebsd14.amd64.pkg` |
| macOS package | macOS 13+ — Apple Silicon and Intel | `cloudbackup_0.0.2_macos_arm64.pkg` |

For other Linux distributions, or any platform without a pre-built package,
build from source — see the
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

It also creates a dedicated system user and group named `cloudbackup` (unused by
default — see [the note below](#note-on-the-service-account)), and **enables —
but does not start** — the `cloudbackup` systemd service. The service
stays stopped on purpose: the shipped config contains a placeholder password hash
that you must replace before the first start. Chapter
[2 — Getting started](02-getting-started.md) walks through that.

When you are ready:

```bash
sudo systemctl start cloudbackup
sudo systemctl status cloudbackup
journalctl -u cloudbackup -f        # follow the (JSON-formatted) logs
```

### Note on the service account

The shipped unit runs the daemon as `root`, with no systemd sandboxing. That is
deliberate: a backup daemon that cannot read `/home`, `/etc` or any other
root-only path backs up very little, and the failure mode (files silently
missing from a backup) is worse than the alternative. The daemon backs up files
*as the OS user it runs as*, so running as `root` is what lets it read
everything you point it at.

If that is more privilege than your threat model allows, harden it yourself —
override the unit rather than editing the packaged file, so your changes survive
package upgrades:

```bash
sudo systemctl edit cloudbackup
```

The package already creates an unprivileged system user and group named
`cloudbackup` for exactly this purpose:

```ini
[Service]
User=cloudbackup
Group=cloudbackup
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/cloudbackup
```

Then hand the state directory to that user and restart:

```bash
sudo chown -R cloudbackup:cloudbackup /var/lib/cloudbackup
sudo systemctl daemon-reload && sudo systemctl restart cloudbackup
```

Two consequences to plan for before you do: the daemon can then only back up
files the `cloudbackup` user can read, and `ProtectHome=true` makes `/home`
invisible to the service entirely (use `ProtectHome=read-only` if you need to
back up home directories). Drop or relax whichever directives get in the way —
each one is independent.

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

## 1.4 FreeBSD (`.pkg`)

Packages are built per FreeBSD major release, because `pkg` refuses to install a
package built for a different branch. Download the one matching your system
(`freebsd14` or `freebsd15`):

```sh
sudo pkg add ./cloudbackup-0.0.2_1.freebsd14.amd64.pkg
```

The package installs:

| Path | Purpose |
|------|---------|
| `/usr/local/bin/cloudbackup` | The binary (server + CLI client) |
| `/usr/local/etc/cloudbackup/config.yaml` | Server configuration, seeded from the shipped sample on first install |
| `/usr/local/share/cloudbackup/webstatic/` | Web UI, this documentation and the Swagger API reference |
| `/usr/local/etc/rc.d/cloudbackup` | rc.d service script |
| `/var/db/cloudbackup/` | Data directory (SQLite databases, reports), mode `0750` |

Unlike the Linux packages it does **not** edit `/etc/rc.conf` — a package
writing to it is not the FreeBSD way. Enable and start the service yourself once
you have replaced the placeholder password hash, as chapter
[2. Getting started](02-getting-started.md) describes:

```sh
sudo sysrc cloudbackup_enable=YES
sudo service cloudbackup start
sudo service cloudbackup status
```

The daemon runs as `root` for the same reason it does on Linux (see
[the note above](#note-on-the-service-account)); set `cloudbackup_user` in
`/etc/rc.conf` to run it as someone else. Logs go to syslog tagged
`cloudbackup`, or to a file if you set `cloudbackup_logfile`.

### Upgrading and uninstalling on FreeBSD

```sh
# Upgrade
sudo pkg add -f ./cloudbackup-<newver>_1.freebsd14.amd64.pkg

# Uninstall. Your config and /var/db/cloudbackup are kept.
sudo pkg delete cloudbackup
```

---

## 1.5 macOS (`.pkg`)

Download the `.pkg` matching your Mac — `arm64` for Apple Silicon, `amd64` for
Intel.

These packages are currently **unsigned**, so Gatekeeper refuses to open them by
double-click. Installing from a terminal is not subject to that check:

```sh
sudo installer -pkg cloudbackup_0.0.2_macos_arm64.pkg -target /
```

To use the graphical installer instead, clear the quarantine flag first:

```sh
xattr -d com.apple.quarantine cloudbackup_0.0.2_macos_arm64.pkg
```

The package installs:

| Path | Purpose |
|------|---------|
| `/usr/local/bin/cloudbackup` | The binary (server + CLI client) |
| `/usr/local/etc/cloudbackup/config.yaml` | Server configuration, seeded from the shipped sample on first install |
| `/usr/local/share/cloudbackup/webstatic/` | Web UI, this documentation and the Swagger API reference |
| `/Library/LaunchDaemons/eu.ionica.cloudbackup.plist` | launchd service definition, installed disabled |
| `/usr/local/var/cloudbackup/` | Data directory (SQLite databases, reports), mode `0750` |

The launchd job ships **disabled** on purpose: a plist in
`/Library/LaunchDaemons` is otherwise loaded at the next boot, which with the
placeholder password hash still in place would leave the daemon failing and
being restarted on a loop. Enable it once the config is ready:

```sh
sudo launchctl enable system/eu.ionica.cloudbackup
sudo launchctl bootstrap system /Library/LaunchDaemons/eu.ionica.cloudbackup.plist
sudo launchctl print system/eu.ionica.cloudbackup
```

The daemon runs as `root`, for the reason given in
[the note above](#note-on-the-service-account); add a `UserName` key to the
plist to run it as someone else. Logs go to
`/usr/local/var/log/cloudbackup.log`. After a config change, restart with:

```sh
sudo launchctl kickstart -k system/eu.ionica.cloudbackup
```

### Upgrading and uninstalling on macOS

Install the newer `.pkg` the same way to upgrade; your config and data directory
are preserved. A macOS package carries no uninstaller, so removal ships as a
script:

```sh
sudo /usr/local/share/cloudbackup/uninstall.sh            # keeps config + data
sudo /usr/local/share/cloudbackup/uninstall.sh --purge    # removes those too
```

---

## 1.6 Verifying an installation

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
