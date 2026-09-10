---
name: packaging
description: Build, debug or extend cloudbackup's native installers — the nfpm-driven Linux .deb/.rpm, the WiX v5 Windows .msi and zip, and the pkg(8) FreeBSD .pkg — plus the manual Release workflow that publishes them. Use when the user asks about packaging, `make packages` / `winpackage` / `freebsdpackage`, nfpm, WiX, `pkg create`, install paths, systemd units or rc.d scripts, pre/post install-remove scripts, or adding a new distro, architecture or OS target.
---

# Packaging — how the three installer families work

Three package families, one deliberate shape. Read this before changing any
install path, adding a target, or debugging a package that installs wrong.

## The constraint that governs everything

`database/database.go` imports `github.com/mattn/go-sqlite3`, so **every build
needs `CGO_ENABLED=1`**. That single fact explains most of the design:

- There is **no cross-compile path** for Linux or FreeBSD. Each package is
  built on a host of its own OS and architecture.
- CI uses native runners where GitHub has them (`ubuntu-24.04-arm`,
  `windows-11-arm`) and a VM where it does not (FreeBSD).
- The one exception is Windows/arm64, which cross-compiles via the clang-based
  llvm-mingw toolchain — ordinary mingw gcc cannot emit Windows/arm64 objects.

If someone proposes swapping in pure-Go `modernc.org/sqlite`, that is the
change that would delete the llvm-mingw dance and make FreeBSD/arm64 free.
It is a real migration: driver name, plus the `SetMaxOpenConns(1)` and WAL
tuning in `database/database.go` would need re-validating.

## Where things live

| Concern | File |
|---|---|
| Linux orchestration (distro × arch matrix, QEMU preflight) | `packaging/build-all.sh` |
| Linux builder image (Go + nfpm, deb or rpm base) | `packaging/Dockerfile.builder` |
| Linux in-container build + nfpm invocation | `packaging/build-in-container.sh` |
| Linux package definition (paths, deps, scripts) | `packaging/nfpm.yaml` |
| Linux lifecycle scripts | `packaging/files/scripts/{pre,post}{install,remove}.sh` |
| systemd unit + sample config | `packaging/files/cloudbackup.service`, `packaging/files/config.yaml.sample` |
| Windows MSI build driver | `packaging/windows/build-msi.ps1` |
| Windows MSI definition (files, service, ARP entry) | `packaging/windows/cloudbackup.wxs` |
| Windows/arm64 cross-build from Unix | `packaging/windows/build-winarm64.sh`, `Dockerfile.winarm-builder` |
| Windows operator docs (install, logs, Event Log) | `packaging/windows/README.md` |
| FreeBSD build + plist/manifest generation | `packaging/freebsd/build-pkg.sh` |
| FreeBSD pinned Go toolchain installer | `packaging/freebsd/install-go.sh` |
| FreeBSD manifest, rc.d script, sample config | `packaging/freebsd/{manifest.template,cloudbackup.rc,config.yaml.sample}` |
| FreeBSD lifecycle scripts | `packaging/freebsd/scripts/{post-install,pre-deinstall,post-deinstall}.sh` |
| Release pipeline | `.github/workflows/release.yml` |

## Common shape across all three

Every package ships the same payload and behaves the same way:

1. The binary, the trimmed `webstatic` tree, and a platform-specific sample config.
2. A service definition that is **installed but never auto-started** — the admin
   must set a bcrypt hash in the config first.
3. Config and data directories that **survive upgrade and removal**. Backup
   metadata must never be destroyed by a package operation.
4. Version comes from `misc/version.txt`, stamped into `misc/version.go` by
   `generate_version.sh` / `generate_version.ps1`. Both shell out to `git` for
   the short commit id, which is why every CI job checks out with
   `fetch-depth: 0`.

