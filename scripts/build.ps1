param(
    [string]$Output = "bin/clipnote.exe",
    [Alias("Debug")]
    [switch]$DebugBuild
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot

if (-not [IO.Path]::IsPathRooted($Output)) {
    $Output = Join-Path $repoRoot $Output
}
$outputDirectory = Split-Path -Parent $Output
New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

$ldflags = if ($DebugBuild) {
    "-s -w"
} else {
    "-s -w -H=windowsgui"
}

Push-Location $repoRoot
try {
    & go build -buildvcs=false -trimpath -ldflags $ldflags -o $Output ./cmd/clipnote
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed with exit code $LASTEXITCODE"
    }
    Write-Host "Built: $Output"
} finally {
    Pop-Location
}
