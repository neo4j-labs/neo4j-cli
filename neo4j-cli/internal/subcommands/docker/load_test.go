// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/shlex"
	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/flags"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/test/utils/testfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadDeps captures the seam overrides a load test installs.
type loadDeps struct {
	resolveSpec dataset.Spec
	resolveErr  error
	resolveGot  *struct {
		ownerRepo string
		version   string
	}

	downloadPath string
	downloadErr  error
	downloadGot  *struct {
		maxBytes int64
	}

	waitErr error
	waitGot *bool
}

// runLoad builds the docker parent with the load leaf wired, swaps every seam
// the leaf touches (dataset resolve/download, docker client, listener factory,
// bolt wait) for deterministic fakes, and executes `docker load <args>`.
func runLoad(t *testing.T, fake *fakeDockerClient, deps *loadDeps, args string) (*clicfg.Config, string, string, error) {
	t.Helper()

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)

	origFactory := clientFactory
	clientFactory = func(bool) dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })

	stubListenerFactory(t)

	origResolve, origDownload, origWait := resolveDatasetFn, downloadDatasetFn, waitForBoltFn
	t.Cleanup(func() { resolveDatasetFn, downloadDatasetFn, waitForBoltFn = origResolve, origDownload, origWait })

	deps.resolveGot = &struct {
		ownerRepo string
		version   string
	}{}
	deps.downloadGot = &struct{ maxBytes int64 }{}
	called := false
	deps.waitGot = &called

	resolveDatasetFn = func(_ context.Context, ownerRepo, version string) (dataset.Spec, error) {
		deps.resolveGot.ownerRepo = ownerRepo
		deps.resolveGot.version = version
		return deps.resolveSpec, deps.resolveErr
	}
	downloadDatasetFn = func(_ context.Context, _ dataset.Spec, maxBytes int64) (string, func(), error) {
		deps.downloadGot.maxBytes = maxBytes
		if deps.downloadErr != nil {
			return "", nil, deps.downloadErr
		}
		path := deps.downloadPath
		if path == "" {
			path = filepath.Join(t.TempDir(), "movies-50.dump")
		}
		require.NoError(t, os.WriteFile(path, []byte("dump"), 0o600))
		return path, func() {}, nil
	}
	waitForBoltFn = func(_ context.Context, _, _, _ string, _ time.Duration) error {
		called = true
		return deps.waitErr
	}

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)

	out := bytes.NewBuffer(nil)
	errBuf := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(errBuf)

	argv, splitErr := shlex.Split(args)
	require.NoError(t, splitErr)
	cmd.SetArgs(append([]string{"load"}, argv...))

	execErr := cmd.Execute()
	return cfg, out.String(), errBuf.String(), execErr
}

func moviesSpec() dataset.Spec {
	return dataset.Spec{
		Owner:    "neo4j-graph-examples",
		Repo:     "movies",
		Branch:   "main",
		DumpPath: "data/movies-50.dump",
		Plugins:  []string{"apoc"},
	}
}

func TestLoad_NewContainer_LoadsAndCreates(t *testing.T) {
	fake := newFakeDockerClient() // Inspect default-misses → ErrNotFound → new path
	deps := &loadDeps{resolveSpec: moviesSpec()}

	cfg, stdout, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies")
	require.NoError(t, err)

	// Resolve uses the default --version latest.
	assert.Equal(t, "neo4j-graph-examples/movies", deps.resolveGot.ownerRepo)
	assert.Equal(t, "latest", deps.resolveGot.version)

	// Two Run calls: the one-shot loader, then the server container.
	require.Len(t, fake.RunCalls, 2)
	loader := fake.RunCalls[0]
	loaderStr := strings.Join(loader, " ")
	// The loader runs via the image's DEFAULT entrypoint (no --entrypoint
	// override, no -c shell script) so neo4j-admin runs as the neo4j user.
	assert.NotContains(t, loader, "--entrypoint")
	assert.NotContains(t, loader, "-c")
	assert.Contains(t, loaderStr, "neo4j-admin database load neo4j --from-path="+loaderImportDir+" --overwrite-destination=true")
	assert.Contains(t, loaderStr, ":/data")
	assert.Contains(t, loaderStr, ":"+loaderImportDir+":ro")
	// The default entrypoint enforces the enterprise license gate, so the loader
	// must accept it via -e or neo4j-admin never runs.
	assert.Contains(t, loaderStr, "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval")
	// The image arg precedes the neo4j-admin command (default entrypoint form).
	// Default --version latest → enterprise image neo4j:enterprise (NOT neo4j:latest-enterprise).
	imageIdx := slices.Index(loader, "neo4j:enterprise")
	cmdIdx := slices.Index(loader, "neo4j-admin")
	require.GreaterOrEqual(t, imageIdx, 0)
	require.GreaterOrEqual(t, cmdIdx, 0)
	assert.Less(t, imageIdx, cmdIdx, "image must precede the neo4j-admin command")
	// The license -e flag must precede the image arg.
	licenseIdx := slices.Index(loader, "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval")
	require.GreaterOrEqual(t, licenseIdx, 0)
	assert.Less(t, licenseIdx, imageIdx, "license -e flag must precede the image arg")

	server := strings.Join(fake.RunCalls[1], " ")
	assert.Contains(t, server, "--name movies")
	assert.Contains(t, server, `NEO4J_PLUGINS=["apoc"]`)
	assert.Contains(t, server, "neo4j:enterprise")

	// The generated password travels via env, not argv.
	serverEnv := strings.Join(fake.RunEnvCalls[1].Env, " ")
	assert.Contains(t, serverEnv, "NEO4J_AUTH=neo4j/")

	// A dbms credential was stored for the new container.
	cred, gerr := cfg.Credentials.Dbms.Get("movies")
	require.NoError(t, gerr)
	assert.Equal(t, "neo4j", cred.Username)

	assert.Contains(t, stdout, "movies")
}

