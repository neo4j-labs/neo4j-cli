// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"errors"
	engine "github.com/neo4j/cli/internal/docker"
	"sort"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/dataset"
	"github.com/neo4j/cli/internal/debug"
	"github.com/neo4j/cli/internal/flags"
	commonoutput "github.com/neo4j/cli/internal/output"
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

			if err := engine.ValidateDatabaseName(database); err != nil {
				return err
			}
			canonicalVersion, err := engine.ValidateVersion(version)
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
			case errors.Is(inspectErr, engine.ErrNotFound):
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
func loadIntoNewContainerLeaf(cmd *cobra.Command, cfg *clicfg.Config, client engine.Client, spec dataset.Spec, name, database, version string, maxSize int64, wait bool) error {
	ctx := cmd.Context()

	dumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
	if err != nil {
		cmd.SilenceUsage = true
		return clierr.NewUsageError("download dataset dump: %s", err.Error())
	}
	defer cleanup()

	result, err := engine.LoadDumpIntoNewContainer(ctx, cfg.DbmsCredentials(), client, engine.NewContainerLoad{
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
func loadIntoExistingContainer(cmd *cobra.Command, cfg *clicfg.Config, client engine.Client, container engine.Container, spec dataset.Spec, database string, maxSize int64, force bool) error {
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

	if err := engine.LoadDumpIntoExistingContainer(ctx, client, container.Name, hostDumpPath, database, engine.BoltAuth{
		URI:      cred.URI,
		Username: cred.Username,
		Password: cred.Password,
	}); err != nil {
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
