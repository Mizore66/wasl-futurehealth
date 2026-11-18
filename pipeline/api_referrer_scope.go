package main

import (
	"net/http"
	"net/url"
	"strings"
)

// A referring clinician (WASL_REFERRING_CLINICIANS) holds an ordinary clinician token, so the role alone would let it
// write to any provider's case (consent, review, actions). It acts only for its own referring provider: every case
// write by such an actor on another provider's case is refused before the route runs. Reads are unchanged; the demo
// clinician role reads the shared worklist. A receiving-unit clinician is scoped by its own route checks instead.

// caseIDFromPath resolves the case of a /api/cases/{id}/... path (ids contain slashes, encoded or not).
func (s *apiServer) caseIDFromPath(path string) string {
	rest, err := url.PathUnescape(strings.TrimPrefix(path, "/api/cases/"))
	if err != nil {
		return ""
	}
	for i := 0; i < 4 && rest != ""; i++ {
		if s.cases[rest] != nil {
			return rest
		}
		j := strings.LastIndex(rest, "/")
		if j < 0 {
			break
		}
		rest = rest[:j]
	}
	return ""
}

func (s *apiServer) refuseReferrerOutsideProvider(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return false
	}
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	provider := s.referrers[actor]
	if role != RoleClinician || provider == "" || s.unitClinicians[actor] != "" {
		return false
	}
	c := s.cases[s.caseIDFromPath(r.URL.Path)]
	if c == nil || referrerProvider(c) == provider {
		return false
	}
	writeJSONResp(w, http.StatusForbidden, s.envelope(map[string]interface{}{
		"error":    "a referring clinician acts only for its own provider's cases",
		"provider": referrerProvider(c),
	}))
	return true
}

// referrerScoped applies refuseReferrerOutsideProvider in front of every case route.
func (s *apiServer) referrerScoped(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.refuseReferrerOutsideProvider(w, r) {
			return
		}
		next(w, r)
	}
}
