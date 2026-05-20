Add-Type -AssemblyName System.Drawing
if (-not (Test-Path assets)) { New-Item -ItemType Directory -Path assets }

# 1. Create SVG
$svg = '<svg width="256" height="256" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <linearGradient id="grad1" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" style="stop-color:#2c3e50;stop-opacity:1" />
      <stop offset="100%" style="stop-color:#000000;stop-opacity:1" />
    </linearGradient>
  </defs>
  <rect x="5" y="5" width="90" height="90" rx="15" fill="url(#grad1)" />
  <rect x="15" y="20" width="30" height="25" rx="4" fill="#ecf0f1" />
  <text x="30" y="38" font-family="Arial" font-size="10" font-weight="bold" text-anchor="middle" fill="#2c3e50">Alt</text>
  <rect x="50" y="50" width="35" height="30" rx="5" fill="#e67e22" />
  <text x="67.5" y="71" font-family="Arial" font-size="10" font-weight="bold" text-anchor="middle" fill="white">Ctrl</text>
  <path d="M48 35 L58 45 L48 55" stroke="white" stroke-width="4" fill="none" stroke-linecap="round" stroke-linejoin="round" />
  <line x1="35" y1="45" x2="55" y2="45" stroke="white" stroke-width="4" stroke-linecap="round" />
</svg>'
$svg | Out-File -FilePath assets\altcmd-icon.svg -Encoding utf8

# 2. Create ICO using System.Drawing
$size = 256
$bmp = New-Object System.Drawing.Bitmap($size, $size)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias

# Helper for rounded rect
function Fill-RoundedRectangle($graph, $brush, $x, $y, $w, $h, $r) {
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $path.AddArc($x, $y, $r, $r, 180, 90)
    $path.AddArc($x + $w - $r, $y, $r, $r, 270, 90)
    $path.AddArc($x + $w - $r, $y + $h - $r, $r, $r, 0, 90)
    $path.AddArc($x, $y + $h - $r, $r, $r, 90, 90)
    $path.CloseFigure()
    $graph.FillPath($brush, $path)
}

# Background
$rect = New-Object System.Drawing.Rectangle(0, 0, $size, $size)
$brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush($rect, [System.Drawing.Color]::FromArgb(44, 62, 80), [System.Drawing.Color]::Black, 45)
Fill-RoundedRectangle $g $brush 10 10 236 236 40

# Alt Key
$altBrush = [System.Drawing.Brushes]::WhiteSmoke
Fill-RoundedRectangle $g $altBrush 35 45 85 65 15
$fontAlt = New-Object System.Drawing.Font("Arial", 28, [System.Drawing.FontStyle]::Bold)
$g.DrawString("Alt", $fontAlt, [System.Drawing.Brushes]::DimGray, 42, 58)

# Ctrl Key
$ctrlBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(230, 126, 34))
Fill-RoundedRectangle $g $ctrlBrush 135 145 95 75 15
$fontCtrl = New-Object System.Drawing.Font("Arial", 28, [System.Drawing.FontStyle]::Bold)
$g.DrawString("Ctrl", $fontCtrl, [System.Drawing.Brushes]::White, 140, 160)

# Arrow
$pen = New-Object System.Drawing.Pen([System.Drawing.Color]::White, 12)
$pen.EndCap = [System.Drawing.Drawing2D.LineCap]::ArrowAnchor
$g.DrawLine($pen, 100, 100, 145, 145)

# Save as ICO
$iconHandle = $bmp.GetHicon()
$icon = [System.Drawing.Icon]::FromHandle($iconHandle)
$fs = [System.IO.File]::Create("assets\altcmd.ico")
$icon.Save($fs)
$fs.Close()
$bmp.Dispose()
$g.Dispose()

Write-Output "Done: assets\altcmd-icon.svg and assets\altcmd.ico created."
