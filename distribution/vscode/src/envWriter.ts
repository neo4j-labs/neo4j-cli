import * as fs from 'fs';
import * as path from 'path';
import * as vscode from 'vscode';
import { ActiveConnection } from './connectionTree';

const ENV_KEYS = ['NEO4J_URI', 'NEO4J_USERNAME', 'NEO4J_PASSWORD', 'NEO4J_DATABASE'] as const;

export async function offerEnvWrite(connection: ActiveConnection): Promise<void> {
  const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
  if (!workspaceRoot) return;

  const answer = await vscode.window.showInformationMessage(
    `Write Neo4j connection to .env in your workspace?`,
    { modal: false },
    'Write .env',
    'Skip'
  );
  if (answer !== 'Write .env') return;

  writeEnv(workspaceRoot, connection);
}

export function writeEnv(workspaceRoot: string, connection: ActiveConnection): void {
  const envPath = path.join(workspaceRoot, '.env');

  const existing = fs.existsSync(envPath) ? fs.readFileSync(envPath, 'utf8') : '';
  const lines = existing.split('\n');

  const uri = connection.boltUri ?? `neo4j://localhost:7687`;
  const username = connection.username ?? 'neo4j';
  const database = connection.database ?? 'neo4j';

  const updates: Record<string, string> = {
    NEO4J_URI: uri,
    NEO4J_USERNAME: username,
    NEO4J_PASSWORD: '',   // never expose stored passwords; user fills in
    NEO4J_DATABASE: database,
  };

  const updated = applyEnvUpdates(lines, updates);
  fs.writeFileSync(envPath, updated, 'utf8');

  vscode.window
    .showInformationMessage(
      `Updated .env with NEO4J_URI=${uri}. Fill in NEO4J_PASSWORD.`,
      'Open .env'
    )
    .then((choice) => {
      if (choice === 'Open .env') {
        vscode.window.showTextDocument(vscode.Uri.file(envPath));
      }
    });
}

// Updates lines in-place for known keys; appends any missing keys at the end.
// Preserves all existing lines (comments, blanks, unrelated keys).
function applyEnvUpdates(
  lines: string[],
  updates: Record<string, string>
): string {
  const remaining = new Set(Object.keys(updates));
  const out = lines.map((line) => {
    const match = /^([A-Z0-9_]+)=/.exec(line);
    if (match && remaining.has(match[1])) {
      remaining.delete(match[1]);
      const key = match[1] as keyof typeof updates;
      // Preserve empty password placeholder so the user can see it
      if (key === 'NEO4J_PASSWORD' && updates[key] === '') {
        return `NEO4J_PASSWORD=`;
      }
      return `${key}=${updates[key]}`;
    }
    return line;
  });

  // Strip trailing blank that split() may produce
  while (out.length && out[out.length - 1] === '') out.pop();

  // Append any keys not already present
  if (remaining.size > 0) {
    out.push('');
    out.push('# Neo4j — managed by Neo4j VS Code extension');
    for (const key of ENV_KEYS) {
      if (remaining.has(key)) {
        const val = key === 'NEO4J_PASSWORD' ? '' : updates[key];
        out.push(`${key}=${val}`);
      }
    }
  }

  return out.join('\n') + '\n';
}
