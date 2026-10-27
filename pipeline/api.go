package main

// Stage 5: thin read API over a finished, materialised run (docs/BACKEND-HANDOFF.md §5).
//
//   wsi7 serve -run <run dir> -data <cohort dir> [-addr 127.0.0.1:8088]
//
// Everything served is derived from the run's own logs (out_log, fhir_log, review_queue, reviewer_decisions, anchors,
// report7, dashboard) — no client-side or invented records. Patient data is synthetic. Review actions (POST) are
// RBAC-checked and appended to <run>/api_reviews.ndjson as hash-chained events; nothing in the run is overwritten.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const apiVersion = "wasl-api/1"

type apiCase struct {
	ID        string    `json:"id"`        // loop id (ServiceRequest reference)
	Programme string    `json:"programme"` // crc | cervical
	State     string    `json:"state"`
	Dropoff   string    `json:"dropoff"`
	Owner     string    `json:"owner,omitempty"`
	Provider  string    `json:"provider"`
	SRDay     float64   `json:"referral_day"`
	Deadline  float64   `json:"deadline_day,omitempty"`
	Overdue   bool      `json:"overdue"`
	Priority  string    `json:"priority,omitempty"`
	Consent   string    `json:"consent"` // contact permission at last decision: unknown|granted|denied|opted_out
	Review    string    `json:"review"`  // none | open | signed
	Closed    bool      `json:"closed"`
	ClosedDay float64   `json:"closed_day,omitempty"`
	Fixture   string    `json:"fixture,omitempty"` // demo fixture tag, if this case is one of the three
	last      *Decision // full last decision (detail view)
	decisions []*Decision
}

type apiServer struct {
	run, data string
	meta      map[string]interface{}
	cases     map[string]*apiCase
	order     []string
	queue     map[string][]*ReviewItem // loop -> raised items
	signoffs  map[string][]map[string]interface{}
	facts     map[string][]map[string]interface{} // loop -> FHIR timeline entries (no free text)
	ids       map[string][]map[string]string     // loop -> linked registrations
	dashboard map[string]interface{}
	report    map[string]interface{}
	anchors   map[string]string
	booking   map[string]interface{} // stage 6: simulated booking decision support (booking/suggest.py)
	fixtures  map[string]string
	mu        sync.Mutex
	reviews   []map[string]interface{}
}

func runServe(run, data, addr string) {
	s := loadAPI(run, data)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/dashboard", s.getDashboard)
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/cases/", s.caseRoutes)
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	srv := &http.Server{Addr: addr, Handler: cors(mux), ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintf(os.Stderr, "wasl api: %d cases from %s on http://%s (fixtures: %v)\n", len(s.cases), run, addr, s.fixtures)
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Wasl-Role, X-Wasl-Actor")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func loadAPI(run, data string) *apiServer {
	s := &apiServer{run: run, data: data, cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{},
		signoffs: map[string][]map[string]interface{}{}, facts: map[string][]map[string]interface{}{}, ids: map[string][]map[string]string{}}
	readJSON := func(name string, v interface{}) { b, _ := os.ReadFile(filepath.Join(run, name)); json.Unmarshal(b, v) }
	readJSON("dashboard.json", &s.dashboard)
	readJSON("report7.json", &s.report)
	readJSON("anchors.json", &s.anchors)
	readJSON("booking.json", &s.booking)
	// decisions (OUT stream log): every decision kept; the last per loop is the current state
	for _, m := range readLog(filepath.Join(run, "out_log.ndjson")) {
		oe := decodeEnv(m)
		if oe.Dec != nil {
			d := oe.Dec
			c := s.cases[d.LoopID]
			if c == nil {
				c = &apiCase{ID: d.LoopID}
				s.cases[d.LoopID] = c
				s.order = append(s.order, d.LoopID)
			}
			c.decisions = append(c.decisions, d)
			c.last = d
		}
		if oe.Item != nil {
			s.queue[oe.Item.LoopID] = append(s.queue[oe.Item.LoopID], oe.Item)
		}
	}
	for _, c := range s.cases {
		d := c.last
		c.Programme, c.State, c.Dropoff, c.Owner, c.Provider = d.Prog, d.State, d.Dropoff, d.Owner, d.Provider
		c.SRDay, c.Deadline, c.Overdue, c.Priority, c.Closed, c.ClosedDay = d.SRDay, d.Deadline, d.Overdue, d.Priority, d.State == "closed", d.ClosedDay
		c.Consent = consentFromDecision(d)
	}
	// reviewer sign-offs (signed resolutions)
	if f, err := os.Open(filepath.Join(run, "reviewer_decisions.ndjson")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r map[string]interface{}
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				l := fmt.Sprint(r["loop"])
				s.signoffs[l] = append(s.signoffs[l], r)
			}
		}
		f.Close()
	}
	for id, c := range s.cases {
		switch {
		case len(s.signoffs[id]) > 0:
			c.Review = "signed"
		case len(s.queue[id]) > 0:
			c.Review = "open"
		default:
			c.Review = "none"
		}
	}
	s.loadTimelines()
	s.pickFixtures()
	s.loadReviews()
	s.meta = map[string]interface{}{"api": apiVersion, "data_mode": "simulated", "run": filepath.Base(run),
		"policy": PolicyName, "reader_model": s.readerModel(), "loaded_at": time.Now().UTC().Format(time.RFC3339),
		"note": "All patients, providers and outcomes are synthetic (Wikidata names, MTSamples report texts). Not real patient data."}
	if len(s.order) > 0 {
		if p := s.cases[s.order[0]].last.Policy; p != "" {
			s.meta["policy"] = p
		}
	}
	return s
}

