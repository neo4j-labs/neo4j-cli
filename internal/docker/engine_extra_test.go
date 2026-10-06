// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExecClient_LookupMissingDocker confirms REQ-F-060: when docker is
// absent from PATH, the resolver returns the documented clierr.UsageError
// with the install hint. We force the miss by clearing PATH on the spawn.
func TestExecClient_LookupMissingDocker(t *testing.T) {
	t.Setenv("PATH", "")
	ec := &execClient{}
	_, err := ec.resolve()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker not found in PATH")
	assert.Contains(t, err.Error(), "install Docker Desktop")
}

// TestLabelsConstants is a regression guard for REQ-F-011: every label key
// must remain under the org.neo4j.cli namespace so the discovery filter
// (label=org.neo4j.cli.managed=true) continues to scope correctly.
func TestLabelsConstants(t *testing.T) {
	for _, lbl := range []string{
		LabelManaged,
		LabelEdition,
		LabelVersion,
		LabelBoltPort,
		LabelHTTPPort,
		LabelEphemeral,
	} {
		assert.True(t, strings.HasPrefix(lbl, "org.neo4j.cli."),
			"label %q must remain under the org.neo4j.cli namespace", lbl)
	}
}

func TestParseNeo4jPluginsEnv(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want []string
	}{
		{"present", []string{"FOO=bar", `NEO4J_PLUGINS=["apoc","graph-data-science"]`}, []string{"apoc", "graph-data-science"}},
		{"absent", []string{"FOO=bar"}, nil},
		{"empty", []string{"NEO4J_PLUGINS="}, nil},
		{"unparseable", []string{"NEO4J_PLUGINS=apoc"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseNeo4jPluginsEnv(tc.env))
		})
	}
}
