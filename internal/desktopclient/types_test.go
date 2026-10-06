// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"encoding/json"
	"testing"
)

func TestDbmsInfo_TolerantUnmarshal(t *testing.T) {
	// Minimal payload — Desktop omits all but id/name in some edge cases
	// (e.g. when a DBMS is mid-creation). Parser must not reject these.
	body := `{"id":"x","name":"X"}`
	var got DbmsInfo
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != "x" || got.Name != "X" {
		t.Fatalf("got %+v", got)
	}
	if got.Version != "" || got.Edition != "" || got.Status != "" {
		t.Fatalf("expected zero-value optional fields, got %+v", got)
	}
}

func TestDbmsInfo_FullPayload(t *testing.T) {
	body := `{
		"id": "abc",
		"name": "graph",
		"description": "test",
		"tags": ["t1", "t2"],
		"connectionUri": "neo4j://localhost:7687",
		"status": "online",
		"serverStatus": "running",
		"version": "5.18.0",
		"edition": "enterprise",
		"prerelease": "rc1"
	}`
	var got DbmsInfo
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ConnectionURI != "neo4j://localhost:7687" {
		t.Fatalf("ConnectionURI = %q", got.ConnectionURI)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "t1" {
		t.Fatalf("Tags = %v", got.Tags)
	}
	if got.Edition != "enterprise" {
		t.Fatalf("Edition = %q", got.Edition)
	}
}

func TestEnvJSON_MissingOptionalsOK(t *testing.T) {
	body := `{"name":"Default","id":"x","active":true,"type":"LOCAL"}`
	var got EnvJSON
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.RelateDataPath != "" || got.HTTPOrigin != "" {
		t.Fatalf("expected empty optionals, got %+v", got)
	}
}

func TestCredentials_Unmarshal(t *testing.T) {
	body := `{"username":"neo4j","password":"hunter2"}`
	var got Credentials
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Username != "neo4j" || got.Password != "hunter2" {
		t.Fatalf("got %+v", got)
	}
}
