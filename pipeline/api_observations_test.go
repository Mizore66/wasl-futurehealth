package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// steppingClock returns a deterministic clock whose next reading is set by the caller.
type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppingClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *steppingClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func observationsBase() time.Time { return time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC) }

func TestAPIRouteLabelIsClosedAndDropsIdentifiers(t *testing.T) {
	cases := map[string]string{
		"/healthz":                          "/healthz",
		"/api/operations":                   "/api/operations",
		"/api/cases":                        "/api/cases",
		"/api/cases/loop-crc-PATIENT-123":   "/api/cases/:case",
		"/api/cases/loop-1/review?actor=x":  "/api/cases/:case",
		"/api/people/person-42":             "/api/people/:person",
		"/api/recovery/confirm":             "/api/recovery/:path",
		"/api/anything-attacker-chose-9f3a": "/api/other",
		"/favicon.ico":                      "other",
		"/":                                 "other",
	}
	for path, want := range cases {
		if got := apiRouteLabel(path); got != want {
			t.Errorf("apiRouteLabel(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestObservedResponseWriterCapturesStatusAndPreservesWriterBehaviour(t *testing.T) {
	clock := &steppingClock{now: observationsBase()}
	o := newAPIRequestObserver(clock.read)
	handlers := []struct {
		name string
		want int
		h    http.HandlerFunc
	}{
		{"explicit 503", 503, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); _, _ = w.Write([]byte("x")) }},
		{"implicit 200 on write", 200, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }},
		{"no write", 200, func(w http.ResponseWriter, r *http.Request) {}},
		{"first header wins", 404, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404); w.WriteHeader(500) }},
		{"flush then write", 200, func(w http.ResponseWriter, r *http.Request) {
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("flush through observer: %v", err)
			}
			_, _ = w.Write([]byte("streamed"))
		}},
	}
	for _, tc := range handlers {
		rec := httptest.NewRecorder()
		o.observe(tc.h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
		if rec.Code != tc.want {
			t.Errorf("%s: client saw %d, want %d", tc.name, rec.Code, tc.want)
		}
		got := o.samples[len(o.samples)-1].status
		if got != tc.want {
			t.Errorf("%s: recorded status %d, want %d", tc.name, got, tc.want)
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("observer swallowed a handler panic")
			}
		}()
		o.observe(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/cases", nil))
	}()
	if got := o.samples[len(o.samples)-1]; got.status != 500 || got.route != "/api/cases" {
		t.Errorf("panicking handler recorded as %+v, want a 500 on /api/cases", got)
	}
}

// TestOperationsMeasuresControlledSyntheticRequests serves synthetic requests with known durations and statuses through
// the authenticated handler and checks every reported value against hand-computed expectations.
func TestOperationsMeasuresControlledSyntheticRequests(t *testing.T) {
	s := operationsTestServer(t)
	availableOperationsProbe(t)
	clock := &steppingClock{now: observationsBase()}
	s.requests = newAPIRequestObserver(clock.read)
	oldNow := operationsNow
	operationsNow = clock.read
	t.Cleanup(func() { operationsNow = oldNow })
	const token = "staff-token-123456"
	inner, err := s.handler(token+":coordinator:coordinator-one", "")
	if err != nil {
		t.Fatal(err)
	}
	// The inner handler advances the clock by the planned duration and answers with the planned status, so the
	// observer's measured durations are exactly the planned ones.
	plan := []struct {
		path   string
		status int
		ms     int
	}{}
	for i := 1; i <= 10; i++ {
		status := 200
		switch i {
		case 4:
			status = 404
		case 9, 10:
			status = 503
		}
		plan = append(plan, struct {
			path   string
			status int
			ms     int
		}{fmt.Sprintf("/api/cases/%s/probe-%d", contractID("operations"), i), status, i * 10})
	}
	step := 0
	scripted := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/operations" {
			inner.ServeHTTP(w, r)
			return
		}
		p := plan[step]
		step++
		clock.advance(time.Duration(p.ms) * time.Millisecond)
		w.WriteHeader(p.status)
	})
	served := s.requests.observe(scripted)
	for range plan {
		rec := httptest.NewRecorder()
		served.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, plan[step].path, nil))
	}
	// One unauthenticated request through the real handler: a client error with zero duration on the operations route.
	served.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/operations", nil))
	clock.advance(time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	served.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operations: %d %s", rec.Code, rec.Body.String())
	}
	var body operationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	m := body.APIMeasurements
	if m.Window == nil || m.Window.Samples != 11 || m.Window.StartBound != "process_start" || m.Window.Capacity != apiObservationCapacity || m.Window.LengthSeconds != 300 {
		t.Fatalf("window = %+v", m.Window)
	}
	if m.Window.ProcessStartedAt != "2026-11-17T09:00:00Z" || m.Window.Start != "2026-11-17T09:00:00Z" || m.Window.End != "2026-11-17T09:00:01.55Z" || m.Window.LatestSampleAt != "2026-11-17T09:00:00.55Z" {
		t.Fatalf("window boundaries = %+v", m.Window)
	}
	// Durations: 0 (the 401) and 10..100 ms. Nearest rank over 11 samples: p50 = 6th = 50, p90 = 10th = 90, p99 = 11th = 100.
	l := m.Latency
	if l.Status != "measured" || l.Unit != "ms" || *l.Samples != 11 || *l.P50 != 50 || *l.P90 != 90 || *l.P99 != 100 || *l.Max != 100 {
		t.Fatalf("latency = %+v p50=%v p90=%v p99=%v max=%v", l, *l.P50, *l.P90, *l.P99, *l.Max)
	}
	e := m.ErrorRate
	if e.Status != "measured" || e.Definition != "http_5xx_responses_per_request" || *e.Samples != 11 || *e.ServerErrors != 2 || *e.ClientErrors != 2 || *e.Rate != 0.181818 {
		t.Fatalf("error rate = %+v rate=%v", e, *e.Rate)
	}
	wantRoutes := []operationsAPIRoute{{Route: "/api/cases/:case", Requests: 10, ServerErrors: 2, ClientErrors: 1}, {Route: "/api/operations", Requests: 1, ClientErrors: 1}}
	if fmt.Sprint(m.Routes) != fmt.Sprint(wantRoutes) {
		t.Fatalf("routes = %+v, want %+v", m.Routes, wantRoutes)
	}
	for _, secret := range []string{contractID("operations"), "probe-", "coordinator-one", token} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("operations observations leaked %q", secret)
		}
	}
}

