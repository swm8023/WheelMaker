[CmdletBinding()]
param(
    [string]$Root = (Join-Path $env:USERPROFILE ".wheelmaker\session"),
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$MagicText = "WMT2"
$MagicBytes = [System.Text.Encoding]::ASCII.GetBytes($MagicText)
$PreambleSize = 8
$SlotSize = 8
$Version1 = 1
$Version2 = 2
$TurnsPerFileV1 = 128
$TurnsPerFileV2 = 256

function Get-TurnFileFormat {
    param([int]$Version)

    switch ($Version) {
        $Version1 {
            return [pscustomobject]@{
                Version = $Version1
                TurnsPerFile = $TurnsPerFileV1
                HeaderSize = $PreambleSize + $TurnsPerFileV1 * $SlotSize
            }
        }
        $Version2 {
            return [pscustomobject]@{
                Version = $Version2
                TurnsPerFile = $TurnsPerFileV2
                HeaderSize = $PreambleSize + $TurnsPerFileV2 * $SlotSize
            }
        }
        default {
            throw "Unsupported turn file version $Version"
        }
    }
}

function Read-UInt16LE {
    param([byte[]]$Bytes, [int]$Offset)
    return [System.BitConverter]::ToUInt16($Bytes, $Offset)
}

function Read-UInt32LE {
    param([byte[]]$Bytes, [int]$Offset)
    return [System.BitConverter]::ToUInt32($Bytes, $Offset)
}

function Write-UInt16LE {
    param([byte[]]$Bytes, [int]$Offset, [uint16]$Value)
    [System.BitConverter]::GetBytes($Value).CopyTo($Bytes, $Offset)
}

function Write-UInt32LE {
    param([byte[]]$Bytes, [int]$Offset, [uint32]$Value)
    [System.BitConverter]::GetBytes($Value).CopyTo($Bytes, $Offset)
}

function Test-ByteArrayEqual {
    param([byte[]]$Left, [byte[]]$Right)

    if ($Left.Length -ne $Right.Length) {
        return $false
    }
    for ($i = 0; $i -lt $Left.Length; $i++) {
        if ($Left[$i] -ne $Right[$i]) {
            return $false
        }
    }
    return $true
}

function Read-TurnFile {
    param([string]$Path)

    $name = [System.IO.Path]::GetFileName($Path)
    if ($name -notmatch '^t(\d{6})\.bin$') {
        return $null
    }

    $fileNo = [int64]$Matches[1]
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt $PreambleSize) {
        throw "Turn file header too short: $Path"
    }
    $magic = [System.Text.Encoding]::ASCII.GetString($bytes, 0, 4)
    if ($magic -ne $MagicText) {
        throw "Invalid turn file magic: $Path"
    }

    $version = [int](Read-UInt16LE -Bytes $bytes -Offset 4)
    $format = Get-TurnFileFormat -Version $version
    if ($bytes.Length -lt $format.HeaderSize) {
        throw "Turn file header too short for version $version`: $Path"
    }

    $turns = New-Object 'System.Collections.Generic.List[object]'
    $expectedEnd = [int64]$format.HeaderSize
    $sawEmpty = $false
    for ($slot = 0; $slot -lt $format.TurnsPerFile; $slot++) {
        $slotOffset = $PreambleSize + $slot * $SlotSize
        $offset = [int64](Read-UInt32LE -Bytes $bytes -Offset $slotOffset)
        $length = [int64](Read-UInt32LE -Bytes $bytes -Offset ($slotOffset + 4))

        if ($offset -eq 0 -and $length -eq 0) {
            $sawEmpty = $true
            continue
        }
        if ($offset -eq 0 -or $length -eq 0) {
            throw "Partial turn slot $slot in $Path"
        }
        if ($sawEmpty) {
            throw "Non-empty turn slot $slot after an empty slot in $Path"
        }
        if ($offset -lt $format.HeaderSize) {
            throw "Turn slot $slot offset points into header in $Path"
        }
        if ($offset -lt $expectedEnd) {
            throw "Turn slot $slot overlaps previous body in $Path"
        }
        $end = $offset + $length
        if ($end -gt $bytes.Length) {
            throw "Turn slot $slot points outside file in $Path"
        }

        $content = New-Object byte[] $length
        [System.Buffer]::BlockCopy($bytes, [int]$offset, $content, 0, [int]$length)
        $turnIndex = $fileNo * [int64]$format.TurnsPerFile + [int64]$slot + 1
        $turns.Add([pscustomobject]@{
            Index = $turnIndex
            Content = $content
        }) | Out-Null
        $expectedEnd = $end
    }

    return [pscustomobject]@{
        FileNo = $fileNo
        Version = $version
        TurnsPerFile = $format.TurnsPerFile
        Turns = $turns
    }
}

