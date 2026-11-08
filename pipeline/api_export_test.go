package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exportLoop = "exp-prov/ServiceRequest/EXP-sr-1"

// exportRun writes a small synthetic run (one CRC loop with an evidence block, a link decision and a hold) to a temp
// dir, loads it through loadAPI and returns the server plus the run dir.
func exportRun(t *testing.T) (*apiServer, string) {
	t.Helper()
	run := t.TempDir()
	ch := appendBlock(nil, "link_decision", map[string]interface{}{"weakest_edge": map[string]interface{}{"a": "p|1", "b": "p|2"}})
	ch = appendBlock(ch, "evidence", map[string]interface{}{"agree": true, "label": "adequate", "quote": "CANARY-RAW-CLINICAL-TEXT caecum reached", "reader_model": "gemma-4-26b/cached"})
	ch = appendBlock(ch, "hold_decision", map[string]interface{}{"why": "consent_unknown"})
	d := &Decision{ID: "dec-1", LoopID: exportLoop, InSeq: 7, Prog: "crc", Provider: "exp-prov", State: "closed", Dropoff: "none",
		Policy: "v7strict", Contact: "granted", Chain: ch, Sig: "x"}
	d.Sig = decSig(d)
	m := LogMsg{Seq: 1, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(run, "out_log.ndjson"), append(b, '\n'), 0644)
	os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0644)
	os.WriteFile(filepath.Join(run, "review_queue.json"), []byte("[]"), 0644)
	return loadAPI(run, ""), run
}

// The export route is registered on the same mux as the detail and audit routes.
func exportRoutes(s *apiServer) http.Handler {
	mux := http.NewServeMux()
	caseAPI := s.withConsent(s.undoRoutes(s.caseRoutes))
	mux.HandleFunc("/api/cases/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			s.caseExportRoute(w, r)
			return
		}
		caseAPI.ServeHTTP(w, r)
	})
	return mux
}

func fetchBody(t *testing.T, h http.Handler, method, path string) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec.Code, rec.Body.Bytes()
}

func decodeMap(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v: %s", err, b)
	}
	return m
}

func TestAuditAnchorNoteSeparatesLocalAnchorsFromDesignedWORM(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, auditRaw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/audit")
	_, exportRaw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	audit := decodeMap(t, auditRaw)
	exportedAudit := decodeMap(t, exportRaw)["audit"].(map[string]interface{})

	for name, body := range map[string]map[string]interface{}{"live audit": audit, "export audit": exportedAudit} {
		note, _ := body["anchor_note"].(string)
		lower := strings.ToLower(note)
		for _, want := range []string{"local anchors", "materialised run", "out stream ledger heads", "fhir log hash", "external in-uae worm storage and co-signing", "designed, not built"} {
			if !strings.Contains(lower, want) {
				t.Errorf("%s anchor note %q does not disclose %q", name, note, want)
			}
		}
	}
	if audit["anchor_note"] != exportedAudit["anchor_note"] {
		t.Fatalf("live and export anchor notes differ: %q != %q", audit["anchor_note"], exportedAudit["anchor_note"])
	}
}

// The export's detail and audit are the live bodies after redaction only: with the redacted keys removed from the
// live bodies, they are JSON-equal (meta excluded), except for the fields the export sets itself.
func TestExportReusesDetailAndAudit(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	id := exportLoop
	_, exp := fetchBody(t, h, "GET", "/api/cases/"+id+"/export")
	_, det := fetchBody(t, h, "GET", "/api/cases/"+id)
	_, aud := fetchBody(t, h, "GET", "/api/cases/"+id+"/audit")
	e, d, a := decodeMap(t, exp), decodeMap(t, det), decodeMap(t, aud)
	delete(d, "meta")
	delete(a, "meta")
	rd := redactExportDetail(d)
	ra, _ := redactExport(a)
	ea := e["audit"].(map[string]interface{})
	for _, k := range []string{"chain_scope", "chain_note", "anchor_note"} {
		delete(ea, k)
	}
	ra.(map[string]interface{})["chain_verified"], ra.(map[string]interface{})["chain_length"] = true, float64(3)
	if got, want := string(canon(e["detail"])), string(canon(rd)); got != want {
		t.Fatalf("export detail differs from redacted GET /api/cases/{id}:\n got %s\nwant %s", got, want)
	}
	if got, want := string(canon(ea)), string(canon(ra)); got != want {
		t.Fatalf("export audit differs from redacted GET /api/cases/{id}/audit:\n got %s\nwant %s", got, want)
	}
	if e["schema"] != "wasl-case-export/1" || e["meta"] == nil || e["redacted"] != true {
		t.Fatalf("schema, meta or redacted flag wrong: %v", e["schema"])
	}
	if _, has := e["exported_at"]; has {
		t.Fatal("export must not carry exported_at")
	}
	if _, has := e["files"]; has {
		t.Fatal("export must not carry files")
	}
}

