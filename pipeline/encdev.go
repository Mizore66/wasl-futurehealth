package main

// Encoder development harnesses (used by encoder/*.py; not part of the runtime path).
//   wsi encembed -encfile <encoder.json> -in names.txt -out emb.json   Go embeddings + normalised strings, one per input line
//   wsi encnoise -wasl <data dir> -split eval -seed 7 -out pairs.json   synthetic eval protocol: one noisy Arabic + one noisy
//                                                                        Latin registration per person (gen.go noise model v1, or v2 with -noise v2)
//   wsi featdump -wasl <data dir> -encfile <encoder.json> -split train -persons N -seed 1 -out feats.csv   pair-scorer rows
//   wsi nameidf -wasl <data dir> -split train -persons 5000 -noise v1t -idfseeds 1019,... -out name_idf_pairfeat6.json   pairfeat6 name-rarity table
import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
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

// validateModelPolicy: the pairfeat1/pairfeat2/pairfeat3 model sets are calibrated for the strict closure tier only, so any other
// policy (v7ltt's LTT-selected tau, v6, or none) is a configuration error rather than a silent mismatch.
func validateModelPolicy(modelSet, policy string) error {
	if (modelSet == "pairfeat1" || modelSet == "pairfeat2" || modelSet == "pairfeat3" || modelSet == "pairfeat4") && policy != "v7strict" {
		return fmt.Errorf("modelset %s supports only -policy v7strict (got %q); v7ltt needs a separate selection", modelSet, policy)
	}
	if modelSet == "pairfeat5" && policy != "v7strict" {
		return fmt.Errorf("modelset pairfeat5 supports only -policy v7strict (got %q); its cross-script review cap was selected under v7strict", policy)
	}
	if usesNameIDF(modelSet) && policy != "v7strict" { // pairfeat6, pairfeat7
		return fmt.Errorf("modelset %s supports only -policy v7strict (got %q); it keeps pairfeat5's cross-script review cap, selected under v7strict", modelSet, policy)
	}
	return nil
}

// validateNoise: v1t is the training-only DOB noise model, valid for featdump and nameidf; every other command
// (gen, evaluation runs) would silently mix training noise into evaluation data, so it is a configuration error.
func validateNoise(cmd, noise string) error {
	if noise == "v1t" && cmd != "featdump" && cmd != "nameidf" {
		return fmt.Errorf("-noise v1t is training-only and valid only for featdump and nameidf (got command %q)", cmd)
	}
	return nil
}

// modelPolicyArgs is the child-process forwarding of -modelset and -policy. Children re-run main(), so a child
// that receives -modelset must also receive -policy or validateModelPolicy rejects it. Both empty = no argv.
func modelPolicyArgs(modelSet, policy string) []string {
	var a []string
	if modelSet != "" {
		a = append(a, "-modelset", modelSet)
	}
	if policy != "" {
		a = append(a, "-policy", policy)
	}
	return a
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

// fdReg: one featdump registration (person index, name, DOB, sex, Emirates ID).
type fdReg struct {
	pid                 int
	name, dob, sex, eid string
	tid                 int // Person.Idx of the registering person
}

func readPersons(wasl string) []Person {
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var all []Person
	if err := json.Unmarshal(b, &all); err != nil {
		panic(err)
	}
	for i := range all {
		all[i].Idx = i + 1
	}
	return all
}

// fdSample: featdump's person sample: the split's persons with both names, shuffled with the seed and cut to nPersons,
// the split pool in file order, and the generator after the shuffle.
func fdSample(all []Person, split string, nPersons int, seed int64) ([]Person, []Person, *rand.Rand) {
	ps := []Person{}
	for _, p := range all {
		if p.Spl == split && p.Ar != "" && p.En != "" {
			ps = append(ps, p)
		}
	}
	pool := append([]Person(nil), ps...) // every person of the split, file order, before the shuffle
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(ps), func(i, j int) { ps[i], ps[j] = ps[j], ps[i] })
	if nPersons > 0 && nPersons < len(ps) {
		ps = ps[:nPersons]
	}
	return ps, pool, r
}

