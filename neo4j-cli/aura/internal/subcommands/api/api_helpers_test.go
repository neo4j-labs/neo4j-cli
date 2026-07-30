// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	testOrgID       = "org-abc-123"
	testProjectID   = "proj-def-456"
	testPayloadFile = "payload.json"
)

// newEndpointTestConfig builds a config with no base-url and no credentials:
// endpoint resolution must never issue a request, so any regression that does
// fails loudly rather than reaching a live host.
func newEndpointTestConfig(t *testing.T, extraAuraCfg string) *clicfg.Config {
	t.Helper()

	cfgJSON := fmt.Sprintf(`{"format": "json", "aura": {"auth-url": "", "base-url": ""%s}}`, extraAuraCfg)
	fs, err := testfs.GetTestFs(cfgJSON, "{}")
	require.NoError(t, err)

	return clicfg.NewConfig(fs, "test", clicfg.AuraScope)
}

// newEndpointTestCmd registers the org/project flags and parses args so
// Flags().GetString sees them, mirroring how the api command is mounted.
func newEndpointTestCmd(t *testing.T, args []string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{
		Use:  "api",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	auraflags.RegisterOrgProjectFlags(cmd)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())

	return cmd
}

// newParamsTestCmd returns a command whose stdin is the given text, so `@-` and
// `--input -` read a deterministic payload.
func newParamsTestCmd(stdin string) *cobra.Command {
	cmd := &cobra.Command{Use: "api"}
	cmd.SetIn(strings.NewReader(stdin))

	return cmd
}

// newParamsTestConfig seeds the in-memory FS with testPayloadFile so `@file`
// and `--input <file>` have something to read.
func newParamsTestConfig(t *testing.T, payload string) *clicfg.Config {
	t.Helper()

	cfg := newEndpointTestConfig(t, "")
	require.NoError(t, afero.WriteFile(cfg.Aura.Fs(), testPayloadFile, []byte(payload), 0600))

	return cfg
}
