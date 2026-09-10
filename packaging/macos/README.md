# macOS installer (.pkg)

This directory builds the native macOS installer — the macOS analog of the Linux
`.deb`/`.rpm` packaging in `../` and the FreeBSD package in `../freebsd/`.

| File | Purpose |
|------|---------|
| `build-pkg.sh` | Build driver: compiles the binary, stages the payload, runs `pkgbuild`, optionally signs and notarizes. |
| `cloudbackup.plist` | LaunchDaemon definition installed to `/Library/LaunchDaemons/`. |
| `config.yaml.sample` | Sample config with macOS paths and `launchctl` commands. |
| `scripts/preinstall.sh` | Packaged as `preinstall`; stops a running daemon before its binary is replaced. |
| `scripts/postinstall.sh` | Packaged as `postinstall`; creates the data dir, seeds `config.yaml`, prints next steps. |
| `uninstall.sh` | Shipped in the payload. A macOS `.pkg` has no uninstall phase, so removal is a script. |

## What the installer does

- Installs `cloudbackup` to `/usr/local/bin/` and `webstatic/` to
  `/usr/local/share/cloudbackup/`.
- Seeds `/usr/local/etc/cloudbackup/config.yaml` from the shipped `.sample` on
  first install only, and creates `/usr/local/var/cloudbackup/` (mode `0750`).
  Both are **preserved across upgrades and uninstall**, matching every other
  platform — backup metadata must survive a package operation.
- Installs a LaunchDaemon at `/Library/LaunchDaemons/eu.ionica.cloudbackup.plist`
  with `Disabled` set to true. A plist in that directory is otherwise loaded at
  the next boot; with the placeholder password hash still in the config the
  daemon would fail and be restarted on a throttled loop. Shipping it disabled
  keeps "installed but not started" consistent with Linux, Windows and FreeBSD.

There is **no system user**. macOS has no `useradd`, creating one needs `dscl`
plus a free-UID search, and the daemon runs as root on every platform anyway.
Add a `UserName` key to the plist to drop privileges.

## Building

Must run **on macOS** — `pkgbuild` is an Xcode command line tool, and
`go-sqlite3` needs cgo so there is no cross-OS build path.

```sh
make macospackage                     # host architecture
make macospackage PKG_ARCH=amd64      # cross-build the Intel slice on Apple Silicon
```

Cross-building between `arm64` and `amd64` works because Xcode ships both SDKs;
the script points `CC`/`CXX` at `clang -arch <slice>`. Binaries are built with
`MACOSX_DEPLOYMENT_TARGET=13.0` unless you override it.

Output: `dist/packages/cloudbackup_<version>_macos_<arch>.pkg`.

## Signing and notarization

Both are **off by default**, driven entirely by environment variables, so the
build works with no Apple Developer account. The release workflow maps repo
secrets onto these, meaning signing can be switched on by adding secrets
without editing the workflow.

| Variable | Effect |
|----------|--------|
| `MACOS_SIGN_IDENTITY` | Set to a `Developer ID Installer: …` identity present in the keychain to sign with `productsign`. |
| `MACOS_NOTARY_KEY_PATH` | Path to an App Store Connect API key (`.p8`). Requires `MACOS_SIGN_IDENTITY` too. |
| `MACOS_NOTARY_KEY_ID` | The key's ID. |
| `MACOS_NOTARY_KEY_ISSUER` | The issuer UUID. |

With all four set the script signs, submits to `xcrun notarytool submit --wait`,
then staples and validates the ticket.

The corresponding repo secrets are `MACOS_SIGN_IDENTITY`, `MACOS_CERT_P12`
(base64 of the `.p12`), `MACOS_CERT_PASSWORD`, `MACOS_NOTARY_KEY_P8` (base64 of
the `.p8`), `MACOS_NOTARY_KEY_ID` and `MACOS_NOTARY_KEY_ISSUER`.

## Installing an unsigned package

Gatekeeper blocks double-clicking an unsigned `.pkg`. Installing from a terminal
is not subject to that check:

```sh
sudo installer -pkg dist/packages/cloudbackup_<version>_macos_arm64.pkg -target /
```

To use the graphical installer, clear the quarantine flag first:

```sh
xattr -d com.apple.quarantine cloudbackup_<version>_macos_arm64.pkg
```

## Manage and uninstall

```sh
# Enable and start (after editing the config):
sudo launchctl enable system/eu.ionica.cloudbackup
sudo launchctl bootstrap system /Library/LaunchDaemons/eu.ionica.cloudbackup.plist

# Status, restart, stop:
sudo launchctl print system/eu.ionica.cloudbackup
sudo launchctl kickstart -k system/eu.ionica.cloudbackup
sudo launchctl bootout system/eu.ionica.cloudbackup

# Uninstall (config and data kept; --purge removes those too):
sudo /usr/local/share/cloudbackup/uninstall.sh
```

## Logs

launchd gives a daemon no console, so the plist sends stdout and stderr to
`/usr/local/var/log/cloudbackup.log`. The daemon's own `--logfile` flag is
deliberately unused, keeping the plist the single place to redirect logs.
Logs are JSON by default; add `--textlog` to the plist's `ProgramArguments`
for plaintext.

```sh
tail -f /usr/local/var/log/cloudbackup.log
```
