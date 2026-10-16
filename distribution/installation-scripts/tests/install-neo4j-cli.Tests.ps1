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

# Agent feedback. The installer's own run (above) cannot reach the feedback
# step with a working binary: the archive stub installs an empty placeholder
# file. So the end-to-end case below checks only that a binary that cannot
# start never fails the install, and the logic of Invoke-FeedbackSetup is
# driven directly, pulled out of the installer with the PowerShell parser, with
# a stub CLI passed as -Cli.
Describe "install-neo4j-cli.ps1 agent feedback: failure isolation" {

    It "a binary that cannot start skips feedback setup and the install still succeeds" {
        $result = Invoke-Installer

        $result.ExitCode | Should -Be 0
        $result.Output | Should -Match "Skipped agent feedback setup"
        $result.Output | Should -Match "Questions or found a bug"
    }
}

Describe "Invoke-FeedbackSetup" {

    BeforeAll {
        $ast = [System.Management.Automation.Language.Parser]::ParseFile($Script:InstallerPath, [ref]$null, [ref]$null)
        foreach ($name in "Write-Step", "Write-Ok", "Write-Warn", "Invoke-FeedbackSetup") {
            $fn = $ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name }, $true) |
                Select-Object -First 1
            $fn | Should -Not -BeNullOrEmpty -Because "function $name must exist in the installer"
            . ([scriptblock]::Create($fn.Extent.Text))
        }

        # The answer a "person" types; Read-Host is replaced so no console is needed.
        function Read-Host { param($Prompt) return $global:FeedbackTestAnswer }

        $Script:FbDir   = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
        $Script:FbCalls = Join-Path $Script:FbDir "calls.txt"
        $null = New-Item -ItemType Directory -Path $Script:FbDir -Force

        # A stub CLI that records its arguments: `feedback enable --help` lists
        # --accept unless STUB_SUPPORTED=0, `feedback enable` prints a
        # disclosure and exits 2 (or, with STUB_ENABLE_REFUSE set, prints
        # nothing and exits with it: 4 = agent detected, 2 = switched off),
        # `feedback enable --accept` exits STUB_ENABLE_EXIT, `feedback
        # register` prints STUB_REGISTER_OUT.
        if ($IsWindows -or $env:OS -eq "Windows_NT") {
            $Script:FbStub = Join-Path $Script:FbDir "neo4j-cli.cmd"
            $stub = @'
@echo off
pwsh -NoProfile -NonInteractive -Command "Add-Content -Path '%STUB_CALLS%' -Value ('%*')"
if "%1 %2 %3"=="feedback enable --help" (
  if not "%STUB_SUPPORTED%"=="0" echo       --accept   Accept the disclosure
  exit 0
)
if "%1 %2 %3"=="feedback enable --accept" exit %STUB_ENABLE_EXIT%
if "%1 %2"=="feedback enable" if not "%STUB_ENABLE_REFUSE%"=="" exit %STUB_ENABLE_REFUSE%
if "%1 %2"=="feedback enable" (
  echo Enabling neo4j-cli feedback. Reports contain only: category, expected, observed.
  exit 2
)
if "%1 %2"=="feedback register" echo %STUB_REGISTER_OUT%
exit 0
'@
            Set-Content -Path $Script:FbStub -Value $stub -Encoding ASCII
        } else {
            $Script:FbStub = Join-Path $Script:FbDir "neo4j-cli"
            $stub = @'
#!/bin/sh
echo "$*" >> "$STUB_CALLS"
case "$1 $2 $3" in
  "feedback enable --help") [ "$STUB_SUPPORTED" = "0" ] || echo "      --accept   Accept the disclosure"; exit 0 ;;
  "feedback enable --accept") exit "$STUB_ENABLE_EXIT" ;;
esac
case "$1 $2" in
  "feedback enable") [ -n "$STUB_ENABLE_REFUSE" ] && exit "$STUB_ENABLE_REFUSE"; echo "Enabling neo4j-cli feedback. Reports contain only: category, expected, observed."; exit 2 ;;
  "feedback register") printf '%s\n' "$STUB_REGISTER_OUT"; exit 0 ;;
