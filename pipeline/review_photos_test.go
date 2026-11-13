package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	photoTokClin = "ir-clin-a-token-1234567890"
	photoTokSC   = "ir-sc-token-1234567890"
	photoTokRev  = "ir-rev-a-token-1234567890"
	photoTokCo   = "ir-coord-token-1234567890"
	photoTokAud  = "ir-aud-token-1234567890"

	photoReport      = "skmc-endo/DiagnosticReport/R1"
	photoOtherReport = "skmc-endo/DiagnosticReport/R2"
)

// synthPNG is a generated 8x8 flat-colour image: synthetic test pixels, never clinical.
func synthPNG(t *testing.T, shade uint8) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type photoEnv struct {
	*irEnv
	pix                                                  string
	caecum, mucosa1, mucosa2, retro, otherCaecum, absent string
	bytesBySha                                           map[string][]byte
}

func photoMedia(refs ...[2]string) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, r := range refs {
		out = append(out, map[string]interface{}{"comment": r[1], "link": map[string]interface{}{"display": "sha256:" + r[0]}})
	}
	return out
}

func newPhotoEnv(t *testing.T) *photoEnv {
	t.Helper()
	pix := t.TempDir()
	e := &photoEnv{pix: pix, bytesBySha: map[string][]byte{}}
	for name, shade := range map[string]uint8{"caecum.png": 10, "m1.png": 60, "m2.png": 120, "retro.png": 180, "other.png": 240} {
		b := synthPNG(t, shade)
		if err := os.WriteFile(filepath.Join(pix, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
		e.bytesBySha[sha(b)] = b
		switch name {
		case "caecum.png":
			e.caecum = sha(b)
		case "m1.png":
			e.mucosa1 = sha(b)
		case "m2.png":
			e.mucosa2 = sha(b)
		case "retro.png":
			e.retro = sha(b)
		case "other.png":
			e.otherCaecum = sha(b)
		}
	}
	e.absent = sha([]byte("a photo whose pixels were never stored"))

	run := irRun(t)
	out, err := os.OpenFile(filepath.Join(run, "out_log.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for i, spec := range []struct{ item, reason, report string }{
		{"rv-comp", CompletionPurpose, photoReport},
		{"rv-one", "prep_grade_one_click", photoReport},
		{"rv-conf", "image_prep_conflict", photoReport},
		{"rv-text", "", photoReport},
		{"rv-other", CompletionPurpose, photoOtherReport},
		{"rv-lost", CompletionPurpose, "skmc-endo/DiagnosticReport/R3"},
	} {
		it := &ReviewItem{ItemID: spec.item, Reason: spec.reason}
		it.LoopID, it.Kinds = ledgerLoop, []string{"evidence"}
		it.Chain = appendBlock(nil, "evidence", map[string]interface{}{"report": spec.report, "image_reader": map[string]interface{}{"model": "synthetic", "prep": "adequate"}})
		m := LogMsg{Seq: uint64(30 + i), Subject: "out.p0", Hdr: map[string]string{}}
		encodeEnv(&m, OutEnvelope{Kind: "review", Item: it})
		b, _ := json.Marshal(m)
		out.Write(append(b, '\n'))
	}
	out.Close()

	fhir, err := os.Create(filepath.Join(run, "fhir_log.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	for i, rep := range []struct {
		id    string
		media []map[string]interface{}
	}{
		{"R1", photoMedia([2]string{e.caecum, "caecum"}, [2]string{e.mucosa1, "mucosa"}, [2]string{e.mucosa2, "mucosa"}, [2]string{e.retro, "retroflexion"})},
		{"R2", photoMedia([2]string{e.otherCaecum, "caecum"})},
		{"R3", photoMedia([2]string{e.absent, "caecum"})},
	} {
		res := map[string]interface{}{"resourceType": "DiagnosticReport", "id": rep.id, "status": "final", "effectiveDateTime": "2026-01-11T00:00:00Z",
			"code": map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "18746-8"}}}, "conclusion": "synthetic", "media": rep.media}
		data, _ := json.Marshal(res)
		line, _ := json.Marshal(map[string]interface{}{"seq": i + 1, "subj": "fhir.skmc-endo.res", "hdr": map[string]string{"Wsi-Arr": "10"}, "data": json.RawMessage(data), "ts": 1})
		fhir.Write(append(line, '\n'))
	}
	fhir.Close()

	e.irEnv = &irEnv{run: run, path: "/api/cases/" + url.PathEscape(ledgerLoop)}
	e.reload(t)
	return e
}

// reload is a fresh process over the same run directory and pixel directory.
func (e *photoEnv) reload(t *testing.T) {
	t.Helper()
	e.s = loadAPI(e.run, "")
	e.s.pixels = newPixelStore(e.pix)
	h, err := e.s.handler(irTokens, "")
	if err != nil {
		t.Fatal(err)
	}
	e.h = h
}

func (e *photoEnv) get(token, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *photoEnv) photoURL(item, digest string) string {
	return e.path + "/review-photos/" + item + "/" + digest
}

func (e *photoEnv) projection(t *testing.T) map[string][]map[string]interface{} {
	t.Helper()
	w := e.get(photoTokClin, e.path)
	if w.Code != 200 {
		t.Fatalf("case detail: %d %s", w.Code, w.Body)
	}
	var body struct {
		Photos map[string][]map[string]interface{} `json:"review_photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Photos
}

func shaStations(refs []map[string]interface{}) []string {
	out := []string{}
	for _, r := range refs {
		out = append(out, r["station"].(string)+":"+r["sha"].(string)+":"+map[bool]string{true: "ok", false: "missing"}[r["available"].(bool)])
	}
	return out
}

func (e *photoEnv) itemHasVerdict(t *testing.T, token, item string) bool {
	t.Helper()
	w := e.get(token, e.path)
	if w.Code != 200 {
		t.Fatalf("case detail: %d", w.Code)
	}
	var body struct {
		Items []struct {
			Item  string `json:"item"`
			Chain []struct {
				Body map[string]interface{} `json:"body"`
			} `json:"chain"`
		} `json:"review_items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, it := range body.Items {
		if it.Item == item {
			for _, b := range it.Chain {
				if _, ok := b.Body["image_reader"]; ok {
					return true
				}
			}
			return false
		}
	}
	t.Fatalf("item %s not in case detail", item)
	return false
}

func TestImageVerdictIsWithheldFromAClinicianUntilAFirstReadIsVisible(t *testing.T) {
	e := newPhotoEnv(t)
	if e.itemHasVerdict(t, photoTokClin, "rv-one") || e.itemHasVerdict(t, photoTokSC, "rv-one") {
		t.Fatal("a clinician with no first read must not receive the image model's verdict")
	}
	if !e.itemHasVerdict(t, photoTokRev, "rv-one") {
		t.Fatal("roles that cannot answer the evidence question keep the existing evidence block")
	}
	e.must(t, photoTokClin, irBody("rv-one", "adequate", "r1", ""), 201)
	if !e.itemHasVerdict(t, photoTokClin, "rv-one") {
		t.Fatal("the first reader sees the verdict after their own first read")
	}
	if e.itemHasVerdict(t, "ir-clin-b-token-1234567890", "rv-one") {
		t.Fatal("the second reader must not see the verdict while the first read is concealed from them")
	}
}

func TestReviewPhotosProjectionBindsEachItemToItsStation(t *testing.T) {
	e := newPhotoEnv(t)
	for pass := 0; pass < 2; pass++ { // the second pass is a reload
		got := e.projection(t)
		if want := []string{"caecum:" + e.caecum + ":ok"}; strings.Join(shaStations(got["rv-comp"]), ",") != strings.Join(want, ",") {
			t.Fatalf("pass %d completion: want only the caecal photo, got %v", pass, shaStations(got["rv-comp"]))
		}
		mucosa := map[string]bool{"mucosa:" + e.mucosa1 + ":ok": true, "mucosa:" + e.mucosa2 + ":ok": true}
		for _, item := range []string{"rv-one", "rv-conf"} {
			g := shaStations(got[item])
			if len(g) != 2 || !mucosa[g[0]] || !mucosa[g[1]] {
				t.Fatalf("pass %d %s: want the two mucosal photos only, got %v", pass, item, g)
			}
		}
		if got["rv-text"] != nil {
			t.Fatalf("a text-only CRC item must carry no photos, got %v", got["rv-text"])
		}
		if g := shaStations(got["rv-other"]); len(g) != 1 || !strings.HasPrefix(g[0], "caecum:"+e.otherCaecum) {
			t.Fatalf("pass %d: another report's item must show its own photo, got %v", pass, g)
		}
		if g := shaStations(got["rv-lost"]); len(g) != 1 || !strings.HasSuffix(g[0], ":missing") {
			t.Fatalf("pass %d: a photo with no stored pixels must be reported unavailable, got %v", pass, g)
		}
		e.reload(t)
	}
	if body := e.get(photoTokClin, e.path).Body.String(); strings.Contains(body, e.pix) || strings.Contains(body, ".png") {
		t.Fatal("case detail exposes a filesystem path")
	}
}

func TestReviewPhotoServesOnlyTheItemsOwnPixels(t *testing.T) {
	e := newPhotoEnv(t)
	for _, tok := range []string{photoTokClin, photoTokSC} {
		w := e.get(tok, e.photoURL("rv-comp", e.caecum))
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), e.bytesBySha[e.caecum]) {
			t.Fatalf("authorized clinician: %d %q", w.Code, w.Header().Get("Content-Type"))
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("photo headers: %v", w.Header())
		}
	}
	for _, item := range []string{"rv-one", "rv-conf"} {
		if w := e.get(photoTokClin, e.photoURL(item, e.mucosa2)); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), e.bytesBySha[e.mucosa2]) {
			t.Fatalf("%s mucosal photo: %d", item, w.Code)
		}
	}
}

