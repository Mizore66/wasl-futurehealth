package main

// Modelset pairfeat6 pair features f30..f33, appended after the legacy and pairfeat1 features 0..29 only when a
// name-rarity table is loaded (NameIDF: -modelset pairfeat6 at runtime, featdump -nameidf when training pairfeat6).
// Every other modelset keeps the 30-feature vector byte for byte.
//   f30 placeholder date: NaN if either DOB is unusable (missing, invalid, sentinel; a day-00 date with a real year and
//       month is usable); else 1 if either is a 1 January / 1 July placeholder or has day 00; else 0.
//   f31 real exact date: 1 if both DOBs are real, non-placeholder and equal; 0 if both are and differ; NaN otherwise.
//   f32 shared-token rarity: sum of the rarity weights of the distinct skeleton tokens present in both records.
//   f33 rarest unmatched token: the largest rarity weight among distinct skeleton tokens present on one side only
//       (negative weights are not clamped); 0 when every token matches. f32 and f33 are NaN when a skeleton is empty.
// Rarity weight of token t: log(N / (1 + c(t))), N training registrations and c(t) those whose skeleton contains t
// (wsi nameidf, encdev.go); an unseen token has c = 0. Tokens are summed and compared in sorted order.
import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NFEAT6: the pairfeat6 feature count (NFEAT plus f30..f33).
const NFEAT6 = 34

// NameIDF: the pairfeat6 name-rarity table; nil (the default) leaves pairFeats at NFEAT features.
var NameIDF *NameIDFTable

// NameIDFTable: models/name_idf_pairfeat6.json, written by wsi nameidf.
type NameIDFTable struct {
	Version       int            `json:"version"`
	Split         string         `json:"split"`
	Persons       int            `json:"persons"`
	Seeds         []int64        `json:"seeds"`
	Noise         string         `json:"noise"`
	Registrations int            `json:"registrations"`
	Counts        map[string]int `json:"counts"`
}

func readNameIDF(path string) (*NameIDFTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t NameIDFTable
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if t.Version != 1 || t.Registrations <= 0 || t.Counts == nil {
		return nil, fmt.Errorf("%s: want version 1, registrations > 0 and a counts map", path)
	}
	for k, c := range t.Counts {
		if k == "" || c < 1 || c > t.Registrations {
			return nil, fmt.Errorf("%s: count %d for token %q outside 1..%d", path, c, k, t.Registrations)
		}
	}
	return &t, nil
}

// LoadNameIDF: readNameIDF, panicking on error (as LoadGBM does).
func LoadNameIDF(path string) *NameIDFTable {
	t, err := readNameIDF(path)
	if err != nil {
		panic(err)
	}
	return t
}

// Weight: log(N / (1 + c(t))).
func (t *NameIDFTable) Weight(tok string) float64 {
	return math.Log(float64(t.Registrations) / float64(1+t.Counts[tok]))
}

// nameIDFFor: the table a process uses. featdump takes it only from -nameidf (pairfeat6 training dumps); every other
// command loads <wasl>/name_idf_pairfeat6.json when -modelset is pairfeat6 and uses none otherwise. -nameidf on any
// command other than featdump is a configuration error.
func nameIDFFor(cmd, path, modelSet, wasl string) (*NameIDFTable, error) {
	if cmd == "featdump" {
		if path == "" {
			return nil, nil
		}
		return readNameIDF(path)
	}
	if path != "" {
		return nil, fmt.Errorf("-nameidf is valid only for featdump (got command %q)", cmd)
	}
	if modelSet == "pairfeat6" {
		return readNameIDF(filepath.Join(wasl, "name_idf_pairfeat6.json"))
	}
	return nil, nil
}

func dobUsable6(d string) bool {
	return realDob(d) || (len(d) == 10 && d[8:] == "00" && realDob(d[:8]+"01"))
}

func dobPlaceholder6(d string) bool {
	return len(d) == 10 && (strings.HasSuffix(d, "-01-01") || strings.HasSuffix(d, "-07-01") || d[8:] == "00")
}

// pairfeat6Feats returns f30..f33 (see the file comment).
func pairfeat6Feats(a, b *Rec, t *NameIDFTable) []float64 {
	nan := math.NaN()
	out := []float64{nan, nan, nan, nan}
	if dobUsable6(a.Dob) && dobUsable6(b.Dob) {
		ph := dobPlaceholder6(a.Dob) || dobPlaceholder6(b.Dob)
		out[0] = boolFloat(ph)
		if !ph { // usable and not a placeholder: both are real dates
			out[1] = boolFloat(a.Dob == b.Dob)
		}
	}
	if len(a.skel) > 0 && len(b.skel) > 0 {
		set := func(s []string) map[string]bool {
			m := map[string]bool{}
			for _, x := range s {
				m[x] = true
			}
			return m
		}
		sa, sb := set(a.skel), set(b.skel)
		all := []string{}
		for x := range sa {
			all = append(all, x)
		}
		for x := range sb {
			if !sa[x] {
				all = append(all, x)
			}
		}
		sort.Strings(all)
		shared, rarest, unmatched := 0.0, 0.0, false
		for _, x := range all {
			w := t.Weight(x)
			if sa[x] && sb[x] {
				shared += w
			} else if !unmatched || w > rarest {
				rarest, unmatched = w, true
			}
		}
		out[2], out[3] = shared, rarest
	}
	return out
}
