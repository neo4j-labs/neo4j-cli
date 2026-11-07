// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

// Package k8s implements the `neo4j-cli k8s` command group, behind the
// flag.k8s-commands feature flag: manage Neo4j on Kubernetes through the
// open-source Neo4j operator, by driving the host `kubectl` and `helm`.
package k8s

import (
	engine "github.com/neo4j/cli/internal/k8s"

	k8sflags "github.com/neo4j/cli/internal/commands/k8s/flags"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/spf13/cobra"
)

// clientFactory is the injectable seam for the engine.Client used by
// leaves. Production wires the exec-backed client. Tests swap in an
// engine.FakeClient without touching the leaf code.
var clientFactory = engine.NewClient

// NewCmd returns the `k8s` parent command. The group is a preview: it is
// mounted on the root only while flag.k8s-commands is on.
func NewCmd(cfg *clicfg.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "k8s",
		Short: "Manage Neo4j on Kubernetes through the Neo4j operator",
		Long: "Manage Neo4j on Kubernetes through the open-source Neo4j Kubernetes operator: install the operator, " +
			"create and inspect Neo4j servers and clusters, and connect to them. " +
			"The group drives the host `kubectl` and `helm` binaries, so both must be on PATH. " +
			"Pass --kubeconfig, --context and -n/--namespace on this group to scope every call. " +
			"This group is a preview behind the flag.k8s-commands feature flag. " +
			"Write commands will require --rw. Start with `k8s doctor` to check the local setup.",
	}

	k8sflags.RegisterPersistentFlags(cmd)

	cmd.AddCommand(newDoctorCmd(cfg))

	return cmd
}
