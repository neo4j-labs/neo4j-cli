// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

import (
	"encoding/json"

	"github.com/spf13/afero"
)

// ReloadAuraCredential reads credentials.json from disk using fs and
// filePath, parses it, and returns the named AuraCredential with ok=true.
// It returns nil, false on any error: file missing, parse error, or the
// named credential not found. It never panics.
func ReloadAuraCredential(fs afero.Fs, filePath, name string) (*AuraCredential, bool) {
	data, err := afero.ReadFile(fs, filePath)
	if err != nil {
		return nil, false
	}

	var cf CredentialsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, false
	}

	if cf.Aura == nil {
		return nil, false
	}

	for _, cred := range cf.Aura.Credentials {
		if cred.Name == name {
			return cred, true
		}
	}

	return nil, false
}
