package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	tokClinician       = "clinician-token-1234567890"
	dispositionTokens  = ledgerTokens + ",senior-clin-token-1234567890:senior_clinician:senior-clin," + "reviewer-four-token-123456:reviewer:reviewer-four"
	tokSeniorClinician = "senior-clin-token-1234567890"
)

// holdRun writes a one-case synthetic run whose current decision carries the given programme hold.
func holdRun(t *testing.T, prog, state, dropoff, esc string) string {
	t.Helper()
	run := t.TempDir()
	d := &Decision{ID: "dec-1", LoopID: ledgerLoop, InSeq: 7, Prog: prog, Provider: "exp-prov", SRDay: 3, Now: 40, State: state, Dropoff: dropoff,
		Policy: "v7strict", Contact: "granted", NotifDay: -1, AckDay: -1, ProcDay: -1}
	if esc != "" {
		d.Escalations = []string{esc}
	}
	d.Sig = decSig(d)
	m := LogMsg{Seq: 1, Subject: "out.p0", Hdr: map[string]string{}}
	encodeEnv(&m, OutEnvelope{Kind: "decision", Dec: d})
	b, _ := json.Marshal(m)
	for name, body := range map[string][]byte{"out_log.ndjson": append(b, '\n'), "fhir_log.ndjson": nil, "review_queue.json": []byte("[]")} {
		if err := os.WriteFile(filepath.Join(run, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

var dispositionHolds = []struct{ name, prog, state, dropoff, esc string }{
	{"no fundal view", "diabetic_retinopathy", "not_assessable", "not_assessable", "clinician_decision_no_fundal_view"},
	{"outside Fleischner", "lung_nodule", "out_of_scope", "out_of_scope", "clinician_decision_outside_fleischner"},
}

func dispositionBody(head string, fields string) string {
	return fmt.Sprintf(`{"action":"clinician_disposition","expected_head":%q,%s}`, head, fields)
}

// dispositionReviewDate is a future date: the API refuses a plan review date before today (B7).
var dispositionReviewDate = time.Now().UTC().AddDate(0, 0, 30).Format("2006-01-02")

var validDisposition = `"reason":"No fundal view on two attempts; synthetic","plan":"alternative_diagnostic","details":"Slit-lamp examination at the eye clinic","owner":"Dr Synthetic","review_date":"` + dispositionReviewDate + `"`

func TestClinicianDispositionRecordsPlanWithoutClosingTheLoop(t *testing.T) {
	for _, hc := range dispositionHolds {
		t.Run(hc.name, func(t *testing.T) {
			run := holdRun(t, hc.prog, hc.state, hc.dropoff, hc.esc)
			s := loadAPI(run, "")
			h, err := s.handler(dispositionTokens, "")
			if err != nil {
				t.Fatal(err)
			}
			path := "/api/cases/" + url.PathEscape(ledgerLoop)
			resp := ledgerPost(t, h, path+"/actions", tokClinician, dispositionBody(s.caseHead(s.cases[ledgerLoop]), validDisposition), http.StatusCreated)
			ev := resp["event"].(map[string]interface{})
			if ev["actor"] != "clinician-bound" || ev["role"] != RoleClinician || ev["action"] != ActRecordDisposition {
				t.Fatalf("event identity = %v", ev)
			}
			check := func(when string, d map[string]interface{}) {
				t.Helper()
				c := d["case"].(map[string]interface{})
				if c["state"] != hc.state || c["closed"] != false {
					t.Fatalf("%s: disposition changed the clinical state: %v closed=%v", when, c["state"], c["closed"])
				}
				ov := d["api_overlay"].(map[string]interface{})
				disp, _ := ov["clinician_disposition"].(map[string]interface{})
				want := map[string]interface{}{"plan": "alternative_diagnostic", "owner": "Dr Synthetic", "review_date": dispositionReviewDate, "hold": hc.esc,
					"actor": "clinician-bound", "reason": "No fundal view on two attempts; synthetic", "details": "Slit-lamp examination at the eye clinic"}
				for k, v := range want {
					if disp[k] != v {
						t.Fatalf("%s: clinician_disposition.%s = %v, want %v (%v)", when, k, disp[k], v, ov)
					}
				}
			}
			check("after write", ledgerGet(t, h, path, tokCoordinator))
			s2 := loadAPI(run, "")
			if s2.ledgerErr != "" {
				t.Fatalf("replay rejected the disposition: %s", s2.ledgerErr)
			}
			h2, _ := s2.handler(dispositionTokens, "")
			check("after restart", ledgerGet(t, h2, path, tokCoordinator))
			for _, v := range ledgerGet(t, h2, "/api/cases", tokCoordinator)["cases"].([]interface{}) {
				ov := v.(map[string]interface{})["api_overlay"].(map[string]interface{})
				disp, _ := ov["clinician_disposition"].(map[string]interface{})
				if disp["plan"] != "alternative_diagnostic" || disp["owner"] != "Dr Synthetic" || disp["review_date"] != dispositionReviewDate || disp["reason"] != nil {
					t.Fatalf("list projection = %v, want plan, owner and date without free text", disp)
				}
			}
		})
	}
}

func TestClinicianDispositionRejectsInvalidWrites(t *testing.T) {
	hc := dispositionHolds[0]
	run := holdRun(t, hc.prog, hc.state, hc.dropoff, hc.esc)
	s := loadAPI(run, "")
	h, _ := s.handler(dispositionTokens, "")
	path := "/api/cases/" + url.PathEscape(ledgerLoop) + "/actions"
	head := s.caseHead(s.cases[ledgerLoop])
	drop := func(field string) string {
		parts := strings.Split(validDisposition, `,"`)
		out := []string{}
		for _, p := range parts {
			if !strings.HasPrefix(strings.TrimPrefix(p, `"`), field+`"`) {
				out = append(out, p)
			}
		}
		return strings.Join(out, `,"`)
	}
	for _, tc := range []struct {
		name, token, body string
		want              int
	}{
		{"missing reason", tokClinician, dispositionBody(head, drop("reason")), 400},
		{"missing plan", tokClinician, dispositionBody(head, drop("plan")), 400},
		{"missing owner", tokClinician, dispositionBody(head, drop("owner")), 400},
		{"missing review date", tokClinician, dispositionBody(head, drop("review_date")), 400},
		{"missing details", tokClinician, dispositionBody(head, drop("details")), 400},
		{"unsupported plan", tokClinician, dispositionBody(head, strings.Replace(validDisposition, "alternative_diagnostic", "procedure_done", 1)), 400},
		{"bad review date", tokClinician, dispositionBody(head, strings.Replace(validDisposition, dispositionReviewDate, "next week", 1)), 400},
		{"past review date", tokClinician, dispositionBody(head, strings.Replace(validDisposition, dispositionReviewDate, "2020-01-01", 1)), 400},
		{"review date beyond a year", tokClinician, dispositionBody(head, strings.Replace(validDisposition, dispositionReviewDate, time.Now().UTC().AddDate(1, 1, 0).Format("2006-01-02"), 1)), 400},
		{"client supplied hold", tokClinician, dispositionBody(head, validDisposition+`,"hold":"clinician_decision_no_fundal_view"`), 400},
		{"coordinator", tokCoordinator, dispositionBody(head, validDisposition), 403},
		{"reviewer", "reviewer-four-token-123456", dispositionBody(head, validDisposition), 403},
		{"auditor", tokAuditor, dispositionBody(head, validDisposition), 403},
		{"stale head", tokClinician, dispositionBody(strings.Repeat("a", 64), validDisposition), 409},
		{"clinician cannot write coordinator notes", tokClinician, fmt.Sprintf(`{"action":"note","text":"synthetic","expected_head":%q}`, head), 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledgerPost(t, h, path, tc.token, tc.body, tc.want)
		})
	}
	if b, _ := os.ReadFile(filepath.Join(run, "api_reviews.ndjson")); len(b) != 0 {
		t.Fatalf("a rejected disposition appended to the ledger: %s", b)
	}
	if ov := s.overlayView(s.cases[ledgerLoop], false); ov["clinician_disposition"] != nil || ov["actions"] != 0 {
		t.Fatalf("a rejected disposition changed the projection: %v", ov)
	}
	ledgerPost(t, h, path, tokSeniorClinician, dispositionBody(head, validDisposition), http.StatusCreated)
}

func TestClinicianDispositionNeedsAClinicianDecisionHold(t *testing.T) {
	for _, tc := range []struct{ name, prog, state, dropoff, esc string }{
		{"CRC case", "crc", "open", "not_booked", ""},
		{"lung not assessable is a repeat CT, not a clinician hold", "lung_nodule", "not_assessable", "not_assessable", "repeat_ct_not_assessable"},
		{"hold code on the wrong programme", "crc", "open", "not_booked", "clinician_decision_no_fundal_view"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := loadAPI(holdRun(t, tc.prog, tc.state, tc.dropoff, tc.esc), "")
			h, _ := s.handler(dispositionTokens, "")
			ledgerPost(t, h, "/api/cases/"+url.PathEscape(ledgerLoop)+"/actions", tokClinician, dispositionBody(s.caseHead(s.cases[ledgerLoop]), validDisposition), http.StatusConflict)
		})
	}
}

// Qualifying evidence moves the programme state machine off the hold; the recorded plan stays in history only.
func TestClinicianDispositionStopsProjectingOnceTheHoldClears(t *testing.T) {
	hc := dispositionHolds[0]
	s := loadAPI(holdRun(t, hc.prog, hc.state, hc.dropoff, hc.esc), "")
	h, _ := s.handler(dispositionTokens, "")
	path := "/api/cases/" + url.PathEscape(ledgerLoop)
	ledgerPost(t, h, path+"/actions", tokClinician, dispositionBody(s.caseHead(s.cases[ledgerLoop]), validDisposition), http.StatusCreated)
	c := s.cases[ledgerLoop]
	next := *c.last
	next.State, next.Dropoff, next.Escalations = "open", "no_eye_exam", nil
	c.last = &next
	full := s.overlayView(c, true)
	if full["clinician_disposition"] != nil {
		t.Fatalf("a disposition for a cleared hold is still projected as the current plan: %v", full["clinician_disposition"])
	}
	if hist, _ := full["clinician_dispositions"].([]map[string]interface{}); len(hist) != 1 {
		t.Fatalf("disposition history = %v, want the recorded plan kept", full["clinician_dispositions"])
	}
}
