# 下载、校验并解出随包 ffmpeg（Windows 完整包构建用）。
# 固定来源：gyan.dev 9.0.2 essentials（GPLv3），SHA-256 与 internal/minidownload/ffmpeg.go 保持一致。
param(
    [string]$CacheDir = (Join-Path $PSScriptRoot '..\dist\ffmpeg-cache'),
    [Parameter(Mandatory = $true)]
    [string]$OutputDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$sourceUrl = 'https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip'
$sourceSha256 = '60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba'
$expectedBin = 'bin/ffmpeg.exe'
$expectedLicense = 'LICENSE'

Add-Type -AssemblyName System.IO.Compression.FileSystem

function Get-FileSha256([string]$Path) {
    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $sha = [System.Security.Cryptography.SHA256]::Create()
        try {
            return ([System.BitConverter]::ToString($sha.ComputeHash($stream)) -replace '-', '').ToLowerInvariant()
        } finally {
            $sha.Dispose()
        }
    } finally {
        $stream.Dispose()
    }
}

function Save-RemoteFile([string]$Uri, [string]$Path) {
    $ProgressPreference = 'SilentlyContinue'
    $partial = "$Path.part"
    for ($attempt = 1; $attempt -le 3; $attempt++) {
        try {
            if (Test-Path -LiteralPath $partial) { Remove-Item -LiteralPath $partial -Force }
            Write-Output "Downloading ffmpeg 9.0.2 essentials (attempt $attempt): $Uri"
            Invoke-WebRequest -Uri $Uri -OutFile $partial -UseBasicParsing
            Move-Item -LiteralPath $partial -Destination $Path -Force
            return
        } catch {
            if (Test-Path -LiteralPath $partial) { Remove-Item -LiteralPath $partial -Force }
            if ($attempt -eq 3) { throw "下载 ffmpeg 失败: $($_.Exception.Message)" }
            Start-Sleep -Seconds (2 * $attempt)
        }
    }
}

New-Item -ItemType Directory -Force -Path $CacheDir | Out-Null
$zipPath = Join-Path $CacheDir 'ffmpeg-9.0.2-essentials_build.zip'

if (-not (Test-Path -LiteralPath $zipPath -PathType Leaf)) {
    Save-RemoteFile -Uri $sourceUrl -Path $zipPath
}

$actualSha256 = Get-FileSha256 $zipPath
if ($actualSha256 -ne $sourceSha256) {
    Remove-Item -LiteralPath $zipPath -Force
    throw "ffmpeg 压缩包 SHA-256 不匹配: 期望 $sourceSha256，实际 $actualSha256（已删除缓存，请重试）"
}

$archive = [System.IO.Compression.ZipFile]::OpenRead($zipPath)
try {
    $binEntry = $null
    $licenseEntry = $null
    foreach ($entry in $archive.Entries) {
        $name = $entry.FullName.Replace('\', '/')
        if ($name.EndsWith('/' + $expectedBin)) { $binEntry = $entry }
        elseif ($name.EndsWith('/' + $expectedLicense)) { $licenseEntry = $entry }
    }
    if ($null -eq $binEntry) { throw "ffmpeg 压缩包中未找到 $expectedBin" }
    if ($null -eq $licenseEntry) { throw "ffmpeg 压缩包中未找到 $expectedLicense" }

    New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
    $binPath = Join-Path $OutputDir 'ffmpeg.exe'
    $licensePath = Join-Path $OutputDir 'LICENSE.txt'
    [System.IO.Compression.ZipFileExtensions]::ExtractToFile($binEntry, $binPath, $true)
    [System.IO.Compression.ZipFileExtensions]::ExtractToFile($licenseEntry, $licensePath, $true)
} finally {
    $archive.Dispose()
}

$size = (Get-Item -LiteralPath $binPath).Length
if ($size -lt 50MB) {
    throw "解出的 ffmpeg.exe 过小（$size 字节），疑似下载或解包不完整"
}

Write-Output "ffmpeg ready: $binPath ($size bytes)"
Write-Output "license ready: $licensePath"
