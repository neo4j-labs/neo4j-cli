// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	commonflags "github.com/neo4j/cli/common/flags"
	auraflags "github.com/neo4j/cli/neo4j-cli/aura/internal/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// loadHarness drives NewLoadCmd directly against a local mock Aura server,
// mirroring deployHarness.
type loadHarness struct {
	t      *testing.T
	mux    *http.ServeMux
	server *httptest.Server
	out    *bytes.Buffer
	err    *bytes.Buffer
}

func newLoadHarness(t *testing.T) *loadHarness {
	t.Helper()
	cobra.EnableTraverseRunHooks = true

	h := &loadHarness{
		t:   t,
		mux: http.NewServeMux(),
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
	}
	h.mux.HandleFunc("/oauth/token", func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"access_token":"<token>","expires_in":3600,"token_type":"bearer"}`))
	})
	h.mux.HandleFunc("/v2beta1/organizations/"+deployOrgID+"/projects", func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"data": [{"id": "` + deployProjectID + `", "name": "Test Project"}]}`))
	})
	h.server = httptest.NewServer(h.mux)
	t.Cleanup(h.server.Close)
	return h
}

func (h *loadHarness) handle(pattern string, status int, body string) {
	h.mux.HandleFunc(pattern, func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(status)
		_, _ = res.Write([]byte(body))
	})
}

func (h *loadHarness) run(command string) error {
	h.t.Helper()

	cfgJSON := fmt.Sprintf(`{
		"format": "json",
		"aura": {
			"auth-url": "%s/oauth/token",
			"base-url": "%s/v1"
		}
	}`, h.server.URL, h.server.URL)
	credsJSON := `{
		"aura": {
			"credentials": [{"name": "test-cred", "access-token": "dsa", "token-expiry": 123}],
			"default-credential": "test-cred"
		}
	}`

	fs, err := testfs.GetTestFs(cfgJSON, credsJSON)
	require.NoError(h.t, err)

	cfg := clicfg.NewConfig(fs, "test", clicfg.AuraScope)
	cfg.Aura.SetPollingConfig(5, 0)

	cmd := NewLoadCmd(cfg)
	auraflags.RegisterOrgProjectFlags(cmd)
	commonflags.RegisterOutputFlag(cmd, cfg)
	commonflags.RegisterRwFlag(cmd)
	cmd.PersistentFlags().String("auth-url", "", "")
	cmd.PersistentFlags().String("base-url", "", "")
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		cfg.Aura.BindBaseUrl(c.Flags().Lookup("base-url"))
		cfg.Aura.BindAuthUrl(c.Flags().Lookup("auth-url"))
		return commonflags.ComposeRootPersistentPreRunE(cfg)(c, nil)
	}

	args, err := shlex.Split(command)
	require.NoError(h.t, err)
	cmd.SetArgs(args)
	cmd.SetOut(h.out)
	cmd.SetErr(h.err)
	cmd.SetIn(strings.NewReader(""))

	return cmd.Execute()
}

func registerLoadRunningInstanceMock(h *loadHarness) {
	h.handle("GET "+deployInstancesPath+"/db1d1234", http.StatusOK, `{"data": {"id": "db1d1234", "status": "running"}}`)
}

// withStubbedLoadDeps swaps the dataset/docker seams the load leaf touches with
// deterministic fakes for the duration of the test.
func withStubbedLoadDeps(t *testing.T, spec dataset.Spec, resolveErr error, dockerErr error, stage func(ctx context.Context, cfg *clicfg.Config, load datasetStageLoad, target deployTarget, warnOut io.Writer) error) {
	t.Helper()
	prevResolve := resolveDatasetFn
	prevDownload := downloadDatasetFn
	prevAvail := dockerAvailableFn
	prevStage := stageViaDockerFn

	resolveDatasetFn = func(_ context.Context, _, _ string) (dataset.Spec, error) {
		return spec, resolveErr
	}
	downloadDatasetFn = func(_ context.Context, _ dataset.Spec, _ int64) (string, func(), error) {
		return t.TempDir() + "/movies.dump", func() {}, nil
	}
	dockerAvailableFn = func(_ context.Context) error { return dockerErr }
	stageViaDockerFn = stage

	t.Cleanup(func() {
		resolveDatasetFn = prevResolve
		downloadDatasetFn = prevDownload
		dockerAvailableFn = prevAvail
		stageViaDockerFn = prevStage
	})
}

func moviesLoadSpec() dataset.Spec {
	return dataset.Spec{Owner: "neo4j-graph-examples", Repo: "movies", Branch: "main", DumpPath: "data/movies.dump", Plugins: []string{"apoc"}}
}

