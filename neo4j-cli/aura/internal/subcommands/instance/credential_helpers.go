// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"

	"github.com/neo4j/cli/common/clicfg/credentials"
)

// dbmsGetter is a minimal interface for looking up a stored credential by name.
// *credentials.DbmsCredentials satisfies this interface.
type dbmsGetter interface {
	Get(name string) (*credentials.DbmsCredential, error)
}

// baseCredentialName returns customName when it is non-empty, otherwise it
// returns "<instanceID>-default".
func baseCredentialName(instanceID, customName string) string {
	if customName != "" {
		return customName
	}
	return instanceID + "-default"
}

// resolveCredentialName returns base if no credential with that name exists in
// dbms. If base is already taken it appends "-1", "-2", … until a free slot is
// found.
func resolveCredentialName(dbms dbmsGetter, base string) string {
	if _, err := dbms.Get(base); err != nil {
		// base is free
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, err := dbms.Get(candidate); err != nil {
			return candidate
		}
	}
}
