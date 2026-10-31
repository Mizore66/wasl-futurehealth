package main

import (
	"strings"
	"testing"
)

// Every v2 template discloses the automated service, renders for every provider and passes the content guard.
func TestTemplatesDiscloseAutomation(t *testing.T) {
	n := 0
	for id, tp := range Templates {
		if !strings.Contains(id, "-v2/") {
			continue
		}
		n++
		v1 := Templates[strings.Replace(id, "-v2/", "-v1/", 1)]
		if v1.ID == "" || !strings.HasPrefix(tp.Body, v1.Body) {
			t.Errorf("%s does not extend its v1 text", id)
		}
		want := map[string]string{"en": "automated", "ar": "آلي"}[tp.Lang]
		if !strings.Contains(tp.Body, want) {
			t.Errorf("%s does not disclose automation", id)
		}
		for prov := range ProvDisplay {
			if _, why := Render(id, prov); why != "" {
				t.Errorf("%s for %s: %s", id, prov, why)
			}
		}
	}
	if n != 12 {
		t.Fatalf("%d v2 templates, want 12", n)
	}
}
