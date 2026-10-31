package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// C10: rejected-referral hand-off for the lung work-up (PET/CT or tissue) and the DR eye-clinic referral. These loops
// have no e-consult thread, so the receiving unit's rejection on a trusted link is final, and the packet reuses the CRC
// packet logic (reason code, advice, act-by clock, escalation, referrer action).

func extIntake(prov, id string, day float64, status, reason, basis string, p float64, prog string) DownFact {
	return DownFact{EventID: id, Provider: prov, RType: "ServiceRequest", Status: status, Day: day, Reason: reason, Prog: prog,
		Strength: p, Weak: Edge{A: "ref|M1", B: prov + "|M1", P: p, Basis: basis}, Hops: 1}
}

func TestBuildHandoffDirect(t *testing.T) {
	f := &LoopFacts{LoopID: "phc-retina/ServiceRequest/PHC-sr-1", Provider: "phc-retina", SRDay: 10}
	srs := []srFact{{day: 30, status: "rejected", prov: "eye-hosp", reason: "capacity", ev: "EYE-intake-1"}}
	p := buildHandoffDirect(f, "diabetic_retinopathy", srs, true, 31, "granted")
	if p == nil || p.Decision != "reject" || p.DecidedAt != 30 || p.ReasonCode != "service_not_available" || p.DecidedBy != "eye-hosp" ||
		p.ReferralID != "EYE-intake-1" || p.Priority != "urgent" || p.ActBy != addBD(30, HandoffBDUrgent) || p.Status != "awaiting_referrer" {
		t.Fatalf("DR packet %+v", p)
	}
	if p.Alternative != "" || p.Advice != adviceNoAlternative["service_not_available"] || !strings.Contains(p.Recommendation, "eye") || p.Consent != "granted" {
		t.Fatalf("single-unit programme: no other unit to suggest, %+v", p)
	}
	if !handoffComplete(p, "diabetic_retinopathy") || handoffComplete(p, "crc") {
		t.Fatal("required fields: a single-unit programme needs no alternative, CRC does")
	}
	// routine: 5 business days; the referrer re-refers to the same unit (the only one), which counts as acting
	srs2 := append(srs, srFact{day: 37, status: "accepted", prov: "eye-hosp", ev: "EYE-intake-2"})
	p = buildHandoffDirect(f, "diabetic_retinopathy", srs2, false, 40, "")
	if p.ActBy != addBD(30, HandoffBDRoutine) || p.ActedAt != 37 || p.Status != "referrer_acted" {
		t.Fatalf("re-referral to the same unit: %+v", p)
	}
	// a structured hand-off reply is also an action
	f.Comms = []CommFact{{ID: "c1", Day: 33, Category: "handoff-reply", Action: "re_referred"}}
	if p = buildHandoffDirect(f, "lung_nodule", srs, true, 40, ""); p.ActedAt != 33 || !strings.Contains(p.Recommendation, "PET") {
		t.Fatalf("hand-off reply: %+v", p)
	}
	// no rejection, or a rejection older than this loop: no packet
	if buildHandoffDirect(f, "diabetic_retinopathy", []srFact{{day: 30, status: "accepted", prov: "eye-hosp", ev: "x"}}, true, 40, "") != nil {
		t.Fatal("packet for an accepted referral")
	}
	if buildHandoffDirect(f, "diabetic_retinopathy", []srFact{{day: 2, status: "rejected", prov: "eye-hosp", ev: "x"}}, true, 40, "") != nil {
		t.Fatal("packet for a rejection before the loop opened")
	}
}

const drLoopID = "phc-retina/ServiceRequest/PHC-sr-1"

func TestDRRejectedReferralPacket(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	rej := extIntake("eye-hosp", "EYE-intake-1", 30, "revoked", "incomplete-referral", "eid", 1, "diabetic_retinopathy")
	d, _ := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej), 31)
	if d.Handoff == nil || d.Handoff.Decision != "reject" || d.Handoff.ReasonCode != "needs_more_info" || len(d.Handoff.InfoRequested) != 3 ||
		d.Handoff.Priority != "urgent" || d.State != "open" || d.RefStatus != "rejected" || !hasEsc(d, "referral_rejected") {
		t.Fatalf("DR rejection: state %s ref %s esc %v packet %+v", d.State, d.RefStatus, d.Escalations, d.Handoff)
	}
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej), 60)
	if d.Handoff.Status != "medical_director" || !hasEsc(d, "handoff_medical_director") || !hasEsc(d, "handoff_urgent_14d") {
		t.Fatalf("no action by day 60: %s %v", d.Handoff.Status, d.Escalations)
	}
	// routine grade: routine clock
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R2M0"), rej), 31)
	if d.Handoff.Priority != "routine" || d.Handoff.ActBy != addBD(30, HandoffBDRoutine) {
		t.Fatalf("routine DR packet %+v", d.Handoff)
	}
	// re-referral accepted, then a trusted examined note: closed, and the packet records the action
	acc := extIntake("eye-hosp", "EYE-intake-2", 36, "active", "", "eid", 1, "diabetic_retinopathy")
	d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej, acc, drNote("EYE-dr-1", 50, "examined_treated", "eid", 1, false)), 60)
	if d.State != "closed" || d.Handoff == nil || d.Handoff.ActedAt != 36 || d.Handoff.Status != "referrer_acted" || hasEsc(d, "referral_rejected") {
		t.Fatalf("re-referred and examined: %s %+v %v", d.State, d.Handoff, d.Escalations)
	}
	// an untrusted rejection is asked about, never attached
	d, out := runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), extIntake("eye-hosp", "EYE-intake-9", 30, "revoked", "capacity", "wasl", 0.8, "diabetic_retinopathy")), 31)
	if d.Handoff != nil || !hasReview(out, "link") || d.RefStatus != "none" {
		t.Fatalf("untrusted rejection attached: %+v", d.Handoff)
	}
	// a non-referable grade never makes a packet, and the switch turns it off
	if d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R1M0"), rej), 31); d.Handoff != nil {
		t.Fatal("packet on a no-referral loop")
	}
	Handoff = false
	if d, _ = runLung(t, drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej), 31); d.Handoff != nil {
		t.Fatal("packet with -handoff=false")
	}
}

