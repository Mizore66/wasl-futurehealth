package main

// Bounded, in-memory API request observations for GET /api/operations (issue #91).
//
// The observer records only a fixed low-cardinality route label, the HTTP status class and the handler duration of each
// completed request. It never records paths beyond the route template, query strings, case or person identifiers,
// actors, roles, credentials, headers or bodies. Samples live only in this process's memory: a restart starts from an
// empty window and cannot inherit earlier samples. The values are point-in-time observations of synthetic demo traffic,
// not an accumulated SLO, centralized monitoring or a production-readiness claim.

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	apiObservationWindow   = 5 * time.Minute
	apiObservationCapacity = 4096
	apiObservationScope    = "current_api_process"
	apiObservationSource   = "in_memory_request_observer"
	apiObservationOther    = "other"
)

// apiObservationRoutes is the complete, closed set of route labels. Any other path is recorded as "/api/other" or
// "other", so an attacker-chosen URL cannot create new labels.
var apiObservationRoutes = []string{
	"/healthz", "/api/operations", "/api/dashboard", "/api/cases", "/api/fixtures", "/api/people", "/api/agentshadow/status",
	"/api/patient/journey", "/api/patient/verification", "/api/patient/requests", "/api/patient/outcome",
}

var apiObservationPrefixes = []struct{ prefix, label string }{
	{"/api/cases/", "/api/cases/:case"}, {"/api/people/", "/api/people/:person"}, {"/api/recovery/", "/api/recovery/:path"},
}

// apiRouteLabel maps a request path to one of a fixed set of labels without retaining any identifier.
func apiRouteLabel(path string) string {
	for _, route := range apiObservationRoutes {
		if path == route {
			return route
		}
	}
	for _, p := range apiObservationPrefixes {
		if strings.HasPrefix(path, p.prefix) {
			return p.label
		}
	}
	if strings.HasPrefix(path, "/api/") {
		return "/api/other"
	}
	return apiObservationOther
}

type apiRequestSample struct {
	at       time.Time // completion time
	route    string
	status   int
	duration time.Duration
}

// apiRequestObserver is a fixed-capacity ring of recent request samples, safe for concurrent use.
type apiRequestObserver struct {
	mu        sync.Mutex
	now       func() time.Time
	started   time.Time
	window    time.Duration
	samples   []apiRequestSample // ring buffer, len <= capacity
	next      int                // ring write position once full
	capacity  int
	evictedAt time.Time // completion time of the newest sample overwritten for capacity; zero if none
}

func newAPIRequestObserver(now func() time.Time) *apiRequestObserver {
	return &apiRequestObserver{now: now, started: now(), window: apiObservationWindow, capacity: apiObservationCapacity}
}

func (o *apiRequestObserver) record(sample apiRequestSample) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.samples) < o.capacity {
		o.samples = append(o.samples, sample)
		return
	}
	if evicted := o.samples[o.next].at; evicted.After(o.evictedAt) {
		o.evictedAt = evicted
	}
	o.samples[o.next] = sample
	o.next = (o.next + 1) % o.capacity
}

// observedResponseWriter captures the status and whether anything was written, while forwarding every write and
// keeping optional interfaces reachable through Unwrap (http.ResponseController) and Flush.
type observedResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *observedResponseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = status, true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.status, w.wroteHeader = http.StatusOK, true
	}
	return w.ResponseWriter.Write(body)
}

func (w *observedResponseWriter) Flush() {
	if !w.wroteHeader {
		w.status, w.wroteHeader = http.StatusOK, true
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *observedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// observe wraps the externally served handler. A handler that panics is recorded as a 500 before the panic continues
// to net/http, so a crash is never silently missing from the error rate.
func (o *apiRequestObserver) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := o.now()
		route := apiRouteLabel(r.URL.Path)
		recorder := &observedResponseWriter{ResponseWriter: w}
		completed := false
		defer func() {
			status := recorder.status
			if !completed {
				status = http.StatusInternalServerError
			} else if !recorder.wroteHeader {
				status = http.StatusOK // net/http sends 200 for a handler that writes nothing
			}
			finished := o.now()
			duration := finished.Sub(started)
			if duration < 0 {
				duration = 0
			}
			o.record(apiRequestSample{at: finished, route: route, status: status, duration: duration})
		}()
		next.ServeHTTP(recorder, r)
		completed = true
	})
}

type operationsAPIWindow struct {
	Scope            string `json:"scope"`
	Source           string `json:"source"`
	ProcessStartedAt string `json:"process_started_at"`
	Start            string `json:"start"`
	End              string `json:"end"`
	StartBound       string `json:"start_bound"`
	LengthSeconds    int    `json:"length_seconds"`
	Capacity         int    `json:"capacity"`
	Samples          int    `json:"samples"`
	LatestSampleAt   string `json:"latest_sample_at,omitempty"`
}

type operationsAPILatency struct {
	Status  string   `json:"status"`
	Reason  string   `json:"reason,omitempty"`
	Unit    string   `json:"unit,omitempty"`
	Samples *int     `json:"samples,omitempty"`
	P50     *float64 `json:"p50,omitempty"`
	P90     *float64 `json:"p90,omitempty"`
	P99     *float64 `json:"p99,omitempty"`
	Max     *float64 `json:"max,omitempty"`
}

