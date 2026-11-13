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

// itemPhotos: the report's photos filed at the item's station, sorted by digest.
func (s *apiServer) itemPhotos(it *ReviewItem) []ImgRef {
	station := photoStationFor(it)
	if station == "" {
		return nil
	}
	var out []ImgRef
	seen := map[string]bool{}
	for _, ref := range s.photos[itemReportKey(it)] {
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

// imageVerdictWithheld: a clinician who may answer the evidence question must not see the image model's read until a
// first read of that question is visible to them (their own, or one already revealed). Other roles are not readers.
func (s *apiServer) imageVerdictWithheld(c *apiCase, viewer *apiPrincipal) bool {
	if viewer == nil || Authorize(viewer.role, ActSignEvidence) != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.queue[c.ID] {
		if st := s.rstates[reviewQuestionKey{it.ItemID, "evidence"}]; st != nil && len(st.Reads) > 0 && !concealedFrom(st, *viewer) {
			return false
		}
	}
	return true
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
