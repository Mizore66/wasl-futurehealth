package main

import (
	"encoding/json"
	"sort"
	"testing"
)

// outreachgaps.go: C11 (-drrecallmsg), C12 (-rejmsg), C13 (-drseenlink).

func withGaps(t *testing.T, recall, rej, seen bool) {
	t.Helper()
	a, b, c, d := DRRecallMsg, RejectedMsg, DRSeenLinkAsk, DRNextOn
	DRRecallMsg, RejectedMsg, DRSeenLinkAsk, DRNextOn = recall, rej, seen, true
	t.Cleanup(func() { DRRecallMsg, RejectedMsg, DRSeenLinkAsk, DRNextOn = a, b, c, d })
}

func outreachKinds(d *Decision) map[string]Outreach {
	m := map[string]Outreach{}
	for _, o := range d.Outreach {
		m[o.Kind] = o
	}
	return m
}

func linkItems(outs []OutMsg) []*ReviewItem {
	var its []*ReviewItem
	for _, o := range outs {
		if o.Kind == "review" && o.Item != nil && isLinkQuestion(o.Item) {
			its = append(its, o.Item)
		}
	}
	return its
}

// The lung / DR families added for C12 (and the earlier lnfu / drsfu ids) keep their approved text: the earlier lung
// and DR template ids from -progtpl are pinned here, as TestApprovedTemplateIDsUnchanged pins the screening families.
func TestEarlierLungDRTemplatesUnchanged(t *testing.T) {
	ids := []string{}
	for _, fam := range []string{"lnfu-remind", "lnfu-recall", "drsfu-repeat", "drsfu-remind", "drsfu-recall", "drsfu-rebook"} {
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
	if got := sha([]byte(s)); got != lungDRTemplatesSHA {
		t.Fatalf("an approved lung / DR template's text changed: %s", got)
	}
}

// C11: a resolved DR loop past its next due day gets one consent-gated recall message in the drsfu family; the loop
// stays resolved. Off: no message (as before).
func TestDRRecallOverdueMessage(t *testing.T) {
	facts := func(contact string) *LoopFacts {
		f := drFacts(drScreen("PHC-dr-1", 10, "R1M0"))
		f.Contact, f.SRLang = contact, "ar"
		return f
	}
	EnableDR()
	withGaps(t, false, false, false)
	if d, _ := runLung(t, facts("granted"), 400); !d.DR.Next.Overdue || len(d.Outreach) != 0 {
		t.Fatalf("flag off: recall flagged but no message expected, got %+v", d.Outreach)
	}
	withGaps(t, true, false, false)
	d, _ := runLung(t, facts("granted"), 400)
	o, ok := outreachKinds(d)["dr_next_recall"]
	if !ok || len(d.Outreach) != 1 || !o.Released || o.Basis != "self" || o.Event != d.LoopID || o.Sender != "phc-retina" ||
		o.Template != "drsfu-recall-"+TemplateVersion+"/ar" || d.State != "no_referral_needed" || d.Overdue || !hasEsc(d, "dr_recall_overdue") {
		t.Fatalf("overdue recall: state %s outreach %+v esc %v", d.State, d.Outreach, d.Escalations)
	}
	if txt, why := Render(o.Template, o.Sender); why != "" || txt == "" {
		t.Fatalf("recall template does not render: %s", why)
	}
	if d, _ = runLung(t, facts("granted"), 120); len(d.Outreach) != 0 { // not yet due
		t.Fatalf("message before the due day: %+v", d.Outreach)
	}
	d, _ = runLung(t, facts("unknown"), 400)
	if o := d.Outreach[0]; o.Released || o.Suppress != SupConsentHold || len(d.ConsentLog) != 1 || d.ConsentLog[0].To != "held" {
		t.Fatalf("consent unknown: want held with a consent log entry, got %+v %+v", d.Outreach, d.ConsentLog)
	}
	f := facts("granted")
	f.MsgOptOut = true
	if d, _ = runLung(t, f, 400); d.Outreach[0].Released || d.Outreach[0].Suppress != SupConsent {
		t.Fatalf("opted out: want cancelled, got %+v", d.Outreach)
	}
	// An independent annual screen belongs to a new request, not this episode:
	// it must not mark the old episode seen or suppress its recall.
	f = facts("granted")
	f.Down = append(f.Down, drScreen("PHC-dr-2", 380, "R0M0"))
	if d, _ = runLung(t, f, 400); d.DR.Next.Seen || !d.DR.Next.Overdue || len(d.Outreach) != 1 || d.DR.Screen != "PHC-dr-1" {
		t.Fatalf("independent annual screen altered old-episode recall: %+v %+v", d.DR.Next, d.Outreach)
	}
}

// C12: a finally rejected DR / lung referral gets a consent-gated patient message; patient_informed follows release.
func TestRejectedReferralMessage(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	rej := extIntake("eye-hosp", "EYE-intake-1", 30, "revoked", "capacity", "eid", 1, "diabetic_retinopathy")
	dr := func(contact string) *LoopFacts {
		f := drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), rej)
		f.Contact = contact
		return f
	}
	withGaps(t, false, false, false)
	if d, _ := runLung(t, dr("granted"), 31); d.Handoff == nil || d.Handoff.PatientInformed || len(d.Outreach) != 0 {
		t.Fatalf("flag off: packet without a message expected, got %+v %+v", d.Handoff, d.Outreach)
	}
	withGaps(t, false, true, false)
	d, _ := runLung(t, dr("granted"), 31)
	o, ok := outreachKinds(d)["dr_referral_rejected"]
	if !ok || len(d.Outreach) != 1 || !o.Released || o.Basis != "self" || o.Template != "drsfu-rejected-"+TemplateVersion+"/en" || !d.Handoff.PatientInformed || d.State != "open" {
		t.Fatalf("DR rejection: %+v packet %+v", d.Outreach, d.Handoff)
	}
	d, _ = runLung(t, dr("unknown"), 31)
	if o := outreachKinds(d)["dr_referral_rejected"]; len(d.Outreach) != 1 || o.Released || o.Suppress != SupConsentHold || d.Handoff.PatientInformed {
		t.Fatalf("consent unknown: held, patient not informed; got %+v %+v", o, d.Handoff)
	}
	if d, _ = runLung(t, dr("denied"), 31); len(d.Outreach) != 1 || d.Handoff.PatientInformed || outreachKinds(d)["dr_referral_rejected"].Suppress != SupConsent {
		t.Fatalf("consent denied: cancelled; got %+v", d.Outreach)
	}
	// no rejection: no message
	f := drFacts(drScreen("PHC-dr-1", 10, "R3AM0"))
	f.Contact = "granted"
	if d, _ = runLung(t, f, 31); len(d.Outreach) != 0 {
		t.Fatalf("no rejection, message %+v", d.Outreach)
	}
	// an untrusted rejected intake is asked about, never used for a message
	f = drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), extIntake("eye-hosp", "EYE-intake-1", 30, "revoked", "capacity", "wasl", 0.8, "diabetic_retinopathy"))
	f.Contact = "granted"
	if d, _ = runLung(t, f, 31); d.Handoff != nil || len(d.Outreach) != 0 {
		t.Fatalf("untrusted rejection: %+v %+v", d.Handoff, d.Outreach)
	}
	// lung work-up
	lf := lungFacts(60, "none", "high", big, extIntake("mafraq-rad", "MAF-intake-1", 10, "revoked", "out-of-network", "eid", 1, "lung_nodule"))
	lf.Contact = "granted"
	d, _ = runLung(t, lf, 11)
	if o, ok := outreachKinds(d)["lung_referral_rejected"]; !ok || len(d.Outreach) != 1 || !o.Released || o.Template != "lnfu-rejected-"+TemplateVersion+"/en" || !d.Handoff.PatientInformed {
		t.Fatalf("lung rejection: %+v %+v", d.Outreach, d.Handoff)
	}
	// CRC: the packet now counts its own intent (referral_rejected_next_steps)
	if !crcRejectedKind("referral_rejected_next_steps") || crcRejectedKind("appointment_reminder") {
		t.Fatal("crcRejectedKind with -rejmsg")
	}
	RejectedMsg = false
	if crcRejectedKind("referral_rejected_next_steps") {
		t.Fatal("crcRejectedKind off: unchanged (only the old kind)")
	}
}