// content_sha256 equals sha(canon(document without content_sha256)); any mutation of the document breaks it.
func TestExportHashVerificationAndMutation(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	doc := decodeMap(t, raw)
	want, _ := doc["content_sha256"].(string)
	if len(want) != 64 {
		t.Fatalf("content_sha256 missing or malformed: %q", want)
	}
	verify := func(m map[string]interface{}) bool {
		claimed, _ := m["content_sha256"].(string)
		cp := map[string]interface{}{}
		for k, v := range m {
			if k != "content_sha256" {
				cp[k] = v
			}
		}
		return sha(canon(cp)) == claimed
	}
	if !verify(doc) {
		t.Fatal("served export fails its own hash verification")
	}
	// mutations: a changed case field, a changed chain body, a dropped detail key, an added key
	mutations := map[string]func(m map[string]interface{}){
		"case state": func(m map[string]interface{}) {
			m["detail"].(map[string]interface{})["case"].(map[string]interface{})["state"] = "open"
		},
		"chain body": func(m map[string]interface{}) {
			ev := m["detail"].(map[string]interface{})["evidence"].([]interface{})
			ev[0].(map[string]interface{})["label"] = "inadequate"
		},
		"audit verified flag": func(m map[string]interface{}) {
			m["audit"].(map[string]interface{})["chain_verified"] = false
		},
		"dropped detail key": func(m map[string]interface{}) { delete(m["detail"].(map[string]interface{}), "booking") },
		"added key":          func(m map[string]interface{}) { m["exported_at"] = "2026-01-01T00:00:00Z" },
	}
	for name, mut := range mutations {
		m := decodeMap(t, raw)
		mut(m)
		if verify(m) {
			t.Errorf("mutation %q was not detected by content_sha256", name)
		}
	}
}

// Repeated exports while nothing changes are byte-identical, and a new case review changes the export.
func TestExportRepeatDeterminism(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, a := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	_, b := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if string(a) != string(b) {
		t.Fatal("repeated exports differ with unchanged history")
	}
	s.reviews = append(s.reviews, map[string]interface{}{"case": exportLoop, "decision": "same", "actor": "r1"})
	_, c := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if string(a) == string(c) {
		t.Fatal("a new review in history must change the export")
	}
}

func TestExportDropsUserSuppliedIdentityText(t *testing.T) {
	s, _ := exportRun(t)
	s.reviews = append(s.reviews, map[string]interface{}{"case": exportLoop, "decision": "same", "actor": "rectal bleeding", "target_actor": "polyp", "role": RoleReviewer})
	s.cases[exportLoop].ov.owner = "rectal bleeding"
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	for _, secret := range []string{"rectal bleeding", "polyp"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("export leaked user-supplied identity text %q: %s", secret, raw)
		}
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	detail := doc["detail"].(map[string]interface{})
	review := detail["api_reviews"].([]interface{})[0].(map[string]interface{})
	if review["role"] != RoleReviewer || review["decision"] != "same" {
		t.Fatalf("structured audit metadata was not preserved: %v", review)
	}
}

func TestExportPreservesServerReviewReasons(t *testing.T) {
	s, _ := exportRun(t)
	s.queue[exportLoop] = []*ReviewItem{{ItemID: "image", Reason: "image_prep_inadequate"}, {ItemID: "link", Reason: "link_contradiction_confirm"}}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	for _, reason := range []string{"image_prep_inadequate", "link_contradiction_confirm"} {
		if !bytes.Contains(raw, []byte(reason)) {
			t.Fatalf("export dropped server-generated review reason %q", reason)
		}
	}
}

