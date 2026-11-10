package main

// Loopback-only runtime/process metrics for the external soak collector.
// No run, cohort, patient, key, or request data is exposed by this handler.

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const runtimeMetricsTimeout = 5 * time.Second

type runtimeMetrics struct {
	Schema         string `json:"schema"`
	Goroutines     int    `json:"goroutines"`
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64 `json:"heap_sys_bytes"`
	HeapObjects    uint64 `json:"heap_objects"`
	GCCount        uint32 `json:"gc_count"`
	GCPauseTotalNS uint64 `json:"gc_pause_total_ns"`
	OpenFDs        int    `json:"open_fds"`
	ThreadCount    int    `json:"thread_count"`
	RSSBytes       uint64 `json:"rss_bytes"`
}

func runtimeMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	m := runtimeMetrics{Schema: "wasl-runtime-metrics/1", Goroutines: runtime.NumGoroutine(),
		HeapAllocBytes: mem.Alloc, HeapSysBytes: mem.HeapSys, HeapObjects: mem.HeapObjects,
		GCCount: mem.NumGC, GCPauseTotalNS: mem.PauseTotalNs, OpenFDs: -1, ThreadCount: -1}
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		m.OpenFDs = len(entries)
	}
	if status, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(status), "\n") {
			if strings.HasPrefix(line, "Threads:") {
				m.ThreadCount, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Threads:")))
			}
		}
	}
	if statm, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(statm))
		if len(fields) >= 2 {
			if pages, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				m.RSSBytes = pages * uint64(os.Getpagesize())
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}

func startRuntimeMetrics(addr string) {
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		panic("invalid WASL_RUNTIME_ADDR: " + err.Error())
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		panic("WASL_RUNTIME_ADDR must be a numeric loopback address")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/runtime", runtimeMetricsHandler)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		panic("runtime metrics listener: " + err.Error())
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: runtimeMetricsTimeout}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
}
