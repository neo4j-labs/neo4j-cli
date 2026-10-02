// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/confirm"
	engine "github.com/neo4j/cli/internal/docker"
)

// loadedContainer is a managed container that `docker load` made: it mounts the
// CLI-created data volume.
func loadedContainer(name string, volumes ...string) engine.Container {
	c := managedContainerForDelete(name)
	c.Volumes = volumes
	return c
}

func TestDelete_DataVolume(t *testing.T) {
	const vol = "neo4j-cli-movies-data"
	const hint = "docker volume rm neo4j-cli-movies-data"

	t.Run("scripted without the flag keeps the volume, never asks, and says how to remove it", func(t *testing.T) {
		withStdinIsTerminal(t, true) // even on a TTY: --yes --force means "do not ask"
		s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", vol)}, nil, "y\n")

		require.NoError(t, s.cmd.run("movies --yes --force"))

		assert.Equal(t, []string{"movies"}, s.fake.RemoveForceCalls)
		assert.Empty(t, s.fake.RemoveVolumeCalls)
		assert.NotContains(t, s.cmd.err.String(), "Also remove the data volume")
		assert.Contains(t, s.cmd.err.String(), `info: kept data volume "`+vol+`"`)
		assert.Contains(t, s.cmd.err.String(), hint)
	})

	t.Run("--remove-volume removes it without asking", func(t *testing.T) {
		withStdinIsTerminal(t, false)
		s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", vol)}, nil, "")

		require.NoError(t, s.cmd.run("movies --remove-volume --yes --force"))

		assert.Equal(t, []string{vol}, s.fake.RemoveVolumeCalls)
		assert.Contains(t, s.cmd.err.String(), `info: removed data volume "`+vol+`"`)
		assert.NotContains(t, s.cmd.err.String(), "kept data volume")
	})

	t.Run("on a TTY a yes answer removes it", func(t *testing.T) {
		withStdinIsTerminal(t, true)
		// first answer confirms the container delete, second answers the volume offer
		s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", vol)}, nil, "y\ny\n")

		require.NoError(t, s.cmd.run("movies"))

		assert.Equal(t, []string{vol}, s.fake.RemoveVolumeCalls)
		assert.Contains(t, s.cmd.err.String(), `Also remove the data volume "`+vol+`"`)
	})

	t.Run("on a TTY the offer defaults to no", func(t *testing.T) {
		withStdinIsTerminal(t, true)
		for _, second := range []string{"n\n", "\n", ""} {
			s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", vol)}, nil, "y\n"+second)

			require.NoError(t, s.cmd.run("movies"))

			assert.Equal(t, []string{"movies"}, s.fake.RemoveForceCalls, "the container is still deleted")
			assert.Empty(t, s.fake.RemoveVolumeCalls, "second answer %q keeps the volume", second)
			assert.Contains(t, s.cmd.err.String(), hint)
		}
	})

	t.Run("declining to delete the container never reaches the volume", func(t *testing.T) {
		withStdinIsTerminal(t, true)
		s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", vol)}, nil, "n\n")

		err := s.cmd.run("movies")

		assert.ErrorIs(t, err, confirm.ErrCancelled)
		assert.Empty(t, s.fake.RemoveForceCalls)
		assert.Empty(t, s.fake.RemoveVolumeCalls)
		assert.NotContains(t, s.cmd.err.String(), "data volume")
	})

	t.Run("a container with no CLI data volume says nothing about volumes", func(t *testing.T) {
		withStdinIsTerminal(t, false)
		s := newDeleteSetup(t, map[string]engine.Container{"dev": loadedContainer("dev")}, nil, "")

		require.NoError(t, s.cmd.run("dev --yes --force"))

		assert.Empty(t, s.fake.RemoveVolumeCalls)
		assert.NotContains(t, s.cmd.err.String(), "data volume")
	})
}

// --remove-volume must only ever remove the volume the CLI created for THIS
// container. A volume the user attached, or another container's, is never touched.
func TestDelete_RemoveVolume_NeverRemovesAVolumeTheCLIDidNotCreateForThisContainer(t *testing.T) {
	withStdinIsTerminal(t, false)

	for name, volumes := range map[string][]string{
		"a user volume":                      {"my-own-data"},
		"another container's CLI volume":     {"neo4j-cli-other-data"},
		"a volume with a lookalike prefix":   {"neo4j-cli-movies-data-backup"},
		"mounted but under a different name": {"movies-data"},
	} {
		t.Run(name, func(t *testing.T) {
			s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", volumes...)}, nil, "")

			require.NoError(t, s.cmd.run("movies --remove-volume --yes --force"))

			assert.Equal(t, []string{"movies"}, s.fake.RemoveForceCalls)
			assert.Empty(t, s.fake.RemoveVolumeCalls)
			assert.NotContains(t, s.cmd.err.String(), "data volume")
		})
	}
}

func TestDelete_RemoveVolume_FailureIsAPartialSuccessThatSaysHowToFinish(t *testing.T) {
	withStdinIsTerminal(t, false)
	s := newDeleteSetup(t, map[string]engine.Container{"movies": loadedContainer("movies", "neo4j-cli-movies-data")}, nil, "")
	s.fake.RemoveVolumeFn = func(context.Context, string) error { return errors.New("volume is in use") }

	err := s.cmd.run("movies --remove-volume --yes --force")

	require.Error(t, err)
	assert.Equal(t, []string{"movies"}, s.fake.RemoveForceCalls, "the container itself was removed")
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Contains(t, err.Error(), `container "movies" was removed but its data volume "neo4j-cli-movies-data" could not be`)
	assert.Contains(t, err.Error(), "volume is in use")
	assert.Contains(t, err.Error(), "docker volume rm neo4j-cli-movies-data")
}
