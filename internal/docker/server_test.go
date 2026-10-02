// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
)

func baseSpec() ServerSpec {
	return ServerSpec{
		Name: "dev", Edition: EditionEnterprise, Version: "5.20",
		BoltPort: 7687, HTTPPort: 7474, Password: "s3cret-pw",
	}
}

func TestServerSpecArgs_GoldenShapes(t *testing.T) {
	labels := func(edition, version, bolt, http, ephemeral string) []string {
		return []string{
			"--label", "org.neo4j.cli.managed=true",
			"--label", "org.neo4j.cli.edition=" + edition,
			"--label", "org.neo4j.cli.version=" + version,
			"--label", "org.neo4j.cli.bolt-port=" + bolt,
			"--label", "org.neo4j.cli.http-port=" + http,
			"--label", "org.neo4j.cli.ephemeral=" + ephemeral,
		}
	}
	join := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}

	tests := []struct {
		name string
		spec func(ServerSpec) ServerSpec
		want []string
	}{
		{
			name: "enterprise, eval licence",
			spec: func(s ServerSpec) ServerSpec { return s },
			want: join([]string{
				"--name", "dev", "-p", "7474:7474", "-p", "7687:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
			}, labels("enterprise", "5.20", "7687", "7474", "false"), []string{"neo4j:5.20-enterprise"}),
		},
		{
			name: "enterprise with the licence accepted",
			spec: func(s ServerSpec) ServerSpec { s.AcceptLicense = true; return s },
			want: join([]string{
				"--name", "dev", "-p", "7474:7474", "-p", "7687:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=yes",
			}, labels("enterprise", "5.20", "7687", "7474", "false"), []string{"neo4j:5.20-enterprise"}),
		},
		{
			name: "community has no licence variable and an unsuffixed tag; a licence flag is ignored",
			spec: func(s ServerSpec) ServerSpec { s.Edition = EditionCommunity; s.AcceptLicense = true; return s },
			want: join([]string{
				"--name", "dev", "-p", "7474:7474", "-p", "7687:7687", "-e", "NEO4J_AUTH",
			}, labels("community", "5.20", "7687", "7474", "false"), []string{"neo4j:5.20"}),
		},
		{
			name: "latest enterprise uses the unsuffixed enterprise tag",
			spec: func(s ServerSpec) ServerSpec { s.Version = "latest"; return s },
			want: join([]string{
				"--name", "dev", "-p", "7474:7474", "-p", "7687:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
			}, labels("enterprise", "latest", "7687", "7474", "false"), []string{"neo4j:enterprise"}),
		},
		{
			name: "ephemeral adds --rm right after the name and flips the label",
			spec: func(s ServerSpec) ServerSpec { s.Ephemeral = true; return s },
			want: join([]string{
				"--name", "dev", "--rm", "-p", "7474:7474", "-p", "7687:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
			}, labels("enterprise", "5.20", "7687", "7474", "true"), []string{"neo4j:5.20-enterprise"}),
		},
		{
			name: "plugins and mounts come between the environment and the labels, in order",
			spec: func(s ServerSpec) ServerSpec {
				s.Plugins = []string{"apoc", "graph-data-science"}
				s.Mounts = []Mount{{Source: "/h/data", Target: "/data"}, {Source: "/h/imp", Target: "/import", ReadOnly: true}}
				return s
			},
			want: join([]string{
				"--name", "dev", "-p", "7474:7474", "-p", "7687:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
				"-e", `NEO4J_PLUGINS=["apoc","graph-data-science"]`,
				"-v", "/h/data:/data", "-v", "/h/imp:/import:ro",
			}, labels("enterprise", "5.20", "7687", "7474", "false"), []string{"neo4j:5.20-enterprise"}),
		},
		{
			name: "custom ports are published and labelled",
			spec: func(s ServerSpec) ServerSpec { s.BoltPort, s.HTTPPort = 7700, 7500; return s },
			want: join([]string{
				"--name", "dev", "-p", "7500:7474", "-p", "7700:7687",
				"-e", "NEO4J_AUTH", "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
			}, labels("enterprise", "5.20", "7700", "7500", "false"), []string{"neo4j:5.20-enterprise"}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			argv, _ := tc.spec(baseSpec()).Args()
			assert.Equal(t, tc.want, argv)
		})
	}
}

