// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"io"

	"github.com/neo4j/cli/internal/clicfg/credentials"
)

// Want is what a caller asks for before it starts a container: a preferred name
// and host port pair, and optionally a password it already chose.
type Want struct {
	Name     string
	BoltPort int
	HTTPPort int
	Password string // empty: generate one
}

// Reservation is what was actually secured: a name nothing else uses, a host
// port pair that is free right now, and a password.
type Reservation struct {
	Name     string
	BoltPort int
	HTTPPort int
	Password string
}

// Reserve settles everything a new container needs that depends on the host's
// current state, before any docker side effect:
//
//  1. a free host port pair — when the wanted pair is taken, both ports move by
//     the same offset so the bolt/http delta is preserved;
//  2. a name free in docker and in the stored DBMS credentials (see
//     ResolveContainerName);
//  3. the password — the supplied one, or a freshly minted one (see
//     GeneratePassword, which registers it for redaction).
//
// Each fallback is narrated to narrate as an `info:` line; nil drops the
// narration. dbms may be nil when no credential store is available.
func Reserve(ctx context.Context, client Client, dbms *credentials.DbmsCredentials, want Want, narrate io.Writer) (Reservation, error) {
	bolt, http, err := FindFreePortPair(want.BoltPort, want.HTTPPort)
	if err != nil {
		return Reservation{}, err
	}
	if bolt != want.BoltPort || http != want.HTTPPort {
		writeInfo(narrate, "ports %d/%d in use; using %d/%d (bolt/http)\n", want.BoltPort, want.HTTPPort, bolt, http)
	}

	name, err := ResolveContainerName(ctx, client, dbms, want.Name)
	if err != nil {
		return Reservation{}, err
	}
	if name != want.Name {
		writeInfo(narrate, "name %q already in use; using %q\n", want.Name, name)
	}

	password := want.Password
	if password == "" {
		if password, err = GeneratePassword(); err != nil {
			return Reservation{}, err
		}
	}

	return Reservation{Name: name, BoltPort: bolt, HTTPPort: http, Password: password}, nil
}
