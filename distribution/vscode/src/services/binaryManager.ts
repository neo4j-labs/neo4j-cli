import * as vscode from 'vscode'
import * as path    from 'path'
import * as fs      from 'fs/promises'
import * as fsSync  from 'fs'
import * as os      from 'os'
import { execFile }        from 'child_process'
import { promisify }       from 'util'
import { createWriteStream } from 'fs'
import { Readable }        from 'stream'
import { pipeline }        from 'stream/promises'

// Read the target version from package.json so bumping one field is enough.
// tsconfig.json already has resolveJsonModule: true.
import { cliVersion as REQUIRED_CLI_VERSION } from '../../package.json'

const execFileAsync = promisify(execFile)

// ── GitHub coordinates ──────────────────────────────────────────────────────
// Update these if the CLI repo moves.
const GITHUB_OWNER = 'neo4j'
const GITHUB_REPO  = 'neo4j-cli'

// Binary name inside the archive and on disk
const BINARY = os.platform() === 'win32' ? 'neo4j-cli.exe' : 'neo4j-cli'

// ── Error types ─────────────────────────────────────────────────────────────

export class BinaryInstallError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'BinaryInstallError'
  }
}

// ── Platform mapping ─────────────────────────────────────────────────────────
// Maps Node.js platform/arch strings to GoReleaser's naming convention.

function assetName(version: string): string {
  const platformMap: Record<string, string> = { darwin: 'darwin', linux: 'linux',   win32: 'windows' }
  const archMap:     Record<string, string> = { x64:   'amd64',  arm64: 'arm64',   ia32:  '386'     }

  const p = platformMap[os.platform()]
  const a = archMap[os.arch()]

  if (!p || !a) {
    throw new BinaryInstallError(
      `neo4j-cli does not have a pre-built binary for ${os.platform()}/${os.arch()}. ` +
      'Set neo4j.cliPath in settings to point to a compatible binary.'
    )
  }

  // GoReleaser default naming: <name>_<version>_<os>_<arch>.tar.gz / .zip
  const ext = os.platform() === 'win32' ? 'zip' : 'tar.gz'
  return `neo4j-cli_${version}_${p}_${a}.${ext}`
}

function releaseUrl(version: string): string {
  return [
    `https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}`,
    `releases/download/v${version}`,
    assetName(version),
  ].join('/')
}

// ── BinaryManager ────────────────────────────────────────────────────────────

export class BinaryManager {
  /** The version this extension build expects. Comes from package.json#cliVersion. */
  static readonly requiredVersion: string = REQUIRED_CLI_VERSION

  private readonly binDir:      string
  private readonly binaryPath:  string
  private readonly versionFile: string

  constructor(context: vscode.ExtensionContext) {
    // globalStorageUri persists across extension updates and lives outside the
    // (potentially read-only) extension installation directory.
    const storageRoot = context.globalStorageUri.fsPath
    this.binDir       = path.join(storageRoot, 'bin')
    this.binaryPath   = path.join(this.binDir, BINARY)
    this.versionFile  = path.join(this.binDir, '.cli-version')
  }

  /**
   * Ensures the managed binary is present at the required version.
   * Downloads and extracts from GitHub Releases when needed.
   * Returns the absolute path to the binary.
   *
   * If a download fails but a previous binary already exists, logs a warning
   * and falls back gracefully so activation is not blocked.
   */
  async ensureBinary(
    progress: vscode.Progress<{ message?: string }>
  ): Promise<string> {
    await fs.mkdir(this.binDir, { recursive: true })

    const installedVersion = await this.readVersion()
    const alreadyCurrent   = installedVersion === REQUIRED_CLI_VERSION
                          && fsSync.existsSync(this.binaryPath)

    if (alreadyCurrent) {
      return this.binaryPath   // nothing to do
    }

    const verb = installedVersion ? 'Updating' : 'Downloading'
    progress.report({ message: `${verb} neo4j-cli v${REQUIRED_CLI_VERSION}…` })

    try {
      await this.downloadAndInstall(REQUIRED_CLI_VERSION, progress)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)

      if (fsSync.existsSync(this.binaryPath)) {
        // Degrade gracefully: use whatever we have and warn in the notification area
        void vscode.window.showWarningMessage(
          `Neo4j: could not download neo4j-cli v${REQUIRED_CLI_VERSION} (${msg}). ` +
          `Continuing with v${installedVersion ?? 'unknown'}. ` +
          `Run "Neo4j: Update CLI" to retry.`
        )
        return this.binaryPath
      }

      // Nothing to fall back to — surface a recoverable error notification
      const choice = await vscode.window.showErrorMessage(
        `Neo4j: failed to download neo4j-cli — ${msg}`,
        'Retry',
        'Set Path Manually',
      )
      if (choice === 'Retry') {
        return this.ensureBinary(progress)
      }
      if (choice === 'Set Path Manually') {
        await vscode.commands.executeCommand(
          'workbench.action.openSettings', 'neo4j.cliPath'
        )
      }
      throw new BinaryInstallError(msg)
    }

