// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package docker

import (
	"context"
	"fmt"
)

// fakeDockerClient is the in-memory dockerClient used by every leaf test.
// Each call records its args into the corresponding *Calls slice so tests
// can assert on shape; per-method overrides (RunFn, StartFn, …) let a test
// inject custom return values or simulate failures.
type fakeDockerClient struct {
	// Recorded args.
	RunCalls         [][]string
	StartCalls       []string
	StopCalls        []string
	RemoveForceCalls []string
	PsAllCalls       [][]string
	InspectCalls     []string
	ExecCalls        []ExecCall

	// Optional behaviour overrides.
	RunFn         func(ctx context.Context, args []string) (string, error)
	StartFn       func(ctx context.Context, name string) error
	StopFn        func(ctx context.Context, name string) error
	RemoveForceFn func(ctx context.Context, name string) error
	PsAllFn       func(ctx context.Context, filters []string) ([]PsEntry, error)
	InspectFn     func(ctx context.Context, name string) (Container, error)
	ExecFn        func(ctx context.Context, name string, args []string) (string, error)

	// Stored state for default behaviours.
	Containers map[string]Container
	PsEntries  []PsEntry
}

// ExecCall records the arguments of a single fakeDockerClient.Exec invocation
// so tests can assert both the target container name and the argv passed.
type ExecCall struct {
	Name string
	Args []string
}

func newFakeDockerClient() *fakeDockerClient {
	return &fakeDockerClient{
		Containers: map[string]Container{},
	}
}

func (f *fakeDockerClient) Run(ctx context.Context, args []string) (string, error) {
	f.RunCalls = append(f.RunCalls, append([]string(nil), args...))
	if f.RunFn != nil {
		return f.RunFn(ctx, args)
	}
	return "fake-container-id", nil
}

func (f *fakeDockerClient) Start(ctx context.Context, name string) error {
	f.StartCalls = append(f.StartCalls, name)
	if f.StartFn != nil {
		return f.StartFn(ctx, name)
	}
	return nil
}

func (f *fakeDockerClient) Stop(ctx context.Context, name string) error {
	f.StopCalls = append(f.StopCalls, name)
	if f.StopFn != nil {
		return f.StopFn(ctx, name)
	}
	return nil
}

func (f *fakeDockerClient) RemoveForce(ctx context.Context, name string) error {
	f.RemoveForceCalls = append(f.RemoveForceCalls, name)
	if f.RemoveForceFn != nil {
		return f.RemoveForceFn(ctx, name)
	}
	delete(f.Containers, name)
	return nil
}

func (f *fakeDockerClient) PsAll(ctx context.Context, filters []string) ([]PsEntry, error) {
	f.PsAllCalls = append(f.PsAllCalls, append([]string(nil), filters...))
	if f.PsAllFn != nil {
		return f.PsAllFn(ctx, filters)
	}
	return f.PsEntries, nil
}

func (f *fakeDockerClient) Inspect(ctx context.Context, name string) (Container, error) {
	f.InspectCalls = append(f.InspectCalls, name)
	if f.InspectFn != nil {
		return f.InspectFn(ctx, name)
	}
	c, ok := f.Containers[name]
	if !ok {
		// Default-miss matches execClient.Inspect's contract: a missing
		// container is signalled via ErrNotFound, not a bare error. Tests
		// that need to simulate a daemon-down style failure should set
		// InspectFn explicitly.
		return Container{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return c, nil
}

func (f *fakeDockerClient) Exec(ctx context.Context, name string, args []string) (string, error) {
	f.ExecCalls = append(f.ExecCalls, ExecCall{Name: name, Args: append([]string(nil), args...)})
	if f.ExecFn != nil {
		return f.ExecFn(ctx, name, args)
	}
	return "", nil
}

// Compile-time check that fakeDockerClient satisfies the dockerClient
// interface so a refactor of the interface fails at compile time, not in
// the next test that drives a leaf.
var _ dockerClient = (*fakeDockerClient)(nil)
