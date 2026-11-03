package main

import (
	"fmt"
	"strings"
	"testing"
)

// r1Set switches the evidence policy and the -r1review flag for one test and restores them afterwards.
func r1Set(t *testing.T, on bool) {
	p0, q0, r0 := EvPolicy, PrepQuery, R1ReviewOn
	EvPolicy, PrepQuery, R1ReviewOn = "v7strict", true, on
	t.Cleanup(func() { EvPolicy, PrepQuery, R1ReviewOn = p0, q0, r0 })
}

func r1Judge(text string, g gemmaEntry) *TwoVerdict {
	return (&Judge2{cache: map[string]gemmaEntry{textSha(text): g}}).Judge(text, "colonoscopy")
}

const (
	r1ValveText   = "Colonoscopy: the colonoscope was advanced to the ileocecal valve. The mucosa appeared normal."
	r1ValveTissue = "Colonoscopy: the colonoscope was advanced to the ileocecal valve. A 5 mm polyp was removed by cold snare."
	r1PastValve   = "Colonoscopy: the colonoscope could not be advanced past the ileocecal valve into the cecal pole. Good bowel preparation."
	r1Explicit    = "Colonoscopy: the colonoscope was advanced to the caecum, identified by the appendiceal orifice. Good bowel preparation."
)

var r1ValveG = gemmaEntry{Caecum: "not_stated", Prep: "not_stated", Polyps: "no", Histology: "no", Quote: "advanced to the ileocecal valve",
	Quotes: map[string]string{"caecum": "advanced to the ileocecal valve"}}

// (a) neither reader finds a caecum statement: flag off -> automatic inadequate (R1); flag on -> clinician review.
func TestR1ReviewEvidenceValveOnly(t *testing.T) {
	r1Set(t, false)
	v := r1Judge(r1ValveText, r1ValveG)
	if v.Regex.Caecum != "not_stated" || !v.Agree || v.Label != "inadequate" || !strings.Contains(v.Rule, "R1_completion_not_documented") || v.R1Review {
		t.Fatalf("flag off: want agreed inadequate by R1_completion_not_documented, got %+v", v)
	}
	R1ReviewOn = true
	v = r1Judge(r1ValveText, r1ValveG)
	if v.Agree || !v.R1Review || v.Label != "disagree" || !strings.Contains(v.Rule, "R1_completion_review") || strings.Contains(v.Rule, "R1_completion_not_documented") {
		t.Fatalf("flag on: want not agreed, R1Review, rule R1_completion_review; got %+v", v)
	}
}

// (b) a stated failure to pass the valve is caecum "no": still automatically inadequate with the flag on.
func TestR1ReviewPastValveStaysInadequate(t *testing.T) {
	r1Set(t, true)
	g := gemmaEntry{Caecum: "no", Prep: "adequate", Polyps: "no", Histology: "no"}
	v := r1Judge(r1PastValve, g)
	if v.Regex.Caecum != "no" || !v.Agree || v.Label != "inadequate" || v.R1Review {
		t.Fatalf("want automatic inadequate, got %+v", v)
	}
}

// (c) an explicit caecum statement is decided as before with the flag on.
func TestR1ReviewExplicitCaecumUnchanged(t *testing.T) {
	g := gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no", Quote: "advanced to the caecum, identified by the appendiceal orifice",
		Quotes: map[string]string{"caecum": "advanced to the caecum, identified by the appendiceal orifice"}}
	r1Set(t, false)
	off := fmt.Sprintf("%+v", *r1Judge(r1Explicit, g))
	R1ReviewOn = true
	on := r1Judge(r1Explicit, g)
	if fmt.Sprintf("%+v", *on) != off || on.Label != "adequate" || !on.Agree {
		t.Fatalf("explicit caecum changed by -r1review:\noff %s\non  %+v", off, *on)
	}
}

func r1Report(t *testing.T, text string, g gemmaEntry) DownFact {
	r := df("DiagnosticReport", "final", 1.0, "eid", false)
	r.ReportID = "R1"
	r.Ev = r1Judge(text, g)
	return r
}

func r1HoldState(d *Decision) string {
	for _, b := range d.Chain {
		if b.Type == "hold_decision" {
			s, _ := b.Body["state"].(string)
			return s
		}
	}
	return ""
}

