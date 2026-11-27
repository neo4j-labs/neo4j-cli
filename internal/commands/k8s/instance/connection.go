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

func warnUnverifiedTLS(cmd *cobra.Command, credential engine.InstanceCredential) {
	if credential.BoltTLS {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: bolt+ssc encrypts traffic but accepts any server certificate.") //nolint:errcheck // Warning output is best effort.
	}
}

func connectionReady(cmd *cobra.Command, cfg *clicfg.Config, credential engine.InstanceCredential, pod, envOutFile string) func(context.Context, int) error {
	return func(ctx context.Context, _ int) error {
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

// envQuote escapes the characters that the dotenv reader treats as special
// inside double quotes.
var envQuote = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`)

// envValue quotes one value for the dotenv reader in envfile.Parse. A
// single-quoted value is literal, so it keeps dollar signs, double quotes and
// backslashes unchanged. The reader decodes line-ending escapes only inside
// double quotes, and there it rewrites a backslash before n or r. It also reads
// a closing quote after a backslash as escaped, in either quoting form. A value
// that breaks these rules has no faithful spelling.
func envValue(value string) (string, error) {
	if strings.HasSuffix(value, `\`) {
		return "", errors.New("a trailing backslash has no faithful dotenv spelling")
	}
	if !strings.ContainsAny(value, "'\n\r") {
		return "'" + value + "'", nil
	}
	if strings.Contains(value, `\n`) || strings.Contains(value, `\r`) {
		return "", errors.New(`a single quote or line ending cannot be combined with a literal \n or \r`)
	}
	return `"` + envQuote.Replace(value) + `"`, nil
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
		value, err := envValue(entry.value)
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