| | Linux | Windows | FreeBSD |
|---|---|---|---|
| Binary | `/usr/bin/cloudbackup` | `C:\Program Files\cloudbackup\` | `/usr/local/bin/cloudbackup` |
| Web UI | `/usr/share/cloudbackup/webstatic` | `…\cloudbackup\webstatic` | `/usr/local/share/cloudbackup/webstatic` |
| Config | `/etc/cloudbackup/config.yaml` | `C:\ProgramData\cloudbackup\config.yaml` | `/usr/local/etc/cloudbackup/config.yaml` |
| Data | `/var/lib/cloudbackup` | `C:\ProgramData\cloudbackup\data\` | `/var/db/cloudbackup` |
| Service | systemd unit, enabled not started | SCM service, `Start="demand"` | rc.d script, not enabled |
| Config preserved by | nfpm `config\|noreplace` | `Permanent`+`NeverOverwrite` component | not packaged; seeded by `+POST_INSTALL` |

## Linux — nfpm in Docker

```bash
make packages                          # host arch, full distro matrix
make packages DISTROS="deb12 el9"      # subset
make packages ARCHS="amd64 arm64"      # arm64 on x86_64 runs under QEMU
```

`build-all.sh` iterates `<base_image>|<pkg_family>|<distro_tag>` specs
(debian:12, ubuntu:24.04, ubuntu:26.04, rockylinux:8, rockylinux:9), builds
`Dockerfile.builder` per (distro, arch), then runs `build-in-container.sh`
inside it. One nfpm YAML produces both families; `overrides:` carries the
per-family dependencies.

Things that are easy to get wrong:

- **nfpm does not expand env vars in `contents.src` or `scripts` paths.**
  `build-in-container.sh` renders `nfpm.yaml` through `sed` first. Any new
  placeholder must be added to that sed pipeline or it ships literally.
- All paths in `nfpm.yaml` are absolute `/src/...` (the container mount), so
  the rendered file can live anywhere.
- Cross-arch needs QEMU binfmt registered. `build-all.sh` fails early with the
  fix-it hint: `docker run --privileged --rm tonistiigi/binfmt --install all`.
- `preremove.sh` is called with different arguments by deb (`remove`,
  `upgrade`, …) and rpm (`0` uninstall, `1` upgrade). The `case` handles both —
  keep it that way if you touch it.

## Windows — WiX Toolset v5

```powershell
make winpackage                        # on Windows; -Arch defaults to amd64
pwsh packaging\windows\build-msi.ps1 -Arch arm64
```

`build-msi.ps1` maps `-Arch` to a WiX arch (`amd64`→`x64`, `arm64`→`arm64`),
builds the exe, stages the payload under `dist/win/cloudbackup`, and passes
that path to `wix build` as `-d StageDir=…`. The `.wxs` harvests it with
`<Files Include="$(StageDir)\webstatic\**">`, so adding a webstatic directory
needs no `.wxs` edit. Output: `dist/packages/cloudbackup_<version>_<arch>.msi`.

- The service runs as `LocalSystem` with `Start="demand"` — installed, not
  started, matching Linux. SCM integration is in `cliargs/service_windows.go`.
- Config and data survive via two attributes doing different jobs on the
  `ConfigFile` / `DataFolder` components: `NeverOverwrite="yes"` protects
  against **upgrade**, `Permanent="yes"` against **uninstall**. Both are needed;
  dropping either silently reintroduces data loss on one of the two paths.
- A Windows service has no console, so with no `--logfile` the daemon logs to
  the **Windows Event Log** (source `cloudbackup`), not stdout. See
  `misc/logging_windows.go` and the Logs section of the Windows README.
- `make winbuild-arm64` cross-builds the arm64 **zip** from a Unix host with
  Docker, to prove the code compiles. It does not produce an MSI, and it is not
  runtime verification — that needs the `windows-11-arm` runner or real hardware.

## FreeBSD — base pkg(8)

```sh
gmake freebsdpackage                   # ON FreeBSD only
```

**FreeBSD's `make` is bmake and cannot parse this GNU Makefile — use `gmake`.**
Build locally in the `freebsd14` Vagrant VM, or in CI inside the
`vmactions/freebsd-vm` VM that runs on the Linux runner.

nfpm has **no FreeBSD backend**, so `build-pkg.sh` drives base `pkg(8)` directly:

```
pkg create -m <metadata dir> -p <plist> -r <staged root> -o dist/packages
```

- The metadata dir holds `+MANIFEST` (rendered from `manifest.template`) plus
  `+POST_INSTALL`, `+PRE_DEINSTALL`, `+POST_DEINSTALL`.
- The plist is generated by `find` over the staged root. `@dir` entries are
  emitted **only** for trees we own (`etc/cloudbackup`, `share/cloudbackup`,
  `share/doc/cloudbackup`) and deepest-first. Never `@dir` a shared system
  directory like `/usr/local/bin` — pkg would try to remove it on deinstall.
- `pkg` stamps the **building host's ABI** (`FreeBSD:14:amd64` vs
  `FreeBSD:15:amd64`) into the package and refuses to install across major
  releases. That is why the workflow builds one package per branch. The output
  is renamed to `cloudbackup-<version>_<rev>.freebsd<major>.<arch>.pkg`.
- **Only `config.yaml.sample` is packaged.** `+POST_INSTALL` seeds
  `config.yaml` from it, keeping the live config out of the plist so pkg can
  never clobber it on upgrade. `+PRE_DEINSTALL` reproduces `@sample` semantics
  by hand — it removes the config only when byte-identical to the sample, and
  must run there because the sample is gone by `+POST_DEINSTALL` time.
- The rc.d script supervises via `daemon(8) -R 5` to match
  `Restart=on-failure` / `RestartSec=5s`. `pidfile`/`procname` track the
  **supervisor**, not the child: point them at the child and
  `service cloudbackup stop` kills it only for the supervisor to restart it.
- Unlike Linux, the package does **not** touch `/etc/rc.conf`. `+POST_INSTALL`
  prints `sysrc cloudbackup_enable=YES` instead. Deliberate — editing rc.conf
  from a package is bad form on FreeBSD.
- `install-go.sh` pins the Go toolchain rather than using the `go` port, whose
  version floats. Keep it in step with `GO_VERSION` at the top of `Vagrantfile`.

## The release pipeline

`.github/workflows/release.yml` is **manual-only** (`workflow_dispatch`) and
**does not run tests** — tests stay a local concern. It reads
`misc/version.txt`, fails early if tag `v<version>` already exists on origin,
fans out to `linux-packages` (amd64 + arm64), `windows-zip`, `windows-msi`
(amd64 + arm64 each) and `freebsd-package` (14.5 + 15.1), then publishes every
artifact on a GitHub Release.

Bump `misc/version.txt` and commit **before** dispatching. See the
`release-prep` skill for the pre-release checklist.

## Gotchas that have actually bitten

- **The webstatic staging block is duplicated in four places** —
  `packaging/build-in-container.sh`, `packaging/windows/build-msi.ps1`, the
  `windows-zip` job's "Assemble release tree" step in `release.yml`, and
  `packaging/freebsd/build-pkg.sh`. Add a directory under `webstatic/` and you
  must update **all four**, or that platform silently ships an incomplete UI.
- **PowerShell `Copy-Item -Recurse` into an existing destination nests the
  source inside it** (`ui/js/js/...`). Create `ui/` but let `Copy-Item` create
  `ui/js` itself. Both Windows staging sites carry this comment.
- **Never repeat a `${PLACEHOLDER}` in a comment** inside `nfpm.yaml` or
  `manifest.template` — substitution is a plain `sed` and rewrites the comment
  too.
- The service user and data directory **deliberately survive package removal**
  on every platform. If a task asks to "clean up on uninstall", confirm it
  really means backup metadata, and say no by default.

## Adding a new target

- **New Linux distro**: add a `<image>|<family>|<tag>` spec to `TARGETS` in
  `build-all.sh`. Verify the base image has a Go-compatible glibc and that the
  family's `overrides:` deps in `nfpm.yaml` are right for it.
- **New architecture**: prefer a native runner. Emulated builds work but are
  slow enough to be worth avoiding in CI.
- **New OS**: the FreeBSD layer is the template — a staged root, a native
  packaging tool, an OS-idiomatic service definition, and a lifecycle-script
  set that preserves config and data. Expect to add a CI job that builds on
  that OS rather than cross-compiling, because of cgo.
