package main

// v6 governance layer:
//   - RBAC: every state-changing event type is checked against a role/action matrix (enforced in the linker for
//     sign-offs and consent, at outreach dispatch, and in the auditor/tamper verifier).
//   - Consent: a per-patient outreach flag (FHIR Consent, provision deny) that suppresses patient messaging but not
//     tracking (tracking runs on the public-health basis; messaging needs consent).
//   - Outreach gate: a patient message triggered by a cross-provider fact is released only if the link that carried
//     the fact is trusted (p_cal>=tau | Luhn-valid EID | human-confirmed). Below tau it is held until confirmed.
//   - Noisy, timed reviewer: error rate e per read, lognormal seconds per read, optional double review of high-risk
//     sign-offs. Deterministic in (seed, item, reader), so the live process and the offline sweep agree.
import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// ---------------- RBAC ----------------

const (
	RoleCoordinator = "coordinator" // screening coordinator / nurse navigator: consent, outreach
	RoleClinician   = "clinician"   // endoscopist / GP: evidence adequacy, histology, clinical override
	RoleReviewer    = "reviewer"    // identity reviewer (HIM / registration): merge / split
	RoleAuditor     = "auditor"     // read-only: verifies the ledger, publishes anchors, no PHI writes
	RoleSystem      = "system"      // linker / closure services
)

const (
	ActSignLink      = "signoff_link"     // confirm (merge) or reject (split) an identity link
	ActSignEvidence  = "signoff_evidence" // label a colonoscopy / pathology report
	ActRecordConsent = "record_consent"
	ActDispatch      = "dispatch_outreach"
	ActReadQueue     = "read_review_queue" // sees names, DOB, report quotes
	ActReadAudit     = "read_audit"        // sees hashes, ids, decisions; no free text
	ActAnchor        = "anchor_audit"
	ActEmitDecision  = "emit_decision"
	ActAutoLink      = "auto_link"
)

var rbac = map[string]map[string]bool{
	RoleCoordinator: {ActRecordConsent: true, ActDispatch: true, ActReadQueue: true},
	RoleClinician:   {ActSignEvidence: true, ActRecordConsent: true, ActReadQueue: true},
	RoleReviewer:    {ActSignLink: true, ActReadQueue: true},
	RoleAuditor:     {ActReadAudit: true, ActAnchor: true},
	RoleSystem:      {ActEmitDecision: true, ActAutoLink: true},
}

// RBACEnforce can be switched off only by the counterfactual audit replay ("what would forged events have done").
var RBACEnforce = true

// OutreachOn: v6 outreach intents on decisions (off = byte-identical v5 decisions, for verifying v5 runs).
var OutreachOn = true

// DualMerge: identity merges need two distinct concurring reader ids (set by -dual in every process).
var DualMerge = false

// Authorize returns nil if role may perform action (least privilege: anything not listed is denied).
func Authorize(role, action string) error {
	if !RBACEnforce || rbac[role][action] {
		return nil
	}
	return fmt.Errorf("rbac: role %q may not %s", role, action)
}

// CheckSignoff enforces RBAC (and, if dual, the two-reader rule for merges) on a sign-off before the linker applies it.
func CheckSignoff(so *Signoff, dual bool) error {
	if !RBACEnforce {
		return nil
	}
	role := so.Role // no role = denied (least privilege)
	for _, k := range so.Kinds {
		switch k {
		case "link":
			if err := Authorize(role, ActSignLink); err != nil {
				return err
			}
			if dual && so.Link == "same" && distinct(so.Reads) < 2 {
				return fmt.Errorf("rbac: merge needs 2 distinct readers, got %d", distinct(so.Reads))
			}
		case "evidence":
			if err := Authorize(role, ActSignEvidence); err != nil {
				return err
			}
		default:
			return fmt.Errorf("rbac: unknown sign-off kind %q", k)
		}
	}
	return nil
}

func distinct(xs []string) int {
	m := map[string]bool{}
	for _, x := range xs {
		m[x] = true
	}
	return len(m)
}

// ---------------- consent + outreach ----------------

