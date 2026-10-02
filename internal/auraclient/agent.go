// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
)

// Agent is an Aura Agent. Record is the full server record, which is what
// commands render.
type Agent struct {
	ID          string
	Name        string
	Description string

	Record map[string]any
}

// AgentSpec is the complete definition used to create or replace an agent.
type AgentSpec struct {
	Name         string
	Description  string
	DBID         string
	IsPrivate    bool
	Tools        []any
	SystemPrompt string
	IsMCPEnabled bool
	Enabled      bool
}

// AgentPatch is a partial update: only non-nil fields are sent. Tools is sent
// when non-nil (an empty, non-nil slice clears the tools).
type AgentPatch struct {
	Name         *string
	Description  *string
	DBID         *string
	SystemPrompt *string
	Tools        []any
	IsPrivate    *bool
	IsMCPEnabled *bool
	Enabled      *bool
}

// AgentInvocation is the result of invoking an agent.
type AgentInvocation struct {
	// InvocationID is the server's X-Agent-Invocation-Id, empty if not sent.
	InvocationID string
	ID           string
	Type         string
	Role         string
	Status       string
	EndReason    string
	Content      []AgentContent
	Usage        AgentUsage

	Record map[string]any
}

// AgentContent is one block of an invocation's response.
type AgentContent struct {
	Type string
	Text string
}

// AgentUsage reports token usage for an invocation.
type AgentUsage struct {
	RequestTokens  int
	ResponseTokens int
	TotalTokens    int
}

// AgentService operates on Aura Agents.
type AgentService interface {
	List(ctx context.Context, scope Scope) ([]Agent, error)
	Get(ctx context.Context, scope Scope, id string) (*Agent, error)
	Create(ctx context.Context, scope Scope, spec AgentSpec) (*Agent, error)
	Replace(ctx context.Context, scope Scope, id string, spec AgentSpec) (*Agent, error)
	Update(ctx context.Context, scope Scope, id string, patch AgentPatch) (*Agent, error)
	Delete(ctx context.Context, scope Scope, id string) error

	// Invoke sends input to the agent. An application-level failure reported
	// inside a 2xx response is returned as an error, as are HTTP errors; both
	// carry the invocation id when the server provided one.
	Invoke(ctx context.Context, scope Scope, id, input string) (*AgentInvocation, error)
}

type agentService struct {
	cfg *clicfg.Config
}

func newAgent(rec map[string]any) Agent {
	return Agent{ID: str(rec, "id"), Name: str(rec, "name"), Description: str(rec, "description"), Record: rec}
}

func (s agentService) path(scope Scope, id string, suffix ...string) (string, error) {
	if err := ValidateResourceID("organization", scope.OrgID); err != nil {
		return "", err
	}
	if err := ValidateResourceID("project", scope.ProjectID); err != nil {
		return "", err
	}
	p := fmt.Sprintf("/organizations/%s/projects/%s/agents", scope.OrgID, scope.ProjectID)
	if id != "" {
		if err := ValidateResourceID("agent", id); err != nil {
			return "", err
		}
		p += "/" + id
	}
	for _, sfx := range suffix {
		p += "/" + sfx
	}
	return p, nil
}

func (s agentService) do(ctx context.Context, method, path string, body map[string]any, doing string) ([]byte, error) {
	resBody, status, err := transport.MakeRequest(ctx, s.cfg, path, &transport.RequestConfig{
		Method:   method,
		PostBody: body,
		Version:  transport.AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}
	if !transport.IsSuccessful(status) {
		return nil, fmt.Errorf("unexpected status %d %s", status, doing)
	}
	return resBody, nil
}

func (s agentService) one(ctx context.Context, method, path string, body map[string]any, doing string) (*Agent, error) {
	resBody, err := s.do(ctx, method, path, body, doing)
	if err != nil {
		return nil, err
	}
	rec, err := decodeBareRecord(resBody)
	if err != nil {
		return nil, err
	}
	a := newAgent(rec)
	return &a, nil
}

func (s agentService) List(ctx context.Context, scope Scope) ([]Agent, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	resBody, err := s.do(ctx, http.MethodGet, p, nil, "listing agents")
	if err != nil {
		return nil, err
	}
	rows, err := decodeBareRecords(resBody)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, len(rows))
	for i, r := range rows {
		out[i] = newAgent(r)
	}
	return out, nil
}

func (s agentService) Get(ctx context.Context, scope Scope, id string) (*Agent, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodGet, p, nil, "fetching agent")
}

func (s agentService) Create(ctx context.Context, scope Scope, spec AgentSpec) (*Agent, error) {
	p, err := s.path(scope, "")
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPost, p, spec.body(), "creating agent")
}

func (s agentService) Replace(ctx context.Context, scope Scope, id string, spec AgentSpec) (*Agent, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPut, p, spec.body(), "replacing agent")
}

