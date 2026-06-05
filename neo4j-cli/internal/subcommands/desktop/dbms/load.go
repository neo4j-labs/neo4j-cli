// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/neo4j/cli/common/clicfg"
	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/common/output"
	"github.com/neo4j/cli/neo4j-cli/internal/dataset"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/cobra"
)

// resolveDatasetFn / downloadDatasetFn are the injectable seams the load leaf
// uses to talk to the internal/dataset support layer (manifest resolution +
// secure LFS download). Production wires dataset.Resolve / dataset.Download;
// the in-package load_test.go swaps deterministic fakes so the leaf's
// orchestration can be exercised without touching the network. Mirrors the
// docker load leaf seam.
var (
	resolveDatasetFn  = dataset.Resolve
	downloadDatasetFn = dataset.Download
)

var dbmsLoadFields = []string{"id", "name", "version", "status", "connection_uri"}

// dbmsLoadResult adapts a `*DbmsInfo` to output.ResponseData, matching the
// shape `desktop dbms create`/`list` emit so scripts can key off the same
// fields after a load.
type dbmsLoadResult struct {
	Item *desktopclient.DbmsInfo
}

func (r dbmsLoadResult) AsArray() []map[string]any {
	if r.Item == nil {
		return nil
	}
	return []map[string]any{
		{
			"id":             r.Item.ID,
			"name":           r.Item.Name,
			"version":        r.Item.Version,
			"status":         r.Item.Status,
			"connection_uri": r.Item.ConnectionURI,
		},
	}
}

func (r dbmsLoadResult) MarshalJSON() ([]byte, error) {
	if r.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.Item)
}

func newLoadCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		dbmsID   string
		name     string
		database string
		version  string
		password string
		maxSize  int64
		force    bool
		wait     bool
	)

	const (
		dbmsIDFlag   = "dbms-id"
		nameFlag     = "name"
		databaseFlag = "database"
		versionFlag  = "version"
		passwordFlag = "password"
		maxSizeFlag  = "max-size"
		forceFlag    = "force"
		waitFlag     = "wait"
	)

	cmd := &cobra.Command{
		Use:         "load <owner/repo>",
		Short:       "Load an example dataset into a Neo4j Desktop 2 DBMS",
		Annotations: map[string]string{"write": "true"},
		Long: "Load an example Neo4j dataset (a `.dump` published by a GitHub repo carrying a " +
			"`relate.project-install.json` manifest, e.g. `neo4j-graph-examples/movies`) into a DBMS managed by the " +
			"local Neo4j Desktop 2 install. The manifest is resolved, the matching dump is downloaded from the " +
			"Git-LFS media host, and the data is loaded into the `--database` (default `neo4j`). " +
			"Exactly one of `--dbms-id` or `--name` is required (they are mutually exclusive). " +
			"`--dbms-id <uuid>` targets an EXISTING DBMS: the load OVERWRITES that database's contents and therefore " +
			"REQUIRES `--force`; the DBMS is stopped, the dump is restored, manifest plugins are installed, then it is " +
			"restarted. `--name <name>` creates a NEW DBMS (newest stable enterprise version Desktop knows about, or " +
			"pin one with `--version`), loads the dump, installs plugins, and starts it. " +
			"Talks to Desktop's local relate API on http://localhost:<port>/fastify/api — Desktop must be running.",
		Example: `# Load the movies dataset into a brand-new Desktop DBMS
neo4j-cli desktop dbms load neo4j-graph-examples/movies --name movies --password supersecret --rw

# Overwrite an existing Desktop DBMS's data with a dataset (requires --force)
neo4j-cli desktop dbms load neo4j-graph-examples/movies --dbms-id 1234abcd --force --rw

# Load into a new DBMS and emit the resolved DbmsInfo as JSON for scripting
neo4j-cli desktop dbms load neo4j-graph-examples/recommendations --name recs --password supersecret --format json --rw`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ownerRepo := args[0]

			switch {
			case dbmsID == "" && name == "":
				return clierr.NewUsageError("exactly one of --%s or --%s is required", dbmsIDFlag, nameFlag)
			case dbmsID != "" && name != "":
				return clierr.NewUsageError("--%s and --%s are mutually exclusive", dbmsIDFlag, nameFlag)
			}

			ctx := cmd.Context()
			fs := cfg.Aura.Fs()
			port, _ := cmd.Flags().GetInt(portFlag)

			if dbmsID != "" {
				if !force {
					return clierr.NewUsageError(
						"loading a dataset into an existing DBMS OVERWRITES the %q database (destroying its current contents). Pass --force to proceed, or use --name to create a new DBMS.",
						database,
					)
				}
				cmd.SilenceUsage = true
				client, err := newDesktopClientFn(ctx, fs, port)
				if err != nil {
					return err
				}
				return loadIntoExistingDbms(ctx, cmd, cfg, client, ownerRepo, dbmsID, database, maxSize)
			}

			if password == "" {
				pw, perr := promptCreatePassword(cmd)
				if perr != nil {
					return perr
				}
				password = pw
			}
			cmd.SilenceUsage = true
			client, err := newDesktopClientFn(ctx, fs, port)
			if err != nil {
				return err
			}
			return loadIntoNewDbms(ctx, cmd, cfg, client, ownerRepo, name, database, version, password, maxSize, wait)
		},
	}

	cmd.Flags().StringVar(&dbmsID, dbmsIDFlag, "", "ID of an EXISTING DBMS to overwrite (mutually exclusive with --name; requires --force).")
	cmd.Flags().StringVar(&name, nameFlag, "", "Name for a NEW DBMS to create and load the dataset into (mutually exclusive with --dbms-id).")
	cmd.Flags().StringVar(&database, databaseFlag, "neo4j", "The target database the dump is loaded into.")
	cmd.Flags().StringVar(&version, versionFlag, "", "Neo4j version for the new DBMS (e.g. 5.26.1 or 2026.04.0). When omitted, the latest stable enterprise version Desktop knows about is used. Ignored with --dbms-id.")
	cmd.Flags().StringVar(&password, passwordFlag, "", "Initial password for the new DBMS's `neo4j` user (stored by Desktop's safeStorage). Prefer the interactive TTY prompt (omit --password) so the value does not land in argv. Ignored with --dbms-id.")
	cmd.Flags().Int64Var(&maxSize, maxSizeFlag, dataset.DefaultMaxDumpBytes, "Maximum dump download size in bytes; the download is refused if exceeded.")
	cmd.Flags().BoolVar(&force, forceFlag, false, "Required to overwrite an EXISTING DBMS's database (the load destroys its current contents). Only valid with --dbms-id.")
	cmd.Flags().BoolVar(&wait, waitFlag, false, "Block until the new DBMS reports status=started (polled every 1s, 30s ceiling). Only meaningful with --name.")

	return cmd
}

// installManifestPlugins installs each manifest plugin on the DBMS. A failure
// to install a required plugin aborts the load — a dataset that needs a plugin
// is unusable without it.
func installManifestPlugins(ctx context.Context, client *desktopclient.Client, dbmsID string, plugins []string) error {
	for _, p := range plugins {
		if _, err := client.InstallPlugin(ctx, dbmsID, p); err != nil {
			return clierr.NewFatalError("install plugin %q on DBMS %q: %s", p, dbmsID, err.Error())
		}
	}
	return nil
}

