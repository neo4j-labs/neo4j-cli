// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package urlcheck

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// metadataIP is the well-known cloud-provider instance metadata service IP.
// It must always be rejected because exfiltrating credentials via SSRF to
// this endpoint is a common attack pattern.
const metadataIP = "169.254.169.254"

// ValidateRemoteURL enforces an SSRF allowlist on outbound URLs the CLI is
// about to fetch from. It accepts:
//
//   - https:// to any host
//   - http:// only when the host is loopback (localhost, 127.0.0.1, ::1)
//
// It rejects:
//
//   - empty or otherwise malformed URLs
//   - URLs whose scheme is anything other than http or https
//   - http:// to non-loopback hosts (cleartext to third parties)
//   - IP-literal hosts that are private, link-local unicast, link-local
//     multicast, or multicast
//   - the cloud metadata IP 169.254.169.254 explicitly
//
// Hostnames are passed through after the scheme/loopback gate; DNS rebinding
// is documented as out of scope (mitigation would require resolving the host
// at request time and re-checking, which is a substantially larger change).
func ValidateRemoteURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("url is empty")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url is malformed: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme %q not allowed (only http and https)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host: %q", raw)
	}

	// Explicit metadata-IP block (string match catches the literal even when
	// passed through DNS oddities like trailing dots).
	if strings.EqualFold(strings.TrimSuffix(host, "."), metadataIP) {
		return fmt.Errorf("url host %q is the cloud metadata IP and is blocked", host)
	}

	// Try parsing the host as an IP literal. ParseIP returns nil for hostnames.
	ip := net.ParseIP(host)
	if ip != nil {
		if err := validateIP(ip); err != nil {
			return err
		}
	}

	// Cleartext http is only allowed to loopback.
	if scheme == "http" {
		if ip != nil {
			if !ip.IsLoopback() {
				return fmt.Errorf("http url to non-loopback host %q is not allowed (use https)", host)
			}
		} else {
			// Hostname: only "localhost" (or its dotted variant) is loopback.
			h := strings.TrimSuffix(strings.ToLower(host), ".")
			if h != "localhost" {
				return fmt.Errorf("http url to non-loopback host %q is not allowed (use https)", host)
			}
		}
	}

	return nil
}

// validateIP rejects IP literals that point at internal networks. Loopback
// is allowed (the http branch handles whether http to loopback is permitted).
func validateIP(ip net.IP) error {
	switch {
	case ip.IsLoopback():
		return nil
	case ip.IsPrivate():
		return fmt.Errorf("url host %q is in a private network and is blocked", ip.String())
	case ip.IsLinkLocalUnicast():
		return fmt.Errorf("url host %q is link-local unicast and is blocked", ip.String())
	case ip.IsLinkLocalMulticast():
		return fmt.Errorf("url host %q is link-local multicast and is blocked", ip.String())
	case ip.IsMulticast():
		return fmt.Errorf("url host %q is multicast and is blocked", ip.String())
	case ip.IsUnspecified():
		return fmt.Errorf("url host %q is the unspecified address and is blocked", ip.String())
	}
	return nil
}
