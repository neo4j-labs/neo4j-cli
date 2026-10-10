// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package catalogcmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/afero"
)

func TestCreate_HappyPath_JSON(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	var gotName, gotColor string
	spec.Create = func(_ context.Context, _ *desktopclient.Client, name, color string) (*fakeState, error) {
		gotName, gotColor = name, color
		return &fakeState{entries: []fakeEntry{{ID: "w1", Name: "my-widget"}}}, nil
	}
	spec.FindByName = findByName

	if err := h.run(spec, "widget create my-widget --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotName != "my-widget" || gotColor != "" {
		t.Fatalf("create hook got name=%q color=%q, want my-widget/''", gotName, gotColor)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "w1" || got["name"] != "my-widget" {
		t.Fatalf("unexpected create output: %v", got)
	}
}

func TestCreate_FinderError_SurfacesFatalAndPrintsNoNull(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.Create = func(context.Context, *desktopclient.Client, string, string) (*fakeState, error) {
		return &fakeState{}, nil
	}
	spec.FindByName = findByName

	err := h.run(spec, "widget create my-widget --format json")
	if err == nil || !strings.Contains(err.Error(), "missing from the catalog state") {
		t.Fatalf("expected finder error to surface, got: %v", err)
	}
	if strings.Contains(h.out.String(), "null") {
		t.Fatalf("must not print null on failure, got:\n%s", h.out.String())
	}
}

func TestCreate_RequiresName(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	if err := h.run(baseSpec(t), "widget create"); err == nil {
		t.Fatalf("expected error when <name> is missing")
	}
}

func TestCreate_ColorValidatedBeforeConnect(t *testing.T) {
	h := newHarness(t)
	connectCalled := false
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		connectCalled = true
		return nil, nil //nolint:nilnil // unreachable in this test
	}))
	spec := withColor(baseSpec(t))

	err := h.run(spec, "widget create my-widget --color 99")
	if err == nil {
		t.Fatalf("expected usage error for --color 99")
	}
	if !strings.Contains(err.Error(), "--color") ||
		!strings.Contains(err.Error(), "1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12") {
		t.Fatalf("expected palette listing in error, got: %v", err)
	}
	if connectCalled {
		t.Fatalf("connect must not run when --color is invalid")
	}
}

func TestCreate_ColorForwarded(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := withColor(baseSpec(t))
	var gotColor string
	spec.Create = func(_ context.Context, _ *desktopclient.Client, _, color string) (*fakeState, error) {
		gotColor = color
		return &fakeState{entries: []fakeEntry{{ID: "w1", Name: "my-widget", Color: "5"}}}, nil
	}
	spec.FindByName = findByName

	if err := h.run(spec, "widget create my-widget --color 5 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotColor != "5" {
		t.Fatalf("expected --color 5 forwarded to the create hook, got %q", gotColor)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["color"] != "5" {
		t.Fatalf("expected color in output, got %v", got)
	}
}

func TestCreate_NoColorFlagWithoutColorSpec(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	err := h.run(baseSpec(t), "widget create my-widget --color 5")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown-flag error for --color on a colorless spec, got: %v", err)
	}
}
