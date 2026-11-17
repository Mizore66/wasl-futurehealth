package main

// The positive fixture is unedited parent-80ef8fd4 HTTP history over parent
// ClosureCore.Apply Decisions (nonempty Sig) and its Python catalogue, not a
// reconstructed ledger. See its manifest for the disclosed private harness.
// Synthetic tests below deliberately forge/re-hash lines. An unkeyed SHA chain
// does NOT authenticate a file writer: on an old OR fresh ledger before its first
// stamp, a chain-valid unstamped acquisition can waive readiness, and ONLY that
// predicate. The boundary-positive test pins this intentional compatibility cost.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const pr205Fixture = "testdata/pr205_legacy_booking_history"
const pr205Returned = "pr205-returned"
const pr205Ready = "pr205-ready"

type pr205Manifest struct {
	Parent string `json:"parent_commit"`
	Files  map[string]struct {
		SHA   string `json:"sha256"`
		Bytes int    `json:"bytes"`
	} `json:"files"`
	Cases map[string]struct {
		State      string `json:"state"`
		Pending    int    `json:"pending_length"`
		Sig        string `json:"sig"`
		Head       string `json:"expected_head"`
		Provenance string `json:"canonical_provenance"`
	} `json:"cases"`
	HTTP []struct {
		Request     map[string]interface{} `json:"request"`
		Status      int                    `json:"status"`
		Event       map[string]interface{} `json:"event"`
		Reservation map[string]interface{} `json:"reservation"`
	} `json:"HTTP"`
	F2 struct {
		Canonical bool `json:"canonical"`
		Catalogue bool `json:"catalogue"`
		HTTP      int  `json:"HTTP"`
		Withheld  bool `json:"withheld"`
	} `json:"F2"`
}

