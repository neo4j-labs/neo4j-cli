# neo4j-cli desktop

## Contents

- [neo4j-cli desktop connection](#neo4j-cli-desktop-connection)
- [neo4j-cli desktop connection create](#neo4j-cli-desktop-connection-create)
- [neo4j-cli desktop connection delete](#neo4j-cli-desktop-connection-delete)
- [neo4j-cli desktop connection list](#neo4j-cli-desktop-connection-list)
- [neo4j-cli desktop connection update](#neo4j-cli-desktop-connection-update)
- [neo4j-cli desktop dbms](#neo4j-cli-desktop-dbms)
- [neo4j-cli desktop dbms create](#neo4j-cli-desktop-dbms-create)
- [neo4j-cli desktop dbms delete](#neo4j-cli-desktop-dbms-delete)
- [neo4j-cli desktop dbms list](#neo4j-cli-desktop-dbms-list)
- [neo4j-cli desktop dbms load](#neo4j-cli-desktop-dbms-load)
- [neo4j-cli desktop dbms plugin](#neo4j-cli-desktop-dbms-plugin)
- [neo4j-cli desktop dbms plugin available](#neo4j-cli-desktop-dbms-plugin-available)
- [neo4j-cli desktop dbms plugin install](#neo4j-cli-desktop-dbms-plugin-install)
- [neo4j-cli desktop dbms plugin list](#neo4j-cli-desktop-dbms-plugin-list)
- [neo4j-cli desktop dbms plugin uninstall](#neo4j-cli-desktop-dbms-plugin-uninstall)
- [neo4j-cli desktop dbms start](#neo4j-cli-desktop-dbms-start)
- [neo4j-cli desktop dbms stop](#neo4j-cli-desktop-dbms-stop)
- [neo4j-cli desktop dbms upgrade](#neo4j-cli-desktop-dbms-upgrade)
- [neo4j-cli desktop doctor](#neo4j-cli-desktop-doctor)
- [neo4j-cli desktop install](#neo4j-cli-desktop-install)
- [neo4j-cli desktop list](#neo4j-cli-desktop-list)

Manage DBMSes under a local Neo4j Desktop 2 install

Manage Neo4j Desktop 2 — local DBMSes (`dbms`), saved remote connections (`connection`), and install the Desktop app itself (`install`). `desktop list` shows DBMSes and saved connections together; use `desktop dbms list` or `desktop connection list` for single-resource views. Write commands (`dbms create/delete/start/stop`, `connection create/update/delete`, `install`) require `--rw`.

Usage: `neo4j-cli desktop`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | 0 | Pin the Desktop relate API to a specific port instead of probing 44222..44232 |

## neo4j-cli desktop connection

Manage saved remote DB connections registered with Neo4j Desktop 2

Manage the saved remote DB connection profiles Neo4j Desktop 2 stores via its local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Connections are remote Neo4j endpoints (Aura, self-hosted, …) the user has registered with Desktop; they appear under `Remote connections` in `neo4j-cli desktop list`. Use `neo4j-cli desktop connection list` for a connection-only view, or `neo4j-cli desktop list` for the composed view alongside local DBMSes. Use `neo4j-cli query --credential desktop-connection:<uuid>` to run Cypher against a saved connection without restating the URI / username / password. Write commands (`create`, `update`, `delete`) require `--rw`.

Usage: `neo4j-cli desktop connection`

### neo4j-cli desktop connection create

Register a saved remote DB connection with Neo4j Desktop 2

Register a saved remote DB connection profile with the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Saved connections appear under `Remote connections` in `neo4j-cli desktop list` and can be selected at query time via `--credential desktop-connection:<uuid>`. The password is stored by Desktop via its safeStorage mechanism on the `connection:<id>` key and is NOT written to `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle. `--password` is mandatory at runtime: pass it as a flag, or omit it on an interactive terminal to be prompted with no echo; non-TTY callers without `--password` fail with a usage error.

Usage: `neo4j-cli desktop connection create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--description` | string | - | Optional human-readable description for the saved connection |
| `--name` | string | - | (required) Human-readable name for the saved connection |
| `--password` | string | - | Password for the remote DB. Prefer the interactive TTY prompt (omit --password) so the value does not land in argv (`ps aux` / Task Manager) or in shell history. Required on non-TTY callers |
| `--uri` | string | - | (required) Bolt URI of the remote DB (e.g. neo4j+s://abc.databases.neo4j.io) |
| `--username` | string | - | (required) Username used to authenticate against the remote DB |

Examples:

```
# Create a saved connection against an Aura instance, passing the password as a flag
neo4j-cli desktop connection create --name aura-prod --uri neo4j+s://abc123.databases.neo4j.io --username neo4j --password supersecret --rw

# Create a saved connection and be prompted for the password interactively (TTY only)
neo4j-cli desktop connection create --name local-bolt --uri neo4j://localhost:7687 --username neo4j --rw

# Create a saved connection with a description, emitting the full Connection as JSON
neo4j-cli desktop connection create --name aura-dev --uri neo4j+s://xyz789.databases.neo4j.io --username neo4j --password supersecret --description "dev tier" --format json --rw
```

### neo4j-cli desktop connection delete

Delete a saved remote DB connection registered with Neo4j Desktop 2

Delete a saved remote DB connection profile by id. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. Desktop owns the saved connection's credential lifecycle — the `connection:<id>` safeStorage entry is removed by Desktop as part of the DELETE; this leaf does NOT mutate `~/.neo4j/cli/credentials.json`. Find connection ids with `neo4j-cli desktop list`.

Usage: `neo4j-cli desktop connection delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a saved connection with an interactive y/N confirmation
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --rw

# Delete a saved connection without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --rw

# Delete a saved connection and emit a machine-readable confirmation for scripting
neo4j-cli desktop connection delete f4e2f3c0-1111-2222-3333-444455556666 --yes --force --format json --rw
```

### neo4j-cli desktop connection list

List saved remote DB connections registered with Neo4j Desktop 2

List saved remote DB connections registered with the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Returns connections only; for the composed view that also includes local DBMSes see `neo4j-cli desktop list`. For local DBMSes alone see `neo4j-cli desktop dbms list`. `--format json` emits a JSON array of full `Connection` objects (every wire field Desktop returns). `--format toon` mirrors the JSON shape.

Usage: `neo4j-cli desktop connection list`

Examples:

```
# List saved remote connections as a table
neo4j-cli desktop connection list

# List saved remote connections as JSON (full Connection payload, agent-friendly)
neo4j-cli desktop connection list --format json

# List saved remote connections against a pinned port instead of probing 44222..44232
neo4j-cli desktop connection list --port 44225
```

### neo4j-cli desktop connection update

Update a saved remote DB connection registered with Neo4j Desktop 2

Update a saved remote DB connection profile by id. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. At least one of `--name --uri --username --password --description` must be supplied; the PATCH body contains ONLY the keys you set, so empty-string is a legitimate update for `--description`. `--password` with an empty value prompts interactively (no echo) on a TTY and fails with a usage error on a non-TTY, mirroring `desktop connection create`. Find connection ids with `neo4j-cli desktop list`.

Usage: `neo4j-cli desktop connection update <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--description` | string | - | New description for the saved connection. Pass an empty string to clear the existing description |
| `--name` | string | - | New human-readable name for the saved connection |
| `--password` | string | - | New password for the remote DB. Pass an empty value on a TTY to be prompted (no echo); fails on non-TTY |
| `--uri` | string | - | New Bolt URI for the remote DB (e.g. neo4j+s://abc.databases.neo4j.io) |
| `--username` | string | - | New username used to authenticate against the remote DB |

Examples:

```
# Rename a saved connection
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --name aura-prod-renamed --rw

# Rotate the password and update the URI in one PATCH
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --uri neo4j+s://new-host.databases.neo4j.io --password new-secret --rw

# Clear the description by sending an empty string and emit the updated Connection as JSON
neo4j-cli desktop connection update f4e2f3c0-1111-2222-3333-444455556666 --description "" --format json --rw
```

## neo4j-cli desktop dbms

Manage local DBMSes under a Neo4j Desktop 2 install

Manage local Neo4j DBMSes running under a Neo4j Desktop 2 install — list, create, delete, start, stop, upgrade. Write commands (`create`, `delete`, `start`, `stop`, `upgrade`) require `--rw`. For a composed view of DBMSes plus saved remote connections see `neo4j-cli desktop list`.

Usage: `neo4j-cli desktop dbms`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | int | 0 | Pin the Desktop relate API to a specific port instead of probing 44222..44232 |

### neo4j-cli desktop dbms create

Create a new DBMS under the local Neo4j Desktop 2 install

Create a new DBMS managed by the local Neo4j Desktop 2 install and start it. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Desktop 2 ships enterprise-only and the create route defaults to enterprise; the CLI does NOT expose an `--edition` flag because there is no choice. `--version` is optional: when omitted, the CLI queries Desktop's `GET /dbmss/versions` catalog and auto-picks the highest stable enterprise version (preferring already-cached entries on ties), emitting a stderr breadcrumb naming the picked version + origin. Desktop owns the credential lifecycle — the initial password is stored via Desktop's safeStorage and is NOT written to `~/.neo4j/cli/credentials.json`. Use `credential dbms add` separately if you want a persisted neo4j-cli profile pointing at this DBMS. A pre-flight check refuses to create+start when another DBMS is already running, since Neo4j Desktop 2 runs one DBMS at a time on port 7687; pass `--force` to stop the conflicting DBMS first and then proceed. By default the command returns as soon as Desktop's `POST /start` call resolves (the DBMS is created and the start request has been issued, but may still be transitioning). Pass `--wait` to block while the CLI polls every 1s for up to 30s for `status=started`, exiting non-zero if that threshold is exceeded.

Usage: `neo4j-cli desktop dbms create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Stop any other running Desktop DBMS first to free port 7687, then proceed. Without --force, the command refuses when another DBMS is running. |
| `--name` | string | - | (required) Name of the new DBMS |
| `--password` | string | - | Initial password for the `neo4j` user (stored by Desktop's safeStorage; not persisted in neo4j-cli credentials). Prefer the interactive TTY prompt (omit --password) so the value does not land in argv (`ps aux` / Task Manager) or in shell history. Required on non-TTY callers. |
| `--version` | string | - | Neo4j version (e.g. 2026.04.0 or 5.26.1). When omitted, picks the latest stable enterprise version Desktop knows about. |
| `--wait` | bool | false | Block until Desktop reports `status=started` for the new DBMS (polled every 1s, 30s ceiling). Without --wait the command returns as soon as the start request resolves. |

Examples:

```
# Create a DBMS using the latest stable enterprise version Desktop knows about (returns once start is issued)
neo4j-cli desktop dbms create --name my-dbms --password supersecret --rw

# Create a DBMS pinned to a specific version
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --rw

# Create a DBMS and block until it reports status=started
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --wait --rw

# Stop any other running DBMS first to free port 7687, then create+start this one
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --force --rw

# Create a DBMS and emit the full DbmsInfo as JSON for scripting
neo4j-cli desktop dbms create --name my-dbms --version 5.21.0 --password supersecret --format json --rw
```

### neo4j-cli desktop dbms delete

Delete a DBMS managed by the local Neo4j Desktop 2 install

Delete a DBMS managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Destructive: requires `--yes --force` (or a `y` answer at the TTY prompt) when invoked non-interactively. This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle (REQ-F-025); any persisted neo4j-cli credential pointing at this DBMS is left intact and must be cleaned up via `credential dbms remove`.

Usage: `neo4j-cli desktop dbms delete <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Confirm the destructive action. Required together with --yes for non-TTY callers. |
| `--yes` | bool | false | Confirm the destructive action. Required together with --force for non-TTY callers. |

Examples:

```
# Delete a DBMS with an interactive y/N confirmation
neo4j-cli desktop dbms delete my-dbms-id --rw

# Delete a DBMS without prompting (scripts, CI, non-TTY shells)
neo4j-cli desktop dbms delete my-dbms-id --yes --force --rw

# Delete a DBMS and emit a machine-readable confirmation for scripting
neo4j-cli desktop dbms delete my-dbms-id --yes --force --format json --rw
```

### neo4j-cli desktop dbms list

List local DBMSes managed by Neo4j Desktop 2

List local DBMSes managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Returns DBMSes only; for the composed view that also includes saved remote connections see `neo4j-cli desktop list`. For saved remote connections alone see `neo4j-cli desktop connection list`. `--format json` emits a JSON array of full `DbmsInfo` objects (every wire field Desktop returns). `--format toon` mirrors the JSON shape.

Usage: `neo4j-cli desktop dbms list`

Examples:

```
# List local DBMSes as a table
neo4j-cli desktop dbms list

# List local DBMSes as JSON (full DbmsInfo payload, agent-friendly)
neo4j-cli desktop dbms list --format json

# List local DBMSes against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms list --port 44225
```

### neo4j-cli desktop dbms load

Load an example dataset into a Neo4j Desktop 2 DBMS

Load an example Neo4j dataset (a `.dump` published by a GitHub repo carrying a `relate.project-install.json` manifest, e.g. `neo4j-graph-examples/movies`) into a DBMS managed by the local Neo4j Desktop 2 install. The manifest is resolved, the matching dump is downloaded from the Git-LFS media host, and the data is loaded into the `--database` (default `neo4j`). Exactly one of `--dbms-id` or `--name` is required (they are mutually exclusive). `--dbms-id <uuid>` targets an EXISTING DBMS: the load OVERWRITES that database's contents and therefore REQUIRES `--force`; the DBMS is stopped, the dump is restored, manifest plugins are installed, then it is restarted. `--name <name>` creates a NEW DBMS (newest stable enterprise version Desktop knows about, or pin one with `--version`), loads the dump, installs plugins, and starts it. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running.

Usage: `neo4j-cli desktop dbms load <owner/repo> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--database` | string | neo4j | The target database the dump is loaded into. |
| `--dbms-id` | string | - | ID of an EXISTING DBMS to overwrite (mutually exclusive with --name; requires --force). |
| `--force` | bool | false | Required to overwrite an EXISTING DBMS's database (the load destroys its current contents). Only valid with --dbms-id. |
| `--max-size` | int64 | 2147483648 | Maximum dump download size in bytes; the download is refused if exceeded. |
| `--name` | string | - | Name for a NEW DBMS to create and load the dataset into (mutually exclusive with --dbms-id). |
| `--password` | string | - | Initial password for the new DBMS's `neo4j` user (stored by Desktop's safeStorage). Prefer the interactive TTY prompt (omit --password) so the value does not land in argv. Ignored with --dbms-id. |
| `--version` | string | - | Neo4j version for the new DBMS (e.g. 5.26.1 or 2026.04.0). When omitted, the latest stable enterprise version Desktop knows about is used. Ignored with --dbms-id. |
| `--wait` | bool | false | Block until the new DBMS reports status=started (polled every 1s, 30s ceiling). Only meaningful with --name. |

Examples:

```
# Load the movies dataset into a brand-new Desktop DBMS
neo4j-cli desktop dbms load neo4j-graph-examples/movies --name movies --password supersecret --rw

# Overwrite an existing Desktop DBMS's data with a dataset (requires --force)
neo4j-cli desktop dbms load neo4j-graph-examples/movies --dbms-id 1234abcd --force --rw

# Load into a new DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms load neo4j-graph-examples/recommendations --name recs --password supersecret --format json --rw
```

### neo4j-cli desktop dbms plugin

Manage Neo4j plugins on a local Desktop-managed DBMS

Manage Neo4j plugins on a local Neo4j Desktop 2-managed DBMS — list installed plugins, browse the installable catalog, install, and uninstall. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. Plugin changes take effect after the DBMS is restarted; `install` and `uninstall` auto-restart a running DBMS (Stop → Start) unless `--no-restart` is passed. Write commands (`install`, `uninstall`) require `--rw`.

Usage: `neo4j-cli desktop dbms plugin`

#### neo4j-cli desktop dbms plugin available

List the installable plugin catalog for a local Desktop-managed DBMS

List the installable plugin catalog Neo4j Desktop 2 exposes for a local managed DBMS. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. `<dbms-id>` is the DBMS id (Desktop UUID); see `neo4j-cli desktop dbms list` for the catalog. `--format json` emits a JSON array of full `DbmsPlugin` objects (every wire field Desktop returns). `--format toon` mirrors the JSON shape. Pair with `neo4j-cli desktop dbms plugin install <dbms-id> <name>` to install one of the entries.

Usage: `neo4j-cli desktop dbms plugin available <dbms-id>`

Examples:

```
# Browse the installable plugin catalog for a DBMS as a table
neo4j-cli desktop dbms plugin available my-dbms-id

# List the installable catalog as JSON (full DbmsPlugin payload, agent-friendly)
neo4j-cli desktop dbms plugin available my-dbms-id --format json

# Browse against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms plugin available my-dbms-id --port 44225
```

#### neo4j-cli desktop dbms plugin install

Install a plugin on a local Desktop-managed DBMS

Install a Neo4j plugin on a DBMS managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. `--plugin` is the plugin name from `neo4j-cli desktop dbms plugin available <dbms-id>` (e.g. `apoc`) or an absolute path to a local plugin JAR; Desktop dispatches name-vs-path server-side. Plugin changes take effect only after the DBMS is restarted. By default this command auto-restarts a running DBMS (Stop → Start) so the new plugin becomes active immediately; pass `--no-restart` to defer the restart explicitly. If the DBMS is currently stopped, no restart is issued and the plugin will activate on the next start. The plugin install operation itself may take up to 2 minutes; the auto-restart adds up to a further 60 seconds (30s Stop poll + 30s Start poll).

Usage: `neo4j-cli desktop dbms plugin install <dbms-id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-restart` | bool | false | Do not auto-restart a running DBMS after the install; the plugin will activate only on the next manual start |
| `--plugin` | string | - | (required) Plugin name from the Desktop catalog (e.g. `apoc`) OR an absolute path to a local plugin JAR; Desktop dispatches name-vs-path server-side |

Examples:

```
# Install a named plugin from Desktop's catalog (auto-restart if running)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin apoc --rw

# Install a plugin from a local JAR path (auto-restart if running)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin /tmp/custom-plugin.jar --rw

# Install a plugin without auto-restarting the DBMS (will activate on next start)
neo4j-cli desktop dbms plugin install my-dbms-id --plugin apoc --no-restart --rw
```

#### neo4j-cli desktop dbms plugin list

List plugins installed on a local Desktop-managed DBMS

List plugins installed on a local Neo4j Desktop 2-managed DBMS. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. `<dbms-id>` is the DBMS id (Desktop UUID); see `neo4j-cli desktop dbms list` for the catalog. `--format json` emits a JSON array of full `DbmsPlugin` objects (every wire field Desktop returns). `--format toon` mirrors the JSON shape. A `pending_restart: true` entry means the plugin JAR is on disk but the running DBMS has not yet been restarted to pick it up — restart the DBMS or pass `--no-restart` to `install`/`uninstall` to defer the restart explicitly.

Usage: `neo4j-cli desktop dbms plugin list <dbms-id>`

Examples:

```
# List installed plugins on a DBMS as a table
neo4j-cli desktop dbms plugin list my-dbms-id

# List installed plugins as JSON (full DbmsPlugin payload, agent-friendly)
neo4j-cli desktop dbms plugin list my-dbms-id --format json

# List installed plugins against a pinned port instead of probing 44222..44232
neo4j-cli desktop dbms plugin list my-dbms-id --port 44225
```

#### neo4j-cli desktop dbms plugin uninstall

Uninstall a plugin from a local Desktop-managed DBMS

Uninstall a Neo4j plugin from a DBMS managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. `--plugin` is the plugin name from `neo4j-cli desktop dbms plugin list <dbms-id>` (e.g. `apoc`); relate matches it against the DBMS's installed plugin set. Plugin changes take effect only after the DBMS is restarted. By default this command auto-restarts a running DBMS (Stop → Start) so the JVM drops the plugin JAR immediately; pass `--no-restart` to defer the restart explicitly. If the DBMS is currently stopped, no restart is issued and the removal will be picked up on the next start. The uninstall is idempotent — removing an already-uninstalled plugin still exits 0 with the same confirmation shape. The plugin uninstall operation itself may take up to 2 minutes; the auto-restart adds up to a further 60 seconds (30s Stop poll + 30s Start poll).

Usage: `neo4j-cli desktop dbms plugin uninstall <dbms-id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-restart` | bool | false | Do not auto-restart a running DBMS after the uninstall; the running JVM will keep the plugin loaded until the next manual restart |
| `--plugin` | string | - | (required) Plugin name to uninstall (must match an entry from `neo4j-cli desktop dbms plugin list <dbms-id>`) |

Examples:

```
# Uninstall a plugin (auto-restart if the DBMS is running)
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --rw

# Uninstall a plugin without auto-restarting the DBMS (will deactivate on next restart)
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --no-restart --rw

# Uninstall a plugin and emit a machine-readable confirmation for scripting
neo4j-cli desktop dbms plugin uninstall my-dbms-id --plugin apoc --format json --rw
```

### neo4j-cli desktop dbms start

Start a DBMS managed by the local Neo4j Desktop 2 install

Start a DBMS managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. By default the command returns as soon as Desktop accepts the start request; the DBMS may still be booting. Pass `--wait` to poll every 1s for up to 30s until `status=started`; on timeout the command exits non-zero with the last-seen status. A pre-flight check refuses to start a second DBMS when another is already running, since Neo4j Desktop 2 runs one DBMS at a time on port 7687; pass `--force` to stop the conflicting DBMS first and then proceed. This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle.

Usage: `neo4j-cli desktop dbms start <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Stop any other running Desktop DBMS first to free port 7687, then proceed. Without --force, the command refuses when another DBMS is running. |
| `--wait` | bool | false | Poll every 1s for up to 30s until the DBMS reports status=started; exits non-zero on timeout |

Examples:

```
# Start a DBMS and return immediately (do not wait for boot)
neo4j-cli desktop dbms start my-dbms-id --rw

# Start a DBMS and wait until it reports status=started (30s ceiling)
neo4j-cli desktop dbms start my-dbms-id --wait --rw

# Stop any other running DBMS first to free port 7687, then start this one
neo4j-cli desktop dbms start my-dbms-id --force --rw

# Start a DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms start my-dbms-id --wait --format json --rw
```

### neo4j-cli desktop dbms stop

Stop a DBMS managed by the local Neo4j Desktop 2 install

Stop a DBMS managed by the local Neo4j Desktop 2 install. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. By default the command returns as soon as Desktop accepts the stop request; the DBMS may still be draining. Pass `--wait` to poll every 1s for up to 30s until `status=stopped`; on timeout the command exits non-zero with the last-seen status. This leaf does NOT mutate `~/.neo4j/cli/credentials.json` — Desktop owns the credential lifecycle.

Usage: `neo4j-cli desktop dbms stop <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | false | Poll every 1s for up to 30s until the DBMS reports status=stopped; exits non-zero on timeout |

Examples:

```
# Stop a DBMS and return immediately (do not wait for shutdown)
neo4j-cli desktop dbms stop my-dbms-id --rw

# Stop a DBMS and wait until it reports status=stopped (30s ceiling)
neo4j-cli desktop dbms stop my-dbms-id --wait --rw

# Stop a DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms stop my-dbms-id --wait --format json --rw
```

### neo4j-cli desktop dbms upgrade

Upgrade a DBMS managed by the local Neo4j Desktop 2 install

Upgrade a DBMS managed by the local Neo4j Desktop 2 install to a newer Neo4j version. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. `--version` is optional: when omitted, the CLI queries Desktop's `GET /dbmss/versions` catalog and auto-picks the highest stable enterprise version (preferring already-cached entries on ties), emitting a stderr breadcrumb naming the picked version + origin. Desktop upgrades a DBMS only while it is stopped: the command refuses when the target is running unless `--force` is passed, in which case it stops the DBMS (polling until stopped) and then upgrades. `--plugin-upgrade-mode` controls how installed plugins are migrated (`all`, `none`, or `upgradable`); `--no-migrate` skips the store-format migration; `--backup` (default true) takes a backup before upgrading. The upgrade can take several minutes; the command blocks until Desktop reports it complete and leaves the DBMS stopped — start it again with `neo4j-cli desktop dbms start <id> --rw`.

Usage: `neo4j-cli desktop dbms upgrade <id> [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--backup` | bool | true | Take a backup of the DBMS before upgrading. |
| `--force` | bool | false | Stop the DBMS first if it is running, then upgrade. Without --force, the command refuses when the DBMS is running. |
| `--no-migrate` | bool | false | Skip the store-format migration step during the upgrade. |
| `--plugin-upgrade-mode` | string | upgradable | How to migrate installed plugins during the upgrade: all, none, or upgradable. |
| `--version` | string | - | Neo4j version to upgrade to (e.g. 2026.04.0 or 5.26.1). When omitted, picks the latest stable enterprise version Desktop knows about. |

Examples:

```
# Upgrade a DBMS to the latest stable enterprise version Desktop knows about
neo4j-cli desktop dbms upgrade my-dbms-id --rw

# Upgrade a DBMS to a specific version
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --rw

# Stop the DBMS first if it is running, then upgrade it
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --force --rw

# Upgrade without a pre-upgrade backup and skip plugin migration
neo4j-cli desktop dbms upgrade my-dbms-id --backup=false --plugin-upgrade-mode none --rw

# Upgrade a DBMS and emit the upgraded DbmsInfo as JSON for scripting
neo4j-cli desktop dbms upgrade my-dbms-id --version 5.26.1 --format json --rw
```

## neo4j-cli desktop doctor

Diagnose a local Neo4j Desktop 2 install end-to-end

Run an ordered sequence of seven health checks against the local Neo4j Desktop 2 install: (1) install present, (2) mDNS discovery, (3) standard port probe, (4) Desktop info (version, app path, data path), (5) data directory present, (6) auth data readable, (7) authenticated probe. Each check produces a `{name, status, detail, hint?}` record; when a check FAILs, dependent later checks render as `skip` with a `(depends on …)` detail. Discovery tries mDNS first, then the 44222..44232 fallback scan. The mDNS-discovery check is purely diagnostic: it reports the advertised port when a responder answers and renders as INFO (never blocking) otherwise. The Desktop-info check is also purely diagnostic: an unavailable `/info/app` endpoint (older Desktop) renders as INFO and never blocks subsequent checks. `--format json` (or `toon`) emits a single `{checks: [...], summary: {reachable, port?, standard_port_range, next_step?}}` document for agent consumption. Default-TTY table format renders aligned name / status / detail columns and a trailing one-line summary. Inherits `--port <n>` from the `desktop` parent: when set, the standard-port probe tries only that port instead of the 44222..44232 fallback scan. The leaf is read-only and always exits 0 — parse `summary.reachable` to gate downstream actions.

Usage: `neo4j-cli desktop doctor`

Examples:

```
# Run all seven checks against the local Desktop install (default table output)
neo4j-cli desktop doctor

# Pin the probe to a specific port instead of the 44222..44232 fallback scan
neo4j-cli desktop doctor --port 44222

# Emit a structured JSON report suitable for agent consumption
neo4j-cli desktop doctor --format json
```

## neo4j-cli desktop install

Install Neo4j Desktop 2 from the public publish feed

Install Neo4j Desktop 2 on the local machine. Already-installed detection (macOS `.app` + `Info.plist`, Linux AppImage glob under `~/Applications`, Windows `%LOCALAPPDATA%\Programs\neo4j-desktop`) runs BEFORE any network call: on a hit the command prints `Neo4j Desktop 2 already installed at <path> (version <X>). Pass --force to re-install.` and exits 0 unless `--force` is supplied. On a clean system the command fetches the per-OS electron-builder manifest from `dist.neo4j.org/neo4j-desktop-2/...`, picks the platform artifact (DMG / AppImage / NSIS .exe), downloads it to a tempfile, verifies its base64-decoded SHA-512 against the manifest entry, and then dispatches to the per-OS install action. The command does NOT prompt for license acceptance (REQ-F-021) and does NOT auto-launch Desktop on success (REQ-F-022) — a stderr next-step hint is printed instead. Linux arm64 hard-errors with a deployment-center URL because no upstream arm64 build is published.

Usage: `neo4j-cli desktop install [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | bool | false | Resolve the manifest + artifact URL and print them; skip download and install |
| `--force` | bool | false | Re-install regardless of already-installed detection |
| `--target-dir` | string | - | Override the per-OS default install directory |

Examples:

```
# Install Neo4j Desktop 2 using the per-OS default target dir
neo4j-cli desktop install --rw

# Re-install over an existing on-disk Desktop install
neo4j-cli desktop install --force --rw

# Resolve the manifest + artifact URL without downloading or installing
neo4j-cli desktop install --dry-run --rw
```

## neo4j-cli desktop list

List local DBMSes and saved remote connections managed by Neo4j Desktop 2

List local DBMSes and saved remote connections managed by the local Neo4j Desktop 2 install — composed view. Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running. For single-resource views use `neo4j-cli desktop dbms list` (DBMSes only) or `neo4j-cli desktop connection list` (connections only). Table format renders two labelled sections: `Local DBMSes` (id, name, version, status, connection_uri) and `Remote connections` (id, name, connection_uri). `--format json` emits `{"dbmss": [...], "connections": [...]}` carrying the full wire payload for each. `--format toon` mirrors the JSON shape.

Usage: `neo4j-cli desktop list`

Examples:

```
# List DBMSes and saved remote connections as a two-section table
neo4j-cli desktop list

# List as JSON (full payload, agent-friendly) — shape: {dbmss, connections}
neo4j-cli desktop list --format json

# List against a pinned port instead of probing 44222..44232
neo4j-cli desktop list --port 44225
```