// fdName: one noisy registration name of p in the given script (gen.go noise: v2, else v1/v1t).
func fdName(p *Person, arabic bool, r *rand.Rand) string {
	if arabic {
		if NoiseModel == "v2" {
			return noisyArabic2(p.Ar, r)
		}
		return noisyArabic(p.Ar, r)
	}
	opts := append([]string{p.En}, p.Als...)
	src := p.En
	if r.Float64() < .5 {
		src = opts[r.Intn(len(opts))]
	}
	if NoiseModel == "v2" {
		return noisyLatin2(src, r)
	}
	return noisyLatin(src, r)
}

// fdRegister: p registers at 1-4 distinct providers (featdump's registration model); appended to regs.
func fdRegister(regs []fdReg, pid int, p *Person, r *rand.Rand) []fdReg {
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
		name := fdName(p, r.Float64() < prov.PAr, r)
		var dob string
		if NoiseModel == "v2" {
			dob = noisyDob2(p.Dob, r, .3)
		} else if NoiseModel == "v1t" {
			dob = noisyDobV1T(p.Dob, r, .3)
		} else {
			dob = noisyDob(p.Dob, r, .3)
		}
		sex := p.Sex
		if r.Float64() < .02 {
			sex = "U"
		}
		regs = append(regs, fdReg{pid, name, dob, sex, drawEID(p, r), p.Idx})
	}
	return regs
}

// runNameIDF: the pairfeat6 name-rarity table (pairfeat6.go). For each seed it draws featdump's person sample and base
// registrations exactly as featdump does (same code and RNG sequence; no namesake or cross-script registrations) and
// counts, per skeleton token, the registrations whose skeleton contains it; N is the registration total.
func runNameIDF(wasl, split, outPath string, nPersons int, seeds []int64) {
	if len(seeds) == 0 {
		panic("nameidf: -idfseeds is required")
	}
	all := readPersons(wasl)
	t := NameIDFTable{Version: 2, Own: map[string]map[string]int{}, OwnRegs: map[string]int{}, Split: split, Persons: nPersons, Seeds: seeds, Noise: NoiseModel, Counts: map[string]int{}}
	for _, seed := range seeds {
		ps, _, r := fdSample(all, split, nPersons, seed)
		var regs []fdReg
		for pid := range ps {
			regs = fdRegister(regs, pid, &ps[pid], r)
		}
		for _, g := range regs {
			seen := map[string]bool{}
			key := strconv.Itoa(g.tid - 1) // persons-file index
			if t.Own[key] == nil {
				t.Own[key] = map[string]int{}
			}
			for _, s := range skeleton(tokensOf(latinForm(g.name))) {
				if !seen[s] {
					seen[s] = true
					t.Counts[s]++
					t.Own[key][s]++
				}
			}
			t.OwnRegs[key]++
			t.Registrations++
		}
	}
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "nameidf %s: %d registrations, %d tokens\n", split, t.Registrations, len(t.Counts))
}

// parseIDFSeeds: -idfseeds "1019,1021,..." (empty items skipped).
func parseIDFSeeds(s string) []int64 {
	var out []int64
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x == "" {
			continue
		}
		v, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			panic("bad -idfseeds " + s)
		}
		out = append(out, v)
	}
	return out
}

