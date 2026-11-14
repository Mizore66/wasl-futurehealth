package main

// Stage 5: thin read API over a finished, materialised run (docs/BACKEND-HANDOFF.md §5).
//
//   wsi7 serve -run <run dir> -data <cohort dir> [-addr 127.0.0.1:8088]
//
// Everything served is derived from the run's own logs (out_log, fhir_log, review_queue, reviewer_decisions, anchors,
// report7, dashboard) — no client-side or invented records. Patient data is synthetic. Every /api route requires a
// token-bound principal; reads and mutations have separate RBAC checks. Review actions (POST) are appended to
// <run>/api_reviews.ndjson as hash-chained events; nothing in the run is overwritten.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const apiVersion = "wasl-api/1"

func auditAnchorNote(anchors map[string]string) string {
	if len(anchors) == 0 {
		return "No local anchors are recorded for this materialised run; this response has no OUT stream ledger heads or FHIR log hash. `wsi7 tamper` re-verifies the whole run. External in-UAE WORM storage and co-signing are designed, not built"
	}
	return "Local anchors for this materialised run: OUT stream ledger heads and a FHIR log hash; `wsi7 tamper` re-verifies the whole run. External in-UAE WORM storage and co-signing are designed, not built"
}

type apiCase struct {
	ID        string           `json:"id"`        // loop id (ServiceRequest reference)
	Programme string           `json:"programme"` // crc | cervical | lung_nodule | diabetic_retinopathy | breast | lung_ldct | ifhas
	State     string           `json:"state"`
	Dropoff   string           `json:"dropoff"`
	Owner     string           `json:"owner,omitempty"`
	Provider  string           `json:"provider"`
	SRDay     float64          `json:"referral_day"`
	Deadline  float64          `json:"deadline_day,omitempty"`
	Overdue   bool             `json:"overdue"`
	Priority  string           `json:"priority,omitempty"`
	Consent   string           `json:"consent"` // contact permission at last decision: unknown|granted|denied|opted_out
	Review    string           `json:"review"`  // none | open | signed
	Closed    bool             `json:"closed"`
	ClosedDay float64          `json:"closed_day,omitempty"`
	Fixture   string           `json:"fixture,omitempty"` // demo fixture tag, if this case is one of the demo cases
	ov        caseOverlay      // coordinator actions replayed from the ledger; never merged into the canonical fields above
	patient   patientCaseState // patient-link actions; separate from coordinator, consent and booking heads
	last      *Decision        // full last decision (detail view)
	decisions []*Decision
}

type apiServer struct {
	run, data       string
	meta            map[string]interface{}
	cases           map[string]*apiCase
	order           []string
	queue           map[string][]*ReviewItem // loop -> currently eligible raised items
	retiredQueue    map[string][]*ReviewItem // loop -> historical raised items, replay-only; never actionable
	signoffs        map[string][]map[string]interface{}
	facts           map[string][]map[string]interface{} // loop -> FHIR timeline entries (no free text)
	ids             map[string][]map[string]string      // loop -> linked registrations
	dashboard       map[string]interface{}
	report          map[string]interface{}
	anchors         map[string]string
	booking         map[string]interface{} // stage 6: simulated booking decision support (booking/suggest.py)
	bookingCalendar string
	bookingSlots    map[string]*simulatedSlot // union of generated candidates; reservations replay from the ledger
	recovery        recoveryProjection
	fixtures        map[string]string
	agentInv        map[string][]agentInvestigation // loop -> cached agent investigations consumed by a sign-off
	agentProv       map[string]interface{}          // cached provenance allow-list (ordinary run: cached=false)
	mu              sync.Mutex
	reviews         []map[string]interface{}
	rstates         map[reviewQuestionKey]*reviewProtocolState // independent double review overlay, rebuilt from the ledger
	claims          map[reviewQuestionKey]*reviewClaimState    // independent-review leases, rebuilt from claim events
	now             func() time.Time                           // UTC clock; replaceable in lease tests
	ledgerErr       string                                     // non-empty: api_reviews.ndjson failed validation at replay; coordinator writes are refused
	appendLine      func([]byte) error                         // appends one ledger line durably (replaceable in tests)
	patientRequests map[string]patientRequestRecord            // request-id dedup across every case-bound patient link
}

func apiHandler(s *apiServer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/api/patient/journey", s.patientJourney)
	mux.HandleFunc("/api/operations", s.getOperations)
	mux.HandleFunc("/api/dashboard", s.getDashboard)
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/recovery/", s.recoveryRoutes)
	mux.HandleFunc("/api/people", s.peopleRoutes)
	mux.HandleFunc("/api/people/", s.peopleRoutes)
	mux.HandleFunc("/api/agentshadow/status", s.getAgentShadowStatus)
	caseAPI := s.withConsent(s.undoRoutes(s.caseRoutes))
	mux.HandleFunc("/api/cases/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			s.caseExportRoute(w, r)
			return
		}
		caseAPI.ServeHTTP(w, r)
	}))
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	return cors(mux)
}

func basicAPIHandler(s *apiServer) http.Handler {
	return s.authorizeVerified(apiHandler(s))
}

func runServe(run, data, addr string) {
	if os.Getenv("WASL_SYNTHETIC_DATA_ONLY") != "1" {
		panic("serve requires WASL_SYNTHETIC_DATA_ONLY=1; the demo API must not be used with real clinical data")
	}
	s, err := loadAPIVerified(run, data)
	if err != nil {
		panic(err)
	}
	startRuntimeMetrics(os.Getenv("WASL_RUNTIME_ADDR"))
	// Recovery deployments use their configured Basic-auth identities and TLS;
	// all other serving modes require the bearer-bound synthetic-demo principals.
	var handler http.Handler
	if os.Getenv("WASL_API_USERS_FILE") != "" {
		handler = basicAPIHandler(s)
	} else {
		handler, err = s.handler(os.Getenv("WASL_API_TOKENS"), os.Getenv("WASL_API_ORIGINS"))
		if err != nil {
			panic(err)
		}
		handler, err = s.withRecoveryRoutes(handler, os.Getenv("WASL_API_TOKENS"), os.Getenv("WASL_API_ORIGINS"))
		if err != nil {
			panic(err)
		}
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	auth, err := apiAuthFromEnvironment()
	if err != nil {
		panic(err)
	}
	srv.Handler = auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		handler.ServeHTTP(w, r)
		writeStructuredLog("info", "api request", "method", r.Method, "path", r.URL.Path, "actor", r.Header.Get("X-Wasl-Actor"), "duration_ms", time.Since(started).Milliseconds())
	}))
	certFile, keyFile := os.Getenv("WASL_API_TLS_CERT_FILE"), os.Getenv("WASL_API_TLS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		panic("WASL_API_TLS_CERT_FILE and WASL_API_TLS_KEY_FILE must be set together")
	}
	if os.Getenv("WASL_API_USERS_FILE") != "" && certFile == "" {
		panic("WASL_API_USERS_FILE requires WASL_API_TLS_CERT_FILE and WASL_API_TLS_KEY_FILE")
	}
	scheme := "http"
	if certFile != "" {
		scheme = "https"
	}
	writeStructuredLog("info", "api listener ready", "cases", len(s.cases), "run", run, "address", addr, "scheme", scheme)
	if certFile != "" {
		err = srv.ListenAndServeTLS(certFile, keyFile)
	} else {
		err = srv.ListenAndServe()
	}
	if err != nil {
		panic(err)
	}
}