func TestExportDropsUserControlledOperationalCode(t *testing.T) {
	s, _ := exportRun(t)
	const clinical = "rectal_bleeding"
	s.cases[exportLoop].ov.snooze = map[string]interface{}{"until": "2026-11-09T00:00:00Z", "reason": clinical}
	s.reviews = append(s.reviews, map[string]interface{}{"case": exportLoop, "type": coordEventType, "data": map[string]interface{}{"until": "2026-11-09T00:00:00Z", "reason": clinical}})
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	if bytes.Contains(raw, []byte(clinical)) {
		t.Fatalf("export trusted a user-controlled operational code: %s", raw)
	}
}

// Building exports neither writes nor reads-to-modify any run file.
func TestExportNoRunFileChanges(t *testing.T) {
	s, run := exportRun(t)
	h := exportRoutes(s)
	snap := func() map[string]string {
		out := map[string]string{}
		entries, _ := os.ReadDir(run)
		for _, e := range entries {
			b, _ := os.ReadFile(filepath.Join(run, e.Name()))
			out[e.Name()] = string(b)
		}
		return out
	}
	before := snap()
	for i := 0; i < 3; i++ {
		fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	}
	after := snap()
	if len(before) != len(after) {
		t.Fatalf("export created or removed run files: %d -> %d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("export changed run file %s", k)
		}
	}
}

func TestExportStatusCodes(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	if code, _ := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export"); code != 200 {
		t.Fatalf("known case: want 200, got %d", code)
	}
	if code, _ := fetchBody(t, h, "GET", "/api/cases/no%2Fsuch%2Fcase/export"); code != 404 {
		t.Fatalf("unknown case: want 404, got %d", code)
	}
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH"} {
		if code, _ := fetchBody(t, h, m, "/api/cases/"+exportLoop+"/export"); code != 405 {
			t.Fatalf("%s export: want 405, got %d", m, code)
		}
	}
	// the handler itself answers 404 for a path it does not own
	if code, _ := fetchBody(t, http.HandlerFunc(s.caseExportRoute), "GET", "/api/cases/x"); code != 404 {
		t.Fatalf("non-export path: want 404, got %d", code)
	}
}

// Leak canaries: the export must not carry salts, cache paths, raw model messages or reasoning, prompt paths or
// file-system paths. The run's directory path is never written, only its base name in meta.run.
func TestExportLeakCanaries(t *testing.T) {
	s, run := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	body := string(raw)
	if strings.Contains(body, run) {
		t.Errorf("export contains the run directory path %q", run)
	}
	for _, banned := range []string{
		"pair_salt", "salt", "gemma_cache", "cache.json", "/Users/", "/tmp/", "prompt_ver",
		"reasoning", "raw_message", "system_prompt", "messages",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("export contains forbidden token %q", banned)
		}
	}
}

const exportCanary = "CANARY-RAW-CLINICAL-TEXT"
const exportCanaryProse = "CANARY-RAW-CLINICAL-TEXT caecum reached and prep was good"