// readerModel reports the second evidence reader actually recorded in this run's evidence provenance.
func (s *apiServer) readerModel() string {
	for _, id := range s.order {
		for _, d := range s.cases[id].decisions {
			for _, b := range d.Chain {
				if b.Type == "evidence" {
					if m, ok := b.Body["reader_model"].(string); ok && m != "" {
						return m
					}
				}
			}
		}
	}
	return "unknown"
}

func consentFromDecision(d *Decision) string {
	if d.Contact != "" {
		return d.Contact
	}
	return "unknown"
}

// loadTimelines indexes the FHIR log by patient registration so a case timeline can show every linked event
// (type, provider, status, day, raw-message hash) without free text.
func (s *apiServer) loadTimelines() {
	type ent struct {
		pk  string
		row map[string]interface{}
	}
	byPK := map[string][]map[string]interface{}{}
	srPK := map[string]string{}
	for _, m := range readLog(filepath.Join(s.run, "fhir_log.ndjson")) {
		if !strings.HasSuffix(m.Subject, ".res") {
			continue
		}
		prov := strings.Split(m.Subject, ".")[1]
		arr, _ := strconv.ParseFloat(m.Hdr["Wsi-Arr"], 64)
		e, so := normalise(prov, arr, m.Data)
		if e == nil || so != nil || e.PatientKey == "" {
			continue
		}
		h := sha256.Sum256(m.Data)
		row := map[string]interface{}{"seq": m.Seq, "type": e.RType, "id": prov + "/" + e.RType + "/" + e.ID, "provider": prov, "status": e.Status,
			"day": round4(e.Day), "arrival_day": round4(arr), "category": e.Category, "raw_sha": hex.EncodeToString(h[:])[:16],
			"source": firstNonEmpty(m.Hdr["Wsi-Src"], "fhir")}
		byPK[e.PatientKey] = append(byPK[e.PatientKey], row)
		if e.RType == "ServiceRequest" && e.Category != "referral-intake" {
			srPK[prov+"/ServiceRequest/"+e.ID] = e.PatientKey
		}
	}
	for id, c := range s.cases {
		pks := map[string]bool{srPK[id]: true}
		// linked downstream registrations come from the link_decision blocks of the case's decisions
		for _, d := range c.decisions {
			for _, b := range d.Chain {
				if b.Type == "link_decision" {
					if we, ok := b.Body["weakest_edge"].(map[string]interface{}); ok {
						pks[fmt.Sprint(we["a"])], pks[fmt.Sprint(we["b"])] = true, true
					}
				}
			}
			for _, o := range d.Outreach {
				if o.Pair[1] != "" {
					pks[o.Pair[1]] = true
				}
			}
		}
		rows := []map[string]interface{}{}
		ids := []map[string]string{}
		for pk := range pks {
			if pk == "" || pk == "<nil>" {
				continue
			}
			role := "downstream"
			if pk == srPK[id] {
				role = "referral"
			}
			ids = append(ids, map[string]string{"registration": pk, "provider": strings.Split(pk, "|")[0], "role": role})
			rows = append(rows, byPK[pk]...)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i]["seq"].(uint64) < rows[j]["seq"].(uint64) })
		sort.Slice(ids, func(i, j int) bool { return ids[i]["role"] > ids[j]["role"] || (ids[i]["role"] == ids[j]["role"] && ids[i]["registration"] < ids[j]["registration"]) })
		s.facts[id], s.ids[id] = rows, ids
	}
}

