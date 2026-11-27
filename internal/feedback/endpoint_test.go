// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package feedback

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEndpointAllowed(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://127.0.0.1:8765/v1/feedback": true,
		"http://localhost:1/x":              true,
		"http://[::1]:8765/x":               true,
		"https://127.0.0.1/":                false,
		"http://10.0.0.1/":                  false,
		"http://example.com/":               false,
		"http://localhost.example.com/":     false,
		"http://127.0.0.1.example.com/":     false,
		"":                                  false,
		"ftp://127.0.0.1/":                  false,
		"https://api.mixpanel.com":          true,
		"https://api.mixpanel.com/":         true,
		"https://api-eu.mixpanel.com":       true,
		"https://api-in.mixpanel.com":       true,
		"http://api.mixpanel.com":           false, // https only
		"https://api.mixpanel.com:8443":     false,
		"https://api.mixpanel.com/track":    false, // no path, so the region is the only choice
		"https://api.mixpanel.com/?x=1":     false,
		"https://u:p@api.mixpanel.com":      false,
		"https://api.mixpanel.com.evil.com": false,
		"https://evilapi.mixpanel.com":      false,
		"https://mixpanel.com":              false,
		"https://example.com":               false,
		"https://api.mixpanel.com#frag":     false, // would turn /track?ip=0 into a fragment
		"https://api.mixpanel.com?":         false, // an empty query, same effect
		"https://api.mixpanel.com/%2F":      false,
		"http://127.0.0.1:8765/x#frag":      false,
		"http://127.0.0.1:8765/x?y=1":       false,
	} {
		assert.Equal(t, want, EndpointAllowed(raw), raw)
	}
}

func TestDefaultEndpoint_OnlyWithAToken(t *testing.T) {
	withToken(t, "")
	assert.Equal(t, "", DefaultEndpoint(), "a build without a token has no destination of its own")
	withToken(t, "some-token")
	assert.Equal(t, mixpanelBase, DefaultEndpoint())
	assert.True(t, EndpointAllowed(DefaultEndpoint()), "the compiled-in destination must be an allowed endpoint")
}

func TestTrackURL_IsBuiltFromParts(t *testing.T) {
	for ep, want := range map[string]string{
		"https://api.mixpanel.com":          "https://api.mixpanel.com/track?ip=0&verbose=1",
		"https://api-eu.mixpanel.com/":      "https://api-eu.mixpanel.com/track?ip=0&verbose=1",
		"http://127.0.0.1:8765/v1/feedback": "http://127.0.0.1:8765/v1/feedback/track?ip=0&verbose=1",
	} {
		got, ok := trackURL(ep)
		assert.True(t, ok, ep)
		assert.Equal(t, want, got)
	}
	_, ok := trackURL("https://api.mixpanel.com#frag")
	assert.False(t, ok)
}
