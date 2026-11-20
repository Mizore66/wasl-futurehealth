package main

// Public Arabic-English name-pair benchmark for the name encoder alone (evidence only; not part of the runtime path).
//
//   wsi namepairs -pairs dict_FINAL.json -encfile ../models/encoder_weights.json \
//       -netrans wd_arabic -variants wikidata_variants.jsonl -persons ../data/persons3.json -out result.json
//
// Data: Sayedyousef/arabic-name-pairs (Hugging Face, CC-BY-4.0), a flat English -> Arabic map of single-token names.
// The encoder never trained on it. Overlap with the encoder's training sources (NETransliteration wd_arabic, the mined
// Wikidata variants, persons3 canonical names and aliases; all splits, to be conservative) is removed after the
// runtime normalisers (normLat / normAr):
//   - tier "pair_dedup": drop a pair whose (English, Arabic) equals any training (Latin, Arabic) pair.
//   - tier "unseen_tokens": also drop a pair whose English string is a token of any training Latin string, or any of
//     whose Arabic tokens is a token of any training Arabic string (names the encoder never saw as a word).
// Per tier: ROC-AUC of cosine for positives vs hard negatives (another Arabic name in the tier with the same first
// letter and length within one letter) and vs random negatives; precision/recall at a cosine threshold (the runtime
// has no standalone encoder threshold, so it is chosen for best F1 on a random 20% of pairs and reported on the
// other 80%, 1:1 hard negatives); top-1/top-10 retrieval against the tier's whole gallery in both directions.
import (
	"bufio"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

type namePair struct{ En, Ar string } // normalised

type trainNames struct {
	pairs          map[string]bool // normLat + "\t" + normAr
	latTok, arTok  map[string]bool
	nPairs, nLines int
}

func newTrainNames() *trainNames {
	return &trainNames{pairs: map[string]bool{}, latTok: map[string]bool{}, arTok: map[string]bool{}}
}

func (t *trainNames) add(lats, ars []string) {
	nl, na := []string{}, []string{}
	for _, s := range lats {
		if n := normLat(s); n != "" {
			nl = append(nl, n)
			for _, w := range strings.Fields(n) {
				t.latTok[w] = true
			}
		}
	}
	for _, s := range ars {
		if n := normAr(s); n != "" {
			na = append(na, n)
			for _, w := range strings.Fields(n) {
				t.arTok[w] = true
			}
		}
	}
	for _, l := range nl {
		for _, a := range na {
			if !t.pairs[l+"\t"+a] {
				t.pairs[l+"\t"+a] = true
				t.nPairs++
			}
		}
	}
	t.nLines++
}

func loadTrainNames(netrans, variants, persons string) (*trainNames, error) {
	t := newTrainNames()
	f, err := os.Open(netrans)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		p := strings.SplitN(sc.Text(), "\t", 2)
		if len(p) == 2 {
			t.add([]string{p[0]}, []string{p[1]})
		}
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	f, err = os.Open(variants)
	if err != nil {
		return nil, err
	}
	sc = bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<22), 1<<22)
	for sc.Scan() {
		var v struct {
			Lat []string `json:"lat"`
			Ar  []string `json:"ar"`
		}
		if json.Unmarshal(sc.Bytes(), &v) == nil {
			t.add(v.Lat, v.Ar)
		}
	}
	f.Close()
	if err := sc.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(persons)
	if err != nil {
		return nil, err
	}
	var ps []struct {
		En  string   `json:"en"`
		Ar  string   `json:"ar"`
		Als []string `json:"als"`
	}
	if err := json.Unmarshal(b, &ps); err != nil {
		return nil, err
	}
	for _, p := range ps {
		lats, ars := []string{p.En}, []string{p.Ar}
		for _, a := range p.Als {
			if isAr(a) {
				ars = append(ars, a)
			} else {
				lats = append(lats, a)
			}
		}
		t.add(lats, ars)
	}
	return t, nil
}

