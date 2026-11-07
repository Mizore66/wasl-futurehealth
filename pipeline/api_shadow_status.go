package main

// GET /api/agentshadow/status: read-only lifecycle/progress of the agent-shadow spike (docs/AGENT-SHADOW.md).
// Reads <run>/status-agentshadow.json (via readStatus) and <run>/agent_shadow/health.json on every request, no caching.
// Only an allow-list of lifecycle, progress, cap and counter fields is returned. It never returns observations,
// recommendations, aliases, raw ids, prompts, cache paths or free-text degrade reasons.
// The handler is not registered here; api.go wires it: mux.HandleFunc("/api/agentshadow/status", s.getAgentShadowStatus).

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

var shadowStatusFields = []string{"mode", "done", "degraded", "killed", "incomplete", "read_only", "publishes",
	"prep_applied", "prep_pending", "live_waiting", "items_attempted", "max_items", "uncached_calls", "max_calls",
	"tokens_uncached", "uptime_s", "providers_done", "counts", "fallback_reasons"}

func (s *apiServer) getAgentShadowStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONResp(w, http.StatusMethodNotAllowed, s.envelope(map[string]interface{}{"error": "method not allowed"}))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	st := readStatus(s.run, "agentshadow")
	if st == nil {
		writeJSONResp(w, http.StatusOK, s.envelope(map[string]interface{}{"available": false, "status": "not_run"}))
		return
	}
	out := map[string]interface{}{"available": true}
	for _, k := range shadowStatusFields {
		if v, ok := st[k]; ok {
			out[k] = v
		}
	}
	done, _ := st["done"].(bool)
	clean := done
	for _, k := range []string{"degraded", "killed", "incomplete"} {
		if b, _ := st[k].(bool); b {
			clean = false
		}
	}
	switch {
	case !done:
		out["status"] = "running"
	case clean:
		out["status"] = "finished"
	default:
		out["status"] = "finished_degraded"
	}
	out["clean_success"] = clean
	health := map[string]interface{}{}
	if b, err := os.ReadFile(filepath.Join(s.run, "agent_shadow", "health.json")); err == nil {
		var h map[string]interface{}
		if json.Unmarshal(b, &h) == nil {
			if ts, ok := h["ts"].(string); ok {
				health["ts"] = ts
			}
		}
	}
	out["health"] = health
	writeJSONResp(w, http.StatusOK, s.envelope(out))
}
