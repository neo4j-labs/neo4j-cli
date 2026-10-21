// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package auraclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neo4j/cli/internal/auraclient/transport"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/testutil/testfs"
)

const evaluationDatasetsPath = "/organizations/org-1/projects/proj-1/agents/evaluation-datasets"

var evaluationDatasetScope = Scope{OrgID: "org-1", ProjectID: "proj-1"}

// newEvaluationDatasetClient serves handler for the dataset collection and
// member paths (plus the token endpoint) and returns a Client wired to it, so
// the HTTP adapter is exercised end to end.
func newEvaluationDatasetClient(t *testing.T, handler http.HandlerFunc) Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token":"tok","expires_in":3600,"token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("/v2beta1"+evaluationDatasetsPath, handler)
	mux.HandleFunc("/v2beta1"+evaluationDatasetsPath+"/", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cfgJSON := fmt.Sprintf(`{"format":"json","aura":{"auth-url":"%s/oauth/token","base-url":"%s"}}`, srv.URL, srv.URL)
	fs, err := testfs.GetTestFs(cfgJSON, testCredJSON)
	require.NoError(t, err)
	return New(clicfg.NewConfig(fs, "test"))
}

func TestEvaluationDatasetPath(t *testing.T) {
	p, err := scopedResourcePath(evaluationDatasetScope, "agents/evaluation-datasets", "evaluation-dataset", "")
	require.NoError(t, err)
	assert.Equal(t, evaluationDatasetsPath, p)

	p, err = scopedResourcePath(evaluationDatasetScope, "agents/evaluation-datasets", "evaluation-dataset", "ds-1")
	require.NoError(t, err)
	assert.Equal(t, evaluationDatasetsPath+"/ds-1", p)
}

func TestEvaluationDatasetSpecBody_CarriesNameDescriptionAndQuestions(t *testing.T) {
	body := EvaluationDatasetSpec{
		Name:        "n",
		Description: "d",
		Questions: []EvaluationQuestion{
			{QueryNumber: 1, QueryText: "q", ExpectedAnswer: "a"},
		},
	}.body()

	assert.Equal(t, map[string]any{
		"dataset_name": "n",
		"description":  "d",
		"questions": []map[string]any{
			{"query_number": 1, "query_text": "q", "expected_answer": "a"},
		},
	}, body)
}

func TestEvaluationDatasetPatchBody_OnlySendsSetFields(t *testing.T) {
	assert.Empty(t, EvaluationDatasetPatch{}.body())

	name, empty := "renamed", ""
	assert.Equal(t, map[string]any{"dataset_name": "renamed", "description": ""},
		EvaluationDatasetPatch{Name: &name, Description: &empty}.body(),
		"a set description is sent even when empty")

	withQuestions := EvaluationDatasetPatch{Questions: []EvaluationQuestion{
		{QueryNumber: 1, QueryText: "q", ExpectedAnswer: "a"},
	}}.body()
	assert.Contains(t, withQuestions, "questions")
	assert.NotContains(t, EvaluationDatasetPatch{}.body(), "questions")
}

func TestEvaluationDatasetQuestionBody(t *testing.T) {
	q := EvaluationQuestion{
		QueryNumber:    1,
		QueryText:      "why?",
		ExpectedAnswer: "because",
		QuestionID:     "q-1",
		ExpectedToolCalls: []EvaluationToolCall{
			{ToolName: "t", ToolType: "function", ToolInput: map[string]any{"x": 1}, ToolOutput: "out"},
		},
	}

	create := q.body(false)
	assert.NotContains(t, create, "question_id", "create must not send question_id")
	calls, ok := create["expected_tool_calls"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, calls, 1)
	assert.Equal(t, map[string]any{
		"tool_name": "t", "tool_type": "function", "tool_input": map[string]any{"x": 1}, "tool_output": "out",
	}, calls[0])

	patch := q.body(true)
	assert.Equal(t, "q-1", patch["question_id"])

	// Optional tool-call fields are omitted, not sent as null.
	bare := EvaluationQuestion{QueryNumber: 1, QueryText: "q", ExpectedAnswer: "a"}.body(false)
	assert.NotContains(t, bare, "expected_tool_calls")
	call := EvaluationToolCall{ToolName: "t", ToolType: "function"}.body()
	assert.Equal(t, map[string]any{"tool_name": "t", "tool_type": "function"}, call)
}