// exportCanaryServer plants the canary separately in every path the export projects. String-valued fields that are
// not evidence, timeline or citations carry the prose form (the prose guard is their only defence under unknown
// keys); evidence, timeline and citations also carry the bare one-token canary under unknown keys, which only the
// allowlists can stop.
func exportCanaryServer(t *testing.T) *apiServer {
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	tainted := func() map[string]interface{} {
		return map[string]interface{}{"agree": true, "label": "adequate", "quote": exportCanaryProse, "gemma_quote": exportCanaryProse,
			"reasoning": exportCanaryProse, "future_field": exportCanary, "two_words": "caecum " + exportCanary, "report": "exp-prov/DiagnosticReport/r1",
			"fields":          map[string]interface{}{"regex": "y/y/n/n", "reader": "y/y/n/n", "extra": exportCanary},
			"reader_b_fields": map[string]interface{}{"label": "adequate", "quote": exportCanaryProse, "caecum": exportCanary},
			"nested":          map[string]interface{}{"raw_text": exportCanaryProse, "k": []interface{}{exportCanaryProse, "ok"}}}
	}
	// detail.evidence, current.chain, audit.chain: an evidence block in the decision's chain
	ch := append(append([]Block{}, c.last.Chain...), appendBlock(c.last.Chain, "evidence", tainted())[len(c.last.Chain)])
	c.last.Chain = ch
	for _, d := range c.decisions {
		d.Chain = ch
	}
	// review_items and their chains
	rch := appendBlock(nil, "evidence", tainted())
	s.queue[exportLoop] = []*ReviewItem{{ItemID: "it1", LoopID: exportLoop, Verdict: "same", Conf: 0.9, Quote: exportCanaryProse, Reason: exportCanaryProse, Chain: rch, Head: rch[0].Hash}}
	s.signoffs[exportLoop] = []map[string]interface{}{{"item": "it1", "reviewer": "r1", "note": exportCanaryProse, "evidence": "adequate", "free": exportCanaryProse}}
	s.reviews = append(s.reviews, map[string]interface{}{"case": exportLoop, "decision": "same", "actor": "r1", "reason": exportCanaryProse, "message": exportCanaryProse})
	s.facts[exportLoop] = []map[string]interface{}{{"resource": "Observation", "id": "o1", "text": exportCanary, "display": exportCanary, "seq": uint64(1), "type": "Observation", "status": "final"}}
	s.agentInv[exportLoop] = []agentInvestigation{{view: map[string]interface{}{"item": "it1", "recommendation": "same", "rationale": exportCanaryProse,
		"citations": []interface{}{map[string]interface{}{"tool": "get_record", "arg": "p|1", "field": "dob", "value": "1976-01-01", "quote": exportCanary, "excerpt": exportCanaryProse, "extra": exportCanary}}}}}
	s.agentProv = map[string]interface{}{"cached": true, "experimental": true, "model": "m1", "notes": exportCanaryProse, "operator_comment": exportCanaryProse, "banner": exportCanaryProse}
	s.booking = map[string]interface{}{"cases": map[string]interface{}{exportLoop: map[string]interface{}{"slot": "2026-02-01", "suggestion": exportCanaryProse, "reason": exportCanaryProse, "detail": exportCanaryProse}}}
	// outreach, consent_log, escalations and case fields
	for _, d := range append([]*Decision{c.last}, c.decisions...) {
		d.Outreach = []Outreach{{Kind: "appointment_reminder", Channel: exportCanaryProse, Sender: exportCanaryProse, Suppress: exportCanaryProse}}
		d.ConsentLog = []ConsentTransition{{Event: exportCanaryProse, Kind: exportCanaryProse, To: "held", Contact: "granted"}}
		d.Escalations = []string{exportCanaryProse}
	}
	c.Owner = exportCanaryProse
	return s
}

func jsonOf(v interface{}) string { b, _ := json.Marshal(v); return string(b) }

