// All domain types shared across the extension.
// These mirror the JSON shapes that neo4j-cli emits when called with --format json.
// Command reference: https://neo4j.sh

// ── Credentials ───────────────────────────────────────────────────────────────

export type CredentialType = 'aura-api' | 'neo4j-db' | 'embed'

export interface Credential {
  name:    string
  type:    CredentialType
  active:  boolean

  // neo4j-db fields
  uri?:      string
  username?: string

  // aura-api fields (secret is never returned in list output)
  clientId?: string

  // embed fields
  provider?:   string   // openai | ollama | huggingface
  model?:      string
  baseUrl?:    string
  dimensions?: number

  // enrichment from test results
  lastTestMs?:   number
  lastTestedAt?: string
}

// Kept for code that hasn't been updated yet
export type Connection = Credential

export type LocalInstanceStatus = 'running' | 'stopped'

export interface LocalInstance {
  name:      string
  version:   string
  edition:   'Enterprise' | 'Community'
  status:    LocalInstanceStatus
  port:      number
  boltUri:   string
  dataPath?: string
  uptime?:   string
}

// ── Aura ─────────────────────────────────────────────────────────────────────

export type AuraInstanceStatus = 'running' | 'paused' | 'loading' | 'destroying'

export interface AuraInstance {
  id:              string
  name:            string
  type:            string
  tier?:           string
  region:          string
  cloudProvider?:  string
  tenantId?:       string
  status:          AuraInstanceStatus
  boltUri:         string
  memoryGB?:       number
  storageGB?:      number
  neo4jVersion?:   string
  connectionName?: string
}

// Returned by `aura tenant list` and `aura tenant get`
export interface AuraTenantRegion {
  name:          string
  cloudProvider: string
}

export interface AuraTenant {
  id:       string
  name:     string
  regions?: AuraTenantRegion[]
}

export interface AISkill {
  agent:             string
  agentDisplayName:  string
  installed:         boolean
  installedVersion?: string
  latestVersion?:    string
  updateAvailable:   boolean
  skillsPath?:       string
  detectedAt?:       string
}

export interface TestResult {
  ok:       boolean
  ms?:      number
  version?: string
  error?:   string
}

// ── Webview ↔ extension host message contracts ────────────────────────────────

export type WebviewMessage =
  | { type: 'runQuery';     payload: { dbms?: string; query: string; write?: boolean } }
  | { type: 'explainQuery'; payload: { dbms?: string; query: string } }
  | { type: 'getSchema';    payload: { dbms?: string } }
  | { type: 'ready' }

export type HostMessage =
  | { type: 'queryResult';       payload: QueryResult }
  | { type: 'queryError';        payload: string }
  | { type: 'schemaResult';      payload: SchemaInfo }
  | { type: 'connectionChanged'; payload: Credential | null }

export interface QueryResult {
  columns:   string[]
  rows:      Record<string, unknown>[]
  ms:        number
  rowCount:  number
  plan?:     QueryPlan
  metadata?: {
    serverVersion?: string
    database?:      string
    resultCount?:   number
  }
}

export interface QueryPlan {
  operatorType: string
  rows:         number
  dbHits:       number
  children:     QueryPlan[]
  identifiers?: string[]
  details?:     string
}

export interface SchemaInfo {
  labels:            LabelInfo[]
  relationshipTypes: RelTypeInfo[]
  indexes:           IndexInfo[]
}

export interface LabelInfo {
  name:       string
  count:      number
  properties: string[]
}

export interface RelTypeInfo {
  name:  string
  count: number
}

export interface IndexInfo {
  name:          string
  labelsOrTypes: string[]
  properties:    string[]
  type:          string
  state:         'ONLINE' | 'POPULATING' | 'FAILED'
}
