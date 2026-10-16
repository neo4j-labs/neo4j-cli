// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"bufio"
	"encoding/json"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readReceipts(t *testing.T, fs afero.Fs, path string) []map[string]any {
	t.Helper()
	f, err := fs.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		require.NoError(t, json.Unmarshal(sc.Bytes(), &m))
		out = append(out, m)
	}
	return out
}

func TestAppendReceipt(t *testing.T) {
	fs := afero.NewMemMapFs()
	path := "/cfg/neo4j/cli/feedback-receipts.jsonl"
	p := testPayload()

	got := appendReceipt(fs, path, Receipt{
		Outcome: "sent", RequestID: p.RequestID, ReportID: "rpt_1", Destination: "http://127.0.0.1:8765/v1/feedback",
		ElapsedMS: 12, Payload: &p,
	})
	require.Equal(t, path, got)
	got = appendReceipt(fs, path, Receipt{
		Outcome: "rejected", Reason: "policy_violation", RequestID: p.RequestID, Destination: "http://127.0.0.1:8765/v1/feedback",
	})
	require.Equal(t, path, got)

	lines := readReceipts(t, fs, path)
	require.Len(t, lines, 2)
	for _, l := range lines {
		for _, k := range []string{"time", "outcome", "reason", "request_id", "report_id", "destination", "elapsed_ms", "payload"} {
			assert.Contains(t, l, k)
		}
	}
	assert.NotNil(t, lines[0]["payload"])
	assert.Nil(t, lines[1]["payload"], "local rejection must store payload: null")
	assert.Equal(t, "rpt_1", lines[0]["report_id"])

	info, err := fs.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, "-rw-------", info.Mode().Perm().String())
}

func TestAppendReceipt_NilFs(t *testing.T) {
	assert.Equal(t, "", appendReceipt(nil, "/x", Receipt{}))
}
