param(
    [Parameter(ValueFromRemainingArguments=$true)]
    [string[]]$ArgsFromCmd
)

$ErrorActionPreference = 'Stop'
# 脚本执行完毕后倒计时等待，便于查看输出（程序调用时自动跳过）
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

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
# 双渠道：官方上游 + 用户 fork。均可作为升级/回滚目标，由用户序号选择。
$OfficialRepo = 'looplj/axonhub'
$ForkRepo     = 'keeyangyy/axonhub'

$IncludeBeta = $false
$IncludeRC   = $false
$VerboseFlag = $false
$Force       = $false
$Yes         = $false

function Show-Usage {
  Write-Host @'
AxonHub Upgrade Script (Windows)

Usage: upgrade.bat [options]

Options:
  -y, --yes        Skip selection, prefer the fork channel automatically
  -f, --force      Force reinstall even if already on the latest version
  -b, --beta       (kept for compatibility)
  -r, --rc         (kept for compatibility)
  -v, --verbose    Print extra debug logs
  -h, --help       Show this help message

Examples:
  upgrade.bat              Check two channels and prompt for a selection
  upgrade.bat -y           Upgrade via fork channel without confirmation
'@
}

foreach($a in $ArgsFromCmd){
  switch -Regex ($a){
    '^(--yes|-y)$'    { $Yes = $true; continue }
    '^(--force|-f)$'  { $Force = $true; continue }
    '^(--beta|-b)$'   { $IncludeBeta = $true; continue }
    '^(--rc|-r)$'     { $IncludeRC = $true; continue }
    '^(--verbose|-v)$'{ $VerboseFlag = $true; continue }
    '^(--help|-h)$'   { Show-Usage; exit 0 }
    default           { Write-Warn "Unknown option: $a" }
  }
}

function Get-Platform {
  $archEnv = $env:PROCESSOR_ARCHITECTURE
  switch ($archEnv.ToLower()){
    'amd64' { return 'windows_amd64' }
    'arm64' { return 'windows_arm64' }
    default { Write-Err "Unsupported architecture: $archEnv"; exit 1 }
  }
}

function Invoke-GHApi([string]$url){
  $headers = @{
    'Accept'='application/vnd.github+json'
    'X-GitHub-Api-Version'='2022-11-28'
    'User-Agent'='axonhub-upgrader'
  }
  if($env:GITHUB_TOKEN){ $headers['Authorization'] = "Bearer $($env:GITHUB_TOKEN)" }
  return Invoke-RestMethod -Method GET -Uri $url -Headers $headers -ErrorAction Stop
}

# 查询指定仓库的最新 release tag；失败返回 $null（由调用方决定去向）
function Get-LatestReleaseTag([string]$repo){
  try {
    $json = Invoke-GHApi "https://api.github.com/repos/$repo/releases/latest"
    if($json -and $json.tag_name){ return $json.tag_name }
    return $null
  } catch {
    Write-Warn "API failed for $repo, falling back to HTML redirect..."
    try {
      $resp = Invoke-WebRequest -Uri "https://github.com/$repo/releases/latest" -Headers @{ 'User-Agent'='axonhub-upgrader' } -MaximumRedirection 0 -ErrorAction Stop
    } catch { $resp = $_.Exception.Response }
    $loc = $null
    if($resp -and $resp.Headers){
      if($resp.Headers.Location){ $loc = [string]$resp.Headers.Location }
      elseif($resp.Headers['Location']){ $loc = $resp.Headers['Location'] }
    }
    if($loc -match '/tag/([^/]+)$'){ return $matches[1] }
    return $null
  }
}

function Get-AssetUrl([string]$repo,[string]$version,[string]$platform){
  Write-Info "Resolving asset for $version ($platform) from $repo ..."
  try {
    $json = Invoke-GHApi "https://api.github.com/repos/$repo/releases/tags/$version"
    $asset = $json.assets | Where-Object { $_.browser_download_url -match $platform -and $_.browser_download_url -like '*.zip' } | Select-Object -First 1
    if($asset){ return $asset.browser_download_url }
  } catch {}
  $clean = $version.TrimStart('v')
  $file = "axonhub_${clean}_${platform}.zip"
  $candidate = "https://github.com/$repo/releases/download/$version/$file"
  try {
    $null = Invoke-WebRequest -Uri $candidate -Method Head -ErrorAction Stop
    return $candidate
  } catch {
    Write-Err "Could not find asset for platform $platform in release $version ($repo)"
    Wait-Exit; exit 1
  }
}

