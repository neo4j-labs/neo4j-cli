# fixture-cli instance

Manage instances

Usage: `fixture-cli instance`

## fixture-cli instance create

Create an instance

Create a new fixture instance with defaults.

Usage: `fixture-cli instance create [flags]`

Flags:

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | - | Instance name |
| `--size` | int | 0 | Size in GB |

Examples:

```
fixture-cli instance create --name foo
```

## fixture-cli instance list

List instances

Usage: `fixture-cli instance list`

Examples:

```
fixture-cli instance list
```

