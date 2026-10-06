// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubListener struct{}

func (stubListener) Accept() (net.Conn, error) { return nil, fmt.Errorf("unused") }
func (stubListener) Close() error              { return nil }
func (stubListener) Addr() net.Addr            { return &net.TCPAddr{} }

// occupy makes the given host ports look taken for the duration of the test.
func occupy(t *testing.T, ports ...int) {
	t.Helper()
	busy := map[int]bool{}
	for _, p := range ports {
		busy[p] = true
	}
	prev := listenerFactory
	listenerFactory = func(port int) (net.Listener, error) {
		if busy[port] {
			return nil, fmt.Errorf("port %d is occupied", port)
		}
		return stubListener{}, nil
	}
	t.Cleanup(func() { listenerFactory = prev })
}

func TestReserve_FreeHostGivesWhatWasAsked(t *testing.T) {
	occupy(t)
	stubRandSource(t)
	var narration bytes.Buffer

	res, err := Reserve(context.Background(), NewFakeClient(), nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474}, &narration)

	require.NoError(t, err)
	assert.Equal(t, "dev", res.Name)
	assert.Equal(t, 7687, res.BoltPort)
	assert.Equal(t, 7474, res.HTTPPort)
	assert.NotEmpty(t, res.Password, "a password is minted when none was supplied")
	assert.Empty(t, narration.String(), "nothing to narrate when nothing moved")
}

func TestReserve_TakenPortsMoveTogetherAndAreNarrated(t *testing.T) {
	occupy(t, 7687)
	var narration bytes.Buffer

	res, err := Reserve(context.Background(), NewFakeClient(), nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474, Password: "given"}, &narration)

	require.NoError(t, err)
	assert.Equal(t, 7688, res.BoltPort)
	assert.Equal(t, 7475, res.HTTPPort, "the bolt/http delta is preserved")
	assert.Equal(t, "info: ports 7687/7474 in use; using 7688/7475 (bolt/http)\n", narration.String())
}

func TestReserve_TakenNameGetsASuffixAndIsNarrated(t *testing.T) {
	occupy(t)
	fake := NewFakeClient()
	fake.PsEntries = []PsEntry{{Names: "dev"}, {Names: "dev-1"}}
	var narration bytes.Buffer

	res, err := Reserve(context.Background(), fake, nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474, Password: "given"}, &narration)

	require.NoError(t, err)
	assert.Equal(t, "dev-2", res.Name)
	assert.Equal(t, "info: name \"dev\" already in use; using \"dev-2\"\n", narration.String())
}

func TestReserve_ASuppliedPasswordIsUsedVerbatimAndNotMinted(t *testing.T) {
	occupy(t)
	setRandSource(t, failingReader{}) // minting would fail: proves it is not attempted

	res, err := Reserve(context.Background(), NewFakeClient(), nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474, Password: "chosen-by-user"}, nil)

	require.NoError(t, err)
	assert.Equal(t, "chosen-by-user", res.Password)
}

func TestReserve_NilNarrationIsTolerated(t *testing.T) {
	occupy(t, 7687)
	fake := NewFakeClient()
	fake.PsEntries = []PsEntry{{Names: "dev"}}

	_, err := Reserve(context.Background(), fake, nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474, Password: "p"}, nil)

	assert.NoError(t, err)
}

func TestReserve_NoFreePortPairIsAUsageError(t *testing.T) {
	var all []int
	for i := 0; i < MaxPortOffset; i++ {
		all = append(all, 7687+i, 7474+i)
	}
	occupy(t, all...)

	_, err := Reserve(context.Background(), NewFakeClient(), nil, Want{Name: "dev", BoltPort: 7687, HTTPPort: 7474}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not find a free port pair")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("entropy unavailable") }