func TestLungWorkupRejectedReferralPacket(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	rej := extIntake("mafraq-rad", "MAF-intake-1", 10, "revoked", "out-of-network", "eid", 1, "lung_nodule")
	d, _ := runLung(t, lungFacts(60, "none", "high", big, rej), 11)
	if d.Handoff == nil || d.Handoff.ReasonCode != "wrong_provider" || d.Handoff.Priority != "urgent" || d.Handoff.DecidedBy != "mafraq-rad" ||
		d.State != "open" || !hasEsc(d, "referral_rejected") {
		t.Fatalf("lung work-up rejection: %s %v %+v", d.State, d.Escalations, d.Handoff)
	}
	acc := extIntake("mafraq-rad", "MAF-intake-2", 14, "active", "", "eid", 1, "lung_nodule")
	d, _ = runLung(t, lungFacts(60, "none", "high", big, rej, acc, lungFU("MAF-pet-1", 20, "pet_low", "eid", 1, false)), 30)
	if d.State != "closed" || d.Handoff == nil || d.Handoff.ActedAt != 14 {
		t.Fatalf("re-referred, PET low: %s %+v", d.State, d.Handoff)
	}
	// another programme's intake (a CRC referral of the same person) is not this loop's rejection
	crc := extIntake("noor-endo", "NOO-intake-1", 10, "revoked", "capacity", "eid", 1, "crc")
	if d, _ = runLung(t, lungFacts(60, "none", "high", big, crc), 11); d.Handoff != nil {
		t.Fatal("a CRC rejection made a lung packet")
	}
}

func TestHandoffExtMetrics(t *testing.T) {
	rec, _ := drRule("R3AM0")
	ho := func(dec float64, act float64) *HandoffTruth {
		return &HandoffTruth{Decided: dec, ActBy: dec + 2, ActDay: act, Urgent: true}
	}
	pk := func(acted float64, st string) *HandoffPacket {
		return &HandoffPacket{ReferralID: "x", ReasonCode: "other", Advice: "a", Responsible: "r", DecidedBy: "eye-hosp", Consent: "granted", ActedAt: acted, Status: st}
	}
	dr := func(st string, h *HandoffTruth) *LoopTruth {
		return &LoopTruth{Prog: "diabetic_retinopathy", State: st, Handoff: h, DR: &DRTruth{Rec: rec, Screens: []string{"S"}}}
	}
	tr := &Truth{Loops: map[string]*LoopTruth{"A": dr("closed", ho(30, 32)), "B": dr("open", ho(30, -1)), "C": dr("open", nil), "D": dr("open", ho(30, 31))}}
	final := map[string]*Decision{
		"A": {State: "closed", Handoff: pk(32, "referrer_acted"), DR: &DRDecision{Rec: &rec}},
		"B": {State: "open", DR: &DRDecision{Rec: &rec}},
		"C": {State: "open", Handoff: pk(-1, "coordinator"), DR: &DRDecision{Rec: &rec}},
		"D": {State: "closed", Handoff: pk(31, "referrer_acted"), DR: &DRDecision{Rec: &rec}},
	}
	h := drMetrics(final, tr)["handoff"].(map[string]interface{})["counts"].(map[string]int)
	want := map[string]int{"truth_packets": 3, "packets_matched": 2, "packets_missed": 1, "packets_pipeline_only": 1, "closed": 2, "false_closures": 1,
		"truth_closed_after_handoff": 1, "acted_state_correct": 2, "packets_all_required_fields": 3, "truth_referrer_acted": 2}
	for k, v := range want {
		if h[k] != v {
			t.Errorf("%s = %d, want %d (all %v)", k, h[k], v, h)
		}
	}
	// lung: same counters under lung_nodule
	lt := &LoopTruth{Prog: "lung_nodule", State: "open", Handoff: ho(10, -1), Lung: &LungTruth{InScope: true}}
	lm := lungMetrics(map[string]*Decision{"L": {State: "open", Handoff: pk(-1, "coordinator"), Lung: &LungDecision{}}}, &Truth{Loops: map[string]*LoopTruth{"L": lt}})
	if c := lm["handoff"].(map[string]interface{})["counts"].(map[string]int); c["packets_matched"] != 1 || c["truth_packets"] != 1 {
		t.Fatalf("lung hand-off counts %v", c)
	}
	// no packets at all: no hand-off block (reports for runs without the switch are unchanged)
	if _, ok := drMetrics(map[string]*Decision{"C": {State: "open", DR: &DRDecision{Rec: &rec}}}, &Truth{Loops: map[string]*LoopTruth{"C": dr("open", nil)}})["handoff"]; ok {
		t.Fatal("hand-off block without packets")
	}
}

