import * as vscode from 'vscode';
import { spawnSync } from 'child_process';
import { ActiveConnection } from './connectionTree';

interface QueryResult {
  columns: string[];
  rows: Record<string, unknown>[];
  truncated: boolean;
  arrays_truncated: number;
}

let panel: vscode.WebviewPanel | undefined;

export function runQueryInPanel(
  binary: string,
  connection: ActiveConnection | null,
  extensionUri: vscode.Uri
): void {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    vscode.window.showErrorMessage('Neo4j: open a .cypher file or select a query first');
    return;
  }

  if (!connection) {
    vscode.window.showErrorMessage(
      'Neo4j: no active connection — right-click an instance in the Neo4j panel and choose Connect'
    );
    return;
  }

  const selection = editor.selection;
  const query = selection.isEmpty
    ? editor.document.getText()
    : editor.document.getText(selection);

  if (!query.trim()) {
    vscode.window.showErrorMessage('Neo4j: nothing to run');
    return;
  }

  ensurePanel(extensionUri);
  showRunning(query, connection.name);

  // Pass query via stdin — the CLI reads from stdin when no positional arg is given
  const raw = spawnSync(
    binary,
    ['query', '--format', 'json', '--credential', connection.credentialName],
    { encoding: 'utf8', input: query, maxBuffer: 10 * 1024 * 1024 }
  );

  if (raw.status !== 0) {
    showError(query, connection.name, raw.stderr.trim() || raw.stdout.trim());
    return;
  }

  let parsed: QueryResult;
  try {
    parsed = JSON.parse(raw.stdout) as QueryResult;
  } catch {
    showError(query, connection.name, 'Could not parse response from neo4j-cli');
    return;
  }

  showResults(query, connection.name, parsed);
}

function ensurePanel(extensionUri: vscode.Uri): void {
  if (panel) {
    panel.reveal(vscode.ViewColumn.Beside, true);
    return;
  }
  panel = vscode.window.createWebviewPanel(
    'neo4j.results',
    'Neo4j Results',
    { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
    { enableScripts: false, retainContextWhenHidden: true }
  );
  panel.onDidDispose(() => { panel = undefined; });
}

function showRunning(query: string, connectionName: string): void {
  panel!.webview.html = buildHtml(query, connectionName, 'running');
}

function showError(query: string, connectionName: string, message: string): void {
  panel!.webview.html = buildHtml(query, connectionName, 'error', undefined, message);
}

function showResults(query: string, connectionName: string, result: QueryResult): void {
  panel!.webview.html = buildHtml(query, connectionName, 'results', result);
}

function buildHtml(
  query: string,
  connectionName: string,
  state: 'running' | 'error' | 'results',
  result?: QueryResult,
  errorMessage?: string
): string {
  const escapeHtml = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  const truncationWarning =
    result?.truncated
      ? `<div class="warning">Results truncated — showing first rows. Run with <code>--max-rows 0</code> to retrieve all.</div>`
      : '';

  const tableHtml = (() => {
    if (state !== 'results' || !result) return '';
    const cols = result.columns.map((c) => `<th>${escapeHtml(c)}</th>`).join('');
    const rowsHtml = result.rows
      .map((row) => {
        const cells = result.columns
          .map((c) => {
            const val = row[c];
            const text =
              val === null || val === undefined
                ? 'null'
                : typeof val === 'string'
                  ? val
                  : JSON.stringify(val);
            return `<td>${escapeHtml(text)}</td>`;
          })
          .join('');
        return `<tr>${cells}</tr>`;
      })
      .join('');
    const rowCount = result.rows.length;
    return `
      ${truncationWarning}
      <div class="row-count">${rowCount} row${rowCount !== 1 ? 's' : ''}</div>
      <div class="table-wrap">
        <table>
          <thead><tr>${cols}</tr></thead>
          <tbody>${rowsHtml}</tbody>
        </table>
      </div>`;
  })();

  const bodyHtml = (() => {
    if (state === 'running') return `<div class="state-msg">Running query…</div>`;
    if (state === 'error') return `<div class="error-block"><strong>Error</strong><pre>${escapeHtml(errorMessage ?? 'unknown error')}</pre></div>`;
    if (result?.rows.length === 0) return `<div class="state-msg">Query returned no rows.</div>`;
    return tableHtml;
  })();

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Neo4j Results</title>
<style>
  body {
    font-family: var(--vscode-font-family);
    font-size: var(--vscode-font-size);
    color: var(--vscode-editor-foreground);
    background: var(--vscode-editor-background);
    margin: 0;
    padding: 12px 16px;
  }
  .header {
    display: flex;
    align-items: baseline;
    gap: 12px;
    margin-bottom: 12px;
    padding-bottom: 8px;
    border-bottom: 1px solid var(--vscode-panel-border);
  }
  .conn-badge {
    font-size: 0.8em;
    padding: 2px 6px;
    border-radius: 3px;
    background: var(--vscode-badge-background);
    color: var(--vscode-badge-foreground);
  }
  .query-block {
    font-family: var(--vscode-editor-font-family, monospace);
    font-size: 0.9em;
    background: var(--vscode-textCodeBlock-background);
    border-left: 3px solid var(--vscode-focusBorder);
    padding: 8px 12px;
    margin-bottom: 12px;
    white-space: pre-wrap;
    word-break: break-all;
  }
  .row-count {
    font-size: 0.85em;
    color: var(--vscode-descriptionForeground);
    margin-bottom: 6px;
  }
  .table-wrap {
    overflow-x: auto;
  }
  table {
    border-collapse: collapse;
    width: 100%;
    font-size: 0.9em;
  }
  thead th {
    text-align: left;
    padding: 6px 10px;
    background: var(--vscode-list-hoverBackground);
    border-bottom: 2px solid var(--vscode-panel-border);
    white-space: nowrap;
    position: sticky;
    top: 0;
  }
  tbody td {
    padding: 5px 10px;
    border-bottom: 1px solid var(--vscode-panel-border);
    max-width: 320px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  tbody tr:hover td {
    background: var(--vscode-list-hoverBackground);
  }
  .state-msg {
    color: var(--vscode-descriptionForeground);
    font-style: italic;
    padding: 20px 0;
  }
  .warning {
    font-size: 0.85em;
    padding: 6px 10px;
    margin-bottom: 8px;
    background: var(--vscode-inputValidation-warningBackground);
    border: 1px solid var(--vscode-inputValidation-warningBorder);
    border-radius: 3px;
  }
  .error-block {
    padding: 10px;
    background: var(--vscode-inputValidation-errorBackground);
    border: 1px solid var(--vscode-inputValidation-errorBorder);
    border-radius: 3px;
  }
  .error-block pre {
    margin: 6px 0 0;
    white-space: pre-wrap;
    font-family: var(--vscode-editor-font-family, monospace);
    font-size: 0.9em;
  }
</style>
</head>
<body>
  <div class="header">
    <span class="conn-badge">${escapeHtml(connectionName)}</span>
  </div>
  <div class="query-block">${escapeHtml(query.trim())}</div>
  ${bodyHtml}
</body>
</html>`;
}
