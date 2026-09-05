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
    Write-Host "`r窗口将在 $i 秒后自动关闭，按任意键保留窗口...  " -NoNewline
    if([Console]::KeyAvailable){
      [void][Console]::ReadKey($true)
      Write-Host ''
      Write-Host '窗口已保留：查看完输出后，请直接关闭本窗口。'
      while($true){ Start-Sleep -Seconds 5 }
    }
    Start-Sleep -Seconds 1
  }
  Write-Host ''
}
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Write-Info([string]$m){ Write-Host "[INFO] $m" -ForegroundColor Cyan }
function Write-Success([string]$m){ Write-Host "[SUCCESS] $m" -ForegroundColor Green }
function Write-Warn([string]$m){ Write-Host "[WARNING] $m" -ForegroundColor Yellow }
function Write-Err([string]$m){ Write-Host "[ERROR] $m" -ForegroundColor Red }

$ServiceName = 'axonhub'
$BaseDir = Join-Path $env:LOCALAPPDATA 'AxonHub'
$PidFile = Join-Path $BaseDir 'axonhub.pid'
$ProcessName = 'axonhub'

function Show-Usage {
  Write-Host @" 
Usage: stop.bat [--force]

This script stops AxonHub directly (no service manager).
Options:
  --force     Force kill all AxonHub processes
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

function Stop-ByPid(){
  Write-Info 'Stopping AxonHub using PID file...'
  if(-not (Test-Path $PidFile)){
    Write-Warn "PID file not found at $PidFile"
    return $false
  }
  try {
    $runningPid = Get-Content -Path $PidFile -ErrorAction Stop
  } catch {
    Write-Warn 'Unable to read PID file'
    Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    return $false
  }
  if(-not ($runningPid -match '^[0-9]+$')){
    Write-Err "Invalid PID in file: $runningPid"
    Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    return $false
  }
  $proc = Get-Process -Id $runningPid -ErrorAction SilentlyContinue
  if(-not $proc){
    Write-Warn "Process with PID $runningPid is not running"
    Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    return $false
  }
  Write-Info "Stopping process $runningPid ..."
  try { Stop-Process -Id $runningPid -ErrorAction SilentlyContinue } catch {}
  # Wait up to 10 seconds
  $timeout = 10
  for($i=0; $i -lt $timeout; $i++){
    Start-Sleep -Seconds 1
    if(-not (Get-Process -Id $runningPid -ErrorAction SilentlyContinue)){ break }
  }
  if(Get-Process -Id $runningPid -ErrorAction SilentlyContinue){
    Write-Warn 'Process did not stop gracefully, forcing termination...'
    try { Stop-Process -Id $runningPid -Force -ErrorAction SilentlyContinue } catch {}
    Start-Sleep -Seconds 2
  }
  if(-not (Get-Process -Id $runningPid -ErrorAction SilentlyContinue)){
    Write-Success "AxonHub stopped successfully (PID: $runningPid)"
    Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    return $true
  } else {
    Write-Err 'Failed to stop AxonHub process'
    return $false
  }
}

function Stop-ByProcessName(){
  Write-Info 'Stopping AxonHub by process name...'
  $procs = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^axonhub' }
  if(-not $procs){
    Write-Warn 'No AxonHub processes found'
    return $false
  }
  Write-Info ("Found AxonHub processes: " + ($procs.Id -join ' '))
  foreach($p in $procs){
    Write-Info ("Stopping process " + $p.Id + ' ...')
    try { Stop-Process -Id $p.Id -ErrorAction SilentlyContinue } catch {}
    # Wait up to 10 seconds each
    $timeout = 10
    for($i=0; $i -lt $timeout; $i++){
      Start-Sleep -Seconds 1
      if(-not (Get-Process -Id $p.Id -ErrorAction SilentlyContinue)){ break }
    }
    if(Get-Process -Id $p.Id -ErrorAction SilentlyContinue){
      Write-Warn ("Process " + $p.Id + ' did not stop gracefully, forcing...')
      try { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } catch {}
    }
  }
  Start-Sleep -Seconds 2
  $remaining = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^axonhub' }
  if(-not $remaining){
    Write-Success 'All AxonHub processes stopped successfully'
    Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    return $true
  } else {
    Write-Err ("Some AxonHub processes are still running: " + ($remaining.Id -join ' '))
    return $false
  }
}

function Check-Running(){
  $procs = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^axonhub' }
  if($procs){
    Write-Info 'Running AxonHub processes:'
    $procs | Select-Object Id,ProcessName,Path | Format-Table -AutoSize | Out-String | Write-Host
    return $true
  }
  return $false
}

if($Force){
  Write-Info 'Force stopping all AxonHub processes...'
  $procs = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^axonhub' }
  if($procs){
    foreach($p in $procs){ try { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue } catch {} }
    Start-Sleep -Seconds 2
    $still = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match '^axonhub' }
    if(-not $still){
      Write-Success 'All AxonHub processes force-stopped'
      Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
    } else {
      Write-Err 'Failed to force-stop some processes'
      Wait-Exit; exit 1
    }
  } else {
    Write-Info 'No AxonHub processes found'
  }
  Wait-Exit; exit 0
}

Write-Info 'Stopping AxonHub...'
$stopped = $false
if(Stop-ByPid){ $stopped = $true }
if(-not $stopped){ if(Stop-ByProcessName){ $stopped = $true } }
if(-not $stopped){
  if(Check-Running){
    Write-Err 'Failed to stop all AxonHub processes'
    Wait-Exit; exit 1
  } else {
    Write-Info 'No AxonHub processes were running'
  }
}
Remove-Item -Force $PidFile -ErrorAction SilentlyContinue
Write-Success 'AxonHub has been stopped'

Wait-Exit
