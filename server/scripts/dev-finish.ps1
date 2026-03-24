param(
  [string]$Remote = "origin",
  [string]$Branch = "",
  [switch]$SkipTest,
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"

function Invoke-Step {
  param(
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][scriptblock]$Action
  )
  Write-Host ("==> {0}" -f $Name)
  & $Action
}

function Invoke-External {
  param(
    [Parameter(Mandatory = $true)][string]$FilePath,
    [string[]]$Arguments = @()
  )
  if ($DryRun) {
    $render = @($FilePath) + $Arguments
    Write-Host ("[dry-run] {0}" -f ($render -join " "))
    return
  }
  & $FilePath @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw ("command failed (exit={0}): {1} {2}" -f $LASTEXITCODE, $FilePath, ($Arguments -join " "))
  }
}

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$restartScript = Join-Path $PSScriptRoot "delayed-restart.ps1"

Push-Location $repoRoot
try {
  if ([string]::IsNullOrWhiteSpace($Branch)) {
    $Branch = (& git branch --show-current).Trim()
  }
  if ([string]::IsNullOrWhiteSpace($Branch)) {
    throw "cannot detect current git branch; pass -Branch explicitly"
  }

  if (-not $SkipTest) {
    Invoke-Step -Name "go test ./..." -Action {
      Invoke-External -FilePath "go" -Arguments @("test", "./...")
    }
  } else {
    Write-Host "==> skip tests (-SkipTest)"
  }

  Invoke-Step -Name ("git push {0} {1}" -f $Remote, $Branch) -Action {
    Invoke-External -FilePath "git" -Arguments @("push", $Remote, $Branch)
  }

  Invoke-Step -Name "trigger delayed restart" -Action {
    if ($DryRun) {
      Write-Host ("[dry-run] powershell -NoProfile -ExecutionPolicy Bypass -File {0}" -f $restartScript)
      return
    }
    powershell -NoProfile -ExecutionPolicy Bypass -File $restartScript
    if ($LASTEXITCODE -ne 0) {
      throw ("restart script failed (exit={0})" -f $LASTEXITCODE)
    }
  }

  Write-Host "==> done"
}
finally {
  Pop-Location
}
