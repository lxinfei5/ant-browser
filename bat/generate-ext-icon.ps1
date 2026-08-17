Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$extDir = "C:\Agent\Output\ForceFont-Extension"
if (-not (Test-Path $extDir)) { New-Item -ItemType Directory -Path $extDir -Force | Out-Null }

# Teal gradient matching app identity
$c0 = [System.Drawing.ColorTranslator]::FromHtml("#0F766E")
$c1 = [System.Drawing.ColorTranslator]::FromHtml("#14B8A6")
$c2 = [System.Drawing.ColorTranslator]::FromHtml("#2DD4BF")

function New-RoundedPath([float]$x, [float]$y, [float]$w, [float]$h, [float]$r) {
    $p = New-Object System.Drawing.Drawing2D.GraphicsPath
    $d = $r * 2
    $p.AddArc($x, $y, $d, $d, 180, 90)
    $p.AddArc($x + $w - $d, $y, $d, $d, 270, 90)
    $p.AddArc($x + $w - $d, $y + $h - $d, $d, $d, 0, 90)
    $p.AddArc($x, $y + $h - $d, $d, $d, 90, 90)
    $p.CloseFigure()
    return $p
}

function Render-Icon([int]$size, [string]$outPath) {
    $bmp = New-Object System.Drawing.Bitmap($size, $size)
    $bmp.SetResolution(96, 96)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
    $g.Clear([System.Drawing.Color]::Transparent)

    $scale = $size / 128.0
    function S([float]$v) { return $v * $scale }

    # Rounded-square badge
    $rect = New-Object System.Drawing.RectangleF((S 8), (S 8), (S 112), (S 112))
    $r = S 28
    $path = New-RoundedPath -x $rect.X -y $rect.Y -w $rect.Width -h $rect.Height -r $r
    $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush(
        $rect,
        $c0, $c2,
        [System.Drawing.Drawing2D.LinearGradientMode]::ForwardDiagonal)
    $g.FillPath($brush, $path)

    # Subtle top-left highlight (glass)
    $hlPath = New-Object System.Drawing.Drawing2D.GraphicsPath
    $hlPath.AddEllipse((S -20), (S -36), (S 160), (S 110))
    $hlBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(40,255,255,255))
    $g.SetClip($path)
    $g.FillPath($hlBrush, $hlPath)
    $g.ResetClip()

    # "A" glyph mark (font identity)
    $fontFamily = New-Object System.Drawing.FontFamily("Segoe UI")
    $font = New-Object System.Drawing.Font($fontFamily, (S 66), [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Pixel)
    $textBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::White)
    $sf = New-Object System.Drawing.StringFormat
    $sf.Alignment = [System.Drawing.StringAlignment]::Center
    $sf.LineAlignment = [System.Drawing.StringAlignment]::Center
    $textRect = New-Object System.Drawing.RectangleF(0, (S 2), $size, $size)
    $g.DrawString("A", $font, $textBrush, $textRect, $sf)

    $g.Dispose()
    $bmp.Save($outPath, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    Write-Host "OK $outPath ($size x $size)"
}

Render-Icon -size 128 -outPath (Join-Path $extDir "icon128.png")
Render-Icon -size 48  -outPath (Join-Path $extDir "icon48.png")
Render-Icon -size 32  -outPath (Join-Path $extDir "icon32.png")
Render-Icon -size 16  -outPath (Join-Path $extDir "icon16.png")

Write-Host "DONE"
