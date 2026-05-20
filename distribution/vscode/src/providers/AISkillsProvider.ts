import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { AISkill } from '../types/index'

export class AISkillItem extends vscode.TreeItem {
  constructor(public readonly skill: AISkill) {
    super(skill.agentDisplayName, vscode.TreeItemCollapsibleState.None)

    if (skill.installed && skill.updateAvailable) {
      this.description   = `v${skill.installedVersion} → v${skill.latestVersion}`
      this.iconPath      = new vscode.ThemeIcon('sparkle', new vscode.ThemeColor('list.warningForeground'))
      this.contextValue  = 'neo4j.skill.outdated'
    } else if (skill.installed) {
      this.description   = `v${skill.installedVersion}`
      this.iconPath      = new vscode.ThemeIcon('sparkle', new vscode.ThemeColor('testing.iconPassed'))
      this.contextValue  = 'neo4j.skill.installed'
    } else {
      this.description   = 'not installed'
      this.iconPath      = new vscode.ThemeIcon('sparkle')
      this.contextValue  = 'neo4j.skill.notInstalled'
    }

    this.tooltip = new vscode.MarkdownString(
      [
        `**${skill.agentDisplayName}**`,
        skill.installed
          ? `Neo4j skills v${skill.installedVersion} installed`
          : 'Neo4j skills not yet installed',
        skill.updateAvailable
          ? `Update available: v${skill.latestVersion}`
          : '',
        skill.skillsPath
          ? `Path: \`${skill.skillsPath}\``
          : '',
        skill.detectedAt
          ? `Agent detected: ${skill.detectedAt}`
          : '',
      ].filter(Boolean).join('\n\n')
    )
  }
}

export class AISkillsProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
  private readonly _onChange = new vscode.EventEmitter<void>()
  readonly onDidChangeTreeData = this._onChange.event

  private skills: AISkill[] = []
  private error: string | null = null
  private loaded = false

  constructor(private readonly cli: CLIService) {}

  refresh(): void { this._onChange.fire() }

  async load(): Promise<void> {
    this.error = null
    try {
      this.skills = await this.cli.listAISkills()
    } catch (err: unknown) {
      this.skills = []
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

    if (this.skills.length === 0) {
      const empty = new vscode.TreeItem('No AI agents detected')
      empty.iconPath    = new vscode.ThemeIcon('info')
      empty.description = 'install Gemini or Copilot to get started'
      return [empty]
    }

    // Updates first, then installed, then not installed
    return [...this.skills]
      .sort((a, b) => {
        if (a.updateAvailable !== b.updateAvailable) return a.updateAvailable ? -1 : 1
        if (a.installed !== b.installed)             return a.installed       ? -1 : 1
        return a.agentDisplayName.localeCompare(b.agentDisplayName)
      })
      .map(s => new AISkillItem(s))
  }
}
