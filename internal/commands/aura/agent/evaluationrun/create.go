// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package evaluationrun

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neo4j/cli/internal/auraclient"
	"github.com/neo4j/cli/internal/clicfg"
	"github.com/neo4j/cli/internal/clierr"
	"github.com/neo4j/cli/internal/commands/aura/output"
	"github.com/neo4j/cli/internal/commands/aura/utils"
	commonflags "github.com/neo4j/cli/internal/flags"
	"github.com/spf13/cobra"
)

// evaluationMetrics are the metric names the API accepts.
var evaluationMetrics = []string{"accuracy", "tool_trajectory", "groundedness", "relevance"}

func newCreateCmd(cfg *clicfg.Config) *cobra.Command {
	var (
		agentID   string
		datasetID string
		metrics   []string
		wait      bool
	)

	const (
		agentIDFlag   = "agent-id"
		datasetIDFlag = "dataset-id"
		metricFlag    = "metric"
	)

	cmd := &cobra.Command{
		Annotations: map[string]string{"write": "true"},
		Use:         "create",
		Short:       "Creates a new evaluation run",
		Long: `Starts an evaluation run of an agent against an evaluation dataset.

At least one --metric is required; repeat the flag to evaluate several metrics. Valid metrics are accuracy, tool_trajectory, groundedness and relevance.

Creating an evaluation run is an asynchronous operation that can be waited for with --wait: without it the command returns immediately with the accepted run (status queued or running), while with it the command blocks until the run reaches a terminal status and prints the finished run.`,
		Example: `# Start a run evaluating accuracy of an agent against a dataset
neo4j-cli aura agent evaluation-run create --agent-id 00000000-0000-0000-0000-000000000000 --dataset-id 11111111-1111-1111-1111-111111111111 --metric accuracy --rw

# Start a run with several metrics and wait until it finishes
neo4j-cli aura agent evaluation-run create --agent-id 00000000-0000-0000-0000-000000000000 --dataset-id 11111111-1111-1111-1111-111111111111 --metric accuracy --metric groundedness --wait --rw

# Start a run and emit the accepted record as JSON for scripting
neo4j-cli aura agent evaluation-run create --agent-id 00000000-0000-0000-0000-000000000000 --dataset-id 11111111-1111-1111-1111-111111111111 --metric relevance --rw --format json`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			organizationId, projectId, err := utils.ResolveOrgProject(cmd, cfg)
			if err != nil {
				return err
			}

			cmd.SilenceUsage = true
			if err := validateMetrics(metrics); err != nil {
				return err
			}

			scope := auraclient.Scope{OrgID: organizationId, ProjectID: projectId}
			runs := auraclient.New(cfg).EvaluationRuns()
			run, err := runs.Create(cmd.Context(), scope, auraclient.EvaluationRunSpec{
				AgentID:   agentID,
				DatasetID: datasetID,
				Metrics:   metrics,
			})
			if err != nil {
				return err
			}

			if !wait {
				output.PrintRecord(cmd, cfg, run.Record, []string{"id", "agent_id", "dataset_id", "status", "created_at"})

				return nil
			}

			// An empty id would make the poller build the collection path
			// instead of the run path, so refuse to wait rather than poll the
			// wrong resource.
			if run.ID == "" {
				return clierr.NewUpstreamError("create response did not carry an evaluation run id, cannot wait")
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Waiting for evaluation run %s to finish...\n", run.ID) //nolint:errcheck // narration to stderr; write errors are not actionable

			status, err := runs.WaitWhileRunning(cmd.Context(), scope, run.ID)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Evaluation Run Status: %s\n", status) //nolint:errcheck // narration to stderr; write errors are not actionable

			// Re-fetch so the printed record carries the aggregate scores (and,
			// on JSON output, the per-question results) rather than the queued
			// snapshot the create call accepted.
			finished, err := runs.Get(cmd.Context(), scope, run.ID)
			if err != nil {
				return err
			}
			output.PrintRecord(cmd, cfg, finished.Record, []string{"id", "agent_id", "dataset_id", "status", "created_at"})

			return nil
		},
	}

	cmd.Flags().StringVar(&agentID, agentIDFlag, "", "(required) The ID of the agent to evaluate")
	cmd.Flags().StringVar(&datasetID, datasetIDFlag, "", "(required) The ID of the evaluation dataset to evaluate the agent against")
	cmd.Flags().StringArrayVar(&metrics, metricFlag, nil, "(required) The metric to evaluate; repeat the flag to evaluate several. Valid metrics: accuracy, tool_trajectory, groundedness, relevance")

	for _, f := range []string{agentIDFlag, datasetIDFlag} {
		cmd.MarkFlagRequired(f) //nolint:errcheck // MarkFlagRequired only errors if the flag name does not exist, which is a programming error caught at startup
	}

	commonflags.RegisterWait(cmd, &wait, "Waits until the evaluation run reaches a terminal status and prints the finished run.")

	return cmd
}

// validateMetrics rejects an unknown metric, and an empty selection, before any
// request is made.
func validateMetrics(metrics []string) error {
	if len(metrics) == 0 {
		return clierr.NewUsageError("at least one --metric is required; pass --metric with one of: %s", strings.Join(evaluationMetrics, ", "))
	}
	for _, metric := range metrics {
		if !slices.Contains(evaluationMetrics, metric) {
			return clierr.NewUsageError("invalid metric %q: must be one of: %s", metric, strings.Join(evaluationMetrics, ", "))
		}
	}

	return nil
}
