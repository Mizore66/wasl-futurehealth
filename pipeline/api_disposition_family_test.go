package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The two clinician dispositions are one family on one route: the same permission, the same ledger event type and
// action, the same expected-head chain and replay, told apart by the typed `action` kind. On a non-CRC case the FIT
// re-offer is neither required nor accepted, and neither kind changes the decision, closes the loop or marks a procedure.
func TestDispositionFamilySharesOneRouteAndKeepsFITToCRC(t *testing.T) {
	hc := dispositionHolds[0]
	run := holdRun(t, hc.prog, hc.state, hc.dropoff, hc.esc)
	s := loadAPI(run, "")
	h, _ := s.handler(dispositionTokens, "")
	path := "/api/cases/" + url.PathEscape(ledgerLoop)
	head := func() string { return s.caseHead(s.cases[ledgerLoop]) }
	review := `"follow_up_owner":"Dr Synthetic","next_review":"` + time.Now().UTC().AddDate(0, 2, 0).Format("2006-01-02") + `"`
	decline := func(extra string) string {
		return fmt.Sprintf(`{"action":"decline_disposition","expected_head":%q,"reason":"procedure_anxiety","counselling":"Explained why the eye exam matters; synthetic","safety_net":"Seek care for sudden vision change; synthetic","plan":"follow_up",%s%s}`, head(), review, extra)
	}

	ledgerPost(t, h, path+"/actions", tokCoordinator, fmt.Sprintf(`{"action":"contact_outcome","outcome":"declined","channel":"phone","expected_head":%q}`, head()), http.StatusCreated)
	ledgerPost(t, h, path+"/actions", tokClinician, decline(`,"fit_reoffer":"offered_declined"`), http.StatusBadRequest)
	if s.cases[ledgerLoop].ov.decline != nil {
		t.Fatal("a refused FIT re-offer on a non-CRC case was recorded")
	}
	ledgerPost(t, h, path+"/actions", tokCoordinator, decline(""), http.StatusForbidden)
	declined := ledgerPost(t, h, path+"/actions", tokClinician, decline(""), http.StatusCreated)["event"].(map[string]interface{})
	planned := ledgerPost(t, h, path+"/actions", tokClinician, dispositionBody(head(), validDisposition), http.StatusCreated)["event"].(map[string]interface{})
	for _, ev := range []map[string]interface{}{declined, planned} {
		if ev["type"] != coordEventType || ev["action"] != ActRecordDisposition || ev["role"] != RoleClinician || ev["actor"] != "clinician-bound" {
			t.Fatalf("disposition event identity = %v", ev)
		}
	}
	if declined["kind"] != declineDispositionKind || planned["kind"] != dispositionKind || planned["prev"] != declined["hash"] {
		t.Fatalf("kinds %v/%v, chain prev %v want %v", declined["kind"], planned["kind"], planned["prev"], declined["hash"])
	}

	if err := validateReviewLedger(filepath.Join(run, "api_reviews.ndjson")); err != nil {
		t.Fatalf("ledger verification: %v", err)
	}
	s2 := loadAPI(run, "")
	if s2.ledgerErr != "" {
		t.Fatalf("replay: %q", s2.ledgerErr)
	}
	h2, _ := s2.handler(dispositionTokens, "")
	d := ledgerGet(t, h2, path, tokCoordinator)
	c := d["case"].(map[string]interface{})
	if c["state"] != hc.state || c["closed"] != false {
		t.Fatalf("dispositions changed the case: state %v closed %v", c["state"], c["closed"])
	}
	ov := d["api_overlay"].(map[string]interface{})
	dd, _ := ov["decline_disposition"].(map[string]interface{})
	cd, _ := ov["clinician_disposition"].(map[string]interface{})
	if dd["plan"] != "follow_up" || dd["fit_reoffer"] != nil || dd["role"] != RoleClinician || cd["plan"] != "alternative_diagnostic" || cd["hold"] != hc.esc {
		t.Fatalf("replayed projections: decline %v, hold plan %v", dd, cd)
	}
	raw, _ := os.ReadFile(filepath.Join(run, "api_reviews.ndjson"))
	for _, banned := range []string{`"procedure_done"`, `"appointment"`, `"closed":true`} {
		if strings.Contains(string(raw), banned) {
			t.Fatalf("a disposition wrote %s to the ledger: %s", banned, raw)
		}
	}
}