// A final rejection replaces, rather than accompanies, due/overdue/rebooking outreach for the same referral.
func TestRejectedReferralReplacesOtherOutreach(t *testing.T) {
	defer func(h bool) { Handoff = h }(Handoff)
	Handoff = true
	dr := func() *LoopFacts {
		f := drFacts(drScreen("PHC-dr-1", 10, "R3AM0"), extIntake("eye-hosp", "EYE-intake-1", 30, "revoked", "capacity", "eid", 1, "diabetic_retinopathy"))
		f.Contact = "granted"
		return f
	}
	withGaps(t, false, false, false)
	d, _ := runLung(t, dr(), 120)
	if len(d.Outreach) != 1 || d.Outreach[0].Kind != "dr_overdue_recall" {
		t.Fatalf("DR setup: expected contradictory overdue outreach, got %+v", d.Outreach)
	}
	withGaps(t, false, true, false)
	d, _ = runLung(t, dr(), 120)
	if len(d.Outreach) != 1 || d.Outreach[0].Kind != "dr_referral_rejected" {
		t.Fatalf("DR rejection must replace overdue outreach: %+v", d.Outreach)
	}
	lung := func() *LoopFacts {
		f := lungFacts(60, "none", "high", big, extIntake("mafraq-rad", "MAF-intake-1", 10, "revoked", "out-of-network", "eid", 1, "lung_nodule"))
		f.Contact = "granted"
		return f
	}
	withGaps(t, false, false, false)
	d, _ = runLung(t, lung(), 120)
	if len(d.Outreach) != 1 || d.Outreach[0].Kind != "lung_overdue_recall" {
		t.Fatalf("lung setup: expected contradictory overdue outreach, got %+v", d.Outreach)
	}
	withGaps(t, false, true, false)
	d, _ = runLung(t, lung(), 120)
	if len(d.Outreach) != 1 || d.Outreach[0].Kind != "lung_referral_rejected" {
		t.Fatalf("lung rejection must replace overdue outreach: %+v", d.Outreach)
	}
}

