import * as vscode from 'vscode';
import { run } from './cli';
import { ActiveConnection } from './connectionTree';

interface NodeProperty {
  nodeLabels: string[];
  propertyName: string;
  propertyTypes: string[];
  mandatory: boolean;
}

interface RelProperty {
  relType: string;
  propertyName: string;
  propertyTypes: string[];
}

interface RelPath {
  from: string[];
  type: string;
  to: string[];
}

interface SchemaResult {
  nodes: NodeProperty[];
  relationships: RelProperty[];
  relationship_paths: RelPath[];
}

export type SchemaNode = SchemaSection | SchemaLabel | SchemaProp;

export class SchemaSection extends vscode.TreeItem {
  constructor(
    label: string,
    public readonly children: (SchemaLabel | SchemaProp)[]
  ) {
    super(
      `${label} (${children.length})`,
      children.length > 0
        ? vscode.TreeItemCollapsibleState.Expanded
        : vscode.TreeItemCollapsibleState.Collapsed
    );
    this.iconPath = new vscode.ThemeIcon('symbol-namespace');
  }
}

export class SchemaLabel extends vscode.TreeItem {
  constructor(
    public readonly labelName: string,
    public readonly props: SchemaProp[]
  ) {
    super(
      labelName,
      props.length > 0
        ? vscode.TreeItemCollapsibleState.Collapsed
        : vscode.TreeItemCollapsibleState.None
    );
    this.description = `${props.length} prop${props.length !== 1 ? 's' : ''}`;
    this.iconPath = new vscode.ThemeIcon('symbol-class');
    this.contextValue = 'schemaLabel';
  }
}

export class SchemaProp extends vscode.TreeItem {
  constructor(name: string, types: string[], mandatory: boolean) {
    super(name, vscode.TreeItemCollapsibleState.None);
    this.description = types.join(' | ') + (mandatory ? '' : '?');
    this.iconPath = new vscode.ThemeIcon('symbol-property');
  }
}

export class SchemaTreeProvider implements vscode.TreeDataProvider<SchemaNode> {
  private readonly _onChange = new vscode.EventEmitter<SchemaNode | undefined>();
  readonly onDidChangeTreeData = this._onChange.event;

  private sections: SchemaSection[] = [];
  private loading = false;

  constructor(private readonly binary: string) {}

  refresh(connection: ActiveConnection | null): void {
    if (!connection) {
      this.sections = [];
      this._onChange.fire(undefined);
      return;
    }
    this.loading = true;
    this._onChange.fire(undefined);
    this.load(connection);
  }

  getTreeItem(element: SchemaNode): vscode.TreeItem {
    if (this.loading && !element) {
      return new vscode.TreeItem('Loading schema…');
    }
    return element;
  }

  getChildren(element?: SchemaNode): SchemaNode[] {
    if (!element) return this.sections;
    if (element instanceof SchemaSection) return element.children;
    if (element instanceof SchemaLabel) return element.props;
    return [];
  }

  private load(connection: ActiveConnection): void {
    const result = run(this.binary, [
      'query', ':schema',
      '--format', 'json',
      '--credential', connection.credentialName,
    ]);
    this.loading = false;

    if (!result.ok) {
      this.sections = [];
      this._onChange.fire(undefined);
      vscode.window.showWarningMessage(
        `Neo4j: could not load schema — ${result.stderr.trim() || 'check connection'}`
      );
      return;
    }

    let schema: SchemaResult;
    try {
      schema = JSON.parse(result.stdout) as SchemaResult;
    } catch {
      this.sections = [];
      this._onChange.fire(undefined);
      return;
    }

    this.sections = this.buildSections(schema);
    this._onChange.fire(undefined);
  }

  private buildSections(schema: SchemaResult): SchemaSection[] {
    // Group node properties by label
    const labelMap = new Map<string, SchemaProp[]>();
    for (const n of schema.nodes ?? []) {
      for (const lbl of n.nodeLabels) {
        if (!labelMap.has(lbl)) labelMap.set(lbl, []);
        if (n.propertyName) {
          labelMap.get(lbl)!.push(
            new SchemaProp(n.propertyName, n.propertyTypes, n.mandatory)
          );
        }
      }
    }
    const nodeLabels = [...labelMap.entries()].map(
      ([lbl, props]) => new SchemaLabel(lbl, props)
    );

    // Group relationship properties by type + path
    const relMap = new Map<string, SchemaProp[]>();
    for (const r of schema.relationships ?? []) {
      const key = r.relType.replace(/^:/, '');
      if (!relMap.has(key)) relMap.set(key, []);
      if (r.propertyName) {
        relMap.get(key)!.push(new SchemaProp(r.propertyName, r.propertyTypes, false));
      }
    }

    // Attach path info as label description
    const pathsByType = new Map<string, RelPath>();
    for (const p of schema.relationship_paths ?? []) {
      pathsByType.set(p.type, p);
    }

    const relLabels = [...relMap.entries()].map(([type, props]) => {
      const p = pathsByType.get(type);
      const item = new SchemaLabel(type, props);
      if (p) {
        item.description = `(${p.from.join('|')})-[]->(${p.to.join('|')})`;
      }
      return item;
    });

    return [
      new SchemaSection('Node Labels', nodeLabels),
      new SchemaSection('Relationships', relLabels),
    ];
  }
}
