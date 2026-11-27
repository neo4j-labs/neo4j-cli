// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package relate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyOpenOpts_SetsEngineEnv(t *testing.T) {
	t.Setenv("RELATE_SNAPSHOT_FORMAT", "json") // ambient value must lose
	t.Setenv("RELATE_MARKDOWN_BODY_PROPERTIES", "")
	t.Setenv("RELATE_DURABILITY_MODE", "")

	require.NoError(t, ApplyOpenOpts([]string{
		"snapshot-format=markdown",
		"markdown-body-properties=Person=bio;Movie=plot;summary", // value keeps '=' and ';'
		"durability-mode=everysecond",
	}))
	assert.Equal(t, "markdown", os.Getenv("RELATE_SNAPSHOT_FORMAT"))
	assert.Equal(t, "Person=bio;Movie=plot;summary", os.Getenv("RELATE_MARKDOWN_BODY_PROPERTIES"))
	assert.Equal(t, "everysecond", os.Getenv("RELATE_DURABILITY_MODE"))
}

func TestApplyOpenOpts_Rejects(t *testing.T) {
	t.Setenv("RELATE_SNAPSHOT_FORMAT", "")
	t.Setenv("RELATE_CHECKPOINT_INTERVAL_SECS", "")
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"no equals", []string{"snapshot-format"}, "expected key=value"},
		{"empty key", []string{"=x"}, "expected key=value"},
		{"unknown key", []string{"preview=ontology"}, `unknown key "preview"`},
		{"bad enum", []string{"snapshot-format=csv"}, "must be one of parquet, json, markdown"},
		{"bad int", []string{"checkpoint-interval-secs=-1"}, "non-negative integer"},
		{"empty value", []string{"markdown-body-properties= "}, "must not be empty"},
		{"size with decimals", []string{"logical-graph-size-limit=1.5gb"}, "whole number"},
		{"size with kb", []string{"logical-graph-size-limit=10kb"}, "mb, mib, gb or gib"},
		{"banana size", []string{"expression-memory-limit=banana"}, "or none"},
		{"none is only for expression-memory-limit", []string{"logical-graph-size-limit=none"}, "mb, mib, gb or gib"},
		{"duplicate", []string{"snapshot-format=json", "snapshot-format=markdown"}, "more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyOpenOpts(tt.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestApplyOpenOpts_AllOrNothing(t *testing.T) {
	t.Setenv("RELATE_SNAPSHOT_FORMAT", "")
	require.Error(t, ApplyOpenOpts([]string{"snapshot-format=markdown", "durability-mode=bogus"}))
	assert.Empty(t, os.Getenv("RELATE_SNAPSHOT_FORMAT"), "a rejected set must export nothing")
}

func TestApplyOpenOpts_NoneIsNoop(t *testing.T) {
	require.NoError(t, ApplyOpenOpts(nil))
}

func TestApplyOpenOpts_AcceptsEngineMemoryGrammar(t *testing.T) {
	t.Setenv("RELATE_LOGICAL_GRAPH_SIZE_LIMIT", "")
	t.Setenv("RELATE_EXPRESSION_MEMORY_LIMIT", "")
	for _, v := range []string{"512mb", "1GiB", "100", "2 gb", "0"} {
		require.NoError(t, ApplyOpenOpts([]string{"logical-graph-size-limit=" + v}), v)
	}
	require.NoError(t, ApplyOpenOpts([]string{"expression-memory-limit=none"}))
}
