# ==============================================================================
# TianQue-Probe (AegisProbe) Full-Architecture Fast Build Script
# Pure Go Static Compilation, Zero CGO, Zero External Dependencies
# ==============================================================================

$ErrorActionPreference = "Stop"
$startTime = Get-Date

Write-Host "==================================================================" -ForegroundColor Cyan
Write-Host "Starting TianQue-Probe Full Cross-Architecture Compilation..." -ForegroundColor Cyan
Write-Host "==================================================================" -ForegroundColor Cyan

$distDir = "release_bins"
$binDir = "bin"
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Path $binDir -Force | Out-Null
}
if (Test-Path $distDir) {
    Remove-Item -Path $distDir -Recurse -Force
}
New-Item -ItemType Directory -Path "$distDir/downloads" -Force | Out-Null
New-Item -ItemType Directory -Path "$distDir/aegis-ping" -Force | Out-Null

$env:CGO_ENABLED = "0"

# 1. Server Build
Write-Host "[1/6] Building Server (Linux amd64)..." -ForegroundColor Yellow
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$binDir/aegis_server_linux_amd64" ./cmd/server
Copy-Item "$binDir/aegis_server_linux_amd64" "$distDir/aegis_server_linux_amd64" -Force

Write-Host "[2/6] Building Server (Windows amd64)..." -ForegroundColor Yellow
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$binDir/aegis_server_windows_amd64.exe" ./cmd/server
Copy-Item "$binDir/aegis_server_windows_amd64.exe" "$distDir/aegis_server_windows_amd64.exe" -Force

# 2. Agent Build
Write-Host "[3/6] Building Agent (Linux amd64)..." -ForegroundColor Yellow
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$binDir/aegis_agent_linux_amd64" ./cmd/agent
Copy-Item "$binDir/aegis_agent_linux_amd64" "$distDir/downloads/aegis_agent_linux_amd64" -Force

Write-Host "[4/6] Building Agent (Linux arm64)..." -ForegroundColor Yellow
$env:GOOS = "linux"; $env:GOARCH = "arm64"
go build -ldflags "-s -w" -o "$binDir/aegis_agent_linux_arm64" ./cmd/agent
Copy-Item "$binDir/aegis_agent_linux_arm64" "$distDir/downloads/aegis_agent_linux_arm64" -Force

Write-Host "[5/6] Building Agent (Linux 386)..." -ForegroundColor Yellow
$env:GOOS = "linux"; $env:GOARCH = "386"
go build -ldflags "-s -w" -o "$binDir/aegis_agent_linux_386" ./cmd/agent
Copy-Item "$binDir/aegis_agent_linux_386" "$distDir/downloads/aegis_agent_linux_386" -Force

Write-Host "[6/6] Building Agent (Windows amd64)..." -ForegroundColor Yellow
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$binDir/aegis_agent_windows_amd64.exe" ./cmd/agent
Copy-Item "$binDir/aegis_agent_windows_amd64.exe" "$distDir/downloads/aegis_agent_windows_amd64.exe" -Force

# 3. Aegis-Ping 家宽测活微探针 (全架构 Linux)
Write-Host "" -ForegroundColor White
Write-Host "[Aegis-Ping 1/5] Building Ping Probe (Linux amd64)..." -ForegroundColor Magenta
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -ldflags "-s -w" -o "$binDir/aegis_ping_linux_amd64" ./cmd/aegis-ping
Copy-Item "$binDir/aegis_ping_linux_amd64" "$distDir/aegis-ping/aegis_ping_linux_amd64" -Force

Write-Host "[Aegis-Ping 2/5] Building Ping Probe (Linux arm64)..." -ForegroundColor Magenta
$env:GOOS = "linux"; $env:GOARCH = "arm64"
go build -ldflags "-s -w" -o "$binDir/aegis_ping_linux_arm64" ./cmd/aegis-ping
Copy-Item "$binDir/aegis_ping_linux_arm64" "$distDir/aegis-ping/aegis_ping_linux_arm64" -Force

Write-Host "[Aegis-Ping 3/5] Building Ping Probe (Linux 386)..." -ForegroundColor Magenta
$env:GOOS = "linux"; $env:GOARCH = "386"
go build -ldflags "-s -w" -o "$binDir/aegis_ping_linux_386" ./cmd/aegis-ping
Copy-Item "$binDir/aegis_ping_linux_386" "$distDir/aegis-ping/aegis_ping_linux_386" -Force

Write-Host "[Aegis-Ping 4/5] Building Ping Probe (Linux arm v7)..." -ForegroundColor Magenta
$env:GOOS = "linux"; $env:GOARCH = "arm"; $env:GOARM = "7"
go build -ldflags "-s -w" -o "$binDir/aegis_ping_linux_armv7" ./cmd/aegis-ping
Copy-Item "$binDir/aegis_ping_linux_armv7" "$distDir/aegis-ping/aegis_ping_linux_armv7" -Force

Write-Host "[Aegis-Ping 5/5] Building Ping Probe (Windows amd64)..." -ForegroundColor Magenta
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:GOARM = ""
go build -ldflags "-s -w" -o "$binDir/aegis_ping_windows_amd64.exe" ./cmd/aegis-ping
Copy-Item "$binDir/aegis_ping_windows_amd64.exe" "$distDir/aegis-ping/aegis_ping_windows_amd64.exe" -Force

# 拷贝安装脚本
Copy-Item "scripts/install_aegis_ping.sh" "$distDir/aegis-ping/install_aegis_ping.sh" -Force

# Restore environment
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:GOARM = ""

$endTime = Get-Date
$duration = [Math]::Round(($endTime - $startTime).TotalSeconds, 2)

Write-Host ""
Write-Host "=================================================================" -ForegroundColor Green
Write-Host "Success! All binaries compiled in $duration seconds." -ForegroundColor Green
Write-Host "Output Directory: $distDir" -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green

Get-ChildItem -Path $distDir -Recurse | Where-Object { -not $_.PSIsContainer } | Select-Object Name, Length | Format-Table -AutoSize
