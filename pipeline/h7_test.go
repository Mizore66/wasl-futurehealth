package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// H7 mitigation tests. Error rates and correlations here are assumed stress-test values, not clinician rates.

// h7Items: evidence items whose true label is inadequate, so a wrong read is often a closing "adequate".
func h7Items(n int) ([]*ReviewItem, *Truth) {
	tr := &Truth{PKQid: map[string]string{}, Reports: map[string]string{}}
	items := []*ReviewItem{}
	for i := 0; i < n; i++ {
		tr.Reports["R"+itoa(i)] = "inadequate"
		items = append(items, &ReviewItem{ItemID: "E" + itoa(i), Kinds: []string{"evidence"}, ReportID: "R" + itoa(i)})
	}
	return items, tr
}

// With every mitigation off, the mitigation parameters change nothing: same sign-off bytes as the pre-mitigation
// model (which had no such parameters, i.e. all zero).
func TestH7MitigationsOffIsIdentical(t *testing.T) {
	items, tr := h7Items(3000)
	legacy := ReviewerCfg{Err: 0.08, Rho: 0.5, Double: "merge+close", Seed: 3, LinkMedS: 45, EvMedS: 90, Sigma: 0.6}
	cur := DefaultReviewer
	cur.Err, cur.Rho, cur.Double, cur.Seed = 0.08, 0.5, "merge+close", 3
	if cur.BlindFactor == 0 || cur.RotateFactor == 0 || cur.SeniorErrRel == 0 {
		t.Fatal("defaults should carry non-zero mitigation parameters for this test to mean anything")
	}
	for _, it := range items {
		a, _ := json.Marshal(legacy.Review(it, tr))
		b, _ := json.Marshal(cur.Review(it, tr))
		if string(a) != string(b) {
			t.Fatalf("mitigations off changed a sign-off:\n%s\n%s", a, b)
		}
	}
	if cur.effRho() != cur.Rho {
		t.Fatalf("effRho %v with mitigations off, want %v", cur.effRho(), cur.Rho)
	}
}

// Blinding and rotation scale the copy probability as documented, and lower wrong closures at rho=1.
func TestH7BlindAndRotateReduceCorrelation(t *testing.T) {
	items, tr := h7Items(20000)
	base := DefaultReviewer
	base.Err, base.Rho, base.Double, base.Seed = 0.2, 1, "merge+close", 7
	blind, both := base, base
	blind.Blind = true
	both.Blind, both.Rotate = true, true
	if blind.effRho() != 0.5 || math.Abs(both.effRho()-0.45) > 1e-12 {
		t.Fatalf("effRho blind %v (want 0.5), blind+rotate %v (want 0.45)", blind.effRho(), both.effRho())
	}
	wrongClose := func(c ReviewerCfg) int {
		n := 0
		for _, it := range items {
			so := c.Review(it, tr)
			if so.Evidence == "adequate" {
				n++
			}
			if c.Blind && !strings.Contains(so.Reviewer, "blind=0.50") {
				t.Fatalf("reviewer string lacks blind: %s", so.Reviewer)
			}
		}
		return n
	}
	b, bl, bo := wrongClose(base), wrongClose(blind), wrongClose(both)
	// rho=1: every first-read "adequate" is copied, ~ 20000*0.2*0.5 = 2000 wrong closures. Blinding halves the copies by
	// r2, but the tie-break read r3 still copies with effRho, so expect ~0.8x (blind) and ~0.755x (blind+rotate).
	if !(b > bl && bl > bo) || b < 1500 || bl > b*85/100 {
		t.Fatalf("wrong closures not reduced as modelled: base %d, blind %d, blind+rotate %d", b, bl, bo)
	}
}

// The senior audit samples about p of signed closing answers, never anything else, is deterministic, overrides the
// answer on disagreement, and is not serialised.
func TestH7SeniorAudit(t *testing.T) {
	items, tr := h7Items(20000)
	c := DefaultReviewer
	c.Err, c.Seed, c.SeniorAudit = 0.3, 11, 0.2
	sos := map[string]*Signoff{}
	closing, audited := 0, 0
	for _, it := range items {
		so := c.Review(it, tr)
		so2 := c.Review(it, tr)
		if so.Evidence != so2.Evidence || strings.Join(so.Reads, ",") != strings.Join(so2.Reads, ",") {
			t.Fatal("audit not deterministic")
		}
		sos[it.ItemID] = so
		if so.audit == nil {
			if strings.Contains(strings.Join(so.Reads, ","), "s1") {
				t.Fatal("senior read without an audit record")
			}
			if so.Evidence == "adequate" {
				closing++
			}
			continue
		}
		audited++
		closing++
		if so.audit.prevEv != "adequate" && so.audit.prevEv != "histology" {
			t.Fatalf("audited a non-closing answer %q", so.audit.prevEv)
		}
		if so.Reads[len(so.Reads)-1] != "s1" || !strings.Contains(so.Reviewer, "audit=0.200") {
			t.Fatalf("audited sign-off lacks the senior read: %+v", so)
		}
		if so.audit.disagree == (so.Evidence == so.audit.prevEv) {
			t.Fatalf("disagreement did not override (or agreement changed) the answer: %+v %+v", so, so.audit)
		}
		if b, _ := json.Marshal(so); strings.Contains(string(b), "audit\"") || strings.Contains(string(b), "prev") {
			t.Fatalf("audit record serialised: %s", b)
		}
	}
	if r := float64(audited) / float64(closing); math.Abs(r-0.2) > 0.03 {
		t.Fatalf("audited %d of %d closing answers (%.3f), want ~0.2", audited, closing, r)
	}
	st := auditStats(sos, tr)
	if st["audited"] != audited || st["fixed"]+st["missed"] != st["wrong_before"] || st["fixed"]+st["introduced"] > st["disagreed"] {
		t.Fatalf("inconsistent audit stats %v", st)
	}
	// every audited answer here is a wrong "adequate"; the senior read (error 0.3*0.5) disagrees ~85% of the time
	if st["wrong_before"] != audited || float64(st["fixed"])/float64(audited) < 0.75 {
		t.Fatalf("senior audit fixed too few: %v", st)
	}
}

// Under the double read, a senior audit never copies the first read even at rho=1.
func TestH7SeniorReadIsIndependent(t *testing.T) {
	items, tr := h7Items(20000)
	c := DefaultReviewer
	c.Err, c.Rho, c.Double, c.Seed, c.SeniorAudit = 0.3, 1, "merge+close", 5, 1
	agree, n := 0, 0
	for _, it := range items {
		so := c.Review(it, tr)
		if so.audit == nil {
			continue
		}
		n++
		if !so.audit.disagree {
			agree++
		}
	}
	// a copying senior would agree with every wrong "adequate"; an independent one with error 0.15 agrees only when it
	// errs onto the same alternative label (~0.15/2)
	if n == 0 || float64(agree)/float64(n) > 0.12 {
		t.Fatalf("senior agreed with %d of %d wrong closures: not independent", agree, n)
	}
}
