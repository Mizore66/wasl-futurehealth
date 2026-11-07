package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func shadowStatusGet(t *testing.T, run, method string) (int, map[string]interface{}, string) {
	t.Helper()
	s := &apiServer{run: run, meta: map[string]interface{}{"api": apiVersion}}
	rec := httptest.NewRecorder()
	s.getAgentShadowStatus(rec, httptest.NewRequest(method, "/api/agentshadow/status", nil))
	m := map[string]interface{}{}
	json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m, rec.Body.String()
}

func writeShadowFiles(t *testing.T, run string, st map[string]interface{}, ts string) {
	t.Helper()
	os.MkdirAll(filepath.Join(run, "agent_shadow"), 0o755)
	writeStatus(run, "agentshadow", st)
	if ts != "" {
		b, _ := json.Marshal(map[string]interface{}{"ts": ts, "degraded": st["degraded"], "killed": st["killed"], "done": st["done"]})
		os.WriteFile(filepath.Join(run, "agent_shadow", "health.json"), b, 0o644)
	}
}

func TestShadowStatusAbsent(t *testing.T) {
	code, m, _ := shadowStatusGet(t, t.TempDir(), "GET")
	if code != 200 || m["available"] != false || m["status"] != "not_run" {
		t.Fatalf("absent: %d %v", code, m)
	}
}

func TestShadowStatusRunning(t *testing.T) {
	run := t.TempDir()
	ts := time.Now().UTC().Format(time.RFC3339)
	writeShadowFiles(t, run, map[string]interface{}{"mode": "cached", "done": false, "degraded": false, "killed": false, "incomplete": false,
		"items_attempted": 3, "max_items": 20, "uncached_calls": 1, "max_calls": 5, "prep_applied": 2, "counts": map[string]int{"eligible": 4}}, ts)
	code, m, _ := shadowStatusGet(t, run, "GET")
	if code != 200 || m["available"] != true || m["status"] != "running" || m["done"] != false || m["items_attempted"] != float64(3) || m["max_items"] != float64(20) {
		t.Fatalf("running: %d %v", code, m)
	}
	if h, _ := m["health"].(map[string]interface{}); h == nil || h["ts"] != ts {
		t.Fatalf("health.ts missing: %v", m["health"])
	}
}

func TestShadowStatusFinalDegradedIsNotCleanSuccess(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, map[string]interface{}{"mode": "live", "done": true, "degraded": true, "degraded_reason": "budget", "killed": true, "incomplete": true,
		"fallback_reasons": map[string]int{"timeout": 2}}, "2026-11-08T10:00:00Z")
	_, m, _ := shadowStatusGet(t, run, "GET")
	if m["done"] != true || m["degraded"] != true || m["killed"] != true || m["incomplete"] != true {
		t.Fatalf("flags lost: %v", m)
	}
	if m["status"] != "finished_degraded" || m["clean_success"] != false {
		t.Fatalf("done must not read as clean: %v", m)
	}
}

func TestShadowStatusCleanFinish(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, map[string]interface{}{"done": true, "degraded": false, "killed": false, "incomplete": false}, "2026-11-08T10:00:00Z")
	_, m, _ := shadowStatusGet(t, run, "GET")
	if m["status"] != "finished" || m["clean_success"] != true {
		t.Fatalf("clean: %v", m)
	}
}

func TestShadowStatusSuccessiveWritesNoCache(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, shFlags(false, false, false, false, map[string]interface{}{"items_attempted": 1}), "2026-11-08T10:00:00Z")
	_, a, _ := shadowStatusGet(t, run, "GET")
	writeShadowFiles(t, run, shFlags(true, true, false, false, map[string]interface{}{"items_attempted": 7}), "2026-11-08T10:05:00Z")
	_, b, _ := shadowStatusGet(t, run, "GET")
	if a["items_attempted"] != float64(1) || b["items_attempted"] != float64(7) || b["done"] != true {
		t.Fatalf("stale: %v %v", a, b)
	}
	if b["health"].(map[string]interface{})["ts"] != "2026-11-08T10:05:00Z" {
		t.Fatalf("health stale: %v", b["health"])
	}
}

