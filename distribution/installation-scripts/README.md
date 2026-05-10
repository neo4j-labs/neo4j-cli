# neo4j-cli Installer Scripts

Install scripts for the [neo4j-cli](https://github.com/neo4j-labs/neo4j-cli) binary. Each script automatically detects your OS and architecture, downloads the correct release archive, verifies the SHA256 checksum, and places the binary on your PATH.

Two scripts are provided depending on your platform:

| Platform | Script |
|----------|--------|
| macOS / Linux | `install-neo4j-cli.sh` |
| Windows | `install-neo4j-cli.ps1` |

> Note: If you edit the ps1 installer on a Linux like OS, run unix2dos afterwards

---

## macOS / Linux — `install-neo4j-cli.sh`

A POSIX-compatible Bash script requiring only `curl` and `tar`, both of which are available by default on macOS and most Linux distributions.

### Quick start

```bash
bash install-neo4j-cli.sh
```

### Options

The script is controlled via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `VERSION` | _(latest)_ | Pin a specific release, e.g. `v0.1.0-alpha.6` |
| `INSTALL_DIR` | `/usr/local/bin` | Directory to install the binary into |


> Note: You will need to set `VERSION` for pre-release versions 


### Examples

```bash
# Install the latest release
bash install-neo4j-cli.sh

# Pin a specific version
VERSION=v0.1.0-alpha.6 bash install-neo4j-cli.sh

# Install to a directory that doesn't require sudo
INSTALL_DIR=~/.local/bin bash install-neo4j-cli.sh
```

### What it does

1. Detects the OS (`Darwin` / `Linux`) and CPU architecture (`x86_64` / `arm64` / `i386`).
2. Queries the GitHub releases page to resolve the latest version (unless `VERSION` is set).
3. Downloads the matching `.tar.gz` archive and the `checksums.txt` file.
4. Verifies the SHA256 checksum using `sha256sum` if available, otherwise falls back to `shasum -a 256`. Works with GNU coreutils (Linux), Apple's BSD `sha256sum` (macOS Tahoe and later), and Perl `shasum` (older macOS).
5. Extracts the binary, sets it executable, and moves it to `INSTALL_DIR`.
6. If `INSTALL_DIR` is not writable, `sudo` is invoked automatically for that step only.
7. Confirms the install path and prints the binary version.

### Notes

- If `/usr/local/bin` is not on your `PATH`, add it with `export PATH="/usr/local/bin:$PATH"`.
- macOS users may see a Gatekeeper prompt on first run. If so, go to **System Settings → Privacy & Security** and select **Open Anyway**.

---

## Windows — `install-neo4j-cli.ps1`

A PowerShell 5.1+ script that uses only built-in Windows cmdlets — no third-party tools required.

### Quick start

```powershell
.\install-neo4j-cli.ps1
```

> **Execution policy:** If PowerShell blocks the script, run this first:
> ```powershell
> Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned
> ```

### Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `-Version` | _(latest)_ | Pin a specific release, e.g. `v0.1.0-alpha.6` |
| `-InstallDir` | `%LOCALAPPDATA%\neo4j-cli` | Directory to install the binary into |

### Examples

```powershell
# Install the latest release
.\install-neo4j-cli.ps1

# Pin a specific version
.\install-neo4j-cli.ps1 -Version v0.1.0-alpha.6

# Install to a custom directory
.\install-neo4j-cli.ps1 -InstallDir C:\tools

# System-wide install (requires an elevated/admin terminal)
.\install-neo4j-cli.ps1 -InstallDir "C:\Program Files\neo4j-cli"
```

### What it does

1. Detects the CPU architecture from `$Env:PROCESSOR_ARCHITECTURE` (`AMD64` / `ARM64` / `x86`).
2. Queries the GitHub releases page to resolve the latest version (unless `-Version` is set).
3. Downloads the matching `.zip` archive and the `checksums.txt` file.
4. Verifies the SHA256 checksum using the built-in `Get-FileHash` cmdlet.
5. Extracts the `.zip` and copies the binary to `-InstallDir`.
6. Adds `-InstallDir` to the current user's `PATH` permanently (no admin rights needed for the default location).
7. Confirms the install path and prints the binary version.

### Notes

- The default install location (`%LOCALAPPDATA%\neo4j-cli`) does **not** require administrator rights.
- PATH changes take effect in new terminal sessions. To use the binary immediately in the current session, run `$Env:PATH += ";$InstallDir"`.
- For a system-wide install visible to all users, run the script in an **elevated** (Administrator) PowerShell terminal and pass `-InstallDir "C:\Program Files\neo4j-cli"`.

---

## Release asset naming

Both scripts resolve release archives from the [neo4j-labs/neo4j-cli releases page](https://github.com/neo4j-labs/neo4j-cli/releases) using the GoReleaser naming convention:

```
neo4j-cli_{version}_{OS}_{arch}.tar.gz   # macOS / Linux
neo4j-cli_{version}_{OS}_{arch}.zip      # Windows
neo4j-cli_{version}_checksums.txt        # SHA256 checksums for all archives
```

Supported combinations:

| OS | Architectures |
|----|--------------|
| Linux | `x86_64`, `arm64`, `i386` |
| macOS (Darwin) | `x86_64`, `arm64` |
| Windows | `x86_64`, `arm64`, `i386` |