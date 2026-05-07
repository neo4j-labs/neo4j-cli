#Requires -Version 5.1
<#
.SYNOPSIS
    Installs the neo4j-cli binary on Windows.
.DESCRIPTION
    Detects OS architecture, downloads the correct release archive from
    https://github.com/neo4j-labs/neo4j-cli, verifies the SHA256 checksum,
    extracts the binary, and adds it to your PATH.
.PARAMETER Version
    Specific release version to install (e.g. "v0.1.0-alpha.6").
    Defaults to the latest release.
.PARAMETER InstallDir
    Directory to install the binary into.
    Defaults to "$Env:LOCALAPPDATA\neo4j-cli" (no admin rights required).
    Use "C:\Program Files\neo4j-cli" for a system-wide install (requires admin).
.EXAMPLE
    .\install-neo4j-cli.ps1
.EXAMPLE
    .\install-neo4j-cli.ps1 -Version v0.1.0-alpha.6 -InstallDir C:\tools
#>
[CmdletBinding()]
param(
    [string] $Version    = "",
    [string] $InstallDir = "$Env:LOCALAPPDATA\neo4j-cli"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── TLS — PowerShell 5.1 defaults to TLS 1.0; GitHub requires TLS 1.2+ ───────
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# ── Constants ─────────────────────────────────────────────────────────────────
$Repo       = "neo4j-labs/neo4j-cli"
$BinaryName = "neo4j-cli.exe"

# ── Helpers ───────────────────────────────────────────────────────────────────
function Write-Step  { param($msg) Write-Host "▶  $msg" -ForegroundColor Cyan }
function Write-Ok    { param($msg) Write-Host "✔  $msg" -ForegroundColor Green }
function Write-Warn  { param($msg) Write-Host "⚠  $msg" -ForegroundColor Yellow }
function Write-Fatal { param($msg) Write-Host "✖  $msg" -ForegroundColor Red; exit 1 }

# ── Detect architecture ───────────────────────────────────────────────────────
function Get-Arch {
    switch ($Env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64"  }
        "x86"   { return "i386"   }
        default { Write-Fatal "Unsupported architecture: $Env:PROCESSOR_ARCHITECTURE" }
    }
}

# ── Resolve latest GitHub release version ────────────────────────────────────
function Get-LatestVersion {
    Write-Step "Resolving latest release version..."
    $url = "https://github.com/$Repo/releases/latest"
    # GitHub redirects /releases/latest → /releases/tag/vX.Y.Z
    $response = Invoke-WebRequest -Uri $url -MaximumRedirection 0 -UseBasicParsing -ErrorAction SilentlyContinue
    if ($response.StatusCode -eq 302) {
        $location = $response.Headers["Location"]
    } else {
        # Let it follow the redirect and grab the final URL
        $response  = Invoke-WebRequest -Uri $url -UseBasicParsing
        $location  = $response.BaseResponse.ResponseUri.AbsoluteUri
    }
    if ($location -match '(v[\d]+\.[\d]+\.[\d]+[^/"]*)$') {
        return $Matches[1]
    }
    Write-Fatal "Could not determine the latest release version. Set -Version explicitly."
}

# ── Verify SHA256 checksum ────────────────────────────────────────────────────
function Test-Checksum {
    param(
        [string] $FilePath,
        [string] $ChecksumFile,
        [string] $ArchiveName
    )
    # Parse the checksums file for our archive's expected hash
    $expected = $null
    foreach ($line in (Get-Content $ChecksumFile)) {
        # Format: <hash>  <filename>
        if ($line -match '^([a-fA-F0-9]{64})\s+' + [regex]::Escape($ArchiveName)) {
            $expected = $Matches[1].ToUpper()
            break
        }
    }
    if (-not $expected) {
        Write-Fatal "Could not find checksum for '$ArchiveName' in checksums file."
    }

    $actual = (Get-FileHash -Path $FilePath -Algorithm SHA256).Hash.ToUpper()

    if ($actual -ne $expected) {
        Write-Fatal "Checksum mismatch!`n  Expected : $expected`n  Got      : $actual"
    }
    Write-Ok "Checksum verified."
}

# ── Add directory to user PATH (persistent) ───────────────────────────────────
function Add-ToUserPath {
    param([string] $Dir)
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    if ($currentPath -split ";" -contains $Dir) {
        return  # Already present
    }
    [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$Dir", "User")
    # Also update current session
    $Env:PATH = "$Env:PATH;$Dir"
    Write-Ok "Added '$Dir' to your user PATH."
    Write-Warn "Restart your terminal (or run: `$Env:PATH += ';$Dir'`) to use it now."
}

# ── Main ──────────────────────────────────────────────────────────────────────

$Arch = Get-Arch

if (-not $Version) {
    $Version = Get-LatestVersion
}

# GoReleaser filenames use bare version (no leading 'v')
$VersionNum  = $Version.TrimStart("v")
$ArchiveName = "neo4j-cli_${VersionNum}_Windows_${Arch}.zip"
$ChecksumFile = "neo4j-cli_${VersionNum}_checksums.txt"
$BaseUrl     = "https://github.com/$Repo/releases/download/$Version"

Write-Host ""
Write-Host "  neo4j-cli installer" -ForegroundColor White
Write-Host "  Version : $Version  |  Arch : $Arch" -ForegroundColor DarkGray
Write-Host ""

# ── Temporary working directory ───────────────────────────────────────────────
$TmpDir = Join-Path $Env:TEMP "neo4j-cli-install-$(New-Guid)"
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    # ── Download archive ──────────────────────────────────────────────────────
    $ArchivePath  = Join-Path $TmpDir $ArchiveName
    $ChecksumPath = Join-Path $TmpDir $ChecksumFile

    Write-Step "Downloading $ArchiveName ..."
    Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName"  -OutFile $ArchivePath  -UseBasicParsing

    Write-Step "Downloading checksums..."
    Invoke-WebRequest -Uri "$BaseUrl/$ChecksumFile" -OutFile $ChecksumPath -UseBasicParsing

    # ── Verify checksum ───────────────────────────────────────────────────────
    Write-Step "Verifying SHA256 checksum..."
    Test-Checksum -FilePath $ArchivePath -ChecksumFile $ChecksumPath -ArchiveName $ArchiveName

    # ── Extract ───────────────────────────────────────────────────────────────
    Write-Step "Extracting archive..."
    $ExtractDir = Join-Path $TmpDir "extracted"
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force

    # Find the binary (may be at root or in a subdirectory)
    $ExtractedBinary = Get-ChildItem -Path $ExtractDir -Recurse -Filter $BinaryName |
        Select-Object -First 1

    if (-not $ExtractedBinary) {
        Write-Fatal "Binary '$BinaryName' not found in the extracted archive."
    }

    # ── Install ───────────────────────────────────────────────────────────────
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    $Destination = Join-Path $InstallDir $BinaryName
    Copy-Item -Path $ExtractedBinary.FullName -Destination $Destination -Force

    Write-Ok "Installed → $Destination"

    # ── Update PATH ───────────────────────────────────────────────────────────
    Add-ToUserPath -Dir $InstallDir

    # ── Smoke test ────────────────────────────────────────────────────────────
    Write-Host ""
    try {
        & $Destination --version 2>$null
    } catch {
        Write-Warn "Could not run '$BinaryName --version'. Check the install manually."
    }

    Write-Host ""
    Write-Ok "Done! Run 'aura-cli --help' to get started."

} finally {
    # Always clean up temp files
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}