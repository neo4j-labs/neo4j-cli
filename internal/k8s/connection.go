// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
	"github.com/neo4j/cli/internal/output"
)

// DefaultBoltPort is the local port used by a stored connection unless the
// user selects another port with --port.
const DefaultBoltPort = 7687

// InstanceCredential identifies a local connection to a standalone Neo4j
// resource. Its Name includes the kube context and namespace.
type InstanceCredential struct {
	Name         string
	InstanceName string
	Context      string
	Namespace    string
	URI          string
	Username     string
	Password     string
	BoltTLS      bool
}

// ReadCredential reads the generated auth Secret and builds a local URI.
// It leaves password storage to the command layer.
func ReadCredential(ctx context.Context, client Client, name string, port int) (InstanceCredential, error) {
	kubeContext, err := client.CurrentContext(ctx)
	if err != nil {
		return InstanceCredential{}, err
	}
	if kubeContext == "" || name == "" {
		return InstanceCredential{}, clierr.NewUsageError("a Kubernetes context and Neo4j name are required for a credential")
	}
	instance, err := client.GetNeo4j(ctx, name)
	if err != nil {
		return InstanceCredential{}, err
	}
	if instance.Mode != "Standalone" {
		return InstanceCredential{}, clierr.NewUsageError(
			"Neo4j %q is not standalone. Credentials and connections for clusters are not supported yet", name)
	}
	data, err := client.GetSecret(ctx, name+"-auth")
	if IsNotFound(err) {
		return InstanceCredential{}, clierr.NewNotFoundError("auth Secret %q does not exist yet", name+"-auth").
			WithSuggestion("Run `neo4j-cli k8s instance get " + name + "` to inspect readiness")
	}
	if err != nil {
		return InstanceCredential{}, err
	}
	auth := string(data["NEO4J_AUTH"])
	if auth == "none" {
		return InstanceCredential{}, clierr.NewUsageError("authentication is disabled for Neo4j %q, so there is no credential to read", name)
	}
	// Register the received value before parsing it, including a malformed
	// value that an error or debug trace must never expose.
	clievents.RegisterSecretValue(auth)
	// dotenv output can encode tabs and line endings, but not terminal
	// control sequences. Reject those before any credential output.
	if output.StripControl(auth) != auth {
		return InstanceCredential{}, clierr.NewUsageError("NEO4J_AUTH in Secret %q contains unsupported control characters", name+"-auth")
	}
	username, password, ok := strings.Cut(auth, "/")
	if !ok || username == "" || password == "" {
		return InstanceCredential{}, clierr.NewUsageError("Secret %q must contain NEO4J_AUTH in username/password form", name+"-auth")
	}
	clievents.RegisterSecretValue(password)
	scheme := "bolt"
	if instance.BoltTLS {
		scheme = "bolt+ssc"
	}
	return InstanceCredential{
		Name:         CredentialName(kubeContext, instance.Namespace, name),
		InstanceName: name,
		Context:      kubeContext,
		Namespace:    instance.Namespace,
		URI:          fmt.Sprintf("%s://127.0.0.1:%d", scheme, port),
		Username:     username,
		Password:     password,
		BoltTLS:      instance.BoltTLS,
	}, nil
}

// CredentialName preserves each scope component. Kubernetes contexts can
// contain colons and slashes, as EKS ARNs do. The credential store accepts them.
func CredentialName(kubeContext, namespace, name string) string {
	return "k8s-" + kubeContext + "-" + namespace + "-" + name
}

// StandalonePod returns a ready server pod and its named Bolt port. Select
// labels rather than a StatefulSet name because pod names are an operator detail.
func StandalonePod(ctx context.Context, client Client, name string) (Pod, error) {
	selector := "app.kubernetes.io/name=neo4j,app.kubernetes.io/instance=" + name +
		",app.kubernetes.io/managed-by=neo4j-operator,neo4j.com/component=workload,neo4j.com/pool=server"
	pods, err := client.ListPods(ctx, selector)
	if err != nil {
		return Pod{}, err
	}
	slices.SortFunc(pods, func(a, b Pod) int { return strings.Compare(a.Name, b.Name) })
	for _, pod := range pods {
		if pod.Ready && !pod.Deleting && pod.Phase == "Running" && pod.BoltPort > 0 {
			return pod, nil
		}
	}
	return Pod{}, clierr.NewUpstreamError(
		"no ready standalone pod found for Neo4j %q. Run `neo4j-cli k8s instance get %s` to inspect readiness", name, name)
}
