import { execFile } from 'child_process'
import { promisify } from 'util'
import type {
  Credential, CredentialType,
  LocalInstance, AuraInstance, AuraTenant, AuraTenantRegion,
  AISkill, TestResult, QueryResult, SchemaInfo,
} from '../types/index'

const execFileAsync = promisify(execFile)

// CLIService wraps the neo4j-cli binary.
// Command reference: https://neo4j.sh
//
// Credential commands — one list call per type, type-specific add / use / remove:
//   neo4j-cli credential aura-client list --format json
//   neo4j-cli credential aura-client use <name>
//   neo4j-cli credential aura-client add --name <n> --client-id <id> --client-secret <s> --rw
//   neo4j-cli credential aura-client remove <name> --rw
//
//   neo4j-cli credential dbms list --format json
//   neo4j-cli credential dbms use <name>
//   neo4j-cli credential dbms add --name <n> --uri <u> --username <u> --password <p> --rw
//   neo4j-cli credential dbms remove <name> --rw
//
//   neo4j-cli credential embed list --format json
//   neo4j-cli credential embed use <name>
//   neo4j-cli credential embed add --name <n> --provider <p> --model <m>
//                                  [--api-key <k>] [--base-url <u>] [--dimensions <d>] --rw
//   neo4j-cli credential embed remove <name> --rw
//
// Aura infrastructure:
//   neo4j-cli aura tenant list --format json
//   neo4j-cli aura tenant get <id> --format json
//   neo4j-cli aura instance list --format json
//   neo4j-cli aura instance get <id> --format json
//   neo4j-cli aura instance create
//       --type <type>                  (required) free-db | professional-db | business-critical |
//                                                enterprise-db | professional-ds | enterprise-ds
//       --tenant-id <id>              (required)
//       --region <region>             (required)
//       [--name <name>]               optional; auto-generated if omitted
//       [--cloud-provider aws|azure|gcp]
//       [--memory <size>]             e.g. 8GB, 64GB
//       [--version <major>]           default "5"
//       [--vector-optimized]          flag
//       [--graph-analytics-plugin]    flag
//       [--credential-name <name>]    name for stored credentials
//       [--customer-managed-key-id <id>]
//       [--no-credential-storage]     flag
//       --rw --format json
//   neo4j-cli aura instance pause <id> --rw
//   neo4j-cli aura instance resume <id> --rw
//   neo4j-cli aura instance delete <id> --rw
//
// Docker (local) instances:
//   neo4j-cli docker list --format json
//   neo4j-cli docker start <name>
//   neo4j-cli docker stop <name>
//   neo4j-cli docker create --name <n> --version <v> --edition <e>
//   neo4j-cli docker remove <name>
//
// Query / Cypher:
//   neo4j-cli query '<cypher>' [--dbms <name>] --format json
//   neo4j-cli query '<cypher>' --rw [--dbms <name>] --format json
//   neo4j-cli query 'EXPLAIN <cypher>' [--dbms <name>] --format json
//   neo4j-cli query ':schema' [--dbms <name>] --format json
//
// Skills (AI agent integration):
//   neo4j-cli skill check --format json
//   neo4j-cli skill install
//
// Note: --rw is required for all state-mutating commands.
//       --format json must always be the last flag.

const CRED_SUBCMD: Record<CredentialType, string> = {
  'aura-api': 'aura-client',
  'neo4j-db': 'dbms',
  'embed':    'embed',
}

export class CLIError extends Error {
  constructor(
    message: string,
    public readonly exitCode: number | null,
    public readonly stderr: string,
  ) {
    super(message)
    this.name = 'CLIError'
  }
}

export class CLINotFoundError extends Error {
  constructor(public readonly cliPath: string) {
    super(`neo4j-cli not found at "${cliPath}". Check the neo4j.cliPath setting.`)
    this.name = 'CLINotFoundError'
  }
}

export class CLIService {
  constructor(private readonly cliPath: string) {}

  // ── Core execution ──────────────────────────────────────────────────────────

  private async exec(args: string[]): Promise<string> {
    try {
      const { stdout } = await execFileAsync(this.cliPath, args, { timeout: 30_000 })
      return stdout.trim()
    } catch (err: unknown) {
      const e = err as NodeJS.ErrnoException & { stderr?: string; code?: number | string }
      if (e.code === 'ENOENT') throw new CLINotFoundError(this.cliPath)
      throw new CLIError(
        e.message ?? 'CLI command failed',
        typeof e.code === 'number' ? e.code : null,
        e.stderr ?? '',
      )
    }
  }

