// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
)

// LoaderImportDir is the in-container mount point the dump is bind-mounted into
// for the one-shot loader. neo4j-admin database load reads from --from-path.
const LoaderImportDir = "/import"

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
// The image is the enterprise tag for the requested version (via EnterpriseImage:
// "latest" → neo4j:enterprise, else neo4j:<version>-enterprise) so neo4j-admin can
// load a dump from any supported source version. neo4j-admin database load requires the dump to
// be named `<database>.dump` under --from-path, so the dump is staged under that
// name in the bind-mounted dir before loading.
func LoadDumpIntoNewContainer(ctx context.Context, cfg *clicfg.Config, client Client, load NewContainerLoad) (NewContainerResult, error) {
	if err := ValidateDatabaseName(load.Database); err != nil {
		return NewContainerResult{}, err
	}
	if strings.TrimSpace(load.Version) == "" {
		load.Version = "latest"
	}
	version, err := ValidateVersion(load.Version)
	if err != nil {
		return NewContainerResult{}, err
	}

	chosenName, err := ResolveContainerName(ctx, client, cfg, load.Name)
	if err != nil {
		return NewContainerResult{}, err
	}
	if chosenName != load.Name {
		writeInfo(load.PreflightO, "name %q already in use; using %q\n", load.Name, chosenName)
	}

	boltPort, httpPort, err := FindFreePortPair(7687, 7474)
	if err != nil {
		return NewContainerResult{}, err
	}

	password := load.Password
	if password == "" {
		password, err = GeneratePassword()
		if err != nil {
			return NewContainerResult{}, err
		}
	}

	image := EnterpriseImage(version)
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
		"-v", stageDir + ":" + LoaderImportDir + ":ro",
		"-v", volume + ":/data",
		"-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT=eval",
		image,
		"neo4j-admin", "database", "load", load.Database,
		"--from-path=" + LoaderImportDir,
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

// writeInfo emits a single `info:` narration line to w, tolerating a nil writer.
func writeInfo(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "info: "+format, args...)
}
