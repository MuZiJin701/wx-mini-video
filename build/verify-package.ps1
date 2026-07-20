param(
    [Parameter(Mandatory = $true)]
    [string]$ZipPath
)

$ErrorActionPreference = 'Stop'
$expected = @(
    'README.md',
    'wx-mini-video.exe',
    'wx-mini-video.yaml'
)

if (-not (Test-Path -LiteralPath $ZipPath -PathType Leaf)) {
    throw "Package not found: $ZipPath"
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $ZipPath))
try {
    $actual = @($archive.Entries | ForEach-Object { $_.FullName.TrimEnd('/') } | Sort-Object -Unique)
} finally {
    $archive.Dispose()
}

$expectedSorted = @($expected | Sort-Object -Unique)
if ($actual.Count -ne $expectedSorted.Count -or (Compare-Object -ReferenceObject $expectedSorted -DifferenceObject $actual)) {
    throw "Unexpected package contents. Expected: $($expectedSorted -join ', '); actual: $($actual -join ', ')"
}

Write-Output "Package contents verified: $ZipPath"
