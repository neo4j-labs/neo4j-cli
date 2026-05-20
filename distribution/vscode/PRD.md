# PRD — Neo4j VS Code Extension
**Status:** Draft  
**Last updated:** 2026-05-20  
**Author:** JG  

---

## 1. Overview

The Neo4j VS Code extension brings the `neo4j-cli` toolchain into the editor, removing the need to context-switch to a terminal or browser for common Neo4j operations. It wraps the CLI with a developer-friendly UI covering credentials, Aura cloud infrastructure, local Docker instances, Cypher queries, and AI agent skill management.

---

## 2. Goals

- Zero terminal required for the happy path of every supported operation.
- Zero browser required for Aura infrastructure management — the CLI provides all the data the UI needs, and leaving VS Code to provision infrastructure breaks developer flow state.
- Surface the right information at the right time — no hardcoded lists, always fetch live data from the CLI.
- Follow VS Code UX conventions so the extension feels native, not bolted-on.
- Treat the CLI as the source of truth; the extension is a UI layer only.

## 3. Non-goals

- Replacing Neo4j Browser or Bloom for data exploration.
- A full-featured Cypher IDE (syntax highlighting, schema-aware autocomplete) — deferred to v1.1.
- Cost estimation or billing UI.
- Instance resize or configuration changes on existing instances.
- Multi-instance query fan-out.
- Notebook-style query cells.

---

## 4. Users

**Primary:** Developers building applications on Neo4j who already use VS Code and have `neo4j-cli` installed or will install it via the extension's managed binary.

**Secondary:** DevOps/platform engineers managing Aura infrastructure who prefer CLI-adjacent tooling over the Aura console.

---

## 5. Lessons from comparable extensions

Research into MongoDB for VS Code, SQLTools, AWS Toolkit, and post-launch analyses of complex VS Code extensions surfaces five patterns that directly shape the design decisions in this PRD.

### 5.1 Nail the first-run experience and provisioning becomes straightforward

MongoDB for VS Code and the AWS Toolkit both chose to offload complex resource provisioning to a browser — but this was a data problem, not a design principle. Their extensions could not easily fetch the configuration options needed to populate a provisioning form dynamically; the browser console was the only place that data lived in a usable form.

The Neo4j CLI is different: it can return the complete data set needed to populate every dropdown in an Aura provisioning form — organisations, projects, permitted cloud providers, regions, instance types, and memory sizes. There is no data-availability reason to redirect to a browser, and there is a strong UX reason not to: bouncing a developer out of VS Code breaks the flow state the extension exists to protect.

The real lesson from the comparison is about *why* those extensions redirect. It is because their first-run experience is weak — users arrive at the provisioning step without having learned the credential and organisation model, so a complex in-editor form confuses them. The browser console has ambient context (logged in, org visible) that the extension has not yet established.

The correct response is to invest in the first-run experience so that by the time a developer opens the "new instance" panel, they already understand organisations, projects and credentials — because they configured them moments earlier. A well-designed provisioning form then feels like a natural continuation of that context.

**All six Aura instance types are handled entirely within VS Code. There is no fallback to the browser for any type.**

**Decision: the full provisioning panel is a v1 feature.** It is gated only on confirming OQ-1 below (one naming question), which is an implementation detail, not a design question.

### 5.2 Connection setup is the primary abandonment moment — and the enabler for everything else

MongoDB's first-run flow is one input: paste connection string, done. SQLTools' most cited frustration is connection model mismatch — one connection per database rather than per server, creating unnecessary friction from the start.

The Neo4j extension presents three credential types on first run. That conceptual load is the highest-risk moment in the user journey — but it is also the foundation that makes everything else work. A developer who successfully adds an aura-api credential has learned what an organisation is, what a client ID means, and how Aura authentication works. They arrive at the "new instance" panel with that context already in place.

**The first-run experience is not just retention hygiene — it is the enabler for in-editor provisioning.**

