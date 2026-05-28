// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package embed

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// vertexScope is the OAuth2 scope required to call the Vertex AI predict
// endpoint. Pinned here so the golang.org/x/oauth2 module is a direct
// dependency ahead of the vertex provider landing in a later task.
//
//nolint:unused // wired up by the vertex provider task
const vertexScope = "https://www.googleapis.com/auth/cloud-platform"

// findDefaultTokenSource resolves Application Default Credentials and returns
// an oauth2.TokenSource scoped for Vertex AI. Indirected as a package-level
// variable so tests can stub it when the vertex provider lands.
//
//nolint:unused // wired up by the vertex provider task
var findDefaultTokenSource = func(ctx context.Context) (oauth2.TokenSource, error) {
	creds, err := google.FindDefaultCredentials(ctx, vertexScope)
	if err != nil {
		return nil, err
	}
	return creds.TokenSource, nil
}