// cors is retained for direct in-process handler tests. It is origin-deny by default; external serving uses s.handler,
// which additionally requires a verified bearer principal on every /api route. Recovery's Basic-auth listener uses
// its single configured WASL_API_CORS_ORIGIN when browser access is explicitly enabled.
func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("WASL_API_USERS_FILE") == "" {
			corsWithOrigins(h, map[string]bool{}).ServeHTTP(w, r)
			return
		}
		if origin := os.Getenv("WASL_API_CORS_ORIGIN"); origin != "" {
			corsWithOrigins(h, map[string]bool{origin: true}).ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func loadAPI(run, data string) *apiServer {
	s := &apiServer{run: run, data: data, cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{}, retiredQueue: map[string][]*ReviewItem{},
		signoffs: map[string][]map[string]interface{}{}, facts: map[string][]map[string]interface{}{}, ids: map[string][]map[string]string{},
		claims: map[reviewQuestionKey]*reviewClaimState{}, now: time.Now}
	s.appendLine = func(line []byte) error { return appendLedgerLine(filepath.Join(run, "api_reviews.ndjson"), line) }
	readJSON := func(name string, v interface{}) { b, _ := os.ReadFile(filepath.Join(run, name)); json.Unmarshal(b, v) }
	readJSON("dashboard.json", &s.dashboard)
	readJSON("report7.json", &s.report)
	readJSON("anchors.json", &s.anchors)
	// Preserve generated booking-number spellings (notably Python's 2.0) for exact cross-language version hashes.
	if b, err := os.ReadFile(filepath.Join(run, "booking.json")); err == nil {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		_ = dec.Decode(&s.booking)
	}
	// Preserve timeline order even if dump lines were reordered. Current state
	// uses the same highest-output-sequence reducer as report7 and preflight.
	outLog := readLog(filepath.Join(run, "out_log.ndjson"))
	sort.SliceStable(outLog, func(i, j int) bool { return outLog[i].Seq < outLog[j].Seq })
	projection := newMaterializedDecisions()
	for _, m := range outLog {
		oe := decodeEnv(m)
		if err := projection.add(m, oe); err != nil {
			panic(err)
		}
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
		d := projection.final[c.ID]
		c.last = d
		c.Programme, c.State, c.Dropoff, c.Owner, c.Provider = d.Prog, d.State, d.Dropoff, d.Owner, d.Provider
		c.SRDay, c.Deadline, c.Overdue, c.Priority, c.Closed, c.ClosedDay = d.SRDay, d.Deadline, d.Overdue, d.Priority, d.State == "closed" || d.State == "no_followup_needed" || d.State == "no_referral_needed" || d.State == "no_recall_needed", d.ClosedDay
		if ongoingCare(d) {
			c.Closed = false
		}
		c.Consent = consentFromDecision(d)
		if d.Inactive {
			// The case and every earlier decision remain available through the
			// authorized detail/audit views, but no current review obligation
			// survives retirement. Earlier claims/reads still require their
			// original raised items to validate append-only ledger history.
			s.retiredQueue[c.ID] = s.queue[c.ID]
			delete(s.queue, c.ID)
		}
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
	s.loadAgent()
	s.pickAgentFixture()
	s.initBookingCatalogue()
	s.loadRecovery()
	if s.ledgerErr == "" {
		s.loadReviews()
	}
	s.meta = map[string]interface{}{"api": apiVersion, "data_mode": "simulated", "run": filepath.Base(run),
		"policy": PolicyName, "reader_model": safeReaderModel(s.readerModel()), "loaded_at": time.Now().UTC().Format(time.RFC3339),
		"note": "All patients, providers and outcomes are synthetic (Wikidata names, MTSamples report texts). Not real patient data."}
	return s
}

func safeReaderModel(model string) string {
	if model == "unknown" {
		return model
	}
	if len(model) == 0 || len(model) > 160 || !strings.HasPrefix(model, "gemma-") || strings.ContainsAny(model, " \t\r\n") {
		return "not-disclosed"
	}
	for _, r := range model {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-_.:/()+", r) {
			return "not-disclosed"
		}
	}
	return model
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
			"source": firstNonEmpty(m.Hdr["Wsi-Src"], "fhir"), "patient_key": e.PatientKey}
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
		sort.Slice(ids, func(i, j int) bool {
			return ids[i]["role"] > ids[j]["role"] || (ids[i]["role"] == ids[j]["role"] && ids[i]["registration"] < ids[j]["registration"])
		})
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

// pickFixtures selects the demo cases from real run data (docs/BACKEND-HANDOFF.md "Frontend contract"):
//
//	evidence_closure  : CRC loop closed by trusted cross-provider link + agreeing readers, consent granted
//	false_closure_trap: a procedure exists but evidence is incomplete -> held/inadequate, never closed
//	review_hold       : uncertain link or reader disagreement raised for review, outreach held for consent/guard, later signed
//	dr_exam_closure   : (runs with DR loops only) referable DR screen closed by an eye-clinic exam, readers agree on both reports
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
	holdAlt := ""
	for _, id := range ids {
		c := s.cases[id]
		if c.Programme == "diabetic_retinopathy" && s.fixtures["dr_exam_closure"] == "" && drExamClosure(c) {
			s.fixtures["dr_exam_closure"] = id
		}
		if c.Programme != "crc" {
			continue
		}
		if s.fixtures["evidence_closure"] == "" && c.Closed && c.Consent == "granted" &&
			has(c, "link_decision", func(b Block) bool { return fmt.Sprint(b.Body["hops"]) != "0" }) &&
			has(c, "evidence", func(b Block) bool {
				return b.Body["agree"] == true && b.Body["label"] == "adequate" && fmt.Sprint(b.Body["human_override"]) == ""
			}) {
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
		// fallback when no loop had consent held then granted (rare once consent is recorded up front): the same signed link
		// and evidence, with outreach held by the consent hold or the second-identifier guard at some point
		if holdAlt == "" && c.Closed && linkSigned(s.signoffs[id]) && evidenceSigned(s.signoffs[id]) && heldEver(c) {
			holdAlt = id
		}
	}
	if s.fixtures["review_hold"] == "" && holdAlt != "" {
		s.fixtures["review_hold"] = holdAlt
	}
	for tag, id := range s.fixtures {
		s.cases[id].Fixture = tag
	}
}

// drExamClosure: a referable DR screen (R2/M1/R3...) closed by an eye-clinic exam within the deadline, over a
// cross-provider link, with both text readers agreeing on the screening report and on the clinic note (no coded
// grade), contact consent granted, and no human help anywhere in the loop's history (no review item, no
// sign-off, no override, no earlier reader disagreement).
func drExamClosure(c *apiCase) bool {
	d := c.last
	if d == nil || d.DR == nil || d.State != "closed" || d.HumanAssisted || c.Consent != "granted" || c.Review != "none" {
		return false
	}
	dr := d.DR
	if dr.Rec == nil || dr.Rec.Action != DRRefer || !dr.Examined || !dr.WithinDeadline || dr.Coded {
		return false
	}
	for _, x := range c.decisions {
		for _, b := range x.Chain {
			if b.Type == "human_signoff" || (b.Type == "evidence" && (b.Body["agree"] != true || fmt.Sprint(b.Body["human_override"]) != "")) {
				return false
			}
		}
	}
	ev, cross := 0, false
	for _, b := range d.Chain {
		switch b.Type {
		case "evidence":
			ev++
		case "link_decision":
			cross = cross || fmt.Sprint(b.Body["hops"]) != "0"
		}
	}
	return ev == 2 && cross
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

func (s *apiServer) healthBody() map[string]interface{} {
	nats := "not_required_for_read_api"
	if conn, err := net.DialTimeout("tcp", natsFirstHostPort(), 300*time.Millisecond); err == nil {
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
	return map[string]interface{}{"status": "ok", "cases": len(s.cases),
		"dependencies": map[string]string{"materialised_run": "loaded", "nats": nats, "reader": reader, "ingest_adapter": src, "review_ledger": s.ledgerStatus()}}
}

func (s *apiServer) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSONResp(w, 200, s.exportEnvelope(s.healthBody()))
}

func (s *apiServer) getDashboard(w http.ResponseWriter, r *http.Request) {
	states, drop, review := map[string]int{}, map[string]int{}, map[string]int{}
	byProg := map[string]map[string]int{}
	overdue, closed, activeCases := 0, 0, 0
	for _, c := range s.cases {
		if c.last != nil && c.last.Inactive {
			continue
		}
		activeCases++
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
		"cases": activeCases, "materialization": s.materializationCounts(), "closed": closed, "overdue": overdue, "states": states, "dropoff": drop, "review": review,
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
	m := s.summary0(c)
	s.mu.Lock()
	recovery := s.recoveryForCaseLocked(c, nil)
	releases, _ := recovery["source_releases"].([]map[string]interface{})
	if recovery["backfill"] != nil || len(releases) > 0 {
		m["recovery"] = recovery
	}
	s.mu.Unlock()
	if c.last != nil && c.last.Lung != nil { // lung_nodule: Fleischner recommendation, window, outcome
		m["lung"] = c.last.Lung
	}
	if c.last != nil && c.last.DR != nil { // diabetic_retinopathy: grade, rule, deadline, eye-clinic outcome
		m["dr"] = c.last.DR
	}
	if c.last != nil && c.last.Breast != nil { // breast: BI-RADS category, rule, deadline, assessment outcome
		m["breast"] = c.last.Breast
	}
	if c.last != nil && c.last.LDCT != nil { // lung_ldct: Lung-RADS category, rule, clocks, follow-up / work-up outcome
		m["ldct"] = c.last.LDCT
	}
	if c.last != nil && c.last.Ifhas != nil { // ifhas: screening cells, rule, per-domain follow-up status
		m["ifhas"] = c.last.Ifhas
	}
	if c.last != nil && c.last.CRC != nil {
		m["crc"] = c.last.CRC
	}
	return m
}

func (s *apiServer) summary0(c *apiCase) map[string]interface{} {
	closed := c.Closed
	if c.last != nil && ongoingCare(c.last) {
		closed = false
	}
	m := map[string]interface{}{"id": c.ID, "programme": c.Programme, "state": c.State, "dropoff": c.Dropoff, "owner": c.Owner,
		"provider": c.Provider, "referral_day": round4(c.SRDay), "deadline_day": c.Deadline, "overdue": c.Overdue, "priority": c.Priority,
		"consent": c.Consent, "review": c.Review, "closed": closed, "closed_day": round4(c.ClosedDay), "fixture": c.Fixture,
		"decisions": len(c.decisions), "api_overlay": s.overlayView(c, false)}
	if c.last != nil && (c.last.CRC != nil || s.dashboard["materialization"] != nil || s.report["materialization"] != nil) {
		// Root-only legacy retained artifacts predate this additive contract.
		// Keep their exported payloads/digests unchanged; new materializations
		// and every typed CRC care projection expose the care metadata.
		kind, _ := materializedCaseKind(c.last)
		m["case_kind"] = kind
		m["screening_complete"] = screeningComplete(c.last)
		m["ongoing_care"] = ongoingCare(c.last)
		if c.last.CRC != nil && c.last.CRC.Parent != "" {
			m["parent_case_id"] = c.last.CRC.Parent
			m["withdrawn"] = c.last.CRC.Withdrawn
		}
	}
	if c.last != nil && c.last.Inactive {
		m["inactive"] = true
		m["lifecycle"] = "retained"
		m["actionable"] = false
		m["programme_availability"] = "selected"
		if c.Programme == "" {
			m["programme_availability"] = "unavailable"
		}
		m["request_status"] = c.last.RequestStatus
		m["request_version"] = c.last.RequestVersion
		m["request_conflict"] = c.last.RequestConflict
	}
	s.mu.Lock()
	if c.patient.Seq > 0 {
		m["api_patient"] = s.patientViewLocked(c, false)
	}
	s.mu.Unlock()
	return m
}

// GET /api/cases?programme=crc&state=closed&owner=...&consent=granted&review=open&overdue=true&fixture=1&limit=50&offset=0&q=<id substring>
func (s *apiServer) getCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	match := func(c *apiCase) bool {
		eq := func(k, v string) bool { x := q.Get(k); return x == "" || x == v }
		if c.last != nil && c.last.Inactive && q.Get("state") != "inactive" && q.Get("include_inactive") != "true" {
			return false
		}
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
	if offset < 0 {
		offset = 0
	}
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
	if strings.HasSuffix(rest, "/review/claim") {
		rest, sub = strings.TrimSuffix(rest, "/review/claim"), "review/claim"
	} else if i := strings.LastIndex(rest, "/"); i >= 0 && (strings.HasSuffix(rest, "/audit") || strings.HasSuffix(rest, "/review") || strings.HasSuffix(rest, "/actions")) {
		rest, sub = rest[:i], rest[i+1:]
	}
	id, _ := url.PathUnescape(rest)
	c := s.cases[id]
	if c == nil {
		envelope := s.envelope
		if sub == "audit" {
			envelope = s.exportEnvelope
		}
		writeJSONResp(w, 404, envelope(map[string]interface{}{"error": "unknown case", "id": id,
			"hint": "case ids are loop ids such as ahs-mushrif/ServiceRequest/AHS-sr-1002; URL-encode the slashes or pass them as-is"}))
		return
	}
	switch {
	case sub == "" && r.Method == http.MethodGet:
		s.caseDetail(w, r, c)
	case sub == "audit" && r.Method == http.MethodGet:
		s.caseAudit(w, c)
	case sub == "review" && r.Method == http.MethodPost:
		s.caseReview(w, r, c)
	case sub == "review/claim" && r.Method == http.MethodPost:
		s.caseReviewClaim(w, r, c)
	case sub == "review/claim":
		w.Header().Set("Allow", "POST")
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "POST"}))
	case sub == "actions" && r.Method == http.MethodPost:
		s.caseAction(w, r, c)
	case sub == "actions":
		w.Header().Set("Allow", "POST")
		writeJSONResp(w, 405, s.exportEnvelope(map[string]interface{}{"error": "method not allowed", "allow": "POST"}))
	default:
		writeJSONResp(w, 405, s.exportEnvelope(map[string]interface{}{"error": "method not allowed"}))
	}
}

