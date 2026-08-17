Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$srcPath = Join-Path $repoRoot "build\appicon.png"

if (-not (Test-Path -LiteralPath $srcPath)) {
    throw "Missing source image: $srcPath"
}

$srcBmp = New-Object System.Drawing.Bitmap($srcPath)
$sizes = @(256, 128, 64, 48, 32, 16)

# Render each frame as a high-quality PNG
$pngFrames = @()
foreach ($s in $sizes) {
    $bmp = New-Object System.Drawing.Bitmap($s, $s)
    $bmp.SetResolution(96, 96)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
    $g.DrawImage($srcBmp, 0, 0, $s, $s)
    $g.Dispose()
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    $pngFrames += , $ms.ToArray()
    $ms.Dispose()
    Write-Host "rendered ${s}x${s}"
}
$srcBmp.Dispose()

# Assemble ICO (Vista+ PNG-compressed entries)
function Write-Ico([string]$outPath) {
    $count = $sizes.Count
    $offset = 6 + 16 * $count
    $ms = New-Object System.IO.MemoryStream
    $bw = New-Object System.IO.BinaryWriter($ms)
    $bw.Write([uint16]0)   # reserved
    $bw.Write([uint16]1)   # type: icon
    $bw.Write([uint16]$count)
    for ($i = 0; $i -lt $count; $i++) {
        $s = $sizes[$i]
        $data = $pngFrames[$i]
        $dim = if ($s -ge 256) { 0 } else { $s }
        $bw.Write([byte]$dim)     # width
        $bw.Write([byte]$dim)     # height
        $bw.Write([byte]0)        # palette colors
        $bw.Write([byte]0)        # reserved
        $bw.Write([uint16]1)      # planes
        $bw.Write([uint16]32)     # bits per pixel
        $bw.Write([uint32]$data.Length)
        $bw.Write([uint32]$offset)
        $offset += $data.Length
    }
    foreach ($d in $pngFrames) { $bw.Write($d) }
    $bw.Flush()
    [System.IO.File]::WriteAllBytes($outPath, $ms.ToArray())
    $bw.Dispose()
    $ms.Dispose()
    Write-Host "OK $outPath ($((Get-Item $outPath).Length) bytes)"
}

Write-Ico (Join-Path $repoRoot "build\windows\icon.ico")
Write-Ico (Join-Path $repoRoot "backend\internal\tray\icon.ico")
Write-Host "DONE"
