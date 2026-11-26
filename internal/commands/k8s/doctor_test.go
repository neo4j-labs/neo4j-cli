// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/shlex"
	engine "github.com/neo4j/cli/internal/k8s"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/flags"
	"github.com/neo4j/cli/internal/testutil/testfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// doctorEnv bundles the per-test seams for runDoctor: which binaries look
// present, the fake the leaf drives, and an optional factory override that
// sees the Settings the leaf built.
type doctorEnv struct {
	fake    *engine.FakeClient
	factory func(engine.Settings) engine.Client
}

// stubPresentBinaries points both engine lookPath seams at a hit, so the
// presence checks pass and the client calls run.
func stubPresentBinaries(t *testing.T) {
	t.Helper()
	engine.SetKubectlLookPathForTest(t, func(string) (string, error) { return "/usr/bin/kubectl", nil })
	engine.SetHelmLookPathForTest(t, func(string) (string, error) { return "/usr/bin/helm", nil })
}

// hideBinary makes one engine lookPath seam report a miss.
func hideBinary(t *testing.T, name string) {
	t.Helper()
	miss := func(string) (string, error) { return "", exec.ErrNotFound }
	if name == "kubectl" {
		engine.SetKubectlLookPathForTest(t, miss)
	} else {
		engine.SetHelmLookPathForTest(t, miss)
	}
}

// runDoctor drives the `k8s doctor` leaf. It returns stdout, stderr and
// the execution error. The clientFactory seam is swapped for the env's
// fake, so no child process ever runs.
func runDoctor(t *testing.T, args string, env *doctorEnv) (string, string, error) {
	t.Helper()
	return runDoctorCtx(t, context.Background(), args, env)
}

// runDoctorCtx is runDoctor with the context the command executes under.
func runDoctorCtx(t *testing.T, ctx context.Context, args string, env *doctorEnv) (string, string, error) {
	t.Helper()

	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	// The --kubeconfig existence check stats the test fs, so the path the
	// flag test passes must exist there.
	require.NoError(t, afero.WriteFile(fs, "/tmp/kube", []byte("apiVersion: v1\n"), 0o600))
	cfg := clicfg.NewConfig(fs, "test")

	origFactory := clientFactory
	if env.factory != nil {
		clientFactory = env.factory
	} else {
		clientFactory = func(engine.Settings) engine.Client { return env.fake }
	}
	t.Cleanup(func() { clientFactory = origFactory })

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"doctor"}, argv...))

	execErr := cmd.ExecuteContext(ctx)
	return out.String(), errBuf.String(), execErr
}

// decodeReport parses the JSON envelope a --format json run emitted.
func decodeReport(t *testing.T, stdout string) DoctorReport {
	t.Helper()
	var report DoctorReport
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report))
	return report
}

// statusOf returns the status of one check row by name.
func statusOf(t *testing.T, report DoctorReport, name string) string {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c.Status
		}
	}
	t.Fatalf("check %q missing from report: %+v", name, report.Checks)
	return ""
}

func TestDoctor_HealthyCluster_AllPass(t *testing.T) {
	stubPresentBinaries(t)
	env := &doctorEnv{fake: engine.NewFakeClient()}

	stdout, _, err := runDoctor(t, "--format json", env)
	require.NoError(t, err)

	report := decodeReport(t, stdout)
	for _, name := range []string{
		CheckKubectlPresent, CheckKubectlVersion, CheckHelmPresent, CheckHelmVersion,
		CheckClusterReachable, CheckKubernetesVersion, CheckVersionSkew,
	} {
		assert.Equal(t, StatusPass, statusOf(t, report, name), "check %s", name)
	}
	assert.True(t, report.Summary.Ready)
	assert.Equal(t, "v1.36.0", report.Summary.KubectlVersion)
	assert.Equal(t, "v3.16.4", report.Summary.HelmVersion)
	assert.Equal(t, "v1.36.0", report.Summary.ServerVersion)
	assert.Empty(t, report.Summary.NextStep)
	// The version call runs exactly once, not once per dependent check.
	assert.Equal(t, 1, env.fake.KubectlVersionCalls)
	assert.Equal(t, 1, env.fake.HelmVersionCalls)
}

