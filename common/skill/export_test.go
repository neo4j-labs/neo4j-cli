// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package skill

import (
	"testing"

	"github.com/neo4j/cli/common/skill/catalog"
)

// SetCatalogCacheRootForTest overrides catalogCacheRootFn for the
// duration of the test. The previous value is restored on Cleanup.
func SetCatalogCacheRootForTest(t *testing.T, fn func() (string, error)) {
	t.Helper()
	prev := catalogCacheRootFn
	catalogCacheRootFn = fn
	t.Cleanup(func() { catalogCacheRootFn = prev })
}

// SetCatalogHTTPDoerForTest overrides catalogHTTPDoer for the duration
// of the test. The previous value is restored on Cleanup.
func SetCatalogHTTPDoerForTest(t *testing.T, fn func() catalog.HTTPDoer) {
	t.Helper()
	prev := catalogHTTPDoer
	catalogHTTPDoer = fn
	t.Cleanup(func() { catalogHTTPDoer = prev })
}
