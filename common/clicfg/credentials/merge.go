// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credentials

// mergeCredentialsFile merges in-memory state onto the current disk state.
//
// In-memory is authoritative for the credential list (adds and removes are
// respected). For Aura access tokens, whichever copy has the later expiry is
// kept — so a fresher token written by a parallel process is not overwritten
// by a stale in-memory value. For Dbms and Embed (no tokens), in-memory wins.
func mergeCredentialsFile(disk, mem *CredentialsFile) *CredentialsFile {
	return &CredentialsFile{
		Aura:  mergeAura(disk.Aura, mem.Aura),
		Dbms:  mem.Dbms,
		Embed: mem.Embed,
	}
}

func mergeAura(disk, mem *AuraCredentials) *AuraCredentials {
	if mem == nil {
		return disk
	}
	if disk == nil {
		return mem
	}

	diskByName := make(map[string]*AuraCredential, len(disk.Credentials))
	for _, c := range disk.Credentials {
		diskByName[c.Name] = c
	}

	result := &AuraCredentials{
		DefaultCredential: mem.DefaultCredential,
		Credentials:       make([]*AuraCredential, 0, len(mem.Credentials)),
	}

	for _, mc := range mem.Credentials {
		merged := *mc
		// Only defer to a fresher disk token when in-memory still holds a token
		// (AccessToken != ""). An empty in-memory token means it was explicitly
		// cleared (e.g. on 401/403) and must not be restored from disk.
		if mc.AccessToken != "" {
			if dc, ok := diskByName[mc.Name]; ok && dc.TokenExpiry > mc.TokenExpiry {
				merged.AccessToken = dc.AccessToken
				merged.TokenExpiry = dc.TokenExpiry
			}
		}
		result.Credentials = append(result.Credentials, &merged)
	}

	return result
}
