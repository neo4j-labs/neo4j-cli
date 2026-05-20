import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { AuraInstance } from '../types/index'

const STATUS_ICON: Record<string, [string, string]> = {
  running:    ['circle-filled', 'testing.iconPassed'],
  paused:     ['circle-filled', 'list.warningForeground'],
  loading:    ['loading~spin',  'foreground'],
  destroying: ['circle-filled', 'testing.iconFailed'],
}

export class AuraItem extends vscode.TreeItem {
  constructor(public readonly instance: AuraInstance) {
    super(instance.name, vscode.TreeItemCollapsibleState.None)

    this.description = `${instance.tier} · ${instance.region} · ${instance.status}`

    this.tooltip = new vscode.MarkdownString(
      [
        `**${instance.name}**  \`${instance.status}\``,
        `${instance.tier} · ${instance.region}`,
        instance.neo4jVersion ? `Neo4j ${instance.neo4jVersion}` : '',
        instance.memoryGB     ? `${instance.memoryGB} GB RAM · ${instance.storageGB ?? '?'} GB storage` : '',
        instance.boltUri      ? `\`${instance.boltUri}\`` : '',
      ].filter(Boolean).join('\n\n')
    )

    const [icon, colour] = STATUS_ICON[instance.status] ?? ['circle-outline', 'foreground']
    this.iconPath = new vscode.ThemeIcon(icon, new vscode.ThemeColor(colour))

    this.contextValue = instance.status === 'paused'
      ? 'neo4j.aura.paused'
      : 'neo4j.aura.running'
  }
}

export class AuraProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
  private readonly _onChange = new vscode.EventEmitter<void>()
  readonly onDidChangeTreeData = this._onChange.event

  private instances: AuraInstance[] = []
  private error: string | null = null
  private loaded = false

  constructor(private readonly cli: CLIService) {}

  refresh(): void { this._onChange.fire() }

  async load(): Promise<void> {
    this.error = null
    try {
      this.instances = await this.cli.listAuraInstances()
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
      const empty = new vscode.TreeItem('No Aura instances — create one with +')
      empty.iconPath = new vscode.ThemeIcon('info')
      return [empty]
    }

    // Running first, then paused, then other
    const order = { running: 0, loading: 1, paused: 2, destroying: 3 }
    return [...this.instances]
      .sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9) || a.name.localeCompare(b.name))
      .map(i => new AuraItem(i))
  }
}