function Read-TurnDirectory {
    param([string]$TurnsDir)

    $turns = New-Object 'System.Collections.Generic.SortedDictionary[System.Int64,System.Byte[]]'
    $versions = @{}
    $files = @(Get-ChildItem -LiteralPath $TurnsDir -File -Filter "t*.bin" | Sort-Object Name)

    foreach ($file in $files) {
        $parsed = Read-TurnFile -Path $file.FullName
        if ($null -eq $parsed) {
            continue
        }
        $versions[[string]$parsed.Version] = $true
        foreach ($turn in $parsed.Turns) {
            if ($turns.ContainsKey([int64]$turn.Index)) {
                throw "Duplicate turn index $($turn.Index) in $TurnsDir"
            }
            $turns.Add([int64]$turn.Index, [byte[]]$turn.Content)
        }
    }

    if ($turns.Count -gt 0) {
        $latest = [int64]($turns.Keys | Select-Object -Last 1)
        for ($index = [int64]1; $index -le $latest; $index++) {
            if (-not $turns.ContainsKey($index)) {
                throw "Missing turn index $index in $TurnsDir"
            }
        }
    }

    return [pscustomobject]@{
        Path = $TurnsDir
        Turns = $turns
        Versions = $versions
    }
}

function Write-V2TurnDirectory {
    param(
        [string]$TargetDir,
        [System.Collections.Generic.SortedDictionary[System.Int64,System.Byte[]]]$Turns
    )

    New-Item -ItemType Directory -Path $TargetDir -Force | Out-Null
    $groups = @{}
    foreach ($entry in $Turns.GetEnumerator()) {
        $turnIndex = [int64]$entry.Key
        $fileNo = [int64][Math]::Floor(($turnIndex - 1) / $TurnsPerFileV2)
        $slot = [int](($turnIndex - 1) % $TurnsPerFileV2)
        $key = [string]$fileNo
        if (-not $groups.ContainsKey($key)) {
            $groups[$key] = New-Object 'System.Collections.Generic.List[object]'
        }
        $groups[$key].Add([pscustomobject]@{
            Slot = $slot
            Content = [byte[]]$entry.Value
        }) | Out-Null
    }

    foreach ($key in ($groups.Keys | Sort-Object { [int64]$_ })) {
        $fileNo = [int64]$key
        $headerSize = $PreambleSize + $TurnsPerFileV2 * $SlotSize
        $header = New-Object byte[] $headerSize
        [System.Buffer]::BlockCopy($MagicBytes, 0, $header, 0, $MagicBytes.Length)
        Write-UInt16LE -Bytes $header -Offset 4 -Value ([uint16]$Version2)

        $stream = [System.IO.MemoryStream]::new()
        try {
            $stream.Write($header, 0, $header.Length)
            foreach ($item in ($groups[$key] | Sort-Object Slot)) {
                $offset = [uint32]$stream.Position
                $content = [byte[]]$item.Content
                $stream.Write($content, 0, $content.Length)
                $slotOffset = $PreambleSize + [int]$item.Slot * $SlotSize
                Write-UInt32LE -Bytes $header -Offset $slotOffset -Value $offset
                Write-UInt32LE -Bytes $header -Offset ($slotOffset + 4) -Value ([uint32]$content.Length)
            }
            $stream.Position = 0
            $stream.Write($header, 0, $header.Length)
            $path = Join-Path $TargetDir ("t{0:D6}.bin" -f $fileNo)
            [System.IO.File]::WriteAllBytes($path, $stream.ToArray())
        }
        finally {
            $stream.Dispose()
        }
    }
}

