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

	"github.com/neo4j/cli/internal/confirm"
	"github.com/neo4j/cli/internal/desktopclient"
	"github.com/spf13/afero"
)

// TestDelete_ConfirmGateFiresBeforeConnect: the confirm gate runs on the RAW
// positional BEFORE any client construction, so non-TTY callers without
// --yes --force fail fast without touching Desktop.
func TestDelete_ConfirmGateFiresBeforeConnect(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	connectCalled := false
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		connectCalled = true
		return nil, nil //nolint:nilnil // unreachable in this test
	}))
	spec := baseSpec(t)

	err := h.run(spec, "widget delete my-widget --yes")
	if err == nil {
		t.Fatalf("expected usage error when --force is missing")
	}
	if !strings.Contains(err.Error(), "pass both --yes and --force") {
		t.Fatalf("expected error to mention 'pass both --yes and --force', got: %v", err)
	}
	if connectCalled {
		t.Fatalf("connect must not run before the confirm gate")
	}
}

// TestDelete_TTYDecline_PromptEchoesRawPositional: the TTY prompt names what
// the user typed (not a resolved id), and declining cancels before any
// Desktop contact.
func TestDelete_TTYDecline_PromptEchoesRawPositional(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return true }))
	h.in.WriteString("n\n")
	connectCalled := false
	t.Cleanup(desktopclient.SetConnectFnForTest(func(context.Context, afero.Fs, int) (*desktopclient.Client, error) {
		connectCalled = true
		return nil, nil //nolint:nilnil // unreachable in this test
	}))
	spec := baseSpec(t)

	err := h.run(spec, "widget delete my-widget --format json")
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected confirm.ErrCancelled on decline; got: %v", err)
	}
	if !strings.Contains(h.err.String(), `widget "my-widget"`) {
		t.Fatalf("expected prompt to echo the typed name; stderr=%q", h.err.String())
	}
	if connectCalled {
		t.Fatalf("connect must not run before the confirm gate")
	}
}

func TestDelete_JSON_SlimEnvelope(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	h.stubConnect()
	spec := baseSpec(t)
	var gotID string
	spec.Resolve = func(_ context.Context, _ *desktopclient.Client, nameOrID string) (string, error) {
		return "w1", nil
	}
	spec.Delete = func(_ context.Context, _ *desktopclient.Client, id string) error {
		gotID = id
		return nil
	}

	if err := h.run(spec, "widget delete my-widget --yes --force --format json"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotID != "w1" {
		t.Fatalf("delete hook got id=%q, want w1", gotID)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(h.out.Bytes()), &got); err != nil {
		t.Fatalf("json out: %v (raw: %s)", err, h.out.String())
	}
	if got["id"] != "w1" || got["deleted"] != true || len(got) != 2 {
		t.Fatalf("expected exactly {id, deleted:true}, got %v", got)
	}
}

func TestDelete_TableConfirmation(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	h.stubConnect()
	spec := baseSpec(t)
	spec.Resolve = func(_ context.Context, _ *desktopclient.Client, nameOrID string) (string, error) {
		return nameOrID, nil
	}
	spec.Delete = func(context.Context, *desktopclient.Client, string) error { return nil }

	if err := h.run(spec, "widget delete "+validFakeID+" --yes --force --format table"); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(h.out.String())
	want := "Deleted widget " + validFakeID + "."
	if got != want {
		t.Fatalf("table confirmation mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestDelete_ResolveError_SurfacesBeforeDelete(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	h.stubConnect()
	spec := baseSpec(t)
	spec.Resolve = func(context.Context, *desktopclient.Client, string) (string, error) {
		return "", errors.New("unknown widget \"nope\"")
	}

	err := h.run(spec, "widget delete nope --yes --force --format json")
	if err == nil || !strings.Contains(err.Error(), "unknown widget") {
		t.Fatalf("expected resolve error to surface, got: %v", err)
	}
}

func TestDelete_HookError_Surfaces(t *testing.T) {
	h := newHarness(t)
	t.Cleanup(confirm.SetStdinIsTerminal(func() bool { return false }))
	h.stubConnect()
	spec := baseSpec(t)
	spec.Resolve = func(_ context.Context, _ *desktopclient.Client, nameOrID string) (string, error) {
		return nameOrID, nil
	}
	spec.Delete = func(context.Context, *desktopclient.Client, string) error {
		return errors.New("delete boom")
	}

	err := h.run(spec, "widget delete "+validFakeID+" --yes --force --format json")
	if err == nil || !strings.Contains(err.Error(), "delete boom") {
		t.Fatalf("expected delete hook error to surface, got: %v", err)
	}
}

func TestDelete_RequiresPositional(t *testing.T) {
	h := newHarness(t)
	h.stubConnect()
	if err := h.run(baseSpec(t), "widget delete --yes --force"); err == nil {
		t.Fatalf("expected error when <widget> is missing")
	}
}
