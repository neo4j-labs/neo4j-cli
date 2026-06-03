// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/spf13/afero"
)

// cachedToken is the on-disk shape of the env-mode Aura JWT cache. It holds the
// derived OAuth access token and its tolerance-adjusted expiry, plus the full
// sha256 identity hash so a load can re-verify it was minted for the exact same
// client id / secret / auth-url. No raw secret is ever written.
type cachedToken struct {
	AccessToken string `json:"access-token"`
	TokenExpiry int64  `json:"token-expiry"`
	Identity    string `json:"identity"`
}

// tokenIdentity derives a stable identity from the minting inputs. The client
// secret is folded into the hash (never stored raw) so any change of identity
// produces a different hash and therefore a cache miss — the CLI can never reuse
// a token minted for a previous identity.
func tokenIdentity(clientID, clientSecret, authURL string) string {
	sum := sha256.Sum256([]byte(clientID + "\x00" + clientSecret + "\x00" + authURL))
	return hex.EncodeToString(sum[:])
}

// tokenCachePath is the OS-temp path for the cache file. The short identity
// hash in the filename keeps distinct identities in distinct files.
func tokenCachePath(identity string) string {
	return filepath.Join(os.TempDir(), "neo4j-cli-aura-token-"+identity[:16]+".json")
}

// loadTokenCache is a best-effort read of the env-mode JWT cache. Any failure —
// missing file, unreadable file, corrupt JSON, identity mismatch, or an
// expired/empty token — is treated as a cache miss (ok=false) and never panics.
func loadTokenCache(fs afero.Fs, clientID, clientSecret, authURL string) (accessToken string, tokenExpiry int64, ok bool) {
	identity := tokenIdentity(clientID, clientSecret, authURL)
	path := tokenCachePath(identity)

	exists, err := afero.Exists(fs, path)
	if err != nil || !exists {
		return "", 0, false
	}

	data, err := afero.ReadFile(fs, path)
	if err != nil {
		return "", 0, false
	}

	var cached cachedToken
	if err := json.Unmarshal(data, &cached); err != nil {
		return "", 0, false
	}

	if cached.Identity != identity {
		return "", 0, false
	}

	cred := credentials.AuraCredential{AccessToken: cached.AccessToken, TokenExpiry: cached.TokenExpiry}
	if !cred.HasValidAccessToken() {
		return "", 0, false
	}

	return cached.AccessToken, cached.TokenExpiry, true
}

// saveTokenCache atomically writes the env-mode JWT cache at mode 0600. tokenExpiry
// is the tolerance-adjusted millisecond value UpdateAccessToken computes.
func saveTokenCache(fs afero.Fs, clientID, clientSecret, authURL, accessToken string, tokenExpiry int64) error {
	identity := tokenIdentity(clientID, clientSecret, authURL)
	data, err := json.Marshal(cachedToken{
		AccessToken: accessToken,
		TokenExpiry: tokenExpiry,
		Identity:    identity,
	})
	if err != nil {
		return err
	}
	return fileutils.WriteFileErr(fs, tokenCachePath(identity), data)
}