const (
	SupBelowTau = "link_below_tau_unconfirmed"
	SupConsent  = "consent_withheld"
	SupSecondID = "second_identifier_unconfirmed"
	SupConsentHold = "consent_unknown_hold" // stage 3: queued until contact permission is granted
)

func outreachKind(d DownFact) string {
	switch {
	case d.RType == "Appointment" && d.Status == "booked":
		return "appointment_reminder"
	case d.RType == "Appointment" && d.Status == "noshow":
		return "rebook_after_noshow"
	case d.RType == "ServiceRequest" && d.Status == "revoked":
		return "referral_rejected_next_steps"
	}
	return ""
}

// BuildOutreach derives the patient-facing outreach intents for a loop from its linked downstream facts.
// Near-miss facts are not linked at all and never trigger a message.
func BuildOutreach(f *LoopFacts, pcal func(float64) float64) []Outreach {
	out := []Outreach{}
	seen := map[string]bool{}
	for _, d := range f.Down {
		if d.NearMiss || d.Day < f.SRDay-1 {
			continue
		}
		if d.Prog != "" && f.Prog != "" && d.Prog != f.Prog {
			continue // another programme's event (a CRC loop must not remind about the same woman's colposcopy)
		}
		k := outreachKind(d)
		if k == "" {
			continue
		}
		ev := d.Provider + "/" + d.RType + "/" + d.EventID
		if seen[ev+k] {
			continue
		}
		seen[ev+k] = true
		pc := pcal(d.Strength)
		basis := "below_tau"
		switch {
		case d.Weak.Basis == "human":
			basis = "human"
		case d.Weak.Basis == "eid":
			basis = "eid"
		case pc >= TauHigh:
			basis = "p_cal"
		}
		if V7Cal != nil {
			pc = d.PCal
			switch {
			case d.AllHuman:
				basis = "human"
			case pc >= TauHigh && d.Weak.Basis == "eid":
				basis = "eid"
			case pc >= TauHigh:
				basis = "p_cal"
			default:
				basis = "below_tau"
			}
		}
		o := Outreach{Kind: k, Event: ev, Basis: basis, PCal: round4(pc)}
		if V7Guard {
			o.Pair = [2]string{f.SRRec, d.RecPk}
		}
		switch {
		case basis == "below_tau" && V7Guard && V7Cal != nil && !d.NearMiss && pc >= TauClose && !msgConfirmed(f, f.SRRec, d.RecPk) && !d.IDConfirmed:
			o.Suppress, o.Guard = SupSecondID, "held_needs_human"
		case basis == "below_tau" && V7Guard && V7Cal != nil && !d.NearMiss && pc >= TauClose:
			o.Released, o.Guard = !f.MsgOptOut, "human_confirmed"
			if f.MsgOptOut {
				o.Suppress = SupConsent
			} else if ConsentHold && f.Contact != "granted" {
				o.Released = false
				if f.Contact == "unknown" || f.Contact == "" {
					o.Suppress, o.Guard = SupConsentHold, "held_until_consent"
				} else {
					o.Suppress = SupConsent
				}
			}
		case basis == "below_tau":
			o.Suppress = SupBelowTau
		case f.MsgOptOut:
			o.Suppress = SupConsent
		case ConsentHold && f.Contact != "granted":
			if f.Contact == "unknown" || f.Contact == "" {
				o.Suppress, o.Guard = SupConsentHold, "held_until_consent"
			} else {
				o.Suppress = SupConsent
			}
		case V7Guard && basis != "human" && !d.AllHuman && !secondID(f.SRDob, d.RecDob) && !d.EIDSecond && !msgConfirmed(f, f.SRRec, d.RecPk) && !d.IDConfirmed:
			o.Suppress, o.Guard = SupSecondID, "held"
		default:
			o.Released = true
			if V7Guard {
				o.Guard = "dob_match"
				if basis == "human" || d.AllHuman {
					o.Guard = "human_path"
				} else if !secondID(f.SRDob, d.RecDob) && d.EIDSecond {
					o.Guard = "eid_match"
				} else if !secondID(f.SRDob, d.RecDob) {
					o.Guard = "human_confirmed"
				}
			}
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Event+out[i].Kind < out[j].Event+out[j].Kind })
	return out
}

