#!/usr/bin/env node
// Wrapper bin shim for @neo4j-labs/cli.
//
// Resolves the platform-specific package (@neo4j-labs/cli-<os>-<arch>) at runtime via
// require.resolve, then execs the binary with full stdio/signal/exit-code passthrough.
// The platform pkgs are listed in @neo4j-labs/cli's optionalDependencies so npm only
// installs the one matching the current platform/arch.

'use strict';

const { spawnSync } = require('child_process');

const SUPPORTED = [
  'darwin-arm64',
  'darwin-x64',
  'linux-arm64',
  'linux-ia32',
  'linux-x64',
  'win32-arm64',
  'win32-ia32',
  'win32-x64',
];

const platform = process.platform;
const arch = process.arch;
const key = platform + '-' + arch;
const exeSuffix = platform === 'win32' ? '.exe' : '';
const pkgName = '@neo4j-labs/cli-' + key;
const binSpecifier = pkgName + '/bin/neo4j-cli' + exeSuffix;

let binary;
try {
  binary = require.resolve(binSpecifier);
} catch (err) {
  const supportedList = SUPPORTED.map((s) => '  - ' + s).join('\n');
  process.stderr.write(
    'neo4j-cli: no prebuilt binary found for platform "' + key + '".\n' +
      '\n' +
      'Supported platforms:\n' +
      supportedList +
      '\n' +
      '\n' +
      'If your platform is in the list above, the platform package "' +
      pkgName +
      '"\nwas not installed. This typically happens when npm/pnpm/yarn was invoked\n' +
      'with --no-optional or --omit=optional. Reinstall without that flag.\n' +
      '\n' +
      'If your platform is NOT in the list, it is not currently supported.\n' +
      'Download a prebuilt binary from https://github.com/neo4j/cli/releases\n' +
      'or build from source.\n',
  );
  process.exit(1);
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' });

if (result.error) {
  process.stderr.write('neo4j-cli: failed to spawn binary: ' + result.error.message + '\n');
  process.exit(1);
}

process.exit(result.status ?? 1);
