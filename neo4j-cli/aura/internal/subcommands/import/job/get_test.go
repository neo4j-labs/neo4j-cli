// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package job_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/neo4j/cli/neo4j-cli/aura/internal/test/testutils"
)

func TestGetImportJobById(t *testing.T) {
	organizationId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	projectId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	jobId := "87d485b4-73fc-4a7f-bb03-720f4672947e"
	mockedResponseWithoutProgress := `
{
    "data": {
        "id": "87d485b4-73fc-4a7f-bb03-720f4672947e",
        "import_type": "cloud",
        "info": {
            "state": "Completed",
            "start_time": "2025-08-15T13:12:51Z",
            "completion_time": "2025-08-15T13:15:19Z",
            "exit_status": {
                "state": "Cancelled",
                "message": "Cancelled"
            },
            "cancellation_requested_time": "2025-08-15T13:15:19.655209Z",
            "submitted_time": "2025-08-15T13:12:50.499797Z",
            "last_update_time": "2025-08-18T12:25:53.469873Z",
            "percentage_complete": 95.23
        },
        "data_source": {
            "id": "177216b3-49b9-4e24-a414-a9ecbb448c53",
            "type": "postgresql",
            "name": "AWS_POSTGRES_FLIGHTS"
        },
        "aura_target": {
            "db_id": "07e49cf5",
            "project_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
        },
        "user_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
    }
}
`
	expectedResponseJsonWithoutProgress := `
{
	"data": {
		"aura_target": {
			"db_id": "07e49cf5",
			"project_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
		},
		"data_source": {
			"id": "177216b3-49b9-4e24-a414-a9ecbb448c53",
			"name": "AWS_POSTGRES_FLIGHTS",
			"type": "postgresql"
		},
		"id": "87d485b4-73fc-4a7f-bb03-720f4672947e",
		"import_type": "cloud",
		"info": {
			"cancellation_requested_time": "2025-08-15T13:15:19.655209Z",
			"completion_time": "2025-08-15T13:15:19Z",
			"exit_status": {
				"message": "Cancelled",
				"state": "Cancelled"
			},
			"last_update_time": "2025-08-18T12:25:53.469873Z",
			"percentage_complete": 95.23,
			"start_time": "2025-08-15T13:12:51Z",
			"state": "Completed",
			"submitted_time": "2025-08-15T13:12:50.499797Z"
		},
		"user_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	}
}
`
	mockedResponseWithProgress := `
{
	"data": {
		"id": "87d485b4-73fc-4a7f-bb03-720f4672947e",
		"import_type": "cloud",
		"info": {
			"state": "Completed",
			"start_time": "2025-08-15T13:12:51Z",
			"completion_time": "2025-08-15T13:15:19Z",
			"exit_status": {
				"state": "Cancelled",
				"message": "Cancelled"
			},
			"cancellation_requested_time": "2025-08-15T13:15:19.655209Z",
			"submitted_time": "2025-08-15T13:12:50.499797Z",
			"last_update_time": "2025-08-18T12:25:53.469873Z",
			"percentage_complete": 95.23,
			"progress": {
				"nodes": [
					{
						"id": "n:1",
						"labels": [
							"AircraftData"
						],
						"total_rows": 9,
						"processed_rows": 9,
						"created_nodes": 9,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:0",
						"labels": [
							"Aircraft"
						],
						"total_rows": 9,
						"processed_rows": 9,
						"created_nodes": 9,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:3",
						"labels": [
							"AirportData"
						],
						"total_rows": 104,
						"processed_rows": 104,
						"created_nodes": 104,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:2",
						"labels": [
							"Airport"
						],
						"total_rows": 104,
						"processed_rows": 104,
						"created_nodes": 104,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:5",
						"labels": [
							"Flight"
						],
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_nodes": 33121,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:4",
						"labels": [
							"Booking"
						],
						"total_rows": 262788,
						"processed_rows": 262788,
						"created_nodes": 262788,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:7",
						"labels": [
							"Ticket"
						],
						"total_rows": 366733,
						"processed_rows": 366733,
						"created_nodes": 366733,
						"created_constraints": 1,
						"created_indexes": 0
					},
					{
						"id": "n:6",
						"labels": [
							"FlightView"
						],
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_nodes": 33121,
						"created_constraints": 1,
						"created_indexes": 0
					}
				],
				"relationships": [
					{
						"id": "r:8",
						"type": "TICKET_BELONGS_TO_BOOKING",
						"total_rows": 366733,
						"processed_rows": 255000,
						"created_relationships": 255000,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:1",
						"type": "FLIGHT_DEPARTS_FROM_AIRPORT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:0",
						"type": "FLIGHT_USES_AIRCRAFT_DATA",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:3",
						"type": "FLIGHT_USES_AIRCRAFT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:2",
						"type": "FLIGHT_ARRIVES_AT_AIRPORT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:5",
						"type": "FLIGHT_VIEW_ARRIVES_AT_AIRPORT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:4",
						"type": "FLIGHT_VIEW_DEPARTS_FROM_AIRPORT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:7",
						"type": "FLIGHT_HAS_TICKET",
						"total_rows": 1045726,
						"processed_rows": 1045726,
						"created_relationships": 1045726,
						"created_constraints": 0,
						"created_indexes": 0
					},
					{
						"id": "r:6",
						"type": "FLIGHT_VIEW_USES_AIRCRAFT",
						"total_rows": 33121,
						"processed_rows": 33121,
						"created_relationships": 33121,
						"created_constraints": 0,
						"created_indexes": 0
					}
				]
			}
		},
		"data_source": {
			"id": "177216b3-49b9-4e24-a414-a9ecbb448c53",
			"type": "postgresql",
			"name": "AWS_POSTGRES_FLIGHTS"
		},
		"aura_target": {
			"db_id": "07e49cf5",
			"project_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
		},
		"user_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	}
}
`
	expectedResponseJsonWithProgress := `
{
	"data": {
		"aura_target": {
			"db_id": "07e49cf5",
			"project_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
		},
		"data_source": {
			"id": "177216b3-49b9-4e24-a414-a9ecbb448c53",
			"name": "AWS_POSTGRES_FLIGHTS",
			"type": "postgresql"
		},
		"id": "87d485b4-73fc-4a7f-bb03-720f4672947e",
		"import_type": "cloud",
		"info": {
			"cancellation_requested_time": "2025-08-15T13:15:19.655209Z",
			"completion_time": "2025-08-15T13:15:19Z",
			"exit_status": {
				"message": "Cancelled",
				"state": "Cancelled"
			},
			"last_update_time": "2025-08-18T12:25:53.469873Z",
			"percentage_complete": 95.23,
			"progress": {
				"nodes": [
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 9,
						"id": "n:1",
						"labels": [
							"AircraftData"
						],
						"processed_rows": 9,
						"total_rows": 9
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 9,
						"id": "n:0",
						"labels": [
							"Aircraft"
						],
						"processed_rows": 9,
						"total_rows": 9
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 104,
						"id": "n:3",
						"labels": [
							"AirportData"
						],
						"processed_rows": 104,
						"total_rows": 104
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 104,
						"id": "n:2",
						"labels": [
							"Airport"
						],
						"processed_rows": 104,
						"total_rows": 104
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 33121,
						"id": "n:5",
						"labels": [
							"Flight"
						],
						"processed_rows": 33121,
						"total_rows": 33121
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 262788,
						"id": "n:4",
						"labels": [
							"Booking"
						],
						"processed_rows": 262788,
						"total_rows": 262788
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 366733,
						"id": "n:7",
						"labels": [
							"Ticket"
						],
						"processed_rows": 366733,
						"total_rows": 366733
					},
					{
						"created_constraints": 1,
						"created_indexes": 0,
						"created_nodes": 33121,
						"id": "n:6",
						"labels": [
							"FlightView"
						],
						"processed_rows": 33121,
						"total_rows": 33121
					}
				],
				"relationships": [
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 255000,
						"id": "r:8",
						"processed_rows": 255000,
						"total_rows": 366733,
						"type": "TICKET_BELONGS_TO_BOOKING"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:1",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_DEPARTS_FROM_AIRPORT"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:0",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_USES_AIRCRAFT_DATA"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:3",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_USES_AIRCRAFT"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:2",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_ARRIVES_AT_AIRPORT"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:5",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_VIEW_ARRIVES_AT_AIRPORT"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:4",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_VIEW_DEPARTS_FROM_AIRPORT"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 1045726,
						"id": "r:7",
						"processed_rows": 1045726,
						"total_rows": 1045726,
						"type": "FLIGHT_HAS_TICKET"
					},
					{
						"created_constraints": 0,
						"created_indexes": 0,
						"created_relationships": 33121,
						"id": "r:6",
						"processed_rows": 33121,
						"total_rows": 33121,
						"type": "FLIGHT_VIEW_USES_AIRCRAFT"
					}
				]
			},
			"start_time": "2025-08-15T13:12:51Z",
			"state": "Completed",
			"submitted_time": "2025-08-15T13:12:50.499797Z"
		},
		"user_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	}
}
`
	tests := map[string]struct {
		mockResponse          string
		executeCommand        string
		expectedQueryParamKey string
		expectedQueryParamVal string
		expectedResponse      string
	}{
		"query with default output format": {
			mockResponse:          mockedResponseWithoutProgress,
			executeCommand:        fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s", organizationId, projectId, jobId),
			expectedQueryParamKey: "progress",
			expectedQueryParamVal: "false",
			expectedResponse:      expectedResponseJsonWithoutProgress,
		}, "query with table output format": {
			mockResponse:          mockedResponseWithoutProgress,
			executeCommand:        fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s --format=table", organizationId, projectId, jobId),
			expectedQueryParamKey: "progress",
			expectedQueryParamVal: "false",
			expectedResponse: `
┌──────────────────────────────────────┬─────────────┬────────────┬────────────────────────┬──────────────────────────┬──────────────────────┬───────────────────┐
│ ID                                   │ IMPORT_TYPE │ INFO:STATE │ INFO:EXIT_STATUS:STATE │ INFO:PERCENTAGE_COMPLETE │ DATA_SOURCE:NAME     │ AURA_TARGET:DB_ID │
├──────────────────────────────────────┼─────────────┼────────────┼────────────────────────┼──────────────────────────┼──────────────────────┼───────────────────┤
│ 87d485b4-73fc-4a7f-bb03-720f4672947e │ cloud       │ Completed  │ Cancelled              │ 95.23                    │ AWS_POSTGRES_FLIGHTS │ 07e49cf5          │
└──────────────────────────────────────┴─────────────┴────────────┴────────────────────────┴──────────────────────────┴──────────────────────┴───────────────────┘
┌──────────────────────────┐
│ INFO:EXIT_STATUS:MESSAGE │
├──────────────────────────┤
│ Cancelled                │
└──────────────────────────┘
`,
		}, "query includes progress with default output format": {
			mockResponse:          mockedResponseWithProgress,
			executeCommand:        fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s --progress", organizationId, projectId, jobId),
			expectedQueryParamKey: "progress",
			expectedQueryParamVal: "true",
			expectedResponse:      expectedResponseJsonWithProgress,
		}, "query includes progress with table output format": {
			mockResponse:          mockedResponseWithProgress,
			executeCommand:        fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s --progress --format=table", organizationId, projectId, jobId),
			expectedQueryParamKey: "progress",
			expectedQueryParamVal: "true",
			expectedResponse: `
┌──────────────────────────────────────┬─────────────┬────────────┬────────────────────────┬──────────────────────────┬──────────────────────┬───────────────────┐
│ ID                                   │ IMPORT_TYPE │ INFO:STATE │ INFO:EXIT_STATUS:STATE │ INFO:PERCENTAGE_COMPLETE │ DATA_SOURCE:NAME     │ AURA_TARGET:DB_ID │
├──────────────────────────────────────┼─────────────┼────────────┼────────────────────────┼──────────────────────────┼──────────────────────┼───────────────────┤
│ 87d485b4-73fc-4a7f-bb03-720f4672947e │ cloud       │ Completed  │ Cancelled              │ 95.23                    │ AWS_POSTGRES_FLIGHTS │ 07e49cf5          │
└──────────────────────────────────────┴─────────────┴────────────┴────────────────────────┴──────────────────────────┴──────────────────────┴───────────────────┘
┌──────────────────────────┐
│ INFO:EXIT_STATUS:MESSAGE │
├──────────────────────────┤
│ Cancelled                │
└──────────────────────────┘
# Progress details:
# Nodes progress:
┌─────┬──────────────────┬────────────────┬────────────┬───────────────┬─────────────────────┬─────────────────┐
│ ID  │ LABELS           │ PROCESSED_ROWS │ TOTAL_ROWS │ CREATED_NODES │ CREATED_CONSTRAINTS │ CREATED_INDEXES │
├─────┼──────────────────┼────────────────┼────────────┼───────────────┼─────────────────────┼─────────────────┤
│ n:1 │ [                │ 9              │ 9          │ 9             │ 1                   │ 0               │
│     │   "AircraftData" │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:0 │ [                │ 9              │ 9          │ 9             │ 1                   │ 0               │
│     │   "Aircraft"     │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:3 │ [                │ 104            │ 104        │ 104           │ 1                   │ 0               │
│     │   "AirportData"  │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:2 │ [                │ 104            │ 104        │ 104           │ 1                   │ 0               │
│     │   "Airport"      │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:5 │ [                │ 33121          │ 33121      │ 33121         │ 1                   │ 0               │
│     │   "Flight"       │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:4 │ [                │ 262788         │ 262788     │ 262788        │ 1                   │ 0               │
│     │   "Booking"      │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:7 │ [                │ 366733         │ 366733     │ 366733        │ 1                   │ 0               │
│     │   "Ticket"       │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
│ n:6 │ [                │ 33121          │ 33121      │ 33121         │ 1                   │ 0               │
│     │   "FlightView"   │                │            │               │                     │                 │
│     │ ]                │                │            │               │                     │                 │
└─────┴──────────────────┴────────────────┴────────────┴───────────────┴─────────────────────┴─────────────────┘
# Relationships progress:
┌─────┬──────────────────────────────────┬────────────────┬──────────────┬───────────────────────┬─────────────────────┬─────────────────┐
│ ID  │ TYPE                             │ PROCESSED_ROWS │ TOTAL_ROWS   │ CREATED_RELATIONSHIPS │ CREATED_CONSTRAINTS │ CREATED_INDEXES │
├─────┼──────────────────────────────────┼────────────────┼──────────────┼───────────────────────┼─────────────────────┼─────────────────┤
│ r:8 │ TICKET_BELONGS_TO_BOOKING        │ 255000         │ 366733       │ 255000                │ 0                   │ 0               │
│ r:1 │ FLIGHT_DEPARTS_FROM_AIRPORT      │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:0 │ FLIGHT_USES_AIRCRAFT_DATA        │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:3 │ FLIGHT_USES_AIRCRAFT             │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:2 │ FLIGHT_ARRIVES_AT_AIRPORT        │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:5 │ FLIGHT_VIEW_ARRIVES_AT_AIRPORT   │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:4 │ FLIGHT_VIEW_DEPARTS_FROM_AIRPORT │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
│ r:7 │ FLIGHT_HAS_TICKET                │ 1.045726e+06   │ 1.045726e+06 │ 1.045726e+06          │ 0                   │ 0               │
│ r:6 │ FLIGHT_VIEW_USES_AIRCRAFT        │ 33121          │ 33121        │ 33121                 │ 0                   │ 0               │
└─────┴──────────────────────────────────┴────────────────┴──────────────┴───────────────────────┴─────────────────────┴─────────────────┘
`,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/import/jobs/%s", organizationId, projectId, jobId), http.StatusOK, tt.mockResponse)

			helper.SetConfigValue("aura.beta-enabled", true)

			helper.ExecuteCommand(tt.executeCommand)

			mockHandler.AssertCalledTimes(1)
			mockHandler.AssertCalledWithMethod(http.MethodGet)
			mockHandler.AssertCalledWithQueryParam(tt.expectedQueryParamKey, tt.expectedQueryParamVal)
			helper.AssertOut(tt.expectedResponse)
		})
	}
}