// parseNamePairs normalises an English -> Arabic map into unique pairs, sorted for determinism.
func parseNamePairs(raw map[string]string) []namePair {
	seen := map[namePair]bool{}
	out := []namePair{}
	for en, ar := range raw {
		p := namePair{normLat(en), normAr(ar)}
		if p.En == "" || p.Ar == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].En != out[j].En {
			return out[i].En < out[j].En
		}
		return out[i].Ar < out[j].Ar
	})
	return out
}

func filterPairs(ps []namePair, t *trainNames, unseen bool) []namePair {
	out := []namePair{}
	for _, p := range ps {
		if t.pairs[p.En+"\t"+p.Ar] {
			continue
		}
		if unseen {
			seen := false
			for _, w := range strings.Fields(p.En) {
				seen = seen || t.latTok[w]
			}
			for _, w := range strings.Fields(p.Ar) {
				seen = seen || t.arTok[w]
			}
			if seen {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// aucMW is the Mann-Whitney ROC-AUC (ties count 1/2) with the Hanley-McNeil 95% interval.
func aucMW(pos, neg []float64) (auc, lo, hi float64) {
	type sv struct {
		s float64
		p bool
	}
	all := make([]sv, 0, len(pos)+len(neg))
	for _, s := range pos {
		all = append(all, sv{s, true})
	}
	for _, s := range neg {
		all = append(all, sv{s, false})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].s < all[j].s })
	rankSum := 0.0
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].s == all[i].s {
			j++
		}
		r := float64(i+j+1) / 2 // average 1-based rank
		for k := i; k < j; k++ {
			if all[k].p {
				rankSum += r
			}
		}
		i = j
	}
	np, nn := float64(len(pos)), float64(len(neg))
	if np == 0 || nn == 0 {
		return math.NaN(), math.NaN(), math.NaN()
	}
	auc = (rankSum - np*(np+1)/2) / (np * nn)
	q1, q2 := auc/(2-auc), 2*auc*auc/(1+auc)
	se := math.Sqrt((auc*(1-auc) + (np-1)*(q1-auc*auc) + (nn-1)*(q2-auc*auc)) / (np * nn))
	r := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	return r(auc), r(math.Max(0, auc-1.96*se)), r(math.Min(1, auc+1.96*se))
}

type npThreshold struct {
	Threshold  float64   `json:"cosine_threshold"`
	ChosenOn   string    `json:"chosen_on"`
	ReportedOn string    `json:"reported_on"`
	Prevalence float64   `json:"positive_share"`
	Precision  febrlRate `json:"precision"`
	Recall     febrlRate `json:"recall"`
	FalseMatch febrlRate `json:"false_match_rate"` // hard negatives above the threshold / hard negatives
}

type npRetrieval struct {
	Direction string    `json:"direction"`
	Gallery   int       `json:"gallery"`
	Top1      febrlRate `json:"top1"`
	Top10     febrlRate `json:"top10"`
}

type npTier struct {
	Tier          string        `json:"tier"`
	Pairs         int           `json:"pairs"`
	HardNegatives int           `json:"hard_negatives"`
	AUCHard       [3]float64    `json:"roc_auc_hard_negatives"` // value, low, high (Hanley-McNeil 95%)
	AUCRandom     [3]float64    `json:"roc_auc_random_negatives"`
	PairedWin     febrlRate     `json:"positive_beats_its_hard_negative"`
	Threshold     npThreshold   `json:"threshold"`
	Retrieval     []npRetrieval `json:"retrieval"`
}

type npResult struct {
	Dataset     string            `json:"dataset"`
	RawEntries  int               `json:"raw_entries"`
	Normalised  int               `json:"unique_normalised_pairs"`
	TrainPairs  int               `json:"training_pairs_checked"`
	DroppedPair int               `json:"dropped_exact_pair_overlap"`
	Tiers       []npTier          `json:"tiers"`
	Hashes      map[string]string `json:"sha256"`
	Seed        int64             `json:"seed"`
}

