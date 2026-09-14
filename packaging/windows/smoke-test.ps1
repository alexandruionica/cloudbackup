<#
Install smoke test for the Windows MSI, run on the Windows host that built it (the release workflow's
runner, or the Vagrant box). Installs silently, checks what the installer promises (binary, service,
config, data dir, web assets), starts the service, talks to it with the CLI client, stops it,
uninstalls and checks what must survive. Must run elevated.

    pwsh packaging/windows/smoke-test.ps1 -Msi dist/packages/cloudbackup_0.0.3_amd64.msi
#>
param([Parameter(Mandatory = $true)][string]$Msi)
$ErrorActionPreference = "Stop"

$Hash = '$2a$05$Ug1eUCXbSYUvfnI6YokjReljCe2fZLYYhO4IQLuiu0/mnpBbsN2M.'
$Password = 'HV}H/y?<9$]Z5N4N'
$Addr = "http://127.0.0.1:8080"
$Exe = "C:\Program Files\cloudbackup\cloudbackup.exe"
$Config = "C:\ProgramData\cloudbackup\config.yaml"
$DataDir = "C:\ProgramData\cloudbackup\data"
$MsiPath = (Resolve-Path $Msi).Path

function Fail($msg) { Write-Error "SMOKE FAIL: $msg"; exit 1 }
function Step($msg) { Write-Host ""; Write-Host "---- $msg" }
function Client { param([string[]]$CliArgs) & $Exe client @CliArgs -a $Addr -u admin -p $Password }

Step "install $MsiPath"
$p = Start-Process msiexec.exe -ArgumentList "/i `"$MsiPath`" /qn /norestart /l*v $env:TEMP\cloudbackup-install.log" -Wait -PassThru
if ($p.ExitCode -ne 0) { Get-Content "$env:TEMP\cloudbackup-install.log" -Tail 40; Fail "msiexec /i exited with $($p.ExitCode)" }

Step "installed files and service"
if (-not (Test-Path $Exe)) { Fail "$Exe missing" }
if (-not ((& $Exe server version) -join "`n" | Select-String '^Server version:')) { Fail "server version does not run" }
if (-not (Test-Path $Config)) { Fail "$Config missing" }
if (-not (Test-Path $DataDir)) { Fail "$DataDir missing" }
if (-not (Test-Path "C:\Program Files\cloudbackup\webstatic\ui\index.html")) { Fail "web UI assets missing" }
$svc = Get-Service cloudbackup -ErrorAction SilentlyContinue
if (-not $svc) { Fail "service 'cloudbackup' not registered" }
if ($svc.StartType -ne "Manual") { Fail "service start type is $($svc.StartType), expected Manual" }
if ($svc.Status -ne "Stopped") { Fail "service is $($svc.Status) right after install, expected Stopped" }

Step "configure and validate as the installer notes instruct"
if (-not (Select-String -Path $Config -Pattern REPLACE_WITH_BCRYPT_HASH -Quiet)) { Fail "config has no placeholder hash" }
(Get-Content $Config) -replace 'REPLACE_WITH_BCRYPT_HASH', $Hash | Set-Content $Config
& $Exe server config validate -c $Config
if ($LASTEXITCODE -ne 0) { Fail "config validation failed" }

Step "start the service"
Start-Service cloudbackup
$ok = $false
for ($i = 0; $i -lt 100 -and -not $ok; $i++) {
    $out = & $Exe client server-version -a $Addr -u admin -p $Password 2>$null
    if ($LASTEXITCODE -eq 0) { $ok = $true } else { Start-Sleep -Milliseconds 200 }
}
if (-not $ok) { Get-Service cloudbackup; Fail "daemon did not answer within 20s" }
if ((Get-Service cloudbackup).Status -ne "Running") { Fail "service not reported Running" }
if (-not ((Client @("backup", "list", "--json")) -join "`n" | Select-String '"result"')) { Fail "backup list over the API failed" }
& $Exe client backup list -a $Addr -u admin -p wrong 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) { Fail "the API accepted a wrong password" }
Stop-Service cloudbackup
if ((Get-Service cloudbackup).Status -ne "Stopped") { Fail "service did not stop" }

Step "uninstall"
$p = Start-Process msiexec.exe -ArgumentList "/x `"$MsiPath`" /qn /norestart /l*v $env:TEMP\cloudbackup-uninstall.log" -Wait -PassThru
if ($p.ExitCode -ne 0) { Get-Content "$env:TEMP\cloudbackup-uninstall.log" -Tail 40; Fail "msiexec /x exited with $($p.ExitCode)" }
if (Test-Path $Exe) { Fail "binary still present after uninstall" }
if (Get-Service cloudbackup -ErrorAction SilentlyContinue) { Fail "service still registered after uninstall" }
if (-not (Test-Path $Config)) { Fail "edited config was discarded on uninstall" }
if (-not (Test-Path $DataDir)) { Fail "$DataDir was deleted on uninstall" }

Write-Host ""
Write-Host "SMOKE OK: $MsiPath"
