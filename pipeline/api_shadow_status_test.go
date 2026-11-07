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
	writeShadowFiles(t, run, map[string]interface{}{"done": false, "items_attempted": 1}, "2026-11-08T10:00:00Z")
	_, a, _ := shadowStatusGet(t, run, "GET")
	writeShadowFiles(t, run, map[string]interface{}{"done": true, "items_attempted": 7, "degraded": true}, "2026-11-08T10:05:00Z")
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
	writeShadowFiles(t, run, map[string]interface{}{"done": false}, "")
	_, m, _ := shadowStatusGet(t, run, "GET")
	if m["available"] != true {
		t.Fatalf("%v", m)
	}
	if h, ok := m["health"].(map[string]interface{}); ok && h["ts"] != nil {
		t.Fatalf("no health expected: %v", m["health"])
	}
}

func TestShadowStatusGETOnly(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, map[string]interface{}{"done": true}, "2026-11-08T10:00:00Z")
	for _, method := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		code, _, _ := shadowStatusGet(t, run, method)
		if code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: %d", method, code)
		}
	}
}

func TestShadowStatusLeaksNothing(t *testing.T) {
	run := t.TempDir()
	writeShadowFiles(t, run, map[string]interface{}{"done": true, "degraded": true, "degraded_reason": "cache /tmp/x/cache failed",
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
