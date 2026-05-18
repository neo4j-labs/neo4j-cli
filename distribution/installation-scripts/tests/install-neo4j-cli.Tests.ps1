# Copyright (c) "Neo4j"
# Neo4j Sweden AB [https://neo4j.com]
# This file is part of Neo4j.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

<#
.SYNOPSIS
    Pester v5 behavioral tests for the NEO4J_CLI_AUTO_INSTALL_SKILL feature
    in install-neo4j-cli.ps1.

.DESCRIPTION
    Each test runs the install script as a pwsh subprocess with all external
    calls (Invoke-WebRequest, Expand-Archive, Get-FileHash, etc.) stubbed via
    PowerShell function overrides. A stub neo4j-cli.cmd placed in a temp
    directory on PATH records its arguments to a file so assertions can verify
    whether `skill install --rw` was called.

    neo4j-cli.cmd is used (rather than .ps1) because Windows resolves bare
    `& neo4j-cli` calls to .cmd/.bat before .ps1 when searching PATH.

    Local run:
        pwsh -NoProfile -NonInteractive -Command "Invoke-Pester 'distribution/installation-scripts/tests/install-neo4j-cli.Tests.ps1' -Output Detailed"

        # Or from the repo root:
        Invoke-Pester distribution/installation-scripts/tests/

    Requirements:
        Pester v5 (pre-installed on windows-latest GitHub Actions runners;
        install locally with: Install-Module -Name Pester -MinimumVersion 5.0 -Force)
#>

BeforeAll {
    # Absolute path to the installer under test
    $Script:InstallerPath = (Resolve-Path (Join-Path $PSScriptRoot ".." "install-neo4j-cli.ps1")).Path

    # Helper: run the installer in a fresh pwsh subprocess with all heavyweight
    # cmdlets stubbed. Returns a hashtable with:
    #   ExitCode - subprocess exit code
    #   Output   - combined stdout+stderr
    #   Calls    - raw content of the neo4j-cli invocation recording file
    function Invoke-Installer {
        param(
            # Value for NEO4J_CLI_AUTO_INSTALL_SKILL; empty string means "unset"
            [string] $AutoInstallSkill = "",
            # Exit code the neo4j-cli stub returns for "skill" subcommands
            [int]    $StubExitCode     = 0
        )

        # Isolated temp tree for this invocation
        $tmpDir     = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
        $installDir = Join-Path $tmpDir "install"
        $stubsDir   = Join-Path $tmpDir "stubs"
        $callsFile  = Join-Path $tmpDir "calls.txt"
        $null       = New-Item -ItemType Directory -Path $installDir, $stubsDir -Force
        $null       = New-Item -ItemType File      -Path $callsFile             -Force

        # -- neo4j-cli.cmd stub -------------------------------------------------
        # A .cmd file is resolved by `& neo4j-cli` on Windows before .ps1 when
        # the directory is in PATH. The stub appends its arguments to $callsFile
        # (passed via the NEO4J_CALLS_FILE env var) and exits with $StubExitCode
        # when the first argument is "skill".
        #
        # Note: %* expands to all arguments; CMD batch files cannot easily write
        # multi-word argument strings with exact spacing, so we use a small pwsh
        # one-liner embedded in the .cmd to write the args faithfully.
        $stubCmd = @"
@echo off
pwsh -NoProfile -NonInteractive -Command "Add-Content -Path '%NEO4J_CALLS_FILE%' -Value ('%*')"
if "%1"=="skill" exit $StubExitCode
exit 0
"@
        Set-Content -Path (Join-Path $stubsDir "neo4j-cli.cmd") -Value $stubCmd -Encoding ASCII

        # -- Wrapper script -----------------------------------------------------
        # Written to a temp .ps1 file (rather than passed via -Command) to avoid
        # quoting and escaping complexity. The wrapper:
        #   1. Sets/clears the env var under test
        #   2. Prepends the stubs dir to PATH
        #   3. Overrides all cmdlets the installer calls (no real HTTP/disk I/O)
        #   4. Invokes the installer; catches terminating errors so exit code is 0
        $wrapper = @"
Set-StrictMode -Off
`$ErrorActionPreference = 'Continue'

# Env var under test
$(
    if ($AutoInstallSkill -ne "") {
        "`$env:NEO4J_CLI_AUTO_INSTALL_SKILL = '$AutoInstallSkill'"
    } else {
        "Remove-Item -Path Env:NEO4J_CLI_AUTO_INSTALL_SKILL -ErrorAction SilentlyContinue"
    }
)

# Inject stubs dir at the front of PATH so neo4j-cli.cmd is found first
`$env:PATH = '$($stubsDir.Replace('\','\\'))' + [IO.Path]::PathSeparator + `$env:PATH