func (s *apiServer) caseDetail(w http.ResponseWriter, r *http.Request, c *apiCase) {
	viewer := apiPrincipal{role: r.Header.Get("X-Wasl-Role"), actor: r.Header.Get("X-Wasl-Actor")}
	body, err := exportGeneric(s.caseDetailFor(c, &viewer))
	if err != nil {
		writeJSONResp(w, 500, s.envelope(map[string]interface{}{"error": "case detail could not be encoded"}))
		return
	}
	body = pseudonymizeRegistrationFields(body, "").(map[string]interface{})
	writeJSONResp(w, 200, s.envelope(body))
}

func (s *apiServer) caseAudit(w http.ResponseWriter, c *apiCase) {
	audit, err := s.redactedCaseAuditBody(c)
	if err != nil {
		writeJSONResp(w, 500, s.exportEnvelope(map[string]interface{}{"error": "audit could not be encoded"}))
		return
	}
	writeJSONResp(w, 200, s.exportEnvelope(audit))
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
	var req struct {
		Item         string  `json:"item"`
		Decision     string  `json:"decision"`
		Note         string  `json:"note"`
		Read         string  `json:"read"`
		ExpectedHead *string `json:"expected_head"`
		ClaimHead    *string `json:"claim_head"`
	}
	if err := decodeAPIBody(w, r, maxReviewBodyBytes, &req); err != nil || req.Decision == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "body must be one strict JSON object with item, decision and note (and read with expected_head for an independent read); duplicate or unknown fields and oversized/trailing data are rejected"}))
		return
	}
	act := reviewActFor(c, req.Decision)
	if act == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "decision must be same|different|adequate|inadequate|confirm_contact, or a programme evidence label (including a signed CRC disposition on CRC cases; docs/API.md)"}))
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
	if utf8.RuneCountInString(actor) > maxActorRunes || utf8.RuneCountInString(req.Note) > maxNoteRunes {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "actor or note exceeds its character limit"}))
		return
	}
	if crcReviewDecisionLabel(req.Decision) && req.Item == "" {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "CRC confirmation requires one current obligation review item"}))
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
		it := s.queueItem(c.ID, req.Item)
		if it.Reason == CRCDispositionPurpose {
			active := false
			if c.last != nil && c.last.CRC != nil {
				for _, p := range c.last.CRC.Obligations {
					active = active || p.ReviewItem == it.ItemID && p.ExpectedHead == it.Head && p.Confirmation == "" && len(p.Blockers) == 0
				}
			}
			if !active {
				writeJSONResp(w, 409, s.envelope(map[string]interface{}{"error": "CRC obligation is blocked, signed or no longer current"}))
				return
			}
			if req.Decision != it.Verdict {
				writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "confirmation must match this obligation's documented proposal"}))
				return
			}
		}
		if it.Reason == CRCDispositionPurpose && !crcReviewDecisionLabel(req.Decision) || crcReviewDecisionLabel(req.Decision) && it.Reason != CRCDispositionPurpose {
			writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "CRC disposition labels are valid only on the crc_disposition item and must use its closed vocabulary"}))
			return
		}
	}
	if req.Read != "" || req.ExpectedHead != nil || req.ClaimHead != nil { // independent double review; without these fields the legacy path below is unchanged
		if msg := protocolShapeErr(req.Item, req.Read, req.ExpectedHead, req.ClaimHead, act); msg != "" {
			writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": msg}))
			return
		}
		s.protocolReview(w, c, s.queueItem(c.ID, req.Item), act, req.Read, *req.ExpectedHead, *req.ClaimHead, req.Decision, req.Note, apiPrincipal{role: role, actor: actor})
		return
	}
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		if req.Decision == "confirm_contact" && s.contactGuardLocked(c).Blocked {
			return 409, map[string]interface{}{"error": "contact is blocked by a recorded denial or opt-out; confirm_contact refused", "conflict": "consent"}
		}
		ev := map[string]interface{}{"case": c.ID, "item": req.Item, "decision": req.Decision, "note": req.Note, "role": role, "actor": actor,
			"action": act, "at": time.Now().UTC().Format(time.RFC3339Nano), "head_signed": c.last.Sig, "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		return 201, map[string]interface{}{"review": s.shownEventLocked(stored, apiPrincipal{role: role, actor: actor}),
			"note": "appended to the review ledger; in a live deployment this is published as a signed Provenance sign-off and replayed through the linker/closure path"}
	}()
	writeJSONResp(w, code, s.envelope(body)) // never write the response while holding s.mu
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

