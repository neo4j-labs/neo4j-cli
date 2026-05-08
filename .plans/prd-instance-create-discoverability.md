# PRD: Instance Create Option Discoverability

## Overview

The `neo4j aura instance create` command is difficult to use in practice because users have no in-command guidance on what values are valid for `--region`, `--cloud-provider`, and `--type`. The current workaround (run `tenant list`, find the tenant ID, run `tenant get`, and manually parse ~2643 possible configurations) is a significant barrier for new users. This feature improves the situation through static help text enhancements: a curated `Examples` section and improved flag descriptions, with a pointer to `tenant get` for the full list.

## Goals

- Reduce friction when creating a non-free-db instance by making valid flag values visible without leaving the command.
- Clarify that `--region` accepts cloud-provider-specific region identifiers (GCP/AWS/Azure naming conventions differ).
- Show representative examples for all three cloud providers and the most common instance types.
- Preserve simplicity — no new subcommands, no extra API calls.

## Non-Goals

- Dynamic region/configuration validation via the API.
- An interactive guided creation mode.
- A new `instance configurations` subcommand.
- Exhaustive listing of all regions (30+) inline in help text.
- Any changes to flag parsing, validation logic, or command behaviour.

## Requirements

### Functional Requirements

- REQ-F-001: Add a cobra `Example` field to `NewCreateCmd` that shows at least four representative invocations:
  1. `free-db` (no region/cloud-provider needed).
  2. `professional-db` on AWS with a US East region.
  3. `professional-db` on Azure with a US East region.
  4. `professional-db` on GCP with a Europe West region.
  5. One `business-critical` example to illustrate an enterprise-tier option.
- REQ-F-002: Each example must include `--name`, `--type`, `--tenant-id` (or a note about the default-tenant shortcut), `--cloud-provider`, `--region`, and `--memory` where required.
- REQ-F-003: Update the `--region` flag usage string to state that values are cloud-provider-specific region identifiers (e.g. `us-east-1` for AWS, `eastus` for Azure, `europe-west1` for GCP) and that the full list for a tenant is available via `tenant get`.
- REQ-F-004: Update the `Long` description to more prominently surface the `tenant get` discovery path and to note that regions follow each cloud provider's own naming convention.
- REQ-F-005: The Examples section must show concrete, copy-paste-ready commands (real region identifiers from the data below, not placeholders).

### Non-Functional Requirements

- REQ-NF-001: No behaviour change — only `Short`, `Long`, `Example`, and flag `Usage` strings are modified.
- REQ-NF-002: Changes must not break `TestGenerator_RoundTrip` (skill bundle test). Run `go generate ./neo4j-cli/internal/skill/...` after editing the command tree to regenerate bundles.
- REQ-NF-003: All existing tests must continue to pass (`make test`, `make fmt-check`, `make lint`).

### Functional Requirements — Flag Usage Strings

- REQ-F-006: Update the `--type` flag usage string to enumerate all valid values with "must be one of" wording: `(required) The type of the instance. Must be one of "free-db", "professional-db", "business-critical", "enterprise-db", "professional-ds", or "enterprise-ds".`
- REQ-F-007: Update the `--cloud-provider` flag usage string to enumerate all valid values with "must be one of" wording: `The cloud provider hosting the instance. Must be one of "aws", "azure", or "gcp".`
- REQ-F-008: Update the `--memory` flag usage string to show representative size examples rather than an exhaustive list, e.g. `The size of the instance memory (e.g. 2GB, 8GB, 64GB). Run with an invalid value to see all accepted sizes.`

## Reference Data

Regions extracted from `tenant get` for a self-serve tenant (used to populate examples):

**AWS** (10 regions): `eu-west-1` (Ireland), `eu-central-1` (Frankfurt), `eu-west-3` (Paris), `us-east-1` (N. Virginia), `us-east-2` (Ohio), `us-west-2` (Oregon), `ap-south-1` (Mumbai), `ap-southeast-1` (Singapore), `ap-southeast-2` (Sydney), `sa-east-1` (São Paulo)

**Azure** (8 regions): `eastus` (Virginia), `westeurope` (Netherlands), `francecentral` (Paris), `uksouth` (London), `brazilsouth` (Brazil), `centralindia` (India), `koreacentral` (Seoul), `westus3` (Arizona)

**GCP** (12 regions): `europe-west1` (Belgium), `europe-west2` (UK), `europe-west3` (Germany), `us-central1` (Iowa), `us-east1` (South Carolina), `us-west1` (Oregon), `asia-east1` (Taiwan), `asia-east2` (Hong Kong), `asia-south1` (Mumbai), `asia-southeast1` (Singapore), `australia-southeast1` (Sydney), `northamerica-northeast1` (Canada)

**Instance types**: `free-db`, `professional-db`, `business-critical`, `enterprise-db`, `professional-ds`, `enterprise-ds`

## Technical Considerations

- Changes are confined to `neo4j-cli/aura/internal/subcommands/instance/create.go` (the `Long`, `Example`, and flag `Usage` strings).
- The `Example` field in cobra renders under a `Examples:` heading in `--help`. Indent each example with two spaces. Prefix with a comment line describing what the example does.
- Region identifiers must match what the API actually accepts — use the identifiers from `instance_configurations[*].region` (not `region_name`).
- After any change to the command `Short`/`Long`/`Example` text, run `go generate ./neo4j-cli/internal/skill/... ./neo4j-cli/aura/internal/skill/...` so bundle reference docs stay in sync and `TestGenerator_RoundTrip` passes.
- No import changes needed — this is purely string edits.

## Acceptance Criteria

- [ ] `neo4j aura instance create --help` shows an `Examples:` section with at least five invocations covering `free-db`, `professional-db` on all three cloud providers, and `business-critical`.
- [ ] Each non-free-db example includes a real `--region` value drawn from the reference data above.
- [ ] The `--region` flag's usage string references cloud-provider naming conventions and points to `tenant get` for the full list.
- [ ] The `Long` description's reference to `tenant get` is clear and actionable.
- [ ] `--help` shows `--type` and `--cloud-provider` usage strings with "must be one of" enumerations of valid values.
- [ ] `--help` shows `--memory` usage string with representative size examples.
- [ ] `make test` passes with no changes to test expectations (no behaviour change).
- [ ] `make fmt-check` and `make lint` are clean.
- [ ] `TestGenerator_RoundTrip` passes (bundles regenerated if needed).

## Out of Scope

- Dynamic API-backed region validation or discovery.
- New subcommands (e.g., `instance configurations`).
- Changes to flag validation logic or error messages.
- Exhaustive region listings as part of flag descriptions.

## Open Questions

_None._
