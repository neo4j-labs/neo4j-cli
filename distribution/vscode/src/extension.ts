import * as vscode from 'vscode';
import { findBinary } from './binary';
import { ActiveConnection, ConnectionItem, ConnectionTreeProvider } from './connectionTree';
import { offerEnvWrite } from './envWriter';
import { run } from './cli';
import { runQueryInPanel } from './resultsPanel';
import { SchemaTreeProvider } from './schemaTree';

export function activate(context: vscode.ExtensionContext): void {
  const binary = findBinary(context);

  const connTree = new ConnectionTreeProvider(binary);
  const schemaTree = new SchemaTreeProvider(binary);
  const statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  statusBar.command = 'neo4j-cli.showConnections';
  statusBar.show();
  updateStatusBar(statusBar, null);

  context.subscriptions.push(
    statusBar,

    vscode.window.registerTreeDataProvider('neo4j.connections', connTree),
    vscode.window.registerTreeDataProvider('neo4j.schema', schemaTree),

    vscode.commands.registerCommand('neo4j-cli.refresh', () => {
      connTree.refresh();
      schemaTree.refresh(connTree.getActive());
    }),

    vscode.commands.registerCommand('neo4j-cli.connect', async (item: ConnectionItem) => {
      if (!item) return;
      await connectToInstance(binary, item, connTree, schemaTree, statusBar);
    }),

    vscode.commands.registerCommand('neo4j-cli.disconnect', () => {
      connTree.setActive(null);
      schemaTree.refresh(null);
      updateStatusBar(statusBar, null);
      vscode.window.showInformationMessage('Neo4j: disconnected');
    }),

    vscode.commands.registerCommand('neo4j-cli.runQuery', () => {
      runQueryInPanel(binary, connTree.getActive(), context.extensionUri);
    }),

    vscode.commands.registerCommand('neo4j-cli.createDocker', async () => {
      await createDockerInstance(connTree);
    }),

    vscode.commands.registerCommand('neo4j-cli.deleteInstance', async (item: ConnectionItem) => {
      if (!item) return;
      await deleteInstance(binary, item, connTree, schemaTree, statusBar);
    }),

    vscode.commands.registerCommand('neo4j-cli.writeEnv', () => {
      const conn = connTree.getActive();
      if (!conn) {
        vscode.window.showErrorMessage('Neo4j: connect to an instance first');
        return;
      }
      const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
      if (root) {
        const { writeEnv } = require('./envWriter') as typeof import('./envWriter');
        writeEnv(root, conn);
      }
    }),

    // Show the connections panel when clicking the status bar
    vscode.commands.registerCommand('neo4j-cli.showConnections', () => {
      vscode.commands.executeCommand('neo4j.connections.focus');
    }),

    vscode.commands.registerCommand('neo4j-cli.selectWorkspace', async () => {
      await selectWorkspaceCommand(binary, connTree);
    }),
  );
}

async function connectToInstance(
  binary: string,
  item: ConnectionItem,
  connTree: ConnectionTreeProvider,
  schemaTree: SchemaTreeProvider,
  statusBar: vscode.StatusBarItem
): Promise<void> {
  if (item.connectionType === 'docker') {
    // Docker containers automatically get a DBMS credential named after the container
    const cred = connTree.getDbmsCredential(item.connectionName);
    if (!cred) {
      vscode.window.showErrorMessage(
        `Neo4j: no stored credential found for "${item.connectionName}". ` +
        `The container may still be starting, or was created outside neo4j-cli.`
      );
      return;
    }

    const conn: ActiveConnection = {
      name: item.connectionName,
      credentialName: cred.name,
      type: 'docker',
      boltUri: cred.uri,
      username: cred.username,
      database: cred['database-name'] || 'neo4j',
    };
    connTree.setActive(conn);
    schemaTree.refresh(conn);
    updateStatusBar(statusBar, conn);
    await offerEnvWrite(conn);

  } else {
    // Aura: check for an existing DBMS credential matching this instance name
    const cred = connTree.getDbmsCredential(item.connectionName);
    if (!cred) {
      const add = await vscode.window.showWarningMessage(
        `No DBMS credential found for "${item.connectionName}". ` +
        `Add one to connect and run Cypher queries.`,
        'Add credential',
        'Cancel'
      );
      if (add === 'Add credential') {
        openAddCredentialTerminal(binary, item.connectionName);
      }
      return;
    }

    const conn: ActiveConnection = {
      name: item.connectionName,
      credentialName: cred.name,
      type: 'aura',
      boltUri: cred.uri,
      username: cred.username,
      database: cred['database-name'] || 'neo4j',
    };
    connTree.setActive(conn);
    schemaTree.refresh(conn);
    updateStatusBar(statusBar, conn);
    await offerEnvWrite(conn);
  }
}