// The generator switch adds rejections only to lung work-up and DR loops that never reached the procedure, on their own
// random streams: every other loop's truth and every event of the cohort without the switch are unchanged.
func TestGenRefRejectOnlyRejectedLoopsDiffer(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedL, savedD, savedR, savedH := append([]ProvCfg{}, Providers...), LungOn, DROn, RefRejectExt, Handoff
	defer func() { Providers, LungOn, DROn, RefRejectExt, Handoff = savedP, savedL, savedD, savedR, savedH }()
	EnableLung()
	EnableDR()
	Handoff = true
	gen := func(on bool) string {
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		RefRejectExt = on
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 5000, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 600, "", 1200, "")
		return out
	}
	a, b := gen(false), gen(true)
	readTruth := func(dir string) *Truth {
		var tr Truth
		bb, _ := os.ReadFile(filepath.Join(dir, "truth.json"))
		json.Unmarshal(bb, &tr)
		return &tr
	}
	ta, tb := readTruth(a), readTruth(b)
	rejected, srOf, qidOf := map[string]bool{}, map[string]string{}, map[string]bool{}
	nProg := map[string]int{}
	for id, lt := range tb.Loops {
		x, _ := json.Marshal(lt)
		y, _ := json.Marshal(ta.Loops[id])
		if bytes.Equal(x, y) {
			continue
		}
		if lt.Handoff == nil || (lt.Prog != "lung_nodule" && lt.Prog != "diabetic_retinopathy") {
			t.Fatalf("loop %s (%s) changed without a rejection", id, lt.Prog)
		}
		if old := ta.Loops[id]; old.State != "open" || old.Handoff != nil {
			t.Fatalf("rejection on a loop that reached the procedure: %s %s", id, old.State)
		}
		rejected[id], srOf["ServiceRequest/"+id[strings.LastIndex(id, "/")+1:]] = true, id
		qidOf[lt.Qid] = true
		nProg[lt.Prog]++
	}
	t.Logf("rejected loops by programme: %v", nProg)
	if nProg["lung_nodule"] == 0 || nProg["diabetic_retinopathy"] == 0 {
		t.Fatalf("no rejections generated: %v", nProg)
	}
	for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging", "mafraq-rad", "phc-retina", "eye-hosp"} {
		la, lb := lines(t, filepath.Join(a, "fhir_"+p+".ndjson")), lines(t, filepath.Join(b, "fhir_"+p+".ndjson"))
		i := 0
		for _, l := range lb {
			if i < len(la) && l == la[i] {
				i++
				continue
			}
			var ev GenEvent
			json.Unmarshal([]byte(l), &ev)
			ref := ""
			if bs, ok := ev.Res["basedOn"].([]interface{}); ok && len(bs) > 0 {
				m := bs[0].(map[string]interface{})
				ref = strings.TrimPrefix(gs(m, "display"), xrefPrefix)
				if r := gs(m, "reference"); r != "" {
					ref = srOf[r]
				}
			}
			pk := ""
			if c, ok := ev.Res["contained"].([]interface{}); ok {
				for _, idv := range c[0].(map[string]interface{})["identifier"].([]interface{}) {
					if id := idv.(map[string]interface{}); strings.HasSuffix(gs(id, "system"), ":mrn") {
						pk = p + "|" + gs(id, "value")
					}
				}
			}
			if !rejected[ref] && !qidOf[tb.PKQid[pk]] {
				t.Fatalf("%s: added event not of a rejected loop: %s", p, l[:200])
			}
		}
		if i != len(la) {
			t.Fatalf("%s: %d events of the cohort without the switch are missing or changed", p, len(la)-i)
		}
	}
	for id := range rejected {
		lt := tb.Loops[id]
		if lt.RefStatus == "none" || lt.Handoff.Intake == "" || lt.Handoff.Unit == "" || lt.Handoff.ActDay > TEnd {
			t.Fatalf("rejected loop truth incomplete: %+v %+v", lt, lt.Handoff)
		}
	}
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}