// bookingFor and its catalogue live in api_booking.go; canonical case data is never changed there.

// agentInvestigation is one cached evidence-investigator recommendation that a human sign-off actually consumed
// (agent-assisted double review). The agent never signs; the human context is the existing sign-off, unchanged.
type agentInvestigation struct {
	view   map[string]interface{}
	label  string
	nCites int
	link   string
	wrong  bool
}

// agentPaths: the served `main` run keeps the agent files one level up; any other run dir (focused tests) keeps them flat.
// Both files must exist, otherwise the run is ordinary.
func agentPaths(run string) (recs, replay string) {
	d := run
	if filepath.Base(filepath.Clean(run)) == "main" {
		d = filepath.Dir(filepath.Clean(run))
	}
	r, p := filepath.Join(d, "agent_recs.ndjson"), filepath.Join(d, "agent_replay.json")
	if _, e1 := os.Stat(r); e1 == nil {
		if _, e2 := os.Stat(p); e2 == nil {
			return r, p
		}
	}
	return "", ""
}

// readAgentProvenance accepts only the nested replay provenance (model, prompt_ver, schema_ver), all non-empty strings.
func readAgentProvenance(path string) (map[string]interface{}, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var raw struct {
		Provenance map[string]interface{} `json:"provenance"`
	}
	if json.Unmarshal(b, &raw) != nil {
		return nil, false
	}
	out := map[string]interface{}{"cached": true, "experimental": true}
	for _, k := range []string{"model", "prompt_ver", "schema_ver"} {
		v, ok := raw.Provenance[k].(string)
		if !ok || v == "" {
			return nil, false
		}
		out[k] = v
	}
	return out, true
}

// cleanCitations keeps only tool/arg/field/value of each citation; a citation missing any of them (or with an empty
// string) and anything malformed rejects the record.
func cleanCitations(raw json.RawMessage) ([]map[string]interface{}, bool) {
	out := []map[string]interface{}{}
	if len(raw) == 0 || string(raw) == "null" {
		return out, true
	}
	var in []map[string]interface{}
	if json.Unmarshal(raw, &in) != nil {
		return nil, false
	}
	for _, c := range in {
		m := map[string]interface{}{}
		for _, k := range []string{"tool", "arg", "field", "value"} {
			v, ok := c[k]
			if !ok {
				return nil, false
			}
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) == "" {
					return nil, false
				}
				m[k] = v
			case float64, bool:
				if k != "value" {
					return nil, false
				}
				m[k] = v
			default:
				return nil, false
			}
		}
		out = append(out, m)
	}
	return out, true
}