func pr205Run(t *testing.T, history bool) (string, pr205Manifest) {
	t.Helper()
	var m pr205Manifest
	if err := json.Unmarshal(actRead(t, filepath.Join(pr205Fixture, "manifest.json")), &m); err != nil {
		t.Fatal(err)
	}
	if m.Parent != "80ef8fd44bf63ec7a7f04fb866bb2b0984b57469" {
		t.Fatal("wrong parent")
	}
	run := t.TempDir()
	for name, f := range m.Files {
		b := actRead(t, filepath.Join(pr205Fixture, name))
		sum := sha256.Sum256(b)
		if len(b) != f.Bytes || hex.EncodeToString(sum[:]) != f.SHA {
			t.Fatalf("fixture hash/length %s", name)
		}
		if name != "api_reviews.ndjson" || history {
			if err := os.WriteFile(filepath.Join(run, name), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return run, m
}
func pr205Snapshot(t *testing.T, run string, m pr205Manifest) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for name := range m.Files {
		out[name] = actRead(t, filepath.Join(run, name))
	}
	return out
}
func pr205Unchanged(t *testing.T, run string, before map[string][]byte) {
	t.Helper()
	for name, want := range before {
		got := actRead(t, filepath.Join(run, name))
		if name == "api_reviews.ndjson" {
			if !bytes.HasPrefix(got, want) {
				t.Fatal("original parent ledger prefix rewritten")
			}
		} else if !bytes.Equal(got, want) {
			t.Fatalf("canonical/catalogue changed: %s", name)
		}
	}
}
func pr205Held(t *testing.T, s *apiServer, id string) {
	t.Helper()
	v := s.bookingFor(id)
	r, _ := v["readiness"].(map[string]interface{})
	if r["ready"] != false || v["reservable"] != false {
		t.Fatalf("held projection %s: %v", id, v)
	}
}
func pr205Alternative(t *testing.T, s *apiServer, id string) string {
	t.Helper()
	for _, v := range s.bookingCase(id)["eligible_slots"].([]interface{}) {
		q := v.(map[string]interface{})
		slot := s.bookingSlots[q["slot_id"].(string)]
		if slot.reservationID == "" && slot.id != s.cases[id].ov.booking.slotID {
			return slot.id
		}
	}
	t.Fatalf("no alternative for %s", id)
	return ""
}
func pr205HTTP(t *testing.T, srv *httptest.Server, id, path, role, body string) (int, map[string]interface{}) {
	t.Helper()
	r, e := http.NewRequest("POST", srv.URL+"/api/cases/"+id+path, strings.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("X-Wasl-Role", role)
	r.Header.Set("X-Wasl-Actor", "synthetic-upgrade-test")
	r.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		t.Fatal(e)
	}
	return resp.StatusCode, decodeMap(t, b)
}

// T1-T5, T7: consume genuine parent bytes; exercise live remedy, refusals and restart.
func TestPR205AuthenticParentHistory(t *testing.T) {
	run, m := pr205Run(t, true)
	before := pr205Snapshot(t, run, m)
	s, srv := serveAgent(t, run)
	item := seedEvidenceSource(t, s, "pr205-unrelated")
	// The seeded synthetic source report is appended; the parent bytes before it must stay intact.
	fhir := actRead(t, filepath.Join(run, "fhir_log.ndjson"))
	if !bytes.HasPrefix(fhir, before["fhir_log.ndjson"]) {
		t.Fatal("parent FHIR log prefix rewritten")
	}
	before["fhir_log.ndjson"] = fhir
	if s.ledgerErr != "" {
		t.Fatalf("authentic parent history must replay, not fail readiness: %s", s.ledgerErr)
	}
	w := httptest.NewRecorder()
	s.healthz(w, httptest.NewRequest("GET", "/healthz", nil))
	health := decodeMap(t, w.Body.Bytes())
	if w.Code != 200 || health["dependencies"].(map[string]interface{})["review_ledger"] != "ok" {
		t.Fatalf("healthz: %s", w.Body.String())
	}
	if len(m.HTTP) != 2 || !m.F2.Canonical || !m.F2.Catalogue || m.F2.HTTP != 201 || m.F2.Withheld {
		t.Fatal("F1/F2 parent HTTP proofs missing; do not claim Pending history")
	}
	for _, r := range m.HTTP {
		id := r.Event["case"].(string)
		c := s.cases[id]
		proof := m.Cases[id]
		if proof.Sig == "" || proof.Sig == "genesis" || proof.Provenance != "real_pipeline" || proof.Head != proof.Sig || r.Request["expected_head"] != proof.Sig || c.last.Sig != proof.Sig {
			t.Fatalf("non-authentic canonical head %s", id)
		}
		if c.State != proof.State || len(c.last.Pending) != proof.Pending {
			t.Fatalf("canonical parent state/Pending changed for %s", id)
		}
		if r.Status != 201 || !bytes.Equal(canon(bookingSummary(s, id)), canon(r.Reservation)) {
			t.Fatalf("parent occupancy differs for %s", id)
		}
		st := &c.ov.booking
		slot := s.bookingSlots[st.slotID]
		if st.status != "reserved" || slot.reservationID != st.reservationID || slot.caseID != id || slot.version != st.reservationVersion {
			t.Fatalf("slot occupancy/version not restored: %s", id)
		}
		pr205Held(t, s, id)
		for _, kind := range []string{"booking_accept", "booking_choose_slot", "booking_reschedule"} {
			ledger := actRead(t, filepath.Join(run, "api_reviews.ndjson"))
			projection := canon(s.bookingFor(id))
			code, body := coord(t, srv, id, bookingRequest(t, s, id, kind, pr205Alternative(t, s, id), "Held legacy reservation"))
			if code != 409 || body["conflict"] != "readiness" {
				t.Fatalf("%s %s: %d %v", id, kind, code, body)
			}
			if !bytes.Equal(ledger, actRead(t, filepath.Join(run, "api_reviews.ndjson"))) || !bytes.Equal(projection, canon(s.bookingFor(id))) {
				t.Fatal("readiness refusal mutated ledger/projection")
			}
		}
		oldSlot := st.slotID
		requireBooking(t, srv, s, id, "booking_cancel", "", "Release legacy capacity")
		if s.bookingSlots[oldSlot].reservationID != "" {
			t.Fatal("cancel did not release legacy occupancy")
		}
		ev := s.reviews[len(s.reviews)-1]
		if len(ev) != 13 || ev["booking_policy"] != nil {
			t.Fatal("cancel must stay unstamped")
		}
		for _, kind := range []string{"booking_accept", "booking_choose_slot", "booking_reschedule"} {
			ledger := actRead(t, filepath.Join(run, "api_reviews.ndjson"))
			code, body := coord(t, srv, id, bookingRequest(t, s, id, kind, pr205Alternative(t, s, id), "Still held after cancel"))
			if code != 409 || body["conflict"] != "readiness" || !bytes.Equal(ledger, actRead(t, filepath.Join(run, "api_reviews.ndjson"))) {
				t.Fatalf("held after remedy: %d %v", code, body)
			}
		}
	}
	// New ready reservations are stamped for each of the three acquisition kinds.
	for _, kind := range []string{"booking_accept", "booking_reschedule", "booking_choose_slot"} {
		slot := ""
		if kind != "booking_accept" {
			slot = pr205Alternative(t, s, pr205Ready)
		}
		requireBooking(t, srv, s, pr205Ready, kind, slot, "Ready control")
		ev := s.reviews[len(s.reviews)-1]
		if len(ev) != 14 || ev["booking_policy"] != bookingPolicyCurrent || ev["hash"] != apiEventHash(ev) {
			t.Fatalf("unstamped/unhashed new %s: %v", kind, ev)
		}
		if kind == "booking_reschedule" {
			requireBooking(t, srv, s, pr205Ready, "booking_cancel", "", "Test later choose")
		}
	}
	for _, body := range []string{`{"action":"note","text":"Synthetic note after stamp"}`, fmt.Sprintf(`{"action":"snooze","until":%q,"reason":"Synthetic callback"}`, time.Now().UTC().Add(time.Hour).Format(time.RFC3339))} {
		var data map[string]interface{}
		if e := json.Unmarshal([]byte(body), &data); e != nil {
			t.Fatal(e)
		}
		data["expected_head"] = headOf(s, pr205Returned)
		b, _ := json.Marshal(data)
		if code, v := coord(t, srv, pr205Returned, string(b)); code != 201 {
			t.Fatalf("non-acquisition after stamp: %d %v", code, v)
		}
	}
	if code, v := pr205HTTP(t, srv, "pr205-unrelated", "/review", RoleClinician, `{"decision":"adequate","note":"Synthetic evidence without an original source"}`); code != 409 {
		t.Fatalf("source-less evidence after stamp: %d %v", code, v)
	}
	if code, v := pr205HTTP(t, srv, "pr205-unrelated", "/review", RoleClinician, `{"item":"`+item+`","decision":"adequate","note":"Synthetic legacy review unaffected"}`); code != 201 {
		t.Fatalf("review after stamp: %d %v", code, v)
	}
	// Unstamped cancel after a stamp neither requires nor resets the global barrier.
	requireBooking(t, srv, s, pr205Ready, "booking_cancel", "", "Final release after stamp")
	pr205Unchanged(t, run, before)
	reload := loadAPI(run, "")
	if reload.ledgerErr != "" || !bytes.Equal(canon(s.bookingFor(pr205Returned)), canon(reload.bookingFor(pr205Returned))) || !bytes.Equal(canon(s.bookingFor(pr205Ready)), canon(reload.bookingFor(pr205Ready))) {
		t.Fatalf("mixed-history restart: %s", reload.ledgerErr)
	}
	pr205Unchanged(t, run, before)
}

// Every event constructed from here down is SYNTHETIC, never a positive fixture.
func pr205Event(t *testing.T, s *apiServer, id, kind string) map[string]interface{} {
	t.Helper()
	data := map[string]interface{}{}
	if bookingActionName(kind) {
		slot := ""
		if kind == "booking_choose_slot" || kind == "booking_reschedule" {
			slot = pr205Alternative(t, s, id)
		}
		if err := json.Unmarshal([]byte(bookingRequest(t, s, id, kind, slot, "Synthetic forgery control")), &data); err != nil {
			t.Fatal(err)
		}
		delete(data, "action")
		delete(data, "expected_head")
	} else if kind == "note" {
		data["text"] = "Synthetic note"
	} else {
		data["outcome"] = "reached"
		data["channel"] = "phone"
	}
	return map[string]interface{}{"type": coordEventType, "kind": kind, "case": id, "data": data, "role": RoleCoordinator, "actor": "synthetic-forgery-test", "action": ActCoordinateCase, "at": "2026-11-13T10:00:00Z", "head": headOf(s, id), "seq": s.cases[id].ov.n + 1, "mode": "demo", "prev": s.lastHashLocked()}
}
func pr205Line(t *testing.T, ev map[string]interface{}) []byte {
	t.Helper()
	ev["hash"] = apiEventHash(ev)
	b, e := json.Marshal(ev)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func pr205Replay(t *testing.T, s *apiServer, ev map[string]interface{}) {
	t.Helper()
	if err := s.replayLedgerLine(pr205Line(t, ev)); err != nil {
		t.Fatalf("synthetic positive: %v", err)
	}
}
func pr205Fresh(t *testing.T) *apiServer {
	t.Helper()
	run, _ := pr205Run(t, false)
	s := loadAPI(run, "")
	if s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	return s
}

func pr205WritesRefused(t *testing.T, s *apiServer) {
	t.Helper()
	if s.ledgerErr == "" {
		t.Fatal("replay failure did not latch ledgerErr")
	}
	r := httptest.NewRequest("POST", "/api/cases/"+pr205Returned+"/actions", strings.NewReader(`{"action":"note","expected_head":"stale","text":"Synthetic invalid-ledger write"}`))
	r.Header.Set("X-Wasl-Role", RoleCoordinator)
	r.Header.Set("X-Wasl-Actor", "synthetic-failure-test")
	w := httptest.NewRecorder()
	s.caseRoutes(w, r)
	if w.Code != 500 {
		t.Fatalf("invalid-ledger write must be 500 before head validation: %d %s", w.Code, w.Body.String())
	}
}

// Synthetic continuation from the already materialized in-memory prefix: the
// next rejected record must propagate through loadReviews into fail-closed HTTP.
// Full-file startup/restart is separately tested with the authentic fixture.
func pr205LatchFailure(t *testing.T, s *apiServer, rejected []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.run, "api_reviews.ndjson"), append(rejected, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	s.loadReviews()
	pr205WritesRefused(t, s)
}

// T6a-g: key-set and stamp checks run before the readiness waiver. All mutations
// are chain-valid so a semantic failure, not an unrelated bad hash, is required.
func TestPR205SyntheticEnvelopeGuards(t *testing.T) {
	cases := []struct {
		name, kind string
		stamp      bool
		change     func(map[string]interface{})
	}{
		{"extra_legacy", "booking_choose_slot", false, func(e map[string]interface{}) { e["extra"] = true }},
		{"extra_stamped", "booking_choose_slot", true, func(e map[string]interface{}) { e["extra"] = true }},
		{"misplaced_stamp", "booking_choose_slot", false, func(e map[string]interface{}) { e["Booking_policy"] = "readiness/1" }},
		{"stamp_cancel", "booking_cancel", true, nil}, {"stamp_note", "note", true, nil}, {"stamp_contact", "contact_outcome", true, nil},
		{"held_stamped", "booking_choose_slot", true, nil},
	}
	for _, value := range []interface{}{nil, 1, true, []interface{}{}, map[string]interface{}{}, "", "readiness/3", "READINESS/2", "READINESS/1", "readiness/1 ", " readiness/1"} {
		v := value
		cases = append(cases, struct {
			name, kind string
			stamp      bool
			change     func(map[string]interface{})
		}{fmt.Sprintf("value_%T_%v", v, v), "booking_choose_slot", false, func(e map[string]interface{}) { e["booking_policy"] = v }})
	}
	for _, key := range []string{"type", "kind", "case", "data", "role", "actor", "action", "at", "head", "seq", "mode", "prev", "hash"} {
		// Removing type intentionally switches to the unchanged chain-only protocol;
		// G1 only governs coordinator_action. Test that separately, not as a rejection.
		if key == "type" || key == "hash" {
			continue
		}
		k := key
		cases = append(cases, struct {
			name, kind string
			stamp      bool
			change     func(map[string]interface{})
		}{"missing_" + k, "booking_choose_slot", false, func(e map[string]interface{}) { delete(e, k) }})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := pr205Fresh(t)
			e := pr205Event(t, s, pr205Returned, tc.kind)
			if tc.stamp {
				e["booking_policy"] = "readiness/1"
			}
			if tc.change != nil {
				tc.change(e)
			}
			before := len(s.reviews)
			if err := s.replayLedgerLine(pr205Line(t, e)); err == nil {
				t.Fatal("synthetic malformed/held stamped event accepted")
			}
			if len(s.reviews) != before {
				t.Fatal("failed event was applied")
			}
			pr205LatchFailure(t, s, pr205Line(t, e))
		})
	}
	t.Run("duplicate", func(t *testing.T) {
		s := pr205Fresh(t)
		e := pr205Event(t, s, pr205Ready, "booking_accept")
		e["booking_policy"] = "readiness/1"
		b := pr205Line(t, e)
		b = append([]byte(`{"booking_policy":"readiness/1",`), b[1:]...)
		if err := s.replayLedgerLine(b); err == nil {
			t.Fatal("duplicate stamp accepted")
		}
		pr205LatchFailure(t, s, b)
	})
	t.Run("missing_hash", func(t *testing.T) {
		s := pr205Fresh(t)
		e := pr205Event(t, s, pr205Returned, "booking_choose_slot")
		delete(e, "hash")
		raw, _ := json.Marshal(e)
		if s.replayLedgerLine(raw) == nil {
			t.Fatal("missing hash accepted")
		}
		pr205LatchFailure(t, s, raw)
	})
	// G1 does not silently expand the schema of existing untyped review history.
	t.Run("untyped_chain_only_unchanged", func(t *testing.T) {
		s := pr205Fresh(t)
		e := map[string]interface{}{"case": pr205Returned, "extra": "legacy chain-only key", "prev": "genesis"}
		pr205Replay(t, s, e)
	})
}

// T6h: barrier is GLOBAL in ledger order, not per case. Unstamped cancel/note
// after a stamp pass and neither set nor require the flag (P4 corrected wording).
func TestPR205SyntheticStampOrder(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprintf("downgrade_same_case_%v", same), func(t *testing.T) {
			s := pr205Fresh(t)
			pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
			e := pr205Event(t, s, pr205Ready, "booking_accept")
			e["booking_policy"] = "readiness/1"
			pr205Replay(t, s, e)
			pr205Replay(t, s, pr205Event(t, s, pr205Returned, "note"))
			pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_cancel"))
			id, kind := "pr205-unrelated", "booking_choose_slot"
			if same {
				id, kind = pr205Ready, "booking_reschedule"
			}
			e = pr205Event(t, s, id, kind)
			if err := s.replayLedgerLine(pr205Line(t, e)); err == nil || !strings.Contains(err.Error(), "unstamped") {
				t.Fatalf("global suffix downgrade accepted/wrong error: %v", err)
			}
			pr205LatchFailure(t, s, pr205Line(t, e))
		})
	}
	t.Run("note_cancel_do_not_set_barrier", func(t *testing.T) {
		s := pr205Fresh(t)
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "note"))
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_cancel"))
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
	})
	t.Run("failed_stamp_does_not_set_barrier", func(t *testing.T) {
		s := pr205Fresh(t)
		e := pr205Event(t, s, pr205Returned, "booking_choose_slot")
		e["booking_policy"] = "readiness/1"
		if err := s.replayLedgerLine(pr205Line(t, e)); err == nil {
			t.Fatal("held stamped accepted")
		}
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
	})
}

