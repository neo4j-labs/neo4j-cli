// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package desktopclient

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// stubMDNS wires the mdnsBrowseFn seam to report the given port (ok==true when
// hit), restoring it via t.Cleanup.
func stubMDNS(t *testing.T, port int, ok bool) {
	t.Helper()
	t.Cleanup(SetMDNSBrowseFnForTest(func(_ context.Context) (int, bool) {
		return port, ok
	}))
}

// stubDNSSD wires the dnssdLookupFn seam to report the given port (ok==true
// when hit), restoring it via t.Cleanup.
func stubDNSSD(t *testing.T, port int, ok bool) {
	t.Helper()
	t.Cleanup(SetDNSSDLookupFnForTest(func(_ context.Context) (int, bool) {
		return port, ok
	}))
}

// stubGOOS pins goosFn to the given value, restoring it via t.Cleanup.
func stubGOOS(t *testing.T, goos string) {
	t.Helper()
	t.Cleanup(SetGOOSFnForTest(func() string { return goos }))
}

// TestDiscover_Tiers drives the full Discover orchestrator across every tier
// purely through the mdnsBrowseFn / dnssdLookupFn / goosFn / httpClientFn
// seams — no real multicast, dns-sd, or network. Origin assertions verify the
// auth-coupled host: 127.0.0.1 for mDNS/dns-sd, localhost for the port scan.
func TestDiscover_Tiers(t *testing.T) {
	cases := []struct {
		name string
		// goos selects whether the dns-sd tier is consulted.
		goos string
		// mDNS multicast tier result.
		mdnsPort int
		mdnsOK   bool
		// dns-sd tier result.
		dnssdPort int
		dnssdOK   bool
		// portScanHits is the set of ports the legacy port-scan leg answers 200
		// on; nil/empty = no responder.
		portScanHits map[int]bool
		wantPort     int
		wantOrigin   string
		wantErr      error
	}{
		{
			name:       "mDNS multicast hit",
			goos:       "darwin",
			mdnsPort:   49500,
			mdnsOK:     true,
			wantPort:   49500,
			wantOrigin: "http://127.0.0.1:49500",
		},
		{
			name:       "mDNS miss + dns-sd hit on darwin",
			goos:       "darwin",
			mdnsOK:     false,
			dnssdPort:  49600,
			dnssdOK:    true,
			wantPort:   49600,
			wantOrigin: "http://127.0.0.1:49600",
		},
		{
			name: "non-darwin skips dns-sd, falls to port scan",
			goos: "linux",
			// A dns-sd hit is wired but MUST NOT be consulted on linux; the
			// port scan is what should answer.
			mdnsOK:       false,
			dnssdPort:    49600,
			dnssdOK:      true,
			portScanHits: map[int]bool{ProbePortStart: true},
			wantPort:     ProbePortStart,
			wantOrigin:   fmt.Sprintf("http://localhost:%d", ProbePortStart),
		},
		{
			name:         "all mDNS/dns-sd miss + port-scan fallback",
			goos:         "darwin",
			mdnsOK:       false,
			dnssdOK:      false,
			portScanHits: map[int]bool{44225: true},
			wantPort:     44225,
			wantOrigin:   "http://localhost:44225",
		},
		{
			name:         "all tiers miss returns ErrNoDesktop",
			goos:         "darwin",
			mdnsOK:       false,
			dnssdOK:      false,
			portScanHits: map[int]bool{},
			wantErr:      ErrNoDesktop,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubGOOS(t, tc.goos)
			stubMDNS(t, tc.mdnsPort, tc.mdnsOK)
			stubDNSSD(t, tc.dnssdPort, tc.dnssdOK)
			pinProbeTo(t, "localhost", tc.portScanHits)

			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			got, err := Discover(ctx, 0)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Discover err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if got.Port != tc.wantPort {
				t.Fatalf("got.Port = %d, want %d", got.Port, tc.wantPort)
			}
			if got.Origin != tc.wantOrigin {
				t.Fatalf("got.Origin = %q, want %q", got.Origin, tc.wantOrigin)
			}
		})
	}
}

// TestDiscover_NonDarwinNeverConsultsDNSSD asserts the dns-sd tier is not
// invoked at all on a non-darwin platform — the seam panics if reached.
func TestDiscover_NonDarwinNeverConsultsDNSSD(t *testing.T) {
	stubGOOS(t, "linux")
	stubMDNS(t, 0, false)
	t.Cleanup(SetDNSSDLookupFnForTest(func(_ context.Context) (int, bool) {
		t.Fatalf("dnssdLookupFn must not be consulted on non-darwin")
		return 0, false
	}))
	pinProbeTo(t, "localhost", map[int]bool{ProbePortStart: true})

	got, err := Discover(context.Background(), 0)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got.Port != ProbePortStart {
		t.Fatalf("got.Port = %d, want %d", got.Port, ProbePortStart)
	}
}

