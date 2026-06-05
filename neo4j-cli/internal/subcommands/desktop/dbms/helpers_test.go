// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"context"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
)

// StubDatasetSeams swaps the unexported resolveDatasetFn/downloadDatasetFn vars
// for the supplied fakes and restores them via t.Cleanup. It lives in a
// `package dbms` test file so the external dbms_test load tests can drive the
// dataset support layer without an exported test-only API on the package
// surface — mirroring the in-package seam swapping in docker/load_test.go.
func StubDatasetSeams(
	t *testing.T,
	resolve func(context.Context, string, string) (dataset.Spec, error),
	download func(context.Context, dataset.Spec, int64) (string, func(), error),
) {
	t.Helper()
	prevResolve, prevDownload := resolveDatasetFn, downloadDatasetFn
	resolveDatasetFn = resolve
	downloadDatasetFn = download
	t.Cleanup(func() {
		resolveDatasetFn = prevResolve
		downloadDatasetFn = prevDownload
	})
}