func TestEvaluationDatasetsList_WalksCursorPages(t *testing.T) {
	var pageTokens []string
	c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, strconv.Itoa(transport.ListPageSize), r.URL.Query().Get("page_limit"))
		token := r.URL.Query().Get("page_token")
		pageTokens = append(pageTokens, token)
		w.WriteHeader(http.StatusOK)
		switch token {
		case "":
			w.Write([]byte(`{"data":[{"id":"ds-1","dataset_name":"one","description":"first","question_count":2}],` + //nolint:errcheck
				`"links":{"next":"` + evaluationDatasetsPath + `?page_limit=1000&page_token=tok-1"}}`))
		case "tok-1":
			w.Write([]byte(`{"data":[{"id":"ds-2","dataset_name":"two","description":"second","question_count":1}],"links":{"next":null}}`)) //nolint:errcheck
		default:
			t.Errorf("unexpected page_token %q", token)
		}
	})

	page, err := c.EvaluationDatasets().List(context.Background(), evaluationDatasetScope, 0)

	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	assert.Equal(t, []string{"", "tok-1"}, pageTokens, "the walk follows links.next to the last page")
	assert.Equal(t, "ds-1", page.Items[0].ID)
	assert.Equal(t, "one", page.Items[0].Name)
	assert.Equal(t, "first", page.Items[0].Description)
	assert.Equal(t, 2, page.Items[0].QuestionCount)
	assert.Equal(t, "ds-2", page.Items[1].ID)
	assert.Equal(t, "two", page.Items[1].Name)
	assert.False(t, page.LimitReached)
	assert.False(t, page.PageCapReached)
}

func TestEvaluationDatasetsList_TruncatesAtTheCallersLimit(t *testing.T) {
	t.Run("a limit inside a page stops the walk and reports it", func(t *testing.T) {
		requests := 0
		c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, _ *http.Request) {
			requests++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"id":"ds-1","dataset_name":"one","question_count":1},` + //nolint:errcheck
				`{"id":"ds-2","dataset_name":"two","question_count":1},` +
				`{"id":"ds-3","dataset_name":"three","question_count":1}],` +
				`"links":{"next":"` + evaluationDatasetsPath + `?page_token=more"}}`))
		})

		page, err := c.EvaluationDatasets().List(context.Background(), evaluationDatasetScope, 2)

		require.NoError(t, err)
		require.Len(t, page.Items, 2)
		assert.Equal(t, "ds-2", page.Items[1].ID)
		assert.True(t, page.LimitReached, "more items remain on the server")
		assert.False(t, page.PageCapReached)
		assert.Equal(t, 1, requests, "the walk stops as soon as the limit is reached")
	})

	t.Run("a limit reached exactly at the end is not truncation", func(t *testing.T) {
		c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"id":"ds-1","dataset_name":"one","question_count":1},` + //nolint:errcheck
				`{"id":"ds-2","dataset_name":"two","question_count":1}],"links":{"next":null}}`))
		})

		page, err := c.EvaluationDatasets().List(context.Background(), evaluationDatasetScope, 2)

		require.NoError(t, err)
		assert.Len(t, page.Items, 2)
		assert.False(t, page.LimitReached)
	})
}

func TestEvaluationDatasetsList_NegativeLimitIsAUsageError(t *testing.T) {
	// A nil config proves no request is attempted: the limit is rejected first.
	_, err := evaluationDatasetService{}.List(context.Background(), evaluationDatasetScope, -1)

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
	assert.Contains(t, ce.Message, "limit")
}

func TestEvaluationDatasetsGet_ReturnsDetailWithQuestions(t *testing.T) {
	detail := `{"data":{"id":"ds-1","dataset_name":"one","description":"d",` +
		`"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",` +
		`"questions":[{"question_id":"q-1","query_number":1,"query_text":"why?","expected_answer":"because",` +
		`"expected_tool_calls":[{"tool_name":"t","tool_type":"function"}]}]}}`
	c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2beta1"+evaluationDatasetsPath+"/ds-1", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detail)) //nolint:errcheck
	})

	ds, err := c.EvaluationDatasets().Get(context.Background(), evaluationDatasetScope, "ds-1")

	require.NoError(t, err)
	assert.Equal(t, "ds-1", ds.ID)
	assert.Equal(t, "one", ds.Name)
	assert.Equal(t, "d", ds.Description)
	assert.Equal(t, 1, ds.QuestionCount, "a detail record derives the count from its questions")
	questions, ok := ds.Record["questions"].([]any)
	require.True(t, ok, "the full server record keeps the questions array")
	assert.Len(t, questions, 1)
	assert.Equal(t, "2026-01-02T00:00:00Z", ds.Record["updated_at"], "unmodelled fields survive into the rendered record")
}