func firstNonEmpty(a ...string) string {
	for _, x := range a {
		if x != "" {
			return x
		}
	}
	return ""
}

// pickFixtures selects the three demo cases from real run data (docs/BACKEND-HANDOFF.md "Frontend contract"):
//   evidence_closure  : CRC loop closed by trusted cross-provider link + agreeing readers, consent granted
//   false_closure_trap: a procedure exists but evidence is incomplete -> held/inadequate, never closed
//   review_hold       : uncertain link or reader disagreement raised for review, outreach held for consent/guard, later signed
func (s *apiServer) pickFixtures() {
	s.fixtures = map[string]string{}
	ids := append([]string{}, s.order...)
	sort.Strings(ids)
	has := func(c *apiCase, typ string, pred func(Block) bool) bool {
		for _, d := range c.decisions {
			for _, b := range d.Chain {
				if b.Type == typ && pred(b) {
					return true
				}
			}
		}
		return false
	}
	for _, id := range ids {
		c := s.cases[id]
		if c.Programme != "crc" {
			continue
		}
		if s.fixtures["evidence_closure"] == "" && c.Closed && c.Consent == "granted" &&
			has(c, "link_decision", func(b Block) bool { return fmt.Sprint(b.Body["hops"]) != "0" }) &&
			has(c, "evidence", func(b Block) bool { return b.Body["agree"] == true && b.Body["label"] == "adequate" && fmt.Sprint(b.Body["human_override"]) == "" }) {
			s.fixtures["evidence_closure"] = id
		}
		if s.fixtures["false_closure_trap"] == "" && !c.Closed && c.State == "inadequate" &&
			has(c, "hold_decision", func(b Block) bool { return true }) {
			s.fixtures["false_closure_trap"] = id
		}
		// review/consent hold: uncertain link AND reader disagreement both went to double review and were signed, outreach was
		// held for consent before the grant arrived, and the loop then closed on the signed evidence.
		if s.fixtures["review_hold"] == "" && c.Closed && linkSigned(s.signoffs[id]) && evidenceSigned(s.signoffs[id]) && consentHeldThenGranted(c) {
			s.fixtures["review_hold"] = id
		}
	}
	for tag, id := range s.fixtures {
		s.cases[id].Fixture = tag
	}
}

func evidenceSigned(so []map[string]interface{}) bool {
	for _, r := range so {
		if fmt.Sprint(r["evidence"]) == "adequate" && r["wrong"] == false {
			return true
		}
	}
	return false
}

func consentHeldThenGranted(c *apiCase) bool {
	held, granted := false, false
	for _, d := range c.decisions {
		for _, t := range d.ConsentLog {
			held = held || t.To == "held"
			granted = granted || (held && t.Contact == "granted")
		}
	}
	return held && granted
}

func linkSigned(so []map[string]interface{}) bool {
	for _, r := range so {
		if fmt.Sprint(r["link"]) == "same" && r["wrong"] == false {
			return true
		}
	}
	return false
}

func heldEver(c *apiCase) bool {
	for _, d := range c.decisions {
		for _, o := range d.Outreach {
			if o.Suppress == SupConsentHold || o.Suppress == SupSecondID {
				return true
			}
		}
	}
	return false
}

