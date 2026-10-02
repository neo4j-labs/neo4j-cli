// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

type PollResponse struct {
	Data struct {
		Id string
		// v2beta1 emits the operational state as legacy_status; see normalize().
		Status       string
		LegacyStatus string `json:"legacy_status"`
	}
}

// normalize applies the v2beta1 legacy_status->status mapping so readiness is
// evaluated against a stable field regardless of API version. A native status
// always wins over legacy_status.
func (r *PollResponse) normalize() {
	if r.Data.Status == "" {
		r.Data.Status = r.Data.LegacyStatus
	}
}

func PollInstance(ctx context.Context, cfg *clicfg.Config, orgID, projectID, instanceId string, waitingStatus string) (*PollResponse, error) {
	path := ScopedInstancePath(orgID, projectID, instanceId)
	return PollWithVersion(ctx, cfg, path, AuraApiVersion2, func(status string) bool {
		return status != waitingStatus
	})
}

func PollSnapshot(ctx context.Context, cfg *clicfg.Config, instanceId string, snapshotId string) (*PollResponse, error) {
	path := fmt.Sprintf("/instances/%s/snapshots/%s", instanceId, snapshotId)
	return Poll(ctx, cfg, path, func(status string) bool {
		return status != SnapshotStatusPending && status != SnapshotStatusInProgress
	})
}

func PollCMK(ctx context.Context, cfg *clicfg.Config, cmkId string) (*PollResponse, error) {
	path := fmt.Sprintf("/customer-managed-keys/%s", cmkId)
	return Poll(ctx, cfg, path, func(status string) bool {
		return status != CMKStatusPending
	})
}

func PollGraphQLDataApi(ctx context.Context, cfg *clicfg.Config, instanceId string, graphQLDataApiId string, waitingStatus string) (*PollResponse, error) {
	path := fmt.Sprintf("/instances/%s/data-apis/graphql/%s", instanceId, graphQLDataApiId)
	return PollWithVersion(ctx, cfg, path, AuraApiVersionBeta1, func(status string) bool {
		return status != waitingStatus
	})
}

func PollGraphAnalyticsSessionReady(ctx context.Context, cfg *clicfg.Config, orgID, projectID, sessionId string, waitingStatus []string) (*PollResponse, error) {
	path := ScopedSessionPath(orgID, projectID, sessionId)
	return PollWithVersion(ctx, cfg, path, AuraApiVersion2, func(status string) bool {
		return !slices.Contains(waitingStatus, status)
	})
}

// PollVirtualGraph waits until the virtual graph's status leaves waitingStatus.
//
// The comparison is case-insensitive on purpose: the status casing returned by
// the API is not guaranteed. A case-sensitive compare would make the first
// poll's condition trivially true whenever the casing differs from the
// VirtualGraphStatus* constant, returning immediately so --wait would silently
// not wait at all.
func PollVirtualGraph(ctx context.Context, cfg *clicfg.Config, orgID, projectID, virtualGraphID string, waitingStatus string) (*PollResponse, error) {
	path := ScopedVirtualGraphPath(orgID, projectID, virtualGraphID)
	return PollWithVersion(ctx, cfg, path, AuraApiVersion2, func(status string) bool {
		return !strings.EqualFold(status, waitingStatus)
	})
}

func Poll(ctx context.Context, cfg *clicfg.Config, url string, cond func(status string) bool) (*PollResponse, error) {
	return PollWithVersion(ctx, cfg, url, AuraApiVersion1, cond)
}

func PollWithVersion(ctx context.Context, cfg *clicfg.Config, url string, version AuraApiVersion, cond func(status string) bool) (*PollResponse, error) {
	debug := cfg.Aura.Debug()
	pollingConfig := cfg.Aura.PollingConfig()
	for i := 0; i < pollingConfig.MaxRetries; i++ {
		if err := sleepCtx(ctx, time.Second*time.Duration(pollingConfig.Interval)); err != nil {
			return nil, err
		}
		resBody, statusCode, err := MakeRequest(ctx, cfg, url, &RequestConfig{
			Method:  http.MethodGet,
			Version: version,
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, clierr.NewUpstreamError("error polling: %w", err)
		}

		if statusCode == http.StatusOK {
			var response PollResponse
			if err := json.Unmarshal(resBody, &response); err != nil {
				return nil, clierr.NewUpstreamError("cannot retrieve response polling: %w", err)
			}
			response.normalize()

			if debug {
				debugInfo("poll attempt %d/%d path %s status %d observed %q interval %ds", i+1, pollingConfig.MaxRetries, url, statusCode, response.Data.Status, pollingConfig.Interval)
			}

			// Successful poll, return last response
			if cond(response.Data.Status) {
				return &response, nil
			}
		} else if debug {
			debugInfo("poll attempt %d/%d path %s status %d interval %ds", i+1, pollingConfig.MaxRetries, url, statusCode, pollingConfig.Interval)
		}
	}

	return nil, clierr.NewUpstreamError("hit max retries [%d] polling", pollingConfig.MaxRetries)
}

// sleepCtx waits for d, returning ctx.Err() as soon as ctx is done. A
// non-positive d returns immediately (after checking ctx).
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
