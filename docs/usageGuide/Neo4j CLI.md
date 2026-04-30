# Neo4j CLI

The Neo4j CLI (`neo4j-cli`) is a unified command-line tool that bundles multiple Neo4j tools into a single binary.
Rather than installing separate tools for each Neo4j product, `neo4j-cli` gives you one entry point for all of them.

## Subcommands

| Subcommand | Description |
|---|---|
| `aura` | Manage Neo4j Aura resources (instances, credentials, tenants, and more) |

## Installation

Downloadable binaries are available from the [releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest) page.
The CLI is fully compatible with Mac, Linux, and Windows.

1. Using your browser, navigate to [https://github.com/neo4j-labs/neo4j-cli/releases](https://github.com/neo4j-labs/neo4j-cli/releases).

2. Download the `neo4j-cli` compressed file that matches your operating system and architecture. Make a note of the folder where the file is saved.

3. Extract the contents of the downloaded archive.

4. Open a terminal and move to the location where you extracted the files.

5. Move the `neo4j-cli` executable into a directory on your PATH.

- Mac/Linux:

```text
sudo mv neo4j-cli /usr/local/bin
```

- Windows:

```text
move neo4j-cli.exe c:\windows\system32
```

6. Verify the installation:

```text
neo4j-cli --version
```

**Note**: Mac users may receive a warning from Apple that `neo4j-cli` could not be verified. If this happens, open **System Settings**, select **Privacy & Security**, and scroll down to select **Open Anyway**. The `neo4j-cli` binary has been through the Apple certification process.

## Usage

Run `neo4j-cli` on its own to see all available subcommands:

```text
neo4j-cli
```

Add `--help` to any command or subcommand for usage information:

```text
neo4j-cli --help
neo4j-cli aura --help
neo4j-cli aura instance --help
```

### Aura subcommand

All Aura resource management is available under the `aura` subcommand. For example:

```text
neo4j-cli aura credential add --name "my-credentials" --client-id <client-id> --client-secret <client-secret>
neo4j-cli aura instance list --output table
neo4j-cli aura instance get <instance-id>
```

For the full Aura command reference, see [A Guide To The New Aura CLI](./A%20Guide%20To%20The%20New%20Aura%20CLI.md).
Every `aura-cli` command in that guide can be run by replacing `aura-cli` with `neo4j-cli aura`.