# Pass the calls file path to the stub via env var
`$env:NEO4J_CALLS_FILE = '$($callsFile.Replace('\','\\'))'

# Stub: no real HTTP calls
function Invoke-WebRequest {
    param([string]`$Uri, [string]`$OutFile, [switch]`$UseBasicParsing,
          [int]`$MaximumRedirection, [string]`$ErrorAction)
    if (`$OutFile) { `$null = New-Item -ItemType File -Path `$OutFile -Force }
}

# Stub: create a placeholder exe in DestinationPath instead of unzipping
function Expand-Archive {
    param([string]`$Path, [string]`$DestinationPath, [switch]`$Force)
    `$null = New-Item -ItemType Directory -Path `$DestinationPath -Force
    `$null = New-Item -ItemType File -Path (Join-Path `$DestinationPath 'neo4j-cli.exe') -Force
}

# Stub: return a deterministic hash so Test-Checksum always passes
function Get-FileHash {
    param([string]`$Path, [string]`$Algorithm)
    return [PSCustomObject]@{ Hash = 'AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD' }
}

# Stub: return a matching checksum line for the checksums file; pass through otherwise
function Get-Content {
    param([string]`$Path)
    if (`$Path -match 'checksums') {
        return 'AABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDDAABBCCDD  neo4j-cli_9.9.9_Windows_x86_64.zip'
    }
    Microsoft.PowerShell.Management\Get-Content -Path `$Path
}

# Stub: skip PATH registry mutation (noisy in tests; current session update is fine)
function Add-ToUserPath { param([string]`$Dir) }

# Fix architecture env var so Get-Arch returns a known value
`$env:PROCESSOR_ARCHITECTURE = 'AMD64'

# Run the installer; suppress any terminating error so exit code reflects the
# installer's own exit rather than an unhandled exception from a stub gap.
try {
    & '$($Script:InstallerPath.Replace('\','\\'))' -Version 'v9.9.9' -InstallDir '$($installDir.Replace('\','\\'))'
} catch {
    Write-Warning "Installer threw: `$_"
}
"@
        $wrapperPath = Join-Path $tmpDir "run-installer.ps1"
        Set-Content -Path $wrapperPath -Value $wrapper -Encoding UTF8

        # -- Run the wrapper in a subprocess ------------------------------------
        $stdoutFile = Join-Path $tmpDir "stdout.txt"
        $stderrFile = Join-Path $tmpDir "stderr.txt"

        $proc = Start-Process -FilePath "pwsh" `
            -ArgumentList @("-NoProfile", "-NonInteractive", "-File", $wrapperPath) `
            -Wait -PassThru -NoNewWindow `
            -RedirectStandardOutput $stdoutFile `
            -RedirectStandardError  $stderrFile

        $stdout = Get-Content $stdoutFile -Raw -ErrorAction SilentlyContinue
        $stderr = Get-Content $stderrFile -Raw -ErrorAction SilentlyContinue
        $calls  = Get-Content $callsFile  -Raw -ErrorAction SilentlyContinue

        # Cleanup
        Remove-Item -Recurse -Force $tmpDir -ErrorAction SilentlyContinue

        return @{
            ExitCode = $proc.ExitCode
            Output   = "$stdout`n$stderr"
            Calls    = $calls
        }
    }
}

Describe "install-neo4j-cli.ps1 auto-skill-install guard" {

    It "NEO4J_CLI_AUTO_INSTALL_SKILL=1: skill install --rw is invoked" {
        $result = Invoke-Installer -AutoInstallSkill "1"

        # The wrapper exits 0 (installer completed without unhandled exception)
        $result.ExitCode | Should -Be 0

        # The stub must have recorded the skill install invocation
        $result.Calls | Should -Not -BeNullOrEmpty
        $result.Calls | Should -Match "skill install --rw"
    }

    It "NEO4J_CLI_AUTO_INSTALL_SKILL unset: skill install is not called" {
        $result = Invoke-Installer -AutoInstallSkill ""

        $result.ExitCode | Should -Be 0

        # Calls file should be empty or not contain skill install
        if ($result.Calls) {
            $result.Calls | Should -Not -Match "skill install"
        }
    }

    It "NEO4J_CLI_AUTO_INSTALL_SKILL=0: skill install is not called" {
        $result = Invoke-Installer -AutoInstallSkill "0"

        $result.ExitCode | Should -Be 0

        if ($result.Calls) {
            $result.Calls | Should -Not -Match "skill install"
        }
    }

    It "NEO4J_CLI_AUTO_INSTALL_SKILL=1 and neo4j-cli exits non-zero: installer still exits 0" {
        $result = Invoke-Installer -AutoInstallSkill "1" -StubExitCode 1

        # Install must complete cleanly even though skill install returned 1
        $result.ExitCode | Should -Be 0

        # skill install was still attempted
        $result.Calls | Should -Not -BeNullOrEmpty
        $result.Calls | Should -Match "skill install --rw"
    }
}