// T9a-f: forged legacy prefix still enforces EVERY other transition and envelope
// guard. The last subtest intentionally succeeds to expose the forgery window.
func TestPR205SyntheticLegacyGuards(t *testing.T) {
	type guard struct {
		name, want string
		change     func(*apiServer, map[string]interface{})
	}
	controls := []guard{
		{"consent", "permission", func(s *apiServer, e map[string]interface{}) {
			s.cases[pr205Returned].Consent = "denied"
			s.cases[pr205Returned].last.Contact = "denied"
		}},
		{"suggestion", "suggestion", func(s *apiServer, e map[string]interface{}) {
			e["data"].(map[string]interface{})["expected_suggestion_version"] = strings.Repeat("a", 64)
		}},
		{"slot_version", "slot", func(s *apiServer, e map[string]interface{}) {
			e["data"].(map[string]interface{})["expected_slot_version"] = strings.Repeat("a", 64)
		}},
		{"noncandidate", "slot", func(s *apiServer, e map[string]interface{}) {
			e["data"].(map[string]interface{})["slot_id"] = "sim:colposcopy:999:1"
		}},
		{"occupied", "slot", func(s *apiServer, e map[string]interface{}) {
			id := e["data"].(map[string]interface{})["slot_id"].(string)
			s.bookingSlots[id].reservationID = "other-reservation"
		}},
		{"closed", "open", func(s *apiServer, e map[string]interface{}) { s.cases[pr205Returned].Closed = true }},
		{"nonopen", "open", func(s *apiServer, e map[string]interface{}) { s.cases[pr205Returned].State = "closed" }},
		{"recovery", "recovery", func(s *apiServer, e map[string]interface{}) { s.recovery.ByCase[pr205Returned] = &recoveryFill{} }},
		{"role", "role", func(s *apiServer, e map[string]interface{}) { e["role"] = RoleAuditor }},
		{"actor", "actor", func(s *apiServer, e map[string]interface{}) { e["actor"] = "" }},
		{"timestamp", "timestamp", func(s *apiServer, e map[string]interface{}) { e["at"] = "yesterday" }},
		{"head", "head", func(s *apiServer, e map[string]interface{}) { e["head"] = strings.Repeat("a", 64) }},
		{"empty_sig_vs_signed_head", "head", func(s *apiServer, e map[string]interface{}) { s.cases[pr205Returned].last.Sig = "" }},
		{"seq", "sequence", func(s *apiServer, e map[string]interface{}) { e["seq"] = 1.5 }},
		{"mode", "mode", func(s *apiServer, e map[string]interface{}) { e["mode"] = "live" }},
		{"data", "canonical", func(s *apiServer, e map[string]interface{}) {
			e["data"].(map[string]interface{})["reason"] = " padded "
		}},
	}
	for _, tc := range controls {
		t.Run(tc.name, func(t *testing.T) {
			s := pr205Fresh(t)
			e := pr205Event(t, s, pr205Returned, "booking_choose_slot")
			tc.change(s, e)
			before := len(s.reviews)
			err := s.replayLedgerLine(pr205Line(t, e))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("guard %s not independently enforced: %v", tc.name, err)
			}
			if len(s.reviews) != before {
				t.Fatal("rejected forgery applied")
			}
			pr205LatchFailure(t, s, pr205Line(t, e))
		})
	}
	t.Run("broken_reschedule_slot_chain", func(t *testing.T) {
		s := pr205Fresh(t)
		pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
		slot := s.bookingSlots[s.cases[pr205Returned].ov.booking.slotID]
		slot.caseID = "wrong-case"
		e := pr205Event(t, s, pr205Returned, "booking_reschedule")
		if err := s.replayLedgerLine(pr205Line(t, e)); err == nil || !strings.Contains(err.Error(), "inconsistent") {
			t.Fatalf("broken old slot chain: %v", err)
		}
		pr205LatchFailure(t, s, pr205Line(t, e))
	})
	t.Run("fresh_and_old_prefix_forgery_boundary", func(t *testing.T) {
		for _, old := range []bool{false, true} {
			s := pr205Fresh(t)
			if old {
				pr205Replay(t, s, pr205Event(t, s, pr205Ready, "note"))
			}
			pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
			if s.cases[pr205Returned].ov.booking.status != "reserved" {
				t.Fatal("documented readiness-only waiver changed")
			}
			pr205Held(t, s, pr205Returned)
		}
	})
}

