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
	"encoding/json"
	"net/http"
	"net/url"
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
		"api_reviews": s.reviewsFor(c.ID), "current": c.last, "booking": s.bookingFor(c.ID),
		"agent_investigations": s.agentFor(c.ID), "agent_provenance": s.agentProvenance(),
	}
}

// caseAuditBody is the body of GET /api/cases/{id}/audit (without the meta envelope). It must stay identical to the
// projection in caseAudit (api.go); api_export_test.go pins the two against each other.
func (s *apiServer) caseAuditBody(c *apiCase) map[string]interface{} {
	chain := c.last.Chain
	return map[string]interface{}{
		"case": c.ID, "chain": nz(chain), "chain_verified": verifyChain(chain), "chain_length": len(chain),
		"anchors": s.anchors, "anchor_note": "daily Merkle/stream heads co-signed into an in-UAE WORM store; `wsi7 tamper` re-verifies the whole run",
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

// caseExportDoc builds the export document for one case. Two calls return identical bytes while the run and the
// case/review history are unchanged.
func (s *apiServer) caseExportDoc(c *apiCase) (map[string]interface{}, error) {
	doc := map[string]interface{}{
		"schema": exportSchema,
		"meta":   s.meta,
		"detail": s.caseDetailBody(c),
		"audit":  s.caseAuditBody(c),
	}
	// Hash the generic-JSON form, not the Go-struct form: struct fields (e.g. Block) would otherwise keep Go order,
	// while a verifier parsing the served document sees sorted map keys. Normalising here makes the hash reproducible
	// from the served bytes: parse, drop content_sha256, canon, sha.
	var norm map[string]interface{}
	if err := json.Unmarshal(canon(doc), &norm); err != nil {
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
