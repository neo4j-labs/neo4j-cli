# neo4j-cli feedback

## Contents

- [neo4j-cli feedback disable](#neo4j-cli-feedback-disable)
- [neo4j-cli feedback enable](#neo4j-cli-feedback-enable)
- [neo4j-cli feedback register](#neo4j-cli-feedback-register)
- [neo4j-cli feedback status](#neo4j-cli-feedback-status)
- [neo4j-cli feedback submit](#neo4j-cli-feedback-submit)

Send feedback about neo4j-cli to the Neo4j team

Send feedback about neo4j-cli to the Neo4j team. Reports are short, structured and checked locally before anything is sent; see `neo4j-cli feedback submit --help`. Sending is only possible after a person enables it, with `neo4j-cli feedback enable` or by accepting the question the feedback tool asks the first time an agent reports.

Usage: `neo4j-cli feedback`

## neo4j-cli feedback disable

Revoke permission to send feedback

Revoke permission to send feedback immediately. Anyone can run this. Local receipts and the other feedback settings are kept. Unregisters the feedback tool from Claude Code and Codex. Run this before uninstalling neo4j-cli: otherwise the agent keeps a registration that points at a binary that no longer exists (to remove it by hand: `claude mcp remove -s user neo4j-feedback`, or `codex mcp remove neo4j-feedback`).

Usage: `neo4j-cli feedback disable`

Examples:

```
# Revoke feedback authorization
neo4j-cli feedback disable

# Revoke and print the resulting state as JSON
neo4j-cli feedback disable --format json
```

## neo4j-cli feedback enable

Authorize sending feedback reports (operator only)

Authorize this machine to send feedback reports for a limited time. This is an operator action and is refused when an agent harness is detected. Without --accept it prints what would be sent and exits with a usage error. Only the destination built into this build, or a loopback test endpoint, is accepted.

Usage: `neo4j-cli feedback enable [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--accept` | bool | false | Accept the disclosure and enable feedback |
| `--days` | int | 30 | Days until authorization expires (1-90) |
| `--endpoint` | string | - | Feedback endpoint URL (loopback test endpoint, or this build's destination) |
| `--register` | bool | true | Register the feedback tool with detected agents (Claude Code, Codex); use --register=false to skip |

Examples:

```
# Show what enabling would authorize (exits 2 until --accept is given)
neo4j-cli feedback enable --rw

# Enable for 30 days and print the resulting state as JSON
neo4j-cli feedback enable --accept --rw --format json
```

## neo4j-cli feedback register

Register the feedback tool with coding agents (does not enable sending)

Register the feedback tool with detected coding agents (Claude Code, Codex) without authorizing anything. Nothing is sent until a person accepts the question the tool asks the first time an agent tries to report, or runs `neo4j-cli feedback enable`. It changes the agents' configuration, so like other writes it needs --rw under an agent.

Usage: `neo4j-cli feedback register [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--endpoint` | string | - | Feedback endpoint URL (loopback test endpoint, or this build's destination) |

Examples:

```
# Register the tool; the first report will ask the person
neo4j-cli feedback register --rw

# Register and print the state as JSON
neo4j-cli feedback register --rw --format json
```

## neo4j-cli feedback status

Show whether feedback sending is enabled

Show whether feedback sending is currently authorized, where reports would go, when authorization expires, and where local receipts are kept.

Usage: `neo4j-cli feedback status`

Examples:

```
# Show the feedback state
neo4j-cli feedback status

# Show the feedback state as JSON
neo4j-cli feedback status --format json
```

## neo4j-cli feedback submit

Send a feedback report about neo4j-cli

Send one structured feedback report about neo4j-cli: a category, what you expected, and what you observed.

Describe product behavior in your own words. Do not include query text (Cypher), parameters, schema/label/property names, result rows, pasted command output or logs, file paths, connection URIs, hostnames, credentials, emails, names, or account/organization/database identifiers. Reports that appear to contain these are rejected locally and never sent; the check cannot catch everything, so leave such details out. This is product feedback, not a support request; no response is guaranteed. Keep expected and observed each under 255 characters.

Usage: `neo4j-cli feedback submit [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--agent-model` | string | - | Optional: public model name, e.g. claude-opus-5-5 |
| `--category` | string | - | Category: unclear_error, unexpected_behavior, missing_capability or other_friction |
| `--command` | string | - | Optional: the neo4j-cli command involved, e.g. query |
| `--error-code` | string | - | Optional: the CLI error code name, e.g. usage_error |
| `--expected` | string | - | What you expected neo4j-cli to do (product behavior, no query text or data) |
| `--observed` | string | - | What neo4j-cli actually did (product behavior, no query text or data) |
| `--request-id` | string | - | Optional: reuse a previous request id (UUIDv4) to retry safely |

Examples:

```
# Report a missing capability
neo4j-cli feedback submit --category missing_capability --expected "An output format suitable for spreadsheets" --observed "Only default, json, table and toon formats exist" --command query

# Report an unclear error and read the outcome as JSON
neo4j-cli feedback submit --category unclear_error --expected "The error should name the missing setting" --observed "The message did not say which setting to change" --error-code usage_error --format json
```

