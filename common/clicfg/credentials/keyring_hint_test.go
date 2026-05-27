// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/stretchr/testify/assert"
)

func TestKeyringSetupHint_NonEmpty(t *testing.T) {
	hint := credentials.KeyringSetupHint()
	assert.NotEmpty(t, hint, "KeyringSetupHint must return a non-empty string")
}

func TestKeyringSetupHint_PlatformKeyword(t *testing.T) {
	hint := credentials.KeyringSetupHint()

	switch runtime.GOOS {
	case "linux":
		assert.Contains(t, hint, "gnome-keyring",
			"linux hint must mention gnome-keyring")
	case "darwin":
		assert.Contains(t, hint, "unlock-keychain",
			"darwin hint must mention unlock-keychain")
	case "windows":
		assert.Contains(t, hint, "VaultSvc",
			"windows hint must mention VaultSvc")
	default:
		assert.Contains(t, strings.ToLower(hint), "keyring",
			"default hint must mention keyring")
	}
}

func TestKeyringSetupHint_NoInsecureFallback(t *testing.T) {
	hint := credentials.KeyringSetupHint()
	assert.NotContains(t, hint, "config set credential-storage insecure",
		"hint must not contain the insecure fallback command")
}
