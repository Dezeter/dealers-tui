# build.ps1 — build dealers-tui.exe (Windows amd64, static, no cgo/gcc needed).
#
#   .\build.ps1              # → dealers-tui.exe
#   .\build.ps1 -Out bin\dealers-tui.exe
#
# Pure-Go build (modernc.org/sqlite), so CGO is off and no C toolchain is
# required. -s -w strips debug info for a smaller binary.

param(
    [string]$Out = "dealers-tui.exe"
)

$ErrorActionPreference = "Stop"

# Stamp the version from git if available, else a timestamp.
$ver = try { (git describe --tags --always --dirty 2>$null) } catch { $null }
if (-not $ver) { $ver = "build-" + (Get-Date -Format "yyyyMMdd-HHmm") }

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

Write-Host "building $Out ($ver)…"
go build -trimpath -ldflags "-s -w -X main.version=$ver" -o $Out ./cmd/dealers-tui
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$size = [math]::Round((Get-Item $Out).Length / 1MB, 1)
Write-Host "done: $Out ($size MB, version $ver)"