// (d) closure with the flag on: one completion item, held as completion_review; the signed answer decides.
func TestR1ReviewClosure(t *testing.T) {
	r1Set(t, true)
	rep := r1Report(t, r1ValveText, r1ValveG)
	outs, d := pilotRun([]DownFact{rep}, nil)
	its := pilotItems(outs, CompletionPurpose)
	nEv := 0
	for _, o := range outs {
		if o.Item != nil && o.Item.Kinds[0] == "evidence" {
			nEv++
		}
	}
	if len(its) != 1 || nEv != 1 || its[0].ReportID != "R1" {
		t.Fatalf("want exactly one evidence item with purpose %s, got %d items (%d evidence)", CompletionPurpose, len(its), nEv)
	}
	if d.State == "inadequate" || d.State == "closed" || hasEsc(d, "rejected_inadequate") || r1HoldState(d) != "completion_review" {
		t.Fatalf("pending completion review: state %s esc %v hold %q", d.State, d.Escalations, r1HoldState(d))
	}
	if !hasBlock(d, "link_decision") || !hasBlock(d, "evidence") {
		t.Fatal("completion_review hold chain incomplete")
	}
	if _, d = pilotRun([]DownFact{rep}, map[string]string{"R1": "adequate"}); d.State != "closed" || !d.HumanAssisted {
		t.Fatalf("signed adequate, no tissue: want human-assisted closure, got %s", d.State)
	}
	if _, d = pilotRun([]DownFact{rep}, map[string]string{"R1": "inadequate"}); d.State != "inadequate" || !hasEsc(d, "rejected_inadequate") {
		t.Fatalf("signed inadequate: want inadequate, got %s %v", d.State, d.Escalations)
	}
	// tissue taken: the clinician decides completion only; the loop still waits for histology
	gt := r1ValveG
	gt.Polyps = "yes"
	tis := r1Report(t, r1ValveTissue, gt)
	if !tis.Ev.R1Review {
		t.Fatalf("tissue report not marked for completion review: %+v", tis.Ev)
	}
	if _, d = pilotRun([]DownFact{tis}, map[string]string{"R1": "adequate"}); d.State != "awaiting_histology" {
		t.Fatalf("signed adequate with tissue taken: want awaiting_histology, got %s", d.State)
	}
}

// (e) the completion decision carries the assumed ungraded error rate.
func TestR1ReviewErrFor(t *testing.T) {
	c := ReviewerCfg{Err: .04}
	if c.errFor(&ReviewItem{Reason: CompletionPurpose}) != ErrUngraded || c.errFor(&ReviewItem{Reason: PrepDay15Purpose}) != ErrUngraded || c.errFor(&ReviewItem{}) != .04 {
		t.Fatal("errFor")
	}
}

// (f) flag off: the same inputs give today's decisions (automatic inadequate, no review item).
func TestR1ReviewFlagOffUnchanged(t *testing.T) {
	r1Set(t, false)
	rep := r1Report(t, r1ValveText, r1ValveG)
	outs, d := pilotRun([]DownFact{rep}, nil)
	for _, o := range outs {
		if o.Item != nil {
			t.Fatalf("flag off raised an item: %+v", o.Item)
		}
	}
	if d.State != "inadequate" || !hasEsc(d, "rejected_inadequate") || r1HoldState(d) != "inadequate_evidence" {
		t.Fatalf("flag off: state %s esc %v hold %q", d.State, d.Escalations, r1HoldState(d))
	}
	// a verdict carrying R1Review is ignored by closure when the flag is off (ordinary evidence item)
	rep.Ev = r1Judge(r1ValveText, r1ValveG)
	rep.Ev.Agree, rep.Ev.Label, rep.Ev.R1Review = false, "disagree", true
	outs, d = pilotRun([]DownFact{rep}, nil)
	if len(pilotItems(outs, CompletionPurpose)) != 0 || len(pilotItems(outs, "")) != 1 || r1HoldState(d) != "" {
		t.Fatalf("flag off with an R1Review verdict: %d completion items, hold %q", len(pilotItems(outs, CompletionPurpose)), r1HoldState(d))
	}
}

// (g) an explicit inadequacy from either reader (here: one reader grades prep inadequate) stays automatically
// inadequate with the flag on, exactly as with it off; only a verdict decided by R1 alone goes to review.
func TestR1ReviewExplicitInadequacyStaysAuto(t *testing.T) {
	text := "Colonoscopy: the colonoscope was advanced to the ileocecal valve. Poor bowel preparation."
	g := gemmaEntry{Caecum: "not_stated", Prep: "not_stated", Polyps: "no", Histology: "no"}
	r1Set(t, false)
	off := r1Judge(text, g)
	if off.Regex.Prep != "inadequate" || off.Regex.Caecum != "not_stated" || !off.Agree || off.Label != "inadequate" {
		t.Fatalf("fixture: want flag-off agreed inadequate with regex prep inadequate, got %+v", off)
	}
	R1ReviewOn = true
	on := r1Judge(text, g)
	if fmt.Sprintf("%+v", *on) != fmt.Sprintf("%+v", *off) {
		t.Fatalf("explicit inadequacy changed by -r1review:\noff %+v\non  %+v", *off, *on)
	}
}

// (h) the pure valve-only case (R1 is the only reason for inadequate) still routes to the completion review.
func TestR1ReviewValveOnlyRoutesToReview(t *testing.T) {
	r1Set(t, true)
	v := r1Judge(r1ValveText, r1ValveG)
	if v.Agree || v.Label != "disagree" || !v.R1Review || v.Rule != "R1_completion_review" {
		t.Fatalf("valve-only: want completion review, got %+v", v)
	}
}
