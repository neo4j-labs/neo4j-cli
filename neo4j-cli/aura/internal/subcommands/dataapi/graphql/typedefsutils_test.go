// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package graphql_test

import (
	"testing"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clicfg/fileutils"
	"github.com/neo4j/cli/neo4j-cli/aura/internal/subcommands/dataapi/graphql"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
)

func TestGetTypeDefsFromFlag(t *testing.T) {
	validTypeDefs := "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ=="
	typDefs := `type Movie {
  title: String
}`
	pathToTypeDefsFile := "typeDefs.graphql"

	fs, err := testfs.GetDefaultTestFs()
	if err != nil {
		t.Fatal(err.Error())
	}
	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)

	fileutils.WriteFile(fs, pathToTypeDefsFile, []byte(typDefs))

	tests := map[string]struct {
		typeDefsValue     string
		typeDefsFileValue string
		expectedValue     string
		expectedErrorMsg  string
	}{
		"valid type defs": {
			typeDefsValue:     validTypeDefs,
			typeDefsFileValue: "",
			expectedValue:     "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwkKfQ==",
			expectedErrorMsg:  "",
		},
		"invalid type defs": {
			typeDefsValue:     "dd",
			typeDefsFileValue: "",
			expectedValue:     "",
			expectedErrorMsg:  "provided type definitions are not valid base64",
		},
		"valid type defs path": {
			typeDefsValue:     "",
			typeDefsFileValue: pathToTypeDefsFile,
			expectedValue:     "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwp9",
			expectedErrorMsg:  "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			val, err := graphql.GetTypeDefsFromFlag(cfg, test.typeDefsValue, test.typeDefsFileValue)
			if test.expectedErrorMsg != "" {
				assert.Contains(t, err.Error(), test.expectedErrorMsg)
			} else {
				assert.Equal(t, test.expectedValue, val)
			}
		})
	}
}

func TestResolveTypeDefsFileFlagValue(t *testing.T) {
	typDefs := `type Movie {
  title: String
}`
	pathToTypeDefsFile := "typeDefs.graphql"
	pathToEmptyFile := "empty.graphql"

	fs, err := testfs.GetDefaultTestFs()
	if err != nil {
		t.Fatal(err.Error())
	}

	fileutils.WriteFile(fs, pathToTypeDefsFile, []byte(typDefs))
	fileutils.WriteFile(fs, pathToEmptyFile, []byte(""))

	tests := map[string]struct {
		flagValue        string
		expectedValue    string
		expectedErrorMsg string
	}{
		"correct path to file": {
			flagValue:        pathToTypeDefsFile,
			expectedValue:    "dHlwZSBNb3ZpZSB7CiAgdGl0bGU6IFN0cmluZwp9",
			expectedErrorMsg: "",
		},
		"invalid path": {
			flagValue:        "path/to/no-file.txt",
			expectedValue:    "",
			expectedErrorMsg: "type definitions file 'path/to/no-file.txt' does not exist",
		},
		"empty file": {
			flagValue:        pathToEmptyFile,
			expectedValue:    "",
			expectedErrorMsg: "type definitions file 'empty.graphql' does not exist",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			val, err := graphql.ResolveTypeDefsFileFlagValue(fs, test.flagValue)
			if test.expectedErrorMsg != "" {
				assert.Contains(t, err.Error(), test.expectedErrorMsg)
			} else {
				assert.Equal(t, test.expectedValue, val)
			}
		})
	}
}
