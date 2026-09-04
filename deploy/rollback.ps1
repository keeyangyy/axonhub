param(
    [Parameter(ValueFromRemainingArguments=$true)]
    [string[]]$ArgsFromCmd
)

$ErrorActionPreference = 'Stop'
function Wait-Exit {
  if($env:AXONHUB_NO_WAIT -eq '1'){ return }
  if([Console]::IsInputRedirected){ return }
  for($i = 10; $i -ge 1; $i--){
    Write-Host "`r窗口将在 $i 秒后自动关闭，按任意键立即关闭...  " -NoNewline
    if([Console]::KeyAvailable){ [void][Console]::ReadKey($true); break }
    Start-Sleep -Seconds 1
  }
  Write-Host ''
}
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Write-Info([string]$m){ Write-Host "[INFO] $m" -ForegroundColor Cyan }
function Write-Success([string]$m){ Write-Host "[SUCCESS] $m" -ForegroundColor Green }
function Write-Warn([string]$m){ Write-Host "[WARNING] $m" -ForegroundColor Yellow }
function Write-Err([string]$m){ Write-Host "[ERROR] $m" -ForegroundColor Red }

function Invoke-GHApi([string]$url){
  $headers = @{
    'Accept'            = 'application/vnd.github+json'
    'X-GitHub-Api-Version' = '2022-11-28'
    'User-Agent'        = 'axonhub-rollback'
  }
  if($env:GITHUB_TOKEN){ $headers['Authorization'] = "Bearer $($env:GITHUB_TOKEN)" }
  return Invoke-RestMethod -Method GET -Uri $url -Headers $headers -ErrorAction Stop
}

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$Repo = 'keeyangyy/axonhub'
$Api  = "https://api.github.com/repos/$Repo"
$BaseDir = Join-Path $env:LOCALAPPDATA 'AxonHub'
$BinaryPath = Join-Path $BaseDir 'axonhub.exe'

if(-not (Test-Path $BinaryPath)){
  Write-Err "AxonHub is not installed at $BinaryPath"
  Write-Info 'Please run install.bat first'
  Wait-Exit; exit 1
}

$ShowCount = 5

Write-Info "Fetching releases from $Repo ..."
try {
  $releases = Invoke-GHApi "$Api/releases?per_page=$ShowCount"
} catch {
  Write-Err "Failed to fetch releases: $($_.Exception.Message)"
  Wait-Exit; exit 1
}

if(-not $releases -or $releases.Count -eq 0){
  Write-Err 'No releases found'
  Wait-Exit; exit 1
}

Write-Host ''
Write-Host ("  {0}  {1,-22}  {2}" -f '#', 'Version', 'Published') -ForegroundColor Gray
Write-Host ('  ' + ('-' * 50)) -ForegroundColor Gray

for($i = 0; $i -lt $releases.Count; $i++){
  $r = $releases[$i]
  $ver = $r.tag_name
  $date = ([datetime]$r.published_at).ToString('yyyy-MM-dd HH:mm')
  $count = $r.assets.Count
  $color = if($i -eq 0){ 'Green' } else { 'White' }
  $tag = if($i -eq 0){ ' (latest)' } else { '' }
  Write-Host ("  {0}  {1,-22}  {2}  ({3} assets){4}" -f ($i+1), $ver, $date, $count, $tag) -ForegroundColor $color
}

Write-Host ''
$selection = Read-Host "Select version [1-$($releases.Count)] (Enter = 1)"

if([string]::IsNullOrWhiteSpace($selection)){ $selection = 1 }
if(-not [int]::TryParse($selection, [ref]$null) -or [int]$selection -lt 1 -or [int]$selection -gt $releases.Count){
  Write-Err "Invalid selection: $selection"
  Wait-Exit; exit 1
}

$chosen = $releases[[int]$selection - 1]
$targetVer = $chosen.tag_name
Write-Host ''

$currentVersion = 'unknown'
try {
  $output = & $BinaryPath version 2>$null
  if($output -match 'v[\d.]'){ $currentVersion = $output.Trim().Split("`n")[0].Trim() }
} catch {}

if($currentVersion -eq $targetVer){
  Write-Warn "Already on $targetVer — nothing to do"
  Wait-Exit; exit 0
}

Write-Host "  Current version : $currentVersion" -ForegroundColor Gray
Write-Host "  Target version  : $targetVer" -ForegroundColor Yellow
Write-Host ''

$confirm = Read-Host "Rollback to $targetVer ? [y/N]"
if($confirm -notmatch '^[Yy]$'){
  Write-Info 'Rollback cancelled'
  Wait-Exit; exit 0
}

$platform = 'windows_amd64'
$zipName  = "axonhub_${targetVer}_${platform}.zip"
$assetUrl = "https://github.com/$Repo/releases/download/$targetVer/$zipName"

Write-Info "Downloading: $assetUrl"
$tempDir = New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())) -Force
$zipPath = Join-Path $tempDir 'axonhub.zip'

try {
  Invoke-WebRequest -Uri $assetUrl -OutFile $zipPath -UseBasicParsing -TimeoutSec 300
} catch {
  Write-Err "Download failed: $($_.Exception.Message)"
  Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
  Wait-Exit; exit 1
}

Write-Info 'Extracting...'
Expand-Archive -Path $zipPath -DestinationPath $tempDir -Force
$newBinary = Get-ChildItem -Path $tempDir -Recurse -Filter 'axonhub.exe' -File | Select-Object -First 1 | ForEach-Object { $_.FullName }
if(-not $newBinary){
  Write-Err 'axonhub.exe not found in archive'
  Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
  Wait-Exit; exit 1
}

Write-Info 'Stopping AxonHub...'
$env:AXONHUB_NO_WAIT = '1'
$stopScript = Join-Path $ScriptDir 'stop.ps1'
& $stopScript --force

Write-Info 'Installing rollback binary...'
Copy-Item -Path $newBinary -Destination $BinaryPath -Force

Write-Info 'Starting AxonHub...'
$startScript = Join-Path $ScriptDir 'start.ps1'
& $startScript
Remove-Item Env:AXONHUB_NO_WAIT -ErrorAction SilentlyContinue

Write-Success "Rolled back to $targetVer"
Wait-Exit
