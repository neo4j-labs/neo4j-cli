// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package instance

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clicfg/credentials"
	"github.com/neo4j/cli/internal/clicfg/envfile"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/clievents"
	"github.com/neo4j/cli/internal/dbconn"
	engine "github.com/neo4j/cli/internal/k8s"
	"github.com/spf13/cobra"
)

var waitForBoltFn = dbconn.WaitForBolt

func validateLocalPort(port int) error {
	if port < 1 || port > 65535 {
		return clierr.NewUsageError("--port must be between 1 and 65535")
	}
	return nil
}

func storeCredential(cfg *clicfg.Config, credential engine.InstanceCredential) error {
	dbms := cfg.DbmsCredentials()
	if _, err := dbms.Get(credential.Name); err == nil {
		return dbms.Update(credential.Name, credential.Username, credential.Password, dbconn.DefaultDatabase, credential.URI)
	}
	return dbms.Add(credential.Name, credential.Username, credential.Password, dbconn.DefaultDatabase, credential.URI)
}

// removeStoredCredential deletes the credential that `instance credential`
// stored for a resource, if there is one. A failure is only a warning,
// because the Neo4j resource itself is already being deleted.
func removeStoredCredential(cmd *cobra.Command, cfg *clicfg.Config, name string) {
	dbms := cfg.DbmsCredentials()
	if dbms == nil {
		return
	}
	if _, err := dbms.Get(name); err != nil {
		return
	}
	stderr := cmd.ErrOrStderr()
	if err := cfg.Credentials.RemoveDbms(name, stderr); err != nil {
		fmt.Fprintf(stderr, "Warning: could not remove stored credential %q: %s\n", name, err) //nolint:errcheck // Warning output is best effort.
		return
	}
	fmt.Fprintf(stderr, "Removed stored credential %q.\n", name) //nolint:errcheck // Diagnostic output is best effort.
}

func warnUnverifiedTLS(cmd *cobra.Command, credential engine.InstanceCredential) {
	if credential.BoltTLS {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: bolt+ssc encrypts traffic but accepts any server certificate.") //nolint:errcheck // Warning output is best effort.
	}
}

func connectionReady(cmd *cobra.Command, cfg *clicfg.Config, credential engine.InstanceCredential, pod, envOutFile string) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := waitForBoltFn(ctx, credential.URI, credential.Username, credential.Password, dbconn.DefaultWaitTimeout); err != nil {
			var timeout *dbconn.BoltTimeoutError
			if errors.As(err, &timeout) {
				return clierr.NewUpstreamError("Neo4j %q did not accept an authenticated connection through pod %s within %s", credential.InstanceName, pod, timeout.Timeout).
					WithSuggestion("Run `neo4j-cli k8s instance get " + credential.InstanceName + "` with the same Kubernetes scope")
			}
			return err
		}
		contents, err := renderCredentialEnv(credential)
		if err != nil {
			return err
		}
		if envOutFile != "" {
			if err := envfile.Write(cfg.Fs(), envOutFile, contents); err != nil {
				return clierr.NewUsageError("write --env-out-file %q: %s", envOutFile, err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Connection details written to %q.\n", envOutFile) //nolint:errcheck // Connection diagnostics are best effort.
		} else {
			fmt.Fprint(cmd.OutOrStdout(), contents) //nolint:errcheck // Credential output is the command result.
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Connection ready. Press Ctrl-C to stop.") //nolint:errcheck // Connection diagnostics are best effort.
		return nil
	}
}

// renderCredentialEnv writes the connection as a dotenv file for `query --env`.
func renderCredentialEnv(credential engine.InstanceCredential) (string, error) {
	var b strings.Builder
	b.WriteString("# neo4j-cli k8s\n")
	for _, entry := range []struct{ name, value string }{
		{credentials.EnvURI, credential.URI},
		{credentials.EnvUsername, credential.Username},
		{credentials.EnvPassword, credential.Password},
		{credentials.EnvDatabase, dbconn.DefaultDatabase},
	} {
		value, err := envfile.Quote(entry.value)
		if err != nil {
			return "", clierr.NewUsageError("cannot write %s for Neo4j %q to a dotenv file: %s", entry.name, credential.InstanceName, err).
				WithSuggestion("Store the connection with `neo4j-cli k8s instance credential " + credential.InstanceName +
					"`, forward the port with kubectl, then query with --credential")
		}
		if entry.name == credentials.EnvPassword {
			// The quoted form also reaches history and failure logs.
			clievents.RegisterSecretValue(value)
		}
		fmt.Fprintf(&b, "%s=%s\n", entry.name, value)
	}
	return b.String(), nil
}
