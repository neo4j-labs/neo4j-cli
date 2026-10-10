// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/clierr"
)

// assertFatalFinderError pins the shared finder policy: every failure is a
// clierr FATAL error (exit 1) whose message carries each wanted substring.
func assertFatalFinderError(t *testing.T, err error, substrings ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected fatal error, got nil")
	}
	var ce *clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != 1 {
		t.Fatalf("error = %v (%T), want clierr fatal error (exit 1)", err, err)
	}
	for _, s := range substrings {
		if !strings.Contains(ce.Message, s) {
			t.Fatalf("message = %q, want substring %q", ce.Message, s)
		}
	}
}

func TestFindProjectByName(t *testing.T) {
	cases := []struct {
		name     string
		state    *ProjectsState
		selector string
		wantID   string
		wantErr  []string
	}{
		{
			name:     "nil state is a fatal miss",
			state:    nil,
			selector: "Proj",
			wantErr:  []string{`project "Proj" is missing from the catalog state`},
		},
		{
			name:     "absent from state is a fatal miss",
			state:    &ProjectsState{Projects: []Project{{ID: "p1", Name: "Other"}}},
			selector: "Proj",
			wantErr:  []string{`project "Proj" is missing from the catalog state`},
		},
		{
			name: "single match is returned",
			state: &ProjectsState{Projects: []Project{
				{ID: "p1", Name: "Other"},
				{ID: "p2", Name: "Proj", CreatedAt: 1747843200000},
			}},
			selector: "Proj",
			wantID:   "p2",
		},
		{
			name: "match is case-sensitive",
			state: &ProjectsState{Projects: []Project{
				{ID: "p1", Name: "proj"},
			}},
			selector: "Proj",
			wantErr:  []string{`project "Proj" is missing from the catalog state`},
		},
		{
			name: "duplicate names are a fatal ambiguity listing candidate ids",
			state: &ProjectsState{Projects: []Project{
				{ID: "p-old", Name: "Proj", CreatedAt: 1700000000000},
				{ID: "p-new", Name: "Proj", CreatedAt: 1747843200000},
			}},
			selector: "Proj",
			wantErr:  []string{`project "Proj" matches 2 entries`, "p-old", "p-new", "duplicates"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindProjectByName(tc.state, tc.selector)
			if len(tc.wantErr) > 0 {
				assertFatalFinderError(t, err, tc.wantErr...)
				return
			}
			if err != nil {
				t.Fatalf("FindProjectByName: %v", err)
			}
			if got == nil || got.ID != tc.wantID {
				t.Fatalf("got %+v, want id %q", got, tc.wantID)
			}
		})
	}
}

func TestFindProjectByID(t *testing.T) {
	cases := []struct {
		name     string
		state    *ProjectsState
		selector string
		wantID   string
		wantErr  []string
	}{
		{
			name:     "nil state is a fatal miss",
			state:    nil,
			selector: "p2",
			wantErr:  []string{`project "p2" is missing from the catalog state`},
		},
		{
			name:     "absent id is a fatal miss",
			state:    &ProjectsState{Projects: []Project{{ID: "p1", Name: "A"}}},
			selector: "p2",
			wantErr:  []string{`project "p2" is missing from the catalog state`},
		},
		{
			name: "matching id is returned even among same-named entries",
			state: &ProjectsState{Projects: []Project{
				{ID: "p1", Name: "Dup"},
				{ID: "p2", Name: "Dup"},
			}},
			selector: "p2",
			wantID:   "p2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindProjectByID(tc.state, tc.selector)
			if len(tc.wantErr) > 0 {
				assertFatalFinderError(t, err, tc.wantErr...)
				return
			}
			if err != nil {
				t.Fatalf("FindProjectByID: %v", err)
			}
			if got == nil || got.ID != tc.wantID {
				t.Fatalf("got %+v, want id %q", got, tc.wantID)
			}
		})
	}
}

func TestFindTagByName(t *testing.T) {
	cases := []struct {
		name     string
		state    *TagsState
		selector string
		wantID   string
		wantErr  []string
	}{
		{
			name:     "nil state is a fatal miss",
			state:    nil,
			selector: "Prod",
			wantErr:  []string{`tag "Prod" is missing from the catalog state`},
		},
		{
			name:     "absent from state is a fatal miss",
			state:    &TagsState{Tags: []Tag{{ID: "t1", Name: "Dev"}}},
			selector: "Prod",
			wantErr:  []string{`tag "Prod" is missing from the catalog state`},
		},
		{
			name: "single match is returned",
			state: &TagsState{Tags: []Tag{
				{ID: "t0", Name: "Dev"},
				{ID: "t1", Name: "Prod", Color: "5"},
			}},
			selector: "Prod",
			wantID:   "t1",
		},
		{
			name: "duplicate names are a fatal ambiguity listing candidate ids",
			state: &TagsState{Tags: []Tag{
				{ID: "t-old", Name: "Prod", Color: "1"},
				{ID: "t-new", Name: "Prod", Color: "5"},
			}},
			selector: "Prod",
			wantErr:  []string{`tag "Prod" matches 2 entries`, "t-old", "t-new", "duplicates"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindTagByName(tc.state, tc.selector)
			if len(tc.wantErr) > 0 {
				assertFatalFinderError(t, err, tc.wantErr...)
				return
			}
			if err != nil {
				t.Fatalf("FindTagByName: %v", err)
			}
			if got == nil || got.ID != tc.wantID {
				t.Fatalf("got %+v, want id %q", got, tc.wantID)
			}
		})
	}
}

func TestFindTagByID(t *testing.T) {
	cases := []struct {
		name     string
		state    *TagsState
		selector string
		wantID   string
		wantErr  []string
	}{
		{
			name:     "nil state is a fatal miss",
			state:    nil,
			selector: "t2",
			wantErr:  []string{`tag "t2" is missing from the catalog state`},
		},
		{
			name:     "absent id is a fatal miss",
			state:    &TagsState{Tags: []Tag{{ID: "t1", Name: "Prod"}}},
			selector: "t2",
			wantErr:  []string{`tag "t2" is missing from the catalog state`},
		},
		{
			name: "matching id is returned even among same-named entries",
			state: &TagsState{Tags: []Tag{
				{ID: "t1", Name: "Dup"},
				{ID: "t2", Name: "Dup", Color: "7"},
			}},
			selector: "t2",
			wantID:   "t2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindTagByID(tc.state, tc.selector)
			if len(tc.wantErr) > 0 {
				assertFatalFinderError(t, err, tc.wantErr...)
				return
			}
			if err != nil {
				t.Fatalf("FindTagByID: %v", err)
			}
			if got == nil || got.ID != tc.wantID {
				t.Fatalf("got %+v, want id %q", got, tc.wantID)
			}
		})
	}
}