func TestDoctor_KubectlMissing_SkipsDependentChecks(t *testing.T) {
	hideBinary(t, "kubectl")
	engine.SetHelmLookPathForTest(t, func(string) (string, error) { return "/usr/bin/helm", nil })
	env := &doctorEnv{fake: engine.NewFakeClient()}

	stdout, _, err := runDoctor(t, "--format json", env)
	require.NoError(t, err, "doctor always exits 0")

	report := decodeReport(t, stdout)
	assert.Equal(t, StatusFail, statusOf(t, report, CheckKubectlPresent))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckKubectlVersion))
	assert.Equal(t, StatusPass, statusOf(t, report, CheckHelmPresent))
	assert.Equal(t, StatusPass, statusOf(t, report, CheckHelmVersion))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckClusterReachable))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckKubernetesVersion))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckVersionSkew))
	assert.False(t, report.Summary.Ready)
	assert.NotEmpty(t, report.Summary.NextStep, "the summary names the first fix")
	assert.Zero(t, env.fake.KubectlVersionCalls, "no call may run when kubectl is missing")
	assert.Equal(t, 1, env.fake.HelmVersionCalls, "helm is checked independently")
}

func TestDoctor_HelmMissing_StillChecksCluster(t *testing.T) {
	stubPresentBinaries(t)
	hideBinary(t, "helm")
	env := &doctorEnv{fake: engine.NewFakeClient()}

	stdout, _, err := runDoctor(t, "--format json", env)
	require.NoError(t, err)

	report := decodeReport(t, stdout)
	assert.Equal(t, StatusFail, statusOf(t, report, CheckHelmPresent))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckHelmVersion))
	assert.Equal(t, StatusPass, statusOf(t, report, CheckClusterReachable))
	assert.False(t, report.Summary.Ready)
}

func TestDoctor_UnreachableCluster_FailsReachabilityAndSkipsServerChecks(t *testing.T) {
	stubPresentBinaries(t)
	fake := engine.NewFakeClient()
	fake.KubectlVersionResult = engine.VersionInfo{ClientVersion: "v1.36.0"}
	fake.KubectlVersionErr = clierr.NewUpstreamError("kubectl: The connection to the server localhost:8080 was refused")
	env := &doctorEnv{fake: fake}

	stdout, _, err := runDoctor(t, "--format json", env)
	require.NoError(t, err)

	report := decodeReport(t, stdout)
	// The client half of the version call still feeds the kubectl floor.
	assert.Equal(t, StatusPass, statusOf(t, report, CheckKubectlVersion))
	assert.Equal(t, StatusFail, statusOf(t, report, CheckClusterReachable))
	assert.Contains(t, detailOf(t, report, CheckClusterReachable), "connection to the server")
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckKubernetesVersion))
	assert.Equal(t, StatusSkip, statusOf(t, report, CheckVersionSkew))
	assert.False(t, report.Summary.Ready)
}

func TestDoctor_VersionChecks(t *testing.T) {
	for _, tc := range []struct {
		name      string
		versions  engine.VersionInfo
		helm      string
		wantRows  map[string]string
		wantReady bool
	}{
		{
			name:     "kubectl below the 1.22 floor fails",
			versions: engine.VersionInfo{ClientVersion: "v1.21.14", ServerVersion: "v1.36.0"},
			wantRows: map[string]string{CheckKubectlVersion: StatusFail},
		},
		{
			name:     "server below the 1.35 floor fails",
			versions: engine.VersionInfo{ClientVersion: "v1.36.0", ServerVersion: "v1.34.5"},
			wantRows: map[string]string{CheckClusterReachable: StatusPass, CheckKubernetesVersion: StatusFail},
		},
		{
			name:     "a skew wider than one minor warns rather than fails",
			versions: engine.VersionInfo{ClientVersion: "v1.38.0", ServerVersion: "v1.35.2"},
			wantRows: map[string]string{CheckVersionSkew: StatusWarn},
			// Only the warn is wrong here, so the report stays ready.
			wantReady: true,
		},
		{
			name:      "a skew of one minor passes",
			versions:  engine.VersionInfo{ClientVersion: "v1.35.1", ServerVersion: "v1.36.0"},
			wantRows:  map[string]string{CheckVersionSkew: StatusPass},
			wantReady: true,
		},
		{
			name:     "unknown versions fail loud",
			versions: engine.VersionInfo{},
			helm:     "-",
			wantRows: map[string]string{CheckKubectlVersion: StatusFail, CheckHelmVersion: StatusFail},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubPresentBinaries(t)
			fake := engine.NewFakeClient()
			fake.KubectlVersionResult = tc.versions
			if tc.helm == "-" {
				fake.HelmVersionResult = ""
			}

			report := decodeReport(t, mustRun(t, &doctorEnv{fake: fake}))
			for name, want := range tc.wantRows {
				assert.Equal(t, want, statusOf(t, report, name), "check %s", name)
			}
			assert.Equal(t, tc.wantReady, report.Summary.Ready)
		})
	}
}