func (s *apiServer) loadReviews() {
	f, err := os.Open(filepath.Join(s.run, "api_reviews.ndjson"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r map[string]interface{}
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			s.reviews = append(s.reviews, r)
		}
	}
}

func writeJSONResp(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	enc.Encode(v)
}

func (s *apiServer) envelope(body map[string]interface{}) map[string]interface{} {
	body["meta"] = s.meta
	return body
}

// ---------- handlers ----------

func (s *apiServer) healthz(w http.ResponseWriter, r *http.Request) {
	nats := "not_required_for_read_api"
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:14822", 300*time.Millisecond); err == nil {
		conn.Close()
		nats = "up"
	}
	reader := "cached"
	if LiveReader != nil {
		reader = map[bool]string{true: "up", false: "down"}[LiveReader.Healthy()]
	}
	src := "fhir"
	if st := readStatus(s.run, "hl7mllp"); st != nil {
		src = "hl7v2"
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{"status": "ok", "cases": len(s.cases),
		"dependencies": map[string]string{"materialised_run": "loaded", "nats": nats, "reader": reader, "ingest_adapter": src}}))
}

func (s *apiServer) getDashboard(w http.ResponseWriter, r *http.Request) {
	states, drop, review := map[string]int{}, map[string]int{}, map[string]int{}
	byProg := map[string]map[string]int{}
	overdue, closed := 0, 0
	for _, c := range s.cases {
		states[c.State]++
		drop[c.Dropoff]++
		review[c.Review]++
		if byProg[c.Programme] == nil {
			byProg[c.Programme] = map[string]int{}
		}
		byProg[c.Programme]["cases"]++
		if c.Closed {
			closed++
			byProg[c.Programme]["closed"]++
		}
		if c.Overdue {
			overdue++
			byProg[c.Programme]["overdue"]++
		}
	}
	rep := map[string]interface{}{}
	for _, k := range []string{"crc_kpi_31d", "closure", "consent_timing", "forged", "provenance", "throughput_ev_s"} {
		rep[k] = s.report[k]
	}
	if cr, ok := s.report["crash_recovery"].(map[string]interface{}); ok {
		rep["exactly_once"] = map[string]interface{}{"duplicate_outputs": cr["duplicate_outputs"], "lost_outputs": cr["lost_outputs"], "ingest_lost": cr["ingest_lost"], "ingest_duplicates": cr["ingest_duplicates"]}
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{
		"cases": len(s.cases), "closed": closed, "overdue": overdue, "states": states, "dropoff": drop, "review": review,
		"by_programme": byProg, "sla": s.dashboard["sla"], "providers": s.dashboard["providers"], "origins": s.dashboard["origins"],
		"small_cell_rule": s.dashboard["small_cell_rule"], "metrics": rep, "fixtures": s.fixtures,
		"capacity": s.capacity(),
	}))
}

func (s *apiServer) getFixtures(w http.ResponseWriter, r *http.Request) {
	out := map[string]interface{}{}
	for tag, id := range s.fixtures {
		out[tag] = s.summary(s.cases[id])
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{"fixtures": out}))
}

func (s *apiServer) summary(c *apiCase) map[string]interface{} {
	return map[string]interface{}{"id": c.ID, "programme": c.Programme, "state": c.State, "dropoff": c.Dropoff, "owner": c.Owner,
		"provider": c.Provider, "referral_day": round4(c.SRDay), "deadline_day": c.Deadline, "overdue": c.Overdue, "priority": c.Priority,
		"consent": c.Consent, "review": c.Review, "closed": c.Closed, "closed_day": round4(c.ClosedDay), "fixture": c.Fixture,
		"decisions": len(c.decisions)}
}

