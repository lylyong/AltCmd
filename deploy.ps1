param(
	[switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$installDir = Join-Path $env:LOCALAPPDATA "Programs\AltCmd"
$installExe = Join-Path $installDir "AltCmd.exe"
$configPath = Join-Path $env:APPDATA "AltCmd\config.json"
$runKeyPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"

function Set-AltCmdAutoStart {
	param([bool]$Enabled)

	if (-not (Test-Path -LiteralPath $configPath)) {
		if (-not $Enabled) {
			return
		}
		$config = [pscustomobject]@{}
	} else {
		$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
	}

	if ($null -eq $config -or $config -isnot [System.Management.Automation.PSCustomObject]) {
		throw "Invalid AltCmd configuration: $configPath"
	}

	$autoStartProperty = $config.PSObject.Properties["auto_start"]
	if ($null -eq $autoStartProperty) {
		$config | Add-Member -NotePropertyName "auto_start" -NotePropertyValue $Enabled
	} else {
		$config.auto_start = $Enabled
	}

	$configDir = Split-Path -Parent $configPath
	New-Item -ItemType Directory -Path $configDir -Force | Out-Null
	$tempConfigPath = "$configPath.$([guid]::NewGuid().ToString('N')).tmp"
	try {
		$json = ConvertTo-Json -InputObject $config -Depth 100
		$encoding = New-Object System.Text.UTF8Encoding($false)
		[System.IO.File]::WriteAllText($tempConfigPath, $json, $encoding)
		Move-Item -LiteralPath $tempConfigPath -Destination $configPath -Force
	} finally {
		if (Test-Path -LiteralPath $tempConfigPath) {
			Remove-Item -LiteralPath $tempConfigPath -Force
		}
	}
}

function Remove-AltCmdAutoStart {
	if (Test-Path -LiteralPath $runKeyPath) {
		$runKey = Get-ItemProperty -LiteralPath $runKeyPath
		if ($null -ne $runKey.PSObject.Properties["AltCmd"]) {
			Remove-ItemProperty -LiteralPath $runKeyPath -Name "AltCmd"
		}
	}
}

function Stop-InstalledAltCmd {
	$expectedPath = [System.IO.Path]::GetFullPath($installExe)
	$runningProcesses = Get-CimInstance -ClassName Win32_Process -Filter "Name = 'AltCmd.exe'" |
		Where-Object {
			$_.ExecutablePath -and [string]::Equals(
				[System.IO.Path]::GetFullPath($_.ExecutablePath),
				$expectedPath,
				[System.StringComparison]::OrdinalIgnoreCase
			)
		}

	foreach ($runningProcess in $runningProcesses) {
		Stop-Process -Id $runningProcess.ProcessId -Force -ErrorAction Stop
		Wait-Process -Id $runningProcess.ProcessId -Timeout 15 -ErrorAction SilentlyContinue

		$stillRunning = Get-CimInstance -ClassName Win32_Process -Filter "ProcessId = $($runningProcess.ProcessId)"
		if ($stillRunning) {
			throw "Could not stop the running AltCmd process (PID $($runningProcess.ProcessId))."
		}
		Write-Output "Stopped running AltCmd process (PID $($runningProcess.ProcessId))."
	}
}

if ($Uninstall) {
	Stop-InstalledAltCmd
	Set-AltCmdAutoStart -Enabled $false
	Remove-AltCmdAutoStart
	if (Test-Path -LiteralPath $installExe) {
		Remove-Item -LiteralPath $installExe -Force
	}
	Write-Output "AltCmd startup entry and installed executable removed. User configuration was kept."
	exit 0
}

$os = Get-CimInstance -ClassName Win32_OperatingSystem
$osVersion = [version]$os.Version
Write-Output ("Detected OS: {0} (version {1}, build {2})" -f $os.Caption, $os.Version, $os.BuildNumber)

if ($osVersion.Major -lt 10) {
	throw "AltCmd is built with Go 1.23 and requires Windows 10 / Windows Server 2016 or later."
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

$projectDir = $PSScriptRoot
$buildOutput = Join-Path $env:TEMP ("AltCmd-build-{0}.exe" -f [guid]::NewGuid().ToString("N"))
$stagedExe = Join-Path $installDir ("AltCmd-{0}.tmp" -f [guid]::NewGuid().ToString("N"))

try {
	Push-Location $projectDir
	try {
		& $go.Source build -ldflags "-H=windowsgui" -buildvcs=false -o $buildOutput .
		if ($LASTEXITCODE -ne 0) {
			throw "Go build failed with exit code $LASTEXITCODE."
		}
	} finally {
		Pop-Location
	}

	New-Item -ItemType Directory -Path $installDir -Force | Out-Null
	Copy-Item -LiteralPath $buildOutput -Destination $stagedExe
	Stop-InstalledAltCmd
	Move-Item -LiteralPath $stagedExe -Destination $installExe -Force

	Set-AltCmdAutoStart -Enabled $true
	New-Item -Path $runKeyPath -Force | Out-Null
	New-ItemProperty -Path $runKeyPath -Name "AltCmd" -Value ('"{0}"' -f $installExe) -PropertyType String -Force | Out-Null

	Start-Process -FilePath $installExe
	Write-Output "AltCmd built with $goVersionOutput, installed for the current user, configured to start at sign-in, and launched."
} finally {
	foreach ($tempPath in @($buildOutput, $stagedExe)) {
		if (Test-Path -LiteralPath $tempPath) {
			Remove-Item -LiteralPath $tempPath -Force
		}
	}
}