// Dispatch is the single exit point to the messaging gateway: RBAC + gate + consent, re-checked at send time.
func Dispatch(role string, o Outreach, optOut bool) error {
	return DispatchC(role, o, optOut, "granted")
}

// DispatchC is Dispatch with the stage-3 invariant: a message may leave only if contact permission is granted *now*.
func DispatchC(role string, o Outreach, optOut bool, contact string) error {
	if err := Authorize(role, ActDispatch); err != nil {
		return err
	}
	if o.Basis == "below_tau" {
		return fmt.Errorf("gate: %s held until link confirmed", o.Event)
	}
	if o.Suppress == SupSecondID {
		return fmt.Errorf("guard: %s held until a second identifier or a human confirms the link", o.Event)
	}
	if optOut {
		return fmt.Errorf("consent: patient opted out of outreach")
	}
	if ConsentHold && contact != "granted" {
		return fmt.Errorf("consent: contact permission is %s (dispatch requires granted)", contact)
	}
	return nil
}

// ConsentHold (stage 3, default on): outreach needs an explicit granted contact permission at dispatch time;
// unknown -> held (queued, re-evaluated when consent arrives), denied/opted_out -> cancelled.
var ConsentHold = true

// ---------------- noisy, timed reviewer ----------------

type ReviewerCfg struct {
	Err      float64 // per-read error probability
	Rho      float64 // H7: P(a later read repeats the first read's outcome, right or wrong) instead of an independent draw
	Double   string  // none | merge | merge+close
	Seed     int64
	LinkMedS float64 // median seconds per link read
	EvMedS   float64 // median seconds per evidence read
	Sigma    float64 // lognormal sigma
	// H7 mitigations (all off by default; with all three off every read is byte-identical to the pre-mitigation model).
	// Correlation model, where Rho is the current design (reader 2 sees reader 1's answer, fixed reader pairs):
	//   effective rho = Rho * (1 - BlindFactor if Blind) * (1 - RotateFactor if Rotate)
	// Blind: the second read does not see the first. BlindFactor is the share of Rho that comes from seeing the first
	// answer (anchoring); the rest (items hard for anyone, shared habits) survives blinding.
	// Rotate: reader pairs rotate instead of staying fixed. Of the correlation left after blinding, a pair-specific part
	// (two fixed colleagues converging on the same habits) is broken by rotation; the item-specific part (an item that
	// is hard for anyone) is not. RotateFactor is that pair-specific share. Without Blind it applies to all of Rho.
	// The same u01 draw decides "copies the first read" in every variant, so a mitigated run copies on a subset of the
	// items the unmitigated run copies on (common random numbers: the comparison is paired, not resampled).
	Blind        bool
	BlindFactor  float64 // share of Rho removed by blinding (evidence-informed default 0.5, range 0.15-0.85)
	Rotate       bool
	RotateFactor float64 // share of the remaining correlation removed by pair rotation (assumption 0.1, range 0-0.3)
	// SeniorAudit: fraction p of signed closing answers (evidence adequate/histology, lung closing answers) that get one
	// extra senior read, chosen by a seeded hash of the item id (replay-identical). The senior read never copies r1/r2
	// and has error rate SeniorErrRel*Err. If it disagrees, the signed answer becomes the senior answer.
	SeniorAudit  float64
	SeniorErrRel float64 // senior per-read error rate relative to Err (assumption 0.5, range 0.25-0.8)
}

var DefaultReviewer = ReviewerCfg{Err: 0, Double: "none", Seed: 1, LinkMedS: 45, EvMedS: 90, Sigma: 0.6,
	BlindFactor: 0.5, RotateFactor: 0.1, SeniorErrRel: 0.5}

// effRho is the probability that a later read copies the first read, after the enabled mitigations.
func (c ReviewerCfg) effRho() float64 {
	r := c.Rho
	if c.Blind {
		r *= 1 - c.BlindFactor
	}
	if c.Rotate {
		r *= 1 - c.RotateFactor
	}
	return r
}

