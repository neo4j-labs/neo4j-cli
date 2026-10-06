// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"fmt"
)

// BoltAuth identifies a container's Bolt endpoint and the credentials used to
// administer it.
type BoltAuth struct {
	URI      string
	Username string
	Password string
}

// LoadDumpIntoExistingContainer overwrites database in an already-running
// container with the dump at hostDumpPath. The flow mirrors PushToAura's
// STOP / load / START pattern over Bolt + docker exec:
//
//  1. stage the dump inside the container's scratch dir (a running container
//     cannot have a new volume bind-mounted), under the <database>.dump name
//     neo4j-admin database load expects;
//  2. STOP DATABASE over Bolt;
//  3. run neo4j-admin database load with --overwrite-destination;
//  4. START DATABASE, deferred so a mid-flight failure never leaves the
//     database stopped.
//
// Policy (managed-only, --force, required plugins, credential lookup) is the
// caller's: this function performs the destructive load unconditionally.
func LoadDumpIntoExistingContainer(ctx context.Context, client Client, containerName, hostDumpPath, database string, auth BoltAuth) error {
	if err := ValidateDatabaseName(database); err != nil {
		return err
	}

	if _, err := client.ExecAs(ctx, containerName, dumpUser, []string{"mkdir", "-p", dumpPath}, nil); err != nil {
		return err
	}
	defer func() { _, _ = client.ExecAs(ctx, containerName, dumpUser, []string{"rm", "-rf", dumpPath}, nil) }()

	destPath := dumpPath + "/" + database + ".dump"
	if err := client.CopyTo(ctx, hostDumpPath, containerName, destPath); err != nil {
		return err
	}

	if err := stopStartFn(ctx, auth.URI, auth.Username, auth.Password, "STOP DATABASE "+database); err != nil {
		return fmt.Errorf("docker load: stop database %q: %w", database, err)
	}
	defer func() {
		_ = stopStartFn(ctx, auth.URI, auth.Username, auth.Password, "START DATABASE "+database)
	}()

	_, err := client.ExecAs(ctx, containerName, dumpUser, []string{
		"neo4j-admin", "database", "load", database,
		"--from-path=" + dumpPath,
		"--overwrite-destination=true",
	}, nil)
	return err
}