func TestReviewPhotoRefusals(t *testing.T) {
	e := newPhotoEnv(t)
	good := e.photoURL("rv-comp", e.caecum)
	if w := e.get("", good); w.Code != 401 {
		t.Fatalf("unauthenticated: %d", w.Code)
	}
	if w := e.get("not-a-token", good); w.Code != 401 {
		t.Fatalf("unknown token: %d", w.Code)
	}
	for name, tok := range map[string]string{"auditor": photoTokAud, "coordinator": photoTokCo, "reviewer": photoTokRev} {
		if w := e.get(tok, good); w.Code != 403 || bytes.Contains(w.Body.Bytes(), []byte("PNG")) {
			t.Fatalf("%s: want 403 without pixels, got %d", name, w.Code)
		}
	}
	other := "/api/cases/" + url.PathEscape("skmc-endo/ServiceRequest/NO-SUCH")
	for name, path := range map[string]string{
		"station mismatch (mucosa via completion item)":         e.photoURL("rv-comp", e.mucosa1),
		"wrong station (retroflexion photo of the same report)": e.photoURL("rv-one", e.retro),
		"digest of another report's item":                       e.photoURL("rv-comp", e.otherCaecum),
		"unknown item":                                          e.photoURL("rv-nope", e.caecum),
		"text-only item":                                        e.photoURL("rv-text", e.caecum),
		"unknown digest":                                        e.photoURL("rv-comp", strings.Repeat("a", 64)),
		"filed but pixels never stored":                         e.photoURL("rv-lost", e.absent),
		"upper-case digest":                                     e.photoURL("rv-comp", strings.ToUpper(e.caecum)),
		"short digest":                                          e.photoURL("rv-comp", e.caecum[:63]),
		"encoded traversal":                                     e.photoURL("rv-comp", "%2e%2e%2f"+e.caecum),
		"file name":                                             e.photoURL("rv-comp", "caecum.png"),
		"extra segment":                                         e.photoURL("rv-comp", e.caecum+"/x"),
		"unknown case":                                          other + "/review-photos/rv-comp/" + e.caecum,
	} {
		w := e.get(photoTokClin, path)
		if w.Code != 404 || bytes.Contains(w.Body.Bytes(), []byte("PNG")) || strings.Contains(w.Body.String(), e.pix) {
			t.Errorf("%s: want a bare 404, got %d %s", name, w.Code, w.Body)
		}
	}
	if w := e.get(photoTokClin, e.photoURL("rv-comp", "..")); w.Code == 200 || bytes.Contains(w.Body.Bytes(), []byte("PNG")) {
		t.Fatalf("dot-dot digest: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, good, strings.NewReader("{}"))
	r.Header.Set("Authorization", "Bearer "+photoTokClin)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != 405 {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestReviewPhotoAbsentWithoutAPixelStore(t *testing.T) {
	e := newPhotoEnv(t)
	e.s.pixels = nil
	if w := e.get(photoTokClin, e.photoURL("rv-comp", e.caecum)); w.Code != 404 {
		t.Fatalf("no pixel store: %d", w.Code)
	}
	if g := shaStations(e.projection(t)["rv-comp"]); len(g) != 1 || !strings.HasSuffix(g[0], ":missing") {
		t.Fatalf("no pixel store must read as unavailable, got %v", g)
	}
}

func TestPixelStoreIgnoresNonImagesAndReplacedFiles(t *testing.T) {
	dir := t.TempDir()
	good := synthPNG(t, 33)
	os.WriteFile(filepath.Join(dir, "a.png"), good, 0o600)
	os.WriteFile(filepath.Join(dir, "b.png"), []byte("not an image"), 0o600)
	os.WriteFile(filepath.Join(dir, "c.txt"), good, 0o600)
	p := newPixelStore(dir)
	if len(p.files) != 1 {
		t.Fatalf("only the real image is indexed, got %d", len(p.files))
	}
	os.WriteFile(filepath.Join(dir, "a.png"), synthPNG(t, 34), 0o600)
	if _, _, ok := p.get(sha(good)); ok {
		t.Fatal("a file replaced after indexing must not be served under the old digest")
	}
}
