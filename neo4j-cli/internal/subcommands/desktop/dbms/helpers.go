// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package dbms

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/neo4j/cli/common/clierr"
	"github.com/neo4j/cli/neo4j-cli/internal/desktopclient"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// newDesktopClient probes the port range, resolves the data dir, loads the
// salt, and signs the JWT. A probe miss or missing/unreadable salt (Desktop
// hasn't finished first-run auth setup) both surface as the canonical
// "Desktop unreachable" hint.
var newDesktopClientFn = newDesktopClient

func newDesktopClient(ctx context.Context, fs afero.Fs, port int) (*desktopclient.Client, error) {
	// ProbePort runs first so its origin can feed ResolveDataDir's /info/app
	// discovery step.
	probe, err := desktopclient.ProbePort(ctx, port)
	if err != nil {
		if errors.Is(err, desktopclient.ErrNoDesktop) {
			return nil, desktopclient.UnreachableError()
		}
		return nil, clierr.NewFatalError("desktop: probe failed: %s", err.Error())
	}
	dataDir, err := desktopclient.ResolveDataDir(ctx, fs, probe)
	if err != nil {
		return nil, clierr.NewFatalError("desktop: could not resolve relate data dir: %s", err.Error())
	}
	salt, err := desktopclient.LoadSalt(fs, dataDir)
	if err != nil {
		// Missing/unreadable salt ⇒ Desktop hasn't finished first-run auth
		// setup; route through the same "unreachable" hint as a probe miss.
		return nil, desktopclient.UnreachableError()
	}
	return desktopclient.NewClient(probe, salt)
}

// SetNewDesktopClientFnForTest overrides the shared client constructor for tests.
func SetNewDesktopClientFnForTest(fn func(context.Context, afero.Fs, int) (*desktopclient.Client, error)) func() {
	prev := newDesktopClientFn
	newDesktopClientFn = fn
	return func() { newDesktopClientFn = prev }
}

type statusPoller interface {
	GetDbms(ctx context.Context, id string) (*desktopclient.DbmsInfo, error)
}

// pollUntilStatus polls GetDbms every createPollInterval up to createPollTimeout,
// returning the first response with Status==target. Timeout error carries the
// last-seen status. Context cancellation aborts the poll.
func pollUntilStatus(ctx context.Context, client statusPoller, id, target string) (*desktopclient.DbmsInfo, error) {
	deadline := createNowFn().Add(createPollTimeout)
	var lastStatus string
	for {
		info, err := client.GetDbms(ctx, id)
		if err != nil {
			return nil, err
		}
		if info.Status == target {
			return info, nil
		}
		lastStatus = info.Status
		if createNowFn().After(deadline) {
			return nil, clierr.NewFatalError(
				"timed out after %s waiting for DBMS %q to reach status %q (last status: %q). "+
					"Re-run without waiting, or check `neo4j-cli desktop list` for the current status.",
				createPollTimeout, id, target, lastStatus)
		}
		if err := ctxSleep(ctx, createPollInterval); err != nil {
			return nil, err
		}
	}
}

// fetchForRender enriches the rendered row after a start/stop call.
// Desktop's lifecycle endpoints return stringified shell output rather than a
// DbmsInfo. On fetch failure it warns to stderr and returns (nil, false) so
// the caller falls back to the slim `{id}` envelope (lifecycle call already
// succeeded).
func fetchForRender(ctx context.Context, client statusPoller, id string, warnOut io.Writer) (*desktopclient.DbmsInfo, bool) {
	info, err := client.GetDbms(ctx, id)
	if err != nil {
		_, _ = fmt.Fprintf(warnOut, "Warning: failed to fetch details for DBMS %q after lifecycle call: %s\n", id, err.Error())
		return nil, false
	}
	return info, true
}

// Status strings Desktop reports on `GET /dbmss/:id`. Live-smoke against
// Desktop 2.1.4 surfaced these — do NOT reintroduce `online`/`offline` aliases
// guessed from TS sources; real Desktop never emits those.
const (
	dbmsStatusStarted = "started"
	dbmsStatusStopped = "stopped"
)

// Bounded fan-out for slim→full enrichment so a user with 50+ DBMSes doesn't
// open 50+ sockets at once.
const listEnrichConcurrency = 8

// ListEnrichClient is the abstraction EnrichDbmsList needs; reused by the
// composed `desktop list` view.
type ListEnrichClient interface {
	GetDbms(ctx context.Context, id string) (*desktopclient.DbmsInfo, error)
}

// DbmsListFields is the default column order for the Local DBMSes table.
var DbmsListFields = []string{"id", "name", "version", "status", "connectionUri"}

type preflightClient interface {
	ListEnrichClient
	ListDbmss(ctx context.Context) ([]desktopclient.DbmsInfo, error)
}

type resolveClient interface {
	preflightClient
	StopDbms(ctx context.Context, id string) error
}