func TestShadowStatusMissingHealthStillAvailable(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, shFlags(false, false, false, false, nil), "")
	_, m, body := shadowStatusGet(t, run, "GET")
	if m["available"] != true {
		t.Fatalf("%v", m)
	}
	if _, ok := m["health"]; ok || strings.Contains(body, `"health"`) {
		t.Fatalf("health must be omitted entirely: %s", body)
	}
}

func TestShadowStatusGETOnly(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, shFlags(true, false, false, false, nil), "2026-11-08T10:00:00Z")
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		code, _, _ := shadowStatusGet(t, run, method)
		if code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: %d", method, code)
		}
	}
}

func TestShadowStatusLeaksNothing(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, map[string]interface{}{"done": true, "degraded": true, "killed": false, "incomplete": false, "degraded_reason": "cache /tmp/x/cache failed",
		"observations": []string{"obs"}, "recommendations": []string{"rec"}, "aliases": map[string]string{"a": "b"},
		"prompt": "p", "cache_path": "/tmp/x", "counts": map[string]int{"eligible": 1}, "raw_ids": []string{"AHS-1"}}, "2026-11-08T10:00:00Z")
	_, m, body := shadowStatusGet(t, run, "GET")
	for _, k := range []string{"observations", "recommendations", "aliases", "prompt", "cache_path", "raw_ids"} {
		if _, ok := m[k]; ok {
			t.Fatalf("leaked %s", k)
		}
	}
	if strings.Contains(body, "/tmp/x") || strings.Contains(body, "AHS-1") {
		t.Fatalf("leak: %s", body)
	}
}