func TestGetImportJobByIdWithOrganizationAndProjectIdFromConfig(t *testing.T) {
	organizationId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	projectId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	jobId := "87d485b4-73fc-4a7f-bb03-720f4672947e"

	helper := testutils.NewAuraTestHelper(t)
	defer helper.Close()

	mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/import/jobs/%s", organizationId, projectId, jobId), http.StatusOK, `{
    "data": {
        "id": "87d485b4-73fc-4a7f-bb03-720f4672947e",
        "import_type": "cloud",
        "info": {
            "state": "Completed",
            "start_time": "2025-08-15T13:12:51Z",
            "completion_time": "2025-08-15T13:15:19Z",
            "exit_status": {
                "state": "Cancelled",
                "message": "Cancelled"
            },
            "cancellation_requested_time": "2025-08-15T13:15:19.655209Z",
            "submitted_time": "2025-08-15T13:12:50.499797Z",
            "last_update_time": "2025-08-18T12:25:53.469873Z",
            "percentage_complete": 95.23
        },
        "data_source": {
            "id": "177216b3-49b9-4e24-a414-a9ecbb448c53",
            "type": "postgresql",
            "name": "AWS_POSTGRES_FLIGHTS"
        },
        "aura_target": {
            "db_id": "07e49cf5",
            "project_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
        },
        "user_id": "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
    }}`)

	helper.SetConfigValue("aura.beta-enabled", true)
	helper.SetDefaultProjectInConfig(organizationId, projectId)
	helper.ExecuteCommand(fmt.Sprintf("import job get %s --format=table", jobId))

	mockHandler.AssertCalledTimes(1)
	mockHandler.AssertCalledWithMethod(http.MethodGet)
	mockHandler.AssertCalledWithQueryParam("progress", "false")
	helper.AssertOut(`
