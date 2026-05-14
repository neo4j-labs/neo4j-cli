#!/usr/bin/env node
// postinstall.js — optional auto-skill-install hook for @neo4j-labs/cli.
//
// Runs `neo4j-cli skill install --rw` after `npm install` when the environment
// variable NEO4J_CLI_AUTO_INSTALL_SKILL is set to exactly "1".
// Any failure (including no agents detected) is caught and suppressed so that
// the npm install itself is never aborted by this hook.

'use strict';

if (process.env.NEO4J_CLI_AUTO_INSTALL_SKILL !== '1') {
  process.exit(0);
}

const path = require('path');
const { spawnSync } = require('child_process');

const shimPath = path.join(__dirname, 'neo4j-cli.js');

try {
  const result = spawnSync(
    process.execPath,
    [shimPath, 'skill', 'install', '--rw'],
    { stdio: 'inherit' },
  );
  if (result.error) {
    // spawn-level error (e.g. ENOENT) — warn but don't fail
    process.stderr.write(
      'neo4j-cli postinstall: skill install failed: ' + result.error.message + '\n',
    );
  }
} catch (err) {
  // Catch any unexpected synchronous throw — must not abort npm install
  process.stderr.write(
    'neo4j-cli postinstall: unexpected error: ' + (err && err.message ? err.message : String(err)) + '\n',
  );
}

// Always exit 0 so npm install is not aborted
process.exit(0);
