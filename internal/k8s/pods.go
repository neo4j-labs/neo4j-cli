// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"encoding/json"

	"github.com/neo4j/cli/internal/clierr"
)

// Pod carries the fields that connection setup needs. A zero BoltPort means
// that the pod declares no port named bolt.
type Pod struct {
	Name     string
	Phase    string
	Ready    bool
	Deleting bool
	BoltPort int32
}

// ListPods implements Client. The caller supplies the instance and pool
// labels. A pod name alone does not identify an operator workload reliably.
func (c *execClient) ListPods(ctx context.Context, selector string) ([]Pod, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlArgs(c.settings, "get", "pods", "-l", selector, "-o", "json")
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "kubectl", args, res)
	}
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &list); err != nil {
		return nil, clierr.NewFatalError("parse pod list output: %s", err)
	}
	pods := make([]Pod, 0, len(list.Items))
	for _, item := range list.Items {
		pod, err := parsePod(item)
		if err != nil {
			return nil, err
		}
		pods = append(pods, pod)
	}
	return pods, nil
}

func parsePod(data []byte) (Pod, error) {
	var raw struct {
		Metadata struct {
			Name              string `json:"name"`
			DeletionTimestamp string `json:"deletionTimestamp"`
		} `json:"metadata"`
		Spec struct {
			Containers []struct {
				Ports []struct {
					Name string `json:"name"`
					Port int32  `json:"containerPort"`
				} `json:"ports"`
			} `json:"containers"`
		} `json:"spec"`
		Status struct {
			Phase      string `json:"phase"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Pod{}, clierr.NewFatalError("parse pod output: %s", err)
	}
	pod := Pod{
		Name:     raw.Metadata.Name,
		Phase:    raw.Status.Phase,
		Deleting: raw.Metadata.DeletionTimestamp != "",
	}
	for _, condition := range raw.Status.Conditions {
		if condition.Type == "Ready" {
			pod.Ready = condition.Status == "True"
		}
	}
	for _, container := range raw.Spec.Containers {
		for _, port := range container.Ports {
			if port.Name == "bolt" {
				pod.BoltPort = port.Port
			}
		}
	}
	return pod, nil
}