type operationsAPIErrorRate struct {
	Status       string   `json:"status"`
	Reason       string   `json:"reason,omitempty"`
	Definition   string   `json:"definition,omitempty"`
	Samples      *int     `json:"samples,omitempty"`
	ServerErrors *int     `json:"server_errors,omitempty"`
	ClientErrors *int     `json:"client_errors,omitempty"`
	Rate         *float64 `json:"rate,omitempty"`
}

type operationsAPIRoute struct {
	Route        string `json:"route"`
	Requests     int    `json:"requests"`
	ServerErrors int    `json:"server_errors"`
	ClientErrors int    `json:"client_errors"`
}

type operationsAPIMeasurements struct {
	Window    *operationsAPIWindow   `json:"window,omitempty"`
	Latency   operationsAPILatency   `json:"latency"`
	ErrorRate operationsAPIErrorRate `json:"error_rate"`
	Routes    []operationsAPIRoute   `json:"routes,omitempty"`
}

// unrecordedAPIMeasurements is used when no observer wraps the handler (in-process tests, published snapshots).
func unrecordedAPIMeasurements() operationsAPIMeasurements {
	return operationsAPIMeasurements{
		Latency:   operationsAPILatency{Status: "unmeasured", Reason: "api_request_latency_not_recorded"},
		ErrorRate: operationsAPIErrorRate{Status: "unmeasured", Reason: "api_request_outcomes_not_recorded"},
	}
}

func observationMillis(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Microsecond)) / 1000
}

// nearestRank returns the nearest-rank percentile of sorted durations.
func nearestRank(sorted []time.Duration, percentile float64) float64 {
	rank := int(math.Ceil(percentile / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return observationMillis(sorted[rank-1])
}

func intPointer(v int) *int              { return &v }
func floatPointer(v float64) *float64    { return &v }
func observationTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// snapshot summarises the samples completed in [start, end] ((start, end] for a capacity bound), where end is the observation time and start is the latest
// of the rolling-window boundary, the process start and the newest capacity eviction.
func (o *apiRequestObserver) snapshot(end time.Time) operationsAPIMeasurements {
	o.mu.Lock()
	samples := append([]apiRequestSample(nil), o.samples...)
	started, evictedAt, window, capacity := o.started, o.evictedAt, o.window, o.capacity
	o.mu.Unlock()

	start, bound := end.Add(-window), "rolling_window"
	if started.After(start) {
		start, bound = started, "process_start"
	}
	if !evictedAt.IsZero() && !evictedAt.Before(start) {
		start, bound = evictedAt, "capacity"
	}
	inWindow := samples[:0]
	var latest time.Time
	for _, sample := range samples {
		// The capacity bound is exclusive: a sample completed at the newest eviction time may have been dropped.
		afterStart := !sample.at.Before(start)
		if bound == "capacity" {
			afterStart = sample.at.After(start)
		}
		if afterStart && !sample.at.After(end) {
			inWindow = append(inWindow, sample)
			if sample.at.After(latest) {
				latest = sample.at
			}
		}
	}
	out := operationsAPIMeasurements{Window: &operationsAPIWindow{
		Scope: apiObservationScope, Source: apiObservationSource, ProcessStartedAt: observationTime(started),
		Start: observationTime(start), End: observationTime(end), StartBound: bound,
		LengthSeconds: int(window / time.Second), Capacity: capacity, Samples: len(inWindow),
	}, Routes: []operationsAPIRoute{}}
	if len(inWindow) == 0 {
		out.Latency = operationsAPILatency{Status: "unmeasured", Reason: "no_requests_in_window"}
		out.ErrorRate = operationsAPIErrorRate{Status: "unmeasured", Reason: "no_requests_in_window"}
		return out
	}
	out.Window.LatestSampleAt = observationTime(latest)
	durations := make([]time.Duration, len(inWindow))
	routes := map[string]*operationsAPIRoute{}
	serverErrors, clientErrors := 0, 0
	for i, sample := range inWindow {
		durations[i] = sample.duration
		route := routes[sample.route]
		if route == nil {
			route = &operationsAPIRoute{Route: sample.route}
			routes[sample.route] = route
		}
		route.Requests++
		switch {
		case sample.status >= 500:
			serverErrors++
			route.ServerErrors++
		case sample.status >= 400:
			clientErrors++
			route.ClientErrors++
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	n := len(durations)
	out.Latency = operationsAPILatency{
		Status: "measured", Unit: "ms", Samples: intPointer(n),
		P50: floatPointer(nearestRank(durations, 50)), P90: floatPointer(nearestRank(durations, 90)),
		P99: floatPointer(nearestRank(durations, 99)), Max: floatPointer(observationMillis(durations[n-1])),
	}
	out.ErrorRate = operationsAPIErrorRate{
		Status: "measured", Definition: "http_5xx_responses_per_request", Samples: intPointer(n),
		ServerErrors: intPointer(serverErrors), ClientErrors: intPointer(clientErrors),
		Rate: floatPointer(math.Round(float64(serverErrors)/float64(n)*1e6) / 1e6),
	}
	for _, route := range routes {
		out.Routes = append(out.Routes, *route)
	}
	sort.Slice(out.Routes, func(i, j int) bool { return out.Routes[i].Route < out.Routes[j].Route })
	return out
}
