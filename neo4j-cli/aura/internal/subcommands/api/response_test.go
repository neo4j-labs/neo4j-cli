// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"bytes"
	"net/http"
	"net/url"
	"testing"

	auraapi "github.com/neo4j/cli/neo4j-cli/aura/internal/api"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestPrintResponseMeta_StripsControlBytes covers the case a real server cannot
// produce: net/http rejects a control byte in a response header, but --base-url
// may point anywhere, so the escape is neutralised rather than trusted.
func TestPrintResponseMeta_StripsControlBytes(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "api"}
	cmd.SetOut(&out)

	printResponseMeta(cmd, &auraapi.RawResponse{
		Proto:  "HTTP/1.1",
		Status: "200 \x1b[31mOK",
		Header: http.Header{
			"X-Trace-Id":   []string{"abc\x1b[31m"},
			"Content-Type": []string{"application/json"},
		},
	})

	got := out.String()
	assert.NotContains(t, got, "\x1b")
	assert.Equal(t, "HTTP/1.1 200 ?[31mOK\nContent-Type: application/json\nX-Trace-Id: abc?[31m\n\n", got)
}

// TestPrintResponseMeta_PrintsEveryValueOfARepeatedHeader pins that a repeated
// header (e.g. Set-Cookie) is not collapsed to its first value.
func TestPrintResponseMeta_PrintsEveryValueOfARepeatedHeader(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{Use: "api"}
	cmd.SetOut(&out)

	printResponseMeta(cmd, &auraapi.RawResponse{
		Proto:  "HTTP/1.1",
		Status: "200 OK",
		Header: http.Header{"Link": []string{"<a>; rel=next", "<b>; rel=prev"}},
	})

	assert.Equal(t, "HTTP/1.1 200 OK\nLink: <a>; rel=next\nLink: <b>; rel=prev\n\n", out.String())
}

func TestMergeQuery(t *testing.T) {
	testCases := []struct {
		name   string
		inline url.Values
		fields url.Values
		want   url.Values
	}{
		{
			name: "both empty",
			want: url.Values{},
		},
		{
			name:   "inline only, repeated keys preserved",
			inline: url.Values{"a": []string{"1", "2"}},
			want:   url.Values{"a": []string{"1", "2"}},
		},
		{
			name:   "disjoint keys are unioned",
			inline: url.Values{"include_deleted": []string{"true"}},
			fields: url.Values{"page_limit": []string{"10"}},
			want:   url.Values{"include_deleted": []string{"true"}, "page_limit": []string{"10"}},
		},
		{
			name:   "an explicit field wins over the inline value",
			inline: url.Values{"page_limit": []string{"1", "2"}},
			fields: url.Values{"page_limit": []string{"10"}},
			want:   url.Values{"page_limit": []string{"10"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mergeQuery(tc.inline, tc.fields))
		})
	}
}