// TestDiscover_Pinned covers the pinned-port branches: an mDNS confirm of the
// pinned port adopts the 127.0.0.1 origin; a silent mDNS/dns-sd falls back to
// the HTTP port probe and the localhost origin.
func TestDiscover_Pinned(t *testing.T) {
	const pinned = 47000

	t.Run("mDNS confirms pinned port", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, pinned, true)
		stubDNSSD(t, 0, false)
		// Port scan would answer too, but the mDNS confirm must win with a
		// 127.0.0.1 origin.
		pinProbeTo(t, "localhost", map[int]bool{pinned: true})

		got, err := Discover(context.Background(), pinned)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if got.Port != pinned {
			t.Fatalf("got.Port = %d, want %d", got.Port, pinned)
		}
		want := fmt.Sprintf("http://127.0.0.1:%d", pinned)
		if got.Origin != want {
			t.Fatalf("got.Origin = %q, want %q", got.Origin, want)
		}
	})

	t.Run("mDNS silent falls to port probe localhost origin", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 0, false)
		stubDNSSD(t, 0, false)
		pinProbeTo(t, "localhost", map[int]bool{pinned: true})

		got, err := Discover(context.Background(), pinned)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if got.Port != pinned {
			t.Fatalf("got.Port = %d, want %d", got.Port, pinned)
		}
		want := fmt.Sprintf("http://localhost:%d", pinned)
		if got.Origin != want {
			t.Fatalf("got.Origin = %q, want %q", got.Origin, want)
		}
	})

	t.Run("mDNS reports different port falls to port probe", func(t *testing.T) {
		stubGOOS(t, "darwin")
		// mDNS sees a responder on a DIFFERENT port — must not be adopted for
		// the pinned request; falls through to the HTTP probe of the pinned port.
		stubMDNS(t, pinned+1, true)
		stubDNSSD(t, 0, false)
		pinProbeTo(t, "localhost", map[int]bool{pinned: true})

		got, err := Discover(context.Background(), pinned)
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		want := fmt.Sprintf("http://localhost:%d", pinned)
		if got.Origin != want {
			t.Fatalf("got.Origin = %q, want %q", got.Origin, want)
		}
	})
}

// TestDiscoverViaMDNS covers the mDNS tier (multicast + the darwin dns-sd
// fallback) in isolation.
func TestDiscoverViaMDNS(t *testing.T) {
	t.Run("multicast hit yields 127.0.0.1 origin", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 49700, true)
		stubDNSSD(t, 0, false)
		got, err := DiscoverViaMDNS(context.Background())
		if err != nil {
			t.Fatalf("DiscoverViaMDNS: %v", err)
		}
		if got.Port != 49700 {
			t.Fatalf("got.Port = %d, want 49700", got.Port)
		}
		if got.Origin != "http://127.0.0.1:49700" {
			t.Fatalf("got.Origin = %q", got.Origin)
		}
	})

	t.Run("multicast miss + dns-sd hit on darwin", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 0, false)
		stubDNSSD(t, 49800, true)
		got, err := DiscoverViaMDNS(context.Background())
		if err != nil {
			t.Fatalf("DiscoverViaMDNS: %v", err)
		}
		if got.Port != 49800 {
			t.Fatalf("got.Port = %d, want 49800", got.Port)
		}
		if got.Origin != "http://127.0.0.1:49800" {
			t.Fatalf("got.Origin = %q", got.Origin)
		}
	})

	t.Run("miss returns ErrNoDesktop", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 0, false)
		stubDNSSD(t, 0, false)
		_, err := DiscoverViaMDNS(context.Background())
		if !errors.Is(err, ErrNoDesktop) {
			t.Fatalf("err = %v, want ErrNoDesktop", err)
		}
	})
}

// TestAdvertisedPort exercises the extracted helper directly: multicast wins
// first, dns-sd is consulted only on darwin, and non-darwin skips it entirely.
func TestAdvertisedPort(t *testing.T) {
	t.Run("multicast hit short-circuits dns-sd", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 49500, true)
		t.Cleanup(SetDNSSDLookupFnForTest(func(_ context.Context) (int, bool) {
			t.Fatalf("dnssdLookupFn must not be consulted after a multicast hit")
			return 0, false
		}))
		port, ok := advertisedPort(context.Background())
		if !ok || port != 49500 {
			t.Fatalf("advertisedPort = (%d, %v), want (49500, true)", port, ok)
		}
	})

	t.Run("multicast miss falls to dns-sd on darwin", func(t *testing.T) {
		stubGOOS(t, "darwin")
		stubMDNS(t, 0, false)
		stubDNSSD(t, 49600, true)
		port, ok := advertisedPort(context.Background())
		if !ok || port != 49600 {
			t.Fatalf("advertisedPort = (%d, %v), want (49600, true)", port, ok)
		}
	})

	t.Run("non-darwin never consults dns-sd", func(t *testing.T) {
		stubGOOS(t, "linux")
		stubMDNS(t, 0, false)
		t.Cleanup(SetDNSSDLookupFnForTest(func(_ context.Context) (int, bool) {
			t.Fatalf("dnssdLookupFn must not be consulted on non-darwin")
			return 0, false
		}))
		port, ok := advertisedPort(context.Background())
		if ok || port != 0 {
			t.Fatalf("advertisedPort = (%d, %v), want (0, false)", port, ok)
		}
	})
}

// TestDiscover_ContextCancelled asserts Discover returns promptly (no hang)
// when the context is already cancelled. The mDNS/dns-sd seams honour the
// cancelled context by reporting a miss, so the port scan's ctx.Err() check
// returns the cancellation.
func TestDiscover_ContextCancelled(t *testing.T) {
	stubGOOS(t, "darwin")
	t.Cleanup(SetMDNSBrowseFnForTest(func(ctx context.Context) (int, bool) {
		return 0, ctx.Err() == nil
	}))
	t.Cleanup(SetDNSSDLookupFnForTest(func(ctx context.Context) (int, bool) {
		return 0, ctx.Err() == nil
	}))
	pinProbeTo(t, "localhost", map[int]bool{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	var err error
	go func() {
		_, err = Discover(ctx, 0)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Discover did not return promptly on a cancelled context")
	}
	if err == nil {
		t.Fatalf("expected an error on a cancelled context")
	}
}
