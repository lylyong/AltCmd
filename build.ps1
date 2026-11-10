param(
	[string]$OutputPath
)

$ErrorActionPreference = "Stop"
$projectDir = $PSScriptRoot
$versionPath = Join-Path $projectDir "VERSION"
$version = (Get-Content -LiteralPath $versionPath -Raw).Trim()

if ($version -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') {
	throw "VERSION must contain a numeric major.minor.patch version, for example 1.2.3."
}
foreach ($component in $version.Split('.')) {
	if ([uint32]$component -gt 65535) {
		throw "VERSION components must not exceed 65535."
	}
}

function Sync-DocumentationVersion {
	param(
		[string]$Path,
		[string]$Pattern,
		[string]$Version
	)

	$content = [System.IO.File]::ReadAllText($Path)
	$matches = [regex]::Matches($content, $Pattern)
	if ($matches.Count -ne 1) {
		throw "Expected exactly one version line in $Path, found $($matches.Count)."
	}
	$match = $matches[0]
	$line = $match.Groups['prefix'].Value + $Version + $match.Groups['suffix'].Value
	$updatedContent = $content.Replace($match.Value, $line)
	if ($updatedContent -cne $content) {
		[System.IO.File]::WriteAllText($Path, $updatedContent, (New-Object System.Text.UTF8Encoding($false)))
	}
}

$readmePath = Join-Path $projectDir 'README.md'
Sync-DocumentationVersion -Path $readmePath -Pattern '(?m)^(?<prefix>[^\r\n]*\*\*)\d+\.\d+\.\d+(?<suffix>\*\*[^\r\n]*)$' -Version $version

$readmeEnglishPath = Join-Path $projectDir 'README.en.md'
Sync-DocumentationVersion -Path $readmeEnglishPath -Pattern '(?m)^(?<prefix>[^\r\n]*\*\*)\d+\.\d+\.\d+(?<suffix>\*\*[^\r\n]*)$' -Version $version

$releaseNotesPath = Join-Path $projectDir 'RELEASE_NOTES.md'
if (Test-Path -LiteralPath $releaseNotesPath) {
	foreach ($pattern in @(
		'(?m)^(?<prefix># AltCmd )\d+\.\d+\.\d+(?<suffix>\s*)$',
		'(?m)(?<prefix>AltCmd-)\d+\.\d+\.\d+(?<suffix>\.exe)'
	)) {
		$content = [System.IO.File]::ReadAllText($releaseNotesPath)
		$matches = [regex]::Matches($content, $pattern)
		if ($matches.Count -ne 2) {
			throw "Expected two release-note version references in $releaseNotesPath, found $($matches.Count)."
		}
		$updatedContent = $content
		foreach ($match in $matches) {
			$replacement = $match.Groups['prefix'].Value + $version + $match.Groups['suffix'].Value
			$updatedContent = $updatedContent.Replace($match.Value, $replacement)
		}
		if ($updatedContent -cne $content) {
			[System.IO.File]::WriteAllText($releaseNotesPath, $updatedContent, (New-Object System.Text.UTF8Encoding($false)))
		}
	}
}

$go = Get-Command "go" -ErrorAction Stop
$goVersionOutput = & $go.Source version
if ($LASTEXITCODE -ne 0) {
	throw "Could not determine the Go version."
}
if ($goVersionOutput -notmatch '\bgo(\d+)\.(\d+)') {
	throw "Could not parse Go version: $goVersionOutput"
}
$goMajor = [int]$Matches[1]
$goMinor = [int]$Matches[2]
if ($goMajor -lt 1 -or ($goMajor -eq 1 -and $goMinor -lt 23)) {
	throw "Building AltCmd requires Go 1.23 or later. Found: $goVersionOutput"
}

$goOS = & $go.Source env GOOS
if ($LASTEXITCODE -ne 0) {
	throw "Could not determine the Go target operating system."
}
if ($goOS -ne "windows") {
	throw "Building AltCmd requires GOOS=windows. Current target: $goOS"
}
$goArch = & $go.Source env GOARCH
if ($LASTEXITCODE -ne 0) {
	throw "Could not determine the Go target architecture."
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
	$OutputPath = Join-Path $projectDir "AltCmd-$version.exe"
} else {
	$OutputPath = [System.IO.Path]::GetFullPath($OutputPath)
}
$outputDir = Split-Path -Parent $OutputPath
if ($outputDir) {
	New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
}

$resourceId = [guid]::NewGuid().ToString('N')
$resourceBase = Join-Path $projectDir "AltCmd-resource-$resourceId"
$resourceObject = $resourceBase + "_windows_" + $goArch + ".syso"
$fileVersion = $version + '.0'
$iconPath = Join-Path $projectDir "assets\altcmd.ico"

try {
	Push-Location $projectDir
	try {
		$resourceArgs = @(
			'run'
			'github.com/tc-hib/go-winres@v0.3.3'
			'simply'
			('--arch=' + $goArch)
			('--out=' + $resourceBase)
			'--manifest=none'
			('--file-version=' + $fileVersion)
			('--product-version=' + $fileVersion)
			'--file-description=AltCmd'
			'--product-name=AltCmd'
			'--original-filename=AltCmd.exe'
			('--icon=' + $iconPath)
		)
		& $go.Source @resourceArgs
		if ($LASTEXITCODE -ne 0) {
			throw "Generating Windows resources failed with exit code $LASTEXITCODE."
		}

		& $go.Source build -ldflags '-H=windowsgui' -buildvcs=false -o $OutputPath .
		if ($LASTEXITCODE -ne 0) {
			throw "Go build failed with exit code $LASTEXITCODE."
		}
	} finally {
		Pop-Location
	}
	Write-Output ('Built {0} (version {1}).' -f $OutputPath, $version)
} finally {
	if (Test-Path -LiteralPath $resourceObject) {
		Remove-Item -LiteralPath $resourceObject -Force
	}
}