function Assert-SameTurns {
    param(
        [System.Collections.Generic.SortedDictionary[System.Int64,System.Byte[]]]$Expected,
        [System.Collections.Generic.SortedDictionary[System.Int64,System.Byte[]]]$Actual,
        [string]$Label
    )

    if ($Expected.Count -ne $Actual.Count) {
        throw "Converted turn count mismatch for $Label`: expected $($Expected.Count), got $($Actual.Count)"
    }
    foreach ($entry in $Expected.GetEnumerator()) {
        if (-not $Actual.ContainsKey([int64]$entry.Key)) {
            throw "Converted output missing turn $($entry.Key) for $Label"
        }
        if (-not (Test-ByteArrayEqual -Left ([byte[]]$entry.Value) -Right ([byte[]]$Actual[[int64]$entry.Key]))) {
            throw "Converted content mismatch at turn $($entry.Key) for $Label"
        }
    }
}

function Convert-TurnDirectory {
    param([pscustomobject]$Snapshot)

    $turnsDir = $Snapshot.Path
    $parent = Split-Path -Parent $turnsDir
    $stamp = Get-Date -Format "yyyyMMddHHmmssfff"
    $tempDir = Join-Path $parent "turns.convert256-$stamp"
    $backupDir = Join-Path $parent "turns.backup-v1-$stamp"

    if (Test-Path -LiteralPath $tempDir) {
        throw "Temporary directory already exists: $tempDir"
    }
    if (Test-Path -LiteralPath $backupDir) {
        throw "Backup directory already exists: $backupDir"
    }

    Write-V2TurnDirectory -TargetDir $tempDir -Turns $Snapshot.Turns
    $converted = Read-TurnDirectory -TurnsDir $tempDir
    Assert-SameTurns -Expected $Snapshot.Turns -Actual $converted.Turns -Label $turnsDir

    try {
        Move-Item -LiteralPath $turnsDir -Destination $backupDir
        Move-Item -LiteralPath $tempDir -Destination $turnsDir
    }
    catch {
        if ((-not (Test-Path -LiteralPath $turnsDir)) -and (Test-Path -LiteralPath $backupDir)) {
            Move-Item -LiteralPath $backupDir -Destination $turnsDir
        }
        throw
    }

    return [pscustomobject]@{
        TurnsDir = $turnsDir
        BackupDir = $backupDir
        Count = $Snapshot.Turns.Count
    }
}

if (-not (Test-Path -LiteralPath $Root)) {
    throw "Session root does not exist: $Root"
}

$rootPath = (Resolve-Path -LiteralPath $Root).ProviderPath
$turnDirs = @(Get-ChildItem -LiteralPath $rootPath -Directory -Recurse -Force | Where-Object { $_.Name -eq "turns" })
$snapshots = New-Object 'System.Collections.Generic.List[object]'

Write-Host "Scanning $($turnDirs.Count) turn directories under $rootPath"
foreach ($dir in $turnDirs) {
    if (-not $dir.FullName.StartsWith($rootPath, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to process path outside root: $($dir.FullName)"
    }
    $snapshot = Read-TurnDirectory -TurnsDir $dir.FullName
    if ($snapshot.Turns.Count -eq 0) {
        continue
    }
    $snapshots.Add($snapshot) | Out-Null
}

$jobs = @($snapshots | Where-Object { $_.Versions.ContainsKey([string]$Version1) })
$alreadyV2 = @($snapshots | Where-Object { -not $_.Versions.ContainsKey([string]$Version1) })

Write-Host "Validated $($snapshots.Count) non-empty turn directories."
Write-Host "Already v2/256: $($alreadyV2.Count)"
Write-Host "Need conversion: $($jobs.Count)"

if ($DryRun) {
    foreach ($job in $jobs) {
        Write-Host ("DRY RUN would convert {0} ({1} turns)" -f $job.Path, $job.Turns.Count)
    }
    exit 0
}

$convertedCount = 0
$convertedTurns = [int64]0
foreach ($job in $jobs) {
    $result = Convert-TurnDirectory -Snapshot $job
    $convertedCount++
    $convertedTurns += [int64]$result.Count
    Write-Host ("Converted {0} turns: {1}" -f $result.Count, $result.TurnsDir)
    Write-Host ("Backup kept at: {0}" -f $result.BackupDir)
}

Write-Host ("Done. Converted {0} directories and {1} turns to WMT2 v2/256." -f $convertedCount, $convertedTurns)
