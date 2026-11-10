package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
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

// ---- Amendment 2 (r1photo): route to the clinician only with an image-verified caecal photo ----

func r1JSON(t *testing.T, v *TwoVerdict) string {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// the flag-off verdict for a report, after the optional image reader, as the prep stage builds it
func r1OffVerdict(t *testing.T, text string, g gemmaEntry, iv *ImageVerdict) *TwoVerdict {
	r0 := R1ReviewOn
	R1ReviewOn = false
	defer func() { R1ReviewOn = r0 }()
	v := r1Judge(text, g)
	applyImage(v, iv)
	return v
}

func r1OnVerdict(text string, g gemmaEntry, iv *ImageVerdict) *TwoVerdict {
	v := r1Judge(text, g)
	applyImage(v, iv)
	r1photoGate(v, iv)
	return v
}

func TestR1PhotoGateVerifiedKeepsReview(t *testing.T) {
	r1Set(t, true)
	iv := &ImageVerdict{Model: "m", Photos: 1, Caecal: "verified", LandmarkP: .93, Prep: "none", Shas: []string{"c1"}}
	v := r1OnVerdict(r1ValveText, r1ValveG, iv)
	if v.Agree || v.Label != "disagree" || !v.R1Review || v.Rule != "R1_completion_review" || v.ReviewHint != "caecal_photo_verified" || v.Image != iv {
		t.Fatalf("verified caecal photo: want completion review with the photo hint, got %+v", v)
	}
}

func TestR1PhotoGateNoVerifiedPhotoIsFlagOffVerdict(t *testing.T) {
	r1Set(t, true)
	for _, iv := range []*ImageVerdict{
		{Model: "m", Photos: 1, Caecal: "unverified", LandmarkP: .12, Prep: "none", Shas: []string{"c2"}},
		{Model: "m", Photos: 0, Caecal: "missing", Prep: "none"},
		nil, // image reader off
	} {
		want := r1JSON(t, r1OffVerdict(t, r1ValveText, r1ValveG, iv))
		got := r1OnVerdict(r1ValveText, r1ValveG, iv)
		if r1JSON(t, got) != want || got.Label != "inadequate" || got.Rule != "R1_completion_not_documented" {
			t.Fatalf("caecal photo %+v: want the exact flag-off verdict\nwant %s\ngot  %s", iv, want, r1JSON(t, got))
		}
	}
}

func TestR1PhotoGateFlagOffNoOp(t *testing.T) {
	r1Set(t, false)
	for _, c := range []struct {
		text string
		g    gemmaEntry
	}{{r1ValveText, r1ValveG}, {r1PastValve, gemmaEntry{Caecum: "no", Prep: "adequate", Polyps: "no", Histology: "no"}}} {
		for _, iv := range []*ImageVerdict{{Caecal: "verified", Prep: "none"}, {Caecal: "missing", Prep: "none"}, nil} {
			v := r1Judge(c.text, c.g)
			applyImage(v, iv)
			before := r1JSON(t, v)
			r1photoGate(v, iv)
			if r1JSON(t, v) != before {
				t.Fatalf("flag off: r1photoGate changed the verdict\nbefore %s\nafter  %s", before, r1JSON(t, v))
			}
		}
	}
	// even a verdict carrying R1Review (never produced with the flag off) is left alone
	v := &TwoVerdict{Kind: "colonoscopy", Label: "disagree", R1Review: true}
	r1photoGate(v, nil)
	if !v.R1Review || v.Label != "disagree" || v.Agree {
		t.Fatalf("flag off: r1photoGate changed %+v", v)
	}
}

// prep-stage integration: the FHIR report goes through prepMsg (judge, image reader, gate) and then closure.
func r1PrepReport(t *testing.T, text string, media []map[string]interface{}) *TwoVerdict {
	res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "R1", "status": "final",
		"effectiveDateTime": "2026-01-11T00:00:00Z", "code": map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}}, "conclusion": text}
	if media != nil {
		res["media"] = media
	}
	b, _ := json.Marshal(res)
	p := prepMsg(1, "fhir.skmc-endo", nats.Header{"Wsi-Arr": []string{"10"}}, b, nil, &sync.Map{})
	if p.Ev == nil || p.Ev.Ev == nil {
		t.Fatalf("prepMsg produced no verdict: %+v", p)
	}
	return p.Ev.Ev
}

