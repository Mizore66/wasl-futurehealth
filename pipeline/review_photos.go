package main

// Photo assets behind image-backed CRC review items. Simulated data only: pixels come from a directory of synthetic
// images named by WASL_IMAGE_PIXELS_DIR, indexed by the SHA-256 of their content. A photo is served only through a
// review item of the requested case, and only when the item's own report filed that digest at the station the item
// asks about. Nothing here scores, selects or answers: the image reader's verdict stays in the evidence block.

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const photoMaxBytes = 4 << 20

var photoDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// pixelStore maps a content digest to a file path. The path is never serialised or sent to a client.
type pixelStore struct{ files map[string]string }

func newPixelStore(dir string) *pixelStore {
	if dir == "" {
		return nil
	}
	p := &pixelStore{files: map[string]string{}}
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png", ".jpg", ".jpeg":
			if b, ok := readPhoto(path); ok {
				p.files[sha(b)] = path
			}
		}
		return nil
	})
	return p
}

func readPhoto(path string) ([]byte, bool) {
	if fi, err := os.Stat(path); err != nil || fi.Size() > photoMaxBytes {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	switch http.DetectContentType(b) {
	case "image/png", "image/jpeg":
		return b, true
	}
	return nil, false
}

// get returns the pixels for a digest, re-hashing so a replaced file is never served under another digest.
func (p *pixelStore) get(digest string) ([]byte, string, bool) {
	if p == nil {
		return nil, "", false
	}
	path, ok := p.files[digest]
	if !ok {
		return nil, "", false
	}
	b, ok := readPhoto(path)
	if !ok || sha(b) != digest {
		return nil, "", false
	}
	return b, http.DetectContentType(b), true
}

// photoStationFor: the capture station a review item's reason asks the clinician to look at ("" = no photos).
func photoStationFor(it *ReviewItem) string {
	if len(it.Kinds) != 1 || it.Kinds[0] != "evidence" {
		return ""
	}
	switch it.Reason {
	case CompletionPurpose:
		return "caecum"
	case "prep_grade_one_click", "image_prep_conflict":
		return "mucosa"
	}
	return ""
}

func itemReportKey(it *ReviewItem) string {
	for _, b := range it.Chain {
		if b.Type == "evidence" {
			if r, ok := b.Body["report"].(string); ok {
				return r
			}
		}
	}
	return ""
}

// observePhotoAssociation records only unambiguous provider/report/version
// associations. Reordered/duplicate references are the same station/digest set.
// An empty observation or any contradiction is sticky: later replay cannot
// restore authority by choosing the first or last positive list. This index is
// derived only; neither normalized source references nor signed OUT are changed.
func (s *apiServer) observePhotoAssociation(report string, version int, refs []ImgRef) {
	if version < 0 {
		return
	}
	if s.photos == nil {
		s.photos = map[string][]ImgRef{}
	}
	key := reportVersionKey(report, version)
	previous, observed := s.photos[key]
	if !observed {
		s.photos[key] = append([]ImgRef(nil), refs...)
		return
	}
	oldSet, newSet := map[ImgRef]bool{}, map[ImgRef]bool{}
	for _, ref := range previous {
		oldSet[ref] = true
	}
	for _, ref := range refs {
		newSet[ref] = true
	}
	if len(oldSet) != len(newSet) {
		s.photos[key] = nil
		return
	}
	for ref := range oldSet {
		if !newSet[ref] {
			s.photos[key] = nil
			return
		}
	}
}

// explicitItemPhotoDigests recognizes only an unambiguous original item source
// with an explicit producer digest list. The version is the immutable item's own
// ReportVersion, used by itemPhotos' existing source key. False does not authorize
// a source or establish legacy provenance: it leaves the pre-existing path alone.
// This bounded consistency veto does not prove original capture stations or raw
// FHIR bytes and introduces no metadata-absent availability policy.
func explicitItemPhotoDigests(it *ReviewItem, report string) (map[string]bool, bool) {
	parts := strings.Split(report, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] != "DiagnosticReport" || parts[2] == "" || (it.ReportID != report && it.ReportID != parts[2]) {
		return nil, false
	}
	var evidence map[string]interface{}
	count := 0
	for _, block := range it.Chain {
		if block.Type == "evidence" {
			evidence = block.Body
			count++
		}
	}
	if count != 1 || evidence["report"] != report {
		return nil, false
	}
	reader, ok := evidence["image_reader"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	raw, present := reader["photo_shas"]
	if !present {
		return nil, false
	}
	var shas []string
	switch refs := raw.(type) {
	case nil: // An explicit null is the producer's empty digest list.
	case []string:
		shas = refs
	case []interface{}:
		for _, value := range refs {
			digest, valid := value.(string)
			if !valid {
				return nil, false
			}
			shas = append(shas, digest)
		}
	default:
		return nil, false
	}
	out := map[string]bool{}
	for _, digest := range shas {
		if !photoDigest.MatchString(digest) {
			return nil, false
		}
		out[digest] = true
	}
	return out, true
}

// itemPhotos: the report's photos filed at the item's station, sorted by digest.
func (s *apiServer) itemPhotos(it *ReviewItem) []ImgRef {
	station := photoStationFor(it)
	if station == "" || it.ReportVersion < 0 {
		return nil
	}
	report := itemReportKey(it)
	// Zero/one use the existing legacy replay identity; a corrected version
	// must never borrow an older version's media through the report-only key.
	refs := s.photos[reportVersionKey(report, it.ReportVersion)]
	if original, explicit := explicitItemPhotoDigests(it, report); explicit {
		observed := map[string]bool{}
		for _, ref := range refs {
			observed[ref.Sha] = true
		}
		if len(original) != len(observed) {
			return nil
		}
		for digest := range observed {
			if !original[digest] {
				return nil
			}
		}
	}
	var out []ImgRef
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref.Station == station && photoDigest.MatchString(ref.Sha) && !seen[ref.Sha] {
			seen[ref.Sha] = true
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sha < out[j].Sha })
	return out
}

// reviewPhotos is the case-detail projection: per item, each photo's digest and station and whether its pixels exist.
func (s *apiServer) reviewPhotos(c *apiCase) map[string][]map[string]interface{} {
	out := map[string][]map[string]interface{}{}
	for _, it := range s.queue[c.ID] {
		for _, ref := range s.itemPhotos(it) {
			_, _, ok := s.pixels.get(ref.Sha)
			out[it.ItemID] = append(out[it.ItemID], map[string]interface{}{"sha": ref.Sha, "station": ref.Station, "available": ok})
		}
	}
	return out
}

// GET /api/cases/{id}/review-photos/{item}/{sha}: clinicians only (evidence discipline); the same 404 answers an unknown
// item, a digest the item's report did not file at its station, and a photo whose pixels are not available.
func (s *apiServer) casePhotoRoute(w http.ResponseWriter, r *http.Request, rest string) bool {
	i := strings.LastIndex(rest, "/review-photos/")
	if i < 0 {
		return false
	}
	args := strings.Split(rest[i+len("/review-photos/"):], "/")
	id, _ := url.PathUnescape(rest[:i])
	c := s.cases[id]
	if len(args) != 2 || c == nil {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "photo unavailable"}))
		return true
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONResp(w, 405, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": "GET"}))
		return true
	}
	s.casePhoto(w, r, c, args[0], args[1])
	return true
}