func shFlags(done, degraded, killed, incomplete bool, extra map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{"done": done, "degraded": degraded, "killed": killed, "incomplete": incomplete}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func writeRawStatus(t *testing.T, run, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(run, "status-agentshadow.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestShadowStatusMissingOrWrongTypeFlagsNeverClean(t *testing.T) {
	cases := map[string]string{
		"only done":     `{"done":true}`,
		"no incomplete": `{"done":true,"degraded":false,"killed":false}`,
		"no killed":     `{"done":true,"degraded":false,"incomplete":false}`,
		"no degraded":   `{"done":true,"killed":false,"incomplete":false}`,
		"no done":       `{"degraded":false,"killed":false,"incomplete":false}`,
		"string flag":   `{"done":true,"degraded":"false","killed":false,"incomplete":false}`,
		"number flag":   `{"done":true,"degraded":false,"killed":0,"incomplete":false}`,
		"null flag":     `{"done":true,"degraded":false,"killed":false,"incomplete":null}`,
		"string done":   `{"done":"true","degraded":false,"killed":false,"incomplete":false}`,
		"only items":    `{"items_attempted":3}`,
	}
	for name, raw := range cases {
		run := t.TempDir()
		writeRawStatus(t, run, raw)
		_, m, _ := shadowStatusGet(t, run, "GET")
		if m["clean_success"] == true || m["status"] == "finished" || m["status"] == "running" {
			t.Fatalf("%s: claimed clean/running: %v", name, m)
		}
		if m["status"] != "unknown" && m["status"] != "finished_degraded" {
			t.Fatalf("%s: status %v", name, m["status"])
		}
	}
}

func TestShadowStatusInvalidFilesMakeNoLiveClaim(t *testing.T) {
	for name, raw := range map[string]string{"empty": ``, "empty map": `{}`, "truncated": `{"done":false,"degraded":fal`,
		"garbage": `not json`, "array": `[1,2]`, "null": `null`, "string": `"x"`} {
		run := t.TempDir()
		writeRawStatus(t, run, raw)
		code, m, body := shadowStatusGet(t, run, "GET")
		if code != http.StatusServiceUnavailable || m["available"] != false || m["status"] != "invalid" {
			t.Fatalf("%s: %d %s", name, code, body)
		}
		if m["status"] == "running" || m["clean_success"] != nil || strings.Contains(body, "running") {
			t.Fatalf("%s: live claim: %s", name, body)
		}
	}
}

func TestShadowStatusCountsAndReasonsClosedSet(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, shFlags(true, false, false, false, map[string]interface{}{
		"counts": map[string]interface{}{"eligible": 4, "outcome_fallback": 2, "AHS-0001": 9, "/tmp/secret/cache": 1,
			"alias_deadbeef": 3, "out_seen": "5", "out_verified": -1, "not_eligible": 1.5},
		"fallback_reasons": map[string]interface{}{"timeout_x": 1, "cache_miss": 2, "provider_http_503": 1, "provider_http_<script>": 4,
			"/home/u/leak": 1, "staging_error:valueerror": 1, "staging_error:/tmp/leak": 1, "bounds_violation": "2",
			"patient Jane Doe": 1},
	}), "2026-11-08T10:00:00Z")
	_, m, body := shadowStatusGet(t, run, "GET")
	for _, bad := range []string{"AHS-0001", "/tmp/secret", "alias_deadbeef", "/home/u", "Jane", "<script>", "timeout_x", "/tmp/leak", "valueerror"} {
		if strings.Contains(body, bad) {
			t.Fatalf("leaked %q: %s", bad, body)
		}
	}
	c, _ := m["counts"].(map[string]interface{})
	if len(c) != 2 || c["eligible"] != float64(4) || c["outcome_fallback"] != float64(2) {
		t.Fatalf("counts: %v", c)
	}
	r, _ := m["fallback_reasons"].(map[string]interface{})
	if len(r) != 3 || r["cache_miss"] != float64(2) || r["provider_http_503"] != float64(1) || r["staging_error"] != float64(1) {
		t.Fatalf("reasons: %v", r)
	}
	// wrong container types are dropped, not copied
	run2 := t.TempDir()
	writeShadowFiles(t, run2, shFlags(true, false, false, false, map[string]interface{}{"counts": []string{"AHS-1"}, "fallback_reasons": "AHS-2"}), "2026-11-08T10:00:00Z")
	_, m2, body2 := shadowStatusGet(t, run2, "GET")
	if strings.Contains(body2, "AHS-") || m2["counts"] != nil || m2["fallback_reasons"] != nil {
		t.Fatalf("container leak: %s", body2)
	}
}

func TestShadowStatusScalarFieldsValidated(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, shFlags(false, false, false, false, map[string]interface{}{"mode": "/tmp/leak", "items_attempted": "AHS-1", "max_items": 20}), "2026-11-08T10:00:00Z")
	_, m, body := shadowStatusGet(t, run, "GET")
	if strings.Contains(body, "/tmp/leak") || strings.Contains(body, "AHS-1") || m["max_items"] != float64(20) {
		t.Fatalf("scalar: %s", body)
	}
}

func TestShadowStatusInvalidHealthTimestampOmitted(t *testing.T) {
	for name, raw := range map[string]string{"bad ts": `{"ts":"/tmp/leak/AHS-1"}`, "number ts": `{"ts":5}`, "no ts": `{}`,
		"malformed": `{"ts":`, "empty ts": `{"ts":""}`} {
		run := t.TempDir()
		writeShadowFiles(t, run, shFlags(false, false, false, false, nil), "")
		os.MkdirAll(filepath.Join(run, "agent_shadow"), 0o755)
		os.WriteFile(filepath.Join(run, "agent_shadow", "health.json"), []byte(raw), 0o644)
		code, m, body := shadowStatusGet(t, run, "GET")
		if code != 200 || m["available"] != true || m["status"] != "running" {
			t.Fatalf("%s: %d %s", name, code, body)
		}
		if _, ok := m["health"]; ok || strings.Contains(body, "leak") {
			t.Fatalf("%s: health must be omitted: %s", name, body)
		}
	}
}
