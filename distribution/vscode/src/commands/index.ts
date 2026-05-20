import * as vscode from 'vscode'
import type { CLIService } from '../services/cli'
import type { StatusBarService } from '../services/statusBar'
import type { AuraTenantRegion } from '../types/index'
import { CredentialsProvider, CredentialItem } from '../providers/ConnectionsProvider'
import { LocalProvider, LocalItem }            from '../providers/LocalProvider'
import { AuraProvider, AuraItem }              from '../providers/AuraProvider'
import { AISkillsProvider, AISkillItem }       from '../providers/AISkillsProvider'
import { QueryPanel }                          from '../panels/QueryPanel'

interface Deps {
  cli:                 CLIService
  statusBar:           StatusBarService
  connectionsProvider: CredentialsProvider
  localProvider:       LocalProvider
  auraProvider:        AuraProvider
  aiSkillsProvider:    AISkillsProvider
}

const EMBED_PROVIDERS = ['openai', 'ollama', 'huggingface'] as const

// All instance types with descriptions shown in the QuickPick.
// DS variants include Graph Data Science; business-critical adds HA.
type AuraInstanceTypeItem = vscode.QuickPickItem & { value: string; isFree: boolean }
const AURA_INSTANCE_TYPES: AuraInstanceTypeItem[] = [
  { value: 'free-db',          label: 'free-db',           detail: 'Free tier — fixed resources, shared infrastructure',          isFree: true  },
  { value: 'professional-db',  label: 'professional-db',   detail: 'Pay-as-you-go graph database',                                isFree: false },
  { value: 'business-critical',label: 'business-critical', detail: 'High availability, SLA-backed graph database',                isFree: false },
  { value: 'enterprise-db',    label: 'enterprise-db',     detail: 'Enterprise graph database — advanced features + HA',          isFree: false },
  { value: 'professional-ds',  label: 'professional-ds',   detail: 'Pay-as-you-go graph database + Graph Data Science (GDS)',     isFree: false },
  { value: 'enterprise-ds',    label: 'enterprise-ds',     detail: 'Enterprise graph database + Graph Data Science (GDS)',        isFree: false },
]

const MEMORY_OPTIONS = [
  { label: 'Default',  detail: 'Use the type default allocation', value: undefined as string | undefined },
  { label: '1GB',   value: '1GB',   detail: '' },
  { label: '2GB',   value: '2GB',   detail: '' },
  { label: '4GB',   value: '4GB',   detail: '' },
  { label: '8GB',   value: '8GB',   detail: 'Recommended for small production workloads' },
  { label: '16GB',  value: '16GB',  detail: '' },
  { label: '32GB',  value: '32GB',  detail: '' },
  { label: '64GB',  value: '64GB',  detail: '' },
  { label: '128GB', value: '128GB', detail: 'Large production workloads' },
  { label: '256GB', value: '256GB', detail: '' },
]

