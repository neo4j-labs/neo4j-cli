// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataVolumeName_IsWhatTheLoaderMounts(t *testing.T) {
	assert.Equal(t, "neo4j-cli-movies-data", DataVolumeName("movies"))

	// The loader and `docker delete` must agree: the server spec the loader
	// builds mounts exactly DataVolumeName(<container>) at /data.
	spec := ServerSpec{Name: "movies", Edition: EditionEnterprise, Version: "5", BoltPort: 7687, HTTPPort: 7474}
	spec.Mounts = []Mount{{Source: DataVolumeName(spec.Name), Target: "/data"}}
	argv, _ := spec.Args()
	assert.Contains(t, argv, "neo4j-cli-movies-data:/data")
}

func TestManagedDataVolume(t *testing.T) {
	tests := []struct {
		name    string
		c       Container
		wantVol string
		wantOK  bool
	}{
		{name: "the CLI volume of this container", c: Container{Name: "movies", Volumes: []string{"neo4j-cli-movies-data"}}, wantVol: "neo4j-cli-movies-data", wantOK: true},
		{name: "found among other volumes", c: Container{Name: "movies", Volumes: []string{"cache", "neo4j-cli-movies-data"}}, wantVol: "neo4j-cli-movies-data", wantOK: true},
		{name: "no volumes (a created container uses bind mounts)", c: Container{Name: "dev"}},
		{name: "a user's own volume", c: Container{Name: "movies", Volumes: []string{"my-data"}}},
		{name: "another container's CLI volume", c: Container{Name: "movies", Volumes: []string{"neo4j-cli-other-data"}}},
		{name: "a lookalike name", c: Container{Name: "movies", Volumes: []string{"neo4j-cli-movies-data-old", "xneo4j-cli-movies-data"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vol, ok := ManagedDataVolume(tc.c)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantVol, vol)
		})
	}
}

// Both runtimes report mounts as [{Type, Name, Destination, ...}], with bind
// mounts typed "bind" and carrying no name.
func TestParseInspectOutput_NamedVolumesOnly(t *testing.T) {
	const inspect = `[{
		"Name": "/movies",
		"State": {"Status": "running", "Running": true},
		"Config": {"Image": "neo4j:enterprise", "Labels": {"org.neo4j.cli.managed": "true"}, "Env": []},
		"Mounts": [
			{"Type": "volume", "Name": "neo4j-cli-movies-data", "Destination": "/data"},
			{"Type": "bind", "Source": "/home/me/logs", "Destination": "/logs"},
			{"Type": "volume", "Name": "", "Destination": "/anon"}
		]
	}]`

	got, err := parseInspectOutput("movies", inspect)

	require.NoError(t, err)
	assert.Equal(t, []string{"neo4j-cli-movies-data"}, got.Volumes, "bind mounts and anonymous volumes are not named volumes")
}

func TestParseInspectOutput_NoMounts(t *testing.T) {
	got, err := parseInspectOutput("dev", `[{"Name":"/dev","State":{},"Config":{"Labels":{}}}]`)
	require.NoError(t, err)
	assert.Empty(t, got.Volumes)
}

func TestExecClient_RemoveVolume_IssuesVolumeRm(t *testing.T) {
	argvFile := stubDocker(t, 0, "", "")

	require.NoError(t, (&execClient{}).RemoveVolume(context.Background(), "neo4j-cli-movies-data"))

	raw, err := os.ReadFile(argvFile)
	require.NoError(t, err)
	assert.Equal(t, "volume\nrm\nneo4j-cli-movies-data\n", string(raw))
}

func TestExecClient_RemoveVolume_SurfacesDockerStderr(t *testing.T) {
	stubDocker(t, 1, "", "volume is in use")

	err := (&execClient{}).RemoveVolume(context.Background(), "v")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "volume is in use")
}
