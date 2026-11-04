package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func withDemoGuard(t *testing.T, on bool) {
	t.Helper()
	old, oldE := DemoGuard, EIDContraGuard
	DemoGuard, EIDContraGuard = on, false
	t.Cleanup(func() { DemoGuard, EIDContraGuard = old, oldE })
}

func demoEv(id, prov, mrn, dob, sex, eid string) *Prep {
	p := eidEvent(id, prov, mrn, "Lamia Eddinari", dob, eid)
	p.Ev.Sex = sex
	return p
}

func demoRun(gbm *GBM, ps ...*Prep) (*LinkerCore, []LinkTrace) {
	L := NewLinkerCore(gbm, 1)
	var traces []LinkTrace
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	applySeq(L, ps...)
	return L, pairsOnly(traces)
}

func wantOnePair(t *testing.T, got []LinkTrace, stage, reason string) LinkTrace {
	t.Helper()
	if len(got) != 1 || got[0].Stage != stage || got[0].Reason != reason {
		t.Fatalf("want one %s/%s, got %+v", stage, reason, got)
	}
	return got[0]
}

func TestDemoClassify(t *testing.T) {
	cases := []struct {
		name, dobA, sexA, dobB, sexB string
		want                         demoDecision
	}{
		{"exact", "1980-02-22", "F", "1980-02-22", "F", demoAutoOK},
		{"dob conflict", "1980-02-22", "F", "1981-02-22", "F", demoDOBConflict},
		{"sex conflict", "1980-02-22", "F", "1980-02-22", "M", demoSexConflict},
		{"both conflict, dob first", "1980-02-22", "F", "1981-02-22", "M", demoDOBConflict},
		{"missing dob", "", "F", "1980-02-22", "F", demoNeedsReview},
		{"invalid dob", "1980-13-45", "F", "1980-02-22", "F", demoNeedsReview},
		{"placeholder dob", "1980-01-01", "F", "1980-01-01", "F", demoNeedsReview},
		{"placeholder vs real differing", "1980-07-01", "F", "1980-02-22", "F", demoNeedsReview},
		{"unknown sex", "1980-02-22", "U", "1980-02-22", "F", demoNeedsReview},
		{"missing sex", "1980-02-22", "", "1980-02-22", "F", demoNeedsReview},
		{"missing dob but sex conflict", "", "F", "1980-02-22", "M", demoSexConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := &Rec{Dob: tc.dobA, Sex: tc.sexA}, &Rec{Dob: tc.dobB, Sex: tc.sexB}
			if got := demoClassify(a, b); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestDemoExactMerges(t *testing.T) {
	withDemoGuard(t, true)
	L, _ := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", "1980-02-22", "F", ""))
	if !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
		t.Fatal("exact demographics with a high score must merge")
	}
}

func TestDemoConflictRejected(t *testing.T) {
	for _, tc := range []struct{ name, dob2, sex2, reason string }{
		{"dob", "1981-02-22", "F", "dob_conflict"}, {"sex", "1980-02-22", "M", "sex_conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withDemoGuard(t, true)
			L, pairs := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", tc.dob2, tc.sex2, ""))
			wantOnePair(t, pairs, "rejected_demographic", tc.reason)
			if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
				t.Fatal("merged")
			}
			a, b := L.byPk["ahs-mushrif|A1"], L.byPk["noor-endo|N1"]
			if !L.cannot[a][b] || len(L.near[a]) != 0 || len(L.near[b]) != 0 {
				t.Fatalf("cannot=%v near=%v", L.cannot, L.near)
			}
			if L.Stats["demoguard_"+tc.reason] != 1 {
				t.Fatalf("stats %v", L.Stats)
			}
		})
	}
}