func (s agentService) Update(ctx context.Context, scope Scope, id string, patch AgentPatch) (*Agent, error) {
	p, err := s.path(scope, id)
	if err != nil {
		return nil, err
	}
	return s.one(ctx, http.MethodPatch, p, patch.body(), "updating agent")
}

func (s agentService) Delete(ctx context.Context, scope Scope, id string) error {
	p, err := s.path(scope, id)
	if err != nil {
		return err
	}
	_, err = s.do(ctx, http.MethodDelete, p, nil, "deleting agent")
	return err
}

func (s agentService) Invoke(ctx context.Context, scope Scope, id, input string) (*AgentInvocation, error) {
	p, err := s.path(scope, id, "invoke")
	if err != nil {
		return nil, err
	}

	var respHeader http.Header
	resBody, status, err := transport.MakeRequest(ctx, s.cfg, p, &transport.RequestConfig{
		Method:         http.MethodPost,
		PostBody:       map[string]any{"input": input},
		Version:        transport.AuraApiVersion2,
		ResponseHeader: &respHeader,
	})
	invocationID := respHeader.Get("X-Agent-Invocation-Id")
	if err != nil {
		if status == http.StatusForbidden {
			return nil, withInvocationID(fmt.Errorf("agent invocation forbidden: agent may be disabled or private"), invocationID)
		}
		return nil, withInvocationID(err, invocationID)
	}
	if !transport.IsSuccessful(status) {
		return nil, withInvocationID(fmt.Errorf("unexpected status %d invoking agent", status), invocationID)
	}

	var wire struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Role      string `json:"role"`
		Status    string `json:"status"`
		EndReason string `json:"end_reason"`
		Content   []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			RequestTokens  int `json:"request_tokens"`
			ResponseTokens int `json:"response_tokens"`
			TotalTokens    int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resBody, &wire); err != nil {
		return nil, fmt.Errorf("unexpected invoke response: %w", err)
	}
	if wire.Type == "error" {
		if wire.Error != nil && wire.Error.Message != "" {
			return nil, withInvocationID(fmt.Errorf("agent invocation failed: %s", wire.Error.Message), invocationID)
		}
		return nil, withInvocationID(fmt.Errorf("agent invocation failed"), invocationID)
	}

	rec, err := decodeBareRecord(resBody)
	if err != nil {
		return nil, err
	}
	inv := &AgentInvocation{
		InvocationID: invocationID,
		ID:           wire.ID,
		Type:         wire.Type,
		Role:         wire.Role,
		Status:       wire.Status,
		EndReason:    wire.EndReason,
		Usage:        AgentUsage(wire.Usage),
		Record:       rec,
	}
	for _, c := range wire.Content {
		inv.Content = append(inv.Content, AgentContent{Type: c.Type, Text: c.Text})
	}
	return inv, nil
}

func (spec AgentSpec) body() map[string]any {
	return map[string]any{
		"name":           spec.Name,
		"description":    spec.Description,
		"dbid":           spec.DBID,
		"is_private":     spec.IsPrivate,
		"tools":          spec.Tools,
		"system_prompt":  spec.SystemPrompt,
		"is_mcp_enabled": spec.IsMCPEnabled,
		"enabled":        spec.Enabled,
	}
}

func (p AgentPatch) body() map[string]any {
	body := map[string]any{}
	set := func(key string, v *string) {
		if v != nil {
			body[key] = *v
		}
	}
	setBool := func(key string, v *bool) {
		if v != nil {
			body[key] = *v
		}
	}
	set("name", p.Name)
	set("description", p.Description)
	set("dbid", p.DBID)
	set("system_prompt", p.SystemPrompt)
	if p.Tools != nil {
		body["tools"] = p.Tools
	}
	setBool("is_private", p.IsPrivate)
	setBool("is_mcp_enabled", p.IsMCPEnabled)
	setBool("enabled", p.Enabled)
	return body
}

// withInvocationID appends the server's invocation id to err's message. For a
// *clierr.CLIError the message is edited in place so envelope rendering keeps
// the id; other errors are wrapped.
func withInvocationID(err error, id string) error {
	if err == nil || id == "" {
		return err
	}
	var ce *clierr.CLIError
	if errors.As(err, &ce) {
		ce.Message = fmt.Sprintf("%s (invocation id: %s)", ce.Message, id)
		return err
	}
	return fmt.Errorf("%w (invocation id: %s)", err, id)
}

// decodeBareRecords reads an un-enveloped response: a JSON array of records, or
// a single record (returned as one row). Malformed JSON is an error.
func decodeBareRecords(body []byte) ([]map[string]any, error) {
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err == nil {
		return rows, nil
	}
	var row map[string]any
	if err := json.Unmarshal(body, &row); err != nil {
		return nil, clierr.NewFatalError("unexpected response from Aura API: %s", err.Error())
	}
	return []map[string]any{row}, nil
}

func decodeBareRecord(body []byte) (map[string]any, error) {
	rows, err := decodeBareRecords(body)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, clierr.NewFatalError("expected 1 array value: %v", len(rows))
	}
	return rows[0], nil
}
