package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var writeReferralHandoffs = flag.Bool("write-referral-handoffs", false, "regenerate the synthetic lung and DR returned-referral controls")

// Explicitly synthetic lung work-up and DR eye-clinic referrals that the single receiving unit returned. They start at
// the canonical facts seam (not ingest or the reader); the packets, signatures and API projections are produced by
// unchanged canonical code and served by the authenticated read handler. Not medical evidence.
type referralHandoffControl struct {
	facts  *LoopFacts
	status string
}

func referralHandoffControls() []referralHandoffControl {
	lung := func(n int, rejected float64, reason string, more ...DownFact) *LoopFacts {
		f := lungFacts(60, "none", "high", big, append([]DownFact{extIntake("mafraq-rad", "MAF-intake-"+itoa(n), rejected, "revoked", reason, "eid", 1, "lung_nodule")}, more...)...)
		f.LoopID, f.SRDay, f.Contact = "rad-imaging/ServiceRequest/RAD-sr-"+itoa(n), rejected-20, "granted"
		for i := range f.Down {
			f.Down[i].Day = max(f.Down[i].Day, f.SRDay)
		}
		return f
	}
	dr := func(n int, rejected float64, grade, reason string, more ...DownFact) *LoopFacts {
		screen := "PHC-dr-" + itoa(n)
		f := drFacts(append([]DownFact{drScreen(screen, rejected-20, grade), extIntake("eye-hosp", "EYE-intake-"+itoa(n), rejected, "revoked", reason, "eid", 1, "diabetic_retinopathy")}, more...)...)
		f.LoopID, f.SRDay, f.DR, f.Contact = "phc-retina/ServiceRequest/PHC-sr-"+itoa(n), rejected-20, &DRSR{Screen: screen}, "granted"
		return f
	}
	return []referralHandoffControl{
		{lung(9101, 119, "out-of-network"), "awaiting_referrer"},
		{dr(9201, 119, "R3AM0", "incomplete-referral"), "awaiting_referrer"},
		{dr(9202, 115, "R3AM0", "capacity"), "coordinator"},
		{dr(9203, 100, "R3AM0", "incomplete-referral"), "medical_director"},
		{dr(9204, 117, "R2M0", "incomplete-referral"), "awaiting_referrer"},
		{dr(9205, 100, "R3AM0", "capacity", extIntake("eye-hosp", "EYE-intake-9205b", 104, "active", "", "eid", 1, "diabetic_retinopathy")), "referrer_acted"},
	}
}

func TestReferralHandoffCanonicalControls(t *testing.T) {
	oldHandoff, oldOutreach, oldCal := Handoff, OutreachOn, V7Cal
	Handoff, OutreachOn, V7Cal = true, false, nil
	defer func() { Handoff, OutreachOn, V7Cal = oldHandoff, oldOutreach, oldCal }()
	controls := referralHandoffControls()
	c := NewClosureCore(0, nil)
	var raw bytes.Buffer
	prev := ""
	emit := func(outs []OutMsg) {
		for _, out := range outs {
			if out.Dec == nil {
				continue
			}
			if out.Dec.Sig == "" || out.Dec.Sig != decSig(out.Dec) || out.Prev != prev {
				t.Fatal("noncanonical decision or ledger")
			}
			prev = out.Hash
			env := OutEnvelope{Kind: "dec", OSeq: out.OSeq, Dec: out.Dec, Prev: out.Prev, Hash: out.Hash, Proc: "closure-0"}
			raw.Write(canon(LogMsg{Seq: out.OSeq, Subject: "out.p0", Data: canon(env)}))
			raw.WriteByte('\n')
		}
	}
	for i, ctl := range controls {
		emit(c.Apply(&FactsMsg{ID: "synthetic-handoff-" + itoa(i), InSeq: uint64(i + 1), Facts: ctl.facts}))
	}
	emit(c.Apply(&FactsMsg{ID: "synthetic-clock", InSeq: uint64(len(controls) + 1), IsTick: true, TickDay: 120}))
	run := t.TempDir()
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	handler, err := loadAPI(run, "").handler("referral-handoff-test-token:reviewer:synthetic-referral-reviewer", "")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) map[string]interface{} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer referral-handoff-test-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	var served []interface{}
	for _, ctl := range controls {
		id := ctl.facts.LoopID
		detail := get("/api/cases/" + id)
		current := detail["current"].(map[string]interface{})
		packet, _ := current["handoff"].(map[string]interface{})
		if packet == nil || packet["status"] != ctl.status || packet["suggested_alternative"] != "" {
			t.Fatalf("%s: want a %s packet with no alternative unit, got %v", id, ctl.status, packet)
		}
		served = append(served, map[string]interface{}{"case": detail["case"], "current": current})
	}
	// The case list carries a free-text-free marker of each packet, so an inbox needs no detail read for other cases.
	listed := 0
	for _, prog := range []string{"lung_nodule", "diabetic_retinopathy"} {
		for _, x := range get("/api/cases?programme=" + prog)["cases"].([]interface{}) {
			sum := x.(map[string]interface{})
			marker, _ := sum["handoff"].(map[string]interface{})
			for _, ctl := range controls {
				if ctl.facts.LoopID != sum["id"] {
					continue
				}
				listed++
				if marker == nil || marker["status"] != ctl.status || marker["referral_id"] == "" || marker["advice_to_referrer"] != nil || marker["info_requested"] != nil {
					t.Fatalf("%s: list marker %v", sum["id"], marker)
				}
			}
		}
	}
	if listed != len(controls) {
		t.Fatalf("list served %d of %d returned controls", listed, len(controls))
	}
	encoded, err := json.MarshalIndent(served, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("..", "web", "data", "referral-handoff-synthetic.json")
	if *writeReferralHandoffs {
		if err := os.WriteFile(path, encoded, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got, want interface{}
	if json.Unmarshal(saved, &got) != nil || json.Unmarshal(encoded, &want) != nil || !bytes.Equal(canon(got), canon(want)) {
		t.Fatal("referral-handoff-synthetic.json is stale; regenerate with -write-referral-handoffs")
	}
}
