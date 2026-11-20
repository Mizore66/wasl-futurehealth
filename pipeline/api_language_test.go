package main

import (
	"strings"
	"testing"
)

// The referral's recorded preferred language is served on the case summary and detail as a BCP-47 tag, omitted when
// not recorded or not tag-shaped, and kept out of the auditor export projection.
func TestCaseServesPreferredLanguage(t *testing.T) {
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	if _, ok := s.summary(c)["language"]; ok {
		t.Fatal("language served when not recorded")
	}
	c.lang = "ar"
	if got := s.summary(c)["language"]; got != "ar" {
		t.Fatalf("summary language = %v", got)
	}
	if got := s.caseDetailBody(c)["case"].(map[string]interface{})["language"]; got != "ar" {
		t.Fatalf("detail language = %v", got)
	}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	if strings.Contains(string(raw), `"language"`) {
		t.Fatal("export carries the preferred language")
	}
	for in, want := range map[string]string{"en": "en", "AR": "ar", "en-GB": "en-GB", "": "", "Arabic please": "", "e": "", "ar\n": "ar"} {
		if got := servedLanguage(in); got != want {
			t.Errorf("servedLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}