// runFeatDump: training data for the pair scorer and its calibrators, produced exactly like the runtime linker
// (LinkerCore.candidates: exact top-K cosine + rule-key blocking, pairFeats). Persons of one split register at 1-4
// distinct providers (gen.go registration model: provider script mix, name/DOB/sex noise); registrations stream in
// random order and every candidate pair (new, earlier) is written once.
// CSV columns: label,corr,f0..f29,near_eid,decade_dob,same_surname,placeholder_dob,cross_script[,p from -gbmfile]
// (37 columns, 38 with p); with -nameidf (pairfeat6) f0..f33: 41 columns, 42 with p. Features and p are %.17g (exact float64 round-trip, NaN as NaN); slice columns are 0/1.
func runFeatDump(wasl, encPath, gbmPath, split, outPath string, nPersons int, seed int64) {
	var gbm *GBM
	if gbmPath != "" {
		gbm = LoadGBM(gbmPath)
	}
	ps, pool, r := fdSample(readPersons(wasl), split, nPersons, seed)
	regs := []fdReg{}
	register := func(pid int, p *Person, r *rand.Rand) { regs = fdRegister(regs, pid, p, r) }
	for pid := range ps {
		register(pid, &ps[pid], r)
	}
	nsBase := map[int]int{}
	nsAdded := 0
	if NamesakeRate > 0 {
		nr := rand.New(rand.NewSource(namesakeSeed(seed)))
		ns, base := buildNamesakes(ps, pool, NamesakeRate, nr)
		for i := range ns {
			register(len(ps)+i, &ns[i], nr)
			nsBase[len(ps)+i] = base[i]
		}
		nsAdded = len(ns)
	}
	nfBase := map[int]int{}
	nfAdded := 0
	if FamilyNamesakeRate > 0 {
		fr := rand.New(rand.NewSource(familySeed(seed)))
		nf, fbase := buildFamilyNamesakes(ps, pool, FamilyNamesakeRate, fr)
		for i := range nf {
			register(len(ps)+nsAdded+i, &nf[i], fr)
			nfBase[len(ps)+nsAdded+i] = fbase[i]
		}
		nfAdded = len(nf)
	}
	xsBase := map[int]int{}
	xsAdded := 0
	if XSNamesakeRate > 0 { // pairfeat6: base and cross-script namesake registrations (namesake.go)
		xr := rand.New(rand.NewSource(xsSeed(seed)))
		xs, xbase := buildXSNamesakes(ps, pool, XSNamesakeRate, xr)
		first := len(ps) + nsAdded + nfAdded
		for i := range xs {
			regs = xsRegister(regs, xbase[i], &ps[xbase[i]], first+i, &xs[i], xr)
			xsBase[first+i] = xbase[i]
		}
		xsAdded = len(xs)
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
	nsRows, nfRows, xsRows := []int{}, []int{}, []int{}
	rowIdx := 0
	for _, g := range regs {
		rec := &Rec{Name: g.name, Dob: g.dob, Sex: g.sex, Eid: g.eid, tid: g.tid}
		rec.eidOK = luhnOK(rec.Eid)
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
				if namesakePairRow(nsBase, g.pid, pids[c.j]) {
					nsRows = append(nsRows, rowIdx)
				}
				if namesakePairRow(nfBase, g.pid, pids[c.j]) {
					nfRows = append(nfRows, rowIdx)
				}
				if namesakePairRow(xsBase, g.pid, pids[c.j]) {
					xsRows = append(xsRows, rowIdx)
				}
			}
			corr := 0
			if corroborated(rec, L.recs[c.j], c.feat) {
				corr = 1
			}
			fmt.Fprintf(w, "%d,%d", lab, corr)
			for _, x := range c.feat {
				fmt.Fprintf(w, ",%.17g", x)
			}
			slices := []int{
				boolInt(!math.IsNaN(c.feat[23]) && (c.feat[23] == 1 || c.feat[23] == 2)),
				boolInt(c.feat[24] == 1),
				boolInt(c.feat[6] == 1),
				boolInt(placeholderDob(rec.Dob) || placeholderDob(L.recs[c.j].Dob)),
				boolInt(c.feat[20] == 0),
			}
			for _, x := range slices {
				fmt.Fprintf(w, ",%d", x)
			}
			if gbm != nil {
				fmt.Fprintf(w, ",%.17g", gbm.Predict(c.feat))
			}
			w.WriteByte('\n')
			rowIdx++
		}
		rec.idx = len(L.recs)
		L.recs = append(L.recs, rec)
		pids = append(pids, g.pid)
		for _, k := range rec.keys {
			L.keyIdx[k] = append(L.keyIdx[k], rec.idx)
		}
		if rec.eidOK {
			L.eidIdx[rec.Eid] = append(L.eidIdx[rec.Eid], rec.idx)
		}
	}
	w.Flush()
	f.Close()
	scm := map[string]any{"rate": NamesakeRate, "seed": seed, "namesakes_added": nsAdded,
		"base_namesake_rows": len(nsRows), "rows": nsRows}
	if FamilyNamesakeRate > 0 {
		scm["family_rate"], scm["family_namesakes_added"] = FamilyNamesakeRate, nfAdded
		scm["family_base_namesake_rows"], scm["family_rows"] = len(nfRows), nfRows
	}
	if XSNamesakeRate > 0 {
		scm["xs_rate"], scm["xs_namesakes_added"] = XSNamesakeRate, xsAdded
		scm["xs_base_namesake_rows"], scm["xs_rows"] = len(xsRows), xsRows
	}
	sc, err := json.Marshal(scm)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(outPath+".namesakes.json", append(sc, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "featdump %s: %d persons, %d registrations, %d positive / %d negative pairs\n", split, len(ps), len(regs), npos, nneg)
}