func TestDemoMissingDemotedKeepsScore(t *testing.T) {
	withDemoGuard(t, true)
	L, pairs := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", "", "F", ""))
	got := wantOnePair(t, pairs, "near", "demographic_missing")
	if got.P != L.pscore[pk2(L.byPk["ahs-mushrif|A1"], L.byPk["noor-endo|N1"])] || got.P < LinkThr {
		t.Fatalf("score changed: %v", got.P)
	}
	if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") || L.Stats["demoguard_missing"] != 1 || L.Stats["demoguard_eid_missing"] != 0 {
		t.Fatalf("stats %v", L.Stats)
	}
}

func TestDemoBelowFloorStaysBelowReview(t *testing.T) {
	withDemoGuard(t, true)
	_, pairs := demoRun(scoredGBM(-10), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", "", "F", ""))
	wantOnePair(t, pairs, "below_review", "")
}

func TestDemoMidScoreMissingStaysNear(t *testing.T) {
	withDemoGuard(t, true)
	withLinkFloor(t, 0.01)
	L, pairs := demoRun(scoredGBM(-1), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", "", "F", ""))
	if len(pairs) != 1 || pairs[0].Stage != "near" || pairs[0].Reason != "" || L.Stats["demoguard_missing"] != 0 {
		t.Fatalf("below-threshold pair should follow the ordinary path: %+v %v", pairs, L.Stats)
	}
}

func TestDemoEqualEID(t *testing.T) {
	eid := luhnComplete("78419802077524")
	t.Run("exact force-links", func(t *testing.T) {
		withDemoGuard(t, true)
		L, _ := demoRun(scoredGBM(-20), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), demoEv("b", "noor-endo", "N1", "1980-02-22", "F", eid))
		if !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") || L.Stats["eid_links"] != 1 {
			t.Fatalf("stats %v", L.Stats)
		}
	})
	t.Run("conflict cannot-links", func(t *testing.T) {
		withDemoGuard(t, true)
		L, pairs := demoRun(scoredGBM(-20), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), demoEv("b", "noor-endo", "N1", "1981-02-22", "F", eid))
		wantOnePair(t, pairs, "rejected_demographic", "dob_conflict")
		if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") || !L.cannot[0][1] || L.Stats["eid_links"] != 0 || L.Stats["demoguard_eid_rejected"] != 1 {
			t.Fatalf("stats %v", L.Stats)
		}
	})
	t.Run("missing becomes near without inflation", func(t *testing.T) {
		withDemoGuard(t, true)
		L, pairs := demoRun(scoredGBM(-20), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), demoEv("b", "noor-endo", "N1", "", "F", eid))
		got := wantOnePair(t, pairs, "near", "demographic_missing")
		if got.P != L.pscore[pk2(0, 1)] || got.P >= 0.01 {
			t.Fatalf("score inflated: %v", got.P)
		}
		if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") || L.near[0][1] != got.P || L.Stats["demoguard_eid_missing"] != 1 || L.Stats["eid_links"] != 0 {
			t.Fatalf("stats %v near %v", L.Stats, L.near)
		}
	})
	t.Run("missing and high score still needs review", func(t *testing.T) {
		withDemoGuard(t, true)
		L, pairs := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), demoEv("b", "noor-endo", "N1", "", "F", eid))
		wantOnePair(t, pairs, "near", "demographic_missing")
		if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
			t.Fatal("merged")
		}
	})
}