Design requirements:
- Every empty panel shows an actionable prompt, not a blank space.
- Adding an aura-api credential must complete in under 90 seconds.
- Adding a neo4j-db credential must complete in under 60 seconds.
- A "Quick connect" shortcut (single `neo4j+s://user:pass@host` URI) available for experienced users.

### 5.3 Silent failures cause abandonment

From the Kilo Code week-one retrospective: invalid or incomplete configuration left the extension in a broken state with no indication of what was wrong — empty panels, unresponsive UI, zero error messages. This was a top abandonment driver.

**Every failure state must produce an explicit, actionable message:**

| Failure | Message | Action offered |
|---|---|---|
| CLI not found | "neo4j-cli not found — click to install or set path" | Link to install docs / open settings |
| CLI wrong version | "neo4j-cli vX.Y found, vZ.W required — click to update" | Run `neo4j-cli update` |
| No aura-api credential | "Add an Aura API credential to manage instances" | Open add-credential flow, pre-set to aura-api |
| No neo4j-db credential | "Add a Neo4j DB credential to run queries" | Open add-credential flow, pre-set to neo4j-db |
| Network / API timeout | "Could not reach Aura API — check network or credential" | Refresh button |

Startup validation runs on activation and surfaces problems before the user tries to use any feature.

### 5.4 The query experience is what retains users long-term

MongoDB Playgrounds — rich editor with IntelliSense for operators and field names — are the extension's primary retention feature, not the connection manager. The Neo4j query panel is the equivalent. The schema sidebar, clean results rendering, and keyboard-first operation are the minimum viable retention experience.

### 5.5 Native feel over console replication

The extensions that survive feel like VS Code features, not web consoles in iframes. The status bar, tree views with context menus, and command palette integration are the right idioms.

---

## 6. Feature areas

### 6.1 First-run experience

The single highest-leverage investment.

**Activation sequence:**
1. Extension activates → checks for CLI binary.
2. CLI not found → notification with install/configure action; sidebar shows explicit onboarding state.
3. CLI found, no credentials → sidebar shows onboarding cards with guided setup.
4. Credentials found → normal state.

**Empty states — required for every panel:**

| Panel | Empty state message | Action |
|---|---|---|
| Credentials | "Connect to Neo4j to get started. Choose a credential type." | Three cards: Neo4j DB / Aura API / Embed |
| Aura | "Add an Aura API credential to manage cloud instances." | Button → add aura-api credential |
| Local | "No local instances. Create one to run Neo4j on Docker." | Button → new local instance |
| AI Skills | "No agents detected. Install neo4j-cli skills to get started." | Button → install skills |

**Quick connect** (experienced users):
- Command palette: `Neo4j: Quick Connect`
- Accepts `neo4j+s://username:password@host` or `neo4j://username:password@host`
- Parses credentials automatically; prompts only for a friendly name.
- Saves as neo4j-db credential and sets it active.

---

### 6.2 Credentials

Three types:

| Type | Purpose | Required fields |
|---|---|---|
| `neo4j-db` | Bolt connection for Cypher queries | URI, username, password, name |
| `aura-api` | Aura Console API for infrastructure management | Client ID, client secret, name |
| `embed` | Embedding provider for vector operations | Provider, model, name; API key (provider-dependent); base URL, dimensions (optional) |

**Sidebar panel** (`Credentials`)
- All credentials from the three CLI list commands, merged and tagged by type.
- Sort: active first → neo4j-db → aura-api → embed → alphabetical within each group.
- Each item: friendly name, type-appropriate secondary label, coloured icon (green = active).
- Context menu: **Set Active**, **Test Connection** (neo4j-db only), **Remove**.
- Empty state: onboarding cards.

