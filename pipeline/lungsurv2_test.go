package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// -lungsurv2 (later PET/CT surveillance CTs) and -lungsurvstable (review discipline, truth and metrics for the stable-CT
// surveillance child), lungsurv.go.

// runSurvFlags: one lung loop through the closure engine with the given surveillance flags; returns every final decision.
func runSurvFlags(t *testing.T, pet, pet2, stable bool, f *LoopFacts, now float64) (map[string]*Decision, []OutMsg) {
	t.Helper()
	s1, s2, s3, savedCal := LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal
	LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal = pet, pet2, stable, nil
	defer func() { LungPETSurv, LungPETSurv2, LungStableSurv, V7Cal = s1, s2, s3, savedCal }()
	c := NewClosureCore(0, nil)
	out := c.Apply(&FactsMsg{ID: "f1", InSeq: 1, Facts: f})
	out = append(out, c.Apply(&FactsMsg{ID: "tick", InSeq: 2, IsTick: true, TickDay: now})...)
	return c.Final(), out
}

func pendingItem(out []OutMsg, loop, kind string) bool {
	for _, o := range out {
		if o.Item != nil && o.Item.LoopID == loop && o.Item.Kinds[0] == kind {
			return true
		}
	}
	return false
}

// A stable first surveillance CT opens the 1-year child, a stable 1-year CT the 2-year child; windows count from the PET/CT.
func TestPETSurv2RoundsChain(t *testing.T) {
	ct1 := lungFU("MAF-ct-7", petDay+80, "stable", "eid", 1, false)
	ct2 := lungFU("MAF-ct-8", petDay+330, "stable", "eid", 1, false)
	f := lungFacts(60, "none", "high", big, petLow(), ct1, ct2)
	fin, _ := runSurvFlags(t, true, true, false, f, petDay+500)
	p, s1, s2, s3 := fin[f.LoopID], fin[f.LoopID+"#surv"], fin[f.LoopID+"#surv2"], fin[f.LoopID+"#surv3"]
	if s1 == nil || s1.State != "closed" || s2 == nil || s2.State != "closed" || s2.ClosingReport != "MAF-ct-8" || s3 == nil || s3.State != "open" {
		t.Fatalf("rounds: %+v %+v %+v", s1, s2, s3)
	}
	if s2.Lung.WindowStart != petDay+300 || s2.Lung.WindowEnd != petDay+420 || s3.Lung.WindowStart != petDay+670 || s3.Lung.WindowEnd != petDay+790 ||
		s2.SRDay != petDay || s3.Overdue || s3.Lung.Surveillance.State != "due" || s2.Lung.Surveillance.Round != 2 || s3.Lung.Surveillance.Round != 3 {
		t.Fatalf("windows: %+v / %+v", s2.Lung, s3.Lung)
	}
	if s2.Chain[0].Body["parent"] != s1.LoopID || s2.Chain[0].Body["parent_head"] != s1.Chain[len(s1.Chain)-1].Hash ||
		s3.Chain[0].Body["parent"] != s2.LoopID || !verifyChain(s2.Chain) || !verifyChain(s3.Chain) {
		t.Fatalf("hand-off provenance: %v / %v", s2.Chain[0].Body, s3.Chain[0].Body)
	}
	if sv := p.Lung.Surveillance; sv == nil || sv.Next == nil || sv.Next.Loop != s2.LoopID || sv.Next.Next == nil || sv.Next.Next.Loop != s3.LoopID {
		t.Fatalf("parent case object must show the chain: %+v", sv)
	}
	for _, d := range []*Decision{p, s1, s2, s3} { // decisions re-verify after a JSON round trip (audit, tamper)
		b, _ := json.Marshal(d)
		var x Decision
		json.Unmarshal(b, &x)
		sig := x.Sig
		x.Sig, x.ID, x.Now, x.InSeq = "", "", 0, 0
		if sha(canon(&x)) != sig {
			t.Fatalf("%s does not re-verify", d.LoopID)
		}
	}
	// resolved on the first CT: discharged, no later rounds; growth escalates and stops the chain
	for _, lab := range []string{"resolved", "growing"} {
		g := lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", petDay+80, lab, "eid", 1, false))
		fin, _ := runSurvFlags(t, true, true, false, g, petDay+500)
		if fin[g.LoopID+"#surv2"] != nil || fin[g.LoopID].Lung.Surveillance.Next != nil {
			t.Fatalf("%s: later round opened", lab)
		}
	}
	// the 1-year CT never seen: overdue with a recall in the parent's name
	g := lungFacts(60, "none", "high", big, petLow(), ct1)
	g.Contact = "granted"
	fin, _ = runSurvFlags(t, true, true, false, g, petDay+430)
	if s := fin[g.LoopID+"#surv2"]; s == nil || !s.Overdue || !hasEsc(s, "overdue_lung_surveillance_ct") || len(s.Outreach) != 1 ||
		s.Outreach[0].Kind != "lung_surveillance_recall" || s.Outreach[0].Event != g.LoopID || !s.Outreach[0].Released {
		t.Fatalf("overdue 1-year CT: %+v", s)
	}
}