func TestLoad_NewContainer_WaitUsesWaitForBolt(t *testing.T) {
	fake := newFakeDockerClient()
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --wait")
	require.NoError(t, err)
	assert.True(t, *deps.waitGot, "expected WaitForBolt to be invoked with --wait")
}

func TestLoad_NewContainer_DatabaseOverride(t *testing.T) {
	fake := newFakeDockerClient()
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --database custom")
	require.NoError(t, err)
	loader := strings.Join(fake.RunCalls[0], " ")
	assert.Contains(t, loader, "neo4j-admin database load custom --from-path="+loaderImportDir+" --overwrite-destination=true")
}

func TestLoad_NewContainer_ExplicitVersionImageMapping(t *testing.T) {
	for _, tc := range []struct {
		name      string
		version   string
		wantImage string
	}{
		{name: "default latest", version: "", wantImage: "neo4j:enterprise"},
		{name: "explicit latest", version: "latest", wantImage: "neo4j:enterprise"},
		{name: "minor", version: "5.26", wantImage: "neo4j:5.26-enterprise"},
		{name: "calver", version: "2026.04.0", wantImage: "neo4j:2026.04.0-enterprise"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDockerClient()
			deps := &loadDeps{resolveSpec: moviesSpec()}

			args := "neo4j-graph-examples/movies --name movies"
			if tc.version != "" {
				args += " --version " + tc.version
			}
			_, _, _, err := runLoad(t, fake, deps, args)
			require.NoError(t, err)

			require.Len(t, fake.RunCalls, 2)
			assert.Contains(t, fake.RunCalls[0], tc.wantImage, "loader image")
			assert.Contains(t, fake.RunCalls[1], tc.wantImage, "server image")

			// The (possibly defaulted) version is forwarded to Resolve.
			wantResolveVersion := tc.version
			if wantResolveVersion == "" {
				wantResolveVersion = "latest"
			}
			assert.Equal(t, wantResolveVersion, deps.resolveGot.version)
		})
	}
}

func TestLoad_NewContainer_MaxSizeForwarded(t *testing.T) {
	fake := newFakeDockerClient()
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --max-size 12345")
	require.NoError(t, err)
	assert.Equal(t, int64(12345), deps.downloadGot.maxBytes)
}

func TestLoad_ExistingContainer_RefusedWithoutForce(t *testing.T) {
	fake := newFakeDockerClient()
	fake.Containers["movies"] = Container{Name: "movies", Managed: true, Plugins: []string{"apoc"}}
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--force")
	// No download / load attempted.
	assert.Empty(t, fake.ExecCalls)
}

func TestLoad_ExistingContainer_ForceOverwrites(t *testing.T) {
	fake := newFakeDockerClient()
	fake.Containers["movies"] = Container{Name: "movies", Managed: true, Plugins: []string{"apoc"}}
	deps := &loadDeps{resolveSpec: moviesSpec()}

	// runLoad seeds an empty credential store, so the existing-container path
	// fails at credential resolution before any load happens.
	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no stored dbms credential")
}

