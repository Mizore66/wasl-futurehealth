package main

// Synthetic photo attachment for colonoscopy reports (-imgphotos). Images are HyperKvasir held-out (fold-1) test images,
// referenced by sha256 only; their true class comes from the dataset label. Rates are assumptions [A], chosen to
// exercise every image-reader path; they are not measured UAE documentation rates.
//   report documents caecum reached: 85% attach a caecal photo (true caecum image); 5% attach a photo filed under the
//       caecum station that is actually another view (retroflexion / polyp / colitis); 10% attach none
//   report documents no caecum: 10% attach a mis-filed non-caecal "caecum" photo
//   90% of reports attach 2 mucosal-view photos whose prep class follows the text grade (adequate -> BBPS 2-3,
//       inadequate -> BBPS 0-1); ungraded reports draw a latent grade, 85% adequate [A]. The answer key is unchanged:
//       it is the text key (an ungraded report stays a human decision), so photos never change the truth.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

var ImgPhotos bool
var ImgCachePath string

type PhotoTruth struct {
	Sha     string `json:"sha"`
	Station string `json:"station"`
	Class   string `json:"class"` // HyperKvasir class of the image
}

type photoPool struct{ byClass map[string][]string }

func loadPhotoPool(path string) *photoPool {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j struct {
		Images map[string]struct {
			Class string `json:"hk_class"`
		} `json:"images"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	p := &photoPool{byClass: map[string][]string{}}
	for s, v := range j.Images {
		p.byClass[v.Class] = append(p.byClass[v.Class], s)
	}
	for _, v := range p.byClass {
		sort.Strings(v)
	}
	return p
}

func hu(seed int64, parts ...string) float64 {
	h := sha256.New()
	fmt.Fprint(h, seed)
	for _, x := range parts {
		h.Write([]byte("|" + x))
	}
	s := h.Sum(nil)
	return (float64(binary.BigEndian.Uint64(s[:8])>>11) + 0.5) / float64(uint64(1)<<53)
}

func (p *photoPool) pick(seed int64, cls []string, key ...string) PhotoTruth {
	c := cls[int(hu(seed, append(key, "cls")...)*float64(len(cls)))]
	xs := p.byClass[c]
	return PhotoTruth{Sha: xs[int(hu(seed, append(key, "img")...)*float64(len(xs)))], Class: c}
}

func (p *photoPool) attach(seed int64, reportID string, src RepSrc) ([]interface{}, []PhotoTruth) {
	var out []PhotoTruth
	other := []string{"retroflex-rectum", "polyps", "ulcerative-colitis-grade-2"}
	u := hu(seed, reportID, "caecal")
	if src.KeyFields["caecum"] == "yes" {
		switch {
		case u < .85:
			ph := p.pick(seed, []string{"cecum"}, reportID, "c0")
			ph.Station = "caecum"
			out = append(out, ph)
		case u < .90:
			ph := p.pick(seed, other, reportID, "c0")
			ph.Station = "caecum"
			out = append(out, ph)
		}
	} else if u < .10 {
		ph := p.pick(seed, other, reportID, "c0")
		ph.Station = "caecum"
		out = append(out, ph)
	}
	if hu(seed, reportID, "mucosa") < .90 {
		grade := src.KeyFields["prep"]
		if grade == "not_stated" || grade == "" {
			grade = map[bool]string{true: "adequate", false: "inadequate"}[hu(seed, reportID, "latent") < .85]
		}
		cls := map[string]string{"adequate": "bbps-2-3", "inadequate": "bbps-0-1"}[grade]
		for i := 0; i < 2; i++ {
			ph := p.pick(seed, []string{cls}, reportID, fmt.Sprint("m", i))
			ph.Station = "mucosa"
			out = append(out, ph)
		}
	}
	media := []interface{}{}
	for _, ph := range out {
		media = append(media, map[string]interface{}{"comment": ph.Station, "link": map[string]string{"reference": "Media/" + ph.Sha[:16], "display": "sha256:" + ph.Sha}})
	}
	return media, out
}
