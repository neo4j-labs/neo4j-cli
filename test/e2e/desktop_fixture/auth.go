// Copyright (c) "Neo4j"
// Neo4j Sweden AB [http://neo4j.com]

package main

import (
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// requireAuth wraps an authed /fastify/api/* handler with the same header
// validation Desktop's `packages/web/src/fastify/auth/api-token.middleware.ts`
// performs: X-Client-Id must be non-empty, X-API-Token must verify under the
// HS256 composite key `<salt>-<httpOrigin>-<clientId>`. Failures yield 401
// with a short body. The wrapper also honours state.auth — when set to
// reject/status500/close the handler never runs, simulating the three
// transport sad-paths the production client must cope with.
func requireAuth(s *state, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		mode := s.auth
		salt := s.salt
		origin := s.origin
		s.mu.Unlock()

		// Simulate transport sad-paths BEFORE the auth check — that way a
		// reject scenario surfaces a 401 even when the request carries a
		// valid token, and the close scenario hangs up the socket even if
		// auth would have passed.
		switch mode {
		case authModeReject:
			http.Error(w, "unauthorized (scenario reject)", http.StatusUnauthorized)
			s.logRequest(fmt.Sprintf("%s %s -> 401 (scenario reject)", r.Method, r.URL.Path))
			return
		case authModeStatus500:
			http.Error(w, "scenario 500: simulated upstream failure", http.StatusInternalServerError)
			s.logRequest(fmt.Sprintf("%s %s -> 500 (scenario)", r.Method, r.URL.Path))
			return
		case authModeClose:
			// Hijack the connection and close it mid-stream. Mirrors a
			// Desktop crash / network reset — the production client maps
			// this to REQ-F-008's canonical "doesn't appear to be running"
			// hint.
			hj, ok := w.(http.Hijacker)
			if !ok {
				// Fallback: write a body then panic-close. Standard
				// net/http on go1.20+ supports Hijack on the default
				// server, so this branch should be unreachable in
				// practice.
				http.Error(w, "scenario close: cannot hijack", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
			s.logRequest(fmt.Sprintf("%s %s -> CLOSE (scenario)", r.Method, r.URL.Path))
			return
		}

		clientID := r.Header.Get("X-Client-Id")
		token := r.Header.Get("X-API-Token")
		if clientID == "" || token == "" {
			http.Error(w, "missing X-Client-Id or X-API-Token", http.StatusUnauthorized)
			s.logRequest(fmt.Sprintf("%s %s -> 401 (missing headers)", r.Method, r.URL.Path))
			return
		}
		key := fmt.Sprintf("%s-%s-%s", salt, origin, clientID)
		_, err := jwt.Parse(token, func(_ *jwt.Token) (any, error) { return []byte(key), nil })
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid X-API-Token: %v", err), http.StatusUnauthorized)
			s.logRequest(fmt.Sprintf("%s %s -> 401 (jwt: %v)", r.Method, r.URL.Path, err))
			return
		}
		// Auth passed — delegate to the real handler. handlers.go logs the
		// outcome with whatever status it writes.
		h(w, r)
	}
}
