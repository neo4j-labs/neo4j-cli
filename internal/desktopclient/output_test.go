// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFormatUnixMsRFC3339(t *testing.T) {
	tests := []struct {
		name   string
		unixMs int64
		want   string
	}{
		{name: "positive unix-ms renders RFC3339 UTC", unixMs: 1747843200000, want: "2025-05-21T16:00:00Z"},
		{name: "zero renders empty", unixMs: 0, want: ""},
		{name: "negative renders empty", unixMs: -1, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatUnixMsRFC3339(tc.unixMs); got != tc.want {
				t.Fatalf("FormatUnixMsRFC3339(%d) = %q, want %q", tc.unixMs, got, tc.want)
			}
		})
	}
}

func TestConnectionOutputs_CreatedAtRendering(t *testing.T) {
	// Desktop 2.3 reports createdAt as unix-milliseconds; the CLI renders it
	// as RFC3339 UTC under `created_at`, omitted when Desktop does not report
	// one (older Desktop versions / legacy entries).
	const unixMs = 1747843200000
	tests := []struct {
		name       string
		connection Connection
		wantKey    bool
	}{
		{
			name:       "numeric createdAt renders RFC3339 UTC",
			connection: Connection{ID: "c1", Name: "aura", ConnectionURI: "neo4j+s://abc.databases.neo4j.io", CreatedAt: unixMs},
			wantKey:    true,
		},
		{
			name:       "absent createdAt is omitted",
			connection: Connection{ID: "c2", Name: "local", ConnectionURI: "neo4j://localhost:7687"},
			wantKey:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(ConnectionOutputs([]Connection{tc.connection}))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got []map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal: %v (raw: %s)", err, raw)
			}
			if len(got) != 1 {
				t.Fatalf("len = %d, want 1", len(got))
			}
			gotVal, ok := got[0]["created_at"]
			if ok != tc.wantKey {
				t.Fatalf("created_at presence = %v, want %v (raw: %s)", ok, tc.wantKey, raw)
			}
			if tc.wantKey {
				if want := time.UnixMilli(unixMs).UTC().Format(time.RFC3339); gotVal != want {
					t.Fatalf("created_at = %v, want %q", gotVal, want)
				}
			}
		})
	}
}
