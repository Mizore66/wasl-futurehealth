package main

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The templates approved before the lung and DR families were added keep their ids and exact text, so messages
// already sent still verify (VerifyComm).
func TestApprovedTemplateIDsUnchanged(t *testing.T) {
	ids := []string{}
	for _, fam := range []string{"ccsfu-notify", "ccsfu-remind", "fitfu-notify", "fitfu-remind", "tb-clarify", "tb-prompt"} {
		for _, v := range []string{"v1", "v2"} {
			for _, l := range []string{"ar", "en"} {
				ids = append(ids, fam+"-"+v+"/"+l)
			}
		}
	}
	sort.Strings(ids)
	s := ""
	for _, id := range ids {
		tp, ok := Templates[id]
		if !ok {
			t.Fatalf("approved template %s missing", id)
		}
		s += id + "\x00" + tp.Body + "\x00"
	}
	if got := sha([]byte(s)); got != "6b3efa58bad61855689979f3fd92d42141b2ff5fcb92ad31e211c024e5fe1bc7" {
		t.Fatalf("an approved template's text changed: %s", got)
	}
}

// Lung-nodule (lnfu) and diabetic-retinopathy (drsfu) families: one template per outreach kind, English and Arabic,
// v1 and the v2 AI-use disclosure; the same wording rules as the screening families (no test name, no result, the
// privacy line, "Reply 1" to confirm), and each renders for every provider of its programme.
func TestProgrammeTemplateFamilies(t *testing.T) {
	EnableLung()
	EnableDR()
	testEN := regexp.MustCompile(`(?i)\b(nodul\w*|lungs?|pulmonary|ct|scan\w*|retin\w*|macul\w*|diabet\w*|photo\w*|grad\w*|x-?ray)\b`)
	testAR := []string{"عقدة", "رئة", "رئوي", "الشبكية", "اعتلال", "السكري", "تصوير", "أشعة"}
	provs := map[string][]string{"lung_nodule": {"rad-imaging", "mafraq-rad"}, "diabetic_retinopathy": {"phc-retina", "eye-hosp"}}
	n := 0
	for kind, tk := range outreachTplKind {
		prog := map[bool]string{true: "lung_nodule", false: "diabetic_retinopathy"}[strings.HasPrefix(kind, "lung_")]
		for _, lang := range []string{"en", "ar"} {
			for _, v := range []string{"v1", "v2"} {
				id := Rules[prog].Tpl + "-" + tk + "-" + v + "/" + lang
				tp, ok := Templates[id]
				if !ok {
					t.Fatalf("%s: no template %s", kind, id)
				}
				n++
				if tp.Lang != lang {
					t.Errorf("%s has language %s", id, tp.Lang)
				}
				privacy := map[string]string{"en": "For your privacy, no clinical details are ever included in messages.", "ar": "حفاظًا على خصوصيتك، لا تتضمن الرسائل أي تفاصيل طبية."}[lang]
				reply := map[string]string{"en": "Reply 1 to confirm you received this message.", "ar": "أرسل 1 لتأكيد استلام هذه الرسالة."}[lang]
				if !strings.Contains(tp.Body, privacy) || !strings.Contains(tp.Body, reply) {
					t.Errorf("%s lacks the privacy line or the Reply 1 confirmation", id)
				}
				if m := testEN.FindString(tp.Body); m != "" {
					t.Errorf("%s names the test or a finding (%q)", id, m)
				}
				for _, w := range testAR {
					if strings.Contains(tp.Body, w) {
						t.Errorf("%s names the test or a finding (%q)", id, w)
					}
				}
				for _, p := range provs[prog] {
					if _, why := Render(id, p); why != "" {
						t.Errorf("%s for %s: %s", id, p, why)
					}
				}
			}
		}
	}
	if n != 36 { // 9 outreach kinds x 2 languages x 2 versions (the C11 recall kind reuses drsfu-recall)
		t.Fatalf("%d lung/DR templates, want 36", n)
	}
}

// The content guard also blocks lung and retina findings in free text.
func TestGuardBlocksLungAndRetinaFindings(t *testing.T) {
	for _, s := range []string{"Your CT showed a lung nodule.", "The pulmonary nodule needs a follow-up scan.",
		"Your retinopathy screening needs a referral.", "Maculopathy was found.", "تم العثور على عقدة في الرئة.", "يوجد اعتلال في الشبكية."} {
		if guardText(s) == "" {
			t.Errorf("guard passed %q", s)
		}
	}
}

// Lung and DR outreach intents name the approved template of their programme family and the loop's language; flag
// off, they carry no template (as before).
func TestLungDROutreachNamesTemplate(t *testing.T) {
	defer func(v bool) { ProgTemplates = v }(ProgTemplates)
	ProgTemplates = true
	EnableLung()
	EnableDR()
	fd := &LoopFacts{LoopID: "phc-retina/ServiceRequest/D1", Provider: "phc-retina", SRLang: "ar", Contact: "granted"}
	dd := &DRDecision{Rec: &DRRec{Action: DRRepeatPhoto}}
	od := drOutreach(fd, dd, "open", false, 10)
	if len(od) != 1 || od[0].Template != "drsfu-repeat-"+TemplateVersion+"/ar" {
		t.Fatalf("DR repeat-photo intent: %+v", od)
	}
	if txt, why := Render(od[0].Template, fd.Provider); why != "" || txt == "" {
		t.Fatalf("DR template does not render for the sender: %s", why)
	}
	fl := &LoopFacts{LoopID: "rad-imaging/ServiceRequest/L1", Provider: "rad-imaging", Contact: "granted"}
	ld := &LungDecision{Rec: &FlRec{Action: FlCT}, Scope: "in_scope", WindowEnd: 100}
	ol := lungOutreach(fl, ld, "open", true, 120)
	if len(ol) != 1 || ol[0].Template != "lnfu-recall-"+TemplateVersion+"/en" { // no recorded language: the provider default
		t.Fatalf("lung overdue intent: %+v", ol)
	}
	ProgTemplates = false
	if o := drOutreach(fd, dd, "open", false, 10); len(o) != 1 || o[0].Template != "" {
		t.Fatalf("flag off: no template expected, got %+v", o)
	}
}