func TestDemoReviewerRules(t *testing.T) {
	setup := func(dobB, sexB string) *LinkerCore {
		withDemoGuard(t, true)
		L, _ := demoRun(scoredGBM(-10), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""), demoEv("b", "noor-endo", "N1", dobB, sexB, ""))
		return L
	}
	t.Run("missing can be linked", func(t *testing.T) {
		L := setup("", "F")
		if !L.Link(0, 1, 1, true, "human") || !sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") || L.Stats["demoguard_human_missing_merge"] != 1 {
			t.Fatalf("stats %v", L.Stats)
		}
	})
	t.Run("exact human link is not counted as missing", func(t *testing.T) {
		L := setup("1980-02-22", "F")
		if !L.Link(0, 1, 1, true, "human") || L.Stats["demoguard_human_missing_merge"] != 0 {
			t.Fatalf("stats %v", L.Stats)
		}
	})
	for _, c := range [][2]string{{"1981-02-22", "F"}, {"1980-02-22", "M"}, {"", "M"}} {
		L := setup(c[0], c[1])
		if L.Link(0, 1, 1, true, "human") || sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
			t.Fatalf("human same overrode a conflict %v", c)
		}
	}
	t.Run("same via signoff", func(t *testing.T) {
		L := setup("1981-02-22", "F")
		L.Apply(&Prep{Seq: 9, Kind: "signoff", So: &Signoff{LoopID: "x", Link: "same", Pair: [2]string{"ahs-mushrif|A1", "noor-endo|N1"}}})
		if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
			t.Fatal("signoff overrode a conflict")
		}
	})
}

func TestDemoClusterBridgeBlocked(t *testing.T) {
	withDemoGuard(t, true)
	var traces []LinkTrace
	L := NewLinkerCore(scoredGBM(-10), 1)
	L.trace = func(e LinkTrace) { traces = append(traces, e) }
	applySeq(L,
		demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""),
		demoEv("b", "noor-endo", "N1", "", "F", ""),
		demoEv("c", "medlab", "M1", "1981-02-22", "F", ""),
	)
	a, b, c := L.byPk["ahs-mushrif|A1"], L.byPk["noor-endo|N1"], L.byPk["medlab|M1"]
	if !L.Link(a, b, 1, true, "human") {
		t.Fatal("setup: A-B human link")
	}
	traces = nil
	if L.Link(b, c, 1, true, "human") {
		t.Fatal("B-C merged across a conflicting cluster")
	}
	if L.Find(a) == L.Find(c) || L.Find(b) == L.Find(c) {
		t.Fatal("A, B and C share a cluster")
	}
	if L.Stats["demoguard_cluster_conflict"] != 1 || !L.cannot[b][c] {
		t.Fatalf("stats %v", L.Stats)
	}
	if got := pairsOnly(traces); len(got) != 1 || got[0].Reason != "demographic_cluster_conflict" || got[0].Stage != "rejected_demographic" {
		t.Fatalf("trace %+v", got)
	}
	// a later model link B-C is refused too, and a non-demographic failure keeps the old reason
	traces = nil
	if L.Link(b, c, 0.99, false, "wasl") {
		t.Fatal("model link bridged")
	}
}

func TestDemoModelMergeCannotBridge(t *testing.T) {
	withDemoGuard(t, true)
	// with an exact-demographic model path blocked by the pair rule alone, the cluster check still holds
	L, _ := demoRun(constGBM(),
		demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", ""),
		demoEv("c", "medlab", "M1", "1981-02-22", "F", ""),
		demoEv("b", "noor-endo", "N1", "", "F", ""))
	if sameCluster(L, "ahs-mushrif|A1", "medlab|M1") {
		t.Fatal("conflicting records share a cluster")
	}
}

func TestDemoGuardOffParity(t *testing.T) {
	eid := luhnComplete("78419802077524")
	for _, tc := range []struct {
		name       string
		dob2, sex2 string
		eid        string
		wantMerged bool
	}{
		{"dob conflict", "1981-02-22", "F", "", true},
		{"sex conflict", "1980-02-22", "M", "", true},
		{"missing dob", "", "F", "", true},
		{"equal eid conflicting", "1981-02-22", "F", eid, true},
		{"equal eid missing", "", "F", eid, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withDemoGuard(t, false)
			L, pairs := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", tc.eid), demoEv("b", "noor-endo", "N1", tc.dob2, tc.sex2, tc.eid))
			if sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") != tc.wantMerged {
				t.Fatalf("cluster outcome changed; pairs %+v", pairs)
			}
			for _, p := range pairs {
				if p.Reason == "demographic_missing" || strings.HasPrefix(p.Reason, "demographic_") {
					t.Fatalf("guard trace with flag off: %+v", p)
				}
			}
			for k := range L.Stats {
				if strings.HasPrefix(k, "demoguard_") {
					t.Fatalf("guard stat with flag off: %s", k)
				}
			}
		})
	}
	t.Run("guard-off equal-EID trace", func(t *testing.T) {
		withDemoGuard(t, false)
		_, pairs := demoRun(constGBM(), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), demoEv("b", "noor-endo", "N1", "", "F", eid))
		if len(pairs) != 0 && !reflect.DeepEqual(pairs[len(pairs)-1].Stage, "linked") {
			t.Fatalf("%+v", pairs)
		}
	})
}

