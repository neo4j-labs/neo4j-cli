// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"fmt"
	"testing"

	"github.com/neo4j/cli/common/clicfg/credentials"
	"github.com/neo4j/cli/common/clierr"
	"github.com/stretchr/testify/assert"
)

// stubDbmsGetter is a test double for dbmsGetter that treats a fixed set of
// names as "taken" and returns an error for all others.
type stubDbmsGetter struct {
	taken map[string]bool
}

func (s *stubDbmsGetter) Get(name string) (*credentials.DbmsCredential, error) {
	if s.taken[name] {
		return &credentials.DbmsCredential{Name: name}, nil
	}
	return nil, clierr.NewUsageError("could not find credential with name %s", name)
}

func TestDatabaseName(t *testing.T) {
	testCases := []struct {
		name         string
		instanceType string
		username     string
		want         string
	}{
		{
			name:         "free-db with non-neo4j username returns username",
			instanceType: "free-db",
			username:     "alice",
			want:         "alice",
		},
		{
			name:         "free-db with neo4j username returns neo4j",
			instanceType: "free-db",
			username:     "neo4j",
			want:         "neo4j",
		},
		{
			name:         "professional-db always returns neo4j",
			instanceType: "professional-db",
			username:     "alice",
			want:         "neo4j",
		},
		{
			name:         "business-critical always returns neo4j",
			instanceType: "business-critical",
			username:     "alice",
			want:         "neo4j",
		},
		{
			name:         "empty instance type returns neo4j",
			instanceType: "",
			username:     "alice",
			want:         "neo4j",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := databaseName(tc.instanceType, tc.username)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBaseCredentialName(t *testing.T) {
	testCases := []struct {
		name       string
		instanceID string
		customName string
		want       string
	}{
		{
			name:       "empty customName returns instanceID-default",
			instanceID: "abc",
			customName: "",
			want:       "abc-default",
		},
		{
			name:       "non-empty customName is returned as-is",
			instanceID: "abc",
			customName: "myname",
			want:       "myname",
		},
		{
			name:       "non-empty customName with empty instanceID",
			instanceID: "",
			customName: "myname",
			want:       "myname",
		},
		{
			name:       "empty customName with empty instanceID",
			instanceID: "",
			customName: "",
			want:       "-default",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := baseCredentialName(tc.instanceID, tc.customName)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveCredentialName(t *testing.T) {
	testCases := []struct {
		name  string
		taken []string
		base  string
		want  string
	}{
		{
			name:  "base name is free",
			taken: nil,
			base:  "abc-default",
			want:  "abc-default",
		},
		{
			name:  "base taken, first suffix free",
			taken: []string{"abc-default"},
			base:  "abc-default",
			want:  "abc-default-1",
		},
		{
			name:  "base and -1 taken, -2 is free",
			taken: []string{"abc-default", "abc-default-1"},
			base:  "abc-default",
			want:  "abc-default-2",
		},
		{
			name:  "base and first two suffixes taken, -3 is free",
			taken: []string{"abc-default", "abc-default-1", "abc-default-2"},
			base:  "abc-default",
			want:  "abc-default-3",
		},
		{
			name:  "custom base name free",
			taken: nil,
			base:  "myname",
			want:  "myname",
		},
		{
			name:  "custom base taken",
			taken: []string{"myname"},
			base:  "myname",
			want:  "myname-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			takenSet := make(map[string]bool, len(tc.taken))
			for _, n := range tc.taken {
				takenSet[n] = true
			}
			dbms := &stubDbmsGetter{taken: takenSet}
			got := resolveCredentialName(dbms, tc.base)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestResolveCredentialNameIntegrationWithFormat verifies that the suffix format
// matches the "<base>-N" pattern documented in the acceptance criteria.
func TestResolveCredentialNameIntegrationWithFormat(t *testing.T) {
	base := "db1d1234-default"
	// Simulate 5 collisions.
	takenSet := map[string]bool{base: true}
	for i := 1; i <= 4; i++ {
		takenSet[fmt.Sprintf("%s-%d", base, i)] = true
	}
	dbms := &stubDbmsGetter{taken: takenSet}
	got := resolveCredentialName(dbms, base)
	assert.Equal(t, "db1d1234-default-5", got)
}