  private parse(raw: string): unknown {
    try {
      return JSON.parse(raw)
    } catch {
      throw new CLIError(`CLI returned non-JSON output: ${raw.slice(0, 200)}`, null, '')
    }
  }

  private async execJSON<T>(args: string[]): Promise<T> {
    const raw    = await this.exec([...args, '--format', 'json'])
    const parsed = this.parse(raw) as Record<string, unknown>
    if (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed) && 'data' in parsed) {
      return parsed['data'] as T
    }
    return parsed as T
  }

  private async execJSONList<T>(args: string[]): Promise<T[]> {
    const raw    = await this.exec([...args, '--format', 'json'])
    const parsed = this.parse(raw)
    if (Array.isArray(parsed)) return parsed as T[]
    if (parsed !== null && typeof parsed === 'object') {
      const w = parsed as Record<string, unknown>
      if (Array.isArray(w['data'])) return w['data'] as T[]
    }
    console.warn('neo4j-cli: unexpected list response shape:', JSON.stringify(parsed).slice(0, 200))
    return []
  }

  // ── Credentials ─────────────────────────────────────────────────────────────

  async listCredentials(): Promise<Credential[]> {
    const [auraResult, dbmsResult, embedResult] = await Promise.allSettled([
      this.execJSONList<Omit<Credential, 'type'>>(['credential', 'aura-client', 'list']),
      this.execJSONList<Omit<Credential, 'type'>>(['credential', 'dbms',        'list']),
      this.execJSONList<Omit<Credential, 'type'>>(['credential', 'embed',       'list']),
    ])

    const out: Credential[] = []
    if (auraResult.status === 'fulfilled') out.push(...auraResult.value.map(c => ({ ...c, type: 'aura-api' as const })))
    if (dbmsResult.status  === 'fulfilled') out.push(...dbmsResult.value.map(c => ({ ...c, type: 'neo4j-db' as const })))
    if (embedResult.status === 'fulfilled') out.push(...embedResult.value.map(c => ({ ...c, type: 'embed'    as const })))
    return out
  }

  async getActiveConnection(): Promise<Credential | null> {
    const creds = await this.listCredentials()
    return creds.find(c => c.type === 'neo4j-db' && c.active) ?? null
  }

  async useCredential(name: string, type: CredentialType): Promise<void> {
    await this.exec(['credential', CRED_SUBCMD[type], 'use', name])
  }

  async useConnection(name: string): Promise<void> {
    await this.exec(['credential', 'dbms', 'use', name])
  }

  async removeCredential(name: string, type: CredentialType): Promise<void> {
    await this.exec(['credential', CRED_SUBCMD[type], 'remove', name, '--rw'])
  }

  async addDbmsCredential(params: {
    name:     string
    uri:      string
    username: string
    password: string
  }): Promise<void> {
    await this.exec(['credential', 'dbms', 'add',
      '--name',     params.name,
      '--uri',      params.uri,
      '--username', params.username,
      '--password', params.password,
      '--rw',
    ])
  }

  async addAuraCredential(params: {
    name:         string
    clientId:     string
    clientSecret: string
  }): Promise<void> {
    await this.exec(['credential', 'aura-client', 'add',
      '--name',          params.name,
      '--client-id',     params.clientId,
      '--client-secret', params.clientSecret,
      '--rw',
    ])
  }

  async addEmbedCredential(params: {
    name:        string
    provider:    string
    model:       string
    apiKey?:     string
    baseUrl?:    string
    dimensions?: number
  }): Promise<void> {
    const args = ['credential', 'embed', 'add',
      '--name',     params.name,
      '--provider', params.provider,
      '--model',    params.model,
    ]
    if (params.apiKey)                   args.push('--api-key',    params.apiKey)
    if (params.baseUrl)                  args.push('--base-url',   params.baseUrl)
    if (params.dimensions !== undefined) args.push('--dimensions', String(params.dimensions))
    args.push('--rw')
    await this.exec(args)
  }

  async testConnection(name: string): Promise<TestResult> {
    const start = Date.now()
    try {
      const result = await this.execJSON<QueryResult>(['query', 'RETURN 1 AS ok', '--dbms', name])
      return { ok: true, ms: Date.now() - start, version: result.metadata?.serverVersion }
    } catch (err: unknown) {
      return { ok: false, error: err instanceof Error ? err.message : String(err) }
    }
  }

  // ── Docker (local) instances ─────────────────────────────────────────────────

  async listLocalInstances(): Promise<LocalInstance[]> {
    return this.execJSONList<LocalInstance>(['docker', 'list'])
  }

  async startLocalInstance(name: string): Promise<void> {
    await this.exec(['docker', 'start', name])
  }

  async stopLocalInstance(name: string): Promise<void> {
    await this.exec(['docker', 'stop', name])
  }

  async createLocalInstance(params: { name: string; version: string; edition: string }): Promise<void> {
    await this.exec(['docker', 'create',
      '--name',    params.name,
      '--version', params.version,
      '--edition', params.edition,
    ])
  }

  async removeLocalInstance(name: string): Promise<void> {
    await this.exec(['docker', 'remove', name])
  }

  // ── Aura infrastructure ──────────────────────────────────────────────────────

  async listAuraInstances(): Promise<AuraInstance[]> {
    return this.execJSONList<AuraInstance>(['aura', 'instance', 'list'])
  }

  async getAuraInstance(id: string): Promise<AuraInstance> {
    return this.execJSON<AuraInstance>(['aura', 'instance', 'get', id])
  }

  async listAuraTenants(): Promise<AuraTenant[]> {
    return this.execJSONList<AuraTenant>(['aura', 'tenant', 'list'])
  }

  // Returns full tenant detail including available regions per cloud provider.
  async getAuraTenant(id: string): Promise<AuraTenant> {
    return this.execJSON<AuraTenant>(['aura', 'tenant', 'get', id])
  }

  async createAuraInstance(params: {
    // Required
    type:                  string   // free-db | professional-db | business-critical | enterprise-db | professional-ds | enterprise-ds
    tenantId:              string
    region:                string
    // Optional
    name?:                 string   // auto-generated if omitted
    cloudProvider?:        string   // aws | azure | gcp
    memory?:               string   // e.g. "8GB"
    version?:              string   // major version, default "5"
    vectorOptimized?:      boolean
    graphAnalyticsPlugin?: boolean
    credentialName?:       string   // name for stored credentials
    customerManagedKeyId?: string
    noCredentialStorage?:  boolean
  }): Promise<AuraInstance> {
    const args = ['aura', 'instance', 'create',
      '--type',      params.type,
      '--tenant-id', params.tenantId,
      '--region',    params.region,
    ]
    if (params.name)                  args.push('--name',                     params.name)
    if (params.cloudProvider)         args.push('--cloud-provider',           params.cloudProvider)
    if (params.memory)                args.push('--memory',                   params.memory)
    if (params.version)               args.push('--version',                  params.version)
    if (params.vectorOptimized)       args.push('--vector-optimized')
    if (params.graphAnalyticsPlugin)  args.push('--graph-analytics-plugin')
    if (params.credentialName)        args.push('--credential-name',          params.credentialName)
    if (params.customerManagedKeyId)  args.push('--customer-managed-key-id',  params.customerManagedKeyId)
    if (params.noCredentialStorage)   args.push('--no-credential-storage')
    args.push('--rw')
    return this.execJSON<AuraInstance>(args)
  }

  async pauseAuraInstance(id: string): Promise<void> {
    await this.exec(['aura', 'instance', 'pause', id, '--rw'])
  }

  async resumeAuraInstance(id: string): Promise<void> {
    await this.exec(['aura', 'instance', 'resume', id, '--rw'])
  }

  async deleteAuraInstance(id: string): Promise<void> {
    await this.exec(['aura', 'instance', 'delete', id, '--rw'])
  }

  // ── Cypher queries ───────────────────────────────────────────────────────────

  async runCypher(params: { dbms?: string; query: string; write?: boolean }): Promise<QueryResult> {
    const args = ['query', params.query]
    if (params.dbms)  args.push('--dbms', params.dbms)
    if (params.write) args.push('--rw')
    return this.execJSON<QueryResult>(args)
  }

  async explainCypher(params: { dbms?: string; query: string }): Promise<QueryResult> {
    const args = ['query', `EXPLAIN ${params.query}`]
    if (params.dbms) args.push('--dbms', params.dbms)
    return this.execJSON<QueryResult>(args)
  }

  async getSchema(dbms?: string): Promise<SchemaInfo> {
    const args = ['query', ':schema']
    if (dbms) args.push('--dbms', dbms)
    return this.execJSON<SchemaInfo>(args)
  }

  // ── AI Skills ────────────────────────────────────────────────────────────────

  async listAISkills(): Promise<AISkill[]> {
    return this.execJSONList<AISkill>(['skill', 'check'])
  }

  async installAISkills(): Promise<void> {
    await this.exec(['skill', 'install', '--rw'])
  }
}