    return this.binaryPath
  }

  /** True when any binary exists under global storage (may be an older version). */
  get isInstalled(): boolean {
    return fsSync.existsSync(this.binaryPath)
  }

  get path(): string {
    return this.binaryPath
  }

  // ── Private ────────────────────────────────────────────────────────────────

  private async readVersion(): Promise<string | null> {
    try {
      return (await fs.readFile(this.versionFile, 'utf8')).trim()
    } catch {
      return null
    }
  }

  private async downloadAndInstall(
    version:  string,
    progress: vscode.Progress<{ message?: string }>,
  ): Promise<void> {
    const asset   = assetName(version)
    const url     = releaseUrl(version)
    const tmpPath = path.join(os.tmpdir(), asset)

    // Download ────────────────────────────────────────────────────────────────
    progress.report({ message: `Downloading ${asset}…` })
    await this.download(url, tmpPath)

    // Extract ─────────────────────────────────────────────────────────────────
    progress.report({ message: 'Extracting…' })
    await this.extract(tmpPath)

    // Post-install ─────────────────────────────────────────────────────────────
    await this.postInstall()

    // Record version ──────────────────────────────────────────────────────────
    await fs.writeFile(this.versionFile, version, 'utf8')

    // Clean up temp file (best-effort)
    await fs.unlink(tmpPath).catch(() => { /* ignore */ })
  }

  private async download(url: string, dest: string): Promise<void> {
    let response: Response
    try {
      // fetch follows redirects by default (GitHub releases redirect to S3)
      response = await fetch(url)
    } catch (err: unknown) {
      throw new BinaryInstallError(
        `Network error downloading neo4j-cli: ${err instanceof Error ? err.message : err}`
      )
    }

    if (!response.ok) {
      throw new BinaryInstallError(
        `HTTP ${response.status} downloading neo4j-cli from ${url}. ` +
        (response.status === 404
          ? `Release v${REQUIRED_CLI_VERSION} not found — it may not be published yet.`
          : 'Check your internet connection.')
      )
    }

    if (!response.body) {
      throw new BinaryInstallError('Empty response body from GitHub.')
    }

    // Stream to disk — avoids loading a 20–30 MB binary fully into memory
    // Readable.fromWeb requires Node 18+ (VS Code ≥ 1.82 ships Node 18)
    const writeStream = createWriteStream(dest)
    await pipeline(Readable.fromWeb(response.body as Parameters<typeof Readable.fromWeb>[0]), writeStream)
  }

  private async extract(archivePath: string): Promise<void> {
    if (archivePath.endsWith('.tar.gz')) {
      // tar is available on macOS and all Linux distributions
      await execFileAsync('tar', ['-xzf', archivePath, '-C', this.binDir])

    } else if (archivePath.endsWith('.zip')) {
      // PowerShell Expand-Archive ships with Windows 5.0+
      await execFileAsync('powershell', [
        '-NoProfile', '-NonInteractive', '-Command',
        `Expand-Archive -LiteralPath '${archivePath}' -DestinationPath '${this.binDir}' -Force`,
      ])
    } else {
      throw new BinaryInstallError(`Unrecognised archive format: ${path.basename(archivePath)}`)
    }
  }

  private async postInstall(): Promise<void> {
    if (!fsSync.existsSync(this.binaryPath)) {
      throw new BinaryInstallError(
        `Expected binary not found at ${this.binaryPath} after extraction. ` +
        'The archive layout may have changed — please file an issue.'
      )
    }

    if (os.platform() !== 'win32') {
      // Ensure the binary is executable
      await fs.chmod(this.binaryPath, 0o755)
    }

    if (os.platform() === 'darwin') {
      // Remove the macOS Gatekeeper quarantine flag that gets set when a file is
      // downloaded from the internet. Without this the binary triggers a
      // "developer cannot be verified" dialog on first run.
      // Silently ignored if the flag isn't present.
      await execFileAsync('xattr', ['-d', 'com.apple.quarantine', this.binaryPath])
        .catch(() => { /* not quarantined — no-op */ })
    }
  }
}