┌──────────────────────────────────────┬─────────────┬────────────┬────────────────────────┬──────────────────────────┬──────────────────────┬───────────────────┐
│ ID                                   │ IMPORT_TYPE │ INFO:STATE │ INFO:EXIT_STATUS:STATE │ INFO:PERCENTAGE_COMPLETE │ DATA_SOURCE:NAME     │ AURA_TARGET:DB_ID │
├──────────────────────────────────────┼─────────────┼────────────┼────────────────────────┼──────────────────────────┼──────────────────────┼───────────────────┤
│ 87d485b4-73fc-4a7f-bb03-720f4672947e │ cloud       │ Completed  │ Cancelled              │ 95.23                    │ AWS_POSTGRES_FLIGHTS │ 07e49cf5          │
└──────────────────────────────────────┴─────────────┴────────────┴────────────────────────┴──────────────────────────┴──────────────────────┴───────────────────┘
┌──────────────────────────┐
│ INFO:EXIT_STATUS:MESSAGE │
├──────────────────────────┤
│ Cancelled                │
└──────────────────────────┘
	`)
}

func TestGetImportJobError(t *testing.T) {
	organizationId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	projectId := "f607bebe-0cc0-4166-b60c-b4eed69ee7ee"
	jobId := "87d485b4-73fc-4a7f-bb03-720f4672947e"
	testCases := map[string]struct {
		executeCommand      string
		statusCode          int
		expectedCalledTimes int
		expectedError       string
		returnBody          string
	}{
		"correct command with error response 1": {
			executeCommand:      fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s", organizationId, projectId, jobId),
			statusCode:          http.StatusNotFound,
			expectedCalledTimes: 1,
			expectedError:       "Error: [The job 87d485b4-73fc-4a7f-bb03-720f4672947e does not exist]",
			returnBody: `{
				"errors": [
					{
					"message": "The job 87d485b4-73fc-4a7f-bb03-720f4672947e does not exist",
					"reason": "Requested"
					}
				]
			}`,
		},
		"correct command with error response 2": {
			executeCommand:      fmt.Sprintf("import job get --organization-id=%s --project-id=%s %s", organizationId, projectId, jobId),
			statusCode:          http.StatusMethodNotAllowed,
			expectedCalledTimes: 1,
			expectedError:       "Error: [string]",
			returnBody: `{
				"errors": [
					{
					"message": "string",
					"reason": "string",
					"field": "string"
					}
				]
			}`,
		},
		"incorrect command with missing organization id": {
			executeCommand:      fmt.Sprintf("import job get --project-id=%s %s", projectId, jobId),
			statusCode:          http.StatusBadRequest,
			expectedCalledTimes: 0,
			expectedError:       "Error: required flag(s) \"organization-id\" not set",
			returnBody:          ``,
		},
		"incorrect command with missing project id": {
			executeCommand:      fmt.Sprintf("import job get --organization-id=%s %s", organizationId, jobId),
			statusCode:          http.StatusBadRequest,
			expectedCalledTimes: 0,
			expectedError:       "Error: required flag(s) \"project-id\" not set",
			returnBody:          ``,
		},
		"incorrect command with missing job id": {
			executeCommand:      fmt.Sprintf("import job get --organization-id=%s --project-id=%s", organizationId, projectId),
			statusCode:          http.StatusBadRequest,
			expectedCalledTimes: 0,
			expectedError:       "Error: accepts 1 arg(s), received 0",
			returnBody:          ``,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			helper := testutils.NewAuraTestHelper(t)
			defer helper.Close()

			mockHandler := helper.NewRequestHandlerMock(fmt.Sprintf("/v2beta1/organizations/%s/projects/%s/import/jobs/%s", organizationId, projectId, jobId), testCase.statusCode, testCase.returnBody)

			helper.SetConfigValue("aura.beta-enabled", true)

			helper.ExecuteCommand(testCase.executeCommand)

			mockHandler.AssertCalledTimes(testCase.expectedCalledTimes)
			if testCase.expectedCalledTimes > 0 {
				mockHandler.AssertCalledWithMethod(http.MethodGet)
			}

			helper.AssertErr(testCase.expectedError)
		})
	}
}
