# Contributing

Thanks for your interest in contributing to the Neo4j CLI, [issues](https://github.com/neo4j-labs/neo4j-cli/issues) and [pull requests](https://github.com/neo4j-labs/neo4j-cli/pulls) are welcome.

If you want to contribute code, make sure to [sign the CLA](https://neo4j.com/developer/contributing-code/#sign-cla).

## Development

### Testing

The full suite of tests can be run using the following command:

```bash
make test
```

### Local running

The CLI can be run locally without building a binary. To run the standalone `aura-cli`:

```bash
make run-aura
```

To run the `neo4j-cli` super CLI:

```bash
make run-neo4j
```

### Linting and formatting

To lint the codebase:

```bash
make lint
```

To format all Go source files:

```bash
make fmt
```

### Pull requests

As well as your code changes, pull requests need a changelog entry. These are added using the tool [`changie`](https://changie.dev/). You will need to install this using the following command:

```bash
go install github.com/miniscruff/changie@latest
```

If changie is not available, you may need to add `/go/bin` to your path: `export PATH="$HOME/go/bin:$PATH"`

This repository uses a **multi-project** changie setup with two projects: `aura-cli` and `neo4j-cli`.

Run `make changelog` and follow the prompts. Changie will ask you to select one or more projects and a change kind, then generate a YAML file per project in `.changes/unreleased/`. Commit those files alongside your code changes.

#### Changes affecting multiple projects

Because `neo4j-cli` bundles its child CLIs, any change to a child CLI also affects `neo4j-cli`. Select both projects when prompted — `make changelog` supports multi-select in interactive mode.

For non-interactive use (e.g. scripts or agents), pass `--projects` multiple times:

```bash
changie new --projects aura-cli --projects neo4j-cli --kind Patch --body "your change description"
```

Only changes specific to the `neo4j-cli` wrapper itself need a single `neo4j-cli` entry.

### License

All `.go` files must begin with the following license comment:

```go
// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]
```

To check that all files comply, run:

```bash
make license-check
```

> Note: `make license-check` requires a Unix shell (`bash`/`sh`) with `find` and `xargs`. It will not work natively on Windows without WSL or Git Bash.

### Building

Builds for releases are handled in GitHub Actions. If you want to create local builds, there are a couple of approaches.

To build both `aura-cli` and `neo4j-cli` into the `bin/` directory:

```bash
make build
```

You can also build each binary individually:

```bash
make build-aura   # produces bin/aura-cli
make build-neo4j  # produces bin/neo4j-cli
```

To remove build artifacts:

```bash
make clean
```

If you want to build binaries for all varieties of platforms, you can do so with the following command:

```bash
GORELEASER_CURRENT_TAG=dev goreleaser release --snapshot --clean
```

In the above command, `GORELEASER_CURRENT_TAG` can be substituted for any version of your choosing.

## CLI Guidelines

The Aura CLI aims to provide a consistent and reliable experience to the end user. Any change made to the CLI must comform to the following guidelines.

### Commands

- All commands must be singular
    - ✅ `aura-cli instance`
    - ❌ `aura-cli instances`
- Verbs and nouns should be separate, with the action at the end
    - ✅ `aura-cli instance list`
    - ❌ `aura-cli list-instance`
    - ❌ `aura-cli list instance`

### Parameters

To avoid confusion, this guide uses the term **flags** to refer to any named argument, whether it has values or not (e.g. `-l`, `--output json`) and **arguments** exclusively for positional arguments (e.g. `list 1234`).

- Only one argument should be used, if more than one is needed, use flags instead. This is to avoid confusion when passing parameters without enough context
    - ✅ `aura-cli instance get <id>`
    - ❌ `aura-cli instance get <id> <deployment-id>`
    - ✅ `aura-cli instance get <id> --deployment-id <deployment-id>`
    - ⚠️ `aura-cli instance get --instance-id <id> --deployment-id <deployment-id>`  
      This valid, but the option above is preferred as it is more concise
- The argument must always refer to the closest noun
    - ❌ `aura-cli instance snapshot list <instance-id>`
    - ✅ `aura-cli instance snapshot list --instance-id <instance-id>`
- No arguments between commands
    - ❌ `aura-cli tenant <tenant-id> instance get <id>`
    - ✅ `aura-cli instance get <id> --tenant-id <tenant-id>`
- Flags, if set, take precedence over global configuration or default values
- Flags should have descriptions, if the flag is expected to be always set. The description must start with `(required)`

#### Output

- Read operations should support the following `--output` options:
    - `json`: Provides the raw JSON output of the API, formatted to be human-readable.
    - `table`: Provides a subset of the output, formatted to be human readable on a table. Try to keep the table output below 120 characters to avoid overflowing the screen.

> These guidelines are based on https://clig.dev

### Structure

Aura CLI is divided in top level commands, for example:

- `instance`
- `config`

Each of these commands handle a certain resource of the API and have several subcommands for the actions, for example:

- `instance list`
- `instance get`

Nested subcommands are also allowed, for example:

- `instance snapshot list`

Folders and files should follow the same structure as the commands. So for example, `instance snapshot list` should be implemented in the folder `subcommands/instance/snapshot/list.go`. A single command per file

### Common subcommands

Most commands targetting API resources contain some of the following subcommands as actions:

- `get`
- `list`
- `delete`
- `create`

Commands may also have some extra, specific commands, such as `instance pause`.

For asynchronous operations (i.e. operations that trigger a job that won't be finished in the same request), the flag `--await` can be used to wait until the operation has been completed, generally polling for the status. If this flag is not set, all operations must finish when the request has been completed, even if a job is pending.

## Resources

- [CLI Usage Guide](./docs/usageGuide/A%20Guide%20To%20The%20New%20Aura%20CLI.md).
- [Neo4j Aura API](https://neo4j.com/docs/aura/platform/api/specification/)
- https://clig.dev