func (s *apiServer) casePhoto(w http.ResponseWriter, r *http.Request, c *apiCase, item, digest string) {
	if err := Authorize(r.Header.Get("X-Wasl-Role"), ActSignEvidence); err != nil {
		writeJSONResp(w, 403, s.envelope(map[string]interface{}{"error": "photo permission denied"}))
		return
	}
	var b []byte
	var kind string
	ok := false
	if photoDigest.MatchString(digest) {
		for _, it := range s.queue[c.ID] {
			if it.ItemID != item {
				continue
			}
			for _, ref := range s.itemPhotos(it) {
				if ref.Sha == digest {
					b, kind, ok = s.pixels.get(digest)
				}
			}
		}
	}
	if !ok {
		writeJSONResp(w, 404, s.envelope(map[string]interface{}{"error": "photo unavailable"}))
		return
	}
	h := w.Header()
	h.Set("Content-Type", kind)
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	w.Write(b)
}

// visibleImageQuestions is one active-protocol visibility snapshot for this
// viewer. A nil map preserves the existing non-reader/auditor projection. A
// clinician gets model bytes only in the exact answered item's own chain; case
// aggregate evidence/current chains have no exact question/source binding and
// are conservatively withheld. No historical or unrelated read unlocks them.
func (s *apiServer) visibleImageQuestions(c *apiCase, viewer *apiPrincipal) map[string]bool {
	if viewer == nil || Authorize(viewer.role, ActSignEvidence) != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	visible := map[string]bool{}
	states := s.protoStatesLocked()
	for _, it := range s.queue[c.ID] {
		if !itemHasKind(it, "evidence") {
			continue
		}
		models := 0
		for _, block := range it.Chain {
			if _, ok := block.Body["image_reader"]; ok {
				models++
			}
		}
		st := states[reviewQuestionKey{it.ItemID, "evidence"}]
		// A multi-model chain cannot identify which model belongs to the
		// question without the shared item/source context contract. Withhold
		// ambiguous bytes instead of selecting a first/local-ID match.
		visible[it.ItemID] = models == 1 && st != nil && len(st.Reads) > 0 && !concealedFrom(st, *viewer)
	}
	return visible
}

func withoutImageVerdict(chain []Block) []Block {
	out := make([]Block, len(chain))
	for i, b := range chain {
		if _, has := b.Body["image_reader"]; has {
			body := make(map[string]interface{}, len(b.Body))
			for k, v := range b.Body {
				if k != "image_reader" {
					body[k] = v
				}
			}
			b.Body = body
		}
		out[i] = b
	}
	return out
}
