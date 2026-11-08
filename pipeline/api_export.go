package main

// Workflow E: case evidence export (docs/API-EXPORT.md).
//
//   GET /api/cases/{id}/export  ->  {"schema":"wasl-case-export/1","meta":...,"detail":...,"audit":...,"content_sha256":...}
//
// The export composes the same case detail and audit projections that GET /api/cases/{id} and
// GET /api/cases/{id}/audit serve. It is read-only: nothing is written to the run directory, no timestamp is added
// to the document, and content_sha256 = sha(canon(document without content_sha256)) authenticates the document
// contents. It does not identify the export across restarts (meta.loaded_at changes when the server reloads).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const exportSchema = "wasl-case-export/1"

// caseDetailBody is the body of GET /api/cases/{id} (without the meta envelope). It must stay identical to the
// projection in caseDetail (api.go); api_export_test.go pins the two against each other.
func (s *apiServer) caseDetailBody(c *apiCase) map[string]interface{} {
	ev := []map[string]interface{}{}
	links := []map[string]interface{}{}
	holds := []map[string]interface{}{}
	seen := map[string]bool{}
	for _, d := range c.decisions {
		for _, b := range d.Chain {
			key := b.Hash
			if seen[key] {
				continue
			}
			seen[key] = true
			switch b.Type {
			case "evidence":
				ev = append(ev, b.Body)
			case "link_decision":
				links = append(links, b.Body)
			case "hold_decision":
				holds = append(holds, b.Body)
			}
		}
	}
	hist := []map[string]interface{}{}
	for _, d := range c.decisions {
		hist = append(hist, map[string]interface{}{"decision_id": d.ID, "in_seq": d.InSeq, "state": d.State, "dropoff": d.Dropoff, "owner": d.Owner,
			"overdue": d.Overdue, "escalations": nz(d.Escalations), "consent_log": nz(d.ConsentLog), "outreach": nz(d.Outreach), "policy": d.Policy, "sig": d.Sig})
	}
	return map[string]interface{}{
		"case": s.summary(c), "identities": nz(s.ids[c.ID]), "timeline": nz(s.facts[c.ID]), "links": links, "evidence": ev, "holds": holds,
		"outreach": nz(c.last.Outreach), "decisions": hist, "review_items": nz(s.queue[c.ID]), "signoffs": nz(s.signoffs[c.ID]),
		"api_reviews": s.reviewsFor(c.ID), "api_overlay": s.overlayView(c, true), "review_ledger": s.ledgerView(), "current": c.last, "booking": s.bookingFor(c.ID),
		"agent_investigations": s.agentFor(c.ID), "agent_provenance": s.agentProvenance(),
	}
}

// caseAuditBody is the body of GET /api/cases/{id}/audit (without the meta envelope). It must stay identical to the
// projection in caseAudit (api.go); api_export_test.go pins the two against each other.
func (s *apiServer) caseAuditBody(c *apiCase) map[string]interface{} {
	chain := c.last.Chain
	return map[string]interface{}{
		"case": c.ID, "chain": nz(chain), "chain_verified": verifyChain(chain), "chain_length": len(chain),
		"anchors": s.anchors, "anchor_note": auditAnchorNote,
		"decision_signatures": func() []map[string]string {
			out := []map[string]string{}
			for _, d := range c.decisions {
				out = append(out, map[string]string{"decision_id": d.ID, "sig": d.Sig, "recomputed": decSig(d), "policy": d.Policy})
			}
			return out
		}(),
		"history": s.reviewsFor(c.ID),
	}
}

// ---- redaction -------------------------------------------------------------------------------------------------