func TestR1PhotoGatePrepStage(t *testing.T) {
	r1Set(t, true)
	j0, i0 := Judge, ImageR
	t.Cleanup(func() { Judge, ImageR = j0, i0 })
	Judge = &Judge2{cache: map[string]gemmaEntry{textSha(r1ValveText): r1ValveG}}
	ImageR = &ImageReader{Mode: "cached", Model: "m", ThrLand: .5, ThrPr: .5, cache: map[string]imgScore{
		"c1": {Sha: "c1", LandmarkP: .9}, "c2": {Sha: "c2", LandmarkP: .1}}}
	photo := func(sha string) []map[string]interface{} {
		return []map[string]interface{}{{"comment": "caecum", "link": map[string]interface{}{"display": "sha256:" + sha}}}
	}
	closeWith := func(ev *TwoVerdict) ([]OutMsg, *Decision) {
		r := df("DiagnosticReport", "final", 1.0, "eid", false)
		r.ReportID, r.Ev = "R1", ev
		return pilotRun([]DownFact{r}, nil)
	}
	// verified caecal photo: one completion_undocumented item, held for the clinician
	v := r1PrepReport(t, r1ValveText, photo("c1"))
	outs, d := closeWith(v)
	if its := pilotItems(outs, CompletionPurpose); len(its) != 1 || d.State == "inadequate" || r1HoldState(d) != "completion_review" {
		t.Fatalf("verified photo: want one completion item and completion_review hold, got %d items, state %s, hold %q", len(its), d.State, r1HoldState(d))
	}
	// photo not recognised / no photo / image reader off: automatic inadequate, as with the flag off
	for name, setup := range map[string]func() []map[string]interface{}{
		"unverified": func() []map[string]interface{} { return photo("c2") },
		"missing":    func() []map[string]interface{} { return nil },
		"image off":  func() []map[string]interface{} { ImageR = nil; return photo("c1") },
	} {
		ir := ImageR
		media := setup()
		got := r1PrepReport(t, r1ValveText, media)
		R1ReviewOn = false
		want := r1PrepReport(t, r1ValveText, media)
		R1ReviewOn, ImageR = true, ir
		if r1JSON(t, got) != r1JSON(t, want) {
			t.Fatalf("%s: want the flag-off prep verdict\nwant %s\ngot  %s", name, r1JSON(t, want), r1JSON(t, got))
		}
		if outs, d := closeWith(got); len(pilotItems(outs, CompletionPurpose)) != 0 || d.State != "inadequate" {
			t.Fatalf("%s: want automatic inadequate, got state %s", name, d.State)
		}
	}
}

// reader B alone grades prep inadequate: an explicit inadequacy, so automatically inadequate with the flag on.
func TestR1ReviewReaderBPrepInadequateStaysAuto(t *testing.T) {
	g := gemmaEntry{Caecum: "not_stated", Prep: "inadequate", Polyps: "no", Histology: "no"}
	r1Set(t, false)
	off := r1Judge(r1ValveText, g)
	if off.Regex.Prep != "not_stated" || off.Gemma.Prep != "inadequate" || !off.Agree || off.Label != "inadequate" {
		t.Fatalf("fixture: want flag-off agreed inadequate with reader-B prep inadequate, got %+v", off)
	}
	R1ReviewOn = true
	if on := r1Judge(r1ValveText, g); fmt.Sprintf("%+v", *on) != fmt.Sprintf("%+v", *off) {
		t.Fatalf("reader-B explicit inadequacy changed by -r1review:\noff %+v\non  %+v", *off, *on)
	}
}

