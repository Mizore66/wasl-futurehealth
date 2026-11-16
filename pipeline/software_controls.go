package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const softwareSmokeFactoryPath = "/api/software-smoke/sessions"
const softwareSmokeFactoryHeader = "Wasl-Software-Smoke-Factory"

// attachSyntheticSmokeFactory exposes only opt-in software-contract controls.
// Each session owns a NEW verified source-bound lane and ordinary API handler;
// no fixture cases, source records or overlay events enter the actual server.
func attachSyntheticSmokeFactory(s *apiServer, handler http.Handler, tokens, origins string) http.Handler {
	if os.Getenv("WASL_SYNTHETIC_DATA_ONLY") != "1" {
		return handler
	}
	principals, err := parseAPIPrincipals(tokens)
	if err != nil {
		panic(err)
	}
	allowedOrigins, err := parseAPIOrigins(origins)
	if err != nil {
		panic(err)
	}
	var mu sync.Mutex
	sessions := map[string]http.Handler{}
	attempts := 0
	create := corsWithOrigins(s.authenticate(principals, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Wasl-Role") != RoleCoordinator {
			writeJSONResp(w, http.StatusForbidden, map[string]interface{}{"error": "software session creation requires coordinator"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSONResp(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "POST required"})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if attempts >= 8 {
			writeJSONResp(w, http.StatusTooManyRequests, map[string]interface{}{"error": "software session limit reached"})
			return
		}
		attempts++ // Failed attempts also consume the bound; old lanes are retained.
		stage := "resolve_actual"
		fail := func() {
			// Fixed stage labels only: never log raw source text, paths or errors.
			writeStructuredLog("error", "synthetic software session creation failed", "stage", stage)
			writeJSONResp(w, http.StatusInternalServerError, map[string]interface{}{"error": "software session creation failed"})
		}
		actual, err := filepath.EvalSymlinks(s.run)
		if err != nil {
			fail()
			return
		}
		stage = "resolve_temp"
		temp, err := filepath.EvalSymlinks(os.TempDir())
		if err != nil {
			fail()
			return
		}
		stage = "temp_containment"
		rel, err := filepath.Rel(actual, temp)
		if err != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			fail() // Never create even a temporary parent inside actual evidence.
			return
		}
		stage = "reserve_temp"
		root, err := os.MkdirTemp("", "wasl-software-smoke-")
		if err != nil {
			fail()
			return
		}
		run := filepath.Join(root, "fixture")
		stage = "fixture_generation"
		if err := writeIndependentSmokeFixture(run); err != nil {
			if strings.HasPrefix(err.Error(), "synthetic core must raise one link and one evidence item;") {
				stage = "fixture_question_counts"
			}
			fail()
			return
		}
		stage = "fixture_verification"
		fixture, err := loadAPIVerified(run, "")
		if err != nil {
			fail()
			return
		}
		stage = "fixture_handler"
		fixtureHandler, err := fixture.handler(tokens, origins)
		if err != nil {
			fail()
			return
		}
		stage = "session_nonce"
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			fail()
			return
		}
		base := softwareSmokeFactoryPath + "/" + hex.EncodeToString(nonce[:])
		sessions[base] = http.StripPrefix(base, fixtureHandler)
		writeJSONResp(w, http.StatusCreated, map[string]interface{}{
			"schema": "wasl-software-smoke-session/1", "synthetic": true, "base_path": base,
			"purpose": "built synthetic software fixture; not clinical or inference evidence",
		})
	})), allowedOrigins)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set(softwareSmokeFactoryHeader, softwareSmokeFactoryPath)
		}
		if r.URL.Path == softwareSmokeFactoryPath {
			create.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, softwareSmokeFactoryPath+"/") {
			suffix := strings.TrimPrefix(r.URL.Path, softwareSmokeFactoryPath+"/")
			id, _, _ := strings.Cut(suffix, "/")
			mu.Lock()
			session := sessions[softwareSmokeFactoryPath+"/"+id]
			mu.Unlock()
			if session == nil {
				writeJSONResp(w, http.StatusNotFound, map[string]interface{}{"error": "unknown software session"})
				return
			}
			session.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