func TestDemoGuardArgvAndPolicy(t *testing.T) {
	withDemoGuard(t, false)
	if len(demoGuardArgs()) != 0 || demoGuardPolicySuffix() != "" {
		t.Fatal("guard off must add no argv and no policy stamp")
	}
	base := []string{"-x"}
	if got := append(append([]string{}, base...), demoGuardArgs()...); !reflect.DeepEqual(got, base) {
		t.Fatalf("argv changed with guard off: %v", got)
	}
	DemoGuard = true
	if !reflect.DeepEqual(demoGuardArgs(), []string{"-demoguard"}) || demoGuardPolicySuffix() != "+demoguard" {
		t.Fatal("guard on must forward -demoguard and stamp +demoguard")
	}
}

// main.go must use the helpers (source-level pin for the wiring that cannot be unit-run).
func TestDemoGuardMainWiring(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PolicyName += demoGuardPolicySuffix()", "v7Args = append(v7Args, demoGuardArgs()...)", "DemoGuard = *demoGuard"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("main.go missing %q", want)
		}
	}
}

// With -eidguard also on, an equal-EID pair that needs review keeps its own score (no 0.2 clamp); eidguard-only is unchanged.
func TestDemoEqualEIDWithEIDGuardNoInflation(t *testing.T) {
	eid := luhnComplete("78419802077524")
	// placeholder DOBs of different years and different names: needs review for the demo guard, contradiction for eidguard
	mk := func() []*Prep {
		x := demoEv("a", "ahs-mushrif", "A1", "1980-01-01", "F", eid)
		y := demoEv("b", "noor-endo", "N1", "1985-07-01", "F", eid)
		y.Ev.Name = "Qamar Zubaydi"
		return []*Prep{x, y}
	}
	withDemoGuard(t, true)
	EIDContraGuard = true
	L, pairs := demoRun(scoredGBM(-20), mk()...)
	if len(pairs) != 1 || pairs[0].Stage != "below_review" || pairs[0].P >= 0.2 || pairs[0].P != L.pscore[pk2(0, 1)] || sameCluster(L, "ahs-mushrif|A1", "noor-endo|N1") {
		t.Fatalf("score inflated or merged: %+v", pairs)
	}
	withDemoGuard(t, false)
	EIDContraGuard = true
	_, pairs = demoRun(scoredGBM(-20), mk()...)
	if len(pairs) != 1 || pairs[0].Stage != "near" || pairs[0].P != 0.2 {
		t.Fatalf("eidguard-only clamp changed: %+v", pairs)
	}
	// eidguard-only, contradicting sex: clamp to 0.2 as before
	withDemoGuard(t, false)
	EIDContraGuard = true
	b := demoEv("b", "noor-endo", "N1", "1980-02-22", "M", eid)
	_, pairs = demoRun(scoredGBM(-20), demoEv("a", "ahs-mushrif", "A1", "1980-02-22", "F", eid), b)
	if len(pairs) != 1 || pairs[0].Stage != "near" || pairs[0].P != 0.2 {
		t.Fatalf("eidguard-only behavior changed: %+v", pairs)
	}
}
