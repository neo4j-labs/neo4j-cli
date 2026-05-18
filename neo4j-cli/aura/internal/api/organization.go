// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neo4j/cli/common/clicfg"
)

// Organization represents an entry from GET /organizations.
type Organization struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// ListOrganizationsResponse is the response body from GET /organizations.
type ListOrganizationsResponse struct {
	Data []Organization `json:"data"`
}

// GetOrganizationResponse is the response body from GET /organizations/{organizationId}.
type GetOrganizationResponse struct {
	Data Organization `json:"data"`
}

// Project represents an entry from GET /organizations/{organizationId}/projects.
type Project struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// ListProjectsResponse is the response body from GET /organizations/{organizationId}/projects.
type ListProjectsResponse struct {
	Data []Project `json:"data"`
}

// GetProjectResponse is the response body from GET /organizations/{organizationId}/projects/{projectId}.
type GetProjectResponse struct {
	Data Project `json:"data"`
}

// ListOrganizations calls GET /organizations on the v2beta1 API.
func ListOrganizations(cfg *clicfg.Config) (*ListOrganizationsResponse, error) {
	resBody, _, err := MakeRequest(cfg, "/organizations", &RequestConfig{
		Method:  http.MethodGet,
		Version: AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}

	var response ListOrganizationsResponse
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse list organizations response: %w", err)
	}

	return &response, nil
}

// GetOrganization calls GET /organizations/{organizationId} on the v2beta1 API.
func GetOrganization(cfg *clicfg.Config, organizationId string) (*GetOrganizationResponse, error) {
	resBody, _, err := MakeRequest(cfg, fmt.Sprintf("/organizations/%s", organizationId), &RequestConfig{
		Method:  http.MethodGet,
		Version: AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}

	var response GetOrganizationResponse
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse get organization response: %w", err)
	}

	return &response, nil
}

// ListProjects calls GET /organizations/{organizationId}/projects on the v2beta1 API.
func ListProjects(cfg *clicfg.Config, organizationId string) (*ListProjectsResponse, error) {
	resBody, _, err := MakeRequest(cfg, fmt.Sprintf("/organizations/%s/projects", organizationId), &RequestConfig{
		Method:  http.MethodGet,
		Version: AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}

	var response ListProjectsResponse
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse list projects response: %w", err)
	}

	return &response, nil
}

// GetProject calls GET /organizations/{organizationId}/projects/{projectId} on the v2beta1 API.
func GetProject(cfg *clicfg.Config, organizationId string, projectId string) (*GetProjectResponse, error) {
	resBody, _, err := MakeRequest(cfg, fmt.Sprintf("/organizations/%s/projects/%s", organizationId, projectId), &RequestConfig{
		Method:  http.MethodGet,
		Version: AuraApiVersion2,
	})
	if err != nil {
		return nil, err
	}

	var response GetProjectResponse
	if err := json.Unmarshal(resBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse get project response: %w", err)
	}

	return &response, nil
}
