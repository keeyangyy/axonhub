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

function Show-Usage {
  Write-Host @" 
Usage: restart.bat [--force]

This script restarts AxonHub by stopping and starting it.
Options:
  --force     Force kill before restart
  --help, -h  Show this help message
"@
}

$Force = $false
foreach($a in $ArgsFromCmd){
  switch -Regex ($a){
    '^(--force)$' { $Force = $true; continue }
    '^(--help|-h)$' { Show-Usage; exit 0 }
    default { Write-Warn "Unknown option: $a" }
  }
}

Write-Info 'Restarting AxonHub...'

# Stop AxonHub（子脚本以 NO_WAIT 模式调用，避免中途等待按键）
Write-Info 'Stopping AxonHub...'
$env:AXONHUB_NO_WAIT = '1'
$stopScript = Join-Path $ScriptDir 'stop.ps1'
if($Force){
  & $stopScript --force
} else {
  & $stopScript
}

# Brief pause to ensure clean shutdown
Start-Sleep -Seconds 5

# Start AxonHub
Write-Info 'Starting AxonHub...'
$startScript = Join-Path $ScriptDir 'start.ps1'
& $startScript
Remove-Item Env:AXONHUB_NO_WAIT -ErrorAction SilentlyContinue

Write-Success 'AxonHub has been restarted'

Wait-Exit
