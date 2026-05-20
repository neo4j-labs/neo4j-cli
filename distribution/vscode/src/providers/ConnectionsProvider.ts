import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { Credential, CredentialType } from '../types/index'

const TYPE_ICON: Record<CredentialType, string> = {
  'aura-api': 'cloud',
  'neo4j-db': 'database',
  'embed':    'symbol-array',
}

const TYPE_LABEL: Record<CredentialType, string> = {
  'aura-api': 'Aura API',
  'neo4j-db': 'Neo4j DB',
  'embed':    'Embed',
}

function credentialDescription(c: Credential): string {
  switch (c.type) {
    case 'neo4j-db': return c.uri ?? ''
    case 'aura-api': return 'Aura API'
    case 'embed':    return c.provider && c.model ? `${c.provider} · ${c.model}` : 'Embed'
  }
}

function credentialTooltip(c: Credential): string {
  const lines: string[] = [`**${c.name}**  (${TYPE_LABEL[c.type]})`]
  switch (c.type) {
    case 'neo4j-db':
      if (c.uri)      lines.push(`\`${c.uri}\``)
      if (c.username) lines.push(`User: ${c.username}`)
      if (c.lastTestMs !== undefined) lines.push(`Last test: ${c.lastTestMs}ms`)
      break
    case 'aura-api':
      if (c.clientId) lines.push(`Client ID: ${c.clientId}`)
      break
    case 'embed':
      if (c.provider)              lines.push(`Provider: ${c.provider}`)
      if (c.model)                 lines.push(`Model: ${c.model}`)
      if (c.baseUrl)               lines.push(`Base URL: ${c.baseUrl}`)
      if (c.dimensions !== undefined && c.dimensions > 0) lines.push(`Dimensions: ${c.dimensions}`)
      break
  }
  return lines.filter(Boolean).join('\n\n')
}

export class CredentialItem extends vscode.TreeItem {
  constructor(public readonly credential: Credential) {
    super(credential.name, vscode.TreeItemCollapsibleState.None)

    this.description = credentialDescription(credential)
    this.tooltip      = new vscode.MarkdownString(credentialTooltip(credential))

    this.iconPath = new vscode.ThemeIcon(
      TYPE_ICON[credential.type],
      new vscode.ThemeColor(credential.active ? 'testing.iconPassed' : 'foreground'),
    )

    // contextValue drives menu when-clauses in package.json:
    //   neo4j.credential.neo4j-db         (inactive)
    //   neo4j.credential.neo4j-db.active
    //   neo4j.credential.aura-api         (inactive)
    //   neo4j.credential.aura-api.active
    //   neo4j.credential.embed            (inactive)
    //   neo4j.credential.embed.active
    this.contextValue = credential.active
      ? `neo4j.credential.${credential.type}.active`
      : `neo4j.credential.${credential.type}`

    this.command = {
      command:   'neo4j.credential.use',
      title:     'Set Active',
      arguments: [this],
    }
  }
}

export { CredentialItem as ConnectionItem }

class ConfigureItem extends vscode.TreeItem {
  constructor() {
    super('Configure neo4j-cli path…', vscode.TreeItemCollapsibleState.None)
    this.iconPath     = new vscode.ThemeIcon('settings-gear')
    this.command      = { command: 'workbench.action.openSettings', title: 'Open Settings', arguments: ['neo4j.cliPath'] }
    this.contextValue = 'neo4j.configure'
  }
}

export class CredentialsProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
  private readonly _onChange = new vscode.EventEmitter<void>()
  readonly onDidChangeTreeData = this._onChange.event

  private credentials: Credential[] = []
  private error: string | null = null
  private loaded = false

  constructor(private readonly cli: CLIService) {}

  refresh(): void { this._onChange.fire() }

  async load(): Promise<void> {
    this.error = null
    try {
      this.credentials = await this.cli.listCredentials()
    } catch (err: unknown) {
      this.credentials = []
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
      return [item, new ConfigureItem()]
    }

    if (this.credentials.length === 0) {
      const empty = new vscode.TreeItem('No credentials — add one with +')
      empty.iconPath = new vscode.ThemeIcon('info')
      return [empty]
    }

    // Sort: active first, then by type (neo4j-db → aura-api → embed), then by name
    const typeOrder: Record<CredentialType, number> = { 'neo4j-db': 0, 'aura-api': 1, 'embed': 2 }
    return [...this.credentials]
      .sort((a, b) => {
        if (a.active !== b.active) return a.active ? -1 : 1
        const td = typeOrder[a.type] - typeOrder[b.type]
        return td !== 0 ? td : a.name.localeCompare(b.name)
      })
      .map(c => new CredentialItem(c))
  }
}

export { CredentialsProvider as ConnectionsProvider }