function Get-CurrentVersion([string]$binary){
  if(Test-Path $binary){
    try {
      $output = & $binary version 2>$null
      if($output){
        return ($output -split "`n")[0].Trim()
      }
    } catch {}
  }
  return ''
}

function Compare-Version([string]$a,[string]$b){
  $aNorm = $a.TrimStart('v')
  $bNorm = $b.TrimStart('v')
  $aParts = $aNorm -split '[.\-]'
  $bParts = $bNorm -split '[.\-]'
  $maxLen = [Math]::Max($aParts.Length, $bParts.Length)
  for($i=0; $i -lt $maxLen; $i++){
    $aVal = if($i -lt $aParts.Length){ $aParts[$i] } else { '0' }
    $bVal = if($i -lt $bParts.Length){ $bParts[$i] } else { '0' }
    # 每段解析为「字母前缀 + 数字」，如 beta7 -> (beta, 7)
    $ma = [regex]::Match($aVal, '^([a-zA-Z]*)(\d*)$')
    $mb = [regex]::Match($bVal, '^([a-zA-Z]*)(\d*)$')
    $pa = $ma.Groups[1].Value.ToLower()
    $pb = $mb.Groups[1].Value.ToLower()
    $na = if($ma.Groups[2].Value){ [int]$ma.Groups[2].Value } else { 0 }
    $nb = if($mb.Groups[2].Value){ [int]$mb.Groups[2].Value } else { 0 }
    if($pa -eq $pb){
      if($na -lt $nb){ return -1 }
      if($na -gt $nb){ return 1 }
    } else {
      if($pa -lt $pb){ return -1 }
      if($pa -gt $pb){ return 1 }
    }
  }
  return 0
}

function Ensure-Dirs([string]$path){ if(-not (Test-Path $path)){ New-Item -ItemType Directory -Force -Path $path | Out-Null } }

Write-Info 'Checking for AxonHub updates...'

$BaseDir = Join-Path $env:LOCALAPPDATA 'AxonHub'
$BinaryPath = Join-Path $BaseDir 'axonhub.exe'

if(-not (Test-Path $BinaryPath)){
  Write-Err "AxonHub is not installed at $BinaryPath"
  Write-Info 'Please run install.bat first'
  Wait-Exit; exit 1
}

$CurrentVersion = Get-CurrentVersion $BinaryPath
if(-not $CurrentVersion){
  Write-Warn 'Could not determine current version'
  $CurrentVersion = 'unknown'
} else {
  Write-Info "Current version: $CurrentVersion"
}

# 查两个渠道的最新版本
$OfficialLatest = Get-LatestReleaseTag $OfficialRepo
$ForkLatest     = Get-LatestReleaseTag $ForkRepo

Write-Host ''
if($OfficialLatest){
  Write-Host ("  [官方] {0,-24} 最新: {1}" -f $OfficialRepo, $OfficialLatest)
} else {
  Write-Warn "官方渠道 ($OfficialRepo) 获取失败"
}
if($ForkLatest){
  Write-Host ("  [fork] {0,-24} 最新: {1}" -f $ForkRepo, $ForkLatest)
} else {
  Write-Warn "fork 渠道 ($ForkRepo) 获取失败"
}
Write-Host ''

if(-not $OfficialLatest -and -not $ForkLatest){
  Write-Err '无法获取任何渠道的版本信息，请检查网络或稍后重试'
  Wait-Exit; exit 1
}

# 判断是否有更新（任一渠道比当前版本新即认为有更新）
$hasUpdate = $false
$updateDetail = @()
if($OfficialLatest -and ($CurrentVersion -eq 'unknown' -or (Compare-Version $CurrentVersion $OfficialLatest) -lt 0)){
  $hasUpdate = $true
  $updateDetail += "官方 $OfficialLatest"
}
if($ForkLatest -and ($CurrentVersion -eq 'unknown' -or (Compare-Version $CurrentVersion $ForkLatest) -lt 0)){
  $hasUpdate = $true
  $updateDetail += "fork $ForkLatest"
}

