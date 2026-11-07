// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package k8s

import (
	"context"
	"strings"
)

// ApplyCRDs implements Client. It server-side applies the operator CRD
// manifest and returns the CRD names kubectl reported.
func (c *execClient) ApplyCRDs(ctx context.Context, source string) ([]string, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlApplyCRDArgs(c.settings, source)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseCRDNames(res.stdout), nil
}

// DeleteCRDs implements Client. It deletes the four operator CRDs by name and
// returns the names kubectl reported deleting; CRDs that are already gone are
// omitted rather than an error.
func (c *execClient) DeleteCRDs(ctx context.Context) ([]string, error) {
	path, err := resolveKubectl()
	if err != nil {
		return nil, err
	}
	args := kubectlDeleteCRDArgs(c.settings)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseCRDNames(res.stdout), nil
}

// HelmUpgrade implements Client.
func (c *execClient) HelmUpgrade(ctx context.Context, opts HelmUpgradeOptions) error {
	path, err := resolveHelm()
	if err != nil {
		return err
	}
	args := helmUpgradeArgs(c.settings, opts)
	res := c.exec(ctx, "helm", path, args)
	if res.err != nil {
		return classifyRunError(ctx, "helm", args, res)
	}
	return nil
}

// HelmUninstall implements Client.
func (c *execClient) HelmUninstall(ctx context.Context) error {
	path, err := resolveHelm()
	if err != nil {
		return err
	}
	args := helmUninstallArgs(c.settings)
	res := c.exec(ctx, "helm", path, args)
	if res.err != nil {
		return classifyRunError(ctx, "helm", args, res)
	}
	return nil
}

// HelmReleases implements Client.
func (c *execClient) HelmReleases(ctx context.Context) ([]HelmRelease, error) {
	path, err := resolveHelm()
	if err != nil {
		return nil, err
	}
	args := helmListArgs(c.settings)
	res := c.exec(ctx, "helm", path, args)
	if res.err != nil {
		return nil, classifyRunError(ctx, "helm", args, res)
	}
	releases, err := parseHelmReleases(res.stdout)
	if err != nil {
		return nil, err
	}
	return releases, nil
}

// CurrentContext implements Client. An explicit --context is the answer
// without asking kubectl.
func (c *execClient) CurrentContext(ctx context.Context) (string, error) {
	if c.settings.Context != "" {
		return c.settings.Context, nil
	}
	path, err := resolveKubectl()
	if err != nil {
		return "", err
	}
	args := kubectlCurrentContextArgs(c.settings)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return "", classifyRunError(ctx, "kubectl", args, res)
	}
	return strings.TrimSpace(res.stdout), nil
}

// Deployment implements Client.
func (c *execClient) Deployment(ctx context.Context, name string) (DeploymentStatus, error) {
	path, err := resolveKubectl()
	if err != nil {
		return DeploymentStatus{}, err
	}
	args := kubectlGetArgs(c.settings, "deployment", name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return DeploymentStatus{}, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseDeployment(res.stdout)
}

// CRD implements Client.
func (c *execClient) CRD(ctx context.Context, name string) (CRDInfo, error) {
	path, err := resolveKubectl()
	if err != nil {
		return CRDInfo{}, err
	}
	args := kubectlGetArgs(c.settings, "crd", name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		return CRDInfo{}, classifyRunError(ctx, "kubectl", args, res)
	}
	return parseCRD(res.stdout)
}

// NamespaceExists implements Client.
func (c *execClient) NamespaceExists(ctx context.Context, name string) (bool, error) {
	path, err := resolveKubectl()
	if err != nil {
		return false, err
	}
	args := kubectlGetArgs(c.settings, "namespace", name)
	res := c.exec(ctx, "kubectl", path, args)
	if res.err != nil {
		classified := classifyRunError(ctx, "kubectl", args, res)
		if IsNotFound(classified) {
			return false, nil
		}
		return false, classified
	}
	return true, nil
}
