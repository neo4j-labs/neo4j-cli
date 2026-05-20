import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { LocalInstance } from '../types/index'

export class LocalItem extends vscode.TreeItem {
  constructor(public readonly instance: LocalInstance) {
    super(instance.name, vscode.TreeItemCollapsibleState.None)

    const running = instance.status === 'running'

    this.description = running
      ? `${instance.version} · :${instance.port}${instance.uptime ? ` · ${instance.uptime}` : ''}`
      : `${instance.version} · stopped`

    this.tooltip = new vscode.MarkdownString(
      [
        `**${instance.name}**`,
        `${instance.version} ${instance.edition}`,
        running ? `bolt://localhost:${instance.port}` : 'stopped',
        instance.dataPath ? `\`${instance.dataPath}\`` : '',
      ].filter(Boolean).join('\n\n')
    )

    this.iconPath = running
      ? new vscode.ThemeIcon('vm-running', new vscode.ThemeColor('testing.iconPassed'))
      : new vscode.ThemeIcon('vm')

    this.contextValue = running ? 'neo4j.local.running' : 'neo4j.local.stopped'
  }
}

export class LocalProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
  private readonly _onChange = new vscode.EventEmitter<void>()
  readonly onDidChangeTreeData = this._onChange.event

  private instances: LocalInstance[] = []
  private error: string | null = null
  private loaded = false

  constructor(private readonly cli: CLIService) {}

  refresh(): void { this._onChange.fire() }

  async load(): Promise<void> {
    this.error = null
    try {
      this.instances = await this.cli.listLocalInstances()
    } catch (err: unknown) {
      this.instances = []
      this.error = err instanceof Error ? err.message : String(err)
    }
    this.loaded = true
    this.refresh()
  }

  getTreeItem(el: vscode.TreeItem): vscode.TreeItem { return el }

  async getChildren(): Promise<vscode.TreeItem[]> {
    if (!this.loaded) await this.load()

    if (this.error) {
      const item = new vscode.TreeItem(this.error)
      item.iconPath = new vscode.ThemeIcon('warning')
      return [item]
    }

    if (this.instances.length === 0) {
      const empty = new vscode.TreeItem('No local instances — create one with +')
      empty.iconPath = new vscode.ThemeIcon('info')
      return [empty]
    }

    // Running instances first, then stopped
    return [...this.instances]
      .sort((a, b) => {
        if (a.status === b.status) return a.name.localeCompare(b.name)
        return a.status === 'running' ? -1 : 1
      })
      .map(i => new LocalItem(i))
  }
}
