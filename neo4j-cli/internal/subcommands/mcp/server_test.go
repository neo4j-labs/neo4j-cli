// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package mcp

import (
	"errors"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServe_NotAnnotatedWrite proves serve is NOT annotated write:"true", so
// EnforceWriteGate does not demand --rw merely to start the server.
func TestServe_NotAnnotatedWrite(t *testing.T) {
	root := &cobra.Command{Use: "neo4j-cli"}
	serve := newServeCmd(nil)
	root.AddCommand(serve)

	// serve must not carry annotations: stdout is never a TTY under MCP,
	// so EnforceWriteGate would demand --rw merely to start the server,
	// destroying the read-only default (REQ-NF-006).
	assert.Empty(t, serve.Annotations, "serve must not carry annotations")
}

// ----- Credential store probe tests -----

func TestCheckCredentialStore_KeyringUnavailable(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return errors.New("simulated keyring failure")
	}

	fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.Error(t, err)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce), "error must be a CLIError")
	require.Equal(t, 1, ce.Code)
	assert.Contains(t, ce.Message, "OS keyring is locked")
	assert.Contains(t, ce.Message, "config set credential-storage insecure")
}

func TestCheckCredentialStore_KeyringOK(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return nil
	}

	fs, err := testfs.GetTestFs(`{"credential-storage":"keyring"}`, "{}")
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.NoError(t, err)
}

func TestCheckCredentialStore_InsecureModeSkipsProbe(t *testing.T) {
	prevProbe := probeKeyringFn
	t.Cleanup(func() { probeKeyringFn = prevProbe })

	probeKeyringFn = func() error {
		return errors.New("should not be called")
	}

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	defer cfg.Events.Flush()

	err = checkCredentialStore(cfg)
	require.NoError(t, err, "insecure mode must skip probe")
}
