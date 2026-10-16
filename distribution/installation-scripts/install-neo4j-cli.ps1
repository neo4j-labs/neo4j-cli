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

# -- TLS -- PowerShell 5.1 defaults to TLS 1.0; GitHub requires TLS 1.2+ ------
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# -- Constants ----------------------------------------------------------------
$Repo       = "neo4j-labs/neo4j-cli"
$BinaryName = "neo4j-cli.exe"

# -- Helpers ------------------------------------------------------------------
function Write-Step  { param($msg) Write-Host "->  $msg"   -ForegroundColor Cyan }
function Write-Ok    { param($msg) Write-Host "[OK] $msg"  -ForegroundColor Green }
function Write-Warn  { param($msg) Write-Host "[!]  $msg"  -ForegroundColor Yellow }
function Write-Fatal { param($msg) Write-Host "[X]  $msg"  -ForegroundColor Red; exit 1 }

# -- Detect architecture ------------------------------------------------------
function Get-Arch {
    switch ($Env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "x86_64" }
        "ARM64" { return "arm64"  }
        "x86"   { return "i386"   }
        default { Write-Fatal "Unsupported architecture: $Env:PROCESSOR_ARCHITECTURE" }
    }
}

# -- Resolve latest GitHub release version ------------------------------------
function Get-LatestVersion {
    Write-Step "Resolving latest release version..."
    $url = "https://github.com/$Repo/releases/latest"
    # GitHub redirects /releases/latest -> /releases/tag/vX.Y.Z
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

# -- Verify SHA256 checksum ---------------------------------------------------
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

# -- Add directory to user PATH (persistent) ----------------------------------
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

# -- Agent feedback (opt-in) --------------------------------------------------
# Mirrors the "Agent feedback" section of install-neo4j-cli.sh. Offered only
# when this build ships `neo4j-cli feedback enable` (look for its --accept
# flag). Enables nothing unless a person types y: no environment variable
# answers for them (only NEO4J_CLI_SKIP_FEEDBACK_PROMPT=1 suppresses the
# question), there is no prompt without a terminal, and none under
# DO_NOT_TRACK=1 or with telemetry off. The default answer is No. With no
# terminal (CI), or when a coding agent runs this script even with a terminal,
# it registers the inert tool instead, so the first report still gets a human
# decision, and says how to turn feedback on.
# $Cli is the installed binary; $Interactive says whether a person can answer.
function Invoke-FeedbackSetup {
    param(
        [string] $Cli,
        [bool]   $Interactive
    )
    if ($Env:NEO4J_CLI_SKIP_FEEDBACK_PROMPT -eq "1" -or $Env:DO_NOT_TRACK -eq "1") { return }

    # Native commands that write to stderr must not stop the installer.
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $help = (& $Cli feedback enable --help 2>$null) -join "`n"
        if ($help -notmatch '--accept') { return }

        # `feedback enable` without --accept is the probe: it prints the
        # disclosure and exits 2 when a person could enable feedback, prints
        # nothing when the person's settings switch feedback off (DO_NOT_TRACK=1,
        # telemetry off) or the build has no destination, and exits 4 when it
        # detects a coding agent, because only a person may enable feedback.
        # --rw: enable and register are writes, which the CLI refuses without it
        # when there is no terminal or an agent runs this script.
        $disclosure = (& $Cli feedback enable --rw 2>$null) -join "`n"
        $probeExit = $LASTEXITCODE
        $mode = "none"
        if ($disclosure) {
            $mode = if ($Interactive) { "ask" } else { "register" }
        } elseif ($probeExit -eq 4) {
            $mode = "register"
        }
        if ($mode -eq "none") { return }

        if ($mode -eq "ask") {
            Write-Host ""
            Write-Host $disclosure
            Write-Host ""
            $answer = Read-Host "Let coding agents send short feedback reports about neo4j-cli to Neo4j? [y/N]"
            if ($answer -match '^(y|yes)$') {
                & $Cli feedback enable --accept --rw | Out-Null
                if ($LASTEXITCODE -eq 0) {
                    # Keep the command available while it is behind flag.feedback-command; delete with the flag.
                    & $Cli config set flag.feedback-command true --rw *>$null
                    Write-Ok "Agent feedback enabled. Turn it off any time with: neo4j-cli feedback disable"
                } else {
                    Write-Warn "Could not enable agent feedback. Run '$Cli feedback enable' to try again."
                }
            }
            return
        }

        Write-Host ""
        # `register` exits 0 even when no supported agent was found, so look at
        # the agents it reports.
        $registered = (& $Cli feedback register --rw --format json 2>$null) -join "`n"
        if ($registered -match '"claude-code"|"codex"') {
            Write-Host "Agent feedback is off. Registered the feedback tool for coding agents (it appears in your next Claude Code or Codex session): it sends nothing, and the first time an agent reports a problem with neo4j-cli you are asked to accept or decline. Remove it with: $Cli feedback disable"
        } elseif ($registered -match 'registration failed') {
            Write-Host "Agent feedback is off. A coding agent was found, but registering the feedback tool failed. Run '$Cli feedback register' to see why."
            Write-Host "A person can turn feedback on with: $Cli feedback enable"
        } else {
            Write-Host "Agent feedback is off. The feedback tool was not registered: no supported coding agent was found (Claude Code and Codex are supported)."
            Write-Host "A person can turn feedback on with: $Cli feedback enable"
        }
    } catch {
        # A binary that cannot start (blocked, quarantined, wrong architecture)
        # or a host that cannot prompt must never fail the install itself.
        Write-Warn "Skipped agent feedback setup: $($_.Exception.Message)"
    } finally {
        $ErrorActionPreference = $previous
    }
}

