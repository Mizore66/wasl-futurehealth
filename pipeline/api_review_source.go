package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// reviewSource is the DiagnosticReport revision a raised evidence item judges, re-read from the source log per request.
type reviewSource struct {
	Report   string  `json:"report"`
	Version  int     `json:"report_version"`
	Day      float64 `json:"day"`
	Category string  `json:"category,omitempty"`
	Text     string  `json:"text"`
}

// sourcePointer locates one received DiagnosticReport revision. The API keeps the pointer, never the report text.
type sourcePointer struct {
	seq uint64
	sha [32]byte
}

const (
	sourceMissing    = "missing"
	sourceAmbiguous  = "ambiguous"
	sourceUnreadable = "unreadable"
)

func sourceKey(report string, version int) string {
	return report + "@" + strconv.Itoa(max(version, 1))
}

func (s *apiServer) indexSource(prov string, e *Event, m LogMsg) {
	if e.RType != "DiagnosticReport" || e.ReportID == "" {
		return
	}
	key := sourceKey(prov+"/DiagnosticReport/"+e.ReportID, e.Version)
	p := sourcePointer{seq: m.Seq, sha: sha256.Sum256(m.Data)}
	if !slices.ContainsFunc(s.sources[key], func(q sourcePointer) bool { return q.sha == p.sha }) {
		s.sources[key] = append(s.sources[key], p)
	}
}

// reviewSourceFor binds an evidence item to exactly one received report revision: the provider-qualified report its
// evidence chain names, at the item's report_version. Anything else is a named unavailability, never a guess.
func (s *apiServer) reviewSourceFor(it *ReviewItem) (*reviewSource, string) {
	report := ""
	for _, b := range it.Chain {
		ref, _ := b.Body["report"].(string)
		if it.ReportID == "" || !strings.HasSuffix(ref, "/DiagnosticReport/"+it.ReportID) {
			continue
		}
		if report != "" && report != ref {
			return nil, sourceAmbiguous
		}
		report = ref
	}
	if report == "" {
		return nil, sourceMissing
	}
	ptrs := s.sources[sourceKey(report, it.ReportVersion)]
	if len(ptrs) == 0 {
		return nil, sourceMissing
	}
	if len(ptrs) > 1 {
		return nil, sourceAmbiguous
	}
	m, ok := s.fhirLogEntry(ptrs[0])
	if !ok {
		return nil, sourceUnreadable
	}
	e, _ := normalise(strings.Split(m.Subject, ".")[1], 0, m.Data)
	if e == nil || strings.TrimSpace(e.Text) == "" {
		return nil, sourceUnreadable
	}
	return &reviewSource{Report: report, Version: max(e.Version, 1), Day: round4(e.Day), Category: e.Category, Text: e.Text}, ""
}

func (s *apiServer) fhirLogEntry(p sourcePointer) (LogMsg, bool) {
	f, err := os.Open(filepath.Join(s.run, "fhir_log.ndjson"))
	if err != nil {
		return LogMsg{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var m LogMsg
		if json.Unmarshal(sc.Bytes(), &m) == nil && m.Seq == p.seq && sha256.Sum256(m.Data) == p.sha && apiFHIRSubject.MatchString(m.Subject) {
			return m, true
		}
	}
	return LogMsg{}, false
}

// GET /api/cases/{id}/review/source?item=<raised evidence item>: the complete report an evidence signer judges.
// Opening it records nothing and never answers the question.
func (s *apiServer) caseReviewSource(w http.ResponseWriter, r *http.Request, c *apiCase) {
	w.Header().Set("Cache-Control", "no-store")
	if err := Authorize(r.Header.Get("X-Wasl-Role"), ActSignEvidence); err != nil {
		writeJSONResp(w, 403, s.envelope(map[string]interface{}{"error": "only an evidence signer may open a source report"}))
		return
	}
	it := s.queueItem(c.ID, r.URL.Query().Get("item"))
	if it == nil {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "review item not raised for this case"}))
		return
	}
	if !slices.Contains(it.Kinds, "evidence") {
		writeJSONResp(w, 400, s.envelope(map[string]interface{}{"error": "this item raises no evidence question"}))
		return
	}
	src, reason := s.reviewSourceFor(it)
	if reason != "" {
		writeJSONResp(w, 409, s.envelope(map[string]interface{}{"error": "source report unavailable", "source_status": reason}))
		return
	}
	writeJSONResp(w, 200, s.envelope(map[string]interface{}{"item": it.ItemID, "source": src}))
}

// evidenceSourceRefusal is the write-side gate: an evidence read is accepted only for a raised evidence item whose
// matching source report can be opened now. Identity-only reads are unaffected.
func (s *apiServer) evidenceSourceRefusal(c *apiCase, item string) map[string]interface{} {
	it := s.queueItem(c.ID, item)
	if it == nil {
		return map[string]interface{}{"error": "an evidence read must name a raised evidence item", "source_status": sourceMissing}
	}
	if !slices.Contains(it.Kinds, "evidence") {
		return nil
	}
	if _, reason := s.reviewSourceFor(it); reason != "" {
		return map[string]interface{}{"error": "source report unavailable; an evidence read needs the matching report", "source_status": reason}
	}
	return nil
}
