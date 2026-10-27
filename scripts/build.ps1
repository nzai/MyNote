param(
    [string]$Output = "bin/clipnote.exe",
    [string]$Version = "",
    [Alias("Debug", "Console", "ConsoleBuild")]
    [switch]$DebugBuild
)

if ($args.Count -gt 0) {
    throw "unknown build argument(s): $($args -join ' ')"
}

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

function Get-PeSubsystem([string]$Path) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt 64) {
        throw "not a PE file: $Path"
    }
    $peOffset = [BitConverter]::ToInt32($bytes, 0x3C)
    if ($peOffset -le 0 -or $bytes.Length -lt $peOffset + 94) {
        throw "invalid PE header offset in $Path"
    }
    if ($bytes[$peOffset] -ne 0x50 -or
        $bytes[$peOffset + 1] -ne 0x45 -or
        $bytes[$peOffset + 2] -ne 0 -or
        $bytes[$peOffset + 3] -ne 0) {
        throw "missing PE signature in $Path"
    }
    $optionalHeaderOffset = $peOffset + 24
    $magic = [BitConverter]::ToUInt16($bytes, $optionalHeaderOffset)
    if ($magic -ne 0x10B -and $magic -ne 0x20B) {
        throw "unsupported PE optional header in $Path"
    }
    return [BitConverter]::ToUInt16($bytes, $optionalHeaderOffset + 68)
}

if (-not [IO.Path]::IsPathRooted($Output)) {
    $Output = Join-Path $repoRoot $Output
}
$outputDirectory = Split-Path -Parent $Output
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

$ldflags = if ($DebugBuild) {
    "-H=windows"
} else {
    "-s -w -H=windowsgui"
}
if ($Version) {
    $Version = $Version.Trim()
    if ($Version -notmatch "^[0-9A-Za-z][0-9A-Za-z._+-]*$") {
        throw "invalid version string: $Version"
    }
    $ldflags += " -X main.version=$Version"
}
$expectedSubsystem = if ($DebugBuild) { 3 } else { 2 }

Push-Location $repoRoot
try {
    & go build -buildvcs=false -trimpath -ldflags $ldflags -o $Output ./cmd/clipnote
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
    $subsystem = Get-PeSubsystem $Output
    if ($subsystem -ne $expectedSubsystem) {
        throw "unexpected PE subsystem $subsystem in $Output; expected $expectedSubsystem"
    }
    $subsystemName = if ($subsystem -eq 2) { "GUI" } else { "console" }
    Write-Host "Built: $Output ($subsystemName subsystem)"
} finally {
    Pop-Location
}