// a pending completion review does not mask another inadequate report in the same loop: the loop holds as inadequate.
func TestR1ReviewPendingPlusInadequateHoldsInadequate(t *testing.T) {
	r1Set(t, true)
	rep := r1Report(t, r1ValveText, r1ValveG)
	rep.EventID = "ev-r1"
	bad := autoReport("R2", "inadequate")
	bad.EventID, bad.Day = "ev-r2", 11
	outs, d := pilotRun([]DownFact{rep, bad}, nil)
	if len(pilotItems(outs, CompletionPurpose)) != 1 || d.State != "inadequate" || r1HoldState(d) != "inadequate_evidence" || !hasEsc(d, "rejected_inadequate") {
		t.Fatalf("pending completion review + inadequate report: state %s hold %q esc %v", d.State, r1HoldState(d), d.Escalations)
	}
}

// readerscore has no image reader: with -r1review every completion-review conversion reverts (shipped behaviour
// without photos), so the scored verdict is the flag-off one.
func TestR1PhotoGateReaderScore(t *testing.T) {
	r1Set(t, false)
	j0 := Judge
	t.Cleanup(func() { Judge = j0 })
	Judge = &Judge2{cache: map[string]gemmaEntry{textSha(r1ValveText): r1ValveG}}
	off := r1JSON(t, readerScoreVerdict(r1ValveText))
	R1ReviewOn = true
	v := readerScoreVerdict(r1ValveText)
	if !v.Agree || v.Label != "inadequate" || v.Rule != "R1_completion_not_documented" || v.R1Review || r1JSON(t, v) != off {
		t.Fatalf("readerscore with -r1review: want the flag-off verdict\nwant %s\ngot  %s", off, r1JSON(t, v))
	}
}

// Final review M3 (defence in depth): a signed completion-review "adequate" always waits for histology when tissue was
// taken, keyed on the signed item's purpose, even if a re-judging process reverted the verdict (R1Review false; image
// reader off or -r1review off).
func TestR1ReviewSignedCompletionWaitsForHistologyAfterRevert(t *testing.T) {
	for _, on := range []bool{true, false} {
		r1Set(t, false)
		gt := r1ValveG
		gt.Polyps = "yes"
		rep := r1Report(t, r1ValveTissue, gt) // flag-off verdict: agreed inadequate, R1Review false
		if rep.Ev.R1Review || !rep.Ev.Agree || rep.Ev.Label != "inadequate" || rep.Ev.Regex.Polyps != "yes" {
			t.Fatalf("fixture: want the reverted (flag-off) verdict with tissue taken, got %+v", rep.Ev)
		}
		R1ReviewOn = on
		so := Signoff{ItemID: "rv-completion", LoopID: "L", Kinds: []string{"evidence"}, ReportID: "R1", Evidence: "adequate", Purpose: CompletionPurpose}
		C := NewClosureCore(0, nil)
		outs := C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Down: []DownFact{rep},
			EvOver: map[string]string{"R1": "adequate"}, Signoffs: []Signoff{so}}, InSeq: 1})
		outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 12, InSeq: 2})...)
		if d := lastDec(outs); d.State != "awaiting_histology" {
			t.Fatalf("flag=%v: signed completion adequate on a reverted verdict with tissue taken: want awaiting_histology, got %s", on, d.State)
		}
		// an ordinary signed adequate (no completion purpose) is unchanged
		so.Purpose = ""
		C = NewClosureCore(0, nil)
		outs = C.Apply(&FactsMsg{ID: "f", Facts: &LoopFacts{LoopID: "L", SRDay: 0, Down: []DownFact{rep},
			EvOver: map[string]string{"R1": "adequate"}, Signoffs: []Signoff{so}}, InSeq: 1})
		outs = append(outs, C.Apply(&FactsMsg{ID: "t", IsTick: true, TickDay: 12, InSeq: 2})...)
		if d := lastDec(outs); d.State != "closed" {
			t.Fatalf("flag=%v: ordinary signed adequate changed: %s", on, d.State)
		}
	}
}