async function createDockerInstance(connTree: ConnectionTreeProvider): Promise<void> {
  const name = await vscode.window.showInputBox({
    prompt: 'Container name (leave blank for auto-generated)',
    placeHolder: 'e.g. my-neo4j',
  });
  if (name === undefined) return; // cancelled

  const edition = await vscode.window.showQuickPick(
    [
      { label: 'Community', description: 'Free, single instance', value: 'community' },
      { label: 'Enterprise', description: 'Requires license', value: 'enterprise' },
    ],
    { placeHolder: 'Select Neo4j edition' }
  );
  if (!edition) return;

  const args = ['docker', 'create', '--edition', edition.value];
  if (name.trim()) args.push('--name', name.trim());

  const terminal = vscode.window.createTerminal({ name: 'Neo4j: Create Instance', isTransient: true });
  terminal.show();
  terminal.sendText(`neo4j-cli ${args.join(' ')}`);

  // Refresh tree after a short delay to pick up the new container
  setTimeout(() => connTree.refresh(), 8000);
}

async function deleteInstance(
  binary: string,
  item: ConnectionItem,
  connTree: ConnectionTreeProvider,
  schemaTree: SchemaTreeProvider,
  statusBar: vscode.StatusBarItem
): Promise<void> {
  const confirm = await vscode.window.showWarningMessage(
    `Delete "${item.connectionName}"? This is irreversible.`,
    { modal: true },
    'Delete'
  );
  if (confirm !== 'Delete') return;

  const terminal = vscode.window.createTerminal({ name: 'Neo4j: Delete', isTransient: true });
  terminal.show();

  if (item.connectionType === 'docker') {
    terminal.sendText(`"${binary}" docker delete ${item.connectionName}`);
  } else {
    const meta = item.meta as { id: string };
    terminal.sendText(`"${binary}" aura instance delete ${meta.id} --wait`);
  }

  // Clear active connection if it was the deleted one
  if (connTree.getActive()?.name === item.connectionName) {
    connTree.setActive(null);
    schemaTree.refresh(null);
    updateStatusBar(statusBar, null);
  }

  setTimeout(() => connTree.refresh(), 4000);
}

async function selectWorkspaceCommand(
  binary: string,
  connTree: ConnectionTreeProvider
): Promise<void> {
  const workspaces = connTree.listWorkspaces();

  if (workspaces.length === 0) {
    vscode.window.showErrorMessage(
      'Neo4j: no workspaces found — check your Aura credentials with `neo4j-cli credential aura-client list`'
    );
    return;
  }

  const items = workspaces.map((w) => ({
    label: w.projectName,
    description: w.workspace,
    detail: w.default ? '(current default)' : undefined,
  }));

  const picked = await vscode.window.showQuickPick(items, {
    placeHolder: 'Select Aura workspace',
    matchOnDescription: true,
  });
  if (!picked) return;

  const result = run(binary, ['aura', 'workspace', 'use', picked.description!, '--rw']);
  if (result.ok) {
    vscode.window.showInformationMessage(`Neo4j: workspace set to "${picked.label}"`);
    connTree.refresh();
  } else {
    vscode.window.showErrorMessage(
      `Neo4j: failed to set workspace — ${result.stderr.trim()}`
    );
  }
}

function openAddCredentialTerminal(binary: string, instanceName: string): void {
  const terminal = vscode.window.createTerminal({ name: 'Neo4j: Add Credential' });
  terminal.show();
  terminal.sendText(
    `"${binary}" credential dbms add --name "${instanceName}" --uri <bolt-uri> --username neo4j --password <password> --rw`
  );
}

function updateStatusBar(bar: vscode.StatusBarItem, conn: ActiveConnection | null): void {
  if (conn) {
    bar.text = `$(circle-filled) ${conn.name}`;
    bar.tooltip = `Connected to ${conn.boltUri ?? conn.name} — click to view connections`;
    bar.color = undefined;
  } else {
    bar.text = `$(circle-slash) Neo4j`;
    bar.tooltip = 'Not connected — click to view connections';
    bar.color = new vscode.ThemeColor('descriptionForeground');
  }
}

export function deactivate(): void {}
