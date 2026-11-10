package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestRuntimeMetricsReportsGoRuntimeAndProcessSources(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/debug/runtime", nil)
	w := httptest.NewRecorder()
	runtimeMetricsHandler(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got struct {
		Schema         string `json:"schema"`
		Goroutines     int    `json:"goroutines"`
		HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
		HeapSysBytes   uint64 `json:"heap_sys_bytes"`
		GCCount        uint32 `json:"gc_count"`
		GCPauseTotalNS uint64 `json:"gc_pause_total_ns"`
		OpenFDs        int    `json:"open_fds"`
		ThreadCount    int    `json:"thread_count"`
		RSSBytes       uint64 `json:"rss_bytes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Schema != "wasl-runtime-metrics/1" || got.Goroutines < 1 || got.HeapAllocBytes == 0 || got.HeapSysBytes == 0 {
		t.Fatalf("missing runtime metrics: %+v", got)
	}
	if runtime.GOOS == "linux" && (got.OpenFDs < 1 || got.ThreadCount < 1 || got.RSSBytes == 0) {
		t.Fatalf("missing Linux process metrics: %+v", got)
	}
}

func TestRuntimeMetricsRejectsNonGET(t *testing.T) {
	w := httptest.NewRecorder()
	runtimeMetricsHandler(w, httptest.NewRequest(http.MethodPost, "/debug/runtime", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestRuntimeMetricsListenerRefusesNonLoopbackAddress(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("startRuntimeMetrics accepted a non-loopback listener")
		}
	}()
	startRuntimeMetrics("0.0.0.0:0")
}