// loadAgent projects cached agent recommendations onto the loops whose sign-off consumed them. Any malformed or
// mismatched input yields no projection at all (ordinary run).
func (s *apiServer) loadAgent() {
	s.agentInv = map[string][]agentInvestigation{}
	s.agentProv = map[string]interface{}{"cached": false, "experimental": false}
	recsPath, replayPath := agentPaths(s.run)
	if recsPath == "" {
		return
	}
	prov, ok := readAgentProvenance(replayPath)
	if !ok {
		return
	}
	ar, err := LoadAgentRecs(recsPath)
	if err != nil || ar == nil {
		return
	}
	byItem := map[string]*AgentRec{}
	cites := map[string][]map[string]interface{}{}
	for _, r := range ar.recs {
		cs, ok := cleanCitations(r.Citations)
		if !ok || r.Item == "" || byItem[r.Item] != nil {
			return
		}
		byItem[r.Item], cites[r.Item] = r, cs
	}
	raised := map[string]*ReviewItem{}
	for _, items := range s.queue {
		for _, it := range items {
			raised[it.ItemID] = it
		}
	}
	got := map[string][]agentInvestigation{}
	for loop, sos := range s.signoffs {
		seen := map[string]string{} // item -> semantic key of its first sign-off; an identical repeat dedupes, a different one fails closed
		for _, so := range sos {
			item := fmt.Sprint(so["item"])
			key := fmt.Sprintf("%v|%v|%v|%v|%v|%v", so["loop"], so["link"], so["wrong"], so["reads"], so["role"], so["agent_read"])
			if prev, dup := seen[item]; dup {
				if prev != key {
					return // a superseded or conflicting sign-off for one item: no projection at all
				}
				continue
			}
			seen[item] = key
			label := fmt.Sprint(so["agent_read"])
			reads, _ := so["reads"].([]interface{})
			consumed := false
			for _, r := range reads {
				consumed = consumed || r == "agent"
			}
			if label == "" || label == "<nil>" || !consumed {
				continue
			}
			rec, it := byItem[item], raised[item]
			if rec == nil || it == nil || it.LoopID != loop || fmt.Sprint(so["loop"]) != loop || !isLinkQuestion(it) ||
				pairKey(rec.Pair[0], rec.Pair[1]) != pairKey(it.Pair[0], it.Pair[1]) || rec.Label != label {
				return
			}
			link := fmt.Sprint(so["link"])
			human := map[string]interface{}{"link": link, "reads": reads, "role": so["role"]}
			wrong, _ := so["wrong"].(bool)
			got[loop] = append(got[loop], agentInvestigation{label: label, nCites: len(cites[item]), link: link, wrong: wrong,
				view: map[string]interface{}{"item": item, "pair": []string{rec.Pair[0], rec.Pair[1]}, "label": rec.Label,
					"citations": cites[item], "human": human}})
		}
	}
	for loop := range got {
		l := got[loop]
		sort.Slice(l, func(i, j int) bool { return fmt.Sprint(l[i].view["item"]) < fmt.Sprint(l[j].view["item"]) })
	}
	s.agentInv, s.agentProv = got, prov
}

func (s *apiServer) agentFor(loop string) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, x := range s.agentInv[loop] {
		out = append(out, x.view)
	}
	return out
}

func (s *apiServer) agentProvenance() map[string]interface{} {
	if s.agentProv == nil {
		return map[string]interface{}{"cached": false, "experimental": false}
	}
	return s.agentProv
}

// pickAgentFixture adds `agent_review`: the first CRC loop by id that is closed with consent granted and whose agent
// `same` recommendation (with citations) was consumed by a human sign-off that finally said `same`, not wrong. It never
// takes a case that already carries another fixture tag, so existing fixture ids and single tags stay stable.
func (s *apiServer) pickAgentFixture() {
	if s.fixtures == nil {
		s.fixtures = map[string]string{}
	}
	delete(s.fixtures, "agent_review")
	ids := []string{}
	for id := range s.agentInv {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := s.cases[id]
		if c == nil || (c.Fixture != "" && c.Fixture != "agent_review") || c.Programme != "crc" || !c.Closed || c.Consent != "granted" {
			continue
		}
		for _, x := range s.agentInv[id] {
			if x.label == "same" && x.nCites > 0 && x.link == "same" && !x.wrong {
				s.fixtures["agent_review"], c.Fixture = id, "agent_review"
				return
			}
		}
	}
}

// ---------- coordinator actions (Workflow A) ----------
//
// POST /api/cases/{id}/actions  headers: X-Wasl-Role (coordinator), X-Wasl-Actor
// body: {"action": "assign_owner|snooze|contact_outcome|note", "expected_head": "<api_overlay.head>", ...fields of the action}
//
// Each accepted action is one hash-chained event in api_reviews.ndjson (the shared review ledger) and is projected
// into the case's `api_overlay`. Canonical decisions, out_log and signatures are never touched. A case's overlay head
// starts as the canonical decision signature and becomes the hash of its latest coordinator event, so concurrent
// writers on one case conflict (409) while different cases proceed independently.

const (
	coordEventType   = "coordinator_action"
	maxActionBody    = 8 << 10
	maxOwnerRunes    = 64
	maxActorRunes    = 64
	maxReasonRunes   = 200
	maxNoteRunes     = 1000
	maxSnoozeHorizon = 365 * 24 * time.Hour
)

var (
	contactOutcomes = map[string]bool{"reached": true, "no_answer": true, "voicemail": true, "wrong_number": true, "declined": true, "appointment_agreed": true}
	contactChannels = map[string]bool{"phone": true, "sms": true, "email": true, "in_person": true}
	// fields each action accepts besides action and expected_head
	actionFields = map[string][]string{
		"assign_owner":        {"owner"},
		"snooze":              {"until", "reason"},
		"contact_outcome":     {"outcome", "channel", "note"},
		"note":                {"text"},
		"booking_accept":      {"expected_suggestion_version", "slot_id", "expected_slot_version"},
		"booking_choose_slot": {"expected_suggestion_version", "slot_id", "expected_slot_version", "reason"},
		"booking_reschedule":  {"reservation_id", "expected_reservation_version", "expected_suggestion_version", "slot_id", "expected_slot_version", "reason"},
		"booking_cancel":      {"reservation_id", "expected_reservation_version", "reason"},
	}
)

// caseOverlay is the replayed coordinator state of one case (api metadata only).
type caseOverlay struct {
	head      string // "" until the first coordinator event
	n         int
	owner     string
	snooze    map[string]interface{}
	contacts  []map[string]interface{}
	notes     []map[string]interface{}
	agreement map[string]interface{}
	booking   caseBookingState
	lastAt    string
}

func (s *apiServer) caseHead(c *apiCase) string {
	if c.ov.head != "" {
		return c.ov.head
	}
	if c.last != nil && c.last.Sig != "" {
		return c.last.Sig
	}
	return "genesis"
}

