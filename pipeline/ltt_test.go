package main

import "testing"

// -lttprog: each loop is scored with its own programme's resolution, and a CRC/cervical-only truth gives exactly the
// counts evalDecisions gives (so the committed CRC + cervical calibration is unchanged by the per-programme path).
func TestLTTProgrammeResolution(t *testing.T) {
	tr := &Truth{Loops: map[string]*LoopTruth{
		"crc1":  {Prog: "crc", State: "closed"},
		"crc2":  {Prog: "crc", State: "open"},
		"cerv1": {Prog: "cervical", State: "open"},
		"lung1": {Prog: "lung_nodule", State: "open", Lung: &LungTruth{}},               // predicted no_followup_needed: false
		"lung2": {Prog: "lung_nodule", State: "no_followup_needed", Lung: &LungTruth{}}, // resolved both ways: tp
		"lung3": {Prog: "lung_nodule", State: "closed", Lung: &LungTruth{Surv: &LungSurvTruth{Loop: "lung3#surv", State: "open"}}},
		"dr1":   {Prog: "diabetic_retinopathy", State: "no_referral_needed", DR: &DRTruth{Rec: DRRec{Action: DRRefer}}},
		"dr2":   {Prog: "diabetic_retinopathy", State: "open", DR: &DRTruth{Rec: DRRec{Action: "rescreen"}}},
		"br1":   {Prog: "breast", State: "open", Breast: &BreastTruth{Rec: BreastRec{Action: BreastRecallA}}},
	}}
	final := map[string]*Decision{
		"crc1": {State: "closed"}, "crc2": {State: "closed"}, "cerv1": {State: "open"},
		"lung1": {State: "no_followup_needed"}, "lung2": {State: "no_followup_needed"},
		"lung3":      {State: "closed"},
		"lung3#surv": {LoopID: "lung3#surv", State: "closed", Lung: &LungDecision{Surveillance: &LungSurv{}}}, // child closed, truth open
		"dr1":        {State: "closed"},                                                                       // referable loop "closed" on no_referral_needed
		"dr2":        {State: "no_referral_needed"},
		"br1":        {State: "no_recall_needed"},
	}
	bp := lttByProg(final, tr)
	want := map[string][4]int{ // loops, closed, tp, false closures
		"crc": {2, 2, 1, 1}, "cervical": {1, 0, 0, 0}, "lung_nodule": {3, 3, 2, 2}, "diabetic_retinopathy": {2, 2, 1, 2}, "breast": {1, 1, 0, 1},
	}
	for pg, w := range want {
		x := bp[pg]
		if x == nil || x.Loops != w[0] || x.Closed != w[1] || x.TP != w[2] || x.FalseClosures != w[3] {
			t.Fatalf("%s: got %+v want loops/closed/tp/fc %v", pg, x, w)
		}
	}
	// CRC + cervical only: identical to evalDecisions
	tr2 := &Truth{Loops: map[string]*LoopTruth{}, PKQid: map[string]string{}}
	for _, id := range []string{"crc1", "crc2", "cerv1"} {
		tr2.Loops[id] = tr.Loops[id]
	}
	acc, bp2 := evalDecisions(final, tr2, map[string]string{}), lttByProg(final, tr2)
	fc, tp, closed := 0, 0, 0
	for _, x := range bp2 {
		fc, tp, closed = fc+x.FalseClosures, tp+x.TP, closed+x.Closed
	}
	if fc != acc["false_closures"].(int) || tp != acc["tp"].(int) || closed != acc["closed_pred"].(int) {
		t.Fatalf("CRC/cervical: per-programme counts fc %d tp %d closed %d differ from evalDecisions %v", fc, tp, closed, acc)
	}
}
