package main

import "testing"

// v7 (after the sixth corpus, by a third model family; disclosed): the ileocaecal valve alone is not a completed
// examination, and more wordings say the caecum was not reached.
func TestValveAloneIsNotCaecum(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Extent: The instrument tip reached the distal lip of the ileocecal valve, but due to a tight alpha loop the scope could not be advanced past the valve into the cecal pole; the appendiceal orifice was not reached. Colonoscopy.", "no"},
		{"Extent of Exam: Advanced to the level of the ileocecal valve orifice; however, the cecal base could not be intubated. Colonoscopy.", "no"},
		{"Extent: advanced proximally. Aphthous erosions noted near the ileocaecal valve. Colonoscopy.", "not_stated"},
		{"The colonoscope was advanced to the caecum, identified by the appendiceal orifice and ileocaecal valve.", "yes"},
		{"Colonoscopy: terminal ileum intubated.", "yes"},
		{"The scope was advanced to the mid-ascending colon. From this vantage point, the cecal pole and appendiceal orifice were visualized at a distance, though severe looping prevented advancing the tip into the cecum itself.", "no"},
		{"Colonoscopy: the caecal caput was seen distally across a fluid-filled segment, but the colonoscope tip could not be manoeuvred into the caecum.", "no"},
		{"Colonoscopy: the caecum was reached and the appendiceal orifice photographed; the terminal ileum could not be intubated.", "yes"},
	}
	for _, c := range cases {
		if got, _, _, _ := regexFields(c.text); got != c.want {
			t.Fatalf("regex caecum %q, want %q: %q", got, c.want, c.text)
		}
	}
	text := cases[0].text
	if got := checkCaecQuote("yes", "The instrument tip reached the distal lip of the ileocecal valve", text); got == "yes" {
		t.Fatal("valve-only quote kept as caecum reached")
	}
	if got := checkCaecQuote("yes", "advanced to the caecum, identified by the appendiceal orifice", "The colonoscope was advanced to the caecum, identified by the appendiceal orifice."); got != "yes" {
		t.Fatalf("explicit caecum quote changed to %q", got)
	}
}
