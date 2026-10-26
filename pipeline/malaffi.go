package main

// Mock Malaffi (Abu Dhabi HIE) FHIR R4 server with Subscriptions Backport IG semantics, for testing the consumer.
//   POST /fhir/Subscription                         create (rest-hook; payload full-resource | id-only); handshake sent
//   GET  /fhir/Subscription/{id}/$status            SubscriptionStatus (events-since-subscription-start)
//   GET  /fhir/Subscription/{id}/$events?eventsSinceNumber=&eventsUntilNumber=   replay of past notifications (gap fill)
//   GET  /fhir/{type}/{id}/_history/{vid}           vread (id-only payloads)
//   GET  /fhir/{type}/{id}                          read (latest version raised so far)
// Every resource version in the v5 cohort becomes one topic event, numbered in global HIE arrival order (the same
// stable order the v5 offline `sim` uses). Events are released on the arrival clock (speed = simulated days per wall
// second; 0 = as fast as delivery allows) and pushed as history Bundles. Injected faults on the push channel: duplicate
// notifications, reordered notifications, dropped notifications; a failed POST (consumer down) is NOT retried -- the
// consumer must recover the gap through $status/$events. All REST calls require a bearer token.
import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type hieEvent struct {
	Num      int64
	Arrival  float64
	Type, ID string
	Vid      string
	Body     []byte
	occurred time.Time // simulated HIE receipt instant (event-time; carried in notification-event.timestamp)
	relWall  int64     // wall-clock ns when the HIE raised the event (for notification-to-NATS latency)
}

type mockSub struct {
	ID, Endpoint, HookHdr, Content string
	MaxCount                       int
	Heartbeat                      time.Duration
	status                         string
	released                       int64 // events raised so far (events-since-subscription-start)
}

type Malaffi struct {
	mu      sync.Mutex
	events  []*hieEvent
	byKey   map[string]*hieEvent // Type/id/vid
	latest  map[string]*hieEvent // Type/id -> latest raised version
	sub     *mockSub
	speed   float64
	faults  struct{ dup, reorder, drop float64 }
	rng     *rand.Rand
	stats   map[string]int
	runDir  string
	client  *http.Client
	started time.Time
}

func loadHIE(dataDir string) []*hieEvent {
	all := []GenEvent{}
	for _, pc := range Providers {
		f, err := os.Open(filepath.Join(dataDir, "fhir_"+pc.ID+".ndjson"))
		if err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			all = append(all, g)
		}
		f.Close()
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Arrival < all[j].Arrival })
	out := make([]*hieEvent, len(all))
	for i, g := range all {
		meta := g.Res["meta"].(map[string]interface{})
		b, _ := json.Marshal(g.Res) // byte-identical to what the v5 producer publishes
		out[i] = &hieEvent{Num: int64(i + 1), Arrival: g.Arrival, Type: g.Res["resourceType"].(string), ID: g.Res["id"].(string),
			Vid: meta["versionId"].(string), Body: b, occurred: baseTime.Add(time.Duration(math.Round(g.Arrival * 86400 * 1e9)))}
	}
	return out
}

