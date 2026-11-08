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
	"unicode"
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

// exportDenyKeys are removed entirely (key and value) wherever they appear. Matching is on the lower-cased key and is
// exact: there is no prefix matching, so harmless keys such as raw_id, raw_sha or quoted_by survive.
var exportDenyKeys = map[string]bool{
	"quote": true, "quotes": true, "text": true, "raw": true, "raw_text": true, "report_text": true, "clinical_text": true, "note": true,
	"actor": true, "target_actor": true, "owner": true, "canonical_owner": true,
	"message": true, "messages": true, "prompt": true, "prompt_text": true, "reasoning": true,
	"raw_report": true, "raw_report_text": true, "document_text": true, "narrative": true, "comment": true,
	"comments": true, "summary_text": true, "excerpt": true, "snippet": true, "free_text": true,
}

// exportDenySuffixes catch variants such as gemma_quote, reviewer_note, model_reasoning.
var exportDenySuffixes = []string{"_quote", "_quotes", "_text", "_note", "_notes", "_message", "_messages", "_prompt", "_reasoning", "_excerpt", "_snippet", "_narrative", "_comment"}

func exportDenyKey(k string) bool {
	k = strings.ToLower(k)
	if exportDenyKeys[k] {
		return true
	}
	for _, suf := range exportDenySuffixes {
		if strings.HasSuffix(k, suf) {
			return true
		}
	}
	return false
}

// Operational keys (hint, reason, why and their _hint/_reason variants) carry machine codes such as
// prep_grade_one_click or second_identifier. Shape is sufficient only on server-generated paths; coordinator/snooze
// user input is handled separately below.
var exportOpCode = regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)+(?:[+/.:-][a-z0-9_]+)*$`)

func exportOpKey(k string) bool {
	k = strings.ToLower(k)
	return k == "hint" || k == "reason" || k == "why" || strings.HasSuffix(k, "_hint") || strings.HasSuffix(k, "_reason")
}

// exportStaticText lists fixed sentences the server itself writes (no case data in them).
var exportStaticText = map[string]bool{
	"case does not need a colonoscopy slot at the end of the run": true,
}

func exportSafeOpValue(v string) bool { return exportStaticText[v] || exportOpCode.MatchString(v) }

// exportLooksLikeProse is the value-side guard for keys nobody listed: three or more whitespace-separated tokens
// of which at least two contain a letter reads as a sentence, not an identifier, hash, timestamp or code.
func exportLooksLikeProse(v string) bool {
	words := 0
	for _, f := range strings.Fields(v) {
		for _, r := range f {
			if unicode.IsLetter(r) {
				words++
				break
			}
		}
	}
	return len(strings.Fields(v)) >= 3 && words >= 2
}

// ---- allowlisted projection ------------------------------------------------------------------------------------
//
// Evidence bodies, timeline rows and citations are projected through an allowlist: a key that is not listed is
// dropped whatever its value, so a one- or two-word clinical phrase under an unknown key cannot leak. Listed keys
// still pass through the scalar guard (bounded length, no prose, no newlines).

type exportAllow struct {
	leaf bool                    // a scalar, or a list of scalars, is kept
	kids map[string]*exportAllow // keys kept when the value is an object; "*" matches any key
}

var exportLeaf = &exportAllow{leaf: true}

func exportScalarOK(v interface{}) bool {
	switch x := v.(type) {
	case string:
		return len(x) <= 200 && !strings.ContainsAny(x, "\n\r") && !exportLooksLikeProse(x)
	case bool, nil, json.Number, float64:
		return true
	}
	return false
}

func exportProject(v interface{}, a *exportAllow) (interface{}, bool) {
	switch x := v.(type) {
	case map[string]interface{}:
		if a.kids == nil {
			return nil, false
		}
		out := map[string]interface{}{}
		for k, val := range x {
			sub := a.kids[strings.ToLower(k)]
			if sub == nil {
				sub = a.kids["*"]
			}
			if sub == nil {
				continue
			}
			if r, keep := exportProject(val, sub); keep {
				out[k] = r
			}
		}
		return out, true
	case []interface{}:
		if !a.leaf {
			return nil, false
		}
		out := []interface{}{}
		for _, val := range x {
			if exportScalarOK(val) {
				out = append(out, val)
			}
		}
		return out, true
	default:
		if a.leaf && exportScalarOK(v) {
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
	m["status"] = &exportAllow{kids: map[string]*exportAllow{"*": exportLeaf}}
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

// exportEvidenceBlock reports whether m is a chain block of type evidence.
func exportEvidenceBlock(m map[string]interface{}) bool {
	t, _ := m["type"].(string)
	_, hasBody := m["body"].(map[string]interface{})
	return t == "evidence" && hasBody
}

func exportProjectList(v interface{}, a *exportAllow) interface{} {
	list, ok := v.([]interface{})
	if !ok {
		return v
	}
	out := []interface{}{}
	for _, it := range list {
		if r, keep := exportProject(it, a); keep {
			out = append(out, r)
		}
	}
	return out
}

// redactExport returns a redacted copy of a generic-JSON value. It never mutates its input. Evidence block bodies
// and citations go through their allowlists; maps lose denylisted keys; operational hint/reason/why strings keep
// only machine codes on server-generated paths. Coordinator snooze/data reasons are user-controlled and always drop.
func redactExport(v interface{}) (interface{}, bool) {
	return redactExportScope(v, false)
}

func redactExportScope(v interface{}, userOperational bool) (interface{}, bool) {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		isEv := exportEvidenceBlock(x)
		isCoordinator := x["type"] == coordEventType
		for k, val := range x {
			lk := strings.ToLower(k)
			switch {
			case lk == "citations" || lk == "citation":
				out[k] = exportCitations(val)
				continue
			case isEv && k == "body":
				pb, _ := exportProject(val, exportEvidenceAllow)
				out[k] = pb
				continue
			case exportDenyKey(k):
				continue
			case exportOpKey(k):
				if sv, isStr := val.(string); isStr {
					if !userOperational && exportSafeOpValue(sv) {
						out[k] = sv
					}
					continue
				}
			}
			childUserOperational := userOperational || lk == "snooze" || (lk == "data" && isCoordinator)
			if r, keep := redactExportScope(val, childUserOperational); keep {
				out[k] = r
			}
		}
		return out, true
	case []interface{}:
		out := make([]interface{}, 0, len(x))
		for _, val := range x {
			if r, keep := redactExportScope(val, userOperational); keep {
				out = append(out, r)
			}
		}
		return out, true
	case string:
		if exportLooksLikeProse(x) {
			return nil, false
		}
		return x, true
	}
	return v, true
}

// redactExportDetail applies the allowlists to the detail paths that hold bare bodies (evidence, timeline) and then
// the generic redaction, which also projects the evidence blocks inside every chain.
func redactExportDetail(det map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(det))
	for k, v := range det {
		cp[k] = v
	}
	if v, ok := cp["evidence"]; ok {
		cp["evidence"] = exportProjectList(v, exportEvidenceAllow)
	}
	if v, ok := cp["timeline"]; ok {
		cp["timeline"] = exportProjectList(v, exportTimelineAllow)
	}
	r, _ := redactExport(cp)
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