# Test-NonInteractiveHost reports whether PowerShell was started with
# -NonInteractive, where Read-Host throws even in a real console.
function Test-NonInteractiveHost {
    return [bool]([Environment]::GetCommandLineArgs() | Where-Object { $_ -match '^-noni' })
}

# -- Main ---------------------------------------------------------------------

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

# -- Temporary working directory ----------------------------------------------
$TmpDir = Join-Path $Env:TEMP "neo4j-cli-install-$(New-Guid)"
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    # -- Download archive -----------------------------------------------------
    $ArchivePath  = Join-Path $TmpDir $ArchiveName
    $ChecksumPath = Join-Path $TmpDir $ChecksumFile

    Write-Step "Downloading $ArchiveName ..."
    Invoke-WebRequest -Uri "$BaseUrl/$ArchiveName"  -OutFile $ArchivePath  -UseBasicParsing

    Write-Step "Downloading checksums..."
    Invoke-WebRequest -Uri "$BaseUrl/$ChecksumFile" -OutFile $ChecksumPath -UseBasicParsing

    # -- Verify checksum ------------------------------------------------------
    Write-Step "Verifying SHA256 checksum..."
    Test-Checksum -FilePath $ArchivePath -ChecksumFile $ChecksumPath -ArchiveName $ArchiveName

    # -- Extract --------------------------------------------------------------
    Write-Step "Extracting archive..."
    $ExtractDir = Join-Path $TmpDir "extracted"
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force

    # Find the binary (may be at root or in a subdirectory)
    $ExtractedBinary = Get-ChildItem -Path $ExtractDir -Recurse -Filter $BinaryName |
        Select-Object -First 1

    if (-not $ExtractedBinary) {
        Write-Fatal "Binary '$BinaryName' not found in the extracted archive."
    }

    # -- Install --------------------------------------------------------------
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }

    $Destination = Join-Path $InstallDir $BinaryName
    Copy-Item -Path $ExtractedBinary.FullName -Destination $Destination -Force

    Write-Ok "Installed -> $Destination"

    # -- Update PATH ----------------------------------------------------------
    Add-ToUserPath -Dir $InstallDir

    # -- Smoke test -----------------------------------------------------------
    Write-Host ""
    try {
        & $Destination --version 2>$null
    } catch {
        Write-Warn "Could not run '$BinaryName --version'. Check the install manually."
    }

    Write-Host ""
    Write-Ok "Done! Run 'neo4j-cli --help' to get started."

    # -- Skill install --------------------------------------------------------
    # Precedence: env-var override > interactive prompt > skip.
    switch ($env:NEO4J_CLI_AUTO_INSTALL_SKILL) {
        '1' { try { & neo4j-cli skill install --rw } catch { } }
        '0' { }
        default {
            if ([Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
                $resp = Read-Host "Install neo4j-cli skill bundle for detected AI agents? [Y/n]"
                if ($resp -notmatch '^(n|no)$') {
                    try { & neo4j-cli skill install --rw } catch { }
                }
            }
        }
    }

    # -- Agent feedback -------------------------------------------------------
    $CanAsk = [Environment]::UserInteractive -and -not [Console]::IsInputRedirected -and -not [Console]::IsOutputRedirected -and -not (Test-NonInteractiveHost)
    Invoke-FeedbackSetup -Cli $Destination -Interactive $CanAsk

    Write-Host ""
    Write-Step "Questions or found a bug? https://github.com/neo4j-labs/neo4j-cli/issues"

} finally {
    # Always clean up temp files
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}