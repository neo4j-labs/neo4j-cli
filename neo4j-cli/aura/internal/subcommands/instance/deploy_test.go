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
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	deployOrgID     = "test-org-id"
	deployProjectID = "YOUR_TENANT_ID"
)

// deployHarness drives NewDeployCmd directly (the leaf is not yet registered in
// the instance tree — task-006 does that) against a local mock Aura server.
type deployHarness struct {
	t      *testing.T
	mux    *http.ServeMux
	server *httptest.Server
	out    *bytes.Buffer
	err    *bytes.Buffer
}

func newDeployHarness(t *testing.T) *deployHarness {
	t.Helper()
	cobra.EnableTraverseRunHooks = true

	h := &deployHarness{
		t:   t,
		mux: http.NewServeMux(),
		out: &bytes.Buffer{},
		err: &bytes.Buffer{},
	}
	h.mux.HandleFunc("/oauth/token", func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"access_token":"<token>","expires_in":3600,"token_type":"bearer"}`))
	})
	// Project validation for ResolveAndValidateOrgProject.
	h.mux.HandleFunc("/v2beta1/organizations/"+deployOrgID+"/projects", func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"data": [{"id": "` + deployProjectID + `", "name": "Test Project"}]}`))
	})
	h.server = httptest.NewServer(h.mux)
	t.Cleanup(h.server.Close)
	return h
}

// handle registers a single JSON response for an exact method+path pattern.
func (h *deployHarness) handle(pattern string, status int, body string) {
	h.mux.HandleFunc(pattern, func(res http.ResponseWriter, _ *http.Request) {
		res.WriteHeader(status)
		_, _ = res.Write([]byte(body))
	})
}