func TestLoadGDSHardErrorsBeforeAnyWork(t *testing.T) {
	h := newLoadHarness(t)

	var createCalled bool
	h.mux.HandleFunc("POST "+deployInstancesPath, func(res http.ResponseWriter, _ *http.Request) {
		createCalled = true
		res.WriteHeader(http.StatusAccepted)
	})

	dockerProbed := false
	spec := dataset.Spec{Owner: "neo4j-graph-examples", Repo: "graph-data-science", Plugins: []string{"apoc", "graph-data-science"}}
	withStubbedLoadDeps(t, spec, nil, nil, func(context.Context, *clicfg.Config, datasetStageLoad, deployTarget, io.Writer) error {
		t.Fatal("staging must not run for a GDS dataset")
		return nil
	})
	dockerAvailableFn = func(context.Context) error { dockerProbed = true; return nil }

	err := h.run("neo4j-graph-examples/gds --name demo --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "graph-data-science")
	require.False(t, createCalled, "no instance must be created for a GDS dataset")
	require.False(t, dockerProbed, "GDS must be rejected before the docker probe")
}

func TestLoadDockerAbsentErrorsBeforeInstanceCreation(t *testing.T) {
	h := newLoadHarness(t)

	var createCalled bool
	h.mux.HandleFunc("POST "+deployInstancesPath, func(res http.ResponseWriter, _ *http.Request) {
		createCalled = true
		res.WriteHeader(http.StatusAccepted)
	})

	withStubbedLoadDeps(t, moviesLoadSpec(), nil, errors.New("docker not found in PATH"), func(context.Context, *clicfg.Config, datasetStageLoad, deployTarget, io.Writer) error {
		t.Fatal("staging must not run when docker is unavailable")
		return nil
	})

	err := h.run("neo4j-graph-examples/movies --name demo --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Docker is not available")
	require.False(t, createCalled, "no instance must be created when docker is unavailable")
}

func TestLoadResolveErrorBeforeInstanceCreation(t *testing.T) {
	h := newLoadHarness(t)

	var createCalled bool
	h.mux.HandleFunc("POST "+deployInstancesPath, func(res http.ResponseWriter, _ *http.Request) {
		createCalled = true
		res.WriteHeader(http.StatusAccepted)
	})

	withStubbedLoadDeps(t, dataset.Spec{}, errors.New("manifest not found"), nil, func(context.Context, *clicfg.Config, datasetStageLoad, deployTarget, io.Writer) error {
		t.Fatal("staging must not run when resolve fails")
		return nil
	})

	err := h.run("neo4j-graph-examples/nope --name demo --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolve dataset")
	require.False(t, createCalled)
}

func TestLoadSuccessCreatesStagesAndPushes(t *testing.T) {
	h := newLoadHarness(t)
	h.handle("POST "+deployInstancesPath, http.StatusAccepted, deployCreateResponse)
	registerLoadRunningInstanceMock(h)

	var gotLoad datasetStageLoad
	var gotTarget deployTarget
	withStubbedLoadDeps(t, moviesLoadSpec(), nil, nil, func(_ context.Context, _ *clicfg.Config, load datasetStageLoad, target deployTarget, _ io.Writer) error {
		gotLoad, gotTarget = load, target
		return nil
	})

	err := h.run("neo4j-graph-examples/movies --name Instance01 --database neo4j --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.NoError(t, err)

	require.Equal(t, "neo4j", gotLoad.Database)
	require.Equal(t, []string{"apoc"}, gotLoad.Plugins)
	require.NotEmpty(t, gotLoad.DumpPath)
	require.Equal(t, "neo4j+s://db1d1234.databases.neo4j.io", gotTarget.URI)
	require.Equal(t, "letMeIn123!", gotTarget.Password)

	require.Contains(t, h.out.String(), `"load_status": "succeeded"`)
	require.Contains(t, h.out.String(), `"id": "db1d1234"`)
}

func TestLoadCreatesThenStages(t *testing.T) {
	h := newLoadHarness(t)

	var order []string
	h.mux.HandleFunc("POST "+deployInstancesPath, func(res http.ResponseWriter, _ *http.Request) {
		order = append(order, "create")
		res.WriteHeader(http.StatusAccepted)
		_, _ = res.Write([]byte(deployCreateResponse))
	})
	h.mux.HandleFunc("GET "+deployInstancesPath+"/db1d1234", func(res http.ResponseWriter, _ *http.Request) {
		order = append(order, "poll")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"data": {"id": "db1d1234", "status": "running"}}`))
	})

	withStubbedLoadDeps(t, moviesLoadSpec(), nil, nil, func(context.Context, *clicfg.Config, datasetStageLoad, deployTarget, io.Writer) error {
		order = append(order, "stage")
		return nil
	})

	require.NoError(t, h.run("neo4j-graph-examples/movies --name Instance01 --type free-db --rw --organization-id "+deployOrgID+" --project-id "+deployProjectID))
	require.Equal(t, []string{"create", "poll", "stage"}, order)
}

func TestLoadStageFailureLeavesInstance(t *testing.T) {
	h := newLoadHarness(t)
	h.handle("POST "+deployInstancesPath, http.StatusAccepted, deployCreateResponse)
	registerLoadRunningInstanceMock(h)

	withStubbedLoadDeps(t, moviesLoadSpec(), nil, nil, func(context.Context, *clicfg.Config, datasetStageLoad, deployTarget, io.Writer) error {
		return errors.New("neo4j-admin upload: boom")
	})

	err := h.run("neo4j-graph-examples/movies --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "neo4j-admin upload: boom")
	require.Contains(t, h.out.String(), `"load_status": "failed"`)
	require.Contains(t, h.err.String(), "left in place")
}

func TestLoadRejectsSystemDatabase(t *testing.T) {
	h := newLoadHarness(t)
	err := h.run("neo4j-graph-examples/movies --name demo --database system --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "system database")
}
