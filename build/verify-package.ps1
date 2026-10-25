# 校验 Windows 发布包条目集合，并检查随包 ffmpeg、许可与第三方声明。
param(
    [Parameter(Mandatory = $true)]
    [string]$ZipPath
)

$ErrorActionPreference = 'Stop'
$expected = @(
    'README.md',
    'wx-mini-video.exe',
    'wx-mini-video.yaml',
    'ffmpeg/ffmpeg.exe',
    'ffmpeg/LICENSE.txt',
    'THIRD-PARTY-NOTICES.md'
)
$ffmpegMinBytes = 20MB

if (-not (Test-Path -LiteralPath $ZipPath -PathType Leaf)) {
    throw "Package not found: $ZipPath"
}

function Read-ZipEntryText([System.IO.Compression.ZipArchiveEntry]$Entry) {
    $reader = New-Object System.IO.StreamReader($Entry.Open())
    try {
        return $reader.ReadToEnd()
    } finally {
        $reader.Dispose()
    }
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $ZipPath))
try {
    $entries = @{}
    foreach ($entry in $archive.Entries) {
        $entries[$entry.FullName.Replace('\', '/').TrimEnd('/')] = $entry
    }
    $actual = @($entries.Keys | Sort-Object -Unique)

    $expectedSorted = @($expected | Sort-Object -Unique)
    if ($actual.Count -ne $expectedSorted.Count -or (Compare-Object -ReferenceObject $expectedSorted -DifferenceObject $actual)) {
        throw "Unexpected package contents. Expected: $($expectedSorted -join ', '); actual: $($actual -join ', ')"
    }

    $ffmpeg = $entries['ffmpeg/ffmpeg.exe']
    if ($ffmpeg.Length -lt $ffmpegMinBytes) {
        throw "ffmpeg/ffmpeg.exe 过小: $($ffmpeg.Length) 字节，疑似占位或损坏文件"
    }
    $header = New-Object byte[] 2
    $stream = $ffmpeg.Open()
    try {
        if ($stream.Read($header, 0, 2) -ne 2 -or $header[0] -ne 0x4D -or $header[1] -ne 0x5A) {
            throw "ffmpeg/ffmpeg.exe 不是 PE 可执行文件（缺少 MZ 头）"
        }
    } finally {
        $stream.Dispose()
    }

    $license = Read-ZipEntryText $entries['ffmpeg/LICENSE.txt']
    if ($license -notmatch 'GNU GENERAL PUBLIC LICENSE' -or $license -notmatch 'Version 3') {
        throw "ffmpeg/LICENSE.txt 不是 GPLv3 全文"
    }
    if ($license.Length -lt 30000) {
        throw "ffmpeg/LICENSE.txt 过短: $($license.Length) 字符"
    }

    $notices = Read-ZipEntryText $entries['THIRD-PARTY-NOTICES.md']
    foreach ($required in @('ffmpeg', 'GPLv3', 'Corresponding Source')) {
        if ($notices -notmatch [regex]::Escape($required)) {
            throw "THIRD-PARTY-NOTICES.md 缺少 '$required' 说明"
        }
    }

    Write-Output "Package contents verified: $ZipPath"
} finally {
    $archive.Dispose()
}
