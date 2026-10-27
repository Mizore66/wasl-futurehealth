package main

import "testing"

// Hold-only invariant: whatever the photos say, the image reader never creates agreement or a closure label.
func TestImageReaderNeverCloses(t *testing.T) {
	labels := []string{"adequate", "inadequate", "awaiting_histology", "disagree", "needs_human"}
	fields := []string{"yes", "no", "not_stated"}
	preps := []string{"adequate", "inadequate", "not_stated"}
	ivs := []*ImageVerdict{
		{Caecal: "verified", Prep: "adequate"}, {Caecal: "verified", Prep: "inadequate"}, {Caecal: "missing", Prep: "none"},
		{Caecal: "unverified", Prep: "adequate"}, {Caecal: "missing", Prep: "inadequate"}, {Caecal: "verified", Prep: "none"},
	}
	for _, lab := range labels {
		for _, agree := range []bool{true, false} {
			for _, rc := range fields {
				for _, gc := range fields {
					for _, rp := range preps {
						for _, gp := range preps {
							for _, iv := range ivs {
								v := &TwoVerdict{Kind: "colonoscopy", Agree: agree, Label: lab,
									Regex: ReaderVerdict{Caecum: rc, Prep: rp}, Gemma: ReaderVerdict{Caecum: gc, Prep: gp}}
								applyImage(v, iv)
								if v.Agree && !agree {
									t.Fatalf("image reader created agreement: %+v %+v", v, iv)
								}
								if v.Label != lab && v.Label != "disagree" {
									t.Fatalf("image reader changed label %s -> %s", lab, v.Label)
								}
								if lab != "adequate" && v.Label == "adequate" {
									t.Fatalf("image reader produced adequate from %s", lab)
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestImageReaderRules(t *testing.T) {
	ungraded := func() *TwoVerdict {
		return &TwoVerdict{Kind: "colonoscopy", Agree: false, Label: "disagree",
			Regex: ReaderVerdict{Caecum: "yes", Prep: "not_stated"}, Gemma: ReaderVerdict{Caecum: "yes", Prep: "not_stated"}}
	}
	v := ungraded()
	applyImage(v, &ImageVerdict{Caecal: "verified", Prep: "adequate"})
	if v.ReviewHint != "prep_grade_one_click" || v.Agree {
		t.Fatalf("ungraded + adequate photos must route to a one-click human grade, got %+v", v)
	}
	v = ungraded()
	applyImage(v, &ImageVerdict{Caecal: "missing", Prep: "inadequate"})
	if v.ReviewHint != "image_prep_inadequate" || len(v.Flags) != 1 || v.Flags[0] != "missing_caecal_photo" {
		t.Fatalf("ungraded + inadequate photos must flag and route to full review, got %+v", v)
	}
	ok := &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate", Rule: "per_field_consensus",
		Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate"}, Gemma: ReaderVerdict{Caecum: "yes", Prep: "adequate"}}
	applyImage(ok, &ImageVerdict{Caecal: "verified", Prep: "inadequate"})
	if ok.Agree || ok.Label != "disagree" || ok.ReviewHint != "image_prep_conflict" || ok.PreImage != "adequate" {
		t.Fatalf("photos contradicting an adequate prep grade must hold, got %+v", ok)
	}
	ok2 := &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "adequate",
		Regex: ReaderVerdict{Caecum: "yes", Prep: "adequate"}, Gemma: ReaderVerdict{Caecum: "yes", Prep: "adequate"}}
	applyImage(ok2, &ImageVerdict{Caecal: "missing", Prep: "adequate"})
	if !ok2.Agree || ok2.Label != "adequate" || len(ok2.Flags) != 1 {
		t.Fatalf("missing caecal photo is an audit flag, not a closure blocker, got %+v", ok2)
	}
}

func TestImageReaderAggregation(t *testing.T) {
	r := &ImageReader{Mode: "cached", Model: "m", ThrLand: 0.5, ThrPr: 0.5, cache: map[string]imgScore{
		"c1": {Sha: "c1", LandmarkP: 0.9}, "c2": {Sha: "c2", LandmarkP: 0.1},
		"m1": {Sha: "m1", PrepP: 0.9}, "m2": {Sha: "m2", PrepP: 0.2}}}
	v := r.Read([]ImgRef{{"c1", "caecum"}, {"m1", "mucosa"}, {"m2", "mucosa"}})
	if v.Caecal != "verified" || v.Prep != "inadequate" {
		t.Fatalf("worst mucosal view decides; got %+v", v)
	}
	v = r.Read([]ImgRef{{"c2", "caecum"}, {"zz", "mucosa"}})
	if v.Caecal != "unverified" || v.Prep != "none" || v.Unavailable != 1 {
		t.Fatalf("unrecognised caecal photo / unreadable photo; got %+v", v)
	}
	if v := r.Read(nil); v.Caecal != "missing" || v.Prep != "none" {
		t.Fatalf("no photos; got %+v", v)
	}
}
