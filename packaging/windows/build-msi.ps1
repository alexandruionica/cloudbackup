<#
.SYNOPSIS
    Build the CloudBackup Windows installer (.msi) with the WiX Toolset v5.

.DESCRIPTION
    The Windows analog of packaging/build-in-container.sh. It builds
    cloudbackup.exe, stages the same webstatic tree the Linux packages ship
    plus the sample config, then runs `wix build` against
    packaging/windows/cloudbackup.wxs to produce
    dist/packages/cloudbackup_<version>_amd64.msi.

    Must be run on Windows (GitHub Actions windows-latest, or the Vagrant
    windows2025 VM). Requires:
      * Go (version per go.mod), with CGO enabled (TDM-GCC / mingw) for
        github.com/mattn/go-sqlite3.
      * Git (generate_version.ps1 stamps the short commit id).
      * WiX Toolset v5 on PATH: dotnet tool install --global wix --version 5.*
#>

$ErrorActionPreference = "Stop"

# Repo root is two levels up from this script (packaging/windows/).
$root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $root

if (-not (Get-Command wix -ErrorAction SilentlyContinue)) {
    Write-Error @"
wix (WiX Toolset v5) was not found on PATH.
Install it with:
    dotnet tool install --global wix --version 5.*
and ensure the dotnet global tools dir is on PATH, then re-run.
"@
    exit 1
}

$version = (Get-Content misc/version.txt -Raw).Trim()
if ([string]::IsNullOrWhiteSpace($version)) {
    Write-Error "misc/version.txt is empty"
    exit 1
}
Write-Host "############ Building CloudBackup MSI version $version ############"

# Stamp misc/version.go (AWS/GCP/Azure SDK versions + short commit id).
Write-Host "############ Generating version stamp ############"
pwsh ./generate_version.ps1

# Build the binary. CGO is required by github.com/mattn/go-sqlite3.
$env:CGO_ENABLED = "1"
$stage = "dist/win/cloudbackup"
Write-Host "############ Building cloudbackup.exe ############"
go version
go build -v -mod=vendor -o "$stage/cloudbackup.exe" .

# Stage webstatic exactly like the Linux build (packaging/build-in-container.sh
# and the windows-zip job in .github/workflows/release.yml): docs, docs_api,
# and a trimmed ui.
Write-Host "############ Staging webstatic + sample config ############"
$web = "$stage/webstatic"
if (Test-Path $web) { Remove-Item -Recurse -Force $web }
# Create only ui/ (NOT ui/js): Copy-Item -Recurse into an existing destination
# folder nests the source inside it (ui/js/js/...). Letting Copy-Item create
# ui/js itself puts the files directly under ui/js where /ui/js/* is served.
New-Item -ItemType Directory -Force -Path "$web/ui" | Out-Null
Copy-Item -Recurse webstatic/docs "$web/docs"
Copy-Item -Recurse webstatic/docs_api "$web/docs_api"
Copy-Item webstatic/ui/index.html, webstatic/ui/styles.css "$web/ui/"
Copy-Item -Recurse webstatic/ui/js "$web/ui/js"
# Windows-specific sample config (Windows paths + service commands), not the
# Linux packaging/files/config.yaml.sample.
Copy-Item packaging/windows/config.yaml.sample "$stage/config.yaml.sample"

# Produce the MSI.
$outDir = "dist/packages"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$msi = "$outDir/cloudbackup_${version}_amd64.msi"
Write-Host "############ Running wix build ############"
wix build packaging/windows/cloudbackup.wxs `
    -arch x64 `
    -d "Version=$version" `
    -d "StageDir=$((Resolve-Path $stage).Path)" `
    -o $msi

Write-Host "############ Produced installer ############"
Get-Item $msi | Format-List FullName, Length
