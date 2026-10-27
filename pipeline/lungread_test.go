package main

import "testing"

// Reader A (regex) must reproduce the labelled key on the whole synthetic corpus, traps included.
func TestLungRegexReaderMatchesKey(t *testing.T) {
	for _, kf := range [][2]string{{"lung-finding", "finding_reports.json"}, {"lung-ct", "followup_reports.json"}} {
		for _, d := range loadLungDocs("../data/lung", kf[1]) {
			var got string
			if kf[0] == "lung-finding" {
				got = regexFinding(d.Text).Label
			} else {
				got = regexFollowup(d.Text).Label
			}
			if want := keyLabel(kf[0], d.Key); got != want {
				t.Errorf("%s: regex reader %q, key %q\n%s", d.ID, got, want, d.Text)
			}
		}
	}
}

// Reader B missing (no cache entry) can never produce agreement: the report goes to review.
func TestLungJudgeReaderUnavailableNeverAgrees(t *testing.T) {
	saved := LungCache
	LungCache = nil
	defer func() { LungCache = saved }()
	d := loadLungDocs("../data/lung", "followup_reports.json")[0]
	if v := LungJudge(d.Text, "lung-ct"); v.Agree || v.GemmaHit || v.Label != "disagree" {
		t.Fatalf("reader B unavailable must route to review, got %+v", v)
	}
}