// Operational fields on server-generated paths carry machine codes such as prep_grade_one_click or
// second_identifier. User-controlled coordinator/snooze reasons are not present in those paths' allowlists.
var exportOpCode = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*(?:[+/.:-][a-z0-9_]+)*$`)

// exportStaticText lists fixed sentences the server itself writes (no case data in them).
var exportStaticText = map[string]bool{
	"case does not need a colonoscopy slot at the end of the run": true,
}

func exportSafeOpValue(v string) bool { return exportStaticText[v] || exportOpCode.MatchString(v) }

// ---- allowlisted projection ------------------------------------------------------------------------------------
//
// Evidence bodies, timeline rows and citations are projected through an allowlist: a key that is not listed is
// dropped whatever its value, so a one- or two-word clinical phrase under an unknown key cannot leak. Listed keys
// still pass through the scalar guard (bounded length, no prose, no newlines).

type exportAllow struct {
	leaf      bool                    // a scalar, or a list of scalars, is kept
	op        bool                    // string leaves must be server operational codes or fixed static text
	items     *exportAllow            // an array of objects projected with this schema
	chain     bool                    // an array of hash-chain blocks, projected according to block type
	citations bool                    // an array of citation objects
	kids      map[string]*exportAllow // keys kept when the value is an object; "*" requires keyOK
	keyOK     *regexp.Regexp          // allowed dynamic-map keys; never used for ordinary object fields
}

var exportLeaf = &exportAllow{leaf: true}
var exportOperationalLeaf = &exportAllow{leaf: true, op: true}

func exportScalarOK(v interface{}, op bool) bool {
	switch x := v.(type) {
	case string:
		if op {
			return exportSafeOpValue(x)
		}
		return exportSafeValue.MatchString(x)
	case bool, nil, json.Number, float64:
		return true
	}
	return false
}

func exportObject(kids map[string]*exportAllow) *exportAllow { return &exportAllow{kids: kids} }
func exportList(item *exportAllow) *exportAllow              { return &exportAllow{items: item} }

func exportProject(v interface{}, a *exportAllow) (interface{}, bool) {
	if a == nil {
		return nil, false
	}
	if a.chain {
		return exportChainList(v), true
	}
	if a.citations {
		return exportCitations(v), true
	}
	switch x := v.(type) {
	case map[string]interface{}:
		if a.kids == nil {
			return nil, false
		}
		out := map[string]interface{}{}
		for k, val := range x {
			lk := strings.ToLower(k)
			sub := a.kids[lk]
			if sub == nil {
				sub = a.kids["*"]
				if sub == nil || a.keyOK == nil || !a.keyOK.MatchString(lk) {
					continue
				}
			}
			if r, keep := exportProject(val, sub); keep {
				out[k] = r
			}
		}
		return out, true
	case []interface{}:
		if a.items != nil {
			out := []interface{}{}
			for _, val := range x {
				if r, keep := exportProject(val, a.items); keep {
					out = append(out, r)
				}
			}
			return out, true
		}
		if !a.leaf {
			return nil, false
		}
		out := []interface{}{}
		for _, val := range x {
			if exportScalarOK(val, a.op) {
				out = append(out, val)
			}
		}
		return out, true
	default:
		if a.leaf && exportScalarOK(v, a.op) {
			return v, true
		}
	}
	return nil, false
}

func exportKeys(keys ...string) map[string]*exportAllow {
	m := map[string]*exportAllow{}
	for _, k := range keys {
		m[k] = exportLeaf
	}
	return m
}

// exportReaderFields is the structured output of one reader (colonoscopy label fields, DR grades, BI-RADS, Lung-RADS,
// nodule measurements). Its quote is not listed and so never passes.
var exportReaderFields = &exportAllow{kids: func() map[string]*exportAllow {
	m := exportKeys("r_right", "r_left", "m_right", "m_left", "urgent", "attended", "fundus", "outcome", "review_days", "label",
		"birads_right", "birads_left", "birads_overall", "assessment", "biopsy",
		"type", "long_mm", "short_mm", "count", "solid_mm", "prior_long_mm", "prior_short_mm", "resolved", "not_assessable", "stated", "mean_mm",
		"lungrads_overall", "lungrads_worst", "workup", "mdt", "tissue")
	m["status"] = exportObject(exportKeys("bp", "glyc", "lipid", "weight"))
	return m
}()}

var exportEvidenceAllow = &exportAllow{kids: func() map[string]*exportAllow {
	m := exportKeys("report", "human_override", "regex", "gemma", "agree", "label", "reader_model", "rule", "flags",
		"reader_a_regex", "reader_b", "reader_a", "reader_a_fields", "reader_b_fields")
	m["reader_a_fields"], m["reader_b_fields"] = exportReaderFields, exportReaderFields
	m["fields"] = &exportAllow{kids: exportKeys("regex", "reader")}
	m["image_reader"] = &exportAllow{kids: exportKeys("model", "photos", "caecal_photo", "landmark_p", "prep", "prep_p", "unavailable",
		"photo_shas", "label_before_image", "hint")}
	return m
}()}

var exportTimelineAllow = &exportAllow{kids: exportKeys("seq", "type", "id", "provider", "status", "day", "arrival_day", "category", "raw_sha", "source")}

var exportCitationAllow = &exportAllow{kids: exportKeys("tool", "arg", "field", "value")}

var exportSafeValue = regexp.MustCompile(`^[A-Za-z0-9_.:/|@=+#%-]{1,80}$`)

// exportCitations keeps each citation's tool, arg, field and value and drops every other key (quote, excerpt and
// any report-derived text). A string value is kept only when it is a single identifier-like token or date.
func exportCitations(v interface{}) interface{} {
	list, ok := v.([]interface{})
	if !ok {
		return []interface{}{}
	}
	out := []interface{}{}
	for _, c := range list {
		pm, keep := exportProject(c, exportCitationAllow)
		if !keep {
			continue
		}
		cm := pm.(map[string]interface{})
		if sv, isStr := cm["value"].(string); isStr && !exportSafeValue.MatchString(sv) {
			delete(cm, "value")
		}
		out = append(out, cm)
	}
	return out
}

// ---- remaining detail/audit schemas -----------------------------------------------------------------------------
//
// These schemas deliberately enumerate the served operational contract rather than trying to recognise clinical
// prose. Ordinary strings must be single structural tokens; operational reason fields use the stricter machine-code
// shape. Dynamic maps list their domain keys explicitly, except audit anchors whose partition-subject keys are
// constrained by keyOK.

var exportDomainStatusAllow = exportObject(exportKeys("bp", "glyc", "lipid", "weight"))
var exportDomainDaysAllow = exportObject(exportKeys("bp", "glyc", "lipid", "weight"))

var exportLungSurvAllow = func() *exportAllow {
	a := &exportAllow{}
	a.kids = exportKeys("loop", "from_report", "from_day", "window_start_day", "due_day", "state", "ct_report", "ct_day", "basis", "round")
	a.kids["reason"] = exportOperationalLeaf
	a.kids["next"] = a
	return a
}()

var exportLungDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("finding_report", "finding_label", "risk", "risk_recorded", "window_start_day", "window_end_day", "outcome",
		"followup_report", "within_window", "followup_ct_seen", "closer", "tissue_due_day", "scope")
	m["rec"] = exportObject(exportKeys("action", "win_lo_days", "win_hi_days", "next_step", "optional", "cell", "next_lo_days", "next_hi_days", "next_from", "next_optional"))
	m["surveillance"] = exportLungSurvAllow
	return m
}())

var exportDRDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("screen_report", "screen_label", "coded_grade", "grade_day", "deadline_day", "clinic_note", "outcome", "examined", "within_deadline")
	m["rec"] = exportObject(exportKeys("action", "deadline_days", "urgency", "cell"))
	m["next_due"] = exportObject(exportKeys("kind", "from_day", "interval_days", "due_day", "basis", "seen", "seen_day", "seen_report", "recall_overdue"))
	return m
}())

var exportBreastDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("screen_report", "screen_label", "coded_birads", "screen_day", "deadline_day", "assessment_document", "outcome", "within_deadline", "treatment_referral_due_day")
	m["rec"] = exportObject(exportKeys("action", "deadline_days", "urgency", "cell"))
	return m
}())

var exportLDCTDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("scan_report", "scan_label", "coded_lungrads", "scan_day", "due_day", "deadline_day", "followup_report",
		"followup_label", "followup_next_step", "workup_document", "outcome", "within_deadline")
	m["rec"] = exportObject(exportKeys("action", "due_days", "deadline_days", "urgency", "cell", "next_step"))
	return m
}())

var exportIfhasDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("screen_report", "screen_label", "coded_results_agree", "screen_day", "deadline_day", "followup_documents", "outcome", "within_deadline", "next_step")
	m["rec"] = exportObject(func() map[string]*exportAllow {
		r := exportKeys("action", "deadline_days", "urgency", "cell", "abnormal_domains", "prediabetes")
		r["domain_deadline_days"] = exportDomainDaysAllow
		return r
	}())
	m["domain_status"] = exportDomainStatusAllow
	m["resolved_day"] = exportDomainDaysAllow
	return m
}())