func TestExportRedactsEveryPath(t *testing.T) {
	s := exportCanaryServer(t)
	h := exportRoutes(s)
	_, live := fetchBody(t, h, "GET", "/api/cases/"+exportLoop)
	_, liveAud := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/audit")
	d, a := decodeMap(t, live), decodeMap(t, liveAud)
	first := func(v interface{}) map[string]interface{} {
		if l, ok := v.([]interface{}); ok && len(l) > 0 {
			m, _ := l[0].(map[string]interface{})
			return m
		}
		return map[string]interface{}{}
	}
	cur := d["current"].(map[string]interface{})
	dec0 := first(d["decisions"])
	ri := first(d["review_items"])
	ag := first(d["agent_investigations"])
	// 1. every path is populated with the canary
	paths := map[string]interface{}{
		"detail.evidence": d["evidence"], "detail.review_items": d["review_items"], "detail.review_items[0].chain": ri["chain"],
		"detail.current.chain": cur["chain"], "audit.chain": a["chain"], "detail.signoffs": d["signoffs"], "detail.api_reviews": d["api_reviews"],
		"detail.timeline": d["timeline"], "detail.agent_investigations[0].citations": ag["citations"], "detail.booking": d["booking"],
		"detail.outreach": d["outreach"], "detail.decisions[0].outreach": dec0["outreach"], "detail.decisions[0].consent_log": dec0["consent_log"],
		"detail.decisions[0].escalations": dec0["escalations"], "detail.agent_provenance": d["agent_provenance"], "detail.case": d["case"],
		"audit.history": a["history"],
	}
	for name, v := range paths {
		if !strings.Contains(jsonOf(v), exportCanary) {
			t.Errorf("test setup: path %s is not populated with the canary: %s", name, jsonOf(v))
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	origChain := string(canon(s.cases[exportLoop].last.Chain))
	origQueue := jsonOf(s.queue[exportLoop])
	// 2. the canary never appears in the export
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	body := string(raw)
	for _, banned := range []string{"CANARY", "caecum", "prep was good", `"quote"`, "gemma_quote", `"reasoning"`, "future_field", "two_words", `"raw_text"`, `"message"`, `"text"`, `"rationale"`, `"excerpt"`, `"extra"`} {
		if strings.Contains(body, banned) {
			t.Errorf("export contains %q", banned)
		}
	}
	e := decodeMap(t, raw)
	au := e["audit"].(map[string]interface{})
	if au["chain_verified"] != true || au["chain_length"] != float64(4) || au["chain_scope"] != "current" || e["redacted"] != true {
		t.Fatalf("audit flags wrong: %v %v %v", au["chain_verified"], au["chain_length"], au["chain_scope"])
	}
	if !strings.Contains(au["chain_note"].(string), "cannot be recomputed") {
		t.Fatal("missing hash-recompute note")
	}
	norm := strings.ReplaceAll(body, ": ", ":")
	for _, keep := range []string{`"label":"adequate"`, `"agree":true`, "exp-prov/DiagnosticReport/r1", `"reviewer":"r1"`, `"evidence":"adequate"`,
		`"tool":"get_record"`, `"arg":"p|1"`, `"field":"dob"`, `"value":"1976-01-01"`, `"regex":"y/y/n/n"`} {
		if !strings.Contains(norm, keep) {
			t.Errorf("export lost structured field %s", keep)
		}
	}
	// 3. the redacted chain cannot be recomputed, while the original chain_verified stays true
	var chain []Block
	cb, _ := json.Marshal(au["chain"])
	json.Unmarshal(cb, &chain)
	if len(chain) != 4 || verifyChain(chain) {
		t.Errorf("redacted chain (len %d) unexpectedly verifies; the note would be wrong", len(chain))
	}
	if !verifyChain(s.cases[exportLoop].last.Chain) || au["chain_verified"] != true {
		t.Fatal("original chain no longer verifies or flag lost")
	}
	// 4. live source bodies unchanged
	if after := string(canon(s.cases[exportLoop].last.Chain)); after != origChain {
		t.Fatal("export mutated the live chain")
	}
	if jsonOf(s.queue[exportLoop]) != origQueue {
		t.Fatal("export mutated the review queue")
	}
	_, live2 := fetchBody(t, h, "GET", "/api/cases/"+exportLoop)
	_, liveAud2 := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/audit")
	if string(live) != string(live2) || string(liveAud) != string(liveAud2) {
		t.Fatal("live detail/audit changed after export")
	}
}

// One- and two-word clinical phrases under unknown keys cannot leak from evidence, timeline or citations.
func TestExportAllowlistDropsShortPhrases(t *testing.T) {
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	ev := map[string]interface{}{"label": "adequate", "report": "exp-prov/DiagnosticReport/r1", "finding": "polyp", "dx": "sessile adenoma",
		"reader_b_fields": map[string]interface{}{"caecum": "reached", "prep": "poor", "label": "inadequate", "dx": "tubular adenoma"}}
	ch := appendBlock(c.last.Chain, "evidence", ev)
	c.last.Chain = ch
	c.decisions[0].Chain = ch
	s.facts[exportLoop] = []map[string]interface{}{{"seq": uint64(2), "type": "Observation", "id": "p/Observation/1", "display": "melaena", "code_text": "adenoma", "value": "positive"}}
	s.agentInv[exportLoop] = []agentInvestigation{{view: map[string]interface{}{"item": "it1", "citations": []interface{}{
		map[string]interface{}{"tool": "get_record", "arg": "p|1", "field": "dob", "value": "1976-01-01", "result": "sessile adenoma"},
		map[string]interface{}{"tool": "get_report", "arg": "r1", "field": "dx", "value": "tubular adenoma", "snip": "malignancy"}}}}}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	body := string(raw)
	for _, banned := range []string{"polyp", "adenoma", "melaena", "malignancy", "reached", `"poor"`, "positive", `"dx":`, "display", "code_text"} {
		if strings.Contains(body, banned) {
			t.Errorf("export leaked %q", banned)
		}
	}
	norm := strings.ReplaceAll(body, ": ", ":")
	for _, keep := range []string{`"label":"adequate"`, `"label":"inadequate"`, `"tool":"get_record"`, `"tool":"get_report"`, `"field":"dob"`, `"id":"p/Observation/1"`, `"1976-01-01"`} {
		if !strings.Contains(norm, keep) {
			t.Errorf("export lost %s", keep)
		}
	}
	if n := strings.Count(norm, `"tool":`); n != 2 {
		t.Errorf("citations should be retained (2), got %d", n)
	}
}

// Operational hint/reason/why fields and look-alike keys survive when they carry no clinical text; clinical text under
// the same keys is dropped.
func TestExportKeepsSafeMetadata(t *testing.T) {
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	ev := map[string]interface{}{"label": "adequate", "report": "exp-prov/DiagnosticReport/r1", "image_reader": map[string]interface{}{"model": "img-1", "prep": "inadequate", "hint": "image_prep_conflict"}}
	ch := appendBlock(c.last.Chain, "evidence", ev)
	c.last.Chain = ch
	c.decisions[0].Chain = ch
	s.queue[exportLoop] = []*ReviewItem{{ItemID: "it1", LoopID: exportLoop, Verdict: "same", Reason: "second_identifier"},
		{ItemID: "it2", LoopID: exportLoop, Verdict: "same", Reason: "poor prep"}}
	s.signoffs[exportLoop] = []map[string]interface{}{{"item": "it1", "reviewer": "r1", "raw_id": "abc123", "quoted_by": "r2", "raw_sha": "deadbeef", "hint": "prep_grade_one_click", "why": "consent_unknown"}}
	s.booking = map[string]interface{}{"cases": map[string]interface{}{exportLoop + "x": map[string]interface{}{}}}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	norm := strings.ReplaceAll(string(raw), ": ", ":")
	for _, keep := range []string{`"hint":"image_prep_conflict"`, `"hint":"prep_grade_one_click"`, `"reason":"second_identifier"`, `"why":"consent_unknown"`,
		`"raw_id":"abc123"`, `"quoted_by":"r2"`, `"raw_sha":"deadbeef"`, "case does not need a colonoscopy slot at the end of the run"} {
		if !strings.Contains(norm, keep) {
			t.Errorf("export dropped safe metadata %s", keep)
		}
	}
	if strings.Contains(norm, "poor prep") {
		t.Error("clinical text under reason leaked")
	}
}

// Integers beyond 2^53 and large or small floats keep their literal text, and the hash verifies only with a
// number-preserving parse.
func TestExportLargeNumberCanonicalisation(t *testing.T) {
	s, _ := exportRun(t)
	s.facts[exportLoop] = []map[string]interface{}{{"seq": uint64(9007199254740993), "type": "Observation", "id": "p/Observation/1", "day": 1e21, "arrival_day": 1.5e-7}}
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	for _, lit := range []string{"9007199254740993", "1e+21", "1.5e-7"} {
		if !strings.Contains(string(raw), lit) {
			t.Errorf("literal %s not served", lit)
		}
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	claimed := m["content_sha256"].(string)
	delete(m, "content_sha256")
	if sha(canon(m)) != claimed {
		t.Fatal("hash does not verify with a number-preserving parse")
	}
	var lossy map[string]interface{}
	json.Unmarshal(raw, &lossy)
	delete(lossy, "content_sha256")
	if sha(canon(lossy)) == claimed {
		t.Fatal("a float64 parse unexpectedly reproduces the hash; the documented caveat would be unnecessary")
	}
}

// Numbers keep their literal text through hashing, and the hash verifies over the served bytes.
func TestExportNumberCanonicalisation(t *testing.T) {
	s, _ := exportRun(t)
	h := exportRoutes(s)
	_, raw := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export")
	if _, raw2 := fetchBody(t, h, "GET", "/api/cases/"+exportLoop+"/export"); string(raw) != string(raw2) {
		t.Fatal("not deterministic")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var m map[string]interface{}
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	claimed := m["content_sha256"].(string)
	delete(m, "content_sha256")
	if sha(canon(m)) != claimed {
		t.Fatal("hash does not verify with UseNumber parse")
	}
}
