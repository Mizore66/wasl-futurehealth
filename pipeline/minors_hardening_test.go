package main

import "testing"

// M1: LDCT and IFHAS patient outreach always require a granted contact permission, even with the legacy ConsentHold switch off.
func TestLDCTIfhasOutreachRequiresGrantWhenLegacyHoldOff(t *testing.T) {
	saved, savedHold := OutreachOn, ConsentHold
	OutreachOn, ConsentHold = true, false
	defer func() { OutreachOn, ConsentHold = saved, savedHold }()
	build := map[string]func(contact string) *Decision{
		"ldct": func(contact string) *Decision {
			f := lcFacts(lcScan("LCS-ct-1", 10, "LR4B"))
			f.Contact = contact
			d, _ := runLung(t, f, 60)
			return d
		},
		"ifhas": func(contact string) *Decision {
			f := ifFacts(ifScreen("IFH-dr-1", 10, "bp=H;glyc=N;lipid=N;weight=N"))
			f.Contact = contact
			d, _ := runLung(t, f, 80)
			return d
		},
	}
	for name, run := range build {
		for contact, want := range map[string]string{"unknown": SupConsentHold, "": SupConsentHold, "denied": SupConsent} {
			d := run(contact)
			if len(d.Outreach) != 1 || d.Outreach[0].Released || d.Outreach[0].Suppress != want {
				t.Errorf("%s contact=%q hold off: want suppress %s, got %+v", name, contact, want, d.Outreach)
			}
		}
		if d := run("granted"); len(d.Outreach) != 1 || !d.Outreach[0].Released {
			t.Errorf("%s granted: want released, got %+v", name, d.Outreach)
		}
	}
}

// M2: LDCT and IFHAS providers are programme-specific in cross-programme outreach accounting.
func TestCrossProgrammeOutreachIncludesLDCTAndIfhas(t *testing.T) {
	tr := &Truth{Loops: map[string]*LoopTruth{
		"L1": {Loop: "L1", Qid: "Q1", Prog: "lung_ldct", State: "open"},
		"F1": {Loop: "F1", Qid: "Q1", Prog: "ifhas", State: "open"},
		"C1": {Loop: "C1", Qid: "Q1", Prog: "crc", State: "open"},
	}, PKQid: map[string]string{"p|1": "Q1"}}
	evPK := map[string]string{"L1": "p|1", "F1": "p|1", "C1": "p|1"}
	for _, id := range []string{"lcs-centre/A/1", "thx-hosp/A/2", "ifh-screen/A/3", "ifu-clinic/A/4", "skmc-endo/A/5"} {
		evPK[id] = "p|1"
	}
	o := &offline{tr: tr, optT: map[string]bool{}, dob: map[string]string{}, evPK: evPK}
	out := func(loop string, evs ...string) OutMsg {
		d := &Decision{LoopID: loop}
		for _, e := range evs {
			d.Outreach = append(d.Outreach, Outreach{Kind: "appointment_reminder", Event: e, Released: true, Basis: "eid"})
		}
		return OutMsg{Dec: d}
	}
	rp := &replayOut{L: &LinkerCore{}, outs: []OutMsg{
		out("L1", "lcs-centre/A/1", "thx-hosp/A/2"),                                     // own programme: not cross
		out("F1", "ifh-screen/A/3", "ifu-clinic/A/4"),                                   // own programme: not cross
		out("C1", "lcs-centre/A/1", "thx-hosp/A/2", "ifh-screen/A/3", "ifu-clinic/A/4"), // 4 cross
		out("L1", "ifh-screen/A/3"),                                                     // IFHAS provider on an LDCT loop: cross
		out("F1", "skmc-endo/A/5"),                                                      // CRC provider on an IFHAS loop: cross
	}}
	c := govMetrics(o, rp)["cross_programme_outreach"].(map[string]int)
	if c["intents"] != 6 || c["released"] != 6 {
		t.Fatalf("cross-programme outreach %v, want 6 intents, 6 released", c)
	}
}