**Add credential flow**
- Triggered by "+" toolbar button.
- Step 1: type picker with descriptions.
- Step 2: name (shared).
- Type-specific fields: neo4j-db = URI, username, password (4 inputs total); aura-api = client ID, client secret (4 total); embed = provider picker, model, conditional API key, optional base URL and dimensions.

**CLI commands**
```
credential aura-client list/use/add/remove --format json
credential dbms       list/use/add/remove --format json
credential embed      list/use/add/remove --format json
```

---

### 6.3 Aura infrastructure

**Sidebar panel** (`Aura`)
- Lists Aura instances for the active aura-api credential.
- Each item: name, type, status (running / paused / loading), region.
- Context menu: **Connect** (sets matching neo4j-db credential active), **Pause**, **Resume**, **Delete**.
- Empty state: actionable prompt to add an aura-api credential.
- Toolbar: **Refresh**, **New Instance**.

---

### 6.4 Create Aura Instance — dedicated webview panel

Opens as an editor panel (same architecture as the Query panel). All six instance types are provisioned directly in VS Code — there is no browser redirect for any type.

#### Why a dedicated panel

- The complete form is visible at once. The user sees every choice before committing.
- Cascading dropdowns show live data from the CLI — no hardcoded values anywhere.
- Any field can be changed before submitting. A QuickPick wizard has no back button.
- The panel persists; the user is not forced through a linear sequence.

#### Smart defaulting for instance type

When the panel opens, the extension checks the existing instance list in parallel with loading organisations:

- **No existing free-db** → pre-select `free-db` in the Instance type dropdown with a note: "Free tier — good starting point, change if needed."
- **Free-db already exists** → no pre-selection; user chooses from the full list. (Aura allows only one free-db per account; pre-selecting it again would produce a creation error.)

This surfaces the lowest-friction path for first-time users without restricting experienced ones. All six types remain accessible in every session.

#### Data loading cascade

```
Panel opens
  └── Parallel:
        · aura organization list --format json    → populate Organisation dropdown
        · aura instance list --format json         → free-db existence check

User selects organisation
  └── aura workspace use --organisation-id <id>
      aura project list --format json              → populate Project dropdown
                                                     (auto-select if only one)
User selects project
  └── aura tenant get <project-id> --format json  → ONE call, populates ALL:
        · Available cloud providers
        · Regions per cloud provider
        · Permitted instance types
        · Memory sizes per instance type
      (free-db skips this — its allocation is fixed)

User selects cloud provider → filter region list to that provider
User selects instance type  → filter memory to valid sizes; hide memory for free-db
```

#### Form fields

| # | Field | Type | Required | Notes |
|---|---|---|---|---|
| 1 | **Name** | Text input | No | Blank = Aura auto-generates (e.g. Instance01) |
| 2 | **Organisation** | Dropdown | Yes | From `aura organization list`; auto-selected if only one |
| 3 | **Project** | Dropdown | Yes | From `aura project list`; disabled until org selected |
| 4 | **Cloud provider** | Dropdown | Yes | From `tenant get`; auto-selected if only one |
| 5 | **Region** | Dropdown | Yes | Filtered by cloud provider |
| 6 | **Instance type** | Dropdown | Yes | All six types with descriptions; smart default (see above) |
| 7 | **Memory** | Dropdown | No | From `tenant get`; hidden for free-db |
| 8 | **Neo4j version** | Dropdown | No | From `tenant get`; default `5` |
| 9 | **Vector optimized** | Checkbox | No | `--vector-optimized` flag |
| 10 | **Graph analytics plugin** | Checkbox | No | `--graph-analytics-plugin`; auto-checked for `-ds` types |
| 11 | **Credential name** | Text input | No | `--credential-name`; placeholder shows `<id>-default` |

#### Instance type descriptions

| Type | Description |
|---|---|
| `free-db` | Free tier — fixed resources, shared infrastructure |
| `professional-db` | Pay-as-you-go graph database |
| `business-critical` | High availability, SLA-backed |
| `enterprise-db` | Enterprise features and HA |
| `professional-ds` | Graph database + Graph Data Science (GDS) |
| `enterprise-ds` | Enterprise graph database + GDS |

