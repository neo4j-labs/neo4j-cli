import * as vscode from 'vscode'
import { BinaryManager, BinaryInstallError } from './services/binaryManager'
import { CLIService }          from './services/cli'
import { StatusBarService }    from './services/statusBar'
import { CredentialsProvider } from './providers/ConnectionsProvider'
import { LocalProvider }       from './providers/LocalProvider'
import { AuraProvider }        from './providers/AuraProvider'
import { AISkillsProvider }    from './providers/AISkillsProvider'
import { registerCommands }    from './commands/index'

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  // ── Resolve CLI binary ─────────────────────────────────────────────────────
  const overridePath = vscode.workspace.getConfiguration('neo4j').get<string>('cliPath', '').trim()
  let cliPath: string
  const binaryManager = new BinaryManager(context)

  if (overridePath) {
    cliPath = overridePath
    void vscode.window.showInformationMessage(
      `Neo4j: using CLI override at ${overridePath}. Clear neo4j.cliPath to switch back to the managed binary.`
    )
  } else {
    try {
      cliPath = await vscode.window.withProgress(
        { location: vscode.ProgressLocation.Notification, title: 'Neo4j', cancellable: false },
        progress => binaryManager.ensureBinary(progress)
      )
    } catch (err: unknown) {
      if (err instanceof BinaryInstallError) return
      throw err
    }
  }

  // ── Core services ──────────────────────────────────────────────────────────
  const cli       = new CLIService(cliPath)
  const statusBar = new StatusBarService(context, cli)
  context.subscriptions.push(statusBar)

  // ── Tree view providers ────────────────────────────────────────────────────
  const connectionsProvider = new CredentialsProvider(cli)
  const localProvider       = new LocalProvider(cli)
  const auraProvider        = new AuraProvider(cli)
  const aiSkillsProvider    = new AISkillsProvider(cli)

  context.subscriptions.push(
    vscode.window.createTreeView('neo4j.credentials', {
      treeDataProvider: connectionsProvider,
      showCollapseAll:  false,
    }),
    vscode.window.createTreeView('neo4j.local',    { treeDataProvider: localProvider }),
    vscode.window.createTreeView('neo4j.aura',     { treeDataProvider: auraProvider }),
    vscode.window.createTreeView('neo4j.aiSkills', { treeDataProvider: aiSkillsProvider }),
  )

  // ── Commands ───────────────────────────────────────────────────────────────
  registerCommands(context, {
    cli,
    statusBar,
    connectionsProvider,
    localProvider,
    auraProvider,
    aiSkillsProvider,
  })

  context.subscriptions.push(
    vscode.commands.registerCommand('neo4j.cli.update', async () => {
      if (overridePath) {
        void vscode.window.showInformationMessage(
          'neo4j.cliPath is set — the managed binary is not in use. Clear the setting first.'
        )
        return
      }
      try {
        await vscode.window.withProgress(
          { location: vscode.ProgressLocation.Notification, title: 'Neo4j: updating CLI…', cancellable: false },
          progress => binaryManager.ensureBinary(progress)
        )
        void vscode.window.showInformationMessage(
          `Neo4j CLI updated to v${BinaryManager.requiredVersion}.`
        )
      } catch { /* error notification shown inside ensureBinary */ }
    })
  )

  vscode.workspace.onDidChangeConfiguration(e => {
    if (e.affectsConfiguration('neo4j.cliPath')) {
      void vscode.window.showInformationMessage(
        'Neo4j: CLI path changed — reload the window to apply.',
        'Reload',
      ).then(choice => {
        if (choice === 'Reload') void vscode.commands.executeCommand('workbench.action.reloadWindow')
      })
    }
  }, undefined, context.subscriptions)

  // ── Initial load ───────────────────────────────────────────────────────────
  await Promise.all([
    statusBar.refresh(),
    connectionsProvider.load(),
    localProvider.load(),
    auraProvider.load(),
    aiSkillsProvider.load(),
  ])
}

export function deactivate(): void {}
