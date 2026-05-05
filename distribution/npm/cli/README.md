# @neo4j-labs/cli

The Neo4j command-line tool, distributed as a prebuilt native binary via npm.

## Install

```sh
npm i -g @neo4j-labs/cli
```

This installs a small Node wrapper plus one platform-specific package
containing the prebuilt `neo4j-cli` binary for your OS and CPU. npm picks the
matching platform package automatically — there is no postinstall script and
no source build.

After install, `neo4j-cli` is on your `PATH`:

```sh
neo4j-cli --help
neo4j-cli aura instance list --output table
```

`pnpm add -g @neo4j-labs/cli` and `yarn global add @neo4j-labs/cli` work the same way.

## Supported platforms

| OS        | CPU     |
| --------- | ------- |
| `darwin`  | `arm64` |
| `darwin`  | `x64`   |
| `linux`   | `arm64` |
| `linux`   | `ia32`  |
| `linux`   | `x64`   |
| `win32`   | `arm64` |
| `win32`   | `ia32`  |
| `win32`   | `x64`   |

If your platform is in the table above but the wrapper still complains that no
prebuilt binary was found, your install probably ran with `--no-optional` or
`--omit=optional`. Reinstall without that flag — the per-platform binary lives
in an `optionalDependencies` entry, so excluding optionals also excludes the
binary.

If your platform is not in the table, `npm i -g @neo4j-labs/cli` will still
succeed (the platform packages are optional), but invoking `neo4j-cli` will
print a friendly error naming the detected platform and arch. Download a
prebuilt binary directly from
[GitHub Releases](https://github.com/neo4j-labs/neo4j-cli/releases/latest)
or build from source.

## Release channels

```sh
npm i -g @neo4j-labs/cli           # latest stable
npm i -g @neo4j-labs/cli@alpha     # alpha prerelease
npm i -g @neo4j-labs/cli@beta      # beta prerelease
npm i -g @neo4j-labs/cli@rc        # release candidate
npm i -g @neo4j-labs/cli@next      # any other prerelease (e.g. snapshot, dev)
```

The dist-tag is picked from the version string at publish time:
`X.Y.Z-alpha*` → `alpha`, `X.Y.Z-beta*` → `beta`, `X.Y.Z-rc*` → `rc`, any
other `-`-suffixed version → `next`, plain `X.Y.Z` → `latest`.

## Documentation and feedback

- Project repo and full docs:
  https://github.com/neo4j-labs/neo4j-cli
- Issue tracker:
  https://github.com/neo4j-labs/neo4j-cli/issues
- Aura CLI usage guide:
  https://github.com/neo4j-labs/neo4j-cli/blob/main/docs/usageGuide/A%20Guide%20To%20The%20New%20Aura%20CLI.md

## License

Apache-2.0. See the project repo for the full license text and third-party
notices.
