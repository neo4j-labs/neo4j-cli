// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/debug"
	"github.com/neo4j/cli/common/flags"
	commonoutput "github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/spf13/cobra"
)

// resolveDatasetFn / downloadDatasetFn are the injectable seams the load leaf
// uses to talk to the internal/dataset support layer (manifest resolution +
// secure LFS download). Production wires dataset.Resolve / dataset.Download;
// load_test.go swaps deterministic fakes so the leaf's orchestration can be
// exercised without touching the network. They mirror the existing
// waitForBoltFn / stopStartFn seam idiom in this package.
var (
	resolveDatasetFn  = dataset.Resolve
	downloadDatasetFn = dataset.Download
)

// loaderImportDir is the in-container mount point the dump is bind-mounted into
// for the one-shot loader. neo4j-admin database load reads from --from-path.
const loaderImportDir = "/import"

// newLoadCmd builds the `neo4j-cli docker load <owner/repo>` leaf. It resolves
// the dataset's relate.project-install.json manifest, downloads the matching
// dump from the Git-LFS media host, and loads it into either a NEW container
// (created on a fresh named volume) or an EXISTING managed container (overwrite,
// gated behind --force).
func newLoadCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		name     string
		database string
		version  string
		maxSize  int64
		force    bool
		wait     bool
	)

	const (
		nameFlag     = "name"
		databaseFlag = "database"
		versionFlag  = "version"
		maxSizeFlag  = "max-size"
		forceFlag    = "force"
	)

	cmd := &cobra.Command{
		Use:         "load <owner/repo>",
		Short:       "Load an example dataset into a local Neo4j Docker container",
		Annotations: map[string]string{"write": "true"},
		Long: "Load an example Neo4j dataset (a `.dump` published by a GitHub repo carrying a " +
			"`relate.project-install.json` manifest, e.g. `neo4j-graph-examples/movies`) into a local Neo4j " +
			"Docker container. The manifest is resolved for the requested --version, the matching dump is " +
			"downloaded from the Git-LFS media host, and the data is loaded into the --database (default `neo4j`). " +
			"When --name refers to a container that does not yet exist, a new container is created on a fresh named " +
			"volume with NEO4J_PLUGINS set from the manifest and started. When --name refers to an EXISTING managed " +
			"container, the load OVERWRITES that database's contents and therefore REQUIRES --force; if the existing " +
			"container is missing a manifest-required plugin the load is refused (plugins cannot be added without " +
			"recreating the container). Pass --wait to block until Bolt is reachable.",
		Example: `# Load the movies dataset into a new container (created automatically)
neo4j-cli docker load neo4j-graph-examples/movies --name movies --rw

# Load into a new container and block until Bolt is reachable
neo4j-cli docker load neo4j-graph-examples/recommendations --name recs --wait --rw

# Overwrite an existing container's data with a dataset (requires --force)
neo4j-cli docker load neo4j-graph-examples/movies --name movies --force --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ownerRepo := args[0]
			ctx := cmd.Context()

			if err := validateDatabaseName(database); err != nil {
				return err
			}
			canonicalVersion, err := validateVersion(version)
			if err != nil {
				return err
			}
			version = canonicalVersion

			spec, err := resolveDatasetFn(ctx, ownerRepo, version)
			if err != nil {
				cmd.SilenceUsage = true
				return clierr.NewUsageError("resolve dataset %q: %s", ownerRepo, err.Error())
			}

			client := clientFactory(debug.Resolve(cmd))

			// Decide new-vs-existing by inspecting the requested name. A
			// missing container (ErrNotFound) takes the new path; any other
			// inspect error (daemon down, permission denied) propagates verbatim.
			existing, inspectErr := client.Inspect(ctx, name)
			switch {
			case inspectErr == nil:
				return loadIntoExistingContainer(cmd, cfg, client, existing, spec, database, maxSize, force)
			case errors.Is(inspectErr, ErrNotFound):
				return loadIntoNewContainerLeaf(cmd, cfg, client, spec, name, database, version, maxSize, wait)
			default:
				cmd.SilenceUsage = true
				return inspectErr
			}
		},
	}

	cmd.Flags().StringVar(&name, nameFlag, "", "(required) Container name. New if it does not exist; existing managed container if it does (requires --force).")
	cmd.MarkFlagRequired(nameFlag) //nolint:errcheck // MarkFlagRequired only errors on an unknown flag name, a startup-caught programming error
	cmd.Flags().StringVar(&database, databaseFlag, "neo4j", "The target database the dump is loaded into.")
	cmd.Flags().StringVar(&version, versionFlag, "latest", "Neo4j version to resolve the manifest against and use for the container image. Accepts 5, 5.26, calver (e.g. 2026.04.0), or latest (default). Must satisfy the dump's targetNeo4jVersion.")
	cmd.Flags().Int64Var(&maxSize, maxSizeFlag, dataset.DefaultMaxDumpBytes, "Maximum dump download size in bytes; the download is refused if exceeded.")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Required to overwrite an EXISTING container's database (the load destroys its current contents).")
	flags.RegisterWait(cmd, &wait, "Wait until Bolt is reachable before returning (new container only).")

	return cmd
}

// loadIntoNewContainerLeaf is the new-container branch of `docker load`: it
// downloads the dump and delegates to the reusable LoadDumpIntoNewContainer
// helper, then narrates the result. Kept separate from the helper so the leaf
// owns flag/output concerns while the helper stays reusable by the aura loader.
func loadIntoNewContainerLeaf(cmd *cobra.Command, cfg *clicfg.Config, client dockerClient, spec dataset.Spec, name, database, version string, maxSize int64, wait bool) error {
	ctx := cmd.Context()

	dumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
	if err != nil {
		cmd.SilenceUsage = true
		return clierr.NewUsageError("download dataset dump: %s", err.Error())
	}
	defer cleanup()

	result, err := LoadDumpIntoNewContainer(ctx, cfg, client, NewContainerLoad{
		Name:       name,
		Database:   database,
		Version:    version,
		Plugins:    spec.Plugins,
		DumpPath:   dumpPath,
		Wait:       wait,
		WaitOut:    cmd.ErrOrStderr(),
		PreflightO: cmd.ErrOrStderr(),
	})
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}

	row := map[string]any{
		"name":      result.Name,
		"database":  database,
		"version":   version,
		"bolt_port": result.BoltPort,
		"http_port": result.HTTPPort,
		"uri":       result.URI,
		"username":  "neo4j",
		"password":  result.Password,
		"plugins":   pluginsForOutput(spec.Plugins),
	}
	fields := []string{"name", "database", "version", "bolt_port", "http_port", "uri", "username", "password", "plugins"}
	commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, fields)
	return nil
}

// loadIntoExistingContainer overwrites the database in an already-running
// managed container with the dataset dump (REQ-F-015). It is refused without
// --force, refused for an unmanaged container, and refused when the container
// is missing a manifest-required plugin. The flow mirrors PushToAura's
// STOP / load / START pattern over Bolt + docker exec.
func loadIntoExistingContainer(cmd *cobra.Command, cfg *clicfg.Config, client dockerClient, container Container, spec dataset.Spec, database string, maxSize int64, force bool) error {
	ctx := cmd.Context()

	if !container.Managed {
		cmd.SilenceUsage = true
		return unknownContainerError(container.Name)
	}
	if !force {
		cmd.SilenceUsage = true
		return clierr.NewUsageError(
			"container %q already exists; loading a dataset OVERWRITES the %q database (destroying its current contents). Pass --force to proceed, or use a new --name.",
			container.Name, database,
		)
	}
	if missing := missingPlugins(spec.Plugins, container.Plugins); len(missing) > 0 {
		cmd.SilenceUsage = true
		return clierr.NewUsageError(
			"container %q is missing plugin(s) required by this dataset: %s. Plugins cannot be added to a running container; recreate it (delete then `neo4j-cli docker load %s/%s --name %s`) so the manifest plugins are installed.",
			container.Name, strings.Join(missing, ", "), spec.Owner, spec.Repo, container.Name,
		)
	}

	if cfg == nil || cfg.Credentials == nil || cfg.Credentials.Dbms == nil {
		cmd.SilenceUsage = true
		return clierr.NewUsageError("credential storage is not available; cannot resolve the password for container %q", container.Name)
	}
	cred, err := cfg.Credentials.Dbms.Get(container.Name)
	if err != nil {
		cmd.SilenceUsage = true
		return clierr.NewUsageError(
			"no stored dbms credential named %q for the existing container; it must have been created via `neo4j-cli docker create`/`docker load` to be loadable",
			container.Name,
		)
	}

	hostDumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
	if err != nil {
		cmd.SilenceUsage = true
		return clierr.NewUsageError("download dataset dump: %s", err.Error())
	}
	defer cleanup()

	// Stage the dump inside the container's scratch dir (a running container
	// cannot have a new volume bind-mounted), then STOP / load / START. Order
	// mirrors PushToAura: START is deferred so a mid-flight failure never
	// leaves the database stopped. neo4j-admin database load expects the dump
	// at <database>.dump under --from-path, so we copy it to that name.
	if _, err := client.ExecAs(ctx, container.Name, dumpUser, []string{"mkdir", "-p", dumpPath}, nil); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	defer func() { _, _ = client.ExecAs(ctx, container.Name, dumpUser, []string{"rm", "-rf", dumpPath}, nil) }()

	destPath := dumpPath + "/" + database + ".dump"
	if err := client.CopyTo(ctx, hostDumpPath, container.Name, destPath); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	if err := stopStartFn(ctx, cred.URI, cred.Username, cred.Password, "STOP DATABASE "+database); err != nil {
		cmd.SilenceUsage = true
		return fmt.Errorf("docker load: stop database %q: %w", database, err)
	}
	defer func() {
		_ = stopStartFn(ctx, cred.URI, cred.Username, cred.Password, "START DATABASE "+database)
	}()

	if _, err := client.ExecAs(ctx, container.Name, dumpUser, []string{
		"neo4j-admin", "database", "load", database,
		"--from-path=" + dumpPath,
		"--overwrite-destination=true",
	}, nil); err != nil {
		cmd.SilenceUsage = true
		return err
	}

	row := map[string]any{
		"name":     container.Name,
		"database": database,
		"loaded":   true,
	}
	fields := []string{"name", "database", "loaded"}
	commonoutput.PrintBodyMap(cmd, cfg, singleRow{row: row}, fields)
	return nil
}

// NewContainerLoad bundles the inputs to LoadDumpIntoNewContainer so callers
// (the docker load leaf and the future aura instance load leaf) pass an
// explicit, named set of parameters rather than a long positional argument list.
type NewContainerLoad struct {
	Name     string
	Database string
	Version  string
	Plugins  []string
	DumpPath string // host path to the dump file (from dataset.Download)
	Password string // optional; generated when empty
	Wait     bool
	WaitOut  io.Writer
	// PreflightO receives single-line `info:` narration about name/port
	// fallback. nil is tolerated (narration is dropped).
	PreflightO io.Writer
}

// NewContainerResult reports the connection details of the container created by
// LoadDumpIntoNewContainer.
type NewContainerResult struct {
	Name     string
	BoltPort int
	HTTPPort int
	URI      string
	Password string
}

// LoadDumpIntoNewContainer creates a fresh local Neo4j container pre-loaded with
// a dataset dump. It is the reusable core of `docker load` (new-container path)
// and is also consumed by the aura instance loader to stage a dump through an
// ephemeral local Neo4j before pushing to Aura.
//
// Flow:
//  1. resolve a non-colliding container name + free bolt/http port pair;
//  2. run a one-shot loader container (via the image's default entrypoint, so
//     neo4j-admin runs as the neo4j user) that bind-mounts the staged dump dir
//     read-only at /import and a fresh named volume at /data, running
//     `neo4j-admin database load <db> --from-path=/import --overwrite-destination=true`;
//  3. create the long-lived server container reusing that named volume with
//     NEO4J_PLUGINS from the manifest;
//  4. optionally wait for Bolt.
//
// The image is the enterprise tag for the requested version (via enterpriseImage:
// "latest" → neo4j:enterprise, else neo4j:<version>-enterprise) so neo4j-admin can
// load a dump from any supported source version. neo4j-admin database load requires the dump to
// be named `<database>.dump` under --from-path, so the dump is staged under that
// name in the bind-mounted dir before loading.
func LoadDumpIntoNewContainer(ctx context.Context, cfg *clicfg.Config, client dockerClient, load NewContainerLoad) (NewContainerResult, error) {
	if err := validateDatabaseName(load.Database); err != nil {
		return NewContainerResult{}, err
	}
	if strings.TrimSpace(load.Version) == "" {
		load.Version = "latest"
	}
	version, err := validateVersion(load.Version)
	if err != nil {
		return NewContainerResult{}, err
	}

	chosenName, err := resolveContainerName(ctx, client, cfg, load.Name)
	if err != nil {
		return NewContainerResult{}, err
	}
	if chosenName != load.Name {
		writeInfo(load.PreflightO, "name %q already in use; using %q\n", load.Name, chosenName)
	}

	boltPort, httpPort, err := findFreePortPair(7687, 7474)
	if err != nil {
		return NewContainerResult{}, err
	}

	password := load.Password
	if password == "" {
		password, err = generatePassword()
		if err != nil {
			return NewContainerResult{}, err
		}
	}

	image := enterpriseImage(version)
	volume := "neo4j-cli-" + chosenName + "-data"

	// Stage the dump as <database>.dump in a dedicated dir so the loader
	// container mounts only the single dump file (not the shared host temp dir
	// where dataset.Download's os.CreateTemp places it) and neo4j-admin finds
	// the file named <database>.dump under --from-path. The staging dir is 0755
	// and the staged dump 0644 because the loader runs as the in-container neo4j
	// user (uid 7474), which must be able to read the bind-mounted (:ro) copy; it
	// is a public example dataset in a throwaway container, so world-readable on
	// this staging copy is fine (the original 0600 download temp is untouched).
	stageDir, err := os.MkdirTemp("", "neo4j-cli-load-*")
	if err != nil {
		return NewContainerResult{}, fmt.Errorf("docker load: create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()
	if err := os.Chmod(stageDir, 0o755); err != nil {
		return NewContainerResult{}, fmt.Errorf("docker load: chmod staging dir: %w", err)
	}

	stagedDump := filepath.Join(stageDir, load.Database+".dump")
	if err := copyFile(load.DumpPath, stagedDump); err != nil {
		return NewContainerResult{}, fmt.Errorf("docker load: stage dump: %w", err)
	}

	// Run the loader via the image's DEFAULT entrypoint (no --entrypoint
	// override) so docker-entrypoint.sh drops to the neo4j user
	// (exec su-exec neo4j:neo4j "$@") before running neo4j-admin. This makes the
	// loaded /data/databases/<db> files owned by uid 7474, matching the server
	// container — otherwise neo4j-admin runs as root and the server (which drops
	// to neo4j) cannot write the root-owned files, leaving the database offline.
	// The default entrypoint enforces the enterprise license gate, so the loader
	// must accept it or neo4j-admin never runs and the database ends up empty.
	loaderArgs := []string{
		"--rm",
		"-v", stageDir + ":" + loaderImportDir + ":ro",
		"-v", volume + ":/data",
		"-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
		image,
		"neo4j-admin", "database", "load", load.Database,
		"--from-path=" + loaderImportDir,
		"--overwrite-destination=true",
	}
	if _, err := client.Run(ctx, loaderArgs); err != nil {
		return NewContainerResult{}, fmt.Errorf("docker load: run loader: %w", err)
	}

	// Long-lived server container reusing the loaded volume.
	argv := []string{"--name", chosenName}
	argv = append(argv, "-p", fmt.Sprintf("%d:7474", httpPort))
	argv = append(argv, "-p", fmt.Sprintf("%d:7687", boltPort))
	argv = append(argv, "-e", "NEO4J_AUTH")
	argv = append(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval")
	if pluginsEnv := pluginsEnvValue(load.Plugins); pluginsEnv != "" {
		argv = append(argv, "-e", "NEO4J_PLUGINS="+pluginsEnv)
	}
	argv = append(argv, "-v", volume+":/data")
	argv = append(argv, "--label", LabelManaged+"=true")
	argv = append(argv, "--label", LabelEdition+"=enterprise")
	argv = append(argv, "--label", LabelVersion+"="+version)
	argv = append(argv, "--label", LabelBoltPort+"="+strconv.Itoa(boltPort))
	argv = append(argv, "--label", LabelHTTPPort+"="+strconv.Itoa(httpPort))
	argv = append(argv, "--label", LabelEphemeral+"=false")
	argv = append(argv, image)

	if _, err := client.RunWithEnv(ctx, argv, []string{"NEO4J_AUTH=neo4j/" + password}); err != nil {
		return NewContainerResult{}, err
	}

	uri := fmt.Sprintf("neo4j://localhost:%d", boltPort)

	if cfg != nil && cfg.Credentials != nil && cfg.Credentials.Dbms != nil {
		if err := cfg.Credentials.Dbms.Add(chosenName, "neo4j", password, load.Database, uri); err != nil {
			return NewContainerResult{}, err
		}
	}

	if load.Wait {
		writeInfo(load.WaitOut, "waiting for Bolt on localhost:%d...\n", boltPort)
		if err := waitForBoltFn(ctx, uri, "neo4j", password, waitTimeout); err != nil {
			return NewContainerResult{}, err
		}
	}

	return NewContainerResult{
		Name:     chosenName,
		BoltPort: boltPort,
		HTTPPort: httpPort,
		URI:      uri,
		Password: password,
	}, nil
}

// generatePassword mints a base64 URL-safe password from randSource, matching
// the docker create generator so credentials are interchangeable.
func generatePassword() (string, error) {
	buf := make([]byte, generatedPasswordBytes)
	if _, err := io.ReadFull(randSource, buf); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// copyFile copies src to dst, creating dst with 0644 perms (the in-container
// neo4j user must read the bind-mounted staged dump; see LoadDumpIntoNewContainer).
// Copying (rather than moving) leaves the original temp file for the caller's
// cleanup() to remove.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// pluginsEnvValue renders the manifest plugin slugs into the JSON-array string
// Neo4j's NEO4J_PLUGINS env var expects (e.g. `["apoc","graph-data-science"]`).
// An empty slice yields "" so the caller can skip the -e flag entirely.
func pluginsEnvValue(plugins []string) string {
	if len(plugins) == 0 {
		return ""
	}
	b, err := json.Marshal(plugins)
	if err != nil {
		return ""
	}
	return string(b)
}

// pluginsForOutput returns a non-nil slice so the rendered row shows [] rather
// than null when a dataset declares no plugins.
func pluginsForOutput(plugins []string) []string {
	if plugins == nil {
		return []string{}
	}
	return plugins
}

// missingPlugins returns the required plugins not present in have (case- and
// order-insensitive), sorted for a deterministic error message.
func missingPlugins(required, have []string) []string {
	present := map[string]struct{}{}
	for _, p := range have {
		present[strings.ToLower(strings.TrimSpace(p))] = struct{}{}
	}
	var missing []string
	for _, p := range required {
		if _, ok := present[strings.ToLower(strings.TrimSpace(p))]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	return missing
}

// writeInfo emits a single `info:` narration line to w, tolerating a nil writer.
func writeInfo(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "info: "+format, args...)
}
