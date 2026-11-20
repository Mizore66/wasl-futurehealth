package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func onTimeCases(ds ...*Decision) map[string]*apiCase {
	out := map[string]*apiCase{}
	for _, d := range ds {
		out[d.LoopID] = &apiCase{ID: d.LoopID, Programme: d.Prog, last: d}
	}
	return out
}

// Each programme's own clock and step, as its decision records them; still-running clocks, inactive cases, derived
// care, surveillance children, unproven LDCT clocks and screens with no follow-up are not counted.
func TestOnTimeByProgramme(t *testing.T) {
	drRefer := DRRec{Action: DRRefer, Deadline: 42, Urgency: "urgent"}
	drNone, _ := drRule("R0M0")
	brRecall := BreastRec{Action: BreastRecallA, Deadline: 14, Urgency: "routine"}
	brRoutine := BreastRec{Action: BreastRoutine, Deadline: 730}
	fl := &FlRec{Action: "ct", WinLo: 61, WinHi: 91}
	ldRec := &LDCTRec{Action: LDCTFollowup, Due: 183, Deadline: 213}
	ifRec := &IfhasRec{Action: IfhasFollowUp, Deadline: 10, Domains: []string{"bp", "glyc"}, DomDays: map[string]int{"bp": 10, "glyc": 59}}
	cases := onTimeCases(
		// CRC, 31 days from the referral: on time (day 30), late (day 40), no colonoscopy, and a clock still running.
		&Decision{LoopID: "crc-1", Prog: "crc", State: "closed", SRDay: 10, Deadline: 31, ProcDay: 40},
		&Decision{LoopID: "crc-2", Prog: "crc", State: "awaiting_histology", SRDay: 0, Deadline: 31, ProcDay: 40},
		&Decision{LoopID: "crc-3", Prog: "crc", State: "not_booked", SRDay: 5, Deadline: 31, ProcDay: -1},
		&Decision{LoopID: "crc-4", Prog: "crc", State: "booked", SRDay: 100, Deadline: 31, ProcDay: -1},
		&Decision{LoopID: "crc-5", Prog: "crc", State: "closed", SRDay: 1, Deadline: 31, ProcDay: 2, Inactive: true},
		&Decision{LoopID: "crc-6", Prog: "crc", State: "booked", SRDay: 1, Deadline: 31, ProcDay: -1, CRC: &CRCDecision{Kind: "surveillance", Parent: "crc-1"}},
		// Cervical windows follow priority.
		&Decision{LoopID: "cx-1", Prog: "cervical", State: "closed", SRDay: 10, Deadline: 14, ProcDay: 20},
		&Decision{LoopID: "cx-2", Prog: "cervical", State: "closed", SRDay: 10, Deadline: 56, ProcDay: 70},
		// Lung: follow-up CT by the end of the Fleischner window; the surveillance child and out-of-scope nodule are not counted.
		&Decision{LoopID: "lung-1", Prog: "lung_nodule", State: "closed", SRDay: 0, ProcDay: 80, Lung: &LungDecision{Scope: "in_scope", Rec: fl, FollowUpCT: true}},
		&Decision{LoopID: "lung-2", Prog: "lung_nodule", State: "no_followup_ct", SRDay: 0, ProcDay: -1, Lung: &LungDecision{Scope: "in_scope", Rec: fl}},
		&Decision{LoopID: "lung-2#surv", Prog: "lung_nodule", State: "closed", SRDay: 0, ProcDay: 10, Lung: &LungDecision{Scope: "in_scope", Rec: fl, FollowUpCT: true, Surveillance: &LungSurv{Loop: "lung-2#surv"}}},
		&Decision{LoopID: "lung-3", Prog: "lung_nodule", State: "out_of_scope", SRDay: 0, ProcDay: -1, Lung: &LungDecision{Scope: "age_under_35", Rec: fl}},
		&Decision{LoopID: "lung-4", Prog: "lung_nodule", State: "no_followup_needed", SRDay: 0, ProcDay: -1, Lung: &LungDecision{Scope: "in_scope", Rec: &FlRec{Action: FlNone}}},
		// DR: referable only; the exam must be within the deadline from the grading day.
		&Decision{LoopID: "dr-1", Prog: "diabetic_retinopathy", State: "closed", DR: &DRDecision{Rec: &drRefer, GradeDay: 5, Examined: true, WithinDeadline: true}},
		&Decision{LoopID: "dr-2", Prog: "diabetic_retinopathy", State: "closed", DR: &DRDecision{Rec: &drRefer, GradeDay: 5, Examined: true}},
		&Decision{LoopID: "dr-3", Prog: "diabetic_retinopathy", State: "no_referral_needed", DR: &DRDecision{Rec: &drNone, GradeDay: 5}},
		// Breast: recall to assessment only, outcome documented within the clock and closed.
		&Decision{LoopID: "br-1", Prog: "breast", State: "closed", Breast: &BreastDecision{Rec: &brRecall, GradeDay: 50, WithinDeadline: true}},
		&Decision{LoopID: "br-2", Prog: "breast", State: "no_assessment_outcome", Breast: &BreastDecision{Rec: &brRecall, GradeDay: 50, WithinDeadline: true}},
		&Decision{LoopID: "br-3", Prog: "breast", State: "no_recall_needed", Breast: &BreastDecision{Rec: &brRoutine, GradeDay: 50}},
		// LDCT: only on a proven clock.
		&Decision{LoopID: "ld-1", Prog: "lung_ldct", State: "closed", LDCT: &LDCTDecision{Rec: ldRec, DeadlineDay: 100, WithinDeadline: true}},
		&Decision{LoopID: "ld-2", Prog: "lung_ldct", State: "followup_ldct_not_done", LDCT: &LDCTDecision{Rec: ldRec}},
		// IFHAS: the window is the longest abnormal domain's clock.
		&Decision{LoopID: "if-1", Prog: "ifhas", State: "closed", Ifhas: &IfhasDecision{Rec: ifRec, GradeDay: 30, WithinDeadline: true}},
		&Decision{LoopID: "if-2", Prog: "ifhas", State: "no_followup_documented", Ifhas: &IfhasDecision{Rec: ifRec, GradeDay: 30}},
		&Decision{LoopID: "if-3", Prog: "ifhas", State: "open", Ifhas: &IfhasDecision{Rec: ifRec, GradeDay: 80}},
	)
	got := onTimeByProgramme(cases)
	b, _ := json.Marshal(got)
	var rows []map[string]interface{}
	json.Unmarshal(b, &rows)
	want := []map[string]interface{}{
		{"programme": "crc", "window_days": 31.0, "eligible": 3.0, "on_time": 1.0, "share": 0.3333},
		{"programme": "cervical", "window_days": 14.0, "eligible": 1.0, "on_time": 1.0, "share": 1.0},
		{"programme": "cervical", "window_days": 56.0, "eligible": 1.0, "on_time": 0.0, "share": 0.0},
		{"programme": "lung_nodule", "window_days": 91.0, "eligible": 2.0, "on_time": 1.0, "share": 0.5},
		{"programme": "diabetic_retinopathy", "window_days": 42.0, "eligible": 2.0, "on_time": 1.0, "share": 0.5},
		{"programme": "breast", "window_days": 14.0, "eligible": 2.0, "on_time": 1.0, "share": 0.5},
		{"programme": "lung_ldct", "window_days": 213.0, "eligible": 1.0, "on_time": 1.0, "share": 1.0},
		{"programme": "ifhas", "window_days": 59.0, "eligible": 2.0, "on_time": 1.0, "share": 0.5},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("on_time_by_programme\n got %s\nwant %v", b, want)
	}
	if got := onTimeByProgramme(map[string]*apiCase{}); got == nil || len(got) != 0 {
		t.Fatalf("an empty run must give an empty list, got %#v", got)
	}
}

