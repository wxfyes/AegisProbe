# ==============================================================================
# TianQue-Probe (AegisProbe) Full-Architecture Fast Build Script
# Pure Go Static Compilation, Zero CGO, Zero External Dependencies
# ==============================================================================

$ErrorActionPreference = "Stop"
$startTime = Get-Date

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Starting TianQue-Probe Full Cross-Architecture Compilation..." -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

$distDir = "release_bins"
if (Test-Path $distDir) {
    Remove-Item -Path $distDir -Recurse -Force
}
New-Item -ItemType Directory -Path "$distDir/downloads" -Force | Out-Null

$env:CGO_ENABLED = "0"

# 1. Server Build
Write-Host "[1/6] Building Server (Linux amd64)..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$distDir/aegis_server_linux_amd64" ./cmd/server

Write-Host "[2/6] Building Server (Windows amd64)..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$distDir/aegis_server_windows_amd64.exe" ./cmd/server

# 2. Agent Build
Write-Host "[3/6] Building Agent (Linux amd64)..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$distDir/downloads/aegis_agent_linux_amd64" ./cmd/agent

Write-Host "[4/6] Building Agent (Linux arm64)..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "arm64"
go build -ldflags "-s -w" -o "$distDir/downloads/aegis_agent_linux_arm64" ./cmd/agent

Write-Host "[5/6] Building Agent (Linux 386)..." -ForegroundColor Yellow
$env:GOOS = "linux"
$env:GOARCH = "386"
go build -ldflags "-s -w" -o "$distDir/downloads/aegis_agent_linux_386" ./cmd/agent

Write-Host "[6/6] Building Agent (Windows amd64)..." -ForegroundColor Yellow
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$distDir/downloads/aegis_agent_windows_amd64.exe" ./cmd/agent

# Restore environment
$env:GOOS = "windows"
$env:GOARCH = "amd64"

$endTime = Get-Date
$duration = [Math]::Round(($endTime - $startTime).TotalSeconds, 2)

Write-Host ""
Write-Host "=================================================================" -ForegroundColor Green
Write-Host "Success! All binaries compiled in $duration seconds." -ForegroundColor Green
Write-Host "Output Directory: $distDir" -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green

Get-ChildItem -Path $distDir -Recurse | Where-Object { -not $_.PSIsContainer } | Select-Object Name, Length | Format-Table -AutoSize
