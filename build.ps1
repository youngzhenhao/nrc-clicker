<#
.SYNOPSIS
    Build nrc-clicker.exe for Windows.

.DESCRIPTION
    Runs the full pre-flight + build pipeline:
      1. Verify Go is installed (and on the right arch).
      2. go mod tidy  — refresh indirect dependencies.
      3. go vet ./... — static analysis.
      4. go test ./... — unit tests (skippable via -SkipTest).
      5. go build     — produce dist\nrc-clicker.exe with:
                          * -H windowsgui        (no console window on launch)
                          * -trimpath            (strip local paths from binary)
                          * -ldflags "-s -w"     (strip symbol/debug info)
      6. Print the resulting binary size.

.PARAMETER SkipTest
    Skip the unit-test stage. Useful for fast iteration when only
    cosmetic changes were made.

.PARAMETER VerboseBuild
    Print full go command output instead of suppressing it. Useful for
    debugging build errors. (Renamed from -Verbose to avoid collision
    with PowerShell's built-in -Verbose common parameter.)

.PARAMETER Output
    Override the output exe path. Default: dist\nrc-clicker.exe.

.EXAMPLE
    .\build.ps1
    Build with defaults.

.EXAMPLE
    .\build.ps1 -SkipTest
    Skip the unit-test stage.

.EXAMPLE
    .\build.ps1 -Output bin\clicker.exe -VerboseBuild
    Custom output path + verbose logging.
#>

[CmdletBinding()]
param(
    [switch]$SkipTest,
    [switch]$VerboseBuild,
    [string]$Output = "dist\nrc-clicker.exe"
)

$ErrorActionPreference = "Stop"

# ----------------------------------------------------------------------------
# 0. Locate script directory so relative paths resolve regardless of cwd.
# ----------------------------------------------------------------------------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ScriptDir

# ----------------------------------------------------------------------------
# 1. Verify Go toolchain.
# ----------------------------------------------------------------------------
$go = (Get-Command go -ErrorAction SilentlyContinue)
if (-not $go) {
    Write-Host "❌ 'go' not found on PATH. Install Go 1.21+ from https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

$goVer = (& go version) -replace '^go version ', ''
Write-Host "▶ Go $goVer found at $($go.Path)" -ForegroundColor Cyan

if ($goVer -notmatch '^go(\d+)\.(\d+)') {
    Write-Host "❌ Could not parse go version output: $goVer" -ForegroundColor Red
    exit 1
}
$major = [int]$Matches[1]
$minor = [int]$Matches[2]
if ($major -lt 1 -or ($major -eq 1 -and $minor -lt 21)) {
    Write-Host "❌ Go 1.21+ required, found $goVer" -ForegroundColor Red
    exit 1
}

# Confirm Windows / amd64 target. We don't actually set GOOS / GOARCH here
# because the dev machine already matches, but warn loudly if not.
$env:GOOS = "windows"
$env:GOARCH = "amd64"
if ($VerboseBuild) {
    Write-Host "  GOOS=$env:GOOS GOARCH=$env:GOARCH" -ForegroundColor DarkGray
}

# ----------------------------------------------------------------------------
# 2. go mod tidy.
# ----------------------------------------------------------------------------
Write-Host "▶ go mod tidy ..." -ForegroundColor Cyan
$tidyOut = & go mod tidy 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ go mod tidy failed" -ForegroundColor Red
    Write-Host ($tidyOut -join "`n") -ForegroundColor Red
    exit 1
}

# ----------------------------------------------------------------------------
# 3. go vet.
# ----------------------------------------------------------------------------
Write-Host "▶ go vet ./... ..." -ForegroundColor Cyan
# Use cmd /c to run go vet so PowerShell doesn't trip its $ErrorActionPreference
# on stderr records emitted by the native go.exe process.
$vetTmp = [System.IO.Path]::GetTempFileName()
cmd /c "go vet ./... > `"$vetTmp`" 2>&1" | Out-Null
$vetExit = $LASTEXITCODE
$vetOut = Get-Content -Raw $vetTmp -ErrorAction SilentlyContinue
Remove-Item $vetTmp -ErrorAction SilentlyContinue

# go vet returns 1 on warnings. We tolerate the single informational
# unsafeptr warning emitted for the WH_KEYBOARD_LL callback in
# internal/hotkey/listener.go (kernel-provided lParam; standard pattern
# in golang.org/x/sys/windows callbacks). Any other vet finding fails.
$vetIgnorable = $false
if ($vetExit -ne 0) {
    $lines = @($vetOut -split "`r?`n" | Where-Object { $_ -match '\S' })
    if ($lines.Count -eq 1 -and $lines[0] -match 'internal[\\/]hotkey[\\/]listener\.go:\d+:\d+: possible misuse of unsafe\.Pointer') {
        $vetIgnorable = $true
    }
}
if ($vetExit -ne 0 -and -not $vetIgnorable) {
    Write-Host "❌ go vet failed" -ForegroundColor Red
    Write-Host $vetOut -ForegroundColor Red
    exit 1
}
if ($vetIgnorable) {
    Write-Host "  (ignored informational unsafeptr warning on WH_KEYBOARD_LL callback)" -ForegroundColor DarkYellow
}

# ----------------------------------------------------------------------------
# 4. go test (skippable).
# ----------------------------------------------------------------------------
if (-not $SkipTest) {
    Write-Host "▶ go test ./... ..." -ForegroundColor Cyan
    $testOut = & go test ./... 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ go test failed" -ForegroundColor Red
        Write-Host ($testOut -join "`n") -ForegroundColor Red
        exit 1
    }
    Write-Host "  tests OK" -ForegroundColor Green
} else {
    Write-Host "⏭  skipped tests (-SkipTest)" -ForegroundColor DarkYellow
}

# ----------------------------------------------------------------------------
# 5. go build.
# ----------------------------------------------------------------------------
# Resolve output dir relative to script dir.
$outputAbs = Join-Path $ScriptDir $Output
$outputDir = Split-Path -Parent $outputAbs
if (-not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

# -H windowsgui hides the console window when the user double-clicks the exe.
# -trimpath removes local path info from the binary (reproducible builds).
# -s -w strips symbol table & DWARF info (smaller binary).
$ldflags = "-H windowsgui -s -w"

Write-Host "▶ go build -ldflags '$ldflags' -trimpath -o $Output ..." -ForegroundColor Cyan

if ($VerboseBuild) {
    & go build -ldflags $ldflags -trimpath -o $OutputAbs
} else {
    & go build -ldflags $ldflags -trimpath -o $outputAbs *> $null
}

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ go build failed" -ForegroundColor Red
    if (-not $Verbose) {
        Write-Host "   re-run with -Verbose to see full output" -ForegroundColor DarkYellow
    }
    exit 1
}

# ----------------------------------------------------------------------------
# 6. Report result.
# ----------------------------------------------------------------------------
$exe = Get-Item $outputAbs
$sizeMB = [math]::Round($exe.Length / 1MB, 2)
$sizeKB = [math]::Round($exe.Length / 1KB, 1)
$sizeStr = if ($sizeMB -ge 1) { "$sizeMB MB" } else { "$sizeKB KB" }

Write-Host ""
Write-Host "✅ Built $($exe.Name)  ($sizeStr)" -ForegroundColor Green
Write-Host "   path: $($exe.FullName)" -ForegroundColor DarkGray
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  • Ensure Interception driver is installed (see README.md)" -ForegroundColor Gray
Write-Host "  • Run:  Start-Process '$($exe.FullName)'" -ForegroundColor Gray
Write-Host "  • Or:   &$($exe.FullName)" -ForegroundColor Gray