# Neo4j CLI

## Installation

Downloadable binaries are available from the [releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest) page.

Download the appropriate archive for your operating system and architecture.

## Usage

Extract the executable to a directory of your choosing.

Create Aura API Credentials in your [Account Settings](https://console.neo4j.io/#account), and note down the client ID and secret.

Add these credentials into the CLI with a name of your choosing:

```bash
./neo4j-cli aura credential add --name "Aura API Credentials" --client-id <client-id> --client-secret <client-secret>
```

This will add and set the credential as the default credential for use.

You can then, for example, list your instances in a table format:

```bash
./neo4j-cli aura instance list --output table
```

If you would rather just type `neo4j-cli` then move the neo4j-cli binary into the file path of your computer.  
Windows:

```bash
move neo4j-cli.exe c:\windows\system32
```

Mac:

```bash
sudo mv neo4j-cli /usr/local/bin
```

To see all of the available commands:

```bash
./neo4j-cli
```

Help for each command is accessed by using it without any flags or options. For example, to see help creating an instance:

```bash
./neo4j-cli aura instance create
```

## Feedback / Issues

Please use [GitHub issues](https://github.com/neo4j-labs/neo4j-cli/issues) to provide feedback and report any issues that you have encountered.

## Building locally

Clone the repository and run:

```bash
make build
```

This produces `bin/aura-cli` and `bin/neo4j-cli`. To run without building:

```bash
make run-aura   # standalone aura-cli
make run-neo4j  # neo4j-cli super CLI
```

## Developing and contributing

Read [CONTRIBUTING.md](./CONTRIBUTING.md)