// The later rounds keep the gates: an untrusted link or a reader disagreement never closes them and raises a review item
// on the parent loop that the child lists as pending.
func TestPETSurv2NeedsTrustedLinkAndAgreement(t *testing.T) {
	ct1 := lungFU("MAF-ct-7", petDay+80, "stable", "eid", 1, false)
	for name, c := range map[string]struct {
		fu   DownFact
		kind string
	}{
		"untrusted link": {lungFU("MAF-ct-9", petDay+330, "stable", "wasl", 0.8, false), "link"},
		"disagreement":   {lungFU("MAF-ct-9", petDay+330, "", "eid", 1, false), "evidence"},
	} {
		f := lungFacts(60, "none", "high", big, petLow(), ct1, c.fu)
		fin, out := runSurvFlags(t, true, true, false, f, petDay+340)
		s2 := fin[f.LoopID+"#surv2"]
		if s2 == nil || s2.State == "closed" || fin[f.LoopID+"#surv3"] != nil || !pendingItem(out, f.LoopID, c.kind) || len(s2.Pending) == 0 {
			t.Fatalf("%s: %+v", name, s2)
		}
	}
	f := lungFacts(60, "none", "high", big, petLow(), ct1, lungFU("MAF-ct-9", petDay+330, "", "eid", 1, false))
	f.EvOver = map[string]string{"MAF-ct-9": "stable"}
	if fin, _ := runSurvFlags(t, true, true, false, f, petDay+340); fin[f.LoopID+"#surv2"].State != "closed" || !fin[f.LoopID+"#surv2"].HumanAssisted {
		t.Fatalf("clinician sign-off should close the 1-year round: %+v", fin[f.LoopID+"#surv2"])
	}
}

// -lungsurv2 off, or on without a stable first round: every decision is byte-identical to -lungsurv alone.
func TestPETSurv2OffIdentical(t *testing.T) {
	for name, f := range map[string]*LoopFacts{
		"stable first CT":  lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", petDay+80, "stable", "eid", 1, false)),
		"no CT":            lungFacts(60, "none", "high", big, petLow()),
		"growing first CT": lungFacts(60, "none", "high", big, petLow(), lungFU("MAF-ct-7", petDay+80, "growing", "eid", 1, false)),
	} {
		a, _ := runSurvFlags(t, true, false, false, f, petDay+500)
		b, _ := runSurvFlags(t, true, false, false, f, petDay+500)
		on, _ := runSurvFlags(t, true, true, false, f, petDay+500)
		for id, d := range a {
			if b[id].Sig != d.Sig {
				t.Fatalf("%s: %s not deterministic", name, id)
			}
			if name != "stable first CT" && on[id].Sig != d.Sig {
				t.Fatalf("%s: %s changed with -lungsurv2 although no later round opened", name, id)
			}
		}
		if name == "stable first CT" && (len(on) != len(a)+1 || on[f.LoopID+"#surv2"] == nil) {
			t.Fatalf("%s: want exactly one more loop with -lungsurv2, got %d vs %d", name, len(on), len(a))
		}
	}
}

