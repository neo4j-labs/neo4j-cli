// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package credential_test

import (
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
	"github.com/stretchr/testify/assert"
)

func TestListCredentials(t *testing.T) {
	tests := []struct {
		name          string
		command       string
		credentials   []map[string]string
		wantOutFunc   func(t *testing.T, out string)
		wantErrPrefix string
	}{
		{
			name:    "list credentials as json",
			command: "credential list --output json",
			credentials: []map[string]string{
				{"name": "test-cred", "client-id": "client-abc", "client-secret": "secret-abc"},
				{"name": "other-cred", "client-id": "client-xyz", "client-secret": "secret-xyz"},
			},
			wantOutFunc: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, `"name": "test-cred"`)
				assert.Contains(t, out, `"client-id": "client-abc"`)
				assert.Contains(t, out, `"name": "other-cred"`)
				assert.Contains(t, out, `"client-id": "client-xyz"`)
			},
		},
		{
			name:    "list credentials as table",
			command: "credential list --output table",
			credentials: []map[string]string{
				{"name": "test-cred", "client-id": "client-abc", "client-secret": "secret-abc"},
				{"name": "other-cred", "client-id": "client-xyz", "client-secret": "secret-xyz"},
			},
			wantOutFunc: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "NAME")
				assert.Contains(t, out, "CLIENT-ID")
				assert.Contains(t, out, "test-cred")
				assert.Contains(t, out, "client-abc")
				assert.Contains(t, out, "other-cred")
				assert.Contains(t, out, "client-xyz")
			},
		},
		{
			name:    "list credentials default output uses configured output mode (json in test helper)",
			command: "credential list",
			credentials: []map[string]string{
				{"name": "default-cred", "client-id": "client-default", "client-secret": "secret-default"},
			},
			wantOutFunc: func(t *testing.T, out string) {
				t.Helper()
				// Test helper configures output=json, so default mode renders JSON
				assert.Contains(t, out, `"name": "default-cred"`)
				assert.Contains(t, out, `"client-id": "client-default"`)
			},
		},
		{
			name:        "list credentials with empty list",
			command:     "credential list --output json",
			credentials: []map[string]string{},
			wantOutFunc: func(t *testing.T, out string) {
				t.Helper()
				assert.Contains(t, out, "[]")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			helper.SetCredentialsValue("aura.credentials", tc.credentials)

			helper.ExecuteCommand(tc.command)

			if tc.wantErrPrefix != "" {
				helper.AssertErr(tc.wantErrPrefix)
			} else {
				out := helper.PrintOut()
				tc.wantOutFunc(t, out)
			}
		})
	}
}
