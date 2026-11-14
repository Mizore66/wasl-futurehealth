package main

// Demo API authentication and browser trust boundaries.
//
// Bearer tokens are server configuration, not client-selected identities. Each token is bound to one role and immutable
// actor. This is deliberately a small synthetic-demo control, not an OIDC/mTLS production deployment.

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

type apiPrincipal struct {
	role  string
	actor string
}

func parseAPIPrincipals(config string) (map[[32]byte]apiPrincipal, error) {
	out := map[[32]byte]apiPrincipal{}
	if strings.TrimSpace(config) == "" {
		return nil, fmt.Errorf("WASL_API_TOKENS is required (token:role:actor, comma-separated)")
	}
	for _, entry := range strings.Split(config, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 3)
		if len(parts) != 3 || len(parts[0]) < 16 || strings.TrimSpace(parts[2]) == "" {
			return nil, fmt.Errorf("invalid WASL_API_TOKENS entry: need a token of at least 16 characters, role and actor")
		}
		token, role, actor := parts[0], parts[1], strings.TrimSpace(parts[2])
		if _, known := rbac[role]; !known || role == RoleSystem {
			return nil, fmt.Errorf("invalid WASL_API_TOKENS role %q", role)
		}
		if utf8.RuneCountInString(actor) > maxActorRunes || strings.ContainsAny(actor, "\r\n") {
			return nil, fmt.Errorf("invalid WASL_API_TOKENS actor")
		}
		key := sha256.Sum256([]byte(token))
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("duplicate WASL_API_TOKENS token")
		}
		out[key] = apiPrincipal{role: role, actor: actor}
	}
	return out, nil
}

func parseAPIOrigins(config string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, origin := range strings.Split(config, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("invalid WASL_API_ORIGINS entry %q", origin)
		}
		out[origin] = true
	}
	return out, nil
}

func apiReadAction(path string) string {
	if strings.HasPrefix(path, "/api/cases/") && (strings.HasSuffix(path, "/audit") || strings.HasSuffix(path, "/export")) {
		return ActReadAudit
	}
	return ActReadQueue
}

// authorizeVerified applies read permissions after either bearer or Basic authentication has established identity.
// Preflight carries no credentials or case data and is handled by the downstream CORS policy.
func (s *apiServer) authorizeVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/patient/journey" || !strings.HasPrefix(r.URL.Path, "/api/") || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if err := Authorize(r.Header.Get("X-Wasl-Role"), apiReadAction(r.URL.Path)); err != nil {
			writeJSONResp(w, http.StatusForbidden, map[string]interface{}{"error": "read permission denied"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *apiServer) authenticate(tokens map[[32]byte]apiPrincipal, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/patient/journey" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.Contains(token, " ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wasl-synthetic-demo"`)
			writeJSONResp(w, http.StatusUnauthorized, map[string]interface{}{"error": "authentication required"})
			return
		}
		key := sha256.Sum256([]byte(token))
		var principal apiPrincipal
		found := false
		for candidate, configured := range tokens {
			if subtle.ConstantTimeCompare(key[:], candidate[:]) == 1 {
				principal, found = configured, true
			}
		}
		if !found {
			w.Header().Set("WWW-Authenticate", `Bearer realm="wasl-synthetic-demo"`)
			writeJSONResp(w, http.StatusUnauthorized, map[string]interface{}{"error": "authentication required"})
			return
		}
		// Read authorization is a route property, not a GET-only check. Alternate methods must not bypass it;
		// downstream handlers still enforce their own method and mutation-specific RBAC contracts.
		action := apiReadAction(r.URL.Path)
		if err := Authorize(principal.role, action); err != nil {
			writeJSONResp(w, http.StatusForbidden, map[string]interface{}{"error": "read permission denied"})
			return
		}
		// Downstream workflow handlers retain their action-specific RBAC checks, but see only verified identity.
		r.Header.Set("X-Wasl-Role", principal.role)
		r.Header.Set("X-Wasl-Actor", principal.actor)
		next.ServeHTTP(w, r)
	})
}

func corsWithOrigins(h http.Handler, origins map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/patient/journey" {
			// The Next.js server proxies patient requests. No browser origin is
			// granted direct CORS access to this credentialed endpoint.
			h.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !origins[origin] {
				writeJSONResp(w, http.StatusForbidden, map[string]interface{}{"error": "origin not allowed"})
				return
			}
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin == "" {
				writeJSONResp(w, http.StatusBadRequest, map[string]interface{}{"error": "CORS preflight requires Origin"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// handler builds the complete externally served API. Missing or invalid authentication configuration fails closed.
func (s *apiServer) handler(tokenConfig, originConfig string) (http.Handler, error) {
	tokens, err := parseAPIPrincipals(tokenConfig)
	if err != nil {
		return nil, err
	}
	origins, err := parseAPIOrigins(originConfig)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/patient/journey", s.patientJourney)
	mux.HandleFunc("/api/operations", s.getOperations)
	mux.HandleFunc("/api/dashboard", s.getDashboard)
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/people", s.peopleRoutes)
	mux.HandleFunc("/api/people/", s.peopleRoutes)
	mux.HandleFunc("/api/agentshadow/status", s.getAgentShadowStatus)
	caseAPI := s.withConsent(s.undoRoutes(s.caseRoutes))
	mux.HandleFunc("/api/cases/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			s.caseExportRoute(w, r)
			return
		}
		caseAPI.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	return corsWithOrigins(s.authenticate(tokens, mux), origins), nil
}
