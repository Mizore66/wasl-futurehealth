package main

// Encoder development harnesses (used by encoder/*.py; not part of the runtime path).
//   wsi encembed -encfile <encoder.json> -in names.txt -out emb.json   Go embeddings + normalised strings, one per input line
//   wsi encnoise -wasl <data dir> -split eval -seed 7 -out pairs.json   synthetic eval protocol: one noisy Arabic + one noisy
//                                                                        Latin registration per person (gen.go noise model v1, or v2 with -noise v2)
//   wsi featdump -wasl <data dir> -encfile <encoder.json> -split train -persons N -seed 1 -out feats.csv   pair-scorer rows
import (
	"bufio"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// ModelSet selects an alternative linkage model chain in the models dir (-modelset X): encoder_X.json,
// lgb_stream_X.json, iso_X.json and iso_v7_X.json replace the defaults. Empty = the default files.
var ModelSet = ""

func mf(base string) string {
	if ModelSet == "" {
		return base
	}
	if base == "encoder_weights.json" {
		return "encoder_" + ModelSet + ".json"
	}
	return strings.TrimSuffix(base, ".json") + "_" + ModelSet + ".json"
}

func runEncEmbed(encPath, inPath, outPath string) {
	enc := LoadEncoder(encPath)
	f, err := os.Open(inPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	type row struct {
		Name string    `json:"name"`
		Norm string    `json:"norm"`
		Emb  []float32 `json:"emb"`
	}
	out := []row{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		s := sc.Text()
		n := normLat(s)
		if isAr(s) {
			n = normAr(s)
		}
		out = append(out, row{s, n, enc.Embed(s)})
	}
	b, _ := json.Marshal(out)
	if err := os.WriteFile(outPath, b, 0644); err != nil {
		panic(err)
	}
}

func runEncNoise(wasl, split, outPath string, seed int64) {
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var ps []Person
	if err := json.Unmarshal(b, &ps); err != nil {
		panic(err)
	}
	r := rand.New(rand.NewSource(seed))
	type pair struct {
		Qid string `json:"qid"`
		Ar  string `json:"ar"`
		Lat string `json:"lat"`
	}
	out := []pair{}
	for _, p := range ps {
		if p.Spl != split || p.Ar == "" || p.En == "" {
			continue
		}
		opts := append([]string{p.En}, p.Als...)
		src := opts[r.Intn(len(opts))]
		var a, l string
		if NoiseModel == "v2" {
			a, l = noisyArabic2(p.Ar, r), noisyLatin2(src, r)
		} else {
			a, l = noisyArabic(p.Ar, r), noisyLatin(src, r)
		}
		out = append(out, pair{p.Qid, a, l})
	}
	bb, _ := json.Marshal(out)
	if err := os.WriteFile(outPath, bb, 0644); err != nil {
		panic(err)
	}
}

// runFeatDump: training data for the pair scorer and its calibrators, produced exactly like the runtime linker
// (LinkerCore.candidates: exact top-K cosine + rule-key blocking, pairFeats). Persons of one split register at 1-4
// distinct providers (gen.go registration model: provider script mix, name/DOB/sex noise); registrations stream in
// random order and every candidate pair (new, earlier) is written once.
// CSV columns: label (same person), corr (v7 corroborated), f0..f22[, p from -gbmfile].
func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, seed int64) {
	var gbm *GBM
	if gbmPath != "" {
		gbm = LoadGBM(gbmPath)
	}
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var all []Person
	if err := json.Unmarshal(b, &all); err != nil {
		panic(err)
	}
	ps := []Person{}
	for _, p := range all {
		if p.Spl == split && p.Ar != "" && p.En != "" {
			ps = append(ps, p)
		}
	}
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	if nPersons > 0 && nPersons < len(ps) {
		ps = ps[:nPersons]
	}
	type regT struct {
		pid            int
		name, dob, sex string
	}
	regs := []regT{}
	for pid, p := range ps {
		k := 1
		if x := r.Float64(); x < .1 {
			k = 4
		} else if x < .35 {
			k = 3
		} else if x < .7 {
			k = 2
		}
		for _, pi := range r.Perm(len(Providers))[:k] {
			prov := Providers[pi]
			var name string
			if r.Float64() < prov.PAr {
				if NoiseModel == "v2" {
					name = noisyArabic2(p.Ar, r)
				} else {
					name = noisyArabic(p.Ar, r)
				}
			} else {
				opts := append([]string{p.En}, p.Als...)
				src := p.En
				if r.Float64() < .5 {
					src = opts[r.Intn(len(opts))]
				}
				if NoiseModel == "v2" {
					name = noisyLatin2(src, r)
				} else {
					name = noisyLatin(src, r)
				}
			}
			var dob string
			if NoiseModel == "v2" {
				dob = noisyDob2(p.Dob, r, .3)
			} else {
				dob = noisyDob(p.Dob, r, .3)
			}
			sex := p.Sex
			if r.Float64() < .02 {
				sex = "U"
			}
			regs = append(regs, regT{pid, name, dob, sex})
		}
	}
	r.Shuffle(len(regs), func(i, j int) { regs[i], regs[j] = regs[j], regs[i] })
	enc := LoadEncoder(encPath)
	L := NewLinkerCore(nil, 1)
	f, err := os.Create(outPath)
	if err != nil {
		panic(err)
	}
	w := bufio.NewWriter(f)
	pids := []int{}
	npos, nneg := 0, 0
	for _, g := range regs {
		rec := &Rec{Name: g.name, Dob: g.dob, Sex: g.sex}
		rec.lat = latinForm(rec.Name)
		rec.tok = tokensOf(rec.lat)
		rec.skel = skeleton(rec.tok)
		rec.isAr = isAr(rec.Name)
		rec.keys = ruleKeys(rec)
		rec.emb = enc.Embed(rec.Name)
		for _, c := range L.candidates(rec) {
			lab := 0
			if pids[c.j] == g.pid {
				lab = 1
				npos++
			} else {
				nneg++
			}
			corr := 0
			if corroborated(rec, L.recs[c.j], c.feat) {
				corr = 1
			}
			fmt.Fprintf(w, "%d,%d", lab, corr)
			for _, x := range c.feat {
				fmt.Fprintf(w, ",%.9g", x)
			}
			if gbm != nil {
				fmt.Fprintf(w, ",%.9g", gbm.Predict(c.feat))
			}
			w.WriteByte('\n')
		}
		rec.idx = len(L.recs)
		L.recs = append(L.recs, rec)
		pids = append(pids, g.pid)
		for _, k := range rec.keys {
			L.keyIdx[k] = append(L.keyIdx[k], rec.idx)
		}
	}
	w.Flush()
	f.Close()
	fmt.Fprintf(os.Stderr, "featdump %s: %d persons, %d registrations, %d positive / %d negative pairs\n", split, len(ps), len(regs), npos, nneg)
}