func TestAPIObservationsEmptyAndExpiredWindowsStayUnmeasured(t *testing.T) {
	clock := &steppingClock{now: observationsBase()}
	o := newAPIRequestObserver(clock.read)
	empty := o.snapshot(clock.read())
	if empty.Latency.Status != "unmeasured" || empty.Latency.Reason != "no_requests_in_window" || empty.Latency.P50 != nil ||
		empty.ErrorRate.Status != "unmeasured" || empty.ErrorRate.Reason != "no_requests_in_window" || empty.ErrorRate.Rate != nil ||
		empty.Window.Samples != 0 || empty.Window.LatestSampleAt != "" || len(empty.Routes) != 0 {
		t.Fatalf("empty window invented values: %+v", empty)
	}
	raw, _ := json.Marshal(empty)
	if strings.Contains(string(raw), `"p50"`) || strings.Contains(string(raw), `"rate"`) {
		t.Fatalf("empty window serialised zero metrics: %s", raw)
	}
	o.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	clock.advance(apiObservationWindow + time.Second)
	expired := o.snapshot(clock.read())
	if expired.Latency.Status != "unmeasured" || expired.ErrorRate.Status != "unmeasured" || expired.Window.Samples != 0 || expired.Window.StartBound != "rolling_window" {
		t.Fatalf("expired sample still measured: %+v", expired)
	}
}

func TestAPIObservationsRestartDoesNotInheritSamples(t *testing.T) {
	clock := &steppingClock{now: observationsBase()}
	before := newAPIRequestObserver(clock.read)
	for i := 0; i < 5; i++ {
		before.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/cases", nil))
	}
	clock.advance(time.Second)
	if got := before.snapshot(clock.read()); got.Window.Samples != 5 {
		t.Fatalf("pre-restart samples = %d", got.Window.Samples)
	}
	// A new process builds a new observer; there is no persisted state for it to read.
	clock.advance(time.Second)
	after := newAPIRequestObserver(clock.read)
	clock.advance(time.Second)
	got := after.snapshot(clock.read())
	if got.Window.Samples != 0 || got.Latency.Status != "unmeasured" || got.ErrorRate.Status != "unmeasured" ||
		got.Window.StartBound != "process_start" || got.Window.ProcessStartedAt != "2026-11-17T09:00:02Z" || got.Window.Start != got.Window.ProcessStartedAt {
		t.Fatalf("restarted observer inherited state: %+v", got.Window)
	}
	// A server reloaded from disk has no observer until serve attaches a fresh one.
	s := operationsTestServer(t)
	if s.requests != nil {
		t.Fatal("loaded API server carried a request observer")
	}
	_, body := operationsRequest(t, s)
	if body.APIMeasurements.Window != nil || body.APIMeasurements.Latency.Reason != "api_request_latency_not_recorded" || body.APIMeasurements.ErrorRate.Reason != "api_request_outcomes_not_recorded" {
		t.Fatalf("unobserved handler reported measurements: %+v", body.APIMeasurements)
	}
}

func TestAPIObservationsCapacityBoundsWindowHonestly(t *testing.T) {
	clock := &steppingClock{now: observationsBase()}
	o := newAPIRequestObserver(clock.read)
	o.capacity = 3
	h := o.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { clock.advance(time.Millisecond) }))
	for i := 0; i < 5; i++ {
		clock.advance(time.Second)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/dashboard", nil))
	}
	if len(o.samples) != 3 {
		t.Fatalf("ring grew to %d samples", len(o.samples))
	}
	got := o.snapshot(clock.read())
	// Samples 1 and 2 were overwritten; the window starts at the newest evicted sample (09:00:02.001).
	if got.Window.Samples != 3 || got.Window.StartBound != "capacity" || got.Window.Start != "2026-11-17T09:00:02.002Z" {
		t.Fatalf("capacity window = %+v", got.Window)
	}
}

func TestAPIObservationsConcurrentRecordAndSnapshot(t *testing.T) {
	o := newAPIRequestObserver(time.Now)
	o.capacity = 64
	h := o.observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fail") != "" {
			w.WriteHeader(500)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(2)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				path := "/api/cases"
				if i%5 == 0 {
					path += "?fail=1"
				}
				h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
			}
		}(g)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = o.snapshot(time.Now())
			}
		}()
	}
	wg.Wait()
	got := o.snapshot(time.Now())
	// Completion times can tie or be recorded out of order across goroutines, so retained samples at or before the
	// newest eviction are honestly excluded rather than reported as within the window.
	if got.Window.Samples < 1 || got.Window.Samples > 64 || got.Window.StartBound != "capacity" || *got.ErrorRate.Samples != got.Window.Samples ||
		*got.ErrorRate.ServerErrors > got.Window.Samples {
		t.Fatalf("concurrent snapshot = %+v", got.Window)
	}
}
