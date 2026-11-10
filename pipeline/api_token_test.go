package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireToken(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	const tok = "test-token-0123456789abcdef"
	h := cors(requireToken(tok, ok))
	cases := []struct {
		method, path, auth string
		want               int
	}{
		{"GET", "/api/dashboard", "", 401},
		{"GET", "/api/dashboard", "Bearer wrong-token-0123456789abcdef", 401},
		{"GET", "/api/dashboard", tok, 401}, // scheme required
		{"GET", "/api/dashboard", "Bearer " + tok, 200},
		{"POST", "/api/cases/x/actions", "", 401},
		{"POST", "/api/cases/x/actions", "Bearer " + tok, 200},
		{"GET", "/healthz", "", 200},
		{"OPTIONS", "/api/dashboard", "", 204},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if c.auth != "" {
			r.Header.Set("Authorization", c.auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s auth=%q: got %d, want %d", c.method, c.path, c.auth, w.Code, c.want)
		}
	}
	// No token configured: the original open behaviour.
	w := httptest.NewRecorder()
	cors(requireToken("", ok)).ServeHTTP(w, httptest.NewRequest("GET", "/api/dashboard", nil))
	if w.Code != 200 {
		t.Errorf("open mode: got %d, want 200", w.Code)
	}
}
