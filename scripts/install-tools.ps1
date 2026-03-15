# install-tools.ps1 - Download third-party tool binaries to bin\windows_amd64\
param()
Set-StrictMode -Off

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
$Dest = Join-Path $RepoRoot "bin\windows_amd64"

Write-Host "Platform: windows_amd64"
Write-Host "Destination: $Dest"
New-Item -ItemType Directory -Force -Path $Dest | Out-Null

function Install-CodexAcp {
    Write-Host "Installing codex-acp..."
    $OutFile = Join-Path $Dest "codex-acp.exe"

    # Try GitHub Releases first
    $githubSuccess = $false
    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/zed-industries/codex-acp/releases/latest" -ErrorAction Stop
        $tag = $rel.tag_name
        $url = "https://github.com/zed-industries/codex-acp/releases/download/$tag/codex-acp-windows-amd64.exe"
        Invoke-WebRequest -Uri $url -OutFile $OutFile -ErrorAction Stop
        Write-Host "codex-acp installed from GitHub Releases ($tag)"
        $githubSuccess = $true
    } catch {
        Write-Host "GitHub Releases not available: $_"
    }

    if ($githubSuccess) { return }

    # Fallback: npx install then locate the binary
    $npxCmd = Get-Command npx -ErrorAction SilentlyContinue
    if (-not $npxCmd) {
        Write-Warning "npx not found. Install Node.js from https://nodejs.org/"
        Write-Warning "Then place codex-acp.exe at: $Dest\codex-acp.exe"
        return
    }

    Write-Host "Trying npm install -g @zed-industries/codex-acp..."
    try {
        & npm install -g @zed-industries/codex-acp 2>&1 | Out-Null
    } catch {
        Write-Host "npm install failed: $_"
    }

    # Find the binary: check npm global root and common locations
    $candidates = @()
    $npmRoot = & npm root -g 2>$null
    if ($npmRoot) {
        # Direct binary in package
        $candidates += Join-Path $npmRoot "@zed-industries\codex-acp\codex-acp.exe"
        # Platform-specific binary nested inside the package
        $candidates += Join-Path $npmRoot "@zed-industries\codex-acp\node_modules\@zed-industries\codex-acp-win32-x64\bin\codex-acp.exe"
    }
    $appData = $env:APPDATA
    if ($appData) {
        $candidates += "$appData\npm\codex-acp.exe"
        $candidates += "$appData\npm\node_modules\@zed-industries\codex-acp\node_modules\@zed-industries\codex-acp-win32-x64\bin\codex-acp.exe"
    }

    foreach ($c in $candidates) {
        if (Test-Path $c) {
            Copy-Item $c $OutFile
            Write-Host "codex-acp copied from $c"
            return
        }
    }

    Write-Warning "npx ran but could not locate codex-acp.exe binary."
    Write-Warning "Place codex-acp.exe manually at: $Dest\codex-acp.exe"
}

function Install-ClaudeAgentAcp {
    Write-Host "Installing claude-agent-acp..."

    # claude-agent-acp is a Node.js package; it ships a .cmd wrapper, not a native .exe.
    # We copy the npm-generated .cmd wrapper to bin\windows_amd64\claude-agent-acp.cmd.
    $OutFile = Join-Path $Dest "claude-agent-acp.cmd"

    # Try GitHub Releases first (in case a native binary is published in the future).
    $githubSuccess = $false
    try {
        $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/zed-industries/claude-agent-acp/releases/latest" -ErrorAction Stop
        $tag = $rel.tag_name
        $url = "https://github.com/zed-industries/claude-agent-acp/releases/download/$tag/claude-agent-acp-windows-amd64.exe"
        $exeOut = Join-Path $Dest "claude-agent-acp.exe"
        Invoke-WebRequest -Uri $url -OutFile $exeOut -ErrorAction Stop
        Write-Host "claude-agent-acp installed from GitHub Releases ($tag)"
        $githubSuccess = $true
    } catch {
        Write-Host "GitHub Releases not available: $_"
    }

    if ($githubSuccess) { return }

    # Fallback: npm install then locate the .cmd wrapper.
    $npmCmd = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $npmCmd) {
        Write-Warning "npm not found. Install Node.js from https://nodejs.org/"
        Write-Warning "Then run: npm install -g @zed-industries/claude-agent-acp"
        return
    }

    Write-Host "Trying npm install -g @zed-industries/claude-agent-acp..."
    try {
        & npm install -g @zed-industries/claude-agent-acp 2>&1 | Out-Null
    } catch {
        Write-Host "npm install failed: $_"
    }

    # Locate dist/index.js using npm prefix -g and generate a wrapper .cmd with absolute path.
    # We cannot simply copy the npm .cmd because it uses %dp0% (relative to its own dir).
    $indexJs = $null
    $npmPrefix = & npm prefix -g 2>$null
    if ($npmPrefix) {
        $candidate = Join-Path $npmPrefix "node_modules\@zed-industries\claude-agent-acp\dist\index.js"
        if (Test-Path $candidate) { $indexJs = $candidate }
    }
    if (-not $indexJs) {
        $appData = $env:APPDATA
        if ($appData) {
            $candidate = "$appData\npm\node_modules\@zed-industries\claude-agent-acp\dist\index.js"
            if (Test-Path $candidate) { $indexJs = $candidate }
        }
    }

    if ($indexJs) {
        # Generate a .cmd that calls node with the absolute path to dist/index.js.
        $cmd = "@ECHO off`r`nnode `"$indexJs`" %*`r`n"
        [System.IO.File]::WriteAllText($OutFile, $cmd, [System.Text.Encoding]::ASCII)
        Write-Host "claude-agent-acp.cmd generated (pointing to $indexJs)"
        return
    }

    Write-Warning "npm ran but could not locate claude-agent-acp dist/index.js."
    Write-Warning "Run: npm install -g @zed-industries/claude-agent-acp"
}

Install-CodexAcp
Install-ClaudeAgentAcp
Write-Host "Done."