if(-not $hasUpdate -and -not $Force){
  Write-Success "AxonHub is already up to date ($CurrentVersion)"
  Wait-Exit; exit 0
}

if($Force){
  Write-Warn "Force reinstall requested (current: $CurrentVersion)"
} else {
  Write-Success "发现更新: $($updateDetail -join ', ')"
}

# 构建可选目标（官方 + fork 均列出，允许交叉选择）
$choices = [System.Collections.Generic.List[object]]::new()
if($OfficialLatest){ $choices.Add([pscustomobject]@{ Repo=$OfficialRepo; Version=$OfficialLatest; Channel='官方' }) }
if($ForkLatest){ $choices.Add([pscustomobject]@{ Repo=$ForkRepo; Version=$ForkLatest; Channel='fork' }) }

$targetRepo = $null
$targetVersion = $null

if($Yes){
  # -y：优先 fork，其次官方
  $c = $choices | Where-Object { $_.Channel -eq 'fork' } | Select-Object -First 1
  if(-not $c){ $c = $choices | Select-Object -First 1 }
  $targetRepo = $c.Repo
  $targetVersion = $c.Version
  Write-Info "自动选择 [$($c.Channel)] $($c.Version)"
} else {
  Write-Host '  可选升级目标：'
  $i = 1
  foreach($c in $choices){
    Write-Host ("    {0}) {1,-4} {2}" -f $i, $c.Channel, $c.Version)
    $i++
  }
  Write-Host '    0) 取消'
  Write-Host ''
  $sel = Read-Host "请选择 [0-$($choices.Count)]"
  if([string]::IsNullOrWhiteSpace($sel)){ $sel = '0' }
  if(-not [int]::TryParse($sel, [ref]$null) -or [int]$sel -lt 0 -or [int]$sel -gt $choices.Count){
    Write-Err "Invalid selection: $sel"
    Wait-Exit; exit 1
  }
  if([int]$sel -eq 0){
    Write-Info 'Upgrade cancelled'
    Wait-Exit; exit 0
  }
  $c = $choices[[int]$sel - 1]
  $targetRepo = $c.Repo
  $targetVersion = $c.Version
}

if($targetVersion -eq $CurrentVersion){
  Write-Warn "目标版本与当前相同 ($CurrentVersion)，无需升级"
  Wait-Exit; exit 0
}

$Platform = Get-Platform
Write-Info "Detected platform: $Platform"

$AssetUrl = Get-AssetUrl $targetRepo $targetVersion $Platform
$TempDir = New-Item -ItemType Directory -Path (Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())) -Force
$ZipPath = Join-Path $TempDir 'axonhub.zip'

Write-Info "Downloading: $AssetUrl"
Invoke-WebRequest -Uri $AssetUrl -OutFile $ZipPath -UseBasicParsing

Write-Info 'Extracting archive...'
Expand-Archive -Path $ZipPath -DestinationPath $TempDir -Force

$NewBinary = Get-ChildItem -Path $TempDir -Recurse -Filter 'axonhub.exe' -File | Select-Object -First 1 | ForEach-Object { $_.FullName }
if(-not $NewBinary){ Write-Err 'axonhub.exe not found in archive'; exit 1 }

Write-Info 'Stopping AxonHub...'
$env:AXONHUB_NO_WAIT = '1'
$stopScript = Join-Path $ScriptDir 'stop.ps1'
& $stopScript --force

Write-Info 'Installing new binary...'
Copy-Item -Path $NewBinary -Destination $BinaryPath -Force

Write-Success "AxonHub upgraded to $targetVersion"

Write-Info 'Starting AxonHub...'
$startScript = Join-Path $ScriptDir 'start.ps1'
& $startScript
Remove-Item Env:AXONHUB_NO_WAIT -ErrorAction SilentlyContinue

Write-Success 'Upgrade completed!'

Wait-Exit