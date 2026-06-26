// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// tokenCacheBuffer is subtracted from the cached expiry so a token that is
// about to lapse is treated as a miss and re-minted ahead of the boundary.
const tokenCacheBuffer = 60 * time.Second

// cacheDirFn resolves the directory holding the on-disk token cache. It is a
// package-level seam so tests can redirect writes to a temp dir instead of the
// shared OS temp dir.
var cacheDirFn = os.TempDir

// tokenCacheEntry is the on-disk shape of a cached Aura OAuth token. It holds
// the derived JWT and its expiry only — never the raw client secret, which is
// represented solely by the (one-way) identity hash.
type tokenCacheEntry struct {
	Token  string    `json:"token"`
	Expiry time.Time `json:"expiry"`
	Hash   string    `json:"hash"`
}

// tokenCacheHash binds a cache entry to a specific (clientID, clientSecret,
// authURL) identity. It returns the full sha256 hex digest (stored in the file
// and verified on read) and a 16-char short prefix used to name the file. The
// raw secret never leaves this function.
func tokenCacheHash(clientID, clientSecret, authURL string) (full string, short string) {
	// Length-prefix each component so the "|" separator cannot be forged from
	// within a component — otherwise ("a|b","c") and ("a","b|c") would collide
	// and one identity could receive another's cached JWT.
	input := fmt.Sprintf("%d:%s|%d:%s|%d:%s", len(clientID), clientID, len(clientSecret), clientSecret, len(authURL), authURL)
	sum := sha256.Sum256([]byte(input))
	full = hex.EncodeToString(sum[:])
	return full, full[:16]
}

// tokenCachePath returns the cache file path for the given short hash.
func tokenCachePath(short string) string {
	return filepath.Join(cacheDirFn(), fmt.Sprintf("neo4j-cli-aura-token-%s.json", short))
}

// readTokenCache returns a cached token for the given identity if the file
// exists, parses, its stored hash matches fullHash, and it is not within
// tokenCacheBuffer of expiry. Any miss (absent/corrupt/mismatch/expired)
// returns ok == false with no error.
func readTokenCache(path, fullHash string) (token string, ok bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from a sha256 prefix under os.TempDir
	if err != nil {
		return "", false
	}
	var entry tokenCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", false
	}
	if entry.Hash != fullHash || entry.Token == "" {
		return "", false
	}
	if time.Now().Add(tokenCacheBuffer).After(entry.Expiry) {
		return "", false
	}
	return entry.Token, true
}

// writeTokenCache writes the derived token + expiry + identity hash at 0600.
// Errors are returned for best-effort handling by the caller; a cache write
// failure must not fail the command.
func writeTokenCache(path, fullHash, token string, expiresInSeconds int64) error {
	// Never persist an empty token: readTokenCache already treats one as a miss,
	// so writing it would only leave junk on disk. Keep the cache layer correct
	// independent of whether the caller already screened out failed mints.
	if token == "" {
		return nil
	}
	entry := tokenCacheEntry{
		Token:  token,
		Expiry: time.Now().Add(time.Duration(expiresInSeconds) * time.Second),
		Hash:   fullHash,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	// os.WriteFile only applies the perm argument when CREATING a file; an
	// existing cache file keeps its current (possibly widened) permissions. Write
	// to a fresh 0600 temp file in the same dir and rename over the target so the
	// result is always 0600 and the swap is atomic (no partial-write window).
	tmp, err := os.CreateTemp(filepath.Dir(path), "neo4j-cli-aura-token-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()        //nolint:errcheck,gosec // best-effort cleanup
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()        //nolint:errcheck,gosec // best-effort cleanup
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) //nolint:errcheck,gosec // best-effort cleanup
		return err
	}
	return nil
}
