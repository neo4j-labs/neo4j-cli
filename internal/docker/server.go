// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
)

// Edition is the Neo4j edition of a container.
type Edition string

const (
	EditionCommunity  Edition = "community"
	EditionEnterprise Edition = "enterprise"
)

// Mount is a bind mount or named volume attached to a container. Source is a
// host path or a volume name; Target is the path inside the container.
type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

// arg renders the mount as the value of docker's -v flag.
func (m Mount) arg() string {
	v := m.Source + ":" + m.Target
	if m.ReadOnly {
		v += ":ro"
	}
	return v
}

// ServerSpec describes a long-lived, CLI-managed Neo4j server container. It is
// the single place that knows how such a container is assembled, so `docker
// create`, `docker load` and `aura instance load` cannot drift apart.
//
// Version must already be canonical (see ValidateVersion) and Plugins validated
// (see ValidatePlugins).
type ServerSpec struct {
	Name     string
	Edition  Edition
	Version  string
	BoltPort int // host port published for Bolt (container 7687)
	HTTPPort int // host port published for the browser (container 7474)

	// AcceptLicense selects NEO4J_ACCEPT_LICENSE_AGREEMENT=yes instead of eval.
	// It only applies to the enterprise edition.
	AcceptLicense bool

	// Ephemeral runs the container with --rm and labels it ephemeral.
	Ephemeral bool

	Mounts  []Mount
	Plugins []string

	// Password is the initial password of the neo4j user. It is delivered to the
	// container through the docker process environment, never argv.
	Password string
}

// Image returns the image tag for the spec's edition and version.
func (s ServerSpec) Image() string {
	if s.Edition == EditionEnterprise {
		return EnterpriseImage(s.Version)
	}
	return "neo4j:" + s.Version
}

// Args returns the arguments for `docker run -d` (everything after `run -d`) and
// the environment to run it with. The password appears only in env: argv carries
// just the variable NAME (`-e NEO4J_AUTH`), because argv is world-readable via
// /proc/<pid>/cmdline.
//
// The order is fixed — name, --rm, ports, environment, mounts, labels, image —
// and is part of the contract tests assert against.
func (s ServerSpec) Args() (argv, env []string) {
	argv = []string{"--name", s.Name}
	if s.Ephemeral {
		argv = append(argv, "--rm")
	}
	argv = append(argv,
		"-p", fmt.Sprintf("%d:7474", s.HTTPPort),
		"-p", fmt.Sprintf("%d:7687", s.BoltPort),
		"-e", "NEO4J_AUTH",
	)
	if s.Edition == EditionEnterprise {
		license := "eval"
		if s.AcceptLicense {
			license = "yes"
		}
		argv = append(argv, "-e", "NEO4J_ACCEPT_LICENSE_AGREEMENT="+license)
	}
	if plugins := pluginsEnvValue(s.Plugins); plugins != "" {
		argv = append(argv, "-e", "NEO4J_PLUGINS="+plugins)
	}
	for _, m := range s.Mounts {
		argv = append(argv, "-v", m.arg())
	}
	for _, l := range s.labels() {
		argv = append(argv, "--label", l)
	}
	argv = append(argv, s.Image())

	return argv, []string{"NEO4J_AUTH=neo4j/" + s.Password}
}

// labels returns the metadata labels that mark the container as CLI-managed.
// `docker list`, `get`, `delete` and discovery read exactly these back (see
// labels.go), so they are written here and nowhere else.
func (s ServerSpec) labels() []string {
	ephemeral := "false"
	if s.Ephemeral {
		ephemeral = "true"
	}
	return []string{
		LabelManaged + "=true",
		LabelEdition + "=" + string(s.Edition),
		LabelVersion + "=" + s.Version,
		LabelBoltPort + "=" + strconv.Itoa(s.BoltPort),
		LabelHTTPPort + "=" + strconv.Itoa(s.HTTPPort),
		LabelEphemeral + "=" + ephemeral,
	}
}

// StartServer runs the container described by spec in the background.
func StartServer(ctx context.Context, client Client, spec ServerSpec) error {
	argv, env := spec.Args()
	_, err := client.RunWithEnv(ctx, argv, env)
	return err
}

var pluginNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ValidatePlugins normalises and validates Neo4j plugin names (the values of the
// NEO4J_PLUGINS environment variable the official image understands, such as
// apoc or graph-data-science): each is trimmed and lower-case letters, digits
// and hyphens only; duplicates are dropped, order is kept. Whether a name is a
// plugin the chosen image actually ships is the image's to say, so a typo
// surfaces when the container starts (`docker logs <name>`).
func ValidatePlugins(plugins []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, p := range plugins {
		p = strings.TrimSpace(p)
		if !pluginNamePattern.MatchString(p) {
			return nil, clierr.NewUsageError(
				"invalid argument %q for \"--plugin\" flag: a plugin name is lower-case letters, digits and hyphens (for example apoc or graph-data-science)", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// pluginsEnvValue renders plugin names into the JSON-array string NEO4J_PLUGINS
// expects (e.g. `["apoc","graph-data-science"]`). An empty slice yields "" so
// the caller omits the -e flag entirely.
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
