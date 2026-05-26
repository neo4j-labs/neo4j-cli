# neo4j-cli update

Self-update the neo4j-cli binary

Self-update the neo4j-cli binary by downloading the latest GitHub release and atomically swapping it in place. By default only stable semver tags are considered; pass `--pre-releases` to opt into alpha/beta/rc tags. When the running binary lives under a known package-manager prefix (Homebrew, npm-global, pipx, uv tool), the command refuses to overwrite and prints the channel-correct upgrade command instead — pass `--force` to override. After a successful swap, any installed agent skill bundles are refreshed automatically. See SECURITY.md in the repo for the supply-chain trust root and accepted residual risks.

Usage: `neo4j-cli update [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f, --force` | bool | false | Bypass the package-manager-managed-binary check and proceed with the in-place swap |
| `--pre-releases` | bool | false | Include alpha/beta/rc tags when looking up the latest release |
| `--version` | string | - | Update to the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0) |

Examples:

```
# Self-update to the latest stable release
neo4j-cli update

# Self-update including pre-release (alpha/beta/rc) tags
neo4j-cli update --pre-releases

# Update to a specific tag and emit JSON
neo4j-cli update --version v1.1.0 --format json

# Force an in-place swap even on a package-manager-managed binary
neo4j-cli update --force
```

## neo4j-cli update check

Report whether a newer neo4j-cli release is available without swapping

Compares the running binary's version against the latest GitHub release at neo4j-labs/neo4j-cli and reports the result without downloading or swapping. By default only stable semver tags are considered; pass `--pre-releases` to opt into alpha/beta/rc tags. Exits 0 whether or not a newer version exists; scripts that want to branch on drift compare `current != latest` in the JSON output.

Usage: `neo4j-cli update check [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--pre-releases` | bool | false | Include alpha/beta/rc tags when looking up the latest release |
| `--version` | string | - | Compare against the named release tag instead of the latest (must be a valid semver tag, e.g. v0.1.0) |

Examples:

```
# Check whether a newer stable release is available
neo4j-cli update check

# Check including pre-release tags
neo4j-cli update check --pre-releases

# Emit a JSON document so scripts can diff current vs latest
neo4j-cli update check --format json
```