// GET /api/cases?programme=crc&state=closed&owner=...&consent=granted&review=open&overdue=true&fixture=1&limit=50&offset=0&q=<id substring>
func (s *apiServer) getCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	match := func(c *apiCase) bool {
		eq := func(k, v string) bool { x := q.Get(k); return x == "" || x == v }
		return eq("programme", c.Programme) && eq("state", c.State) && eq("owner", c.Owner) && eq("consent", c.Consent) &&
			eq("review", c.Review) && eq("dropoff", c.Dropoff) && eq("provider", c.Provider) &&
			(q.Get("overdue") == "" || q.Get("overdue") == strconv.FormatBool(c.Overdue)) &&
			(q.Get("fixture") == "" || c.Fixture != "") && (q.Get("q") == "" || strings.Contains(c.ID, q.Get("q")))
	}
	ids := append([]string{}, s.order...)
	sort.Strings(ids)
	sel := []map[string]interface{}{}
	for _, id := range ids {
		if c := s.cases[id]; match(c) {
			sel = append(sel, s.summary(c))
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	total := len(sel)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{"total": total, "offset": offset, "limit": limit, "cases": sel[offset:end]}))
}

func (s *apiServer) caseRoutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/cases/")
	sub := ""
	if i := strings.LastIndex(rest, "/"); i >= 0 && (strings.HasSuffix(rest, "/audit") || strings.HasSuffix(rest, "/review")) {
		rest, sub = rest[:i], rest[i+1:]
	}
	id, _ := url.PathUnescape(rest)
	c := s.cases[id]
	if c == nil {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown case", "id": id,
			"hint": "case ids are loop ids such as ahs-mushrif/ServiceRequest/AHS-sr-1002; URL-encode the slashes or pass them as-is"}))
		return
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		s.caseDetail(w, c)
	case sub == "audit" && r.Method == http.MethodGet:
		s.caseAudit(w, c)
	case sub == "review" && r.Method == http.MethodPost:
		s.caseReview(w, r, c)
	default:
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed"}))
	}
}

func (s *apiServer) caseDetail(w http.ResponseWriter, c *apiCase) {
	ev := []map[string]interface{}{}
	links := []map[string]interface{}{}
	holds := []map[string]interface{}{}
	seen := map[string]bool{}
	for _, d := range c.decisions {
		for _, b := range d.Chain {
			key := b.Hash
			if seen[key] {
				continue
			}
			seen[key] = true
			switch b.Type {
			case "evidence":
				ev = append(ev, b.Body)
			case "link_decision":
				links = append(links, b.Body)
			case "hold_decision":
				holds = append(holds, b.Body)
			}
		}
	}
	hist := []map[string]interface{}{}
	for _, d := range c.decisions {
		hist = append(hist, map[string]interface{}{"decision_id": d.ID, "in_seq": d.InSeq, "state": d.State, "dropoff": d.Dropoff, "owner": d.Owner,
			"overdue": d.Overdue, "escalations": nz(d.Escalations), "consent_log": nz(d.ConsentLog), "policy": d.Policy, "sig": d.Sig})
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{
		"case": s.summary(c), "identities": nz(s.ids[c.ID]), "timeline": nz(s.facts[c.ID]), "links": links, "evidence": ev, "holds": holds,
		"outreach": nz(c.last.Outreach), "decisions": hist, "review_items": nz(s.queue[c.ID]), "signoffs": nz(s.signoffs[c.ID]),
		"api_reviews": s.reviewsFor(c.ID), "current": c.last, "booking": s.bookingFor(c.ID),
	}))
}

func (s *apiServer) caseAudit(w http.ResponseWriter, c *apiCase) {
	chain := c.last.Chain
	ok := verifyChain(chain)
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{
		"case": c.ID, "chain": nz(chain), "chain_verified": ok, "chain_length": len(chain),
		"anchors": s.anchors, "anchor_note": "daily Merkle/stream heads co-signed into an in-UAE WORM store; `wsi7 tamper` re-verifies the whole run",
		"decision_signatures": func() []map[string]string {
			out := []map[string]string{}
			for _, d := range c.decisions {
				out = append(out, map[string]string{"decision_id": d.ID, "sig": d.Sig, "recomputed": decSig(d), "policy": d.Policy})
			}
			return out
		}(),
		"history": s.reviewsFor(c.ID),
	}))
}