#### States and validation

- Each dropdown shows a loading indicator while its data is fetched.
- If a fetch fails, the affected field shows an inline error with **Retry**; upstream selections remain intact.
- **Create** button disabled until: organisation, project, cloud provider, region, and instance type are all selected.
- On submit: button shows a spinner and disables; other fields remain readable.

#### Post-creation

- Panel shows success state: instance name/ID, confirmation that credentials were auto-saved locally.
- Aura sidebar refreshes automatically.
- Panel stays open; user dismisses it or clicks **Create another** (resets form, preserving org and project selections).

#### CLI commands

```
aura organization list --format json                  → organisations
aura workspace use --organisation-id <id>             → set active org
aura project list --format json                       → projects for active org
aura tenant get <project-id> --format json            → all permitted configurations
aura instance list --format json                      → free-db existence check
aura instance create
  --type <type>                                       required; all six types
  --tenant-id <project-id>                            required; tenant = project
  --region <region>                                   required
  [--name <name>]
  [--cloud-provider aws|azure|gcp]
  [--memory <size>]                                   e.g. 8GB — omit for free-db
  [--version <major>]                                 default "5"
  [--vector-optimized]
  [--graph-analytics-plugin]
  [--credential-name <name>]
  [--customer-managed-key-id <id>]
  --rw --format json
```

---

### 6.5 Local Docker instances

**Sidebar panel** (`Local`)
- Lists Docker-managed Neo4j instances with running / stopped status.
- Context menu: **Start**, **Stop**, **Open Neo4j Browser**, **Remove**.
- New instance: name → version → edition (three QuickPick steps).

**CLI commands**
```
docker list/start/stop/create/remove
```

---

### 6.6 Cypher query window

Dedicated editor panel (`Neo4j: Query`).

**v1 requirements:**
- Connection strip: active credential name and last latency.
- Cypher editor (plain text in v1).
- Run / Explain buttons; `Ctrl+Enter` shortcut.
- Schema sidebar: labels with property names, relationship types, indexes.
- Results: Table and JSON tabs, row count, execution time.
- Error display distinguishing connection errors from query errors.
- Copy results as JSON.

**v1.1 targets:**
- Syntax highlighting via Cypher TextMate grammar.
- Basic keyword autocomplete.
- Label and property name autocomplete from live schema.

---

### 6.7 AI Skills

**Sidebar panel** (`AI Skills`)
- Lists detected AI agents with Neo4j skill status: installed / outdated / not installed.
- Context menu: **Install** / **Update**.

**CLI commands**
```
skill check --format json
skill install --rw
```

---

### 6.8 Status bar

Persistent item showing the active neo4j-db credential:

| State | Display |
|---|---|
| Connected | `● prod-db  23ms` |
| Connecting | `⟳ prod-db` |
| Error | `⚠ prod-db` (red background) |
| None | `○ no connection` (prominent background) |

Click opens a QuickPick to switch neo4j-db credential, open a query window, or manage credentials.

---

## 7. Extension architecture

```
src/
  extension.ts               Activation, startup validation, empty state handling
  types/index.ts             Shared domain types and webview message contracts
  services/
    cli.ts                   neo4j-cli wrapper — all CLI calls live here
    statusBar.ts             Status bar item and picker
    binaryManager.ts         Initial install + update delegation to neo4j-cli update
  providers/
    ConnectionsProvider.ts   Credentials tree view
    LocalProvider.ts         Docker instances tree view
    AuraProvider.ts          Aura instances tree view
    AISkillsProvider.ts      AI skills tree view
  commands/
    index.ts                 All command registrations
  panels/
    QueryPanel.ts            Cypher query webview
    CreateInstancePanel.ts   Create Aura instance webview
```