func u01(seed int64, parts ...string) float64 {
	h := sha256.New()
	fmt.Fprint(h, seed)
	for _, p := range parts {
		h.Write([]byte("|" + p))
	}
	s := h.Sum(nil)
	return (float64(binary.BigEndian.Uint64(s[:8])>>11) + 0.5) / float64(uint64(1)<<53)
}

func lognormSecs(med, sigma, u1, u2 float64) float64 {
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return med * math.Exp(sigma*z)
}

var evAlt = map[string][]string{
	"adequate":           {"inadequate", "awaiting_histology"},
	"inadequate":         {"adequate", "awaiting_histology"},
	"awaiting_histology": {"adequate", "inadequate"},
	"histology":          {"no_histology"},
	"no_histology":       {"histology"},
}

type read struct {
	reader string
	link   string
	ev     string
	secs   float64
}

func (c ReviewerCfg) oneRead(it *ReviewItem, tr *Truth, reader string) read {
	r := read{reader: reader}
	kind := it.Kinds[0]
	draw := reader // correlated reads (H7): with probability effRho a later reader shares the first reader's error draw
	if rho := c.effRho(); reader != "r1" && reader != "s1" && rho > 0 && u01(c.Seed, it.ItemID, reader, "corr") < rho {
		draw = "r1"
	}
	e := c.errFor(it)
	if reader == "s1" { // senior audit read: independent draw, its own (lower) assumed error rate
		e *= c.SeniorErrRel
	}
	wrong := u01(c.Seed, it.ItemID, draw, "err") < e
	med := c.EvMedS
	if kind == "evidence" && it.Reason == "prep_grade_one_click" {
		med = OneClickMedS
	}
	if kind == "link" {
		med = c.LinkMedS
		same := tr.PKQid[it.Pair[0]] != "" && tr.PKQid[it.Pair[0]] == tr.PKQid[it.Pair[1]]
		if wrong {
			same = !same
		}
		r.link = map[bool]string{true: "same", false: "different"}[same]
	} else {
		t := tr.Reports[it.ReportID]
		if t == "" {
			t = "inadequate"
		}
		if wrong {
			alts := evAlt[t]
			if alts == nil {
				alts = lungEvAlt(t) // lung-nodule labels (lung.go)
			}
			if len(alts) > 0 {
				t = alts[int(u01(c.Seed, it.ItemID, draw, "alt")*float64(len(alts)))]
			}
		}
		r.ev = t
	}
	r.secs = lognormSecs(med, c.Sigma, u01(c.Seed, it.ItemID, reader, "t1"), u01(c.Seed, it.ItemID, reader, "t2"))
	return r
}

func (c ReviewerCfg) highRisk(it *ReviewItem, r read) bool {
	if (c.Double == "merge+close" || c.Double == "merge+close+g1" || c.Double == "close+guard") && lungClosingAnswer(r.ev) { // lung-nodule closing answers get the same double read
		return true
	}
	switch c.Double {
	case "merge":
		return r.link == "same"
	case "merge+close":
		return r.link == "same" || r.ev == "adequate" || r.ev == "histology"
	case "merge+close+g1": // v7: as merge+close, but messaging-only guard confirmations get a single read
		return (r.link == "same" && it.Reason != "second_identifier") || r.ev == "adequate" || r.ev == "histology"
	case "close+guard": // v7: double-read closures and messaging confirmations; identity merges single-read
		return (r.link == "same" && it.Reason == "second_identifier") || r.ev == "adequate" || r.ev == "histology"
	}
	return false
}