// retrieve counts queries whose top-1 / top-10 gallery item (by cosine) is a correct item.
func retrieve(q [][]float32, g [][]float32, correct func(qi, gi int) bool) (top1, top10 int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	ch := make(chan int, 256)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h1, h10 := 0, 0
			for qi := range ch {
				type sc struct {
					j int
					s float32
				}
				top := make([]sc, 0, 11)
				for j, gv := range g {
					s := dotR(q[qi], gv)
					if len(top) < 10 || s > top[len(top)-1].s || (s == top[len(top)-1].s && j < top[len(top)-1].j) {
						top = append(top, sc{j, s})
						sort.SliceStable(top, func(a, b int) bool {
							if top[a].s != top[b].s {
								return top[a].s > top[b].s
							}
							return top[a].j < top[b].j
						})
						if len(top) > 10 {
							top = top[:10]
						}
					}
				}
				for r, t := range top {
					if correct(qi, t.j) {
						if r == 0 {
							h1++
						}
						h10++
						break
					}
				}
			}
			mu.Lock()
			top1 += h1
			top10 += h10
			mu.Unlock()
		}()
	}
	for i := range q {
		ch <- i
	}
	close(ch)
	wg.Wait()
	return
}

func evalTier(name string, ps []namePair, embed func(string) []float32, seed int64) npTier {
	tier := npTier{Tier: name, Pairs: len(ps)}
	emb := map[string][]float32{}
	get := func(s string) []float32 {
		if v, ok := emb[s]; ok {
			return v
		}
		v := embed(s)
		emb[s] = v
		return v
	}
	cos := func(a, b string) float64 { return float64(dotR(get(a), get(b))) }
	// Arabic gallery, indexed by first letter for hard negatives
	arSet := map[string]bool{}
	for _, p := range ps {
		arSet[p.Ar] = true
	}
	ars := make([]string, 0, len(arSet))
	for a := range arSet {
		ars = append(ars, a)
	}
	sort.Strings(ars)
	byFirst := map[rune][]string{}
	for _, a := range ars {
		r, _ := utf8.DecodeRuneInString(a)
		byFirst[r] = append(byFirst[r], a)
	}
	rng := rand.New(rand.NewSource(seed))
	pos, hard, rnd := []float64{}, []float64{}, []float64{}
	type scoredPair struct{ p, n float64 }
	paired := []scoredPair{}
	for _, p := range ps {
		sp := cos(p.En, p.Ar)
		pos = append(pos, sp)
		r0, _ := utf8.DecodeRuneInString(p.Ar)
		ln := utf8.RuneCountInString(p.Ar)
		cands := []string{}
		for _, a := range byFirst[r0] {
			if a != p.Ar && int(math.Abs(float64(utf8.RuneCountInString(a)-ln))) <= 1 {
				cands = append(cands, a)
			}
		}
		if len(cands) > 0 {
			sn := cos(p.En, cands[rng.Intn(len(cands))])
			hard = append(hard, sn)
			paired = append(paired, scoredPair{sp, sn})
		} else {
			paired = append(paired, scoredPair{sp, math.NaN()})
		}
		for {
			a := ars[rng.Intn(len(ars))]
			if a != p.Ar || len(ars) == 1 {
				rnd = append(rnd, cos(p.En, a))
				break
			}
		}
	}
	tier.HardNegatives = len(hard)
	a, l, h := aucMW(pos, hard)
	tier.AUCHard = [3]float64{a, l, h}
	a, l, h = aucMW(pos, rnd)
	tier.AUCRandom = [3]float64{a, l, h}
	win, n := 0, 0
	for _, sp := range paired {
		if !math.IsNaN(sp.n) {
			n++
			if sp.p > sp.n {
				win++
			}
		}
	}
	tier.PairedWin = wilson95(win, n)
	// threshold: best F1 on a random 20% (selection), reported on the other 80% (positives + their hard negatives)
	idx := rng.Perm(len(paired))
	nSel := len(idx) / 5
	type lab struct {
		s float64
		y bool
	}
	collect := func(ix []int) []lab {
		out := []lab{}
		for _, i := range ix {
			if math.IsNaN(paired[i].n) {
				continue
			}
			out = append(out, lab{paired[i].p, true}, lab{paired[i].n, false})
		}
		return out
	}
	sel, rep := collect(idx[:nSel]), collect(idx[nSel:])
	best, bestF := 0.5, -1.0
	for t := -1.0; t <= 1.0; t += 0.005 {
		tp, fp, fn := 0, 0, 0
		for _, x := range sel {
			switch {
			case x.s >= t && x.y:
				tp++
			case x.s >= t && !x.y:
				fp++
			case x.y:
				fn++
			}
		}
		if tp == 0 {
			continue
		}
		f := 2 * float64(tp) / float64(2*tp+fp+fn)
		if f > bestF {
			best, bestF = math.Round(t*1000)/1000, f
		}
	}
	tp, fp, fn, nneg := 0, 0, 0, 0
	for _, x := range rep {
		if !x.y {
			nneg++
		}
		switch {
		case x.s >= best && x.y:
			tp++
		case x.s >= best && !x.y:
			fp++
		case x.y:
			fn++
		}
	}
	tier.Threshold = npThreshold{Threshold: best, ChosenOn: "random 20% of pairs, best F1", ReportedOn: "other 80%, 1:1 hard negatives",
		Prevalence: 0.5, Precision: wilson95(tp, tp+fp), Recall: wilson95(tp, tp+fn), FalseMatch: wilson95(fp, nneg)}
	// retrieval
	ens := []string{}
	enSet := map[string]bool{}
	arOf, enOf := map[string]map[string]bool{}, map[string]map[string]bool{}
	for _, p := range ps {
		if !enSet[p.En] {
			enSet[p.En] = true
			ens = append(ens, p.En)
		}
		if arOf[p.En] == nil {
			arOf[p.En] = map[string]bool{}
		}
		arOf[p.En][p.Ar] = true
		if enOf[p.Ar] == nil {
			enOf[p.Ar] = map[string]bool{}
		}
		enOf[p.Ar][p.En] = true
	}
	sort.Strings(ens)
	ev, av := make([][]float32, len(ens)), make([][]float32, len(ars))
	for i, s := range ens {
		ev[i] = get(s)
	}
	for i, s := range ars {
		av[i] = get(s)
	}
	t1, t10 := retrieve(ev, av, func(qi, gi int) bool { return arOf[ens[qi]][ars[gi]] })
	tier.Retrieval = append(tier.Retrieval, npRetrieval{"latin_to_arabic", len(ars), wilson95(t1, len(ens)), wilson95(t10, len(ens))})
	t1, t10 = retrieve(av, ev, func(qi, gi int) bool { return enOf[ars[qi]][ens[gi]] })
	tier.Retrieval = append(tier.Retrieval, npRetrieval{"arabic_to_latin", len(ens), wilson95(t1, len(ars)), wilson95(t10, len(ars))})
	return tier
}