// EnrichDbmsList fans out one GetDbms per entry (bounded) and merges the full
// payload into the slim list shape. Output preserves input order regardless of
// goroutine completion order. Per-entry failures are non-fatal — the slim row
// is kept and a stderr warning is emitted.
func EnrichDbmsList(ctx context.Context, client ListEnrichClient, items []desktopclient.DbmsInfo, warnOut io.Writer) []desktopclient.DbmsInfo {
	if len(items) == 0 {
		return items
	}
	enriched := make([]desktopclient.DbmsInfo, len(items))
	copy(enriched, items)

	// Group error is never surfaced: per-entry failures are tolerated.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(listEnrichConcurrency)

	var warnMu sync.Mutex
	for i := range items {
		i := i
		id := items[i].ID
		g.Go(func() error {
			info, err := client.GetDbms(gctx, id)
			if err != nil {
				warnMu.Lock()
				_, _ = fmt.Fprintf(warnOut, "Warning: failed to fetch details for DBMS %q: %s\n", id, err.Error())
				warnMu.Unlock()
				return nil
			}
			if info != nil {
				enriched[i] = *info
			}
			return nil
		})
	}
	_ = g.Wait()
	return enriched
}

// assertNoOtherRunning enforces Desktop's one-DBMS-at-a-time invariant before
// a Start/Create call. Desktop silently refuses to start a second DBMS on port
// 7687, so without this pre-flight `--wait` would time out 30s later with a
// confusing "last status: stopped". Pass selfID="" for create (no DBMS yet).
//
// A slim row from a failed enrichment has empty Status so it can't false-
// positive block.
func assertNoOtherRunning(ctx context.Context, client preflightClient, selfID, selfName, actionVerb string, warnOut io.Writer) error {
	items, err := client.ListDbmss(ctx)
	if err != nil {
		return err
	}
	enriched := EnrichDbmsList(ctx, client, items, warnOut)
	for _, info := range enriched {
		if info.Status != dbmsStatusStarted {
			continue
		}
		if info.ID == selfID {
			// Same-ID already running ⇒ idempotent no-op for start.
			continue
		}
		return clierr.NewFatalError("%s", buildConflictMessage(selfID, selfName, info, actionVerb))
	}
	return nil
}

// resolveConflicting is the --force sibling of assertNoOtherRunning: stop the
// conflicting DBMS (if any), poll until stopped, return the stopped id. Without
// this --force would let Desktop silently no-op the second start. A stderr
// breadcrumb is emitted exactly once when something is actually stopped.
func resolveConflicting(ctx context.Context, client resolveClient, selfID string, warnOut io.Writer) (string, error) {
	items, err := client.ListDbmss(ctx)
	if err != nil {
		return "", err
	}
	enriched := EnrichDbmsList(ctx, client, items, warnOut)
	for _, info := range enriched {
		if info.Status != dbmsStatusStarted {
			continue
		}
		if info.ID == selfID {
			continue
		}
		ref := formatDbmsRef(info.Name, info.ID)
		_, _ = fmt.Fprintf(warnOut, "Stopping DBMS %s to free port 7687...\n", ref)
		if err := client.StopDbms(ctx, info.ID); err != nil {
			return "", clierr.NewFatalError(
				"failed to stop conflicting DBMS %s: %s",
				ref, err.Error(),
			)
		}
		if _, perr := pollUntilStatus(ctx, client, info.ID, dbmsStatusStopped); perr != nil {
			return "", clierr.NewFatalError(
				"failed waiting for conflicting DBMS %s to stop: %s",
				ref, perr.Error(),
			)
		}
		return info.ID, nil
	}
	return "", nil
}

// buildConflictMessage renders the canonical "another DBMS is running" error text.
func buildConflictMessage(selfID, selfName string, other desktopclient.DbmsInfo, actionVerb string) string {
	var b bytes.Buffer
	self := selfDescriptor(selfID, selfName)
	otherDesc := formatDbmsRef(other.Name, other.ID)
	fmt.Fprintf(&b,
		"Cannot %s DBMS %s: DBMS %s is currently running. "+
			"Neo4j Desktop 2 runs one DBMS at a time on port 7687. "+
			"Stop the other first with 'neo4j-cli desktop dbms stop %s', "+
			"or pass --force to skip this check.",
		actionVerb, self, otherDesc, other.ID,
	)
	return b.String()
}

// selfDescriptor formats the "self" DBMS reference: start knows id, create
// knows name. Both paths pass whatever the caller has.
func selfDescriptor(selfID, selfName string) string {
	switch {
	case selfID != "" && selfName != "":
		return fmt.Sprintf("%q (%s)", selfName, selfID)
	case selfID != "":
		return fmt.Sprintf("%q", selfID)
	case selfName != "":
		return fmt.Sprintf("%q", selfName)
	default:
		return "(unknown)"
	}
}

// formatDbmsRef renders `"<name>" (<id>)` or `"<id>"` when no name is present.
func formatDbmsRef(name, id string) string {
	if name == "" {
		return fmt.Sprintf("%q", id)
	}
	return fmt.Sprintf("%q (%s)", name, id)
}

// ctxSleep is the cancel-aware sleep shared by the create/start/stop poll loops.
func ctxSleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		createPollSleepFn(d)
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
