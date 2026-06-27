# Windows installer (MSI)

This directory builds the native Windows installer for CloudBackup — the Windows
analog of the Linux `.deb`/`.rpm` packaging in `../` (see `../nfpm.yaml`).

| File | Purpose |
|------|---------|
| `cloudbackup.wxs` | WiX Toolset v5 source describing the MSI: files, the `cloudbackup` Windows service, and the Add/Remove Programs entry. |
| `build-msi.ps1` | Build driver: compiles `cloudbackup.exe`, stages `webstatic` + the sample config, then runs `wix build`. |

## What the installer does

- Installs `cloudbackup.exe` and `webstatic\` to `C:\Program Files\cloudbackup\`.
- Installs the sample config to `C:\ProgramData\cloudbackup\config.yaml` and creates
  `C:\ProgramData\cloudbackup\data\`. Both are **preserved across upgrades and uninstall**
  (the MSI equivalent of deb's `config|noreplace`) so an edited config and backup metadata
  are never clobbered.
- Registers a Windows service named `cloudbackup` running as `LocalSystem`, start type
  **Manual** — installed but not auto-started, matching the Linux packages. Native SCM
  integration lives in `cliargs/service_windows.go`.

## Building

Must run **on Windows** — the GitHub Actions `windows-latest` runner or the local Vagrant
`windows2025` VM. Prerequisites (installing them in the Vagrant VM is out of scope):

- **Go** (version per `go.mod`) with **CGO enabled** (e.g. TDM-GCC / mingw-w64) —
  `github.com/mattn/go-sqlite3` requires it.
- **Git** — `generate_version.ps1` stamps the short commit id.
- **WiX Toolset v5**:
  ```powershell
  dotnet tool install --global wix --version 5.*
  ```
  Ensure the dotnet global-tools directory is on `PATH` (so `wix` is found).

Then, from the repo root:

```powershell
make winpackage
# or directly:
pwsh packaging\windows\build-msi.ps1
```

The installer is written to `dist\packages\cloudbackup_<version>_amd64.msi`.

## Install / manage / uninstall

```powershell
# Install (silent), with a verbose log:
msiexec /i dist\packages\cloudbackup_<version>_amd64.msi /qn /l*v install.log

# Edit the config, then start the service:
notepad C:\ProgramData\cloudbackup\config.yaml
Start-Service cloudbackup
Get-Service cloudbackup

# Uninstall (config + data are intentionally retained):
msiexec /x dist\packages\cloudbackup_<version>_amd64.msi /qn
```

The service account stays editable post-install via `services.msc` or
`sc.exe config cloudbackup obj= ...` if you prefer a less-privileged account.

## Logs

A Windows service has no console, so the daemon's stdout is discarded by the SCM. On
Windows the log destination is therefore chosen by whether `--logfile` is supplied:

- **No `--logfile` (the default for the MSI-installed service)** → logs go to the
  **Windows Event Log**, under *Event Viewer → Windows Logs → Application*, source
  `cloudbackup`. View the latest entries from PowerShell:

  ```powershell
  Get-WinEvent -ProviderName cloudbackup -MaxEvents 50 | Format-List TimeCreated, LevelDisplayName, Message
  ```

- **`--logfile <path>` supplied** → logs go to that file instead and the Event Log is
  not used. To switch the installed service to a file, add the flag to its command line:

  ```powershell
  $exe='C:\Program Files\cloudbackup\cloudbackup.exe'
  $cfg='C:\ProgramData\cloudbackup\config.yaml'
  $log='C:\ProgramData\cloudbackup\cloudbackup.log'
  sc.exe config cloudbackup binPath= "`"$exe`" server start --configfile `"$cfg`" --logfile `"$log`""
  Restart-Service cloudbackup
  Get-Content $log -Wait
  ```

Logs are JSON by default (add `--textlog` for plaintext). To debug interactively, stop the
service and run the binary in the foreground from an **elevated** console — when not
launched by the SCM it also writes to stdout, so you see output in the console (in addition
to the Event Log):

```powershell
Stop-Service cloudbackup
& "C:\Program Files\cloudbackup\cloudbackup.exe" server start `
    --configfile "C:\ProgramData\cloudbackup\config.yaml" --textlog --debug
```