// Review produces the sign-off for one item: one read, a second independent read for high-risk answers, and a
// third (senior) read only if the first two disagree. Role follows the item kind (RBAC).
func (c ReviewerCfg) Review(it *ReviewItem, tr *Truth) *Signoff {
	so := &Signoff{ItemID: it.ItemID, LoopID: it.LoopID, Kinds: it.Kinds, Pair: it.Pair, ReportID: it.ReportID, SignedHead: it.Head, Purpose: it.Reason}
	so.Role = RoleReviewer
	if it.Kinds[0] == "evidence" {
		so.Role = RoleClinician
	}
	r1 := c.oneRead(it, tr, "r1")
	final, reads, secs := r1, []string{"r1"}, r1.secs
	if c.highRisk(it, r1) {
		r2 := c.oneRead(it, tr, "r2")
		reads, secs = append(reads, "r2"), secs+r2.secs
		if r2.link != r1.link || r2.ev != r1.ev {
			r3 := c.oneRead(it, tr, "r3")
			reads, secs, final = append(reads, "r3"), secs+r3.secs, r3 // senior adjudicator breaks the tie
		}
	}
	if c.SeniorAudit > 0 && closingAnswer(final) && u01(c.Seed, it.ItemID, "audit") < c.SeniorAudit {
		// Senior audit of a signed closing answer. Overriding the answer (rather than re-opening the item) keeps the
		// one-sign-off-per-item model: in the offline replay the sign-off sits where the item was raised, so the loop
		// ends in the state a re-opened and re-decided loop would reach. A real weekly audit corrects it days later.
		s := c.oneRead(it, tr, "s1")
		so.audit = &auditRec{prevLink: final.link, prevEv: final.ev, disagree: s.link != final.link || s.ev != final.ev}
		reads, secs = append(reads, "s1"), secs+s.secs
		if so.audit.disagree {
			final = s
		}
	}
	so.Link, so.Evidence, so.Reads, so.DurationS = final.link, final.ev, reads, math.Round(secs*10)/10
	so.Reviewer = fmt.Sprintf("sim-%s e=%.3f double=%s reads=%v", so.Role, c.Err, c.Double, reads)
	if c.Rho > 0 {
		so.Reviewer += fmt.Sprintf(" rho=%.2f", c.Rho)
	}
	if c.Blind {
		so.Reviewer += fmt.Sprintf(" blind=%.2f", c.BlindFactor)
	}
	if c.Rotate {
		so.Reviewer += fmt.Sprintf(" rotate=%.2f", c.RotateFactor)
	}
	if c.SeniorAudit > 0 {
		so.Reviewer += fmt.Sprintf(" audit=%.3f", c.SeniorAudit)
	}
	return so
}

// auditRec records a senior audit on a sign-off (in memory only, never serialised): the answer before the audit and
// whether the senior read disagreed. The sweep scores both against the truth.
type auditRec struct {
	prevLink, prevEv string
	disagree         bool
}

// closingAnswer: a signed evidence answer that lets a loop close (the audit sample frame).
func closingAnswer(r read) bool {
	return r.ev == "adequate" || r.ev == "histology" || lungClosingAnswer(r.ev)
}

// idWorklist: the review worklist asks one second-identifier question per registration pair (ReuseIDConfirm). Two
// loops of one patient raise their guard items together, so the second is queued before the first answer lands;
// once the pair is answered "same", the queued twin is dropped (the linker already applies the answer to every loop
// of that pair). A "different" answer is not reused: the twin is still asked.
type idWorklist map[[2]string]bool

func pairKey(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

func (w idWorklist) skip(it *ReviewItem) bool {
	return ReuseIDConfirm && it.Reason == "second_identifier" && w[pairKey(it.Pair[0], it.Pair[1])]
}

func (w idWorklist) record(it *ReviewItem, so *Signoff) {
	if it.Reason != "second_identifier" {
		return
	}
	if k := pairKey(it.Pair[0], it.Pair[1]); so.Link == "same" {
		w[k] = true
	} else if so.Link == "different" {
		delete(w, k)
	}
}

// msgConfirmed (v7): a human confirmed (second-identifier item answered "same") that the downstream registration is this patient.
func msgConfirmed(f *LoopFacts, a, b string) bool {
	for _, s := range f.Signoffs {
		if s.Purpose == "second_identifier" && s.Link == "same" && s.Pair[0] == a && s.Pair[1] == b {
			return true
		}
	}
	return false
}