func runMalaffi(dataDir, addr string, speed float64, seed int64, dup, reorder, drop float64, runDir string) {
	m := &Malaffi{events: loadHIE(dataDir), byKey: map[string]*hieEvent{}, latest: map[string]*hieEvent{}, speed: speed,
		rng: rand.New(rand.NewSource(seed)), stats: map[string]int{}, runDir: runDir, client: &http.Client{Timeout: 5 * time.Second}}
	m.faults.dup, m.faults.reorder, m.faults.drop = dup, reorder, drop
	for _, e := range m.events {
		m.byKey[e.Type+"/"+e.ID+"/"+e.Vid] = e
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/fhir/", m.handle)
	go func() {
		for {
			m.mu.Lock()
			cp := map[string]int{}
			for k, v := range m.stats {
				cp[k] = v
			}
			st := map[string]interface{}{"events": len(m.events), "stats": cp}
			if m.sub != nil {
				st["released"], st["status"] = m.sub.released, m.sub.status
			}
			m.mu.Unlock()
			writeStatus(runDir, "malaffi", st)
			time.Sleep(500 * time.Millisecond)
		}
	}()
	fmt.Fprintf(os.Stderr, "mock malaffi: %d topic events on %s (speed %.2f d/s, faults dup %.2f reorder %.2f drop %.2f)\n",
		len(m.events), addr, speed, dup, reorder, drop)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func writeFHIR(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func outcome(code, msg string) map[string]interface{} {
	return map[string]interface{}{"resourceType": "OperationOutcome", "issue": []interface{}{map[string]string{"severity": "error", "code": code, "diagnostics": msg}}}
}

func (m *Malaffi) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+MockToken {
		m.mu.Lock()
		m.stats["auth_rejected"]++
		m.mu.Unlock()
		writeFHIR(w, 401, outcome("security", "missing or invalid bearer token"))
		return
	}
	p := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/fhir/"), "/"), "/")
	switch {
	case r.Method == "POST" && len(p) == 1 && p[0] == "Subscription":
		m.createSub(w, r)
	case r.Method == "GET" && len(p) == 3 && p[0] == "Subscription" && p[2] == "$status":
		m.mu.Lock()
		if m.sub == nil || m.sub.ID != p[1] {
			m.mu.Unlock()
			writeFHIR(w, 404, outcome("not-found", "no such subscription"))
			return
		}
		st := m.statusLocked("query-status", nil)
		m.stats["status_queries"]++
		m.mu.Unlock()
		sb, _ := json.Marshal(st.toParams())
		writeFHIR(w, 200, Bundle{ResourceType: "Bundle", Type: "searchset", Entry: []BEntry{{Resource: sb}}})
	case r.Method == "GET" && len(p) == 3 && p[0] == "Subscription" && p[2] == "$events":
		m.events_(w, r, p[1])
	case r.Method == "GET" && (len(p) == 2 || (len(p) == 4 && p[2] == "_history")):
		m.mu.Lock()
		var e *hieEvent
		if len(p) == 4 {
			if x := m.byKey[p[0]+"/"+p[1]+"/"+p[3]]; x != nil && x.Num <= m.releasedLocked() {
				e = x
			}
		} else {
			e = m.latest[p[0]+"/"+p[1]]
		}
		m.stats["reads"]++
		m.mu.Unlock()
		if e == nil {
			writeFHIR(w, 404, outcome("not-found", r.URL.Path))
			return
		}
		w.Header().Set("Content-Type", "application/fhir+json")
		w.Header().Set("ETag", `W/"`+e.Vid+`"`)
		w.Write(e.Body)
	default:
		writeFHIR(w, 400, outcome("not-supported", r.Method+" "+r.URL.Path))
	}
}

func (m *Malaffi) releasedLocked() int64 {
	if m.sub == nil {
		return 0
	}
	return m.sub.released
}

func (m *Malaffi) statusLocked(typ string, evs []*hieEvent) StatusInfo {
	s := StatusInfo{Sub: "Subscription/" + m.sub.ID, Topic: MalaffiTopic, Status: m.sub.status, Type: typ, EventsSince: m.sub.released}
	for _, e := range evs {
		s.Events = append(s.Events, NotifEvent{Num: e.Num, Focus: e.Type + "/" + e.ID + "/_history/" + e.Vid, Occurred: e.occurred.Format(time.RFC3339Nano)})
	}
	return s
}

func (m *Malaffi) createSub(w http.ResponseWriter, r *http.Request) {
	var sub map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || gs(sub, "resourceType") != "Subscription" {
		writeFHIR(w, 400, outcome("invalid", "not a Subscription"))
		return
	}
	ch, _ := sub["channel"].(map[string]interface{})
	if gs(ch, "type") != "rest-hook" || gs(sub, "criteria") != MalaffiTopic {
		writeFHIR(w, 422, outcome("not-supported", "only rest-hook on "+MalaffiTopic))
		return
	}
	ms := &mockSub{ID: fmt.Sprintf("wsi-crc-%d", time.Now().UnixNano()%1e9), Endpoint: gs(ch, "endpoint"), Content: "id-only",
		MaxCount: 50, Heartbeat: time.Second, status: "requested"}
	if hs, ok := ch["header"].([]interface{}); ok && len(hs) > 0 {
		ms.HookHdr, _ = hs[0].(string)
	}
	exts := func(v interface{}) {
		xs, _ := v.([]interface{})
		for _, x := range xs {
			xm := x.(map[string]interface{})
			switch gs(xm, "url") {
			case ExtPayloadContent:
				ms.Content = gs(xm, "valueCode")
			case ExtMaxCount:
				if f, ok := xm["valuePositiveInt"].(float64); ok {
					ms.MaxCount = int(f)
				}
			case ExtHeartbeat:
				if f, ok := xm["valueUnsignedInt"].(float64); ok {
					ms.Heartbeat = time.Duration(f * float64(time.Second))
				}
			}
		}
	}
	exts(ch["extension"])
	if pl, ok := ch["_payload"].(map[string]interface{}); ok {
		exts(pl["extension"])
	}
	m.mu.Lock()
	if m.sub != nil {
		m.mu.Unlock()
		writeFHIR(w, 409, outcome("duplicate", "subscription exists: "+m.sub.ID))
		return
	}
	m.sub = ms
	m.mu.Unlock()
	sub["id"], sub["status"] = ms.ID, "requested"
	writeFHIR(w, 201, sub)
	go m.deliver()
}

func (m *Malaffi) bundle(st StatusInfo, evs []*hieEvent, full bool) []byte {
	sb, _ := json.Marshal(st.toParams())
	b := Bundle{ResourceType: "Bundle", ID: fmt.Sprintf("n-%d-%d", st.EventsSince, time.Now().UnixNano()), Type: "history",
		Meta: map[string]interface{}{"profile": []string{ProfNotifyR4}}, Timestamp: time.Now().Format(time.RFC3339Nano),
		Entry: []BEntry{{FullURL: "urn:uuid:status-" + strconv.FormatInt(st.EventsSince, 10), Resource: sb, Request: &BReq{"GET", st.Sub + "/$status"}, Response: &BResp{Status: "200"}}}}
	for _, e := range evs {
		en := BEntry{FullURL: "https://malaffi.mock/fhir/" + e.Type + "/" + e.ID + "/_history/" + e.Vid,
			Request: &BReq{"PUT", e.Type + "/" + e.ID}, Response: &BResp{Status: "200", Etag: `W/"` + e.Vid + `"`}}
		if full {
			en.Resource = e.Body
		}
		b.Entry = append(b.Entry, en)
	}
	out, _ := json.Marshal(b)
	return out
}

func (m *Malaffi) post(body []byte, kind string) bool {
	req, _ := http.NewRequest("POST", m.sub.Endpoint, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/fhir+json")
	if h := m.sub.HookHdr; h != "" {
		if i := strings.Index(h, ":"); i > 0 {
			req.Header.Set(strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:]))
		}
	}
	resp, err := m.client.Do(req)
	ok := err == nil && resp.StatusCode/100 == 2
	if resp != nil {
		resp.Body.Close()
	}
	m.mu.Lock()
	if ok {
		m.stats["posted_"+kind]++
	} else {
		m.stats["post_failed_"+kind]++
	}
	m.mu.Unlock()
	return ok
}

// deliver: handshake, then release events on the arrival clock and push notifications (with faults) + heartbeats.
func (m *Malaffi) deliver() {
	for {
		m.mu.Lock()
		hs := m.bundle(m.statusLocked("handshake", nil), nil, false)
		m.mu.Unlock()
		if m.post(hs, "handshake") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	m.mu.Lock()
	m.sub.status = "active"
	m.started = time.Now()
	full := m.sub.Content == "full-resource"
	m.mu.Unlock()
	var held [][]byte // reordered/duplicated notifications waiting to be sent after the next one
	lastSend := time.Now()
	i := 0
	for {
		now := time.Now()
		var batch []*hieEvent
		m.mu.Lock()
		for i < len(m.events) && len(batch) < m.sub.MaxCount {
			e := m.events[i]
			due := m.started
			if m.speed > 0 {
				due = m.started.Add(time.Duration(e.Arrival / m.speed * float64(time.Second)))
			}
			if due.After(now) {
				break
			}
			e.relWall = now.UnixNano()
			m.sub.released = e.Num
			m.latest[e.Type+"/"+e.ID] = e
			batch = append(batch, e)
			i++
		}
		var body []byte
		if len(batch) > 0 {
			body = m.bundle(m.statusLocked("event-notification", batch), batch, full)
			m.stats["notifications"]++
		}
		done := i >= len(m.events)
		m.mu.Unlock()
		if body != nil {
			r := m.rng.Float64()
			switch {
			case r < m.faults.drop:
				m.mu.Lock()
				m.stats["fault_dropped"]++
				m.mu.Unlock()
			case r < m.faults.drop+m.faults.reorder:
				held = append(held, body)
				m.mu.Lock()
				m.stats["fault_reordered"]++
				m.mu.Unlock()
			default:
				m.post(body, "event")
				for _, h := range held {
					m.post(h, "event")
				}
				held = nil
				if m.rng.Float64() < m.faults.dup {
					held = append(held, body) // duplicate: re-sent after the next notification
					m.mu.Lock()
					m.stats["fault_duplicated"]++
					m.mu.Unlock()
				}
			}
			lastSend = time.Now()
		}
		if done {
			for _, h := range held {
				m.post(h, "event")
			}
			held = nil
			m.mu.Lock()
			if m.sub.status != "off" {
				m.sub.status = "off" // feed complete: tells the consumer nothing follows events-since
				f, _ := os.Create(filepath.Join(m.runDir, "malaffi_release.ndjson"))
				bw := bufio.NewWriter(f)
				for _, e := range m.events {
					fmt.Fprintf(bw, "{\"n\":%d,\"w\":%d}\n", e.Num, e.relWall)
				}
				bw.Flush()
				f.Close()
			}
			m.mu.Unlock()
		}
		if time.Since(lastSend) >= m.sub.Heartbeat {
			m.mu.Lock()
			hb := m.bundle(m.statusLocked("heartbeat", nil), nil, false)
			m.mu.Unlock()
			m.post(hb, "heartbeat")
			lastSend = time.Now()
		}
		if len(batch) < m.sub.MaxCount || m.speed > 0 {
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func (m *Malaffi) events_(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	since, _ := strconv.ParseInt(q.Get("eventsSinceNumber"), 10, 64)
	until, _ := strconv.ParseInt(q.Get("eventsUntilNumber"), 10, 64)
	m.mu.Lock()
	if m.sub == nil || m.sub.ID != id {
		m.mu.Unlock()
		writeFHIR(w, 404, outcome("not-found", "no such subscription"))
		return
	}
	rel := m.sub.released
	if until == 0 || until > rel {
		until = rel
	}
	if since < 1 {
		since = 1
	}
	if until-since+1 > 1000 {
		until = since + 999
	}
	evs := []*hieEvent{}
	for n := since; n <= until; n++ {
		evs = append(evs, m.events[n-1])
	}
	st := m.statusLocked("query-event", evs)
	m.stats["events_queries"]++
	m.stats["events_replayed"] += len(evs)
	m.mu.Unlock()
	w.Header().Set("Content-Type", "application/fhir+json")
	w.Write(m.bundle(st, evs, q.Get("content") != "id-only"))
}