// The dashboard serves the metric from the loaded run's final decisions.
func TestDashboardServesOnTimeByProgramme(t *testing.T) {
	run := t.TempDir()
	var lines []string
	for i, d := range []*Decision{
		{LoopID: "ahs/ServiceRequest/AHS-sr-1", Prog: "crc", State: "closed", Provider: "ahs", SRDay: 10, Deadline: 31, ProcDay: 30, ClosedDay: 45},
		{LoopID: "ahs/ServiceRequest/AHS-sr-2", Prog: "crc", State: "not_booked", Provider: "ahs", SRDay: 10, Deadline: 31, ProcDay: -1},
	} {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte(strings.Join(lines, "\n")+"\n"), 0644)
	os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0644)
	s := loadAPI(run, "")
	rec := httptest.NewRecorder()
	s.getDashboard(rec, httptest.NewRequest("GET", "/api/dashboard", nil))
	var body struct {
		Metrics struct {
			OnTime []map[string]interface{} `json:"on_time_by_programme"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []map[string]interface{}{{"programme": "crc", "window_days": 31.0, "eligible": 2.0, "on_time": 1.0, "share": 0.5}}
	if !reflect.DeepEqual(body.Metrics.OnTime, want) {
		t.Fatalf("dashboard on_time_by_programme = %v, want %v", body.Metrics.OnTime, want)
	}
}

// On a real materialised run (WASL_TEST_RUN), the colorectal row is the Wasl arm of report7's crc_kpi_31d: the same
// referrals, the same 31-day clock and the same colonoscopy day.
func TestOnTimeCRCMatchesReport7(t *testing.T) {
	src := os.Getenv("WASL_TEST_RUN")
	if src == "" {
		t.Skip("WASL_TEST_RUN not set")
	}
	s := loadAPI(src, "")
	kpi, _ := s.report["crc_kpi_31d"].(map[string]interface{})
	arm, _ := kpi["wasl_live"].(map[string]interface{})
	if arm == nil {
		t.Skip("run has no crc_kpi_31d")
	}
	for _, r := range onTimeByProgramme(s.cases) {
		if r["programme"] != "crc" {
			continue
		}
		if float64(r["eligible"].(int)) != arm["eligible_referrals"].(float64) || float64(r["on_time"].(int)) != arm["within_31d"].(float64) {
			t.Fatalf("crc on-time %v != report7 wasl_live %v", r, arm)
		}
		return
	}
	t.Fatal("no crc on-time row")
}