**Panel communication pattern:**
- Extension host → webview: `panel.webview.postMessage(msg)`
- Webview → extension host: `vscode.postMessage(msg)` + `onDidReceiveMessage` handler
- All message types defined in `types/index.ts`
- HTML self-contained in `buildHtml()` — no external scripts or stylesheets
- CSP nonce throughout; no inline `on*` handlers

---

## 8. CLI reference

### Hierarchy

```
Organisation  (aura organization list)
  └── Project / Tenant  (aura project list)
        └── Instance  (aura instance list / create / get / pause / resume / delete)
```

Note: The CLI uses the term **tenant** in flags and older commands (e.g. `--tenant-id`, `aura tenant get`). "Tenant" = "Project" throughout. The `--tenant-id` flag on `instance create` takes the **project ID**, not the organisation ID.

### Binary management

**Initial install** — downloads from GitHub Releases using the GoReleaser asset naming convention:

```
neo4j-cli_{version}_{Os}_{Arch}.tar.gz   (macOS / Linux)
neo4j-cli_{version}_{Os}_{Arch}.zip      (Windows)
```

Where `{Os}` is title-cased (`Darwin`, `Linux`, `Windows`) and `{Arch}` maps as:

| Node `os.arch()` | Asset arch |
|---|---|
| `x64` | `x86_64` |
| `arm64` | `arm64` |
| `ia32` | `i386` |

Example: `neo4j-cli_1.4.0_Darwin_arm64.tar.gz`

Base URL: `https://github.com/neo4j-labs/neo4j-cli/releases/download/v{version}/`

**Updates** — the CLI has a built-in self-update mechanism. The extension's `Neo4j: Update CLI` command delegates to `neo4j-cli update` rather than downloading from GitHub directly. This removes the need for the binary manager to handle update logic.

```
neo4j-cli update              # latest stable
neo4j-cli update check        # report availability, exit 1 if newer
neo4j-cli update --version v1.4.0  # pin or downgrade
```

**Post-install (macOS)** — remove quarantine attribute:
```
xattr -d com.apple.quarantine neo4j-cli
```

---

## 9. Open questions

| # | Question | Status |
|---|---|---|
| OQ-1 | Is the organisation list command `aura organization list` or `aura workspace list`? | ⚠️ Confirm exact command name |
| OQ-2 | CLI command to get permitted configurations for a project | ✅ `aura tenant get <project-id> --format json` |
| OQ-3 | Does `--tenant-id` refer to workspace/org ID or project ID? | ✅ Project ID (`tenant` = `project`) |
| OQ-4 | Are memory sizes per instance type available from the API? | ✅ Yes, in `tenant get` response |
| OQ-5 | GoReleaser asset naming for `neo4j-cli` | ✅ `neo4j-cli_{version}_{Os}_{Arch}.tar.gz` — see Section 8 |
| OQ-6 | Does `aura project list` return region data, or just IDs and names? | ✅ Flat JSON (IDs + names only); regions come from `tenant get` |

OQ-1 is the only remaining question before implementation of the Create Instance panel can begin.

---

## 10. Release plan

### v1
- First-run experience: startup validation, actionable empty states, onboarding cards throughout
- Credentials: all three types, full lifecycle; Quick Connect shortcut
- Aura sidebar: list, pause, resume, delete, connect
- **Create Aura Instance panel: all six instance types, organisation → project → config cascade, smart free-db defaulting**
- Local Docker instances: full lifecycle
- Cypher query panel: plain-text editor, schema sidebar, results table, error display
- AI Skills: list, install, update
- Status bar: connection state, quick switch
- Binary manager: initial download + `neo4j-cli update` delegation

### v1.1
- Cypher syntax highlighting and keyword autocomplete
- Label and property name autocomplete in query panel
- Schema browser panel

### v2
- Schema-aware Cypher autocomplete
- Query history and saved snippets
- Notebook-style query cells