func (h *deployHarness) run(command string) error {
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

	cmd := NewDeployCmd(cfg)
	// Mirror the persistent flags the aura tree provides to the leaf.
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

const deployCreateResponse = `{
	"data": {
		"id": "db1d1234",
		"connection_url": "neo4j+s://db1d1234.databases.neo4j.io",
		"username": "neo4j",
		"password": "letMeIn123!",
		"tenant_id": "YOUR_TENANT_ID",
		"cloud_provider": "gcp",
		"region": "europe-west1",
		"type": "free-db",
		"name": "Instance01"
	}
}`

func registerRunningInstanceMock(h *deployHarness) {
	h.handle("GET /v1/instances/db1d1234", http.StatusOK, `{"data": {"id": "db1d1234", "status": "running"}}`)
}

// withStubbedDispatch swaps both dispatch seams with recorders for the duration
// of the test, restoring them afterwards.
func withStubbedDispatch(t *testing.T, dockerFn func(ctx context.Context, cfg *clicfg.Config, container, db string, target deployTarget) error) {
	t.Helper()
	prevDocker := deployViaDockerFn
	prevDesktop := deployViaDesktopFn
	prevEdition := dockerSourceEditionFn
	deployViaDockerFn = dockerFn
	deployViaDesktopFn = func(_ context.Context, _ *clicfg.Config, _, _ string, _ int, _ deployTarget, _ io.Writer) error {
		return errors.New("desktop dispatch not expected in this test")
	}
	// Default the edition guard to enterprise so docker happy-path tests proceed
	// without a real daemon; the community-edition test overrides it directly.
	dockerSourceEditionFn = func(_ context.Context, _ string) (string, error) {
		return "enterprise", nil
	}
	t.Cleanup(func() {
		deployViaDockerFn = prevDocker
		deployViaDesktopFn = prevDesktop
		dockerSourceEditionFn = prevEdition
	})
}

func TestDeploySourceFlagsMutuallyExclusive(t *testing.T) {
	h := newDeployHarness(t)
	err := h.run("deploy --from-docker my-container --from-desktop dbms-1 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "from-docker")
	require.Contains(t, err.Error(), "from-desktop")
}

func TestDeploySourceFlagsOneRequired(t *testing.T) {
	h := newDeployHarness(t)
	err := h.run("deploy --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "from-docker")
	require.Contains(t, err.Error(), "from-desktop")
}

func TestDeployRejectsSystemDatabase(t *testing.T) {
	for _, name := range []string{"system", "SYSTEM", "System"} {
		t.Run(name, func(t *testing.T) {
			h := newDeployHarness(t)
			err := h.run("deploy --from-docker my-container --database " + name + " --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
			require.Error(t, err)
			require.Contains(t, err.Error(), "system database cannot be cloned")
		})
	}
}

func TestDeployFreeDbRejectsTargetSizingFlags(t *testing.T) {
	cases := []struct {
		name string
		flag string
	}{
		{"memory", "--memory 8GB"},
		{"region", "--region europe-west1"},
		{"cloud-provider", "--cloud-provider gcp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newDeployHarness(t)
			err := h.run("deploy --from-docker my-container --type free-db " + tc.flag + " --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
			require.Error(t, err)
			require.Contains(t, err.Error(), "free-db")
		})
	}
}

func TestDeployNonFreeRequiresSizingFlags(t *testing.T) {
	h := newDeployHarness(t)
	err := h.run("deploy --from-docker my-container --type professional-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	// Cobra reports the required flags it is missing.
	require.Contains(t, err.Error(), "required flag")
}

func TestDeployInvalidVersionRejected(t *testing.T) {
	h := newDeployHarness(t)
	err := h.run("deploy --from-docker my-container --type free-db --version 3 --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "version")
}

func TestDeployDockerSuccess(t *testing.T) {
	h := newDeployHarness(t)
	h.handle("POST /v1/instances", http.StatusAccepted, deployCreateResponse)
	registerRunningInstanceMock(h)

	var gotTarget deployTarget
	var gotContainer, gotDB string
	withStubbedDispatch(t, func(_ context.Context, _ *clicfg.Config, container, db string, target deployTarget) error {
		gotContainer, gotDB, gotTarget = container, db, target
		return nil
	})

	err := h.run("deploy --from-docker my-container --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.NoError(t, err)

	require.Equal(t, "my-container", gotContainer)
	require.Equal(t, "neo4j", gotDB)
	require.Equal(t, "neo4j+s://db1d1234.databases.neo4j.io", gotTarget.URI)
	require.Equal(t, "neo4j", gotTarget.Username)
	require.Equal(t, "letMeIn123!", gotTarget.Password)

	require.Contains(t, h.out.String(), `"deploy_status": "succeeded"`)
	require.Contains(t, h.out.String(), `"id": "db1d1234"`)
}

// TestDeployCreatesAndPollsBeforePush asserts the instance is created and polled
// to running before the data-load dispatch runs.
func TestDeployCreatesAndPollsBeforePush(t *testing.T) {
	h := newDeployHarness(t)

	var order []string
	h.mux.HandleFunc("POST /v1/instances", func(res http.ResponseWriter, _ *http.Request) {
		order = append(order, "create")
		res.WriteHeader(http.StatusAccepted)
		_, _ = res.Write([]byte(deployCreateResponse))
	})
	h.mux.HandleFunc("GET /v1/instances/db1d1234", func(res http.ResponseWriter, _ *http.Request) {
		order = append(order, "poll")
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write([]byte(`{"data": {"id": "db1d1234", "status": "running"}}`))
	})

	withStubbedDispatch(t, func(_ context.Context, _ *clicfg.Config, _, _ string, _ deployTarget) error {
		order = append(order, "push")
		return nil
	})

	require.NoError(t, h.run("deploy --from-docker my-container --name Instance01 --type free-db --rw --organization-id "+deployOrgID+" --project-id "+deployProjectID))

	require.Equal(t, []string{"create", "poll", "push"}, order)
}

func TestDeployPushFailureLeavesInstance(t *testing.T) {
	h := newDeployHarness(t)
	h.handle("POST /v1/instances", http.StatusAccepted, deployCreateResponse)
	registerRunningInstanceMock(h)

	pushErr := errors.New("neo4j-admin upload: boom")
	withStubbedDispatch(t, func(_ context.Context, _ *clicfg.Config, _, _ string, _ deployTarget) error {
		return pushErr
	})

	err := h.run("deploy --from-docker my-container --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	// Underlying tool error surfaced verbatim.
	require.Contains(t, err.Error(), "neo4j-admin upload: boom")

	// deploy_status failed + instance fields still printed.
	require.Contains(t, h.out.String(), `"deploy_status": "failed"`)
	require.Contains(t, h.out.String(), `"id": "db1d1234"`)
	// The instance id and a retry/delete hint appear on stderr (no auto-delete).
	require.Contains(t, h.err.String(), "db1d1234")
	require.Contains(t, h.err.String(), "left in place")
}

// TestDeployDesktopDispatch asserts --from-desktop routes to the desktop seam
// (not the docker seam) with the resolved target + port.
func TestDeployDesktopDispatch(t *testing.T) {
	h := newDeployHarness(t)
	h.handle("POST /v1/instances", http.StatusAccepted, deployCreateResponse)
	registerRunningInstanceMock(h)

	prevDocker := deployViaDockerFn
	prevDesktop := deployViaDesktopFn
	t.Cleanup(func() {
		deployViaDockerFn = prevDocker
		deployViaDesktopFn = prevDesktop
	})
	deployViaDockerFn = func(_ context.Context, _ *clicfg.Config, _, _ string, _ deployTarget) error {
		t.Fatal("docker dispatch must not run for --from-desktop")
		return nil
	}
	var gotID, gotDB string
	var gotPort int
	var gotTarget deployTarget
	deployViaDesktopFn = func(_ context.Context, _ *clicfg.Config, dbmsID, db string, port int, target deployTarget, _ io.Writer) error {
		gotID, gotDB, gotPort, gotTarget = dbmsID, db, port, target
		return nil
	}

	err := h.run("deploy --from-desktop dbms-99 --desktop-port 44222 --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.NoError(t, err)

	require.Equal(t, "dbms-99", gotID)
	require.Equal(t, "neo4j", gotDB)
	require.Equal(t, 44222, gotPort)
	require.Equal(t, "letMeIn123!", gotTarget.Password)
	require.Contains(t, h.out.String(), `"deploy_status": "succeeded"`)
}

// TestDeployTargetPasswordNotLeaked asserts the Aura target password never
// reaches stderr (narration / warnings), only the structured stdout output when
// the user has not opted out of printing it.
func TestDeployTargetPasswordNotLeaked(t *testing.T) {
	h := newDeployHarness(t)
	h.handle("POST /v1/instances", http.StatusAccepted, deployCreateResponse)
	registerRunningInstanceMock(h)

	withStubbedDispatch(t, func(_ context.Context, _ *clicfg.Config, _, _ string, _ deployTarget) error {
		return errors.New("neo4j-admin upload: boom")
	})

	_ = h.run("deploy --from-docker my-container --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)

	require.NotContains(t, h.err.String(), "letMeIn123!", "target password must not appear on stderr")
}

// TestDeployDockerCommunityFastFails asserts a community-edition source container
// is rejected with a clear usage error BEFORE any Aura instance is created.
func TestDeployDockerCommunityFastFails(t *testing.T) {
	h := newDeployHarness(t)

	var createCalled bool
	h.mux.HandleFunc("POST /v1/instances", func(res http.ResponseWriter, _ *http.Request) {
		createCalled = true
		res.WriteHeader(http.StatusAccepted)
		_, _ = res.Write([]byte(deployCreateResponse))
	})

	prevDocker := deployViaDockerFn
	prevEdition := dockerSourceEditionFn
	deployViaDockerFn = func(_ context.Context, _ *clicfg.Config, _, _ string, _ deployTarget) error {
		t.Fatal("docker dispatch must not run for a community-edition source")
		return nil
	}
	dockerSourceEditionFn = func(_ context.Context, _ string) (string, error) {
		return "community", nil
	}
	t.Cleanup(func() {
		deployViaDockerFn = prevDocker
		dockerSourceEditionFn = prevEdition
	})

	err := h.run("deploy --from-docker my-container --name Instance01 --type free-db --rw --organization-id " + deployOrgID + " --project-id " + deployProjectID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "enterprise")

	require.False(t, createCalled, "no Aura instance must be created when the source container is community edition")
}
