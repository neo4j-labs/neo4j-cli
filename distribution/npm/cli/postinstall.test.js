// Copyright (c) "Neo4j"
// Neo4j Sweden AB [https://neo4j.com]
// SPDX-License-Identifier: Apache-2.0

// postinstall.test.js — Jest tests for bin/postinstall.js
//
// Run from distribution/npm/cli/:
//   npm test
//
// These tests verify the NEO4J_CLI_AUTO_INSTALL_SKILL guard logic without
// requiring a real neo4j-cli binary.

'use strict';

/**
 * Load bin/postinstall.js in isolation with a mocked child_process, invoke
 * its exported run() function, and return the spawn mock for inspection.
 *
 * @param {string|undefined} envValue - value for NEO4J_CLI_AUTO_INSTALL_SKILL (undefined = unset)
 * @param {object|null} spawnError    - if non-null, assigned to result.error from spawnSync
 * @returns {{ spawnMock: jest.Mock }}
 */
function runPostinstall(envValue, spawnError = null) {
  // Isolate module registry so each test gets a fresh module instance.
  jest.resetModules();

  // Set (or delete) the env var before loading the module.
  if (envValue === undefined) {
    delete process.env.NEO4J_CLI_AUTO_INSTALL_SKILL;
  } else {
    process.env.NEO4J_CLI_AUTO_INSTALL_SKILL = envValue;
  }

  // jest.doMock is not hoisted, so it can reference variables in this closure.
  const spawnMock = jest.fn().mockReturnValue(
    spawnError ? { error: spawnError, status: null } : { error: null, status: 0 },
  );
  jest.doMock('child_process', () => ({ spawnSync: spawnMock }));

  // Require after mocking so the module picks up our stub.
  const { run } = require('./bin/postinstall');
  run();

  return { spawnMock };
}

afterEach(() => {
  jest.restoreAllMocks();
  jest.dontMock('child_process');
  delete process.env.NEO4J_CLI_AUTO_INSTALL_SKILL;
});

describe('postinstall.js', () => {
  test('env=1: spawnSync is called with node, shim path, and skill install --rw', () => {
    const { spawnMock } = runPostinstall('1');

    expect(spawnMock).toHaveBeenCalledTimes(1);

    const [executable, args] = spawnMock.mock.calls[0];
    // First arg is process.execPath (the Node.js binary).
    expect(executable).toBe(process.execPath);
    // Second arg is the path to the shim (ends with neo4j-cli.js).
    expect(args[0]).toMatch(/neo4j-cli\.js$/);
    // Remaining args are the skill subcommand.
    expect(args.slice(1)).toEqual(['skill', 'install', '--rw']);
  });

  test('env unset: spawnSync is never called', () => {
    const { spawnMock } = runPostinstall(undefined);

    expect(spawnMock).not.toHaveBeenCalled();
  });

  test('env=0: spawnSync is never called', () => {
    const { spawnMock } = runPostinstall('0');

    expect(spawnMock).not.toHaveBeenCalled();
  });

  test('env=1 and spawnSync returns an error: run() returns without throwing (error suppressed)', () => {
    const stubError = new Error('ENOENT: binary not found');
    let spawnMock;

    // run() must not throw even when spawnSync reports an error.
    expect(() => {
      ({ spawnMock } = runPostinstall('1', stubError));
    }).not.toThrow();

    // spawnSync was attempted despite the error.
    expect(spawnMock).toHaveBeenCalledTimes(1);
  });
});