func runNamePairs(pairsPath, encPath, netrans, variants, persons string, seed int64, outPath string) error {
	UrduFix = true // as main.go sets it for every runtime command (the encoder was trained with it)
	b, err := os.ReadFile(pairsPath)
	if err != nil {
		return err
	}
	raw := map[string]string{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	all := parseNamePairs(raw)
	t, err := loadTrainNames(netrans, variants, persons)
	if err != nil {
		return err
	}
	enc := LoadEncoder(encPath)
	dedup := filterPairs(all, t, false)
	unseen := filterPairs(all, t, true)
	res := npResult{Dataset: "Sayedyousef/arabic-name-pairs dict_FINAL.json", RawEntries: len(raw), Normalised: len(all),
		TrainPairs: t.nPairs, DroppedPair: len(all) - len(dedup), Seed: seed, Hashes: map[string]string{}}
	res.Tiers = append(res.Tiers, evalTier("pair_dedup", dedup, enc.Embed, seed), evalTier("unseen_tokens", unseen, enc.Embed, seed))
	for _, p := range []string{pairsPath, encPath, netrans, variants, persons} {
		bb, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		res.Hashes[p[strings.LastIndex(p, "/")+1:]] = sha(bb)
	}
	out, _ := json.MarshalIndent(res, "", " ")
	if outPath == "" || outPath == "-" {
		_, err = os.Stdout.Write(append(out, '\n'))
		return err
	}
	return os.WriteFile(outPath, append(out, '\n'), 0644)
}