// -lungsurvstable: the stable-CT child raises review items on an untrusted link or reader disagreement (the old child
// skipped such CTs silently), names its obligation, and closes on a trusted agreed CT exactly as before.
func TestStableSurvReviewItems(t *testing.T) {
	ct1 := lungFU("MAF-ct-1", 50, "stable", "eid", 1, false)
	lo := -200 + float64(months(18))
	for name, c := range map[string]struct {
		fu   DownFact
		kind string
	}{
		"untrusted link": {lungFU("MAF-ct-2", lo+10, "stable", "wasl", 0.8, false), "link"},
		"disagreement":   {lungFU("MAF-ct-2", lo+10, "", "eid", 1, false), "evidence"},
	} {
		f := lungFacts(60, "none", "high", "solid/6-8/single", ct1, c.fu)
		fin, out := runSurvFlags(t, false, false, true, f, lo+20)
		s := fin[f.LoopID+"#surv"]
		if s == nil || s.State != "open" || !pendingItem(out, f.LoopID, c.kind) || len(s.Pending) == 0 || lastBlock(s) != "hold_decision" {
			t.Fatalf("%s: %+v", name, s)
		}
		if sv := s.Lung.Surveillance; sv == nil || sv.Reason != "fleischner_stable" || fin[f.LoopID].Lung.Surveillance != nil {
			t.Fatalf("%s: obligation %+v", name, sv)
		}
		old, oldOut := runSurvFlags(t, false, false, false, f, lo+20) // without the flag: no item, no hold
		if pendingItem(oldOut, f.LoopID, c.kind) || old[f.LoopID+"#surv"].Pending != nil {
			t.Fatalf("%s: flag off must keep the old child", name)
		}
	}
	// an untrusted CT followed by a trusted agreed one: the old child closed on the second, the new one asks about the
	// first and still closes on the second (the untrusted CT is not evidence either way)
	f := lungFacts(60, "none", "high", "solid/6-8/single", ct1, lungFU("MAF-ct-2", lo+10, "growing", "wasl", 0.8, false),
		lungFU("MAF-ct-3", lo+20, "stable", "eid", 1, false))
	fin, out := runSurvFlags(t, false, false, true, f, lo+30)
	if s := fin[f.LoopID+"#surv"]; s.State != "closed" || s.ClosingReport != "MAF-ct-3" || !pendingItem(out, f.LoopID, "link") || s.Lung.Surveillance.State != "done" {
		t.Fatalf("closure after an untrusted CT: %+v", s)
	}
	// overdue and recall, in the imaging centre's name
	g := lungFacts(60, "none", "high", "solid/6-8/single", ct1)
	g.Contact = "granted"
	hi := -200 + float64(months(24))
	fin, _ = runSurvFlags(t, false, false, true, g, hi+1)
	if s := fin[g.LoopID+"#surv"]; !s.Overdue || !hasEsc(s, "overdue_lung_nodule_window") || len(s.Outreach) != 1 || s.Outreach[0].Kind != "lung_surveillance_recall" {
		t.Fatalf("overdue stable child: %+v", s)
	}
}

// Metrics: each family is scored apart; a stable child or a later round never counts as an extra PET/CT child.
func TestSurvFamiliesMetrics(t *testing.T) {
	rec := fleischnerRec(FlSolid, 12, FlSingle, "high")
	tr := &Truth{Loops: map[string]*LoopTruth{
		"a": {Loop: "a", Prog: "lung_nodule", State: "closed", Lung: &LungTruth{InScope: true, Rec: rec, Surv: &LungSurvTruth{Loop: "a#surv", State: "closed", Outcome: "stable", CTReport: "CT-a",
			Later: []*LungSurvTruth{{Loop: "a#surv2", Round: 2, State: "closed", CTReport: "CT-a2"}, {Loop: "a#surv3", Round: 3, State: "open"}}}}},
		"b": {Loop: "b", Prog: "lung_nodule", State: "closed", Lung: &LungTruth{InScope: true, Rec: rec, StableSurv: &LungSurvTruth{Loop: "b#surv", Reason: "fleischner_stable", State: "open", Overdue: true}}},
	}}
	pet, st := &LungSurv{Reason: "pet_low"}, &LungSurv{Reason: "fleischner_stable"}
	final := map[string]*Decision{"a": {State: "closed"}, "b": {State: "closed"},
		"a#surv":  {LoopID: "a#surv", State: "closed", ClosingReport: "CT-a", Lung: &LungDecision{Surveillance: pet}},
		"a#surv2": {LoopID: "a#surv2", State: "closed", ClosingReport: "CT-a2", Lung: &LungDecision{Surveillance: &LungSurv{Reason: "pet_low", Round: 2}}},
		"a#surv3": {LoopID: "a#surv3", State: "open", Lung: &LungDecision{Surveillance: &LungSurv{Reason: "pet_low", Round: 3}}},
		"b#surv":  {LoopID: "b#surv", State: "open", Overdue: true, Lung: &LungDecision{Surveillance: st}},
	}
	m := lungMetrics(final, tr)
	ps, pl, ss := m["pet_surveillance"].(map[string]interface{}), m["pet_surveillance_later"].(map[string]interface{}), m["stable_surveillance"].(map[string]interface{})
	if ps["obligations"] != 1 || ps["opened"] != 1 || ps["extra_opened"] != 0 {
		t.Fatalf("pet_surveillance: %v", ps)
	}
	if pl["obligations"] != 2 || pl["opened"] != 2 || pl["extra_opened"] != 0 || pl["closure"].(map[string]interface{})["tp"] != 1 ||
		pl["by_round"].(map[string]interface{})["3"].(map[string]interface{})["obligations"] != 1 {
		t.Fatalf("pet_surveillance_later: %v", pl)
	}
	if ss["obligations"] != 1 || ss["opened"] != 1 || ss["extra_opened"] != 0 || ss["overdue_flag_accuracy"] != 1.0 {
		t.Fatalf("stable_surveillance: %v", ss)
	}
}