func (s *apiServer) reviewsFor(id string) []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []map[string]interface{}{}
	for _, r := range s.reviews {
		if r["case"] == id {
			out = append(out, r)
		}
	}
	return out
}

// POST /api/cases/{id}/review   headers: X-Wasl-Role (coordinator|clinician|reviewer|auditor), X-Wasl-Actor
// body: {"item": "<review item id>", "decision": "same|different|adequate|inadequate|confirm_contact", "note": "..."}
// Demo mode: appends a hash-chained review event; the run's decisions are never mutated.
func (s *apiServer) caseReview(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	var req struct{ Item, Decision, Note string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Decision == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be {\"item\":..., \"decision\":..., \"note\":...}"}))
		return
	}
	act := map[string]string{"same": ActSignLink, "different": ActSignLink, "adequate": ActSignEvidence, "inadequate": ActSignEvidence,
		"confirm_contact": ActRecordConsent}[req.Decision]
	if act == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "decision must be same|different|adequate|inadequate|confirm_contact"}))
		return
	}
	if err := Authorize(role, act); err != nil {
		writeJSONResp(w, 403, s.envelope(map[string]interface{}{"error": err.Error(), "role": role, "action": act}))
		return
	}
	if actor == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "X-Wasl-Actor header required (who is signing)"}))
		return
	}
	if req.Item != "" {
		found := false
		for _, it := range s.queue[c.ID] {
			found = found || it.ItemID == req.Item
		}
		if !found {
			writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "review item not raised for this case", "item": req.Item}))
			return
		}
	}
	s.mu.Lock()
	prev := "genesis"
	if n := len(s.reviews); n > 0 {
		prev = fmt.Sprint(s.reviews[n-1]["hash"])
	}
	ev := map[string]interface{}{"case": c.ID, "item": req.Item, "decision": req.Decision, "note": req.Note, "role": role, "actor": actor,
		"action": act, "at": time.Now().UTC().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "mode": "demo", "prev": prev}
	h := sha256.Sum256(canon(ev))
	ev["hash"] = hex.EncodeToString(h[:])
	s.reviews = append(s.reviews, ev)
	f, err := os.OpenFile(filepath.Join(s.run, "api_reviews.ndjson"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		b, _ := json.Marshal(ev)
		f.Write(append(b, '\n'))
		f.Close()
	}
	s.mu.Unlock()
	writeJSONResp(w, 201, s.envelope(map[string]interface{}{"review": ev,
		"note": "appended to the review ledger; in a live deployment this is published as a signed Provenance sign-off and replayed through the linker/closure path"}))
}

// nz turns a nil slice into an empty JSON array (frontends should never have to null-check lists).
func nz[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

// capacity: the booking decision-support summary (simulated) or a pointer to generate it.
func (s *apiServer) capacity() map[string]interface{} {
	if s.booking == nil {
		return map[string]interface{}{"status": "not_generated", "how": "python booking/suggest.py <run dir>", "data_mode": "simulated"}
	}
	return map[string]interface{}{"status": "simulated_decision_support", "data_mode": "simulated", "condition": s.booking["capacity"],
		"summary": s.booking["summary"], "inputs": s.booking["inputs"], "note": s.booking["note"], "levers": s.booking["levers"]}
}

// bookingFor: this case's simulated booking suggestion and the capacity condition that governs it.
func (s *apiServer) bookingFor(id string) map[string]interface{} {
	if s.booking == nil {
		return map[string]interface{}{"status": "not_generated"}
	}
	cs, _ := s.booking["cases"].(map[string]interface{})
	c, ok := cs[id].(map[string]interface{})
	if !ok {
		return map[string]interface{}{"status": "not_applicable", "reason": "case does not need a colonoscopy slot at the end of the run", "data_mode": "simulated"}
	}
	out := map[string]interface{}{"data_mode": "simulated", "decision_support_only": true}
	for k, v := range c {
		out[k] = v
	}
	if cap, ok := s.booking["capacity"].(map[string]interface{}); ok {
		out["capacity_condition"], out["pooling_relevant"], out["pooling_note"] = cap["condition"], cap["pooling_relevant"], cap["pooling_note"]
	}
	return out
}
