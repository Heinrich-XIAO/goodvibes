# GoodVibes daemon installer.
#   $env:GOODVIBES_API_KEY = "YOUR_API_KEY"; irm __GOODVIBES_SERVER__/install.ps1 | iex
$ErrorActionPreference = 'Stop'

$Server = "__GOODVIBES_SERVER__"
$ApiKey = $env:GOODVIBES_API_KEY
if (-not $ApiKey) {
  throw "missing API key. Run: `$env:GOODVIBES_API_KEY = 'YOUR_API_KEY'; irm $Server/install.ps1 | iex"
}

$ReleaseBase = if ($env:GOODVIBES_RELEASE_BASE) { $env:GOODVIBES_RELEASE_BASE } else { 'https://github.com/Heinrich-XIAO/goodvibes/releases/latest/download' }
$HomeDir = if ($env:GOODVIBES_HOME) { $env:GOODVIBES_HOME } else { Join-Path $HOME '.goodvibes' }
$BinDir = Join-Path $HomeDir 'bin'
$Exe = Join-Path $BinDir 'goodvibes.exe'

$Arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
  'X64' { 'amd64' }
  'Arm64' { 'arm64' }
  default { throw "unsupported architecture: $_" }
}

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Write-Host "Downloading goodvibes (windows/$Arch)..."
Invoke-WebRequest -UseBasicParsing -Uri "$ReleaseBase/goodvibes-windows-$Arch.exe" -OutFile "$Exe.new"

# Stop the daemon from an earlier install so the new binary can replace it.
Get-Process -Name goodvibes -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep -Seconds 1
Move-Item -Force "$Exe.new" $Exe

& $Exe login --server $Server --api-key $ApiKey | Out-Null
if ($LASTEXITCODE -ne 0) { throw "goodvibes login failed" }

Start-Process -FilePath $Exe -ArgumentList 'daemon' -WindowStyle Hidden `
  -RedirectStandardOutput (Join-Path $HomeDir 'daemon.log') `
  -RedirectStandardError (Join-Path $HomeDir 'daemon.err.log')

Start-Sleep -Seconds 2
& $Exe status | Out-Null
if ($LASTEXITCODE -ne 0) { throw "daemon did not start. See $HomeDir\daemon.err.log" }
Write-Host "GoodVibes daemon is running and tracking. Log: $HomeDir\daemon.log"
if (-not ($env:Path -split ';' | Where-Object { $_ -eq $BinDir })) {
  Write-Host "To run 'goodvibes' from any terminal, add $BinDir to your PATH."
}
