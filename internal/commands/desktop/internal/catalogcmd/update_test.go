// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/afero"
)

func TestUpdate_HappyPath_ResolvesUpdatesAndPrints(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := withColor(baseSpec(t))
	var gotID, gotName, gotColor string
	spec.Resolve = func(_ context.Context, _ *desktopclient.Client, nameOrID string) (string, error) {
		if nameOrID != "my-widget" {
			t.Errorf("resolve got %q, want my-widget", nameOrID)
		}
		return "w1", nil
	}
	spec.Update = func(_ context.Context, _ *desktopclient.Client, id, name, color string) (*fakeState, error) {
		gotID, gotName, gotColor = id, name, color
		return &fakeState{entries: []fakeEntry{{ID: "w1", Name: "renamed", Color: "7"}}}, nil
	}
	spec.FindByID = findByID

	if err := h.run(spec, "widget update my-widget --name renamed --color 7 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotID != "w1" || gotName != "renamed" || gotColor != "7" {
		t.Fatalf("update hook got id=%q name=%q color=%q", gotID, gotName, gotColor)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "w1" || got["name"] != "renamed" || got["color"] != "7" {
		t.Fatalf("unexpected update output: %v", got)
	}
}

func TestUpdate_RequiresNameFlag(t *testing.T) {
	h := newHarness(t)
	connectCalled := false
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		connectCalled = true
		return nil, nil //nolint:nilnil // unreachable in this test
	}))
	spec := baseSpec(t)

	err := h.run(spec, "widget update "+validFakeID)
	if err == nil {
		t.Fatalf("expected required-flag error when --name is missing")
	}
	if !strings.Contains(err.Error(), `"name"`) {
		t.Fatalf("expected error to mention \"name\", got: %v", err)
	}
	if connectCalled {
		t.Fatalf("connect must not run when --name is missing")
	}
}

func TestUpdate_RequiresPositional(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	if err := h.run(baseSpec(t), "widget update --name renamed"); err == nil {
		t.Fatalf("expected error when <widget> is missing")
	}
}

func TestUpdate_ResolveError_SurfacesBeforeUpdate(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.Resolve = func(context.Context, *desktopclient.Client, string) (string, error) {
		return "", errors.New("unknown widget \"nope\"")
	}

	err := h.run(spec, "widget update nope --name renamed --format json")
	if err == nil || !strings.Contains(err.Error(), "unknown widget") {
		t.Fatalf("expected resolve error to surface, got: %v", err)
	}
}

func TestUpdate_FinderError_SurfacesFatalAndPrintsNoNull(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.Resolve = func(context.Context, *desktopclient.Client, string) (string, error) {
		return "w1", nil
	}
	spec.Update = func(context.Context, *desktopclient.Client, string, string, string) (*fakeState, error) {
		return &fakeState{}, nil
	}
	spec.FindByID = findByID

	err := h.run(spec, "widget update "+validFakeID+" --name renamed --format json")
	if err == nil || !strings.Contains(err.Error(), "missing from the catalog state") {
		t.Fatalf("expected finder error to surface, got: %v", err)
	}
	if strings.Contains(h.out.String(), "null") {
		t.Fatalf("must not print null on failure, got:\n%s", h.out.String())
	}
}

func TestUpdate_ColorValidatedBeforeConnect(t *testing.T) {
	h := newHarness(t)
	connectCalled := false
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		connectCalled = true
		return nil, nil //nolint:nilnil // unreachable in this test
	}))
	spec := withColor(baseSpec(t))

	err := h.run(spec, "widget update "+validFakeID+" --name x --color blue")
	if err == nil {
		t.Fatalf("expected usage error for --color blue")
	}
	if !strings.Contains(err.Error(), "--color") {
		t.Fatalf("expected error to mention --color, got: %v", err)
	}
	if connectCalled {
		t.Fatalf("connect must not run when --color is invalid")
	}
}