esac
exit 0
'@
            # This file has CRLF line endings; a shell script must not.
            Set-Content -Path $Script:FbStub -Value ($stub -replace "`r`n", "`n") -NoNewline
            chmod +x $Script:FbStub
        }

        function Invoke-Feedback {
            param(
                [bool]      $Interactive,
                [hashtable] $Env = @{},
                [string]    $Answer = ""
            )
            foreach ($k in "NEO4J_CLI_SKIP_FEEDBACK_PROMPT", "DO_NOT_TRACK", "NEO4J_CLI_AUTO_ENABLE_FEEDBACK", "STUB_SUPPORTED", "STUB_ENABLE_REFUSE") {
                Remove-Item "Env:$k" -ErrorAction SilentlyContinue
            }
            $env:STUB_CALLS = $Script:FbCalls
            $env:STUB_ENABLE_EXIT = "0"
            $env:STUB_REGISTER_OUT = '{"registered_agents": ["claude-code"]}'
            foreach ($k in $Env.Keys) { Set-Item "Env:$k" $Env[$k] }
            Set-Content -Path $Script:FbCalls -Value ""
            $global:FeedbackTestAnswer = $Answer
            $out = Invoke-FeedbackSetup -Cli $Script:FbStub -Interactive $Interactive *>&1 | Out-String
            return @{ Output = $out; Calls = (Get-Content $Script:FbCalls -Raw) }
        }
    }

    AfterAll {
        Remove-Item -Recurse -Force $Script:FbDir -ErrorAction SilentlyContinue
        Remove-Item Env:STUB_CALLS, Env:STUB_ENABLE_EXIT, Env:STUB_REGISTER_OUT, Env:STUB_SUPPORTED, Env:STUB_ENABLE_REFUSE -ErrorAction SilentlyContinue
    }

    It "no terminal: registers the inert tool and enables nothing" {
        $r = Invoke-Feedback -Interactive $false
        $r.Calls | Should -Match "feedback register"
        $r.Calls | Should -Not -Match "enable --accept"
        $r.Output | Should -Match "Registered the feedback tool"
    }

    It "no terminal and no supported agent found: says so" {
        $r = Invoke-Feedback -Interactive $false -Env @{ STUB_REGISTER_OUT = '{"registered_agents": []}' }
        $r.Output | Should -Match "no supported coding agent was found"
        $r.Calls | Should -Not -Match "enable --accept"
    }

    It "no terminal and registration failed: says so" {
        $r = Invoke-Feedback -Interactive $false -Env @{ STUB_REGISTER_OUT = '{"notes": ["Claude Code registration failed (x)"]}' }
        $r.Output | Should -Match "registering the feedback tool failed"
    }

    It "build without the feedback command: no registration, no hint" {
        $r = Invoke-Feedback -Interactive $false -Env @{ STUB_SUPPORTED = "0" }
        $r.Calls | Should -Not -Match "feedback register"
        $r.Output | Should -Not -Match "Agent feedback is off"
    }

    It "NEO4J_CLI_SKIP_FEEDBACK_PROMPT=1: feedback is never touched" {
        $r = Invoke-Feedback -Interactive $true -Answer "y" -Env @{ NEO4J_CLI_SKIP_FEEDBACK_PROMPT = "1" }
        $r.Calls.Trim() | Should -BeNullOrEmpty
    }

    It "DO_NOT_TRACK=1 with a terminal: never asked, never touched" {
        $r = Invoke-Feedback -Interactive $true -Answer "y" -Env @{ DO_NOT_TRACK = "1" }
        $r.Calls.Trim() | Should -BeNullOrEmpty
    }

    It "no environment variable can enable it" {
        $r = Invoke-Feedback -Interactive $false -Env @{ NEO4J_CLI_AUTO_ENABLE_FEEDBACK = "1" }
        $r.Calls | Should -Not -Match "enable --accept"
    }

    It "terminal, empty answer is No" {
        $r = Invoke-Feedback -Interactive $true -Answer ""
        $r.Output | Should -Match "Enabling neo4j-cli feedback"
        $r.Calls | Should -Not -Match "enable --accept"
    }

    It "terminal, answer n: nothing enabled" {
        $r = Invoke-Feedback -Interactive $true -Answer "n"
        $r.Calls | Should -Not -Match "enable --accept"
    }

    It "terminal, answer y: enabled" {
        $r = Invoke-Feedback -Interactive $true -Answer "y"
        $r.Calls | Should -Match "feedback enable --accept"
        $r.Output | Should -Match "Agent feedback enabled"
    }

    It "terminal, answer y but enable fails: warns and keeps going" {
        $r = Invoke-Feedback -Interactive $true -Answer "y" -Env @{ STUB_ENABLE_EXIT = "1" }
        $r.Output | Should -Match "Could not enable agent feedback"
    }

    It "terminal, but a coding agent is detected: not asked, registers the inert tool" {
        $r = Invoke-Feedback -Interactive $true -Answer "y" -Env @{ STUB_ENABLE_REFUSE = "4" }
        $r.Output | Should -Not -Match "Enabling neo4j-cli feedback"
        $r.Calls | Should -Match "feedback register"
        $r.Calls | Should -Not -Match "enable --accept"
    }

    It "switched off by the person's settings (telemetry off): does nothing, with or without a terminal" {
        foreach ($interactive in $true, $false) {
            $r = Invoke-Feedback -Interactive $interactive -Answer "y" -Env @{ STUB_ENABLE_REFUSE = "2" }
            $r.Calls | Should -Not -Match "feedback register"
            $r.Calls | Should -Not -Match "enable --accept"
        }
    }

    It "a CLI that cannot start: warns instead of failing" {
        $env:STUB_CALLS = $Script:FbCalls
        $out = Invoke-FeedbackSetup -Cli (Join-Path $Script:FbDir "missing-neo4j-cli") -Interactive $false *>&1 | Out-String
        $out | Should -Match "Skipped agent feedback setup"
    }
}
