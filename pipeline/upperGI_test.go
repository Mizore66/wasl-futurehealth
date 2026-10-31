package main

import (
	"encoding/json"
	"os"
	"testing"
)

// Combined EGD + colonoscopy reports: tissue taken in the stomach or duodenum is not colon tissue, so it does not make
// the colonoscopy wait for histology. Anything ambiguous still counts as colon tissue (the safe direction).
func TestTissueIgnoresUpperGI(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"gastric biopsies after a normal colon",
			"The colonoscope was advanced to the cecum. There were no polyps. Attention was then turned to the EGD. The stomach was entered. An antral mass was seen. Several biopsies were taken. Fundal polyps were removed with biopsy forceps.", "no"},
		{"colon biopsy after the EGD part",
			"EGD: the stomach was normal. Gastric biopsies were taken. The colonoscope was then inserted into the rectum and advanced to the cecum. Random biopsies were taken.", "yes"},
		{"one sentence names both sites",
			"Upper endoscopy and colonoscopy performed. Biopsies were taken from the stomach and the sigmoid colon. The cecum was reached.", "yes"},
		{"no upper-GI procedure: a gastric history does not hide colon tissue",
			"History of gastric bypass. Colonoscopy to the cecum. A polyp was removed.", "yes"},
		{"colon tissue words v7.1 missed (corpus 4 q275)",
			"OGD: gastric biopsies taken. The scope was passed to the caecal pole. Stricture in the sigmoid colon; biopsied (x4).", "yes"},
		{"colon polypectomy words v7.1 missed (corpus 8 v054)",
			"Separate same-day gastroscopy: gastric biopsies were taken. Cecum reached. Cold-snare excision of a 5 mm descending-colon polyp; specimen recovered and sent.", "yes"},
		{"negated biopsied (corpus 6 s122)",
			"Cecum reached. Elevations were diagnosed as lymphoid aggregates; no tissue was resected or biopsied.", "no"},
		{"tissue sentence with no site right after the colon part",
			"Gastroscopy was normal. The scope was advanced through the colon to the cecum. A 5 mm polyp was snared and retrieved.", "yes"},
	}
	for _, c := range cases {
		if _, _, po, _ := regexFields(c.text); po != c.want {
			t.Errorf("%s: tissue=%s, want %s", c.name, po, c.want)
		}
	}
	var rf struct{ Reports []keyedReport }
	b, _ := os.ReadFile("../data/evidence/reports43.json")
	json.Unmarshal(b, &rf)
	want := map[string]string{"u4": "no", "u2": "yes"} // u4: gastric tissue only; u2: colon biopsies in a combined report
	for _, r := range rf.Reports {
		if w, ok := want[r.UID]; ok {
			if _, _, po, _ := regexFields(r.Text); po != w {
				t.Errorf("MTSamples %s: tissue=%s, want %s", r.UID, po, w)
			}
		}
	}
}