export function registerCommands(context: vscode.ExtensionContext, deps: Deps): void {
  const { cli, statusBar, connectionsProvider, localProvider, auraProvider, aiSkillsProvider } = deps

  const reg = (id: string, fn: (...args: unknown[]) => unknown) =>
    context.subscriptions.push(vscode.commands.registerCommand(id, fn))

  // ── Global ─────────────────────────────────────────────────────────────────

  reg('neo4j.refreshAll', () => {
    void connectionsProvider.load()
    void localProvider.load()
    void auraProvider.load()
    void aiSkillsProvider.load()
    void statusBar.refresh()
  })

  reg('neo4j.statusBar.click', () => void statusBar.showPicker())
  reg('neo4j.openManager',     () => QueryPanel.createOrShow(context, cli, 'connections'))
  reg('neo4j.query.new',       () => QueryPanel.createOrShow(context, cli, 'query'))

  // ── Credentials ────────────────────────────────────────────────────────────

  reg('neo4j.credential.refresh', () => void connectionsProvider.load())

  reg('neo4j.credential.add', async () => {
    type TypeItem = vscode.QuickPickItem & { credType: 'aura-api' | 'neo4j-db' | 'embed' }
    const typeChoice = await vscode.window.showQuickPick<TypeItem>(
      [
        { label: '$(cloud)        Aura API',  description: 'Client ID + Secret — for Aura infrastructure commands',           credType: 'aura-api' },
        { label: '$(database)     Neo4j DB',  description: 'Bolt URI + Username + Password — for running Cypher queries',    credType: 'neo4j-db' },
        { label: '$(symbol-array) Embed',     description: 'Embedding provider — for vector search and similarity operations', credType: 'embed' },
      ],
      { title: 'Add credential — choose type' },
    )
    if (!typeChoice) return

    const typeLabels = { 'aura-api': 'Aura API', 'neo4j-db': 'Neo4j DB', 'embed': 'Embed' }
    const name = await vscode.window.showInputBox({
      title:       `Add ${typeLabels[typeChoice.credType]} credential — name`,
      prompt:      'Friendly name',
      placeHolder: typeChoice.credType === 'aura-api' ? 'e.g. prod-aura' : typeChoice.credType === 'neo4j-db' ? 'e.g. prod-db' : 'e.g. openai-embed',
    })
    if (!name) return

    if (typeChoice.credType === 'aura-api') {
      const clientId = await vscode.window.showInputBox({ title: 'Add Aura API credential — Client ID', prompt: 'Client ID  (Aura console → Account settings)' })
      if (!clientId) return
      const clientSecret = await vscode.window.showInputBox({ title: 'Add Aura API credential — Client Secret', prompt: 'Client Secret', password: true })
      if (clientSecret === undefined) return
      await vscode.window.withProgress(
        { location: vscode.ProgressLocation.Notification, title: `Saving Aura API credential "${name}"…` },
        async () => { await cli.addAuraCredential({ name, clientId, clientSecret }); void connectionsProvider.load() }
      ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Failed to save credential: ${err.message}`))

    } else if (typeChoice.credType === 'neo4j-db') {
      const uri = await vscode.window.showInputBox({ title: 'Add Neo4j DB credential — Bolt URI', prompt: 'Bolt URI', placeHolder: 'neo4j+s://xxxxxxxx.databases.neo4j.io' })
      if (!uri) return
      const username = await vscode.window.showInputBox({ title: 'Add Neo4j DB credential — username', prompt: 'Username', value: 'neo4j' })
      if (!username) return
      const password = await vscode.window.showInputBox({ title: 'Add Neo4j DB credential — password', prompt: 'Password', password: true })
      if (password === undefined) return
      await vscode.window.withProgress(
        { location: vscode.ProgressLocation.Notification, title: `Saving Neo4j DB credential "${name}"…` },
        async () => { await cli.addDbmsCredential({ name, uri, username, password }); void connectionsProvider.load() }
      ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Failed to save credential: ${err.message}`))

    } else {
      const provider = await vscode.window.showQuickPick([...EMBED_PROVIDERS], { title: 'Add Embed credential — provider (required)' })
      if (!provider) return
      const modelPH: Record<string, string> = { openai: 'text-embedding-3-small', ollama: 'nomic-embed-text', huggingface: 'sentence-transformers/all-MiniLM-L6-v2' }
      const model = await vscode.window.showInputBox({ title: 'Add Embed credential — model (required)', prompt: 'Model name', placeHolder: modelPH[provider] ?? '' })
      if (!model) return
      const apiKeyRequired = provider === 'openai' || provider === 'huggingface'
      const apiKey = await vscode.window.showInputBox({ title: `Add Embed credential — API key${apiKeyRequired ? ' (required)' : ' (optional)'}`, prompt: apiKeyRequired ? `API key for ${provider}` : `API key for ${provider} — press Enter to skip`, password: true })
      if (apiKey === undefined) return
      if (apiKeyRequired && !apiKey) return
      const baseUrl = await vscode.window.showInputBox({ title: 'Add Embed credential — base URL (optional)', prompt: 'Override provider endpoint — press Enter to skip', placeHolder: provider === 'ollama' ? 'http://localhost:11434' : '' })
      if (baseUrl === undefined) return
      const dimRaw = await vscode.window.showInputBox({ title: 'Add Embed credential — dimensions (optional)', prompt: 'Embedding dimensions — press Enter for provider default', placeHolder: '0', validateInput: v => (v && !/^\d+$/.test(v)) ? 'Must be a whole number' : undefined })
      if (dimRaw === undefined) return
      const dimensions = dimRaw ? parseInt(dimRaw, 10) : undefined
      await vscode.window.withProgress(
        { location: vscode.ProgressLocation.Notification, title: `Saving Embed credential "${name}"…` },
        async () => {
          await cli.addEmbedCredential({ name, provider, model, apiKey: apiKey || undefined, baseUrl: baseUrl || undefined, dimensions: dimensions && dimensions > 0 ? dimensions : undefined })
          void connectionsProvider.load()
        }
      ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Failed to save credential: ${err.message}`))
    }
  })

  reg('neo4j.credential.use', async (item: unknown) => {
    if (!(item instanceof CredentialItem)) return
    await cli.useCredential(item.credential.name, item.credential.type)
    void connectionsProvider.load()
    void statusBar.refresh()
  })

  reg('neo4j.credential.test', async (item: unknown) => {
    if (!(item instanceof CredentialItem)) return
    if (item.credential.type !== 'neo4j-db') { void vscode.window.showInformationMessage('Test is only available for Neo4j DB credentials.'); return }
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Testing ${item.credential.name}…` },
      async () => {
        const result = await cli.testConnection(item.credential.name)
        if (result.ok) void vscode.window.showInformationMessage(`${item.credential.name}: connected · ${result.ms}ms${result.version ? ` · Neo4j ${result.version}` : ''}`)
        else void vscode.window.showErrorMessage(`${item.credential.name}: ${result.error ?? 'unreachable'}`)
        void connectionsProvider.load()
      }
    )
  })

  reg('neo4j.credential.remove', async (item: unknown) => {
    if (!(item instanceof CredentialItem)) return
    const typeLabel = { 'aura-api': 'Aura API', 'neo4j-db': 'Neo4j DB', 'embed': 'Embed' }
    const confirm = await vscode.window.showWarningMessage(`Remove ${typeLabel[item.credential.type]} credential "${item.credential.name}"?`, { modal: true }, 'Remove')
    if (confirm !== 'Remove') return
    await cli.removeCredential(item.credential.name, item.credential.type)
    void connectionsProvider.load()
    void statusBar.refresh()
  })

  for (const [old, neu] of [
    ['neo4j.connection.refresh', 'neo4j.credential.refresh'],
    ['neo4j.connection.add',     'neo4j.credential.add'],
    ['neo4j.connection.use',     'neo4j.credential.use'],
    ['neo4j.connection.test',    'neo4j.credential.test'],
    ['neo4j.connection.remove',  'neo4j.credential.remove'],
  ] as const) {
    reg(old, (...args: unknown[]) => vscode.commands.executeCommand(neu, ...args))
  }

  // ── Local instances ────────────────────────────────────────────────────────

  reg('neo4j.local.refresh', () => void localProvider.load())

  reg('neo4j.local.new', async () => {
    const name    = await vscode.window.showInputBox({ title: 'New local instance (1/3)', prompt: 'Instance name', placeHolder: 'neo4j-dev' })
    if (!name) return
    const version = await vscode.window.showQuickPick(['5.22 (latest)', '5.21', '5.20', '4.4 LTS'], { title: 'New local instance (2/3)', placeHolder: 'Neo4j version' })
    if (!version) return
    const edition = await vscode.window.showQuickPick(['Enterprise', 'Community'], { title: 'New local instance (3/3)', placeHolder: 'Edition' })
    if (!edition) return
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Launching ${name}…`, cancellable: false },
      async () => { await cli.createLocalInstance({ name, version: version.split(' ')[0]!, edition }); void localProvider.load() }
    ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Failed to create instance: ${err.message}`))
  })

  reg('neo4j.local.start', async (item: unknown) => {
    if (!(item instanceof LocalItem)) return
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Starting ${item.instance.name}…` },
      async () => { await cli.startLocalInstance(item.instance.name); void localProvider.load() }
    )
  })

  reg('neo4j.local.stop', async (item: unknown) => {
    if (!(item instanceof LocalItem)) return
    await cli.stopLocalInstance(item.instance.name)
    void localProvider.load()
  })

  reg('neo4j.local.openBrowser', (item: unknown) => {
    if (!(item instanceof LocalItem)) return
    void vscode.env.openExternal(vscode.Uri.parse(`http://localhost:${item.instance.port}`))
  })

  reg('neo4j.local.remove', async (item: unknown) => {
    if (!(item instanceof LocalItem)) return
    const confirm = await vscode.window.showWarningMessage(`Remove "${item.instance.name}"? All local data will be deleted.`, { modal: true }, 'Remove')
    if (confirm !== 'Remove') return
    await cli.removeLocalInstance(item.instance.name)
    void localProvider.load()
  })

  // ── Aura infrastructure ────────────────────────────────────────────────────

  reg('neo4j.aura.refresh', () => void auraProvider.load())

  reg('neo4j.aura.new', async () => {
    // ── 1. Instance type ──────────────────────────────────────────────────────
    // This is the primary decision — it determines pricing tier, GDS availability,
    // and whether memory selection applies (free-db has fixed resources).
    const typeChoice = await vscode.window.showQuickPick<AuraInstanceTypeItem>(
      AURA_INSTANCE_TYPES,
      { title: 'New Aura instance — type', matchOnDetail: true },
    )
    if (!typeChoice) return

    // ── 2. Tenant ─────────────────────────────────────────────────────────────
    let tenantId: string | undefined
    try {
      const tenants = await cli.listAuraTenants()
      if (tenants.length === 0) {
        void vscode.window.showErrorMessage('No Aura tenants found. Check your Aura API credential is set and active.')
        return
      }
      if (tenants.length === 1) {
        tenantId = tenants[0]!.id
      } else {
        const picked = await vscode.window.showQuickPick(
          tenants.map(t => ({ label: t.name, description: t.id, _id: t.id })),
          { title: 'New Aura instance — tenant' },
        )
        if (!picked) return
        tenantId = picked._id
      }
    } catch {
      // Aura API credential might not be configured — fall back to manual entry
      const manual = await vscode.window.showInputBox({
        title:       'New Aura instance — tenant ID',
        prompt:      'Tenant ID (from console.neo4j.io → Account settings)',
      })
      if (!manual) return
      tenantId = manual
    }
    if (!tenantId) return

    // ── 3. Regions — fetched live from tenant get ─────────────────────────────
    // `tenant get` returns the full list of available regions per cloud provider.
    // We use this to populate the cloud-provider and region pickers dynamically.
    let tenantRegions: AuraTenantRegion[] = []
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: 'Fetching available regions…', cancellable: false },
      async () => {
        try {
          const tenant = await cli.getAuraTenant(tenantId!)
          tenantRegions = tenant.regions ?? []
        } catch {
          tenantRegions = []
        }
      }
    )

    let cloudProvider: string | undefined
    let region:        string | undefined

    if (tenantRegions.length > 0) {
      const providers = [...new Set(tenantRegions.map(r => r.cloudProvider))]

      // Only show the cloud-provider step when there are multiple to choose from
      if (providers.length > 1) {
        type ProvItem = vscode.QuickPickItem & { value: string }
        const provChoice = await vscode.window.showQuickPick<ProvItem>(
          providers.map(p => ({ label: `$(cloud) ${p}`, value: p })),
          { title: 'New Aura instance — cloud provider' },
        )
        if (!provChoice) return
        cloudProvider = provChoice.value
      } else {
        cloudProvider = providers[0]
      }

      // Regions filtered to the chosen provider
      const available = tenantRegions.filter(r => r.cloudProvider === cloudProvider)
      type RegItem = vscode.QuickPickItem & { value: string }
      const regChoice = await vscode.window.showQuickPick<RegItem>(
        available.map(r => ({ label: r.name, value: r.name })),
        { title: `New Aura instance — region  (${cloudProvider})` },
      )
      if (!regChoice) return
      region = regChoice.value

    } else {
      // Fallback when tenant get failed or returned no region data
      const input = await vscode.window.showInputBox({
        title:       'New Aura instance — region',
        prompt:      'Region name',
        placeHolder: 'e.g. us-east-1 (AWS)  ·  eastus (Azure)  ·  europe-west1 (GCP)',
      })
      if (!input) return
      region = input
    }

    // ── 4. Memory (optional — skipped for free-db which has fixed resources) ──
    let memory: string | undefined
    if (!typeChoice.isFree) {
      type MemItem = vscode.QuickPickItem & { value: string | undefined }
      const memChoice = await vscode.window.showQuickPick<MemItem>(
        MEMORY_OPTIONS,
        { title: 'New Aura instance — memory (optional)' },
      )
      if (memChoice === undefined) return   // Escape cancels
      memory = memChoice.value
    }

    // ── 5. Name (optional — Aura auto-generates one if omitted) ──────────────
    const name = await vscode.window.showInputBox({
      title:       'New Aura instance — name (optional)',
      prompt:      'Display name — leave blank and Aura will auto-generate one (e.g. Instance01)',
      placeHolder: 'e.g. my-app-prod',
    })
    if (name === undefined) return   // Escape cancels; blank string is fine

    // ── 6. Advanced options (single multi-select — all optional) ─────────────
    type AdvItem = vscode.QuickPickItem & { key: 'vector' | 'gds' | 'version' | 'credName' }
    const advPick = await vscode.window.showQuickPick<AdvItem>(
      [
        { key: 'vector',   label: '$(symbol-namespace) Vector optimized',    description: 'Optimise storage and indexes for vector workloads',              picked: false },
        { key: 'gds',      label: '$(graph) Graph analytics plugin',         description: 'Enable the GDS plugin (automatic for -ds types)',               picked: false },
        { key: 'version',  label: '$(versions) Specify Neo4j version',       description: 'Override the default major version (default: 5)',               picked: false },
        { key: 'credName', label: '$(tag) Custom credential name',           description: 'Name for locally stored credentials  (default: <id>-default)',  picked: false },
      ],
      { title: 'New Aura instance — advanced options (optional)', canPickMany: true, placeHolder: 'Select options or press Enter to skip' },
    )
    if (advPick === undefined) return   // Escape cancels

    const selected = new Set(advPick.map(i => i.key))

    let neo4jVersion: string | undefined
    if (selected.has('version')) {
      const v = await vscode.window.showInputBox({ title: 'New Aura instance — Neo4j version', prompt: 'Major version', value: '5' })
      if (v === undefined) return
      neo4jVersion = v || '5'
    }

    let credentialName: string | undefined
    if (selected.has('credName')) {
      const cn = await vscode.window.showInputBox({ title: 'New Aura instance — credential name', prompt: 'Name for stored local credentials  (leave blank for default)', placeHolder: 'e.g. prod-aura-db' })
      if (cn === undefined) return
      credentialName = cn || undefined
    }

    // ── Provision ─────────────────────────────────────────────────────────────
    const displayName = name || typeChoice.value
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Provisioning "${displayName}"…`, cancellable: false },
      async () => {
        await cli.createAuraInstance({
          type:                 typeChoice.value,
          tenantId:             tenantId!,
          region:               region!,
          name:                 name       || undefined,
          cloudProvider,
          memory,
          version:              neo4jVersion,
          vectorOptimized:      selected.has('vector'),
          graphAnalyticsPlugin: selected.has('gds'),
          credentialName,
        })
        void auraProvider.load()
        void vscode.window.showInformationMessage(
          `Aura instance "${displayName}" is provisioning — credentials will be saved automatically.`
        )
      }
    ).then(undefined, (err: Error) =>
      vscode.window.showErrorMessage(`Failed to provision instance: ${err.message}`)
    )
  })

  reg('neo4j.aura.connect', async (item: unknown) => {
    if (!(item instanceof AuraItem)) return
    if (item.instance.connectionName) {
      await cli.useConnection(item.instance.connectionName)
      void connectionsProvider.load()
      void statusBar.refresh()
    } else {
      void vscode.window.showInformationMessage('No credential saved for this instance. Add a Neo4j DB credential via the Credentials view.')
    }
  })

  reg('neo4j.aura.pause', async (item: unknown) => {
    if (!(item instanceof AuraItem)) return
    await cli.pauseAuraInstance(item.instance.id)
    void auraProvider.load()
  })

  reg('neo4j.aura.resume', async (item: unknown) => {
    if (!(item instanceof AuraItem)) return
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: `Resuming ${item.instance.name}…` },
      async () => { await cli.resumeAuraInstance(item.instance.id); void auraProvider.load() }
    )
  })

  reg('neo4j.aura.openConsole', () =>
    void vscode.env.openExternal(vscode.Uri.parse('https://console.neo4j.io'))
  )

  reg('neo4j.aura.remove', async (item: unknown) => {
    if (!(item instanceof AuraItem)) return
    const confirm = await vscode.window.showWarningMessage(`Delete Aura instance "${item.instance.name}"? This is permanent.`, { modal: true }, 'Delete')
    if (confirm !== 'Delete') return
    await cli.deleteAuraInstance(item.instance.id)
    void auraProvider.load()
  })

  // ── AI Skills ──────────────────────────────────────────────────────────────

  reg('neo4j.skills.refresh', () => void aiSkillsProvider.load())

  reg('neo4j.skills.install', async () => {
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: 'Installing Neo4j skills to all detected agents…' },
      async () => { await cli.installAISkills(); void aiSkillsProvider.load() }
    ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Skill install failed: ${err.message}`))
  })

  reg('neo4j.skills.update', async () => {
    await vscode.window.withProgress(
      { location: vscode.ProgressLocation.Notification, title: 'Updating Neo4j skills across all detected agents…' },
      async () => {
        await cli.installAISkills()
        void aiSkillsProvider.load()
        void vscode.window.showInformationMessage('Neo4j skills updated across all detected agents.')
      }
    ).then(undefined, (err: Error) => vscode.window.showErrorMessage(`Skill update failed: ${err.message}`))
  })
}