var exportHandoffAllow = exportObject(exportKeys("referral_id", "decision", "reason_code", "advice_ref", "info_requested", "clinical_priority",
	"received_at", "decided_at", "decided_by", "patient_informed", "outreach_consent_status", "act_by", "medical_director_day", "acted_at", "status"))

var exportOutreachAllow = exportObject(exportKeys("kind", "ev", "basis", "p_cal", "released", "suppress", "guard", "pair", "channel", "sender", "tpl"))
var exportConsentTransitionAllow = exportObject(exportKeys("ev", "kind", "from", "to", "contact"))

var exportSnoozeAllow = exportObject(exportKeys("at", "event", "until"))
var exportContactOutcomeAllow = exportObject(exportKeys("at", "event", "outcome", "channel"))
var exportOverlayAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("head", "actions", "last_action_at", "ledger_status")
	m["snooze"] = exportSnoozeAllow
	m["contact_outcomes"] = exportList(exportContactOutcomeAllow)
	return m
}())

var exportCoordinatorDataAllow = exportObject(exportKeys("until", "outcome", "channel"))
var exportReviewEventAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("case", "item", "decision", "role", "action", "at", "head_signed", "mode", "prev", "hash", "type", "kind",
		"head", "seq", "target", "target_action", "resolution", "action_head")
	m["data"] = exportCoordinatorDataAllow
	return m
}())

var exportSignoffAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("item", "loop", "kinds", "pair", "report", "link", "evidence", "reviewer", "signed_head", "run", "role", "reads",
		"dur_s", "purpose", "hold", "agent_read", "agent_s", "wrong", "raw_id", "quoted_by", "raw_sha")
	m["hint"] = exportOperationalLeaf
	m["why"] = exportOperationalLeaf
	return m
}())

var exportLinkAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("loop", "referral_patient", "downstream_event", "path_strength", "path_p_cal", "tau", "hops", "near_miss", "linker", "eid_contra", "review_pair_substituted")
	m["weakest_edge"] = exportObject(exportKeys("a", "b", "p", "basis"))
	return m
}())

var exportHoldAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("loop", "state", "programme", "rule", "policy", "report", "age", "scope", "label", "due_day", "deadline_day")
	m["reason"] = exportOperationalLeaf
	m["why"] = exportOperationalLeaf
	m["domain_status"] = exportDomainStatusAllow
	return m
}())

var exportReviewItemAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("item", "loop", "kinds", "pair", "p", "report", "verdict", "conf", "head", "in_seq", "overdue", "day")
	m["reason"] = exportOperationalLeaf
	m["chain"] = &exportAllow{chain: true}
	return m
}())

var exportAgentInvestigationAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("item", "pair", "label", "recommendation")
	m["citations"] = &exportAllow{citations: true}
	m["human"] = exportObject(exportKeys("link", "reads", "role"))
	return m
}())
var exportAgentProvenanceAllow = exportObject(exportKeys("cached", "experimental", "model", "schema_ver"))

var exportBookingAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("status", "data_mode", "decision_support_only", "procedure", "repeat_after_unsatisfactory", "requested_day", "window_days",
		"rule", "region", "state_at_run_end", "within_window", "cross_region", "wait_days", "target_days", "capacity_condition", "pooling_relevant")
	m["reason"] = exportOperationalLeaf
	m["constraints"] = exportObject(exportKeys("female_endoscopist", "arabic_only", "region", "travel_ok", "after_hours", "avoid_ramadan", "prior_no_show"))
	m["booking"] = exportObject(exportKeys("loop_id", "session_id", "unit", "day", "endoscopist_female", "overbooked", "predicted_no_show",
		"nurse_call", "prep_send_day", "deadline", "within_deadline", "booked_on"))
	m["pathway"] = exportObject(exportKeys("route"))
	m["prep_risk"] = exportObject(func() map[string]*exportAllow {
		p := exportKeys("score", "band", "factors", "requires_clinician_order")
		p["suggested_intervention"] = exportObject(exportKeys("code"))
		return p
	}())
	m["app9"] = exportObject(exportKeys("anticoagulant", "diabetes", "insulin", "device", "prior_abdominal_surgery", "tricyclic", "opioid",
		"constipation", "prior_colonoscopy", "prior_inadequate_prep", "hospitalised", "asa_class", "data_mode"))
	return m
}())

var exportCaseAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("id", "programme", "state", "dropoff", "provider", "referral_day", "deadline_day", "overdue", "priority", "consent",
		"review", "closed", "closed_day", "fixture", "decisions")
	m["api_overlay"] = exportOverlayAllow
	m["lung"], m["dr"], m["breast"], m["ldct"], m["ifhas"] = exportLungDecisionAllow, exportDRDecisionAllow, exportBreastDecisionAllow, exportLDCTDecisionAllow, exportIfhasDecisionAllow
	return m
}())

var exportDecisionAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("id", "loop", "in_seq", "prov", "sr_day", "now", "state", "overdue", "unowned", "reported_closed", "closed_day",
		"closing_report", "pending", "notif_day", "ack_day", "ref_status", "ref_day", "comm_flags", "dropoff", "human_assisted", "prog", "prio",
		"deadline", "proc_day", "ec_state", "ec_turns", "tb_state", "prep_q", "policy", "contact", "sig", "audit_reopened")
	m["esc"] = exportOperationalLeaf
	m["chain"] = &exportAllow{chain: true}
	m["consent_log"] = exportList(exportConsentTransitionAllow)
	m["outreach"] = exportList(exportOutreachAllow)
	m["lung"], m["dr"], m["breast"], m["ldct"], m["ifhas"] = exportLungDecisionAllow, exportDRDecisionAllow, exportBreastDecisionAllow, exportLDCTDecisionAllow, exportIfhasDecisionAllow
	m["handoff"] = exportHandoffAllow
	return m
}())

var exportDecisionHistoryAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("decision_id", "in_seq", "state", "dropoff", "overdue", "policy", "sig")
	m["escalations"] = exportOperationalLeaf
	m["consent_log"] = exportList(exportConsentTransitionAllow)
	m["outreach"] = exportList(exportOutreachAllow)
	return m
}())

var exportContactOverlayAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("status", "demo_pending_consent", "events", "contact_blocked", "mode", "canonical_consent", "canonical_unchanged", "last_event_hash")
	m["history"] = exportList(exportReviewEventAllow)
	return m
}())

var exportDetailAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys()
	m["review_ledger"] = exportObject(exportKeys("status", "events_applied"))
	m["case"] = exportCaseAllow
	m["identities"] = exportList(exportObject(exportKeys("registration", "provider", "role")))
	m["timeline"] = exportList(exportTimelineAllow)
	m["links"] = exportList(exportLinkAllow)
	m["evidence"] = exportList(exportEvidenceAllow)
	m["holds"] = exportList(exportHoldAllow)
	m["outreach"] = exportList(exportOutreachAllow)
	m["decisions"] = exportList(exportDecisionHistoryAllow)
	m["review_items"] = exportList(exportReviewItemAllow)
	m["signoffs"] = exportList(exportSignoffAllow)
	m["api_reviews"] = exportList(exportReviewEventAllow)
	m["api_overlay"] = exportOverlayAllow
	m["current"] = exportDecisionAllow
	m["booking"] = exportBookingAllow
	m["agent_investigations"] = exportList(exportAgentInvestigationAllow)
	m["agent_provenance"] = exportAgentProvenanceAllow
	m["api_contact"] = exportObject(map[string]*exportAllow{"overlay": exportContactOverlayAllow, "history": exportList(exportReviewEventAllow)})
	return m
}())

var exportAuditAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("case", "chain_verified", "chain_length")
	m["chain"] = &exportAllow{chain: true}
	m["anchors"] = &exportAllow{kids: map[string]*exportAllow{"*": exportLeaf}, keyOK: regexp.MustCompile(`^(?:fhir|out\.p[0-9]+)$`)}
	m["decision_signatures"] = exportList(exportObject(exportKeys("decision_id", "sig", "recomputed", "policy")))
	m["history"] = exportList(exportReviewEventAllow)
	return m
}())

var exportClosureAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("loop", "state", "programme", "priority", "closed_day", "sla_days", "within_sla", "policy",
		"rule", "report", "outcome", "window", "within_window", "next_step", "closer", "closed_into", "surveillance_loop", "surveillance_ct_due_day",
		"grade_day", "screen_day", "scan_day", "deadline_day", "due_day", "within_deadline", "followup_report", "followup_label", "followup_next_step",
		"screen_report", "scan_report", "assessment_document", "workup_document")
	m["domain_status"] = exportDomainStatusAllow
	m["resolved_day"] = exportDomainDaysAllow
	return m
}())

var exportRecommendationAllow = exportObject(exportKeys("rule", "cell", "action", "deadline_days", "due_days", "urgency", "window", "next_step", "optional"))
var exportAutoAuditAllow = exportObject(exportKeys("loop", "report", "item", "p", "status", "auto_verdict", "audit_answer", "result"))
var exportEscalationAllow = exportObject(exportKeys("loop", "state", "programme", "rule", "outcome", "to", "day", "basis"))
var exportSurveillanceOpenedAllow = exportObject(func() map[string]*exportAllow {
	m := exportKeys("loop", "parent", "parent_head", "parent_closing_report", "rule", "cell", "next_step", "optional", "from", "window", "round", "basis")
	m["reason"] = exportOperationalLeaf
	return m
}())
var exportOutreachGuardAllow = exportObject(exportKeys("loop", "pair", "event", "basis", "p_cal", "rule", "contradiction"))
var exportBlockShellAllow = exportObject(exportKeys("type", "prev", "hash"))

func exportChainBodyAllow(blockType string) *exportAllow {
	switch blockType {
	case "evidence":
		return exportEvidenceAllow
	case "link_decision":
		return exportLinkAllow
	case "hold_decision":
		return exportHoldAllow
	case "closure_decision":
		return exportClosureAllow
	case "recommendation":
		return exportRecommendationAllow
	case "human_signoff":
		return exportSignoffAllow
	case "auto_audit":
		return exportAutoAuditAllow
	case "escalation_decision":
		return exportEscalationAllow
	case "surveillance_opened":
		return exportSurveillanceOpenedAllow
	case "outreach_guard":
		return exportOutreachGuardAllow
	default:
		return exportObject(map[string]*exportAllow{})
	}
}

func exportChainBlock(v interface{}) (interface{}, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	out, _ := exportProject(m, exportBlockShellAllow)
	block := out.(map[string]interface{})
	blockType, _ := m["type"].(string)
	body, keep := exportProject(m["body"], exportChainBodyAllow(blockType))
	if !keep {
		body = map[string]interface{}{}
	}
	block["body"] = body
	return block, true
}

func exportChainList(v interface{}) interface{} {
	list, ok := v.([]interface{})
	if !ok {
		return []interface{}{}
	}
	out := []interface{}{}
	for _, item := range list {
		if projected, keep := exportChainBlock(item); keep {
			out = append(out, projected)
		}
	}
	return out
}

// redactExport projects the audit body through its default-deny schema. It never mutates its input.
func redactExport(v interface{}) (interface{}, bool) {
	return exportProject(v, exportAuditAllow)
}

