// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package aura is the service layer for Aura resources. Commands depend on the
// interfaces declared here, never on the wire protocol or on whatever
// implements it: today the implementation is the hand-rolled HTTP transport in
// internal/aura/api; it is intended to be replaced by the Aura SDK without
// touching any command.
//
// Rules for implementations (including a future SDK adapter):
//   - Return this package's types and *clierr.CLIError values; never leak the
//     transport's or the SDK's types or errors.
//   - Preserve the transport's cross-cutting behaviour: status to exit-code
//     mapping, token handling, --debug wire tracing and secret redaction.
//   - Populate Instance.Record with the full, normalised server record; the
//     CLI's JSON output is that record, not a re-modelled subset.
package aura

import (
	"context"

	"github.com/neo4j/cli/internal/clicfg"
)

// Scope identifies the organization and project a request is made in.
type Scope struct {
	OrgID     string
	ProjectID string
}

// Client is the entry point to Aura resources.
type Client interface {
	Instances() InstanceService
}

// New returns a Client backed by the Aura HTTP API using cfg's credentials and
// endpoints. This is the single place that chooses the implementation.
func New(cfg *clicfg.Config) Client {
	return &httpClient{cfg: cfg}
}

// InstanceService reads Aura instances.
//
// ctx is accepted so callers and the future SDK adapter can cancel requests;
// the current HTTP transport does not yet honour it.
type InstanceService interface {
	Get(ctx context.Context, scope Scope, id string) (*Instance, error)
	List(ctx context.Context, scope Scope) ([]Instance, error)
}

type httpClient struct {
	cfg *clicfg.Config
}

func (c *httpClient) Instances() InstanceService {
	return instanceService{cfg: c.cfg}
}

var (
	_ Client          = (*httpClient)(nil)
	_ InstanceService = instanceService{}
)