func TestDoctor_InterruptReturnsContextError(t *testing.T) {
	stubPresentBinaries(t)
	ctx, cancel := context.WithCancel(context.Background())
	fake := engine.NewFakeClient()
	// The signal arrives while kubectl runs, as cli.Run's handler would.
	fake.KubectlVersionFn = func(context.Context) (engine.VersionInfo, error) {
		cancel()
		return engine.VersionInfo{}, context.Canceled
	}

	stdout, _, err := runDoctorCtx(t, ctx, "--format json", &doctorEnv{fake: fake})
	require.ErrorIs(t, err, context.Canceled, "an interrupt must not exit 0 with a report")
	assert.Empty(t, stdout)
}

func TestDoctor_ToonOutput_StripsControlBytesFromDetail(t *testing.T) {
	stubPresentBinaries(t)
	fake := engine.NewFakeClient()
	fake.KubectlVersionResult = engine.VersionInfo{ClientVersion: "v1.36.0"}
	// Credential plugins can colour their stderr, which toon rejects.
	fake.KubectlVersionErr = clierr.NewUpstreamError("kubectl: \x1b[31mconnection refused\x1b[0m")

	stdout, _, err := runDoctor(t, "--format toon", &doctorEnv{fake: fake})
	require.NoError(t, err)
	assert.Contains(t, stdout, "connection refused")
	assert.NotContains(t, stdout, "\x1b")
}

func TestDoctor_TableOutput_RendersRowsAndSummary(t *testing.T) {
	stubPresentBinaries(t)
	env := &doctorEnv{fake: engine.NewFakeClient()}

	stdout, _, err := runDoctor(t, "--format table", env)
	require.NoError(t, err)

	assert.Contains(t, stdout, "CHECK")
	assert.Contains(t, stdout, "STATUS")
	assert.Contains(t, stdout, "PASS")
	assert.Contains(t, stdout, "Summary: ready=true")
	assert.Contains(t, stdout, "kubectl_version=v1.36.0")
}

func TestDoctor_PassesGroupFlagsToEngine(t *testing.T) {
	stubPresentBinaries(t)
	fake := engine.NewFakeClient()
	var gotSettings engine.Settings
	env := &doctorEnv{
		fake: fake,
		factory: func(s engine.Settings) engine.Client {
			gotSettings = s
			return fake
		},
	}

	_, _, err := runDoctor(t, "--kubeconfig /tmp/kube --context kind-kind -n neo4j --debug", env)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/kube", gotSettings.Kubeconfig)
	assert.Equal(t, "kind-kind", gotSettings.Context)
	assert.Equal(t, "neo4j", gotSettings.Namespace)
	assert.True(t, gotSettings.Debug, "--debug must reach the engine")
}

func TestDoctor_MissingKubeconfigFileIsUsageError(t *testing.T) {
	stubPresentBinaries(t)
	env := &doctorEnv{fake: engine.NewFakeClient()}

	_, _, err := runDoctor(t, "--kubeconfig /no/such/file --format json", env)
	require.Error(t, err)
	var ce *clierr.CLIError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, 2, ce.Code, "a bad --kubeconfig path is a usage error")
	assert.Contains(t, err.Error(), "does not exist")
	assert.Zero(t, env.fake.KubectlVersionCalls, "no child call may run after the preflight")
}

func TestDoctor_Help_ListsUnderGroup(t *testing.T) {
	fs, err := testfs.GetDefaultTestFs()
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test")

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor", "--help"})
	require.NoError(t, cmd.Execute())
	got := out.String()
	for _, want := range []string{"doctor", "seven checks", "neo4j-cli k8s doctor", "--kubeconfig"} {
		assert.Contains(t, got, want)
	}
}

// mustRun runs doctor with --format json and returns stdout.
func mustRun(t *testing.T, env *doctorEnv) string {
	t.Helper()
	stdout, _, err := runDoctor(t, "--format json", env)
	require.NoError(t, err)
	return stdout
}

// detailOf returns the detail of one check row by name.
func detailOf(t *testing.T, report DoctorReport, name string) string {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c.Detail
		}
	}
	t.Fatalf("check %q missing from report", name)
	return ""
}