// T5-synthetic: a separate in-memory canonical change pins the Pending predicate.
// F2 is independently proven by the authentic fixture, never by this test.
func TestPR205SyntheticPendingBoundary(t *testing.T) {
	s := pr205Fresh(t)
	c := s.cases[pr205Returned]
	c.State = "inadequate"
	c.last.State = "inadequate"
	c.last.Pending = []string{"synthetic-confirm"}
	pr205Replay(t, s, pr205Event(t, s, pr205Returned, "booking_choose_slot"))
	pr205Held(t, s, pr205Returned)
}

// P1: the readiness gate belongs under the lock after ledgerErr and expected_head.
func TestPR205LiveGatePrecedence(t *testing.T) {
	run, _ := pr205Run(t, false)
	s, srv := serveAgent(t, run)
	body := bookingRequest(t, s, pr205Returned, "booking_accept", "", "")
	s.ledgerErr = "synthetic invalid ledger"
	if code, _ := coord(t, srv, pr205Returned, body); code != 500 {
		t.Fatalf("ledgerErr precedence %d", code)
	}
	s.ledgerErr = ""
	var req map[string]interface{}
	json.Unmarshal([]byte(body), &req)
	req["expected_head"] = "stale"
	b, _ := json.Marshal(req)
	if code, v := coord(t, srv, pr205Returned, string(b)); code != 409 || v["conflict"] != "head" {
		t.Fatalf("head precedence %d %v", code, v)
	}
	if code, v := coord(t, srv, pr205Returned, body); code != 409 || v["conflict"] != "readiness" {
		t.Fatalf("readiness precedence %d %v", code, v)
	}
	// The policy stamp is server-owned, not an additional accepted request field.
	req["expected_head"] = headOf(s, pr205Returned)
	req["booking_policy"] = "readiness/1"
	b, _ = json.Marshal(req)
	if code, _ := coord(t, srv, pr205Returned, string(b)); code != 400 {
		t.Fatalf("client envelope injection %d", code)
	}
	if len(s.reviews) != 0 {
		t.Fatal("refusal appended")
	}
}

