// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/neo4j-cli/internal/subcommands/credential/dbms"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
)

func TestDbmsSetEmbed_WriteAnnotation(t *testing.T) {
	fs, err := testfs.GetTestFs("{}", "{}")
	assert.Nil(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	cmd := dbms.NewCmd(cfg)
	for _, c := range cmd.Commands() {
		if c.Name() == "set-embed" {
			assert.Equal(t, "true", c.Annotations["write"], "set-embed must be annotated write=true")
			return
		}
	}
	t.Fatalf("set-embed leaf not registered on dbms command")
}

func TestDbmsCredentialSetEmbed(t *testing.T) {
	tests := []struct {
		name              string
		initialDbmsCreds  []map[string]interface{}
		initialEmbedCreds []map[string]interface{}
		command           string
		wantErr           string
		// wantEmbedOnDbms is the embed-credential value expected on the FIRST dbms cred
		// after the command runs ("" means link cleared/absent on disk via omitempty).
		wantEmbedOnDbms string
	}{
		{
			name: "set-embed links a dbms cred to an embed cred",
			initialDbmsCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			},
			initialEmbedCreds: []map[string]interface{}{
				{"name": "myembed", "provider": "openai", "model": "text-embedding-3-small", "base-url": "", "dimensions": 0, "api-key": "k"},
			},
			command:         "set-embed mydb myembed",
			wantEmbedOnDbms: "myembed",
		},
		{
			name: "set-embed with one arg clears the link",
			initialDbmsCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687", "embed-credential": "myembed"},
			},
			initialEmbedCreds: []map[string]interface{}{
				{"name": "myembed", "provider": "openai", "model": "text-embedding-3-small", "base-url": "", "dimensions": 0, "api-key": "k"},
			},
			command:         "set-embed mydb",
			wantEmbedOnDbms: "",
		},
		{
			name: "set-embed pointing at missing dbms cred returns clear usage error",
			initialDbmsCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			},
			initialEmbedCreds: []map[string]interface{}{
				{"name": "myembed", "provider": "openai", "model": "text-embedding-3-small", "base-url": "", "dimensions": 0, "api-key": "k"},
			},
			command: "set-embed nope myembed",
			wantErr: `no dbms credential named "nope"`,
		},
		{
			name: "set-embed pointing at missing embed cred returns clear usage error",
			initialDbmsCreds: []map[string]interface{}{
				{"name": "mydb", "username": "neo4j", "password": "secret", "database-name": "neo4j", "uri": "bolt://localhost:7687"},
			},
			initialEmbedCreds: []map[string]interface{}{},
			command:           "set-embed mydb nope",
			wantErr:           `no embed credential named "nope"`,
		},
		{
			name:              "set-embed with no args returns arg-count error",
			initialDbmsCreds:  []map[string]interface{}{},
			initialEmbedCreds: []map[string]interface{}{},
			command:           "set-embed",
			wantErr:           "accepts between 1 and 2 arg(s)",
		},
		{
			name:              "set-embed with too many args returns arg-count error",
			initialDbmsCreds:  []map[string]interface{}{},
			initialEmbedCreds: []map[string]interface{}{},
			command:           "set-embed mydb myembed extra",
			wantErr:           "accepts between 1 and 2 arg(s)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newDbmsTestHelper(t)
			h.setCredentialsValue("dbms.credentials", tc.initialDbmsCreds)
			h.setCredentialsValue("embed.credentials", tc.initialEmbedCreds)

			h.executeCommand(tc.command) //nolint:errcheck // error checked via assertErr

			if tc.wantErr != "" {
				h.assertErr(tc.wantErr)
				return
			}

			h.assertErr("")
			// Inspect the persisted dbms credential's embed-credential field.
			// JSON omitempty drops the key when empty, so we assert via gjson on the array.
			if tc.wantEmbedOnDbms == "" {
				h.assertCredentialsValue("dbms.credentials.0.embed-credential", "")
			} else {
				h.assertCredentialsValue("dbms.credentials.0.embed-credential", tc.wantEmbedOnDbms)
			}
		})
	}
}