// loadIntoExistingDbms overwrites a database in an existing Desktop DBMS with
// the dataset dump. The caller has already gated this on --force. The DBMS's
// own version drives manifest resolution. Flow: GetDbms (version + status) ->
// resolve -> download -> stop if running -> LoadDump(overwrite) -> install
// manifest plugins -> start -> render.
func loadIntoExistingDbms(ctx context.Context, cmd *cobra.Command, cfg *clicfg.Config, client *desktopclient.Client, ownerRepo, dbmsID, database string, maxSize int64) error {
	info, err := client.GetDbms(ctx, dbmsID)
	if err != nil {
		return err
	}

	spec, err := resolveDatasetFn(ctx, ownerRepo, info.Version)
	if err != nil {
		return clierr.NewUsageError("resolve dataset %q for neo4j %s: %s", ownerRepo, info.Version, err.Error())
	}

	dumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
	if err != nil {
		return clierr.NewUsageError("download dataset dump: %s", err.Error())
	}
	defer cleanup()

	// load-dump requires the DBMS stopped. Stop unconditionally when running;
	// the start at the end restores it. A DBMS already stopped is left as-is.
	if info.Status == dbmsStatusStarted {
		if err := client.StopDbms(ctx, dbmsID); err != nil {
			return err
		}
		if _, perr := pollUntilStatus(ctx, client, dbmsID, dbmsStatusStopped); perr != nil {
			return perr
		}
	}

	if err := client.LoadDump(ctx, dbmsID, database, dumpPath, true); err != nil {
		return err
	}

	if err := installManifestPlugins(ctx, client, dbmsID, spec.Plugins); err != nil {
		return err
	}

	if err := client.StartDbms(ctx, dbmsID); err != nil {
		return err
	}

	return renderLoaded(ctx, cmd, cfg, client, dbmsID)
}

// loadIntoNewDbms creates a new Desktop DBMS, loads the dataset dump into it,
// installs the manifest plugins, and starts it. The Neo4j version is resolved
// up front (explicit --version, else Desktop's latest stable enterprise) and
// drives both manifest resolution and the create call.
func loadIntoNewDbms(ctx context.Context, cmd *cobra.Command, cfg *clicfg.Config, client *desktopclient.Client, ownerRepo, name, database, version, password string, maxSize int64, wait bool) error {
	if version == "" {
		versions, verr := client.ListDbmsVersions(ctx)
		if verr != nil {
			return verr
		}
		picked, perr := pickLatestStableEnterprise(versions)
		if perr != nil {
			return perr
		}
		version = picked.Version
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Using Neo4j enterprise %s (%s)\n", picked.Version, picked.Origin)
	}

	spec, err := resolveDatasetFn(ctx, ownerRepo, version)
	if err != nil {
		return clierr.NewUsageError("resolve dataset %q for neo4j %s: %s", ownerRepo, version, err.Error())
	}

	dumpPath, cleanup, err := downloadDatasetFn(ctx, spec, maxSize)
	if err != nil {
		return clierr.NewUsageError("download dataset dump: %s", err.Error())
	}
	defer cleanup()

	created, err := client.CreateDbms(ctx, desktopclient.CreateDbmsRequest{
		Name:     name,
		Version:  version,
		Password: password,
	})
	if err != nil {
		return err
	}

	// A freshly created DBMS is stopped, so load-dump can run immediately.
	if err := client.LoadDump(ctx, created.ID, database, dumpPath, true); err != nil {
		return err
	}

	if err := installManifestPlugins(ctx, client, created.ID, spec.Plugins); err != nil {
		return err
	}

	if err := client.StartDbms(ctx, created.ID); err != nil {
		return err
	}

	if wait {
		if _, perr := pollUntilStarted(ctx, client, created.ID); perr != nil {
			return perr
		}
	}

	return renderLoaded(ctx, cmd, cfg, client, created.ID)
}

// renderLoaded fetches the post-load DbmsInfo and prints it. A fetch failure
// after a successful load falls back to a slim `{id}` envelope so the exit code
// still reflects the (successful) load.
func renderLoaded(ctx context.Context, cmd *cobra.Command, cfg *clicfg.Config, client *desktopclient.Client, dbmsID string) error {
	info, ok := fetchForRender(ctx, client, dbmsID, cmd.ErrOrStderr())
	if !ok {
		info = &desktopclient.DbmsInfo{ID: dbmsID}
	}
	output.PrintBodyMap(cmd, cfg, dbmsLoadResult{Item: info}, dbmsLoadFields)
	return nil
}
