// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"fmt"
	"net"

	"github.com/neo4j/cli/internal/clierr"
)

// MaxPortOffset caps the port-pair fallback walk (REQ-F-002). Parity with
// `MaxNameSuffix=99`: 100 offsets (0..99) is enough headroom for everyday
// collisions but not so high that exhaustion silently hides a deeper bug
// (e.g. stale containers piling up on the host).
const MaxPortOffset = 100

// listenerFactory is the injectable seam for the port-conflict pre-flight
// (REQ-F-013). Production binds an ephemeral TCP listener on the requested
// host port (closing immediately on success); tests swap in a fake that
// returns sentinel errors keyed by port so we never touch the network.
var listenerFactory = func(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf(":%d", port))
}

// portFree binds and immediately releases a TCP listener on the given host
// port via the listenerFactory seam. Returns true when the port is free
// (i.e. the listener bound successfully); false otherwise. On success the
// listener is closed before returning so the real `docker run` call can
// claim the port a moment later.
func portFree(port int) bool {
	ln, err := listenerFactory(port)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// FindFreePortPair walks the port-pair fallback loop (REQ-F-001..007).
// Starting from (boltStart, httpStart) it tries offsets 0..maxPortOffset-1,
// returning the first pair where BOTH ports bind successfully. The same
// offset is applied to both ports so the operator's bolt/http delta is
// preserved across the fallback. On exhaustion a clierr.UsageError points
// the operator at --bolt-port / --http-port so they can pin a free pair
// explicitly.
func FindFreePortPair(boltStart, httpStart int) (int, int, error) {
	for offset := 0; offset < MaxPortOffset; offset++ {
		bolt := boltStart + offset
		http := httpStart + offset
		if portFree(bolt) && portFree(http) {
			return bolt, http, nil
		}
	}
	return 0, 0, clierr.NewUsageError(
		"could not find a free port pair starting at %d/%d after %d attempts; pass --bolt-port / --http-port",
		boltStart, httpStart, MaxPortOffset,
	)
}
