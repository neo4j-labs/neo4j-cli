// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalog

import (
	"os"
	"path/filepath"
	"time"
)

// userCacheDirFn is the test seam for os.UserCacheDir. Override in tests to
// avoid touching the real cache dir.
var userCacheDirFn = os.UserCacheDir

// nowFn is the test seam for time.Now. Override in tests to make TTL
// behaviour deterministic.
var nowFn = time.Now

// CacheRoot returns the on-disk cache root for the marketplace cache:
// `<os.UserCacheDir>/neo4j-cli/skill-catalog/`. Returns an error when
// the OS cache dir cannot be resolved.
func CacheRoot() (string, error) {
	base, err := userCacheDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "neo4j-cli", "skill-catalog"), nil
}

// pluginJSONPath returns the absolute on-disk path of the cached
// `plugin.json` for a given cache root.
func pluginJSONPath(cacheRoot string) string {
	return filepath.Join(cacheRoot, "plugin.json")
}

// fetchedAtPath returns the absolute on-disk path of the `fetched-at`
// timestamp marker for a given cache root.
func fetchedAtPath(cacheRoot string) string {
	return filepath.Join(cacheRoot, "fetched-at")
}

// contentPath returns the absolute on-disk path of the extracted content
// root for a given catalog version. Per-skill files live one directory
// deeper at `<contentPath>/<skillName>/`.
func contentPath(cacheRoot, version string) string {
	return filepath.Join(cacheRoot, "content", version)
}