func TestLoad_ExistingContainer_ForceLoadsWithCredential(t *testing.T) {
	fake := newFakeDockerClient()
	fake.Containers["movies"] = Container{Name: "movies", Managed: true, Plugins: []string{"apoc"}}
	deps := &loadDeps{resolveSpec: moviesSpec()}

	// Stop/start over Bolt is a seam; swap it for a recorder.
	origStopStart := stopStartFn
	var stmts []string
	stopStartFn = func(_ context.Context, _, _, _, statement string) error {
		stmts = append(stmts, statement)
		return nil
	}
	t.Cleanup(func() { stopStartFn = origStopStart })

	fs, err := testfs.GetTestFs(`{}`, `{
		"dbms": {"credentials": [], "default-credential": ""},
		"embed": {"credentials": [], "default-credential": ""}
	}`)
	require.NoError(t, err)
	cfg := clicfg.NewConfig(fs, "test", clicfg.GlobalScope)
	require.NoError(t, cfg.Credentials.Dbms.Add("movies", "neo4j", "pw", "neo4j", "neo4j://localhost:7687"))

	origFactory := clientFactory
	clientFactory = func(bool) dockerClient { return fake }
	t.Cleanup(func() { clientFactory = origFactory })
	stubListenerFactory(t)

	origResolve, origDownload := resolveDatasetFn, downloadDatasetFn
	t.Cleanup(func() { resolveDatasetFn, downloadDatasetFn = origResolve, origDownload })
	resolveDatasetFn = func(_ context.Context, _, _ string) (dataset.Spec, error) { return deps.resolveSpec, nil }
	downloadDatasetFn = func(_ context.Context, _ dataset.Spec, _ int64) (string, func(), error) {
		return t.TempDir() + "/movies-50.dump", func() {}, nil
	}

	cmd := NewCmd(cfg)
	flags.RegisterOutputFlag(cmd, cfg)
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"load", "neo4j-graph-examples/movies", "--name", "movies", "--force"})
	require.NoError(t, cmd.Execute())

	assert.Equal(t, []string{"STOP DATABASE neo4j", "START DATABASE neo4j"}, stmts)

	// The dump was copied in and neo4j-admin load was invoked.
	require.Len(t, fake.CopyToCalls, 1)
	assert.Equal(t, "movies", fake.CopyToCalls[0].Name)
	assert.Contains(t, fake.CopyToCalls[0].ContainerPath, "neo4j.dump")

	var loadInvoked bool
	for _, c := range fake.ExecCalls {
		if len(c.Args) >= 3 && c.Args[0] == "neo4j-admin" && c.Args[1] == "database" && c.Args[2] == "load" {
			loadInvoked = true
		}
	}
	assert.True(t, loadInvoked, "expected neo4j-admin database load to run")
}

func TestLoad_ExistingContainer_MissingPluginRefused(t *testing.T) {
	fake := newFakeDockerClient()
	fake.Containers["movies"] = Container{Name: "movies", Managed: true, Plugins: nil}
	deps := &loadDeps{resolveSpec: moviesSpec()} // requires apoc

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing plugin")
	assert.Contains(t, err.Error(), "apoc")
}

func TestLoad_ExistingContainer_UnmanagedRefused(t *testing.T) {
	fake := newFakeDockerClient()
	fake.Containers["other"] = Container{Name: "other", Managed: false}
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name other --force")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no managed container")
}

func TestLoad_ResolveError_Surfaces(t *testing.T) {
	fake := newFakeDockerClient()
	deps := &loadDeps{resolveErr: errors.New("manifest not found")}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/nope --name x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve dataset")
	// No docker side effects on a resolve failure.
	assert.Empty(t, fake.RunCalls)
}

func TestLoad_InspectOperationalError_Propagates(t *testing.T) {
	fake := newFakeDockerClient()
	fake.InspectFn = func(_ context.Context, _ string) (Container, error) {
		return Container{}, errors.New("docker daemon not running")
	}
	deps := &loadDeps{resolveSpec: moviesSpec()}

	_, _, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon not running")
}

func TestLoad_NewContainer_NoPluginsEmitsEmptyArray(t *testing.T) {
	fake := newFakeDockerClient()
	spec := moviesSpec()
	spec.Plugins = nil
	deps := &loadDeps{resolveSpec: spec}

	_, stdout, _, err := runLoad(t, fake, deps, "neo4j-graph-examples/movies --name movies")
	require.NoError(t, err)
	// No NEO4J_PLUGINS env when the manifest declares none.
	server := strings.Join(fake.RunCalls[1], " ")
	assert.NotContains(t, server, "NEO4J_PLUGINS")
	assert.Contains(t, stdout, "movies")
}

func TestParseNeo4jPluginsEnv(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want []string
	}{
		{"present", []string{"FOO=bar", `NEO4J_PLUGINS=["apoc","graph-data-science"]`}, []string{"apoc", "graph-data-science"}},
		{"absent", []string{"FOO=bar"}, nil},
		{"empty", []string{"NEO4J_PLUGINS="}, nil},
		{"unparseable", []string{"NEO4J_PLUGINS=apoc"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseNeo4jPluginsEnv(tc.env))
		})
	}
}

func TestMissingPlugins(t *testing.T) {
	assert.Equal(t, []string{"apoc"}, missingPlugins([]string{"apoc"}, nil))
	assert.Empty(t, missingPlugins([]string{"apoc"}, []string{"APOC"}))
	assert.Equal(t, []string{"bloom", "gds"}, missingPlugins([]string{"gds", "bloom", "apoc"}, []string{"apoc"}))
}
