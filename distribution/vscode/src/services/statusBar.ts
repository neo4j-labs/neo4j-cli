import * as vscode from 'vscode'
import type { CLIService } from './cli'
import { CLINotFoundError } from './cli'
import type { Credential } from '../types/index'

// Status bar states
// connected:  ● prod-db  23ms  (standard blue bg)
// connecting: ⟳ prod-db        (blue bg, spinner)
// error:      ⚠ prod-db        (red bg)
// none:       ○ no connection  (prominent bg)

export class StatusBarService implements vscode.Disposable {
  private readonly item: vscode.StatusBarItem
  private refreshTimer: ReturnType<typeof setInterval> | undefined
  private activeCredential: Credential | null = null

  constructor(
    private readonly context: vscode.ExtensionContext,
    private readonly cli: CLIService,
  ) {
    this.item = vscode.window.createStatusBarItem(
      'neo4j.connection',
      vscode.StatusBarAlignment.Left,
      100,
    )
    this.item.name    = 'Neo4j Connection'
    this.item.command = 'neo4j.statusBar.click'
    this.item.show()
    context.subscriptions.push(this.item)

    this.setNone()
    this.startAutoRefresh()
  }

  // ── Public API ──────────────────────────────────────────────────────────────

  async refresh(): Promise<void> {
    this.setConnecting(this.activeCredential?.name ?? '…')
    try {
      const cred = await this.cli.getActiveConnection()
      this.activeCredential = cred
      if (!cred) {
        this.setNone()
        return
      }
      const showMs = vscode.workspace.getConfiguration('neo4j').get<boolean>('showLatencyInStatusBar', true)
      this.setConnected(cred.name, showMs ? cred.lastTestMs : undefined)
    } catch (err) {
      if (err instanceof CLINotFoundError) {
        this.setError('neo4j-cli not found')
      } else {
        this.setError(this.activeCredential?.name ?? 'error')
      }
    }
  }

  get connection(): Credential | null {
    return this.activeCredential
  }

  // ── Quick-pick on status bar click ──────────────────────────────────────────
  // Shows only Neo4j DB credentials — those are the ones relevant to the
  // active query connection shown in the status bar.

  async showPicker(): Promise<void> {
    type Item = vscode.QuickPickItem & { _cred?: Credential; _action?: string }

    let credentials: Credential[] = []
    try {
      const all = await this.cli.listCredentials()
      credentials = all.filter(c => c.type === 'neo4j-db')
    } catch { /* surfaced in the Credentials tree view */ }

    const active   = credentials.find(c => c.active)
    const inactive = credentials.filter(c => !c.active)

    const items: Item[] = []

    if (inactive.length > 0) {
      items.push({ label: 'switch connection', kind: vscode.QuickPickItemKind.Separator })
      inactive.forEach(c => items.push({
        label:       `$(circle-outline)  ${c.name}`,
        description: c.uri ?? '',
        _cred:       c,
      }))
    }

    items.push({ label: 'actions', kind: vscode.QuickPickItemKind.Separator })
    items.push({ label: '$(terminal)      New Query Window',   _action: 'query'  })
    items.push({ label: '$(database)      Manage Credentials', _action: 'manage' })
    if (active) {
      items.push({ label: '$(x)            Disconnect', _action: 'disconnect' })
    }

    const picked = await vscode.window.showQuickPick<Item>(items, {
      title: active
        ? `$(circle-filled) ${active.name}${active.lastTestMs !== undefined ? `  ${active.lastTestMs}ms` : ''}`
        : 'No active Neo4j DB connection',
      placeHolder: 'Switch connection or run a command',
    })

    if (!picked) return

    if (picked._cred) {
      await vscode.commands.executeCommand('neo4j.credential.use', new Proxy(picked._cred, {}))
      // credential.use expects a CredentialItem; pass the raw credential wrapped in a minimal proxy
      await this.cli.useCredential(picked._cred.name, picked._cred.type)
      void this.refresh()
    } else if (picked._action === 'query') {
      await vscode.commands.executeCommand('neo4j.query.new')
    } else if (picked._action === 'manage') {
      await vscode.commands.executeCommand('neo4j.openManager')
    } else if (picked._action === 'disconnect') {
      this.setNone()
      this.activeCredential = null
    }
  }

  dispose(): void {
    this.stopAutoRefresh()
    this.item.dispose()
  }

  // ── Private state setters ───────────────────────────────────────────────────

  private setConnected(name: string, ms?: number): void {
    this.item.text            = `$(circle-filled)  ${name}${ms !== undefined ? `  ${ms}ms` : ''}`
    this.item.color           = new vscode.ThemeColor('statusBarItem.foreground')
    this.item.backgroundColor = undefined
    this.item.tooltip         = `Neo4j: connected to ${name}`
  }

  private setConnecting(name: string): void {
    this.item.text            = `$(loading~spin)  ${name}`
    this.item.color           = new vscode.ThemeColor('statusBarItem.foreground')
    this.item.backgroundColor = undefined
    this.item.tooltip         = `Neo4j: connecting to ${name}…`
  }

  private setError(name: string): void {
    this.item.text            = `$(warning)  ${name}`
    this.item.color           = undefined
    this.item.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground')
    this.item.tooltip         = `Neo4j: could not reach ${name} — click to switch`
  }

  private setNone(): void {
    this.item.text            = `$(circle-outline)  no connection`
    this.item.color           = new vscode.ThemeColor('statusBarItem.prominentForeground')
    this.item.backgroundColor = new vscode.ThemeColor('statusBarItem.prominentBackground')
    this.item.tooltip         = `Neo4j: no active connection — click to connect`
  }

  // ── Auto-refresh ────────────────────────────────────────────────────────────

  private startAutoRefresh(): void {
    const interval = vscode.workspace.getConfiguration('neo4j').get<number>('autoRefreshInterval', 30)
    if (interval <= 0) return

    this.refreshTimer = setInterval(() => void this.refresh(), interval * 1_000)
    this.context.subscriptions.push({ dispose: () => this.stopAutoRefresh() })

    vscode.workspace.onDidChangeConfiguration(e => {
      if (e.affectsConfiguration('neo4j.autoRefreshInterval')) {
        this.stopAutoRefresh()
        this.startAutoRefresh()
      }
    }, undefined, this.context.subscriptions)
  }

  private stopAutoRefresh(): void {
    if (this.refreshTimer !== undefined) {
      clearInterval(this.refreshTimer)
      this.refreshTimer = undefined
    }
  }
}
