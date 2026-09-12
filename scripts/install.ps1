<#
.SYNOPSIS
    pt-tools Windows 安装脚本。

.DESCRIPTION
    依据 CI/Release 迁移设计 v7 §3.1 与 github-project-scaffold
    references/install-script.md 的 checksum 契约：

      1. 解析 TOOL_VERSION，或回退到最新已发布 Release；
      2. 选择本平台归档；
      3. 从**同一 tag** 下载归档与 checksums.txt；
      4. 在 checksum 文件中定位该归档的精确条目；
      5. 本地计算 SHA-256；
      6. 摘要不符则拒绝解压（绝不降级为警告）。

    本脚本为项目自有实现（已在 .github/scaffold.json 的 drift_allow 登记）：
    pt-tools 采用**无版本号**资产命名以保住 releases/latest/download/... 稳定
    链接，与 profile 自带安装脚本假定的 BINARY_VERSION_OS_ARCH 命名不兼容。

.PARAMETER TOOL_VERSION
    环境变量。要安装的版本（例如 v0.48.0 或 0.48.0）；缺省取最新 Release。

.PARAMETER TOOL_INSTALL_DIR
    环境变量。安装目录，缺省 $env:LOCALAPPDATA\pt-tools\bin。

.NOTES
    支持矩阵：windows/amd64、windows/arm64。不提供 macOS 产物；
    Linux 请使用 scripts/install.sh。
#>

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$Owner = 'sunerpy'
$Repo = 'pt-tools'
$Binary = 'pt-tools'

$InstallDir = if ($env:TOOL_INSTALL_DIR) {
    $env:TOOL_INSTALL_DIR
} else {
    Join-Path $env:LOCALAPPDATA 'pt-tools\bin'
}

function Fail([string]$Message) {
    Write-Error "error: $Message"
    exit 1
}

# ---- 平台探测 ----
$archRaw = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
switch ($archRaw) {
    'X64'   { $archName = 'amd64' }
    'Arm64' { $archName = 'arm64' }
    default { Fail "unsupported architecture: $archRaw" }
}

$asset = "$Binary-windows-$archName.exe.zip"

# ---- 版本解析 ----
if ($env:TOOL_VERSION) {
    $tag = if ($env:TOOL_VERSION.StartsWith('v')) { $env:TOOL_VERSION } else { "v$($env:TOOL_VERSION)" }
} else {
    Write-Host 'Resolving the latest release...'
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" `
        -Headers @{ 'User-Agent' = 'pt-tools-installer' }
    $tag = $latest.tag_name
    if (-not $tag) { Fail 'could not resolve the latest release tag' }
}

Write-Host "Installing $Binary $tag (windows/$archName) into $InstallDir"

$base = "https://github.com/$Owner/$Repo/releases/download/$tag"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    # ---- 从同一 tag 下载归档与校验和 ----
    $archivePath = Join-Path $tmp $asset
    Write-Host "Downloading $asset..."
    try {
        Invoke-WebRequest -Uri "$base/$asset" -OutFile $archivePath -UseBasicParsing
    } catch {
        Fail "download failed: $base/$asset"
    }

    $checksumPath = Join-Path $tmp 'checksums.txt'
    Write-Host 'Downloading checksums.txt...'
    try {
        Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile $checksumPath -UseBasicParsing
    } catch {
        Fail "download failed: $base/checksums.txt (该版本可能早于 checksums 契约)"
    }

    # ---- 定位精确条目并校验 ----
    # checksums.txt 每行形如 "<64 hex>  <basename>"，名称为裸 basename。
    $expected = $null
    foreach ($line in Get-Content -LiteralPath $checksumPath) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split '\s+', 2
        if ($parts.Count -lt 2) { continue }
        $name = $parts[1].Trim().TrimStart('*')
        if ($name -eq $asset) {
            $expected = $parts[0].Trim().ToLowerInvariant()
            break
        }
    }
    if (-not $expected) {
        Fail "checksums.txt 中没有 $asset 的条目；拒绝安装"
    }
    if ($expected -notmatch '^[0-9a-f]{64}$') {
        Fail "checksums.txt 中 $asset 的摘要格式非法: $expected"
    }

    $actual = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()

    if ($expected -ne $actual) {
        Write-Host 'error: 校验和不匹配，拒绝解压' -ForegroundColor Red
        Write-Host "  asset:    $asset"
        Write-Host "  expected: $expected"
        Write-Host "  actual:   $actual"
        exit 1
    }
    Write-Host "Checksum OK ($actual)"

    # ---- 解压并安装 ----
    $extractDir = Join-Path $tmp 'extract'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDir -Force

    $exePath = Join-Path $extractDir "$Binary.exe"
    if (-not (Test-Path -LiteralPath $exePath)) {
        Fail "归档内未找到 $Binary.exe"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $target = Join-Path $InstallDir "$Binary.exe"
    Copy-Item -LiteralPath $exePath -Destination $target -Force

    Write-Host "Installed: $target"

    $pathParts = ($env:PATH -split ';') | Where-Object { $_ }
    if ($pathParts -notcontains $InstallDir) {
        Write-Host "提示：$InstallDir 不在 PATH 中，请将其加入用户环境变量。"
    }

    & $target version
} finally {
    Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}