// Generator: -lungsurv2 and -lungsurvstable off are byte-identical to -lungsurv alone; on, they only append follow-up
// hospital CTs, and the truth they add is consistent (a later round only after a stable round; CTs after the previous one).
func TestGenLungSurv2AndStableFlags(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	savedP, savedOn, s1, s2, s3 := append([]ProvCfg{}, Providers...), LungOn, LungPETSurv, LungPETSurv2, LungStableSurv
	defer func() { Providers, LungOn, LungPETSurv, LungPETSurv2, LungStableSurv = savedP, savedOn, s1, s2, s3 }()
	EnableLung()
	gen := func(pet2, stable bool) string {
		LungPETSurv, LungPETSurv2, LungStableSurv = true, pet2, stable
		out := t.TempDir()
		devnull, _ := os.Open(os.DevNull)
		stdout := os.Stdout
		os.Stdout = devnull
		defer func() { os.Stdout = stdout }()
		runGen("../data", out, "../data/evidence/reports43.json", "../data/evidence/pathology_templates.json", 7, 150, 40, 5000, 60,
			loadPools("../patient-parsers/econsult_testset.json", "../patient-parsers/teachback_testset.json"), 1088, "", 0, "", 0, "")
		return out
	}
	base, later, stable := gen(false, false), gen(true, false), gen(false, true)
	for _, dir := range []string{later, stable} {
		for _, p := range []string{"ahs-mushrif", "medlab", "skmc-endo", "noor-endo", "corniche-wh", "rad-imaging"} {
			fa, _ := os.ReadFile(filepath.Join(base, "fhir_"+p+".ndjson"))
			fb, _ := os.ReadFile(filepath.Join(dir, "fhir_"+p+".ndjson"))
			if len(fa) == 0 || !bytes.Equal(fa, fb) {
				t.Fatalf("provider feed %s changed", p)
			}
		}
		fa, _ := os.ReadFile(filepath.Join(base, "fhir_mafraq-rad.ndjson"))
		fb, _ := os.ReadFile(filepath.Join(dir, "fhir_mafraq-rad.ndjson"))
		on := map[string]bool{}
		for _, l := range bytes.Split(fb, []byte("\n")) {
			on[string(l)] = true
		}
		for _, l := range bytes.Split(fa, []byte("\n")) {
			if !on[string(l)] {
				t.Fatalf("an existing follow-up hospital line changed: %.120s", l)
			}
		}
	}
	readTruth := func(dir string) *Truth {
		var tr Truth
		b, _ := os.ReadFile(filepath.Join(dir, "truth.json"))
		json.Unmarshal(b, &tr)
		return &tr
	}
	tb, tl, ts := readTruth(base), readTruth(later), readTruth(stable)
	rounds, stableN, stableCT := map[int]int{}, 0, 0
	for id, lt := range tl.Loops {
		x := *lt
		var lx LungTruth
		if lt.Lung != nil {
			lx = *lt.Lung
			if lx.Surv != nil {
				sv := *lx.Surv
				prev := &sv
				for _, r := range sv.Later {
					if prev.State != "closed" || prev.Outcome != "stable" || r.Loop == "" || (r.CTReport != "" && r.CTDay <= prev.CTDay) || tl.Reports[r.CTReport] == "" && r.CTReport != "" {
						t.Fatalf("%s: round %d after %+v: %+v", id, r.Round, prev, r)
					}
					rounds[r.Round]++
					prev = r
				}
				sv.Later = nil
				lx.Surv = &sv
			}
			x.Lung = &lx
		}
		a, _ := json.Marshal(&x)
		b, _ := json.Marshal(tb.Loops[id])
		if !bytes.Equal(a, b) {
			t.Fatalf("-lungsurv2 changed loop %s beyond the later rounds", id)
		}
	}
	for id, lt := range ts.Loops {
		if lt.Lung == nil {
			continue
		}
		if s := lt.Lung.StableSurv; s != nil {
			stableN++
			if lt.State != "closed" || s.Loop != id+"#surv" || s.Reason != "fleischner_stable" || lt.Lung.Rec.NextHi == 0 || (s.CTReport != "" && s.CTDay <= lt.ClosedDay) {
				t.Fatalf("stable surveillance truth on %s: %+v", id, s)
			}
			if s.CTReport != "" {
				stableCT++
			}
			x := *lt
			lx := *lt.Lung
			lx.StableSurv, x.Lung = nil, &lx
			a, _ := json.Marshal(&x)
			b, _ := json.Marshal(tb.Loops[id])
			if !bytes.Equal(a, b) {
				t.Fatalf("-lungsurvstable changed loop %s beyond its obligation", id)
			}
		}
	}
	t.Logf("later rounds %v, stable obligations %d (CT seen %d)", rounds, stableN, stableCT)
	if rounds[2] == 0 || stableN == 0 || stableCT == 0 {
		t.Fatalf("flags produced no truth: rounds %v stable %d/%d", rounds, stableN, stableCT)
	}
}
