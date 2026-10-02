// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clierr"
)

// MaxNameSuffix caps the auto-suffix walk for name-collision resolution
// (REQ-F-014). The contract is `<name>-1` … `<name>-99`; exceeding it is
// almost certainly operator error (stale containers piling up) and we
// surface that explicitly rather than spinning forever.
const MaxNameSuffix = 99

// ResolveContainerName implements the REQ-F-014 name-collision contract.
// It enumerates existing names from docker (all containers, managed or
// not — docker enforces global container-name uniqueness) and from the
// stored dbms credentials, then returns the requested name when free or
// the first non-colliding `<name>-<i>` suffix in 1..maxNameSuffix.
// Returns a clierr.UsageError when every suffix in that range is taken
// so the operator gets a clear "pick a different --name" hint.
func ResolveContainerName(ctx context.Context, client Client, dbms *credentials.DbmsCredentials, requested string) (string, error) {
	used, err := collectUsedNames(ctx, client, dbms)
	if err != nil {
		return "", err
	}
	if _, taken := used[requested]; !taken {
		return requested, nil
	}
	for i := 1; i <= MaxNameSuffix; i++ {
		candidate := fmt.Sprintf("%s-%d", requested, i)
		if _, taken := used[candidate]; !taken {
			return candidate, nil
		}
	}
	return "", clierr.NewUsageError(
		"could not find a free name for %q after trying %s-1 through %s-%d; pass --name <other>",
		requested, requested, requested, MaxNameSuffix,
	)
}

// collectUsedNames merges docker container names (from PsAll, unfiltered so
// unmanaged containers count too) with stored dbms credential names into a
// single set used for collision detection. The set is conservative: any
// PsEntry.Names value gets split on `,` and trimmed because Docker emits
// multi-name entries as a comma-separated string.
func collectUsedNames(ctx context.Context, client Client, dbms *credentials.DbmsCredentials) (map[string]struct{}, error) {
	used := map[string]struct{}{}

	entries, err := client.PsAll(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		for _, n := range strings.Split(entry.Names, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				used[n] = struct{}{}
			}
		}
	}

	if dbms != nil {
		for _, cred := range dbms.List() {
			if cred != nil && cred.Name != "" {
				used[cred.Name] = struct{}{}
			}
		}
	}
	return used, nil
}
