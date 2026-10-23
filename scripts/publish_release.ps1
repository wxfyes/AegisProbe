# ==============================================================================
# 🛡️ 天阙·神盾 (AegisProbe) 本地极速编译与 GitHub Release 自动化发布脚本
# ==============================================================================

param (
    [string]$Tag = "v1.0.0",
    [string]$Token = ""
)

$ErrorActionPreference = "Stop"
$repo = "wxfyes/AegisProbe"

if (-not $Token) {
    if (Test-Path ".token") {
        $Token = (Get-Content ".token" -Raw).Trim()
    } elseif ($env:GITHUB_TOKEN) {
        $Token = $env:GITHUB_TOKEN
    }
}

if (-not $Token) {
    Write-Host "Error: GitHub Token not found!" -ForegroundColor Red
    exit 1
}

Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "Starting TianQue-Probe Release Publish: $Tag" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

# 1. 执行全架构编译
Write-Host "Compiling all architectures..." -ForegroundColor Yellow
& powershell -ExecutionPolicy Bypass -File scripts\build_all.ps1

$headers = @{
    "Authorization" = "token $Token"
    "Accept"        = "application/vnd.github.v3+json"
    "User-Agent"    = "TianQue-AegisProbe-Deployer"
}

# 2. 读取发布说明
$releaseBody = ""
if (Test-Path "scripts/release_notes.md") {
    $releaseBody = [System.IO.File]::ReadAllText("scripts/release_notes.md", [System.Text.Encoding]::UTF8)
} else {
    $releaseBody = "AegisProbe Release $Tag"
}

# 3. 检查或创建 Release
Write-Host "Checking Release: $Tag ..." -ForegroundColor Yellow
$release = $null
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/tags/$Tag" -Headers $headers -Method Get
    Write-Host "Release already exists. ID: $($release.id)" -ForegroundColor Green
} catch {
    Write-Host "Creating new Release: $Tag ..." -ForegroundColor Yellow
    $createPayload = @{
        tag_name         = $Tag
        target_commitish = "main"
        name             = "AegisProbe $Tag Official Release"
        body             = $releaseBody
        draft            = $false
        prerelease       = $false
    }
    $createJson = [System.Text.Encoding]::UTF8.GetBytes(($createPayload | ConvertTo-Json -Depth 5))
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases" -Headers $headers -Method Post -Body $createJson -ContentType "application/json; charset=utf-8"
}

$releaseId = $release.id
Write-Host "Release ID: $releaseId" -ForegroundColor Green

# 4. 待上传的文件列表
$filesToUpload = @(
    "release_bins/aegis_server_linux_amd64",
    "release_bins/aegis_server_windows_amd64.exe",
    "release_bins/downloads/aegis_agent_linux_amd64",
    "release_bins/downloads/aegis_agent_linux_arm64",
    "release_bins/downloads/aegis_agent_linux_386",
    "release_bins/downloads/aegis_agent_windows_amd64.exe",
    "install.sh"
)

# 5. 清理已有资产 (防止重名冲突)
$existingAssets = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/$releaseId/assets" -Headers $headers -Method Get
if ($existingAssets) {
    foreach ($oldAsset in $existingAssets) {
        Write-Host "Removing existing asset: $($oldAsset.name)..." -ForegroundColor DarkGray
        Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/assets/$($oldAsset.id)" -Headers $headers -Method Delete | Out-Null
    }
}

# 6. 上传新资产
foreach ($filePath in $filesToUpload) {
    if (-not (Test-Path $filePath)) {
        Write-Host "Warning: File not found: $filePath" -ForegroundColor Yellow
        continue
    }

    $fileName = Split-Path -Leaf $filePath
    $fileBytes = [System.IO.File]::ReadAllBytes((Resolve-Path $filePath))
    $uploadUri = "https://uploads.github.com/repos/$repo/releases/$releaseId/assets?name=$fileName"

    Write-Host "Uploading: $fileName ($([Math]::Round($fileBytes.Length / 1MB, 2)) MB)..." -ForegroundColor Cyan

    $uploadHeaders = @{
        "Authorization"  = "token $Token"
        "Content-Type"   = "application/octet-stream"
        "User-Agent"     = "TianQue-AegisProbe-Deployer"
    }

    $uploadRes = Invoke-RestMethod -Uri $uploadUri -Headers $uploadHeaders -Method Post -Body $fileBytes
    Write-Host "Upload completed: $fileName" -ForegroundColor Green
}

Write-Host ""
Write-Host "=================================================================" -ForegroundColor Green
Write-Host "Release Publish Completed!" -ForegroundColor Green
Write-Host "Release URL: https://github.com/$repo/releases/tag/$Tag" -ForegroundColor Green
Write-Host "Latest URL : https://github.com/$repo/releases/latest" -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green
