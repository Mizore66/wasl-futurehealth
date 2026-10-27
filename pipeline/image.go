package main

// Image evidence reader (third reader; HOLD-ONLY): two small CNNs trained on HyperKvasir (image/train.py)
//   landmark: is a photo the caecum (appendiceal orifice / ileocaecal valve / caecal pole)?
//   prep:     does a mucosal-view photo show adequate bowel cleansing (BBPS 2-3) or not (BBPS 0-1)?
// Photos arrive as DiagnosticReport.media[] references (sha256 in link.display, capture station in comment:
// caecum | retroflexion | mucosa); pixels never enter the stream.
// Modes: cached (models/image_verdicts.json, keyed by sha256; deterministic, no model needed) | live (image/serve.py
// sidecar) | replay (<run>/image_ledger.jsonl only). Off unless -image is set.
//
// Invariant (tested): the image reader can only HOLD. It never turns a not-closed text verdict into a closure and never
// creates agreement. Its outputs are (1) a documentation flag missing_caecal_photo (DoH CRC App. 11 s2.2 requires
// auditable photo documentation of completion; we treat it as a unit-audit indicator, not a closure blocker, because the
// closure rule already requires completion documented in the report and App. 2 audits CIR "with photographic evidence"
// at unit level), (2) a review hint that turns an ungraded-prep report into a one-click endoscopist prep grade when the
// photos look adequate, and (3) a hold when the photos contradict an explicit "adequate prep" text grade.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type ImgRef struct {
	Sha     string `json:"sha"`
	Station string `json:"station"` // caecum | retroflexion | mucosa (endoscopist's capture station)
}

type ImageVerdict struct {
	Model       string   `json:"model"`
	Photos      int      `json:"photos"`
	Caecal      string   `json:"caecal_photo"` // verified | unverified | missing
	LandmarkP   float64  `json:"landmark_p,omitempty"`
	Prep        string   `json:"prep"` // adequate | inadequate | none
	PrepP       float64  `json:"prep_p,omitempty"`
	Unavailable int      `json:"unavailable,omitempty"` // photos the reader could not read (fail-safe: count as absent)
	Shas        []string `json:"shas,omitempty"`
}

type imgScore struct {
	Sha       string  `json:"sha"`
	LandmarkP float64 `json:"landmark_p"`
	PrepP     float64 `json:"prep_p"`
	Model     string  `json:"model"`
}

type ImageReader struct {
	Mode           string
	Model          string
	ThrLand, ThrPr float64
	cache          map[string]imgScore
	url            string
	http           *http.Client
	mu             sync.Mutex
	ledgerFn       string
}

// ImageR is set by -image cached|live|replay; nil = image reader off (v7strict results unchanged).
var ImageR *ImageReader

// OneClickMedS: median seconds for a one-click endoscopist prep grade with the photo shown [A] (vs EvMedS for a full read).
var OneClickMedS = 20.0

func LoadImageCache(path string) *ImageReader {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j struct {
		Model      string             `json:"model_version"`
		Thresholds map[string]float64 `json:"thresholds"`
		Images     map[string]struct {
			Landmark struct{ P float64 } `json:"landmark"`
			Prep     struct{ P float64 } `json:"prep"`
		} `json:"images"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	r := &ImageReader{Mode: "cached", Model: j.Model + "/cached", ThrLand: j.Thresholds["landmark"], ThrPr: j.Thresholds["prep"], cache: map[string]imgScore{}}
	for s, v := range j.Images {
		r.cache[s] = imgScore{Sha: s, LandmarkP: v.Landmark.P, PrepP: v.Prep.P, Model: j.Model}
	}
	return r
}

// NewImageClient: live sidecar (or replay when url is unreachable); answers are appended to a sha256-keyed ledger.
func NewImageClient(mode, url, ledger string, thrLand, thrPrep float64) *ImageReader {
	r := &ImageReader{Mode: mode, Model: "hk-mnv3s-v1/" + mode, ThrLand: thrLand, ThrPr: thrPrep, cache: map[string]imgScore{}, url: url,
		http: &http.Client{Timeout: 20 * time.Second}, ledgerFn: ledger}
	if f, err := os.Open(ledger); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var s imgScore
			if json.Unmarshal(sc.Bytes(), &s) == nil && s.Sha != "" {
				r.cache[s.Sha] = s
			}
		}
		f.Close()
	}
	return r
}

func (r *ImageReader) score(sha string) (imgScore, bool) {
	r.mu.Lock()
	s, ok := r.cache[sha]
	r.mu.Unlock()
	if ok || r.Mode == "cached" || r.Mode == "replay" {
		return s, ok
	}
	body, _ := json.Marshal(map[string]interface{}{"images": []string{sha}})
	resp, err := r.http.Post(r.url+"/v1/image/read", "application/json", bytes.NewReader(body))
	if err != nil {
		return s, false
	}
	defer resp.Body.Close()
	var out struct {
		Model   string     `json:"model_version"`
		Results []imgScore `json:"results"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Results) != 1 || out.Results[0].Sha != sha {
		return s, false
	}
	s = out.Results[0]
	s.Model = out.Model
	r.mu.Lock()
	r.cache[sha] = s
	if f, err := os.OpenFile(r.ledgerFn, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		b, _ := json.Marshal(s)
		f.Write(append(b, '\n'))
		f.Close()
	}
	r.mu.Unlock()
	return s, true
}

