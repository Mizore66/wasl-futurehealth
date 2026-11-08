package main

// GET /api/agentshadow/status: read-only lifecycle/progress of the agent-shadow spike (docs/AGENT-SHADOW.md).
// Reads <run>/status-agentshadow.json (via readStatus) and <run>/agent_shadow/health.json on every request, no caching.
// Only an allow-list of lifecycle, progress, cap and counter fields is returned. It never returns observations,
// recommendations, aliases, raw ids, prompts, cache paths or free-text degrade reasons. counts and fallback_reasons are filtered against closed sets of the names agentshadow.go
// and scripts/agent_shadow_worker.py can produce; every other key is dropped. A status file that is empty, unparsable or
// has no recognised field is reported as 503 {available:false,status:"invalid"}, never as a live run.
// The handler is not registered here; api.go wires it: mux.HandleFunc("/api/agentshadow/status", s.getAgentShadowStatus).

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var shadowBoolFields = []string{"done", "degraded", "killed", "incomplete", "read_only"}
var shadowNumFields = []string{"publishes", "prep_applied", "prep_pending", "live_waiting", "items_attempted", "max_items",
	"uncached_calls", "max_calls", "tokens_uncached", "uptime_s", "providers_done"}

// shadowCountKeys: the keys AgentShadow.counts can hold (agentshadow.go): the fixed names plus "outcome_"+Outcome.
var shadowCountKeys = map[string]bool{"cache_changed": true, "eligible": true, "items_matched": true, "link_items_matched": true,
	"not_eligible": true, "out_duplicates": true, "out_seen": true, "out_verified": true, "twins_skipped": true,
	"outcome_fallback": true, "outcome_ineligible": true, "outcome_skipped": true, "outcome_recommendation": true}

// shadowReasonKeys: every reason code agentshadow.go and the worker can record. Dynamic families are collapsed below.
var shadowReasonKeys = map[string]bool{"cache_miss": true, "killed": true, "stop_file": true, "deadline": true, "cap_calls": true,
	"item_cap": true, "degraded": true, "worker_timeout": true, "worker_error": true, "human_answered": true, "staging_failed": true,
	"bounds_violation": true, "ineligible_item": true, "invalid_metadata": true, "record_conflict": true, "cached_mode": true,
	"no_provider_config": true, "second_identifier": true, "provider_timeout": true, "provider_error": true,
	"provider_malformed": true, "demographic_missing": true, "usage_not_persisted": true}

var shadowHTTPReasonRE = regexp.MustCompile(`^provider_http_[0-9]{3}$`)
var shadowStagingReasonRE = regexp.MustCompile(`^staging_error:[a-z0-9_]{1,40}$`)

func shadowReasonName(k string) (string, bool) {
	switch {
	case shadowReasonKeys[k], shadowHTTPReasonRE.MatchString(k):
		return k, true
	case shadowStagingReasonRE.MatchString(k):
		return "staging_error", true // the exception class name is not exposed
	}
	return "", false
}

func shadowCounter(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f != math.Trunc(f) {
		return 0, false
	}
	return f, true
}

func shadowFilterMap(v interface{}, name func(string) (string, bool)) map[string]interface{} {
	in, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := map[string]interface{}{}
	for k, raw := range in {
		n, ok := name(k)
		if !ok {
			continue
		}
		c, ok := shadowCounter(raw)
		if !ok {
			continue
		}
		prev, _ := out[n].(float64)
		out[n] = prev + c
	}
	return out
}

func shadowHealthTS(run string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(run, "agent_shadow", "health.json"))
	if err != nil {
		return "", false
	}
	var h map[string]interface{}
	if json.Unmarshal(b, &h) != nil {
		return "", false
	}
	ts, ok := h["ts"].(string)
	if !ok || len(ts) > 40 {
		return "", false
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
	if err != nil {
		return "", false
	}
	return t.UTC().Format(time.RFC3339), true
}

func (s *apiServer) getAgentShadowStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONResp(w, http.StatusMethodNotAllowed, s.envelope(map[string]interface{}{"error": "method not allowed"}))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	raw, err := os.ReadFile(filepath.Join(s.run, "status-agentshadow.json"))
	if os.IsNotExist(err) {
		writeJSONResp(w, http.StatusOK, s.envelope(map[string]interface{}{"available": false, "status": "not_run"}))
		return
	}
	var st map[string]interface{}
	if err != nil || json.Unmarshal(raw, &st) != nil || st == nil {
		writeJSONResp(w, http.StatusServiceUnavailable, s.envelope(map[string]interface{}{"available": false, "status": "invalid"}))
		return
	}
	out := map[string]interface{}{}
	flags := map[string]bool{}
	for _, k := range shadowBoolFields {
		if b, ok := st[k].(bool); ok {
			out[k] = b
			flags[k] = true
		}
	}
	for _, k := range shadowNumFields {
		if f, ok := st[k].(float64); ok && !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0 {
			out[k] = f
		}
	}
	if m, ok := st["mode"].(string); ok && (m == "cached" || m == "live") {
		out["mode"] = m
	}
	if c := shadowFilterMap(st["counts"], func(k string) (string, bool) { return k, shadowCountKeys[k] }); c != nil {
		out["counts"] = c
	}
	if c := shadowFilterMap(st["fallback_reasons"], shadowReasonName); c != nil {
		out["fallback_reasons"] = c
	}
	if len(out) == 0 {
		// empty, truncated, unparsable or foreign file: no claim about a live run
		writeJSONResp(w, http.StatusServiceUnavailable, s.envelope(map[string]interface{}{"available": false, "status": "invalid"}))
		return
	}
	out["available"] = true
	allFlags := flags["done"] && flags["degraded"] && flags["killed"] && flags["incomplete"]
	clean := allFlags && out["done"] == true && out["degraded"] == false && out["killed"] == false && out["incomplete"] == false
	switch {
	case !flags["done"] || !allFlags && out["done"] == false:
		out["status"] = "unknown"
	case out["done"] == false:
		out["status"] = "running"
	case clean:
		out["status"] = "finished"
	default:
		out["status"] = "finished_degraded"
	}
	out["clean_success"] = clean
	if ts, ok := shadowHealthTS(s.run); ok {
		out["health"] = map[string]interface{}{"ts": ts}
	}
	writeJSONResp(w, http.StatusOK, s.envelope(out))
}
