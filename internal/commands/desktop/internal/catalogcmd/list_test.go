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

func TestList_JSON_RendersRowJSONArray(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return []fakeEntry{{ID: "w1", Name: "alpha"}, {ID: "w2", Name: "beta"}}, nil
	}

	if err := h.run(spec, "widget list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if len(got) != 2 || got[0]["id"] != "w1" || got[1]["name"] != "beta" {
		t.Fatalf("unexpected list output: %v", got)
	}
}

func TestList_EmptyJSON_RendersEmptyArrayNotNull(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return nil, nil
	}

	if err := h.run(spec, "widget list --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if trimmed := strings.TrimSpace(h.out.String()); trimmed != "[]" {
		t.Fatalf("expected `[]` for an empty list, got %q", trimmed)
	}
}

func TestList_Table_RendersHeaderAndStrippedRows(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return []fakeEntry{{ID: "w1", Name: "al\x1b[31mpha"}}, nil
	}

	if err := h.run(spec, "widget list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	for _, col := range []string{"ID", "NAME"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
	if !strings.Contains(out, "al?[31mpha") {
		t.Fatalf("expected stripped cell value in table, got:\n%s", out)
	}
	if strings.Contains(out, "\x1b") {
		t.Fatalf("control bytes must be stripped from table cells, got:\n%q", out)
	}
}

func TestList_EmptyTable_RendersNonePlaceholder(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return nil, nil
	}

	if err := h.run(spec, "widget list --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := h.out.String()
	if !strings.Contains(out, "(none)") {
		t.Fatalf("expected `(none)` placeholder in empty table, got:\n%s", out)
	}
	for _, col := range []string{"ID", "NAME"} {
		if !strings.Contains(strings.ToUpper(out), col) {
			t.Fatalf("expected column %q in header, got:\n%s", col, out)
		}
	}
}

func TestList_ConnectError_Surfaces(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		return nil, errors.New("boom")
	}))
	spec := baseSpec(t)

	err := h.run(spec, "widget list --format json")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected connect error to surface, got: %v", err)
	}
}

func TestList_HookError_Surfaces(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return nil, errors.New("list boom")
	}

	err := h.run(spec, "widget list --format json")
	if err == nil || !strings.Contains(err.Error(), "list boom") {
		t.Fatalf("expected list hook error to surface, got: %v", err)
	}
}

func TestList_PortFlagPropagatesToClientConstructor(t *testing.T) {
	h := newHarness(t)
	seen := h.stubConnect()
	spec := baseSpec(t)
	spec.List = func(context.Context, *desktopclient.Client) ([]fakeEntry, error) {
		return nil, nil
	}

	if err := h.run(spec, "widget list --port 44225 --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if *seen != 44225 {
		t.Fatalf("expected --port=44225 to reach the client constructor, got %d", *seen)
	}
}

func TestList_RejectsPositional(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	spec := baseSpec(t)

	if err := h.run(spec, "widget list extra"); err == nil {
		t.Fatalf("expected error when a positional is supplied")
	}
}
