param(
  [string]$InstallDir = (Join-Path -Path $HOME -ChildPath ".wheelmaker\\bin"),
  [string]$SourceExe = "",
  [switch]$NoBuild,
  [switch]$WhatIf
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Step {
  param([string]$Text)
  Write-Host ("==> {0}" -f $Text)
}

function Resolve-SourceExe {
  param(
    [string]$RepoRoot,
    [string]$HintSource,
    [switch]$SkipBuild
  )
  if (-not [string]::IsNullOrWhiteSpace($HintSource)) {
    return (Resolve-Path $HintSource).Path
  }
  $defaultExe = Join-Path -Path $RepoRoot -ChildPath "bin\\windows_amd64\\wheelmaker.exe"
  if ($SkipBuild) {
    if (-not (Test-Path $defaultExe)) {
      throw "source binary not found: $defaultExe (build disabled by -NoBuild)"
    }
    return (Resolve-Path $defaultExe).Path
  }
  Write-Step "build wheelmaker binary"
  New-Item -ItemType Directory -Path (Split-Path $defaultExe -Parent) -Force | Out-Null
  & go build -o $defaultExe ./cmd/wheelmaker/
  if ($LASTEXITCODE -ne 0) {
    throw "go build failed (exit=$LASTEXITCODE)"
  }
  return (Resolve-Path $defaultExe).Path
}

function Get-WheelmakerProcesses {
  $rows = Get-CimInstance Win32_Process -Filter "Name='wheelmaker.exe'" |
    Select-Object ProcessId, CommandLine, ExecutablePath
  return @($rows)
}

function Get-ProcessClass {
  param([string]$CommandLine)
  $cmd = [string]$CommandLine
  if ($cmd -match '(^|\s)-d(\s|$)' -and $cmd -notmatch '--daemon-worker') {
    return "guardian"
  }
  if ($cmd -match '--daemon-worker') {
    return "worker"
  }
  return "other"
}

function Stop-WheelmakerProcesses {
  param([switch]$Preview)
  $all = Get-WheelmakerProcesses
  if (@($all).Count -eq 0) {
    Write-Step "no running wheelmaker process found"
    return
  }

  $guardians = @()
  $workers = @()
  $others = @()
  foreach ($p in $all) {
    switch (Get-ProcessClass $p.CommandLine) {
      "guardian" { $guardians += $p; break }
      "worker"   { $workers += $p; break }
      default    { $others += $p; break }
    }
  }
  $ordered = @($guardians + $workers + $others)
  Write-Step ("stopping wheelmaker processes (guardian first): {0}" -f (($ordered | ForEach-Object { $_.ProcessId }) -join ","))

  foreach ($p in $ordered) {
    if ($Preview) {
      Write-Host ("[whatif] stop pid={0} class={1}" -f $p.ProcessId, (Get-ProcessClass $p.CommandLine))
      continue
    }
    Stop-Process -Id $p.ProcessId -ErrorAction SilentlyContinue
  }

  if ($Preview) {
    return
  }

  Start-Sleep -Seconds 2
  $left = Get-WheelmakerProcesses
  if (@($left).Count -gt 0) {
    Write-Step ("force stopping remaining pids: {0}" -f (($left | ForEach-Object { $_.ProcessId }) -join ","))
    foreach ($p in $left) {
      Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
    }
  }
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$source = Resolve-SourceExe -RepoRoot $repoRoot -HintSource $SourceExe -SkipBuild:$NoBuild
$targetDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallDir)
$targetExe = Join-Path -Path $targetDir -ChildPath "wheelmaker.exe"

Write-Step ("source: {0}" -f $source)
Write-Step ("target: {0}" -f $targetExe)
Stop-WheelmakerProcesses -Preview:$WhatIf

if ($WhatIf) {
  Write-Host ("[whatif] copy {0} -> {1}" -f $source, $targetExe)
  Write-Host "[whatif] install done"
  return
}

New-Item -ItemType Directory -Path $targetDir -Force | Out-Null
Copy-Item -Path $source -Destination $targetExe -Force
Write-Step "install done"