func (r *ImageReader) Healthy() bool {
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(r.url + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

// Read aggregates the photos of one colonoscopy report.
func (r *ImageReader) Read(refs []ImgRef) *ImageVerdict {
	v := &ImageVerdict{Model: r.Model, Photos: len(refs), Caecal: "missing", Prep: "none"}
	minPrep := 2.0
	for _, ref := range refs {
		v.Shas = append(v.Shas, ref.Sha)
		s, ok := r.score(ref.Sha)
		if !ok {
			v.Unavailable++
			continue
		}
		switch ref.Station {
		case "caecum":
			if s.LandmarkP > v.LandmarkP {
				v.LandmarkP = round4(s.LandmarkP)
			}
			if s.LandmarkP >= r.ThrLand {
				v.Caecal = "verified"
			} else if v.Caecal == "missing" {
				v.Caecal = "unverified" // a photo filed as caecal that the landmark model does not recognise
			}
		case "mucosa":
			if s.PrepP < minPrep {
				minPrep = s.PrepP
			}
		}
	}
	if minPrep <= 1 {
		v.PrepP = round4(minPrep)
		v.Prep = map[bool]string{true: "adequate", false: "inadequate"}[minPrep >= r.ThrPr] // worst mucosal view decides
	}
	sort.Strings(v.Shas)
	return v
}

// applyImage adds the third reader to a text verdict. It may only downgrade (hold) or annotate.
func applyImage(v *TwoVerdict, iv *ImageVerdict) {
	if v == nil || iv == nil || v.Kind == "pathology" {
		return
	}
	v.Image, v.PreImage = iv, v.Label
	caecumYes := v.Regex.Caecum == "yes" && v.Gemma.Caecum == "yes"
	ungraded := v.Regex.Prep == "not_stated" && v.Gemma.Prep == "not_stated"
	switch {
	case v.Agree && v.Label == "adequate":
		if iv.Caecal != "verified" {
			v.Flags = appendUniq(v.Flags, "missing_caecal_photo")
		}
		if iv.Prep == "inadequate" && (v.Regex.Prep == "adequate" || v.Gemma.Prep == "adequate") {
			v.Agree, v.Label, v.Rule, v.ReviewHint = false, "disagree", strings.TrimPrefix(v.Rule+"+image_prep_conflict", "+"), "image_prep_conflict"
		}
	case !v.Agree && caecumYes && ungraded:
		switch iv.Prep {
		case "adequate":
			v.ReviewHint = "prep_grade_one_click" // a human still grades; the photo is shown with the item
		case "inadequate":
			v.ReviewHint = "image_prep_inadequate" // full review, flagged
		}
		if iv.Caecal != "verified" {
			v.Flags = appendUniq(v.Flags, "missing_caecal_photo")
		}
	}
}

func appendUniq(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

func imageDesc() string {
	if ImageR == nil {
		return "off"
	}
	return fmt.Sprintf("%s(thr_land=%.2f,thr_prep=%.2f)", ImageR.Model, ImageR.ThrLand, ImageR.ThrPr)
}

// imageReport: what the third reader did in a run (report7 "image_reader"). Hold-only check: no closure's evidence
// label differs from its pre-image label in the "adequate" direction.
func imageReport(preps []*Prep, fhir []LogMsg, tr *Truth, final map[string]*Decision) map[string]interface{} {
	if ImageR == nil {
		return map[string]interface{}{"mode": "off"}
	}
	hints, flags, caecal, prep := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	reports, withPhotos, upgraded, downgraded := 0, 0, 0, 0
	hintTruth := map[string]map[string]int{}
	for _, p := range preps {
		if p == nil || p.Ev == nil || p.Ev.RType != "DiagnosticReport" || p.Ev.Ev == nil || p.Ev.Ev.Image == nil {
			continue
		}
		v := p.Ev.Ev
		reports++
		if v.Image.Photos > 0 {
			withPhotos++
		}
		caecal[v.Image.Caecal]++
		prep[v.Image.Prep]++
		if v.ReviewHint != "" {
			hints[v.ReviewHint]++
			t := tr.Reports[p.Ev.ReportID]
			if hintTruth[v.ReviewHint] == nil {
				hintTruth[v.ReviewHint] = map[string]int{}
			}
			hintTruth[v.ReviewHint][t]++
		}
		for _, f := range v.Flags {
			if f == "missing_caecal_photo" {
				flags[f]++
			}
		}
		if v.Label == "adequate" && v.PreImage != "adequate" {
			upgraded++
		}
		if v.PreImage == "adequate" && v.Label != "adequate" {
			downgraded++
		}
	}
	return map[string]interface{}{"mode": ImageR.Mode, "model": ImageR.Model, "thresholds": map[string]float64{"landmark": ImageR.ThrLand, "prep": ImageR.ThrPr},
		"colonoscopy_reports_read": reports, "reports_with_photos": withPhotos, "caecal_photo": caecal, "photo_prep": prep,
		"review_hints": hints, "review_hint_truth": hintTruth, "flags": flags,
		"labels_upgraded_by_image": upgraded, "labels_downgraded_by_image": downgraded,
		"one_click_med_s": OneClickMedS}
}
