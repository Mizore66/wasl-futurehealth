package main

import (
	"fmt"
	"strings"
	"testing"
)

// Review fix 2: ZWNJ/ZWJ are legitimate in Arabic-script names; bidi overrides and other Cf characters are not.
func TestActionTextAllowsArabicZWNJZWJAndRejectsBidiAndOtherCf(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	// Persian with ZWNJ (U+200C), and joiner forms with ZWJ (U+200D)
	for _, owner := range []string{"می‌خواهد", "فاطمه‌زاده", "ک‍ا"} {
		code, m := coord(t, srv, l, hb(headOf(s, l), fmt.Sprintf(`"action":"assign_owner","owner":%q`, owner)))
		if code != 201 {
			t.Fatalf("Arabic-script name %q rejected: %d %v", owner, code, m)
		}
	}
	h := headOf(s, l)
	for _, r := range []rune{0x202E, 0x202A, 0x202B, 0x202C, 0x202D, 0x2066, 0x2067, 0x2068, 0x2069, 0x200E, 0x200F, 0x061C, 0x200B, 0x2060, 0xFEFF, 0x00AD} {
		code, _ := coord(t, srv, l, hb(h, fmt.Sprintf(`"action":"assign_owner","owner":"Fa\u%04xtima"`, r)))
		if code != 400 {
			t.Errorf("U+%04X accepted (%d)", r, code)
		}
	}
	if s2 := loadAPI(run, ""); s2.ledgerErr != "" {
		t.Fatalf("replay of ZWNJ ledger: %s", s2.ledgerErr)
	}
}

// The case-list summary carries the ledger status, so a partial overlay is never listed unannounced.
func TestCaseListOverlayCarriesLedgerStatus(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	coord(t, srv, l, hb(headOf(s, l), `"action":"assign_owner","owner":"Fatima"`))
	listStatus := func() string {
		_, m := rawGet(t, srv.URL+"/api/cases")
		for _, it := range m["cases"].([]interface{}) {
			c := it.(map[string]interface{})
			if c["id"] == l {
				st, _ := c["api_overlay"].(map[string]interface{})["ledger_status"].(string)
				return st
			}
		}
		t.Fatal("case not listed")
		return ""
	}
	if st := listStatus(); st != "ok" {
		t.Fatalf("ok ledger: %q", st)
	}
	s.mu.Lock()
	s.ledgerErr = "line 2: boom"
	s.mu.Unlock()
	if st := listStatus(); st != "invalid" {
		t.Fatalf("invalid ledger not announced on list: %q", st)
	}
}

// Drift is exercised through the real loadReviews on a server whose canonical head changed since the events were written.
func TestLoadReviewsFailsClosedOnDriftedHead(t *testing.T) {
	run := twoCaseRun(t)
	s, srv := serveAgent(t, run)
	l := agentLoop("1")
	coord(t, srv, l, hb(headOf(s, l), `"action":"assign_owner","owner":"Fatima"`))
	coord(t, srv, l, hb(headOf(s, l), `"action":"note","text":"second"`))

	s2 := loadAPI(run, "")
	if s2.ledgerErr != "" || s2.cases[l].ov.n != 2 {
		t.Fatalf("baseline reload: %q n=%d", s2.ledgerErr, s2.cases[l].ov.n)
	}
	// the canonical head changes (re-materialised run), then the real reload runs
	s2.cases[l].last.Sig = "changed-decision-signature"
	s2.reviews, s2.ledgerErr = nil, ""
	s2.cases[l].ov = caseOverlay{}
	s2.loadReviews()
	if !strings.Contains(s2.ledgerErr, "head") || !strings.HasPrefix(s2.ledgerErr, "line 1:") {
		t.Fatalf("drift accepted by loadReviews: %q", s2.ledgerErr)
	}
	if s2.cases[l].ov.n != 0 {
		t.Fatalf("overlay applied past failing line: %d", s2.cases[l].ov.n)
	}
	if v := s2.ledgerView(); v["status"] != "invalid" || v["detail"] == "" {
		t.Fatalf("ledger view: %v", v)
	}
	if ov := s2.overlayView(s2.cases[l], false); ov["ledger_status"] != "invalid" {
		t.Fatalf("list summary overlay: %v", ov)
	}
}
