// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/neo4j/cli/common/clierr"
)

// TestExitCodeForStatus_CrossPathConsistency verifies that every modelled and
// unmodelled HTTP status produces the same clierr.CLIError.Code through both
// error paths — handleResponseError and RawStatusError — for every status
// where a side-effect-free comparison is possible.
//
// 401 and 429 are excluded: 401 routes through formatAuthorizationError
// (ClearAccessToken side effect requiring a real credential), and 429 reads
// the Retry-After header and uses NewRateLimitError (different signature).
//
// The list covers explicit switch cases in both paths (400, 402, 403, 404,
// 405, 409, 500, 502, 503, 504), statuses explicit in one path and
// class-fallback in the other (413, 415, 422), and pure class-fallback
// statuses (307, 308, 408, 425, 451, 507, 599).
func TestExitCodeForStatus_CrossPathConsistency(t *testing.T) {
	statuses := []int{
		307, 308,
		400, 402, 403, 404, 405, 408, 409,
		413, 415, 422, 425,
		451,
		500, 502, 503, 504, 507,
		599,
	}

	for _, s := range statuses {
		s := s
		name := fmt.Sprintf("status_%d", s)
		t.Run(name, func(t *testing.T) {
			body := `{"errors":[{"message":"test"}]}`
			if s == http.StatusForbidden {
				body = `{"error":"access denied"}`
			}

			// Build handleResponseError input. 404 reads res.Request for
			// resource-type tagging; other statuses ignore it.
			httpResp := &http.Response{
				StatusCode: s,
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    &http.Request{URL: &url.URL{Path: "/v1/instances/test-id"}},
			}

			// Build RawStatusError input.
			rawResp := &RawResponse{
				StatusCode: s,
				Status:     fmt.Sprintf("%d %s", s, http.StatusText(s)),
				Header:     http.Header{},
				Body:       []byte(body),
			}

			// Both functions may return nil for non-error paths (2xx), but
			// every status in the list is non-2xx so both return an error.
			httpErr := handleResponseError(httpResp, nil, nil)
			rawErr := RawStatusError(rawResp)

			if httpErr == nil {
				t.Fatal("handleResponseError returned nil for non-2xx status")
			}
			if rawErr == nil {
				t.Fatal("RawStatusError returned nil for non-2xx status")
			}

			var httpCE, rawCE *clierr.CLIError
			if !errors.As(httpErr, &httpCE) {
				t.Fatalf("handleResponseError returned non-CLIError: %T", httpErr)
			}
			if !errors.As(rawErr, &rawCE) {
				t.Fatalf("RawStatusError returned non-CLIError: %T", rawErr)
			}

			if httpCE.Code != rawCE.Code {
				t.Errorf("Code mismatch: handleResponseError=%d, RawStatusError=%d", httpCE.Code, rawCE.Code)
			}
		})
	}
}
