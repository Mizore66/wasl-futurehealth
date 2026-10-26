package main

// v6 ANN blocking benchmark.
//   wsi annsynth  -wasl <persons dir> -out <dir> -n 1000000 -seed 7   synthetic registrations (Wasl name pool + noise + dup persons) + embeddings
//   wsi annbench  -out <dir> -mode ivf|ivfsq8|hnsw|exact -n N ...     streaming query-then-insert, latency/memory/recall/determinism
import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type SynReg struct {
	Pid  int    `json:"pid"`
	Name string `json:"name"`
	Dob  string `json:"dob"`
	Org  string `json:"org"`
}

func luhnDigit(s string) byte {
	sum := 0
	for i := 0; i < len(s); i++ {
		d := int(s[len(s)-1-i] - '0')
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return byte('0' + (10-sum%10)%10)
}

// synthPersons: recombine aligned (Arabic, Latin) token sequences from the Wasl pool -> millions of distinct persons.
func synthPersons(pool []Person, n int, r *rand.Rand) []Person {
	byOrg := map[string][]int{}
	for i, p := range pool {
		if len(strings.Fields(p.Ar)) == len(strings.Fields(p.En)) && len(strings.Fields(p.En)) >= 2 {
			byOrg[p.Origin] = append(byOrg[p.Origin], i)
		}
	}
	out := make([]Person, n)
	for i := 0; i < n; i++ {
		a := pool[r.Intn(len(pool))]
		p := a
		p.Qid = fmt.Sprintf("S%d", i)
		p.Als = nil
		if lst := byOrg[a.Origin]; len(lst) > 0 && r.Float64() < 0.8 {
			b := pool[lst[r.Intn(len(lst))]]
			c := pool[lst[r.Intn(len(lst))]]
			bt, ba := strings.Fields(b.En), strings.Fields(b.Ar)
			ct, ca := strings.Fields(c.En), strings.Fields(c.Ar)
			// given name from b, family name(s) from c
			p.En = strings.Join(append([]string{bt[0]}, ct[1:]...), " ")
			p.Ar = strings.Join(append([]string{ba[0]}, ca[1:]...), " ")
			p.Sex = b.Sex
		}
		y := 1940 + r.Intn(66)
		p.Dob = fmt.Sprintf("%04d-%02d-%02d", y, 1+r.Intn(12), 1+r.Intn(28))
		e := fmt.Sprintf("784%04d%07d", y, r.Intn(10000000))
		p.Eid = e + string(luhnDigit(e))
		out[i] = p
	}
	return out
}

func synthRegs(pool []Person, nRegs int, seed int64) []SynReg {
	r := rand.New(rand.NewSource(seed))
	nP := int(float64(nRegs)/1.6) + 1000
	ps := synthPersons(pool, nP, r)
	regs := make([]SynReg, 0, nRegs+3)
	for pid, p := range ps {
		k := 1
		if x := r.Float64(); x < .15 {
			k = 3
		} else if x < .45 {
			k = 2
		}
		for j := 0; j < k; j++ {
			var name string
			if r.Float64() < .35 {
				name = noisyArabic(p.Ar, r)
			} else {
				opts := append([]string{p.En}, p.Als...)
				src := opts[r.Intn(len(opts))]
				name = noisyLatin(src, r)
			}
			regs = append(regs, SynReg{Pid: pid, Name: name, Dob: noisyDob(p.Dob, r, .3), Org: p.Origin})
		}
		if len(regs) >= nRegs {
			break
		}
	}
	regs = regs[:nRegs]
	r.Shuffle(len(regs), func(i, j int) { regs[i], regs[j] = regs[j], regs[i] })
	return regs
}

func runAnnSynth(wasl, models, out string, n int, seed int64, threads int) {
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var pool []Person
	json.Unmarshal(b, &pool)
	enc := LoadEncoder(filepath.Join(models, "encoder_weights.json"))
	os.MkdirAll(out, 0755)
	do := func(tag string, n int, seed int64) {
		t0 := time.Now()
		regs := synthRegs(pool, n, seed)
		f, _ := os.Create(filepath.Join(out, tag+"_regs.jsonl"))
		w := bufio.NewWriter(f)
		for _, rg := range regs {
			bb, _ := json.Marshal(rg)
			w.Write(bb)
			w.WriteByte('\n')
		}
		w.Flush()
		f.Close()
		embs := make([]float32, len(regs)*enc.OUT)
		t1 := time.Now()
		parallelFor(len(regs), threads, func(i int) {
			copy(embs[i*enc.OUT:(i+1)*enc.OUT], enc.Embed(regs[i].Name))
		})
		el := time.Since(t1).Seconds()
		g, _ := os.Create(filepath.Join(out, tag+"_emb.f32"))
		bw := bufio.NewWriter(g)
		binary.Write(bw, binary.LittleEndian, embs)
		bw.Flush()
		g.Close()
		fmt.Printf("%s: %d regs, gen+write %.1fs, embed %.1fs (%.0f names/s on %d threads)\n", tag, len(regs), t1.Sub(t0).Seconds(), el, float64(len(regs))/el, threads)
	}
	do("train", 60000, seed+1000) // codebook training data: disjoint synthetic persons, different seed
	do("bench", n, seed)
}

func loadEmb(path string, D int) [][]float32 {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	flat := make([]float32, len(b)/4)
	binary.Read(bytesReader(b), binary.LittleEndian, flat)
	n := len(flat) / D
	out := make([][]float32, n)
	for i := range out {
		out[i] = flat[i*D : (i+1)*D : (i+1)*D]
	}
	return out
}

func loadPids(path string) []int {
	f, _ := os.Open(path)
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	out := []int{}
	for sc.Scan() {
		var r SynReg
		json.Unmarshal(sc.Bytes(), &r)
		out = append(out, r.Pid)
	}
	return out
}

func pct(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64{}, xs...)
	sort.Float64s(s)
	i := int(p * float64(len(s)-1))
	return s[i]
}