// cleanText validates a bounded, valid-UTF-8 string without surrounding whitespace, control characters or Unicode
// format (Cf) characters other than ZWNJ/ZWJ after an Arabic-script letter (newline allowed if multiline).
func cleanText(v interface{}, max int, required, multiline bool) (string, error) {
	str, ok := v.(string)
	if !ok {
		return "", errors.New("must be a string")
	}
	if strings.TrimSpace(str) != str {
		return "", errors.New("must not start or end with whitespace")
	}
	if str == "" {
		if required {
			return "", errors.New("is required")
		}
		return "", nil
	}
	if !utf8.ValidString(str) || utf8.RuneCountInString(str) > max {
		return "", fmt.Errorf("must be valid UTF-8 of at most %d characters", max)
	}
	var prev rune
	for _, r := range str {
		if unicode.IsControl(r) && !(multiline && r == '\n') {
			return "", errors.New("must not contain control characters")
		}
		// ZWNJ and ZWJ are needed to write Arabic-script names (Persian, Urdu, Kurdish...): allowed only straight after an
		// Arabic-script letter. Every other format character (bidi overrides, zero-width space, BOM, soft hyphen...) is refused.
		joiner := (r == 0x200C || r == 0x200D) && unicode.Is(unicode.Arabic, prev)
		if unicode.Is(unicode.Cf, r) && !joiner {
			return "", errors.New("must not contain invisible format characters")
		}
		prev = r
	}
	return str, nil
}

// validateActionData checks and normalises the action-specific fields (shared by the write path and ledger replay).
func validateActionData(kind string, in map[string]interface{}) (map[string]interface{}, error) {
	allowed := actionFields[kind]
	if allowed == nil {
		return nil, fmt.Errorf("unknown action %q", kind)
	}
	for k := range in {
		ok := false
		for _, f := range allowed {
			ok = ok || f == k
		}
		if !ok {
			return nil, fmt.Errorf("field %q is not valid for %s", k, kind)
		}
	}
	out := map[string]interface{}{}
	text := func(k string, max int, required, multi bool) error {
		v, present := in[k]
		if !present {
			if required {
				return fmt.Errorf("%s is required", k)
			}
			return nil
		}
		t, err := cleanText(v, max, required, multi)
		if err != nil {
			return fmt.Errorf("%s %v", k, err)
		}
		if t != "" {
			out[k] = t
		}
		return nil
	}
	var err error
	switch kind {
	case "assign_owner":
		err = text("owner", maxOwnerRunes, true, false)
	case "note":
		err = text("text", maxNoteRunes, true, true)
	case "snooze":
		if err = text("until", 40, true, false); err == nil {
			t, perr := time.Parse(time.RFC3339, out["until"].(string))
			if perr != nil {
				return nil, errors.New("until must be an RFC 3339 timestamp")
			}
			out["until"] = t.UTC().Format(time.RFC3339)
			err = text("reason", maxReasonRunes, false, false)
		}
	case "booking_accept", "booking_choose_slot", "booking_reschedule", "booking_cancel":
		return validateBookingActionData(kind, in)
	case "contact_outcome":
		if err = text("outcome", 32, true, false); err == nil {
			err = text("channel", 32, true, false)
		}
		if err == nil && !contactOutcomes[fmt.Sprint(out["outcome"])] {
			err = errors.New("outcome must be one of reached|no_answer|voicemail|wrong_number|declined|appointment_agreed")
		}
		if err == nil && !contactChannels[fmt.Sprint(out["channel"])] {
			err = errors.New("channel must be one of phone|sms|email|in_person")
		}
		if err == nil {
			err = text("note", maxNoteRunes, false, true)
		}
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// applyOverlay projects one validated coordinator event onto the case overlay.
func (c *apiCase) applyOverlay(ev map[string]interface{}) {
	data, _ := ev["data"].(map[string]interface{})
	at, _ := ev["at"].(string)
	actor, _ := ev["actor"].(string)
	entry := map[string]interface{}{"at": at, "actor": actor, "event": ev["hash"]}
	for k, v := range data {
		entry[k] = v
	}
	switch ev["kind"] {
	case "assign_owner":
		c.ov.owner = data["owner"].(string)
	case "snooze":
		c.ov.snooze = entry
	case "contact_outcome":
		c.ov.contacts = append(c.ov.contacts, entry)
		if data["outcome"] == "appointment_agreed" {
			c.ov.agreement = entry
		}
	case "note":
		c.ov.notes = append(c.ov.notes, entry)
	}
	c.ov.head, c.ov.lastAt = ev["hash"].(string), at
	c.ov.n++
}

// overlayView is the api_overlay block of a case: summary form (head, owner, snooze, count) or full form (+ lists).
func (s *apiServer) overlayView(c *apiCase, full bool) map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overlayViewLocked(c, full)
}

func (s *apiServer) overlayViewLocked(c *apiCase, full bool) map[string]interface{} {
	o := &c.ov
	status := "ok"
	if s.ledgerErr != "" {
		status = "invalid" // the overlay may be partial: say so in the list as well as the detail
	}
	m := map[string]interface{}{"head": s.caseHead(c), "actions": o.n, "owner": o.owner, "last_action_at": o.lastAt, "ledger_status": status,
		"booking": s.bookingSummaryLocked(c)}
	if o.snooze != nil {
		m["snooze"] = o.snooze
	}
	// The latest contact outcome (codes and time only, no actor or note) and the run of consecutive no-answers that
	// ends it, so a worklist can place a case from its persisted outcomes without reading every case's detail.
	if n := len(o.contacts); n > 0 {
		last := o.contacts[n-1]
		m["last_contact"] = map[string]interface{}{"at": last["at"], "event": last["event"], "outcome": last["outcome"], "channel": last["channel"]}
		streak := 0
		for i := n - 1; i >= 0 && o.contacts[i]["outcome"] == "no_answer"; i-- {
			streak++
		}
		m["no_answer_streak"] = streak
	}
	if o.agreement != nil {
		m["last_appointment_agreement"] = map[string]interface{}{"at": o.agreement["at"], "event": o.agreement["event"]}
	}
	if full {
		m["contact_outcomes"] = nz(append([]map[string]interface{}{}, o.contacts...))
		m["notes"] = nz(append([]map[string]interface{}{}, o.notes...))
		m["canonical_owner"] = c.Owner
	}
	return m
}

func (s *apiServer) ledgerStatus() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerErr != "" {
		return "invalid"
	}
	return "ok"
}

// ledgerView is the ledger status shown on case detail: a failed replay leaves the overlay partial (events before the
// failing line), so the reader must be told.
func (s *apiServer) ledgerView() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerErr != "" {
		return map[string]interface{}{"status": "invalid", "detail": s.ledgerErr, "events_applied": len(s.reviews),
			"note": "api_overlay reflects only the ledger events before the failure; coordinator writes are refused"}
	}
	return map[string]interface{}{"status": "ok", "events_applied": len(s.reviews)}
}

func (s *apiServer) lastHashLocked() string {
	if n := len(s.reviews); n > 0 {
		return fmt.Sprint(s.reviews[n-1]["hash"])
	}
	return "genesis"
}