// redactExportDetail projects every detail section, including nested chains and coordinator/consent overlays. There is
// intentionally no generic fallback for a future section or field: it must be added to the relevant schema first.
func redactExportDetail(det map[string]interface{}) map[string]interface{} {
	r, keep := exportProject(det, exportDetailAllow)
	if !keep {
		return map[string]interface{}{}
	}
	return r.(map[string]interface{})
}

// exportGeneric deep-copies any value into generic JSON via a marshal/unmarshal round trip (so nothing aliases the
// live server state). Numbers are kept as json.Number so their literal text is not reformatted through float64.
func exportGeneric(v interface{}) (map[string]interface{}, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out map[string]interface{}
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

const exportChainNote = "Block bodies in this export are redacted copies. Block hashes were computed over the original bodies and " +
	"cannot be recomputed from the served bodies; chain_verified was computed server-side over the original, unredacted " +
	"current chain and covers that chain only."

// caseExportDoc builds the export document for one case. Two calls return identical bytes while the run and the
// case/review history are unchanged. detail and audit are deep copies; the live bodies and chains are never touched.
func (s *apiServer) caseExportDoc(c *apiCase) (map[string]interface{}, error) {
	// computed from the original chain, before any redaction
	verified, length := verifyChain(c.last.Chain), len(c.last.Chain)
	liveAudit := s.caseAuditBody(c)
	anchorNote := liveAudit["anchor_note"]
	liveDetail := s.caseDetailBody(c)
	contact, contactHistory := s.apiContactOverlay(c)
	liveDetail["api_contact"] = map[string]interface{}{"overlay": contact, "history": contactHistory}
	det, err := exportGeneric(liveDetail)
	if err != nil {
		return nil, err
	}
	aud, err := exportGeneric(liveAudit)
	if err != nil {
		return nil, err
	}
	ra, _ := redactExport(aud)
	detail, audit := redactExportDetail(det), ra.(map[string]interface{})
	audit["chain_verified"], audit["chain_length"] = verified, length
	audit["chain_scope"] = "current"
	audit["anchor_note"] = anchorNote // static server text, not case data
	audit["chain_note"] = exportChainNote
	doc := map[string]interface{}{
		"schema": exportSchema, "redacted": true,
		"meta":   s.meta,
		"detail": detail,
		"audit":  audit,
	}
	// Hash the generic-JSON form, not the Go-struct form: struct fields (e.g. Block) would otherwise keep Go order,
	// while a verifier parsing the served document sees sorted map keys. Numbers are json.Number so the hashed text
	// is the served text. Normalising here makes the hash reproducible from the served bytes.
	norm, err := exportGeneric(doc)
	if err != nil {
		return nil, err
	}
	doc["content_sha256"] = sha(canon(norm))
	return doc, nil
}

// exportCaseID parses /api/cases/{id}/export and returns the unescaped case id.
func exportCaseID(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, "/api/cases/")
	if !ok || !strings.HasSuffix(rest, "/export") {
		return "", false
	}
	id, err := url.PathUnescape(strings.TrimSuffix(rest, "/export"))
	if err != nil {
		return "", false
	}
	return id, true
}

// caseExportRoute serves GET /api/cases/{id}/export. It answers 404 for an unknown case and 405 for any other method.
// It is self-contained so the integrator can register it, or call it from caseRoutes, without touching the export logic.
func (s *apiServer) caseExportRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := exportCaseID(r.URL.Path)
	if !ok {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "not found", "path": r.URL.Path}))
		return
	}
	c := s.cases[id]
	if c == nil {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "unknown case", "id": id,
			"hint": "case ids are loop ids such as ahs-mushrif/ServiceRequest/AHS-sr-1002; URL-encode the slashes or pass them as-is"}))
		return
	}
	if r.Method != http.MethodGet {
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed"}))
		return
	}
	doc, err := s.caseExportDoc(c)
	if err != nil {
		writeJSONResp(w, 500, s.envelope(map[string]interface{}{"error": "export could not be encoded"}))
		return
	}
	writeJSONResp(w, 200, doc)
}
