import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { WebviewMessage, HostMessage } from '../types/index'

type PanelMode = 'query' | 'connections'

export class QueryPanel {
  private static instance: QueryPanel | undefined

  private readonly panel: vscode.WebviewPanel

  static createOrShow(
    context: vscode.ExtensionContext,
    cli:     CLIService,
    mode:    PanelMode = 'query',
  ): void {
    const column = vscode.window.activeTextEditor?.viewColumn ?? vscode.ViewColumn.One

    if (QueryPanel.instance) {
      QueryPanel.instance.panel.reveal(column)
      return
    }

    const panel = vscode.window.createWebviewPanel(
      'neo4j.queryPanel',
      'Neo4j: Query',
      column,
      {
        enableScripts:           true,
        retainContextWhenHidden: true,
        localResourceRoots:      [vscode.Uri.joinPath(context.extensionUri, 'resources')],
      },
    )

    QueryPanel.instance = new QueryPanel(panel, cli, context)
  }

  private constructor(
    panel:   vscode.WebviewPanel,
    private readonly cli: CLIService,
    context: vscode.ExtensionContext,
  ) {
    this.panel = panel
    this.panel.webview.html = this.buildHtml()

    vscode.workspace.onDidChangeConfiguration(
      () => void this.pushConnectionState(),
      undefined,
      context.subscriptions,
    )

    this.panel.webview.onDidReceiveMessage(
      (msg: WebviewMessage) => void this.handleMessage(msg),
      undefined,
      context.subscriptions,
    )

    this.panel.onDidDispose(() => { QueryPanel.instance = undefined }, null, context.subscriptions)

    void this.pushConnectionState()
  }

  // ── Message bridge ──────────────────────────────────────────────────────────

  private async handleMessage(msg: WebviewMessage): Promise<void> {
    switch (msg.type) {

      case 'ready':
        await this.pushConnectionState()
        break

      case 'runQuery': {
        const { dbms, query, write } = msg.payload
        try {
          const result = await this.cli.runCypher({ dbms, query, write })
          this.post({ type: 'queryResult', payload: result })
        } catch (err: unknown) {
          this.post({ type: 'queryError', payload: err instanceof Error ? err.message : String(err) })
        }
        break
      }

      case 'explainQuery': {
        const { dbms, query } = msg.payload
        try {
          const result = await this.cli.explainCypher({ dbms, query })
          this.post({ type: 'queryResult', payload: result })
        } catch (err: unknown) {
          this.post({ type: 'queryError', payload: err instanceof Error ? err.message : String(err) })
        }
        break
      }

      case 'getSchema': {
        const { dbms } = msg.payload
        try {
          const schema = await this.cli.getSchema(dbms)
          this.post({ type: 'schemaResult', payload: schema })
        } catch {
          // Schema is best-effort; failures are silent
        }
        break
      }
    }
  }

  private post(msg: HostMessage): void {
    void this.panel.webview.postMessage(msg)
  }

  private async pushConnectionState(): Promise<void> {
    try {
      const conn = await this.cli.getActiveConnection()
      this.post({ type: 'connectionChanged', payload: conn })
    } catch {
      this.post({ type: 'connectionChanged', payload: null })
    }
  }

  // ── Webview HTML ────────────────────────────────────────────────────────────
  // Communicates with the extension host via the VS Code webview message API.
  // Host to webview:  window.addEventListener('message', e => handle(e.data))
  // Webview to host:  vscode.postMessage({ type, payload })
  //
  // IMPORTANT: do not use backtick characters inside the template literal below.
  // They will terminate the outer template string and cause TS1005 parse errors.