type BenchCfg struct {
	Mode            string
	N, K            int
	NList, NProbe   int
	Rerank          int
	M, EfC, EfS     int
	RecallN         int // compute recall vs exact for every query up to this many inserted records
	ExactSample     int // after build: sample this many queries for exact-vs-ann recall at full size
	SnapAt          int // snapshot+restore at this point (0 = never)
	Shuffle         bool
	QEvery          int
}

func runAnnBench(dir string, c BenchCfg) map[string]interface{} {
	D := 64
	vecs := loadEmb(filepath.Join(dir, "bench_emb.f32"), D)
	pids := loadPids(filepath.Join(dir, "bench_regs.jsonl"))
	if c.N > len(vecs) {
		c.N = len(vecs)
	}
	vecs, pids = vecs[:c.N], pids[:c.N]
	order := make([]int, c.N)
	for i := range order {
		order[i] = i
	}
	if c.Shuffle { // permute insertion order (only meaningful for order-independence check of IVF on a prefix)
		r := rand.New(rand.NewSource(99))
		r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	}
	vec := func(j int) []float32 { return vecs[j] }
	var cb *Codebook
	var idx ANNIndex
	mk := func() ANNIndex {
		switch c.Mode {
		case "ivf":
			return NewIVF(cb, c.NProbe, false, 0, vec)
		case "ivfsq8":
			return NewIVF(cb, c.NProbe, true, c.Rerank, vec)
		case "hnsw":
			return NewHNSW(c.M, c.EfC, c.EfS, 0x5EED, vec)
		}
		return &ExactIndex{}
	}
	if strings.HasPrefix(c.Mode, "ivf") {
		cbPath := filepath.Join(dir, fmt.Sprintf("codebook_%d.bin", c.NList))
		if _, err := os.Stat(cbPath); err != nil {
			train := loadEmb(filepath.Join(dir, "train_emb.f32"), D)
			t0 := time.Now()
			cb = TrainCodebook(train, c.NList, 15, 2026, runtime.NumCPU())
			cb.Save(cbPath)
			fmt.Fprintf(os.Stderr, "trained codebook nlist=%d on %d vectors in %.1fs sha=%s\n", c.NList, len(train), time.Since(t0).Seconds(), cb.SHA[:16])
		}
		cb = LoadCodebook(cbPath)
	}
	idx = mk()
	exact := &ExactIndex{}
	H := newHasher()
	var qlat, ilat []float64
	win := map[string][2][]float64{}
	recHit, recTot := 0, 0
	pcAnn, pcEx, pcTot := 0, 0, 0
	seenP := map[int]bool{}
	mem := map[string]interface{}{}
	marks := map[int]string{100000: "100k", 1000000: "1M", 50000: "50k"}
	t0 := time.Now()
	for step, i := range order {
		v := vecs[i]
		var res []int
		qd := -1.0
		if c.QEvery <= 1 || step%c.QEvery == 0 {
			ts := time.Now()
			res = idx.Search(v, c.K)
			qd = time.Since(ts).Seconds() * 1e6
			H.add(step, res)
		}
		if step < c.RecallN {
			ex := exact.Search(v, c.K)
			set := map[int]bool{}
			for _, x := range ex {
				set[x] = true
			}
			for _, x := range res {
				if set[x] {
					recHit++
				}
			}
			recTot += len(ex)
			if seenP[pids[i]] { // pair completeness: does the candidate list contain an earlier registration of this person?
				pcTot++
				for _, x := range res {
					if pids[x] == pids[i] {
						pcAnn++
						break
					}
				}
				for _, x := range ex {
					if pids[x] == pids[i] {
						pcEx++
						break
					}
				}
			}
			exact.Add(i, v)
		}
		seenP[pids[i]] = true
		ts := time.Now()
		idx.Add(i, v)
		id := time.Since(ts).Seconds() * 1e6
		if qd >= 0 {
			qlat = append(qlat, qd)
		}
		ilat = append(ilat, id)
		n := step + 1
		if c.SnapAt > 0 && n == c.SnapAt {
			snap := idx.Snapshot()
			switch c.Mode {
			case "ivf":
				idx = RestoreIVF(cb, snap, c.NProbe, false, 0, vec)
			case "ivfsq8":
				idx = RestoreIVF(cb, snap, c.NProbe, true, c.Rerank, vec)
			case "hnsw":
				idx = RestoreHNSW(snap, c.M, c.EfC, c.EfS, 0x5EED, vec)
			}
			mem["snapshot_bytes"] = len(snap)
			mem["snapshot_sha"] = shaB(snap)[:16]
		}
		if tag, ok := marks[n]; ok {
			lo := n - 10000
			if lo < 0 {
				lo = 0
			}
			qlo := len(qlat) - 10000
			if qlo < 0 {
				qlo = 0
			}
			win[tag] = [2][]float64{append([]float64{}, qlat[qlo:]...), append([]float64{}, ilat[lo:n]...)}
			runtime.GC()
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			mem[tag] = map[string]interface{}{"index_MB": float64(idx.Bytes()) / 1e6, "vectors_MB": float64(n*D*4) / 1e6, "heap_MB": float64(ms.HeapAlloc) / 1e6,
				"elapsed_s": time.Since(t0).Seconds()}
			fmt.Fprintf(os.Stderr, "%s %s: %.1fs\n", c.Mode, tag, time.Since(t0).Seconds())
		}
	}
	out := map[string]interface{}{"cfg": c, "n": c.N, "build_s": time.Since(t0).Seconds(), "output_sha256": H.sum(), "mem": mem}
	if recTot > 0 {
		out["recall_at_k_vs_exact"] = float64(recHit) / float64(recTot)
		out["recall_queries"] = c.RecallN
		out["pair_completeness_ann"] = float64(pcAnn) / float64(pcTot)
		out["pair_completeness_exact"] = float64(pcEx) / float64(pcTot)
		out["pair_completeness_n"] = pcTot
	}
	lat := map[string]interface{}{}
	for tag, w := range win {
		lat[tag] = map[string]float64{"query_p50_us": pct(w[0], .5), "query_p99_us": pct(w[0], .99), "insert_p50_us": pct(w[1], .5), "insert_p99_us": pct(w[1], .99)}
	}
	out["latency_last10k"] = lat
	if c.ExactSample > 0 { // full-size recall + exact latency on a deterministic sample of queries
		r := rand.New(rand.NewSource(5))
		ex := &ExactIndex{vecs: vecs, ids: order}
		hit, tot := 0, 0
		var el []float64
		for s := 0; s < c.ExactSample; s++ {
			q := vecs[r.Intn(len(vecs))]
			ts := time.Now()
			e := ex.Search(q, c.K)
			el = append(el, time.Since(ts).Seconds()*1e6)
			a := idx.Search(q, c.K)
			set := map[int]bool{}
			for _, x := range e {
				set[x] = true
			}
			for _, x := range a {
				if set[x] {
					hit++
				}
			}
			tot += len(e)
		}
		out["fullsize_recall_at_k"] = float64(hit) / float64(tot)
		out["fullsize_exact_query_p50_us"] = pct(el, .5)
		out["fullsize_exact_query_p99_us"] = pct(el, .99)
	}
	return out
}

// ---------- helpers ----------

type hasher struct {
	mu  sync.Mutex
	buf []byte
	h   [][]byte
}

func newHasher() *hasher { return &hasher{} }
func (h *hasher) add(step int, ids []int) {
	b := make([]byte, 0, 8+8*len(ids))
	b = binary.LittleEndian.AppendUint64(b, uint64(step))
	for _, x := range ids {
		b = binary.LittleEndian.AppendUint64(b, uint64(x))
	}
	h.buf = append(h.buf, b...)
	if len(h.buf) > 1<<20 {
		h.flush()
	}
}
func (h *hasher) flush() {
	s := shaB(h.buf)
	h.h = append(h.h, []byte(s))
	h.buf = h.buf[:0]
}
func (h *hasher) sum() string {
	h.flush()
	all := []byte{}
	for _, x := range h.h {
		all = append(all, x...)
	}
	return shaB(all)
}
