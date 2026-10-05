// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package auraclient is the client for Aura resources: the one place the rest
// of the CLI talks to Aura through.
//
//	commands/aura  ──►  auraclient  ──►  auraclient/transport  ──►  Aura API
//	(cobra: flags,      (this package:   (HTTP, auth, retries,       (or, later,
//	 output)            Client and       --debug tracing)            the Aura SDK)
//	                    Instances(),
//	                    Agents(), ...)
//
// Commands call auraclient.New(cfg).<Resource>().<Operation>(ctx, scope, ...) and
// render what comes back. They never build request paths, call the HTTP
// transport, or import auraclient/transport (an architecture test enforces
// that), so the transport can be replaced — by the Aura SDK — in one place
// without touching any command.
//
// Rules for implementations (including a future SDK adapter):
//   - Return this package's types and *clierr.CLIError values; never leak the
//     transport's or the SDK's types or errors.
//   - Preserve the transport's cross-cutting behaviour: status to exit-code
//     mapping, token handling, --debug wire tracing and secret redaction.
//   - Populate each resource's Record with the full, normalised server record;
//     the CLI's JSON output is that record, not a re-modelled subset.
//   - Validate every ID before building a path (ValidateResourceID), and
//     register any secret the API mints (clievents.RegisterSecretValue) as soon
//     as it is received.
package auraclient

import (
	"context"

	"github.com/neo4j/cli/internal/auraclient/transport"
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
	Agents() AgentService
	GraphQL() GraphQLService
	VirtualGraphs() VirtualGraphService
	Sessions() SessionService
	Organizations() OrganizationService
	Projects() ProjectService
	CustomerManagedKeys() CustomerManagedKeyService
	Snapshots() SnapshotService
}

// New returns a Client backed by the Aura HTTP API using cfg's credentials and
// endpoints. This is the single place that chooses the implementation.
func New(cfg *clicfg.Config) Client {
	return &httpClient{cfg: cfg}
}

// MaxListPages bounds how many pages a paginated list walks before it reports
// that the result may be incomplete.
const MaxListPages = transport.MaxListPages

// Instance statuses callers may wait on. They mirror the API's values.
const (
	InstanceStatusCreating    = transport.InstanceStatusCreating
	InstanceStatusResuming    = transport.InstanceStatusResuming
	InstanceStatusOverwriting = transport.InstanceStatusOverwriting
)

// InstanceService operates on Aura instances. Callers never see which API
// version an operation uses: some instance endpoints are still v1 (and need a
// project-ownership preflight), others are v2beta1-scoped, and that is the
// implementation's concern.
//
// Delete, Pause and Resume return the instance record the API sent back; the
// instance is usually still transitioning, so use WaitWhile to block until the
// operation has finished.
//
// ctx is honoured end-to-end: requests, token minting, paging and polling all
// cancel with it.
type InstanceService interface {
	Get(ctx context.Context, scope Scope, id string) (*Instance, error)
	List(ctx context.Context, scope Scope) ([]Instance, error)
	Delete(ctx context.Context, scope Scope, id string) (*Instance, error)
	Pause(ctx context.Context, scope Scope, id string) (*Instance, error)
	Resume(ctx context.Context, scope Scope, id string) (*Instance, error)

	// Create provisions an instance. The returned Record carries the generated
	// username and one-time password (registered for redaction as soon as they
	// are received); the instance is still creating, so use WaitWhile with
	// InstanceStatusCreating to block until it is ready.
	Create(ctx context.Context, scope Scope, spec InstanceCreate) (*Instance, error)
	// Update changes an instance's name and/or memory.
	Update(ctx context.Context, scope Scope, id string, patch InstancePatch) (*Instance, error)
	// Overwrite replaces an instance's data with that of a source instance or
	// snapshot.
	Overwrite(ctx context.Context, scope Scope, id string, src OverwriteSource) (*Instance, error)

	// Verify returns a not-found error unless the instance belongs to the
	// scope's project. Use it before operating on an instance through an
	// endpoint that is not itself project-scoped.
	Verify(ctx context.Context, scope Scope, id string) error

	// WaitWhile polls until the instance's status is no longer status and
	// returns the status it settled on.
	WaitWhile(ctx context.Context, scope Scope, id, status string) (string, error)
}

type httpClient struct {
	cfg *clicfg.Config
}

func (c *httpClient) Instances() InstanceService {
	return instanceService{cfg: c.cfg}
}

func (c *httpClient) Agents() AgentService {
	return agentService{cfg: c.cfg}
}

func (c *httpClient) GraphQL() GraphQLService {
	return graphqlService{cfg: c.cfg}
}

func (c *httpClient) VirtualGraphs() VirtualGraphService {
	return virtualGraphService{cfg: c.cfg}
}

func (c *httpClient) Sessions() SessionService {
	return sessionService{cfg: c.cfg}
}

func (c *httpClient) Organizations() OrganizationService {
	return organizationService{cfg: c.cfg}
}

func (c *httpClient) Projects() ProjectService {
	return projectService{cfg: c.cfg}
}

func (c *httpClient) CustomerManagedKeys() CustomerManagedKeyService {
	return cmkService{cfg: c.cfg}
}

func (c *httpClient) Snapshots() SnapshotService {
	return snapshotService{cfg: c.cfg}
}

var (
	_ Client              = (*httpClient)(nil)
	_ InstanceService     = instanceService{}
	_ AgentService        = agentService{}
	_ GraphQLService      = graphqlService{}
	_ AuthProviderService = authProviderService{}
)