// appendEventLocked is the one durable append path for every ledger event (legacy review and coordinator action).
// The caller holds s.mu. The event is hashed and marshalled completely, written, synced and closed; only then does
// the in-memory chain move. A clean failure (nothing reached the file) leaves everything untouched. If the file's
// state is unknown (errLedgerIndeterminate) the ledger is marked failed and all later writes are refused, because
// the in-memory hash could otherwise fork from the file.
func (s *apiServer) appendEventLocked(ev map[string]interface{}) (map[string]interface{}, error) {
	path := filepath.Join(s.run, "api_reviews.ndjson")
	unlock, err := lockReviewLedger(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	// The process-local mutex cannot protect a shared run from another server.
	// Revalidate after acquiring the interprocess lock so two writers cannot both
	// acknowledge an event derived from the same previous hash.
	disk, err := readVerifiedLedger(path)
	if err == nil {
		err = sameLedger(disk, s.reviews)
	}
	if err != nil {
		s.ledgerErr = "ledger changed under another writer: " + err.Error()
		return nil, err
	}
	ev["hash"] = apiEventHash(ev)
	line, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	write := s.appendLine
	if write == nil { // servers built without loadAPI
		write = func(l []byte) error { return appendLedgerLine(filepath.Join(s.run, "api_reviews.ndjson"), l) }
	}
	if err := write(append(line, '\n')); err != nil {
		if errors.Is(err, errLedgerIndeterminate) {
			s.ledgerErr = "append failed and the ledger file state is unknown: " + err.Error()
		}
		return nil, err
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(line, &stored); err != nil {
		s.ledgerErr = "event written but not readable back: " + err.Error()
		return nil, err
	}
	s.reviews = append(s.reviews, stored)
	return stored, nil
}

// errLedgerIndeterminate marks append failures after which the file may or may not hold the line.
var errLedgerIndeterminate = errors.New("ledger state indeterminate")

type ledgerFile interface {
	Stat() (os.FileInfo, error)
	Write([]byte) (int, error)
	Sync() error
	Truncate(int64) error
	Close() error
}

func appendLedgerLine(path string, line []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	return appendToLedgerFile(f, line)
}

// appendToLedgerFile: write, sync and close the line. A write failure that was rolled back by a successful truncate
// is a clean error. A sync, truncate or close failure leaves the file state unknown (errLedgerIndeterminate).
func appendToLedgerFile(f ledgerFile, line []byte) error {
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	n, werr := f.Write(line)
	if werr == nil && n != len(line) {
		werr = io.ErrShortWrite
	}
	if werr != nil {
		terr := f.Truncate(st.Size()) // never leave a partial line behind
		cerr := f.Close()
		if terr != nil || cerr != nil {
			return fmt.Errorf("%w: write: %v; truncate: %v; close: %v", errLedgerIndeterminate, werr, terr, cerr)
		}
		return werr
	}
	if serr := f.Sync(); serr != nil {
		f.Truncate(st.Size())
		f.Close()
		return fmt.Errorf("%w: sync: %v", errLedgerIndeterminate, serr)
	}
	if cerr := f.Close(); cerr != nil {
		return fmt.Errorf("%w: close: %v", errLedgerIndeterminate, cerr)
	}
	return nil
}

// rejectDuplicateKeys fails on any object that repeats a key: encoding/json keeps the last value silently, so
// two readers could disagree about what was submitted.
func rejectDuplicateKeys(raw []byte) error {
	return rejectDuplicateKeysMode(raw, false)
}

// rejectSemanticDuplicateKeys also rejects case variants because typed Go JSON
// decoding matches struct fields with the same Unicode simple-fold semantics.
func rejectSemanticDuplicateKeys(raw []byte) error {
	return rejectDuplicateKeysMode(raw, true)
}

func foldJSONName(s string) string {
	var folded strings.Builder
	folded.Grow(len(s))
	for _, r := range s {
		for {
			next := unicode.SimpleFold(r)
			if next <= r {
				break
			}
			r = next
		}
		folded.WriteRune(r)
	}
	return folded.String()
}

func rejectDuplicateKeysMode(raw []byte, fold bool) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		if d == '[' {
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		seen := map[string]bool{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return err
			}
			k, _ := kt.(string)
			original := k
			if fold {
				k = foldJSONName(k)
			}
			if seen[k] {
				return fmt.Errorf("duplicate JSON key %q", original)
			}
			seen[k] = true
			if err := walk(); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	if err := walk(); err != nil && strings.HasPrefix(err.Error(), "duplicate JSON key") {
		return err
	}
	return nil // other syntax errors are reported by the normal decode
}

func apiEventHash(ev map[string]interface{}) string {
	cp := make(map[string]interface{}, len(ev))
	for k, v := range ev {
		if k != "hash" {
			cp[k] = v
		}
	}
	h := sha256.Sum256(canon(cp))
	return hex.EncodeToString(h[:])
}

func (s *apiServer) actionErr(w http.ResponseWriter, code int, msg string, extra map[string]interface{}) {
	body := map[string]interface{}{"error": msg}
	for k, v := range extra {
		body[k] = v
	}
	writeJSONResp(w, code, s.envelope(body))
}

func (s *apiServer) caseAction(w http.ResponseWriter, r *http.Request, c *apiCase) {
	role, actor := r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
	if err := Authorize(role, ActCoordinateCase); err != nil {
		s.actionErr(w, 403, err.Error(), map[string]interface{}{"role": role, "action": ActCoordinateCase})
		return
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		s.actionErr(w, 400, "X-Wasl-Actor header "+err.Error(), nil)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxActionBody))
	if err != nil {
		s.actionErr(w, 400, "body unreadable or larger than 8 KiB", nil)
		return
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		s.actionErr(w, 400, err.Error(), nil)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var fields map[string]interface{}
	if err := dec.Decode(&fields); err != nil || fields == nil {
		s.actionErr(w, 400, "body must be one JSON object", nil)
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		s.actionErr(w, 400, "body must contain exactly one JSON object", nil)
		return
	}
	kindV, expV := fields["action"], fields["expected_head"]
	delete(fields, "action")
	delete(fields, "expected_head")
	kind, _ := kindV.(string)
	if actionFields[kind] == nil {
		s.actionErr(w, 400, "action must be assign_owner|snooze|contact_outcome|note|booking_accept|booking_choose_slot|booking_reschedule|booking_cancel", nil)
		return
	}
	expected, ok := expV.(string)
	if !ok || expected == "" || len(expected) > 128 {
		s.actionErr(w, 400, "expected_head is required (the case's api_overlay.head)", nil)
		return
	}
	if _, given := fields["owner"]; kind == "assign_owner" && !given {
		fields["owner"] = actor
	}
	data, err := validateActionData(kind, fields)
	if err != nil {
		s.actionErr(w, 400, err.Error(), nil)
		return
	}
	now := time.Now().UTC()
	if kind == "snooze" {
		until, _ := time.Parse(time.RFC3339, data["until"].(string))
		if !until.After(now) || until.After(now.Add(maxSnoozeHorizon)) {
			s.actionErr(w, 400, "until must be in the future and within 365 days", nil)
			return
		}
	}

	// decide and record under the lock, but write the response only after releasing it
	code, body := func() (int, map[string]interface{}) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ledgerErr != "" {
			return 500, map[string]interface{}{"error": "review ledger failed validation; writes refused", "detail": s.ledgerErr}
		}
		if head := s.caseHead(c); expected != head {
			return 409, map[string]interface{}{"error": "stale expected_head: the case changed since it was read", "conflict": "head", "current_head": head}
		}
		if kind == "contact_outcome" {
			guard := s.contactGuardLocked(c)
			if !guard.Allowed {
				return 409, map[string]interface{}{"error": "contact outcomes need canonical contact permission granted and no demo denial or opt-out", "conflict": "consent", "consent": guard.Canonical, "contact_blocked": guard.Blocked}
			}
		}
		var booking *bookingTransition
		if bookingActionName(kind) {
			var failure *bookingFailure
			booking, failure = s.prepareBookingLocked(c, kind, data)
			if failure != nil {
				return failure.code, failure.body
			}
		}
		ev := map[string]interface{}{"type": coordEventType, "kind": kind, "case": c.ID, "data": data, "role": role, "actor": actor,
			"action": ActCoordinateCase, "at": now.Format(time.RFC3339Nano), "head": expected, "seq": c.ov.n + 1, "mode": "demo", "prev": s.lastHashLocked()}
		stored, err := s.appendEventLocked(ev)
		if err != nil {
			return 500, map[string]interface{}{"error": "could not append to the review ledger; nothing was recorded"}
		}
		if booking != nil {
			s.applyBookingLocked(c, stored, booking)
		}
		c.applyOverlay(stored)
		body := map[string]interface{}{"event": s.shownEventLocked(stored, apiPrincipal{role: role, actor: actor}), "api_overlay": s.overlayViewLocked(c, true),
			"note": "recorded in the review ledger as overlay metadata; decisions and signatures are unchanged and nothing was dispatched"}
		if booking != nil {
			body["booking"] = s.bookingForLocked(c.ID)
			body["note"] = "simulated reservation event appended; no provider system or patient was contacted and canonical decisions are unchanged"
		}
		return 201, body
	}()
	writeJSONResp(w, code, s.envelope(body))
}

// loadReviews replays the shared review ledger. Legacy review events and coordinator events share one hash chain.
// Anything malformed stops the replay at that line and sets ledgerErr, which refuses further ledger writes.
func (s *apiServer) loadReviews() {
	f, err := os.Open(filepath.Join(s.run, "api_reviews.ndjson"))
	if err != nil {
		if !os.IsNotExist(err) {
			s.ledgerErr = "cannot open ledger: " + err.Error()
		}
		return
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<16)
	for ln := 1; ; ln++ {
		line, rerr := readLedgerRecord(br)
		if errors.Is(rerr, errLedgerRecordTooLarge) {
			s.ledgerErr = fmt.Sprintf("line %d: %v", ln, rerr)
			return
		}
		if rerr != nil && rerr != io.EOF {
			s.ledgerErr = fmt.Sprintf("line %d: %v", ln, rerr)
			return
		}
		if len(line) == 0 && rerr == io.EOF {
			return
		}
		if rerr == io.EOF {
			s.ledgerErr = fmt.Sprintf("line %d: truncated (no trailing newline)", ln)
			return
		}
		if err := s.replayLedgerLine(bytes.TrimSuffix(line, []byte{'\n'})); err != nil {
			s.ledgerErr = fmt.Sprintf("line %d: %v", ln, err)
			return
		}
	}
}

func (s *apiServer) replayLedgerLine(line []byte) error {
	var ev map[string]interface{}
	if err := rejectDuplicateKeys(line); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	if err := dec.Decode(&ev); err != nil || ev == nil {
		return errors.New("not a JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing data after JSON object")
	}
	prev := "genesis"
	if n := len(s.reviews); n > 0 {
		prev = fmt.Sprint(s.reviews[n-1]["hash"])
	}
	if ev["prev"] != prev {
		return errors.New("broken hash chain (prev)")
	}
	if h, _ := ev["hash"].(string); h == "" || h != apiEventHash(ev) {
		return errors.New("hash mismatch")
	}
	if ev["type"] == patientEventType {
		if err := s.replayPatientEvent(ev); err != nil {
			return fmt.Errorf("patient journey: %v", err)
		}
		return nil
	}
	if ev["type"] == reviewClaimEventType {
		if err := s.replayClaim(ev); err != nil {
			return fmt.Errorf("review claim: %v", err)
		}
		s.reviews = append(s.reviews, ev)
		return nil
	}
	if ev["type"] == recoveryEventType {
		if err := s.replayRecovery(ev); err != nil {
			return fmt.Errorf("waitlist recovery: %v", err)
		}
		return nil
	}
	if _, isCoord := ev["type"]; !isCoord {
		if hasProtocolFields(ev) {
			if err := s.replayProtocol(ev); err != nil {
				return fmt.Errorf("independent review: %v", err)
			}
		}
		s.reviews = append(s.reviews, ev) // legacy review event: chain-checked only
		return nil
	}
	if ev["type"] != coordEventType {
		return fmt.Errorf("unknown event type %v", ev["type"])
	}
	kind, _ := ev["kind"].(string)
	id, _ := ev["case"].(string)
	c := s.cases[id]
	if c == nil {
		return errors.New("coordinator event for unknown case")
	}
	role, _ := ev["role"].(string)
	actor, _ := ev["actor"].(string)
	if !rbac[role][ActCoordinateCase] || ev["action"] != ActCoordinateCase {
		return errors.New("coordinator event with a role that lacks coordinate_case")
	}
	if _, err := cleanText(actor, maxActorRunes, true, false); err != nil {
		return errors.New("bad actor")
	}
	at, _ := ev["at"].(string)
	if _, err := time.Parse(time.RFC3339Nano, at); err != nil {
		return errors.New("bad timestamp")
	}
	if ev["head"] != s.caseHead(c) {
		return errors.New("head does not follow the case's previous event")
	}
	if seq, isNum := ev["seq"].(float64); !isNum || seq != float64(c.ov.n+1) { // an exact integer: 1.5 must not truncate to 1
		return errors.New("bad per-case sequence")
	}
	if ev["mode"] != "demo" {
		return errors.New("bad mode")
	}
	raw, ok := ev["data"].(map[string]interface{})
	if !ok {
		return errors.New("data must be an object")
	}
	norm, err := validateActionData(kind, raw)
	if err != nil || !reflect.DeepEqual(norm, raw) {
		return fmt.Errorf("invalid or non-canonical action data (%v)", err)
	}
	if kind == "contact_outcome" && !s.contactGuardLocked(c).Allowed {
		return errors.New("contact outcome without granted canonical consent or after demo denial/opt-out")
	}
	var booking *bookingTransition
	if bookingActionName(kind) {
		var failure *bookingFailure
		booking, failure = s.prepareBookingLocked(c, kind, norm)
		if failure != nil {
			return fmt.Errorf("invalid simulated booking transition: %v", failure.body["error"])
		}
	}
	s.reviews = append(s.reviews, ev)
	if booking != nil {
		s.applyBookingLocked(c, ev, booking)
	}
	c.applyOverlay(ev)
	return nil
}
