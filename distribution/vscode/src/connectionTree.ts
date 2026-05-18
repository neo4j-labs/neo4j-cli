import * as vscode from 'vscode';
import { run, runJSON, unwrapData } from './cli';

export interface ActiveConnection {
  name: string;
  credentialName: string;
  type: 'docker' | 'aura';
  boltUri?: string;
  username?: string;
  database?: string;
}

interface DockerRow {
  name: string;
  status: string;
  edition: string;
  version: string;
  'bolt-port': string;
  'http-port': string;
  ephemeral: boolean;
}

export interface AuraInstance {
  id: string;
  name: string;
  project_id: string;
  cloud_provider: string;
}

interface DbmsCredential {
  name: string;
  username: string;
  'database-name': string;
  uri: string;
  default: boolean;
}

export interface WorkspaceEntry {
  workspace: string;       // "<org-id>/<project-id>"
  organizationId: string;
  projectId: string;
  projectName: string;
  default: boolean;
}

export type ConnectionNode = SectionNode | ConnectionItem | InfoItem;

export class SectionNode extends vscode.TreeItem {
  constructor(
    public readonly label: string,
    public readonly sectionType: 'docker' | 'aura',
    public readonly children: (ConnectionItem | InfoItem)[]
  ) {
    super(
      label,
      vscode.TreeItemCollapsibleState.Expanded
    );
    this.contextValue = 'section';
    this.iconPath = new vscode.ThemeIcon(sectionType === 'docker' ? 'vm' : 'cloud');
  }
}

export class InfoItem extends vscode.TreeItem {
  constructor(message: string, commandId?: string) {
    super(message, vscode.TreeItemCollapsibleState.None);
    this.iconPath = new vscode.ThemeIcon(commandId ? 'warning' : 'info');
    if (commandId) {
      this.command = { command: commandId, title: message, arguments: [] };
    }
    this.contextValue = 'info';
  }
}

export class ConnectionItem extends vscode.TreeItem {
  constructor(
    public readonly connectionName: string,
    public readonly connectionType: 'docker' | 'aura',
    public readonly running: boolean | null,  // null = unknown (Aura list doesn't include status)
    public readonly isActive: boolean,
    public readonly meta: DockerRow | AuraInstance
  ) {
    super(connectionName, vscode.TreeItemCollapsibleState.None);

    if (isActive) {
      this.description = 'connected';
      this.iconPath = new vscode.ThemeIcon('circle-filled');
    } else if (running === true) {
      this.description = 'running';
      this.iconPath = new vscode.ThemeIcon('circle-outline');
    } else if (running === false) {
      this.description = 'stopped';
      this.iconPath = new vscode.ThemeIcon('circle-slash');
    } else {
      // Aura: status unknown from list endpoint
      this.iconPath = new vscode.ThemeIcon('circle-outline');
    }

    this.contextValue = isActive
      ? 'connectionActive'
      : running === false
        ? 'connectionStopped'
        : 'connectionRunning';

    this.tooltip = this.buildTooltip();
  }

  private buildTooltip(): string {
    if (this.connectionType === 'docker') {
      const d = this.meta as DockerRow;
      return `${d.edition} ${d.version} — bolt: ${d['bolt-port']}`;
    }
    const a = this.meta as AuraInstance;
    return `${a.cloud_provider.toUpperCase()} — id: ${a.id}`;
  }
}

export class ConnectionTreeProvider
  implements vscode.TreeDataProvider<ConnectionNode>
{
  private readonly _onChange = new vscode.EventEmitter<ConnectionNode | undefined>();
  readonly onDidChangeTreeData = this._onChange.event;

  private active: ActiveConnection | null = null;

  constructor(private readonly binary: string) {}

  setActive(conn: ActiveConnection | null): void {
    this.active = conn;
    this._onChange.fire(undefined);
  }

  getActive(): ActiveConnection | null {
    return this.active;
  }

  refresh(): void {
    this._onChange.fire(undefined);
  }

  getTreeItem(element: ConnectionNode): vscode.TreeItem {
    return element;
  }

  getChildren(element?: ConnectionNode): ConnectionNode[] {
    if (element instanceof SectionNode) return element.children;
    return this.buildSections();
  }

  private buildSections(): SectionNode[] {
    return [
      new SectionNode('Local (Docker)', 'docker', this.dockerItems()),
      new SectionNode('Cloud (Aura)', 'aura', this.auraItems()),
    ];
  }

  private dockerItems(): (ConnectionItem | InfoItem)[] {
    const result = run(this.binary, ['docker', 'list', '--format', 'json']);
    if (!result.ok) {
      // docker not installed or daemon not running — show a soft hint
      if (result.stderr.includes('docker') || result.stderr.includes('executable')) {
        return [new InfoItem('Docker not found — install Docker to use local instances')];
      }
      return [];
    }

    let rows: DockerRow[] = [];
    try {
      rows = JSON.parse(result.stdout) as DockerRow[];
      if (!Array.isArray(rows)) rows = [];
    } catch {
      return [];
    }

    if (rows.length === 0) {
      return [new InfoItem('No local instances — click + to create one', 'neo4j-cli.createDocker')];
    }

    return rows.map((r) => {
      const running = r.status.toLowerCase().startsWith('up');
      const isActive = this.active?.type === 'docker' && this.active.name === r.name;
      return new ConnectionItem(r.name, 'docker', running, isActive, r);
    });
  }

  private auraItems(): (ConnectionItem | InfoItem)[] {
    const result = run(this.binary, ['aura', 'instance', 'list', '--format', 'json']);

    if (!result.ok) {
      // Parse the structured JSON error envelope the CLI emits on failure
      let errorMessage = result.stderr.trim();
      try {
        const envelope = JSON.parse(result.stdout) as { error?: { message?: string; code?: string } };
        errorMessage = envelope.error?.message ?? errorMessage;
      } catch { /* use stderr fallback */ }

      if (
        errorMessage.includes('no organization specified') ||
        errorMessage.includes('no workspace') ||
        errorMessage.includes('organization-id')
      ) {
        return [
          new InfoItem('No workspace selected — click to choose', 'neo4j-cli.selectWorkspace'),
        ];
      }

      if (
        errorMessage.includes('credential') ||
        errorMessage.includes('unauthorized') ||
        errorMessage.includes('401') ||
        errorMessage.includes('no aura-client')
      ) {
        return [new InfoItem('No Aura credentials — add one via credential aura-client add')];
      }

      return [new InfoItem(`Error: ${errorMessage.slice(0, 80)}`)];
    }

    let instances: AuraInstance[] = [];
    try {
      instances = unwrapData<AuraInstance>(JSON.parse(result.stdout));
    } catch {
      return [];
    }

    if (instances.length === 0) {
      return [new InfoItem('No instances in this workspace')];
    }

    return instances.map((inst) => {
      const isActive = this.active?.type === 'aura' && this.active.name === inst.name;
      return new ConnectionItem(inst.name, 'aura', null, isActive, inst);
    });
  }

  getDbmsCredential(name: string): DbmsCredential | null {
    const raw = runJSON<unknown>(this.binary, ['credential', 'dbms', 'list']);
    const creds = unwrapData<DbmsCredential>(raw);
    return creds.find((c) => c.name === name) ?? null;
  }

  listWorkspaces(): WorkspaceEntry[] {
    const raw = runJSON<unknown>(this.binary, ['aura', 'workspace', 'list']);
    return unwrapData<WorkspaceEntry>(raw);
  }
}