func TestEvaluationDatasetsCreate_SendsBodyAndReturnsRecord(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{"id":"ds-new"},"links":{"resource":"` + evaluationDatasetsPath + `/ds-new"}}`)) //nolint:errcheck
	})

	ds, err := c.EvaluationDatasets().Create(context.Background(), evaluationDatasetScope, EvaluationDatasetSpec{
		Name:        "one",
		Description: "desc",
		Questions: []EvaluationQuestion{
			{
				QueryNumber:    1,
				QueryText:      "why?",
				ExpectedAnswer: "because",
				ExpectedToolCalls: []EvaluationToolCall{
					{ToolName: "t", ToolType: "function"},
				},
			},
			{QueryNumber: 2, QueryText: "how?", ExpectedAnswer: "like this", QuestionID: "never-sent-on-create"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "ds-new", ds.ID)
	assert.Equal(t, "ds-new", ds.Record["id"])
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v2beta1"+evaluationDatasetsPath, gotPath)
	assert.Equal(t, "one", gotBody["dataset_name"])
	assert.Equal(t, "desc", gotBody["description"])
	questions, ok := gotBody["questions"].([]any)
	require.True(t, ok)
	require.Len(t, questions, 2)
	first, ok := questions[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), first["query_number"])
	assert.Equal(t, "why?", first["query_text"])
	assert.Equal(t, "because", first["expected_answer"])
	assert.NotContains(t, first, "question_id", "create must not send question_id")
	calls, ok := first["expected_tool_calls"].([]any)
	require.True(t, ok)
	require.Len(t, calls, 1)
	assert.Equal(t, "t", calls[0].(map[string]any)["tool_name"])
	second, ok := questions[1].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, second, "question_id", "create must not send question_id")
	assert.NotContains(t, second, "expected_tool_calls", "nil tool calls are omitted")
}

func TestEvaluationDatasetsUpdate_SendsOnlyThePatchedFields(t *testing.T) {
	t.Run("a name-only patch", func(t *testing.T) {
		var gotMethod, gotPath string
		var gotBody map[string]any
		c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decoding request body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"id":"ds-1","dataset_name":"renamed","description":"d","questions":[]}}`)) //nolint:errcheck
		})

		renamed := "renamed"
		ds, err := c.EvaluationDatasets().Update(context.Background(), evaluationDatasetScope, "ds-1", EvaluationDatasetPatch{Name: &renamed})

		require.NoError(t, err)
		assert.Equal(t, "renamed", ds.Name)
		assert.Equal(t, http.MethodPatch, gotMethod)
		assert.Equal(t, "/v2beta1"+evaluationDatasetsPath+"/ds-1", gotPath)
		assert.Equal(t, map[string]any{"dataset_name": "renamed"}, gotBody, "only the non-nil patch field is sent")
	})

	t.Run("a questions patch keeps question_id", func(t *testing.T) {
		var gotBody map[string]any
		c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decoding request body: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"id":"ds-1","dataset_name":"one","questions":[]}}`)) //nolint:errcheck
		})

		_, err := c.EvaluationDatasets().Update(context.Background(), evaluationDatasetScope, "ds-1", EvaluationDatasetPatch{
			Questions: []EvaluationQuestion{{QuestionID: "q-1", QueryNumber: 1, QueryText: "why?", ExpectedAnswer: "because"}},
		})

		require.NoError(t, err)
		questions, ok := gotBody["questions"].([]any)
		require.True(t, ok)
		require.Len(t, questions, 1)
		assert.Equal(t, "q-1", questions[0].(map[string]any)["question_id"])
	})
}

func TestEvaluationDatasetsUpdate_EmptyPatchIsAUsageError(t *testing.T) {
	// A nil config proves no request is attempted: the empty patch is rejected
	// first (the API requires minProperties 1).
	_, err := evaluationDatasetService{}.Update(context.Background(), evaluationDatasetScope, "ds-1", EvaluationDatasetPatch{})

	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code)
}

func TestEvaluationDatasetsDelete_UsesTheMemberPath(t *testing.T) {
	var gotMethod, gotPath string
	c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`)) //nolint:errcheck // the API returns a bare empty object, not a data envelope
	})

	err := c.EvaluationDatasets().Delete(context.Background(), evaluationDatasetScope, "ds-1")

	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/v2beta1"+evaluationDatasetsPath+"/ds-1", gotPath)

	t.Run("a non-200 is an error", func(t *testing.T) {
		c := newEvaluationDatasetClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"errors":[{"message":"boom"}]}`)) //nolint:errcheck
		})

		err := c.EvaluationDatasets().Delete(context.Background(), evaluationDatasetScope, "ds-1")

		require.Error(t, err)
	})
}

func TestEvaluationDatasetsValidateScopeAndIDBeforeAnyRequest(t *testing.T) {
	// A nil config proves no request is attempted: validation fails first.
	s := evaluationDatasetService{}
	good := evaluationDatasetScope
	ctx := context.Background()
	name := "x"

	cases := map[string]func() error{
		"list bad org":     func() error { _, err := s.List(ctx, Scope{OrgID: "a/b", ProjectID: "p"}, 0); return err },
		"list bad project": func() error { _, err := s.List(ctx, Scope{OrgID: "o", ProjectID: ".."}, 0); return err },
		"get bad id":       func() error { _, err := s.Get(ctx, good, "../x"); return err },
		"get bad org":      func() error { _, err := s.Get(ctx, Scope{ProjectID: "p"}, "id"); return err },
		"create bad project": func() error {
			_, err := s.Create(ctx, Scope{OrgID: "o", ProjectID: "a?b"}, EvaluationDatasetSpec{})
			return err
		},
		"update bad id":    func() error { _, err := s.Update(ctx, good, "a#b", EvaluationDatasetPatch{Name: &name}); return err },
		"delete bad id":    func() error { return s.Delete(ctx, good, "../../x") },
		"delete empty org": func() error { return s.Delete(ctx, Scope{ProjectID: "p"}, "id") },
	}
	for caseName, call := range cases {
		t.Run(caseName, func(t *testing.T) {
			err := call()
			var ce *clierr.CLIError
			require.True(t, errors.As(err, &ce), "want a clierr, got %v", err)
		})
	}
}