  private buildHtml(): string {
    const nonce = crypto.randomUUID().replace(/-/g, '')

    return /* html */`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta http-equiv="Content-Security-Policy"
        content="default-src 'none'; style-src 'nonce-${nonce}'; script-src 'nonce-${nonce}';">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Neo4j Query</title>
  <style nonce="${nonce}">
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: var(--vscode-editor-font-family, 'Consolas', monospace);
      font-size:   var(--vscode-editor-font-size, 13px);
      color:       var(--vscode-editor-foreground);
      background:  var(--vscode-editor-background);
      height: 100vh; display: flex; flex-direction: column; overflow: hidden;
    }
    #conn-strip {
      display: flex; align-items: center; gap: 8px;
      padding: 6px 14px; flex-shrink: 0;
      background: var(--vscode-sideBar-background);
      border-bottom: 1px solid var(--vscode-panel-border);
      font-size: 12px;
    }
    #conn-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
                background: var(--vscode-testing-iconPassed); }
    #conn-dot.none  { background: var(--vscode-foreground); opacity: .3; }
    #conn-dot.error { background: var(--vscode-testing-iconFailed); }
    #conn-name      { color: var(--vscode-testing-iconPassed); }
    #conn-name.none { color: var(--vscode-foreground); opacity: .5; }
    #conn-meta      { margin-left: auto; opacity: .5; font-size: 11px; }
    #editor-wrap { flex-shrink: 0; border-bottom: 1px solid var(--vscode-panel-border); }
    #editor {
      padding: 10px 14px; min-height: 140px; outline: none;
      white-space: pre; overflow-x: auto; line-height: 1.75;
    }
    #run-bar {
      display: flex; align-items: center; gap: 8px;
      padding: 7px 14px; flex-shrink: 0;
      background: var(--vscode-sideBar-background);
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    button {
      background: var(--vscode-button-background);
      color: var(--vscode-button-foreground);
      border: none; padding: 4px 12px; border-radius: 2px;
      cursor: pointer; font-size: 12px; font-family: inherit;
    }
    button:hover    { background: var(--vscode-button-hoverBackground); }
    button:disabled { opacity: .4; cursor: not-allowed; }
    button.sec {
      background: transparent; color: var(--vscode-foreground); opacity: .7;
      border: 1px solid var(--vscode-button-secondaryBorder, #555);
    }
    button.sec:hover { opacity: 1; }
    #shortcut { font-size: 11px; opacity: .35; }
    #exec-ms  { font-size: 11px; opacity: .5; margin-left: auto; }
    #results  { flex: 1; display: flex; flex-direction: column; overflow: hidden; }
    #results-empty {
      flex: 1; display: flex; align-items: center; justify-content: center;
      opacity: .3; font-size: 12px;
    }
    #results-tabs {
      display: flex; align-items: center; gap: 2px;
      padding: 0 8px; flex-shrink: 0;
      background: var(--vscode-sideBar-background);
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    .rtab {
      padding: 5px 12px; font-size: 12px; cursor: pointer;
      border-bottom: 2px solid transparent; opacity: .6;
    }
    .rtab.on { opacity: 1; border-bottom-color: var(--vscode-focusBorder); }
    #result-meta    { margin-left: 8px; font-size: 11px; opacity: .5; }
    #result-actions { margin-left: auto; }
    #result-table-wrap { flex: 1; overflow-y: auto; }
    table { width: 100%; border-collapse: collapse; font-size: 12px; table-layout: fixed; }
    thead tr { background: var(--vscode-sideBar-background); position: sticky; top: 0; z-index: 1; }
    th {
      text-align: left; padding: 5px 14px;
      font-size: 10px; font-weight: 400;
      text-transform: uppercase; letter-spacing: .06em; opacity: .5;
      border-bottom: 1px solid var(--vscode-panel-border);
    }
    td {
      padding: 5px 14px;
      border-bottom: 1px solid var(--vscode-panel-border);
      overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    tr:hover td { background: var(--vscode-list-hoverBackground); }
    #result-json { flex: 1; overflow-y: auto; padding: 12px 14px; display: none; }
    pre  { font-size: 12px; line-height: 1.6; white-space: pre-wrap; word-break: break-word; }
    #result-error {
      flex: 1; padding: 16px 14px; display: none;
      color: var(--vscode-testing-iconFailed); font-size: 12px; line-height: 1.6;
    }
  </style>
</head>
<body>

  <div id="conn-strip">
    <span id="conn-dot" class="none"></span>
    <span id="conn-name" class="none">no connection</span>
    <span id="conn-meta"></span>
  </div>

  <div id="editor-wrap">
    <div id="editor" contenteditable="true" spellcheck="false"
         aria-label="Cypher query editor" role="textbox" aria-multiline="true"
    >MATCH (n) RETURN n LIMIT 10</div>
  </div>

  <div id="run-bar">
    <button id="run-btn">&#9654; run</button>
    <button id="exp-btn" class="sec">explain</button>
    <span id="shortcut">Ctrl+Enter</span>
    <span id="exec-ms"></span>
  </div>

  <div id="results">
    <div id="results-empty">run to see results</div>

    <div id="results-tabs" style="display:none">
      <div class="rtab on" data-tab="table">Table</div>
      <div class="rtab"    data-tab="json">JSON</div>
      <span id="result-meta"></span>
      <span id="result-actions">
        <button class="sec" id="copy-btn" style="padding:2px 8px;font-size:11px;">copy</button>
      </span>
    </div>

    <div id="result-table-wrap" style="display:none">
      <table><thead id="tbl-head"></thead><tbody id="tbl-body"></tbody></table>
    </div>
    <div id="result-json"><pre id="json-pre"></pre></div>
    <div id="result-error"></div>
  </div>

  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi()

    let activeName = null
    let lastResult = null
    let activeTab  = 'table'

    const editor       = document.getElementById('editor')
    const runBtn       = document.getElementById('run-btn')
    const expBtn       = document.getElementById('exp-btn')
    const execMs       = document.getElementById('exec-ms')
    const resultsMeta  = document.getElementById('result-meta')
    const resultsEmpty = document.getElementById('results-empty')
    const resultsTabs  = document.getElementById('results-tabs')
    const tblWrap      = document.getElementById('result-table-wrap')
    const tblHead      = document.getElementById('tbl-head')
    const tblBody      = document.getElementById('tbl-body')
    const jsonPre      = document.getElementById('json-pre')
    const jsonDiv      = document.getElementById('result-json')
    const errDiv       = document.getElementById('result-error')
    const copyBtn      = document.getElementById('copy-btn')
    const connDot      = document.getElementById('conn-dot')
    const connName     = document.getElementById('conn-name')
    const connMeta     = document.getElementById('conn-meta')

    function setConnection(conn) {
      if (!conn) {
        connDot.className    = 'none'
        connName.className   = 'none'
        connName.textContent = 'no connection'
        connMeta.textContent = ''
        activeName = null
        return
      }
      activeName           = conn.name
      connDot.className    = ''
      connName.className   = ''
      connName.textContent = conn.name
      connMeta.textContent = [conn.tier, conn.region, conn.lastTestMs ? conn.lastTestMs + 'ms' : '']
        .filter(Boolean).join(' \u00b7 ')
    }

    function currentQuery() { return editor.textContent.trim() }

    function setRunning(label) {
      runBtn.disabled = expBtn.disabled = true
      runBtn.textContent = label
      showEmpty('running\u2026')
      execMs.textContent = ''
    }

    function resetButtons() {
      runBtn.disabled = expBtn.disabled = false
      runBtn.textContent = '\u25b6 run'
    }

    runBtn.addEventListener('click', function() {
      if (!activeName) { showError('No active connection. Switch connection first.'); return }
      setRunning('running\u2026')
      vscode.postMessage({ type: 'runQuery', payload: { dbms: activeName, query: currentQuery() } })
    })

    expBtn.addEventListener('click', function() {
      if (!activeName) { showError('No active connection.'); return }
      setRunning('explaining\u2026')
      vscode.postMessage({ type: 'explainQuery', payload: { dbms: activeName, query: currentQuery() } })
    })

    editor.addEventListener('keydown', function(e) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault()
        runBtn.click()
      }
    })

    function showResult(result) {
      lastResult = result
      resetButtons()
      execMs.textContent      = result.ms + 'ms'
      resultsMeta.textContent = result.rowCount + ' rows \u00b7 ' + result.ms + 'ms'
      resultsEmpty.style.display = 'none'
      resultsTabs.style.display  = 'flex'
      errDiv.style.display       = 'none'
      renderTable(result)
      renderJSON(result)
      switchTab(activeTab)
    }

    function renderTable(result) {
      tblHead.innerHTML = '<tr>' + result.columns.map(function(c) {
        return '<th>' + esc(c) + '</th>'
      }).join('') + '</tr>'
      tblBody.innerHTML = result.rows.map(function(row) {
        return '<tr>' + result.columns.map(function(col) {
          return '<td title="' + esc(String(row[col] != null ? row[col] : '')) + '">'
               + esc(String(row[col] != null ? row[col] : '')) + '</td>'
        }).join('') + '</tr>'
      }).join('')
    }

    function renderJSON(result) {
      jsonPre.textContent = JSON.stringify(result, null, 2)
    }

    function showError(msg) {
      resetButtons()
      resultsEmpty.style.display = resultsTabs.style.display = 'none'
      tblWrap.style.display      = jsonDiv.style.display     = 'none'
      errDiv.style.display = 'block'
      errDiv.textContent   = msg
    }

    function showEmpty(msg) {
      resultsEmpty.style.display = 'flex'
      resultsEmpty.textContent   = msg
      resultsTabs.style.display  = 'none'
      tblWrap.style.display = jsonDiv.style.display = errDiv.style.display = 'none'
    }

    function switchTab(tab) {
      activeTab = tab
      document.querySelectorAll('.rtab').forEach(function(el) {
        el.classList.toggle('on', el.dataset.tab === tab)
      })
      tblWrap.style.display = tab === 'table' ? 'block' : 'none'
      jsonDiv.style.display = tab === 'json'  ? 'block' : 'none'
    }

    document.querySelectorAll('.rtab').forEach(function(el) {
      el.addEventListener('click', function() { switchTab(el.dataset.tab) })
    })

    copyBtn.addEventListener('click', function() {
      if (lastResult) { navigator.clipboard.writeText(JSON.stringify(lastResult.rows, null, 2)) }
    })

    function esc(str) {
      return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    }

    window.addEventListener('message', function(event) {
      var msg = event.data
      if (msg.type === 'queryResult')       { showResult(msg.payload) }
      else if (msg.type === 'queryError')   { showError(msg.payload) }
      else if (msg.type === 'connectionChanged') { setConnection(msg.payload) }
    })

    vscode.postMessage({ type: 'ready' })
  </script>
</body>
</html>`
  }
}