// The new C11-C13 messages keep the granted-consent invariant even when the legacy hold switch is disabled.
func TestSelfOutreachAlwaysRequiresGrantedConsent(t *testing.T) {
	saved := ConsentHold
	ConsentHold = false
	t.Cleanup(func() { ConsentHold = saved })
	EnableDR()
	for _, contact := range []string{"", "unknown", "denied"} {
		f := drFacts(drScreen("PHC-dr-1", 10, "R1M0"))
		f.Contact = contact
		o := selfOutreach(f, "diabetic_retinopathy", "dr_next_recall")
		want := SupConsent
		if contact == "" || contact == "unknown" {
			want = SupConsentHold
		}
		if o.Released || o.Suppress != want {
			t.Errorf("contact %q with ConsentHold=false: got %+v, want suppression %q", contact, o, want)
		}
	}
	f := drFacts(drScreen("PHC-dr-1", 10, "R1M0"))
	f.Contact = "granted"
	if o := selfOutreach(f, "diabetic_retinopathy", "dr_next_recall"); !o.Released || o.Suppress != "" {
		t.Fatalf("granted consent should release: %+v", o)
	}
}

// C13: a later eye-clinic note on an untrusted link raises a link question and counts only after a human "same".
func TestDRSeenUntrustedLinkAsks(t *testing.T) {
	base := func(later DownFact) *LoopFacts {
		return drFacts(drScreen("PHC-dr-1", 10, "R2M0"), drNoteDays("EYE-dr-1", 20, "examined_followup", 91), later) // due day 111
	}
	untrusted := drNote("EYE-dr-2", 108, "examined_followup", "wasl", 0.8, false)
	untrusted.Weak.B = "eye-hosp|EYE-MRN000777" // the visit sits on another registration at the eye clinic
	withGaps(t, false, false, false)
	d, outs := runLung(t, base(untrusted), 120)
	if !d.DR.Next.Overdue || len(linkItems(outs)) != 0 {
		t.Fatalf("flag off: unchanged (overdue, no question), got %+v, %d items", d.DR.Next, len(linkItems(outs)))
	}
	withGaps(t, false, false, true)
	d, outs = runLung(t, base(untrusted), 120)
	its := linkItems(outs)
	if len(its) != 1 || its[0].Pair != [2]string{drSRRec, "eye-hosp|EYE-MRN000777"} || !d.DR.Next.Overdue || d.State != "closed" || d.Pending != nil {
		t.Fatalf("untrusted later note: want one link question, still overdue until answered; got %d items %+v %+v", len(its), d.DR.Next, d.Pending)
	}
	same := untrusted
	same.Weak.Basis = "human" // the linker applies the human "same" to the pair
	if d, outs = runLung(t, base(same), 120); d.DR.Next.Overdue || !d.DR.Next.Seen || d.DR.Next.SeenReport != "EYE-dr-2" || len(linkItems(outs)) != 0 {
		t.Fatalf("after a human same: seen; got %+v", d.DR.Next)
	}
	for name, later := range map[string]DownFact{
		"dna":           drNote("EYE-dr-2", 108, "dna", "wasl", 0.8, false),
		"trusted visit": drNote("EYE-dr-2", 108, "examined_followup", "eid", 1, false),
	} {
		if _, outs = runLung(t, base(later), 120); len(linkItems(outs)) != 0 {
			t.Errorf("%s: no question expected", name)
		}
	}
	// the question is the ordinary link question: worklist-deduplicated per registration pair (-reuselink)
	w := linkWorklist{}
	w.record(its[0], &Signoff{Link: "same"})
	twin := *its[0]
	twin.LoopID = "phc-retina/ServiceRequest/PHC-sr-2"
	if !w.skip(&twin) {
		t.Fatal("a second loop's question on the answered pair must be dropped by the worklist")
	}
	b, _ := json.Marshal(d)
	if len(b) == 0 {
		t.Fatal("decision does not serialise")
	}
}

const lungDRTemplatesSHA = "fb6e71bdfe4f17dc355d68950f2cb9d3e7d2c17a31c20b8ac7801e5057b3ed60" // at dd5f208 (-progtpl)