func TestServerSpecArgs_PasswordTravelsInEnvNeverInArgv(t *testing.T) {
	argv, env := baseSpec().Args()

	assert.Equal(t, []string{"NEO4J_AUTH=neo4j/s3cret-pw"}, env)
	assert.NotContains(t, strings.Join(argv, " "), "s3cret-pw", "argv is world-readable via /proc")
	assert.Contains(t, argv, "NEO4J_AUTH", "only the variable NAME is passed through")
}

// inspectJSON renders what `docker inspect` would report for a container started
// with spec: its labels and its NEO4J_PLUGINS environment variable.
func inspectJSON(t *testing.T, spec ServerSpec) string {
	t.Helper()
	labels := map[string]string{}
	for _, l := range spec.labels() {
		k, v, _ := strings.Cut(l, "=")
		labels[k] = v
	}
	var env []string
	if p := pluginsEnvValue(spec.Plugins); p != "" {
		env = append(env, "NEO4J_PLUGINS="+p)
	}
	b, err := json.Marshal([]map[string]any{{
		"Name":   "/" + spec.Name,
		"State":  map[string]any{"Status": "running", "Running": true},
		"Config": map[string]any{"Image": spec.Image(), "Labels": labels, "Env": env},
	}})
	require.NoError(t, err)
	return string(b)
}

// The labels `create`/`load` write and the ones `list`/`get`/`delete` read are
// one contract; this fails if either side changes alone.
func TestServerSpecLabels_AreExactlyWhatContainerParsingReadsBack(t *testing.T) {
	spec := baseSpec()
	spec.Edition, spec.Ephemeral, spec.BoltPort, spec.HTTPPort = EditionCommunity, true, 7700, 7500
	spec.Plugins = []string{"apoc", "graph-data-science"}

	got, err := parseInspectOutput("dev", inspectJSON(t, spec))

	require.NoError(t, err)
	assert.Equal(t, "dev", got.Name)
	assert.True(t, got.Managed)
	assert.Equal(t, "community", got.Edition)
	assert.Equal(t, "5.20", got.Version)
	assert.Equal(t, "7700", got.BoltPort)
	assert.Equal(t, "7500", got.HTTPPort)
	assert.True(t, got.Ephemeral)
	assert.Equal(t, []string{"apoc", "graph-data-science"}, got.Plugins, "NEO4J_PLUGINS round-trips, so `docker get` shows what create installed")
}

func TestStartServer_RunsTheBuiltArgsWithTheSecretInEnv(t *testing.T) {
	fake := NewFakeClient()
	spec := baseSpec()

	require.NoError(t, StartServer(context.Background(), fake, spec))

	require.Len(t, fake.RunEnvCalls, 1)
	wantArgs, wantEnv := spec.Args()
	assert.Equal(t, wantArgs, fake.RunEnvCalls[0].Args)
	assert.Equal(t, wantEnv, fake.RunEnvCalls[0].Env)
}

func TestStartServer_ReturnsTheDockerError(t *testing.T) {
	fake := NewFakeClient()
	boom := errors.New("docker: boom")
	fake.RunFn = func(context.Context, []string) (string, error) { return "", boom }

	assert.ErrorIs(t, StartServer(context.Background(), fake, baseSpec()), boom)
}

func TestValidatePlugins(t *testing.T) {
	t.Run("trims, keeps order and drops duplicates", func(t *testing.T) {
		got, err := ValidatePlugins([]string{" apoc ", "graph-data-science", "apoc", "n10s"})
		require.NoError(t, err)
		assert.Equal(t, []string{"apoc", "graph-data-science", "n10s"}, got)
	})
	t.Run("none is fine", func(t *testing.T) {
		got, err := ValidatePlugins(nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	for _, bad := range []string{"", "APOC", "ap oc", `apoc","x`, "../apoc", "-apoc", "apoc;rm", "a_b"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			_, err := ValidatePlugins([]string{"apoc", bad})
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce))
			assert.Equal(t, 2, ce.Code, "a bad plugin name is a usage error")
		})
	}
}
