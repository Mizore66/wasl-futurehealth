package main

// Mock Malaffi, polling and bulk tiers (ingest tiers b and d in docs/research/rai-modelcards-malaffi.md):
//   GET /fhir/_history?_since=<instant>&_count=&_offset=   system history: every version whose HIE receipt time
//                                                          (meta.lastUpdated) is at or after _since, oldest first
//   GET /fhir/$export[?_since=<instant>]                   Bulk Data kick-off: 202 + Content-Location
//   GET /fhir/$export-status/{job}                         manifest (the mock finishes jobs at once)
//   GET /fhir/$export-file/{job}/{type}                    NDJSON, the latest visible version of each changed resource
// Fault model for polling: a version's lastUpdated is its HIE receipt time, but it becomes visible to queries later
// (a late-committing transaction) for a fraction of versions (-lagfrac, lag 0.1 to 2 simulated days). A poll that has
// already passed that lastUpdated misses it unless its overlap covers the lag; the nightly $export catches the rest.
// In poll mode the HIE clock runs at -pollspeed simulated days per wall second from the first request.
// X-Wsi-Feed-Complete: true on a history page means every version is visible (a simulation-only end signal).
import (
	"bufio"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type exportJob struct {
	since, at float64 // simulated days; since < 0 = no _since
}

func (m *Malaffi) initPoll(speed, lagFrac float64, seed int64) {
	m.pollSpeed = speed
	r := rand.New(rand.NewSource(seed ^ 0x5eed))
	m.visAt = make([]float64, len(m.events))
	for i, e := range m.events {
		m.visAt[i] = e.Arrival
		if r.Float64() < lagFrac {
			m.visAt[i] += 0.1 + 1.9*r.Float64()
			m.stats["poll_lagged_versions"]++
		}
	}
	m.exports = map[string]exportJob{}
}

func (m *Malaffi) simNowLocked() float64 {
	if m.pollStart.IsZero() {
		m.pollStart = time.Now()
	}
	return time.Since(m.pollStart).Seconds() * m.pollSpeed
}

func simInstant(d float64) string {
	return baseTime.Add(time.Duration(d * 86400 * float64(time.Second))).UTC().Format(time.RFC3339Nano)
}

func parseInstant(s string) (float64, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, false
	}
	return t.Sub(baseTime).Seconds() / 86400, true
}

// pollHandle serves the polling and bulk routes; false = not one of them.
func (m *Malaffi) pollHandle(w http.ResponseWriter, r *http.Request, p []string) bool {
	if m.visAt == nil || r.Method != "GET" {
		return false
	}
	switch {
	case len(p) == 1 && p[0] == "_history":
		m.history(w, r)
	case len(p) == 1 && p[0] == "$export":
		m.mu.Lock()
		since := -1.0
		if s := r.URL.Query().Get("_since"); s != "" {
			if d, ok := parseInstant(s); ok {
				since = d
			}
		}
		id := strconv.Itoa(len(m.exports) + 1)
		m.exports[id] = exportJob{since: since, at: m.simNowLocked()}
		m.stats["export_jobs"]++
		m.mu.Unlock()
		w.Header().Set("Content-Location", "/fhir/$export-status/"+id)
		w.WriteHeader(202)
	case len(p) == 2 && p[0] == "$export-status":
		m.mu.Lock()
		job, ok := m.exports[p[1]]
		m.mu.Unlock()
		if !ok {
			writeFHIR(w, 404, outcome("not-found", "no such export"))
			return true
		}
		types := map[string]bool{}
		for _, e := range m.events {
			types[e.Type] = true
		}
		out := []map[string]string{}
		for _, t := range sortedStr(types) {
			out = append(out, map[string]string{"type": t, "url": "/fhir/$export-file/" + p[1] + "/" + t})
		}
		writeFHIR(w, 200, map[string]interface{}{"transactionTime": simInstant(job.at), "request": "/fhir/$export", "requiresAccessToken": true, "output": out})
	case len(p) == 3 && p[0] == "$export-file":
		m.mu.Lock()
		job, ok := m.exports[p[1]]
		latest := map[string]*hieEvent{}
		if ok {
			for i, e := range m.events {
				if m.visAt[i] <= job.at && e.Arrival > job.since && e.Type == p[2] {
					if x := latest[e.ID]; x == nil || x.Num < e.Num {
						latest[e.ID] = e
					}
				}
			}
			m.stats["export_resources"] += len(latest)
		}
		m.mu.Unlock()
		if !ok {
			writeFHIR(w, 404, outcome("not-found", "no such export"))
			return true
		}
		w.Header().Set("Content-Type", "application/fhir+ndjson")
		bw := bufio.NewWriter(w)
		for _, id := range sortedStr(latest) {
			bw.Write(latest[id].Body)
			bw.WriteByte('\n')
		}
		bw.Flush()
	default:
		return false
	}
	return true
}

func (m *Malaffi) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, _ := parseInstant(q.Get("_since"))
	count, _ := strconv.Atoi(q.Get("_count"))
	off, _ := strconv.Atoi(q.Get("_offset"))
	if count <= 0 || count > 1000 {
		count = 500
	}
	m.mu.Lock()
	now := m.simNowLocked()
	var vis []*hieEvent
	all := true
	for i, e := range m.events {
		if m.visAt[i] > now {
			all = false
			continue
		}
		if e.Arrival >= since {
			vis = append(vis, e)
		}
	}
	sort.SliceStable(vis, func(i, j int) bool { return vis[i].Arrival < vis[j].Arrival })
	if off > len(vis) {
		off = len(vis)
	}
	end := off + count
	if end > len(vis) {
		end = len(vis)
	}
	page := vis[off:end]
	m.stats["history_queries"]++
	m.stats["history_entries"] += len(page)
	m.mu.Unlock()
	b := Bundle{ResourceType: "Bundle", Type: "history", Timestamp: simInstant(now), Entry: []BEntry{}}
	for _, e := range page {
		b.Entry = append(b.Entry, BEntry{FullURL: e.Type + "/" + e.ID + "/_history/" + e.Vid, Resource: e.Body,
			Response: &BResp{Status: "200", LastModified: e.occurred.UTC().Format(time.RFC3339Nano)}})
	}
	if all {
		w.Header().Set("X-Wsi-Feed-Complete", "true")
	}
	w.Header().Set("X-Wsi-Total", fmt.Sprint(len(vis)))
	writeFHIR(w, 200, b)
}

func sortedStr[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
