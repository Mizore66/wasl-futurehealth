package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func testPersonDecision(id, prog string) *Decision {
	return &Decision{ID: "d-" + id, LoopID: id, Prog: prog, Provider: "provider", SRDay: 1, Now: 20, State: "open", Dropoff: "not_booked",
		Deadline: 31, Contact: "granted", Policy: "v7strict+test", Sig: sha([]byte(id)), NotifDay: -1, AckDay: -1, ProcDay: -1}
}

func newPersonTestAPI(t *testing.T) (*apiServer, map[string]string) {
	t.Helper()
	run := t.TempDir()
	for name, body := range map[string]string{"fhir_log.ndjson": "synthetic fhir\n", "out_log.ndjson": "synthetic out\n"} {
		if err := os.WriteFile(filepath.Join(run, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(run, "report7.json"), []byte(`{"live_vs_replay_final_mismatch":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &apiServer{run: run, meta: map[string]interface{}{"api": apiVersion}, report: map[string]interface{}{"live_vs_replay_final_mismatch": float64(0)},
		cases: map[string]*apiCase{}, queue: map[string][]*ReviewItem{}, signoffs: map[string][]map[string]interface{}{}, facts: map[string][]map[string]interface{}{},
		ids: map[string][]map[string]string{}, claims: map[reviewQuestionKey]*reviewClaimState{}, rstates: map[reviewQuestionKey]*reviewProtocolState{}}
	regs := map[string]string{}
	progs := []string{"crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"}
	for i, prog := range progs {
		id := fmt.Sprintf("provider/ServiceRequest/case-%d", i)
		reg := fmt.Sprintf("provider|registration-%d", i%3)
		d := testPersonDecision(id, prog)
		c := &apiCase{ID: id, Programme: prog, State: d.State, Dropoff: d.Dropoff, Provider: d.Provider, SRDay: d.SRDay, Deadline: d.Deadline,
			Consent: "granted", last: d, decisions: []*Decision{d}}
		s.cases[id], s.order, regs[id] = c, append(s.order, id), reg
		s.facts[id] = []map[string]interface{}{{"seq": uint64(i + 1), "type": "ServiceRequest", "id": id, "provider": "provider", "day": float64(1), "raw_sha": "abc123", "patient_key": reg}}
		s.ids[id] = []map[string]string{{"registration": reg, "provider": "provider", "role": "referral"}}
	}
	writePersonTestSnapshot(t, s, regs, map[string][]string{
		"provider|registration-0": {"provider|registration-0", "provider|registration-3"},
		"provider|registration-1": {"provider|registration-1"},
		"provider|registration-2": {"provider|registration-2"},
		"provider|registration-9": {"provider|registration-9"}, // downstream-only canonical registrations need no invented case
	})
	return s, regs
}

func writePersonTestSnapshot(t *testing.T, s *apiServer, regs map[string]string, clusters map[string][]string) {
	t.Helper()
	hash := func(name string) string {
		b, err := os.ReadFile(filepath.Join(s.run, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		return hex.EncodeToString(h[:])
	}
	doc := identityClusterSnapshot{Schema: identityClusterSchema, Policy: "v7strict+test", FHIRSHA256: hash("fhir_log.ndjson"), OutLogSHA256: hash("out_log.ndjson")}
	roots := make([]string, 0, len(clusters))
	for root := range clusters {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		members := append([]string{}, clusters[root]...)
		sort.Strings(members)
		doc.Clusters = append(doc.Clusters, identityCluster{Root: root, Registrations: members})
	}
	ids := make([]string, 0, len(regs))
	for id := range regs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc.Cases = append(doc.Cases, identityCase{CaseID: id, Registration: regs[id]})
	}
	b, _ := json.Marshal(doc)
	if err := os.WriteFile(filepath.Join(s.run, "identity_clusters.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func personRequest(t *testing.T, h http.Handler, method, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("X-Wasl-Role", "auditor")
	r.Header.Set("X-Wasl-Actor", "spoofed")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCaseDetailDoesNotExposeTimelinePatientKeys(t *testing.T) {
	s, regs := newPersonTestAPI(t)
	caseID := "provider/ServiceRequest/case-0"
	w := personRequest(t, secureTestHandler(t, s, ""), http.MethodGet, "/api/cases/"+caseID, "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("GET case detail: %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if timeline, ok := body["timeline"].([]interface{}); !ok || len(timeline) != 1 {
		t.Fatalf("case timeline was not preserved: %s", w.Body.String())
	}
	for _, raw := range regs {
		if strings.Contains(w.Body.String(), raw) {
			t.Errorf("raw registration %q leaked in authenticated case response: %s", raw, w.Body.String())
		}
	}
	if strings.Contains(w.Body.String(), `"patient_key"`) {
		t.Errorf("internal patient key metadata reached case response: %s", w.Body.String())
	}
	if got := s.facts[caseID][0]["patient_key"]; got != regs[caseID] {
		t.Errorf("serialization removed internal person-filtering key: got %v, want %q", got, regs[caseID])
	}
}

func TestPersonJourneyTrustedAggregationAndReadOnlySecurity(t *testing.T) {
	s, regs := newPersonTestAPI(t)
	h := secureTestHandler(t, s, "")
	target := "/api/people?case_id=" + "provider%2FServiceRequest%2Fcase-0"
	before := map[string][]byte{}
	for _, name := range []string{"fhir_log.ndjson", "out_log.ndjson", "identity_clusters.json", "report7.json"} {
		b, _ := os.ReadFile(filepath.Join(s.run, name))
		before[name] = b
	}
	w := personRequest(t, h, http.MethodGet, target, "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("GET person: %d %s", w.Code, w.Body.String())
	}
	var journey map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &journey); err != nil {
		t.Fatal(err)
	}
	if journey["schema"] != "wasl-person-journey/1" {
		t.Fatalf("schema: %v", journey["schema"])
	}
	person := journey["person"].(map[string]interface{})
	if len(person["case_ids"].([]interface{})) != 3 || len(person["programmes"].([]interface{})) != 3 {
		t.Fatalf("trusted cases not aggregated exactly: %v", person)
	}
	if !personIDPattern.MatchString(person["id"].(string)) {
		t.Fatalf("person id: %v", person["id"])
	}
	for _, raw := range regs {
		if strings.Contains(w.Body.String(), raw) {
			t.Fatalf("raw registration %q leaked: %s", raw, w.Body.String())
		}
	}
	for _, forbidden := range []string{"evidence", "current", "review_items", "signoffs", "api_reviews", "api_review_states", "review_ledger", "resolution", "concealed-r1"} {
		if strings.Contains(w.Body.String(), `"`+forbidden+`"`) {
			t.Fatalf("person response exposed %s", forbidden)
		}
	}
	byID := "/api/people/" + person["id"].(string)
	if got := personRequest(t, h, http.MethodGet, byID, "reviewer-token-1234567890"); got.Code != 200 {
		t.Fatalf("GET by person id: %d %s", got.Code, got.Body.String())
	}
	for _, tc := range []struct {
		method, target, token string
		want                  int
	}{
		{http.MethodGet, target, "", 401},
		{http.MethodGet, target, "auditor-token-1234567890", 403},
		{http.MethodGet, "/api/people?case_id=missing", "reviewer-token-1234567890", 404},
		{http.MethodGet, "/api/people/person_nope", "reviewer-token-1234567890", 400},
		{http.MethodGet, "/api/people/person_0000000000000000000000000000000000000000000000000000000000000000", "reviewer-token-1234567890", 404},
		{http.MethodGet, "/api/people", "reviewer-token-1234567890", 400},
		{http.MethodGet, target + "&role=auditor", "reviewer-token-1234567890", 400},
		{http.MethodPost, target, "reviewer-token-1234567890", 405},
	} {
		got := personRequest(t, h, tc.method, tc.target, tc.token)
		if got.Code != tc.want {
			t.Errorf("%s %s = %d, want %d: %s", tc.method, tc.target, got.Code, tc.want, got.Body.String())
		}
		if tc.want == 405 && got.Header().Get("Allow") != "GET" {
			t.Errorf("405 Allow = %q", got.Header().Get("Allow"))
		}
	}
	registrationOnly := personRequest(t, h, http.MethodGet, "/api/people/"+personIDForRegistrations([]string{"provider|registration-9"}), "reviewer-token-1234567890")
	if registrationOnly.Code != 404 {
		t.Fatalf("registration-only cluster invented a person journey: %d %s", registrationOnly.Code, registrationOnly.Body.String())
	}
	for name, want := range before {
		got, _ := os.ReadFile(filepath.Join(s.run, name))
		if !bytes.Equal(got, want) {
			t.Errorf("read mutated %s", name)
		}
	}
}

func TestPersonNamesakesAndOverlayReviewsCannotChangeMembership(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	h := secureTestHandler(t, s, "")
	get := func(caseID string) map[string]interface{} {
		w := personRequest(t, h, http.MethodGet, "/api/people?case_id="+caseID, "reviewer-token-1234567890")
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", caseID, w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	first := get("provider%2FServiceRequest%2Fcase-0")
	second := get("provider%2FServiceRequest%2Fcase-1")
	firstPerson := first["person"].(map[string]interface{})
	secondPerson := second["person"].(map[string]interface{})
	if firstPerson["id"] == secondPerson["id"] {
		t.Fatal("namesakes without a canonical edge received one person ID")
	}
	firstCases := map[string]bool{}
	for _, id := range firstPerson["case_ids"].([]interface{}) {
		firstCases[id.(string)] = true
	}
	for _, id := range secondPerson["case_ids"].([]interface{}) {
		if firstCases[id.(string)] {
			t.Fatalf("disjoint namesake cases were combined: %v %v", firstPerson, secondPerson)
		}
	}
	// Same-looking names and API-overlay link answers are not canonical linker membership.
	s.cases["provider/ServiceRequest/case-0"].last.Provider = s.cases["provider/ServiceRequest/case-1"].last.Provider
	s.reviews = append(s.reviews, map[string]interface{}{"case": "provider/ServiceRequest/case-1", "decision": "same", "note": "concealed-r1-note",
		"prev": "concealed-r1-prev", "hash": "concealed-r1-hash", "resolution": "concealed-r1-resolution", "actor": "concealed-r1-actor"})
	after := get("provider%2FServiceRequest%2Fcase-1")
	if !reflect.DeepEqual(second, after) {
		t.Fatalf("API-overlay review changed canonical person membership: %v %v", second, after)
	}
	if strings.Contains(jsonOf(after), "concealed-r1") {
		t.Fatal("concealed review data reached the person response")
	}
}

func TestPersonProjectionFailsClosed(t *testing.T) {
	for _, tc := range []string{"missing", "malformed", "overlap", "stale", "unmapped", "mismatch"} {
		t.Run(tc, func(t *testing.T) {
			s, regs := newPersonTestAPI(t)
			path := filepath.Join(s.run, "identity_clusters.json")
			switch tc {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				if err := os.WriteFile(path, []byte(`{"schema":"wrong"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "overlap":
				writePersonTestSnapshot(t, s, regs, map[string][]string{
					"provider|registration-0": {"provider|registration-0", "provider|registration-1"}, "provider|registration-1": {"provider|registration-1"}, "provider|registration-2": {"provider|registration-2"}, "provider|registration-3": {"provider|registration-3"},
				})
			case "stale":
				if err := os.WriteFile(filepath.Join(s.run, "fhir_log.ndjson"), []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unmapped":
				delete(regs, "provider/ServiceRequest/case-6")
				writePersonTestSnapshot(t, s, regs, map[string][]string{"provider|registration-0": {"provider|registration-0", "provider|registration-3"}, "provider|registration-1": {"provider|registration-1"}, "provider|registration-2": {"provider|registration-2"}})
			case "mismatch":
				s.report["live_vs_replay_final_mismatch"] = float64(1)
				if err := os.WriteFile(filepath.Join(s.run, "report7.json"), []byte(`{"live_vs_replay_final_mismatch":1}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			h := secureTestHandler(t, s, "")
			w := personRequest(t, h, http.MethodGet, "/api/people?case_id=provider%2FServiceRequest%2Fcase-0", "reviewer-token-1234567890")
			if w.Code != 503 || !strings.Contains(w.Body.String(), "person_projection_unavailable") || strings.Contains(w.Body.String(), "case_ids") {
				t.Fatalf("projection failure was not fail closed: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPersonConcurrentReadsDeterministic(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	h := secureTestHandler(t, s, "")
	const n = 24
	responses := make([]string, n)
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := personRequest(t, h, http.MethodGet, "/api/people?case_id=provider%2FServiceRequest%2Fcase-0", "reviewer-token-1234567890")
			codes[i], responses[i] = w.Code, w.Body.String()
		}(i)
	}
	wg.Wait()
	for i := range responses {
		if codes[i] != 200 || responses[i] != responses[0] {
			t.Fatalf("concurrent response %d differs: %d", i, codes[i])
		}
	}
}

func TestPersonObligationsUseCanonicalProgrammeFields(t *testing.T) {
	mk := func(prog string, mutate func(*Decision)) *apiCase {
		d := testPersonDecision("p/ServiceRequest/"+prog, prog)
		mutate(d)
		return &apiCase{ID: d.LoopID, Programme: prog, State: d.State, Deadline: d.Deadline, Overdue: d.Overdue, Closed: d.State == "closed", last: d, decisions: []*Decision{d}}
	}
	cases := []*apiCase{
		mk("crc", func(d *Decision) { d.Now, d.Overdue = 40, true }),
		mk("cervical", func(d *Decision) {}),
		mk("lung_nodule", func(d *Decision) {
			d.Lung = &LungDecision{WindowStart: 2, WindowEnd: 40, TissueDue: 25, Surveillance: &LungSurv{Loop: d.LoopID + "#surv", State: "due", DueDay: 100, WindowStart: 50, Round: 1, Next: &LungSurv{Loop: d.LoopID + "#surv2", State: "overdue", DueDay: 400, WindowStart: 300, Round: 2}}}
			d.State = "awaiting_tissue"
			d.Now, d.Overdue = 30, true
		}),
		mk("diabetic_retinopathy", func(d *Decision) {
			d.DR = &DRDecision{DeadlineDay: 42, Next: &DRNext{DueDay: 365, Kind: "rescreen", Basis: DRNextAnnual}}
		}),
		mk("breast", func(d *Decision) { d.Breast = &BreastDecision{DeadlineDay: 21, TreatmentDueDay: 35} }),
		mk("lung_ldct", func(d *Decision) {
			d.LDCT = &LDCTDecision{DueDay: 365, DeadlineDay: 30, Rec: &LDCTRec{Action: LDCTFollowup}}
		}),
		mk("ifhas", func(d *Decision) { d.Ifhas = &IfhasDecision{DeadlineDay: 28} }),
	}
	got := []map[string]interface{}{}
	for _, c := range cases {
		got = append(got, personObligations(c)...)
	}
	sortObligations(&apiServer{}, got)
	wantSources := []string{"sr_day+deadline", "dr.deadline_day", "dr.next_due.due_day", "breast.deadline_day", "breast.treatment_referral_due_day", "ldct.due_day", "ldct.deadline_day", "ifhas.deadline_day", "lung.window_end_day", "lung.tissue_due_day", "lung.surveillance.due_day"}
	if len(got) != 13 { // CRC and cervical share a source; surveillance has two rounds.
		t.Fatalf("obligations = %d, want 13: %+v", len(got), got)
	}
	seen := map[string]map[string]interface{}{}
	for _, o := range got {
		source := o["source_field"].(string)
		seen[source] = o
		if source == "breast.treatment_referral_due_day" && o["status"] != "informational" {
			t.Errorf("treatment obligation is actionable: %+v", o)
		}
		if source == "dr.next_due.due_day" && o["status"] != "scheduled" {
			t.Errorf("unseen DR next due missing: %+v", o)
		}
		if source == "lung.tissue_due_day" && o["status"] != "overdue" {
			t.Errorf("canonical tissue overdue state missing: %+v", o)
		}
		if source == "lung.window_end_day" && o["status"] != "open" {
			t.Errorf("tissue overdue incorrectly advanced the lung window: %+v", o)
		}
	}
	for _, source := range wantSources {
		if _, ok := seen[source]; !ok {
			t.Errorf("missing %s in %+v", source, got)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i-1]["due_day"].(float64) > got[i]["due_day"].(float64) {
			t.Fatal("obligations not sorted by due day")
		}
	}
}

func TestPersonOutreachFoldFlagsAndWithheld(t *testing.T) {
	mk := func(id, prog string, ds ...*Decision) *apiCase {
		last := ds[len(ds)-1]
		return &apiCase{ID: id, Programme: prog, State: last.State, Consent: last.Contact, last: last, decisions: ds}
	}
	d := func(id, prog string, o Outreach) *Decision {
		d := testPersonDecision(id, prog)
		d.Outreach = []Outreach{o}
		return d
	}
	a1 := d("a", "crc", Outreach{Kind: "reminder", Event: "event", Released: true})
	a2 := d("a", "crc", Outreach{Kind: "reminder", Event: "event", Released: true})
	b := d("b", "cervical", Outreach{Kind: "reminder", Event: "event", Suppress: SupConsentHold})
	c := d("c", "lung_nodule", Outreach{Kind: "reminder", Event: "event", Suppress: SupConsent})
	cases := []*apiCase{mk("a", "crc", a1, a2), mk("b", "cervical", b), mk("c", "lung_nodule", c)}
	flags := personOutreachFlags(&apiServer{}, cases)
	if len(flags) != 2 {
		t.Fatalf("flags = %+v, want duplicate and overlap only", flags)
	}
	var duplicate, overlap map[string]interface{}
	for _, flag := range flags {
		if flag["match"] == "duplicate" {
			duplicate = flag
		}
		if flag["match"] == "overlap" {
			overlap = flag
		}
	}
	if duplicate == nil || duplicate["actionable"] != true || len(duplicate["intents"].([]map[string]interface{})) != 3 {
		t.Fatalf("duplicate: %+v", duplicate)
	}
	first := duplicate["intents"].([]map[string]interface{})[0]
	if first["first_decision_id"] != a1.ID || first["latest_decision_id"] != a2.ID || first["lifecycle"] != "released" {
		t.Fatalf("history was not folded to latest: %+v", first)
	}
	if overlap == nil || overlap["actionable"] != true || len(overlap["intents"].([]map[string]interface{})) != 3 {
		t.Fatalf("overlap: %+v", overlap)
	}
	for _, f := range flags {
		for _, in := range f["intents"].([]map[string]interface{}) {
			if in["case_id"] == "c" && in["lifecycle"] != "withheld" {
				t.Errorf("denied intent = %s", in["lifecycle"])
			}
		}
	}
	onlyHistory := personOutreachFlags(&apiServer{}, []*apiCase{mk("a", "crc", a1, a2)})
	if len(onlyHistory) != 0 {
		t.Fatalf("repeated decision history alone flagged: %+v", onlyHistory)
	}
}

func testNextCandidate(cases []*apiCase) personActionCandidate {
	obligations := []map[string]interface{}{}
	for _, c := range cases {
		obligations = append(obligations, personObligations(c)...)
	}
	s := &apiServer{}
	sortObligations(s, obligations)
	candidates := personActionCandidates(s, cases, obligations, personOutreachFlags(s, cases))
	if len(candidates) == 0 {
		return personActionCandidate{kind: "none", reasons: []string{"no_open_coordinator_action"}}
	}
	return candidates[0]
}

func TestPersonNextActionOrderingConsentAndNoAction(t *testing.T) {
	mk := func(id, drop, contact, priority string, overdue bool, deadline float64) *apiCase {
		d := testPersonDecision(id, "crc")
		d.Dropoff, d.Contact, d.Priority, d.Overdue, d.Deadline = drop, contact, priority, overdue, deadline
		return &apiCase{ID: id, Programme: "crc", State: "open", Dropoff: drop, Consent: contact, Priority: priority, Overdue: overdue, Deadline: deadline, last: d, decisions: []*Decision{d}}
	}
	hold := mk("hold", "unlinked", "unknown", "", false, 0)
	if got := testNextCandidate([]*apiCase{hold}); got.kind != "none" || len(got.reasons) != 1 || got.reasons[0] != "no_open_coordinator_action" {
		t.Fatalf("clinical hold became action: %+v", got)
	}
	consent := mk("b", "no_show", "opted_out", "", false, 40)
	provider := mk("a", "referral_rejected", "denied", "", false, 50)
	got := testNextCandidate([]*apiCase{consent, provider})
	if got.kind != "review_contact_permission" || got.caseID != consent.ID {
		t.Fatalf("consent or provider-only ranking wrong: %+v", got)
	}
	urgent := mk("z", "not_booked", "granted", "urgent", false, 90)
	overdue := mk("y", "not_booked", "granted", "", true, 80)
	got = testNextCandidate([]*apiCase{provider, overdue, urgent})
	if got.caseID != urgent.ID || got.priority != "urgent" || !containsString(got.reasons, "case_priority_urgent") {
		t.Fatalf("urgent did not rank first: %+v", got)
	}
	got = testNextCandidate([]*apiCase{provider, overdue})
	if got.caseID != overdue.ID || !got.overdue {
		t.Fatalf("overdue did not rank first: %+v", got)
	}
	soon := mk("soon", "not_booked", "granted", "soon", false, 20)
	if got = testNextCandidate([]*apiCase{soon}); got.priority != "routine" {
		t.Fatalf("non-urgent canonical priority escaped the person contract: %+v", got)
	}
	a := mk("same-deadline-a", "not_booked", "granted", "", false, 20)
	b := mk("same-deadline-b", "not_booked", "granted", "", false, 20)
	first := testNextCandidate([]*apiCase{a, b})
	second := testNextCandidate([]*apiCase{b, a})
	if !reflect.DeepEqual(first, second) || first.caseID != a.ID {
		t.Fatalf("non-deterministic tie break: %+v %+v", first, second)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPersonNamesakesAndUncertainLinksStayDisjoint(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	h := secureTestHandler(t, s, "")
	personForCase := func(n int) map[string]interface{} {
		target := fmt.Sprintf("/api/people?case_id=provider%%2FServiceRequest%%2Fcase-%d", n)
		w := personRequest(t, h, http.MethodGet, target, "reviewer-token-1234567890")
		if w.Code != http.StatusOK {
			t.Fatalf("case %d person: %d %s", n, w.Code, w.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body["person"].(map[string]interface{})
	}
	a, b := personForCase(0), personForCase(1)
	if a["id"] == b["id"] {
		t.Fatal("namesake registrations without a trusted cluster edge received one person ID")
	}
	aCases := map[string]bool{}
	for _, id := range a["case_ids"].([]interface{}) {
		aCases[id.(string)] = true
	}
	for _, id := range b["case_ids"].([]interface{}) {
		if aCases[id.(string)] {
			t.Fatalf("uncertain or rejected namesake link merged case %v", id)
		}
	}
}

func TestPersonOverlayReviewsCannotChangeMembershipOrLeak(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	s.reviews = []map[string]interface{}{{
		"case": "provider/ServiceRequest/case-0", "decision": "same", "protocol": "concealed",
		"read": "r1", "note": "concealed-r1-note", "hash": "concealed-r1-hash", "prev": "concealed-r1-prev",
		"resolution": "concealed-r1-resolution", "actor": "concealed-r1-actor",
	}}
	h := secureTestHandler(t, s, "")
	w := personRequest(t, h, http.MethodGet, "/api/people?case_id=provider%2FServiceRequest%2Fcase-0", "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("person read: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "concealed-r1") {
		t.Fatalf("concealed API review data leaked: %s", w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := len(body["person"].(map[string]interface{})["case_ids"].([]interface{})); got != 3 {
		t.Fatalf("API overlay review changed canonical membership: %d", got)
	}
}

func personJourneyForCase(t *testing.T, h http.Handler, caseID string) map[string]interface{} {
	t.Helper()
	w := personRequest(t, h, http.MethodGet, "/api/people?case_id="+url.QueryEscape(caseID), "reviewer-token-1234567890")
	if w.Code != http.StatusOK {
		t.Fatalf("person journey for %s: %d %s", caseID, w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func personJourneyCase(t *testing.T, body map[string]interface{}, caseID string) map[string]interface{} {
	t.Helper()
	for _, raw := range body["cases"].([]interface{}) {
		c := raw.(map[string]interface{})
		if c["case_id"] == caseID {
			return c
		}
	}
	t.Fatalf("case %s missing from person journey", caseID)
	return nil
}

func TestPersonJourneyExcludesNearMissLinksAndRecords(t *testing.T) {
	s, regs := newPersonTestAPI(t)
	caseID := "provider/ServiceRequest/case-0"
	referral, trusted, namesake := regs[caseID], "provider|registration-3", "provider|namesake-untrusted"
	c := s.cases[caseID]
	c.last.Chain = appendBlock(c.last.Chain, "link_decision", map[string]interface{}{
		"loop": caseID, "referral_patient": referral, "downstream_event": "provider/DiagnosticReport/namesake-report", "hops": 1, "near_miss": true,
		"weakest_edge": map[string]interface{}{"a": referral, "b": namesake, "p": 0.2, "basis": "model"},
	})
	c.last.Chain = appendBlock(c.last.Chain, "link_decision", map[string]interface{}{
		"loop": caseID, "referral_patient": referral, "downstream_event": "provider/DiagnosticReport/trusted-report", "hops": 1, "near_miss": false,
		"weakest_edge": map[string]interface{}{"a": referral, "b": trusted, "p": 1, "basis": "human"},
	})
	s.ids[caseID] = []map[string]string{
		{"registration": referral, "provider": "provider", "role": "referral"},
		{"registration": trusted, "provider": "provider", "role": "downstream"},
		{"registration": namesake, "provider": "provider", "role": "downstream"},
	}
	s.facts[caseID] = []map[string]interface{}{
		{"seq": uint64(1), "type": "ServiceRequest", "id": caseID, "provider": "provider", "day": float64(1), "patient_key": referral},
		{"seq": uint64(2), "type": "DiagnosticReport", "id": "provider/DiagnosticReport/trusted-report", "provider": "provider", "day": float64(2), "patient_key": trusted},
		{"seq": uint64(3), "type": "DiagnosticReport", "id": "provider/DiagnosticReport/namesake-report", "provider": "provider", "day": float64(3), "patient_key": namesake},
	}

	body := personJourneyForCase(t, secureTestHandler(t, s, ""), caseID)
	detail := personJourneyCase(t, body, caseID)["detail"].(map[string]interface{})
	links := detail["links"].([]interface{})
	if len(links) != 1 || links[0].(map[string]interface{})["downstream_event"] != "provider/DiagnosticReport/trusted-report" {
		t.Fatalf("near-miss link was presented as trusted: %+v", links)
	}
	identities := detail["identities"].([]interface{})
	if len(identities) != 2 {
		t.Fatalf("untrusted namesake identity was copied into the journey: %+v", identities)
	}
	allowedIdentities := map[string]bool{pseudonymizeRegistration(referral): true, pseudonymizeRegistration(trusted): true}
	for _, raw := range identities {
		if !allowedIdentities[raw.(map[string]interface{})["registration"].(string)] {
			t.Fatalf("unexpected identity: %+v", raw)
		}
	}
	timeline := detail["timeline"].([]interface{})
	if len(timeline) != 2 {
		t.Fatalf("untrusted namesake timeline was copied into the journey: %+v", timeline)
	}
	encoded := jsonOf(body)
	if strings.Contains(encoded, "namesake-report") || strings.Contains(encoded, "patient_key") {
		t.Fatalf("near-miss provenance or internal registration metadata leaked: %s", encoded)
	}
}

func TestPersonJourneyContactBlockedUnlessCanonicalGranted(t *testing.T) {
	for _, tc := range []struct {
		name, canonical, overlay string
		blocked                  bool
	}{
		{name: "unknown without overlay", canonical: "unknown", blocked: true},
		{name: "unknown with overlay grant", canonical: "unknown", overlay: "granted", blocked: true},
		{name: "canonical grant", canonical: "granted", blocked: false},
		{name: "canonical and overlay grant", canonical: "granted", overlay: "granted", blocked: false},
		{name: "canonical denial with overlay grant", canonical: "denied", overlay: "granted", blocked: true},
		{name: "overlay denial", canonical: "granted", overlay: "denied", blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newPersonTestAPI(t)
			caseID := "provider/ServiceRequest/case-0"
			s.cases[caseID].Consent, s.cases[caseID].last.Contact = tc.canonical, tc.canonical
			if tc.overlay != "" {
				s.reviews = []map[string]interface{}{{"case": caseID, "decision": "consent_" + tc.overlay, "action": ActRecordConsent}}
			}
			body := personJourneyForCase(t, secureTestHandler(t, s, ""), caseID)
			contact := personJourneyCase(t, body, caseID)["contact"].(map[string]interface{})
			if contact["contact_blocked"] != tc.blocked {
				t.Fatalf("canonical=%s overlay=%s contact_blocked=%v, want %v", tc.canonical, tc.overlay, contact["contact_blocked"], tc.blocked)
			}
		})
	}
}

func TestPersonJourneyProjectsOnlyLatestDecisionHolds(t *testing.T) {
	s, _ := newPersonTestAPI(t)
	caseID := "provider/ServiceRequest/case-0"
	held := testPersonDecision(caseID, "crc")
	held.Chain = appendBlock(nil, "hold_decision", map[string]interface{}{"loop": caseID, "state": "open", "rule": "review_hold", "reason": "review_hold"})
	closed := testPersonDecision(caseID, "crc")
	closed.ID, closed.State, closed.Now, closed.Dropoff = "d-closed", "closed", 30, ""
	c := s.cases[caseID]
	c.decisions, c.last = []*Decision{held, closed}, closed
	c.State, c.Closed, c.Dropoff = "closed", true, ""
	if got := len(s.caseDetailFor(c, nil)["holds"].([]map[string]interface{})); got != 1 {
		t.Fatalf("test setup did not retain the historical hold: %d", got)
	}
	body := personJourneyForCase(t, secureTestHandler(t, s, ""), caseID)
	detail := personJourneyCase(t, body, caseID)["detail"].(map[string]interface{})
	if got := len(detail["holds"].([]interface{})); got != 0 {
		t.Fatalf("closed case exposed a stale historical hold as current: %+v", detail["holds"])
	}

	closed.State = "open"
	c.State, c.Closed = "open", false
	closed.Chain = appendBlock(closed.Chain, "hold_decision", map[string]interface{}{"loop": caseID, "state": "open", "rule": "current_hold"})
	body = personJourneyForCase(t, secureTestHandler(t, s, ""), caseID)
	detail = personJourneyCase(t, body, caseID)["detail"].(map[string]interface{})
	if got := len(detail["holds"].([]interface{})); got != 1 {
		t.Fatalf("latest canonical hold was not retained: %+v", detail["holds"])
	}
}

func TestPersonOverlapRequiresActionableIntentsAcrossProgrammes(t *testing.T) {
	mk := func(id, programme string, outreach Outreach) *apiCase {
		d := testPersonDecision(id, programme)
		d.Outreach = []Outreach{outreach}
		return &apiCase{ID: id, Programme: programme, last: d, decisions: []*Decision{d}}
	}
	cases := []*apiCase{
		mk("a", "crc", Outreach{Kind: "reminder", Event: "event", Released: true}),
		mk("b", "crc", Outreach{Kind: "reminder", Event: "event", Suppress: SupConsentHold}),
		mk("c", "cervical", Outreach{Kind: "reminder", Event: "event", Suppress: SupConsent}),
	}
	for _, flag := range personOutreachFlags(&apiServer{}, cases) {
		if flag["match"] == "overlap" {
			t.Fatalf("withheld second-programme intent created an actionable overlap: %+v", flag)
		}
	}
}