// T6i: byte mutation/truncation is not the deliberately permitted chain forgery.
func TestPR205AuthenticHistoryMutationFailsClosed(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		t.Run(fmt.Sprint(truncated), func(t *testing.T) {
			run, _ := pr205Run(t, true)
			p := filepath.Join(run, "api_reviews.ndjson")
			b := actRead(t, p)
			if truncated {
				b = b[:len(b)-1]
			} else {
				b = bytes.Replace(b, []byte("synthetic-parent-fixture"), []byte("synthetic-parent-fixturE"), 1)
			}
			if err := os.WriteFile(p, b, 0600); err != nil {
				t.Fatal(err)
			}
			s := loadAPI(run, "")
			if s.ledgerErr == "" {
				t.Fatal("tampered/truncated history replayed")
			}
			pr205WritesRefused(t, s)
		})
	}
}

// A6 source-only synthetic negatives: the original parent ledger copy remains
// byte-identical, while canonical consent or catalogue bytes alone are changed.
func TestPR205SyntheticSourceChangeFailsClosed(t *testing.T) {
	for _, source := range []string{"consent", "catalogue"} {
		t.Run(source, func(t *testing.T) {
			run, _ := pr205Run(t, true)
			ledger := actRead(t, filepath.Join(run, "api_reviews.ndjson"))
			if source == "consent" {
				p := filepath.Join(run, "out_log.ndjson")
				lines := bytes.Split(bytes.TrimSuffix(actRead(t, p), []byte{'\n'}), []byte{'\n'})
				changed := false
				for i, line := range lines {
					var msg LogMsg
					if err := json.Unmarshal(line, &msg); err != nil {
						t.Fatal(err)
					}
					var env OutEnvelope
					if err := json.Unmarshal(msg.Data, &env); err != nil {
						t.Fatal(err)
					}
					if env.Dec != nil && env.Dec.LoopID == pr205Returned {
						env.Dec.Contact = "denied" // intentionally synthetic; do not re-sign
						msg.Data, _ = json.Marshal(env)
						lines[i], _ = json.Marshal(msg)
						changed = true
					}
				}
				if !changed {
					t.Fatal("negative control changed no decision")
				}
				if err := os.WriteFile(p, append(bytes.Join(lines, []byte{'\n'}), '\n'), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				p := filepath.Join(run, "booking.json")
				var doc map[string]interface{}
				if err := json.Unmarshal(actRead(t, p), &doc); err != nil {
					t.Fatal(err)
				}
				doc["cases"].(map[string]interface{})[pr205Returned].(map[string]interface{})["suggestion_version"] = strings.Repeat("a", 64)
				b, _ := json.Marshal(doc)
				if err := os.WriteFile(p, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := loadAPI(run, "")
			pr205WritesRefused(t, s)
			if !bytes.Equal(ledger, actRead(t, filepath.Join(run, "api_reviews.ndjson"))) {
				t.Fatal("source-only negative rewrote the ledger")
			}
		})
	}
}

// T8: coordinator stamp is compatible with the existing redacted audit/export
// projection and undo semantic preflight; no schema changes in those protocols.
func TestPR205HistoryProjections(t *testing.T) {
	run, _ := pr205Run(t, true)
	s, srv := serveAgent(t, run)
	if s.ledgerErr != "" {
		t.Fatal(s.ledgerErr)
	}
	requireBooking(t, srv, s, pr205Ready, "booking_accept", "", "")
	if err := validateLedgerSemantics(s.reviews); err != nil {
		t.Fatalf("undo preflight: %v", err)
	}
	h := exportRoutes(s)
	var audit map[string]interface{}
	for _, suffix := range []string{"/audit", "/export"} {
		code, b := fetchBody(t, h, "GET", "/api/cases/"+pr205Ready+suffix)
		if code != 200 {
			t.Fatalf("%s: %d %s", suffix, code, b)
		}
		v := decodeMap(t, b)
		if suffix == "/audit" {
			audit = v
		}
		if bytes.Contains(b, []byte("booking_policy")) {
			t.Fatalf("%s leaked new envelope key outside allowlist", suffix)
		}
	}
	hist := audit["history"].([]interface{})
	if len(hist) != 1 || hist[0].(map[string]interface{})["kind"] != "booking_accept" {
		t.Fatalf("audit omitted stamped acquisition: %v", audit)
	}
	reload := loadAPI(run, "")
	if reload.ledgerErr != "" || !reflect.DeepEqual(historyKeys(t, audit), historyKeys(t, func() map[string]interface{} {
		_, b := fetchBody(t, exportRoutes(reload), "GET", "/api/cases/"+pr205Ready+"/audit")
		return decodeMap(t, b)
	}())) {
		t.Fatal("audit contract changed across restart")
	}
}
