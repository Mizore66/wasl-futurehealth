package main

// Replay-safe approximate nearest-neighbour blocking (v6).
//
// Why v4/v5 dropped HNSW: github.com/coder/hnsw draws node levels from a shared RNG and iterates Go maps during
// neighbour selection, so two replays of the same log built different graphs and returned different candidates.
//
// Two deterministic replacements, both behind the ANNIndex interface used by LinkerCore.candidates:
//
//  IVF (primary). A coarse codebook (spherical k-means, fixed seed) is trained ONCE on a disjoint sample and shipped
//  as a content-addressed artifact (SHA-256 in the event log / run manifest). It is never retrained during a run.
//  Each record goes to its nearest centroid's posting list; a query scans the nprobe nearest lists. Optional SQ8:
//  posting lists hold int8 codes (global scale fixed with the codebook) scored with INTEGER dot products, then the
//  top R are re-ranked with the float vectors the linker already holds. Result = top-K by (score desc, idx asc).
//  The result is a pure function of (codebook, set of inserted vectors): independent of insertion order, thread
//  count and CPU architecture (integer scoring; float dots use explicit float32() rounding so the Go compiler
//  cannot fuse multiply-adds into FMA, which differs between arm64 and amd64).
//
//  HNSW (secondary, own implementation). Node level = f(splitmix64(seed ^ idx)) instead of a shared RNG; all
//  neighbour lists are slices; every candidate ordering breaks ties by idx. The graph is a deterministic function
//  of the insertion sequence (which the linker fixes by stream seq). Snapshot = serialised graph + SHA-256.
import (
	"bytes"
	"container/heap"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"math/rand"
	"os"
	"sort"
)

type ANNIndex interface {
	Add(idx int, v []float32)
	Search(q []float32, k int) []int // record indices, best first
	Bytes() int64                    // approximate index-owned memory
	Snapshot() []byte
}

// dotR: float32 dot with per-product rounding (no FMA fusion) and fixed summation order -> bit-identical everywhere.
func dotR(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += float32(a[i] * b[i])
	}
	return s
}

type scored struct {
	j int
	s float32
}

// better: score desc, idx asc (total order -> deterministic)
func better(a, b scored) bool {
	if a.s != b.s {
		return a.s > b.s
	}
	return a.j < b.j
}

// topK keeps the k best scored items in a min-heap (worst at root).
type topK struct {
	k int
	h []scored
}

func (t *topK) Len() int           { return len(t.h) }
func (t *topK) Less(a, b int) bool { return better(t.h[b], t.h[a]) } // worst on top
func (t *topK) Swap(a, b int)      { t.h[a], t.h[b] = t.h[b], t.h[a] }
func (t *topK) Push(x any)         { t.h = append(t.h, x.(scored)) }
func (t *topK) Pop() any           { x := t.h[len(t.h)-1]; t.h = t.h[:len(t.h)-1]; return x }
func (t *topK) offer(x scored) {
	if len(t.h) < t.k {
		heap.Push(t, x)
	} else if better(x, t.h[0]) {
		t.h[0] = x
		heap.Fix(t, 0)
	}
}
func (t *topK) sorted() []scored {
	out := append([]scored{}, t.h...)
	sort.Slice(out, func(a, b int) bool { return better(out[a], out[b]) })
	return out
}

// ---------------- exact (v5 baseline) ----------------

type ExactIndex struct {
	vecs [][]float32
	ids  []int
}

func (e *ExactIndex) Add(idx int, v []float32) { e.vecs = append(e.vecs, v); e.ids = append(e.ids, idx) }
func (e *ExactIndex) Search(q []float32, k int) []int {
	t := &topK{k: k}
	for i, v := range e.vecs {
		t.offer(scored{e.ids[i], dotR(q, v)})
	}
	return idsOf(t.sorted())
}
func (e *ExactIndex) Bytes() int64      { return int64(len(e.ids)) * 8 }
func (e *ExactIndex) Snapshot() []byte  { return nil }
func idsOf(s []scored) []int {
	o := make([]int, len(s))
	for i, x := range s {
		o[i] = x.j
	}
	return o
}

// ---------------- codebook ----------------

type Codebook struct {
	D, N  int
	C     []float32 // N x D, unit-norm
	Scale float32   // SQ8 global scale (max |x| on training data / 127)
	SHA   string
}

func (c *Codebook) bytesLE() []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, int32(c.D))
	binary.Write(&b, binary.LittleEndian, int32(c.N))
	binary.Write(&b, binary.LittleEndian, c.Scale)
	binary.Write(&b, binary.LittleEndian, c.C)
	return b.Bytes()
}
func (c *Codebook) Save(path string) {
	b := c.bytesLE()
	h := sha256.Sum256(b)
	c.SHA = hex.EncodeToString(h[:])
	os.WriteFile(path, b, 0644)
}
func LoadCodebook(path string) *Codebook {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	r := bytes.NewReader(b)
	var d, n int32
	c := &Codebook{}
	binary.Read(r, binary.LittleEndian, &d)
	binary.Read(r, binary.LittleEndian, &n)
	binary.Read(r, binary.LittleEndian, &c.Scale)
	c.D, c.N = int(d), int(n)
	c.C = make([]float32, c.D*c.N)
	binary.Read(r, binary.LittleEndian, c.C)
	h := sha256.Sum256(b)
	c.SHA = hex.EncodeToString(h[:])
	return c
}

func normalize(v []float32) {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(s))
	if n == 0 {
		return
	}
	for i := range v {
		v[i] /= n
	}
}

// TrainCodebook: spherical k-means (k-means++ init, fixed seed), parallel assignment with a fixed-order reduction.
func TrainCodebook(X [][]float32, n int, iters int, seed int64, threads int) *Codebook {
	D := len(X[0])
	r := rand.New(rand.NewSource(seed))
	C := make([]float32, n*D)
	// k-means++ init on a sample for speed
	first := r.Intn(len(X))
	copy(C[:D], X[first])
	dist := make([]float64, len(X))
	for i := range dist {
		dist[i] = math.Inf(1)
	}
	for c := 1; c < n; c++ {
		prev := C[(c-1)*D : c*D]
		var tot float64
		parallelFor(len(X), threads, func(i int) {
			d := 1 - float64(dotR(X[i], prev))
			if d < dist[i] {
				dist[i] = d
			}
		})
		for _, d := range dist {
			tot += d
		}
		u := r.Float64() * tot
		pick := len(X) - 1
		for i, d := range dist {
			u -= d
			if u <= 0 {
				pick = i
				break
			}
		}
		copy(C[c*D:(c+1)*D], X[pick])
	}
	assign := make([]int, len(X))
	cb := &Codebook{D: D, N: n, C: C}
	for it := 0; it < iters; it++ {
		parallelFor(len(X), threads, func(i int) { assign[i] = cb.nearest(X[i]) })
		sum := make([]float64, n*D)
		cnt := make([]int, n)
		for i, a := range assign { // fixed order reduction
			cnt[a]++
			for d := 0; d < D; d++ {
				sum[a*D+d] += float64(X[i][d])
			}
		}
		for c := 0; c < n; c++ {
			if cnt[c] == 0 { // re-seed empty cluster deterministically
				copy(C[c*D:(c+1)*D], X[r.Intn(len(X))])
				continue
			}
			for d := 0; d < D; d++ {
				C[c*D+d] = float32(sum[c*D+d])
			}
			normalize(C[c*D : (c+1)*D])
		}
	}
	var mx float32
	for _, v := range X {
		for _, x := range v {
			if x < 0 {
				x = -x
			}
			if x > mx {
				mx = x
			}
		}
	}
	cb.Scale = mx / 127
	return cb
}

func (c *Codebook) nearest(v []float32) int {
	best, bs := 0, float32(-2)
	for i := 0; i < c.N; i++ {
		if s := dotR(v, c.C[i*c.D:(i+1)*c.D]); s > bs {
			best, bs = i, s
		}
	}
	return best
}

// nearestN returns the n best centroids ordered by (score desc, id asc).
func (c *Codebook) nearestN(v []float32, n int) []int {
	t := &topK{k: n}
	for i := 0; i < c.N; i++ {
		t.offer(scored{i, dotR(v, c.C[i*c.D:(i+1)*c.D])})
	}
	return idsOf(t.sorted())
}

func (c *Codebook) quant(v []float32) []int8 {
	q := make([]int8, len(v))
	for i, x := range v {
		y := math.Round(float64(x / c.Scale))
		if y > 127 {
			y = 127
		} else if y < -127 {
			y = -127
		}
		q[i] = int8(y)
	}
	return q
}

// ---------------- IVF ----------------

type ivfList struct {
	ids   []int32
	vecs  []float32 // flat mode: contiguous D floats per id
	codes []int8    // sq8 mode
}

type IVFIndex struct {
	cb      *Codebook
	NProbe  int
	SQ8     bool
	Rerank  int                // sq8: candidates re-ranked with float vectors
	vec     func(int) []float32 // sq8: record store (linker already holds every embedding)
	lists   []ivfList
	n       int
}

func NewIVF(cb *Codebook, nprobe int, sq8 bool, rerank int, vec func(int) []float32) *IVFIndex {
	return &IVFIndex{cb: cb, NProbe: nprobe, SQ8: sq8, Rerank: rerank, vec: vec, lists: make([]ivfList, cb.N)}
}

func (x *IVFIndex) Add(idx int, v []float32) {
	c := x.cb.nearest(v)
	l := &x.lists[c]
	l.ids = append(l.ids, int32(idx))
	if x.SQ8 {
		l.codes = append(l.codes, x.cb.quant(v)...)
	} else {
		l.vecs = append(l.vecs, v...)
	}
	x.n++
}

func (x *IVFIndex) Search(q []float32, k int) []int {
	D := x.cb.D
	probes := x.cb.nearestN(q, x.NProbe)
	if !x.SQ8 {
		t := &topK{k: k}
		for _, c := range probes {
			l := &x.lists[c]
			for i, id := range l.ids {
				t.offer(scored{int(id), dotR(q, l.vecs[i*D:(i+1)*D])})
			}
		}
		return idsOf(t.sorted())
	}
	qq := x.cb.quant(q)
	R := x.Rerank
	if R < k {
		R = k
	}
	t := &topK{k: R}
	for _, c := range probes {
		l := &x.lists[c]
		for i, id := range l.ids {
			cd := l.codes[i*D : (i+1)*D]
			var s int32
			for d := 0; d < D; d++ {
				s += int32(qq[d]) * int32(cd[d])
			}
			t.offer(scored{int(id), float32(s)}) // exact in float32 (|s| < 2^24)
		}
	}
	t2 := &topK{k: k}
	for _, c := range t.h {
		t2.offer(scored{c.j, dotR(q, x.vec(c.j))})
	}
	return idsOf(t2.sorted())
}

func (x *IVFIndex) Bytes() int64 {
	var b int64 = int64(len(x.cb.C)) * 4
	for _, l := range x.lists {
		b += int64(cap(l.ids))*4 + int64(cap(l.vecs))*4 + int64(cap(l.codes)) + 72
	}
	return b
}

// Snapshot: codebook SHA + posting lists (ids only for sq8 codes are re-derivable from the record store).
func (x *IVFIndex) Snapshot() []byte {
	var b bytes.Buffer
	b.WriteString(x.cb.SHA)
	for _, l := range x.lists {
		binary.Write(&b, binary.LittleEndian, int32(len(l.ids)))
		binary.Write(&b, binary.LittleEndian, l.ids)
	}
	return b.Bytes()
}

// RestoreIVF rebuilds lists from a snapshot + the record store (no re-assignment needed).
func RestoreIVF(cb *Codebook, snap []byte, nprobe int, sq8 bool, rerank int, vec func(int) []float32) *IVFIndex {
	x := NewIVF(cb, nprobe, sq8, rerank, vec)
	r := bytes.NewReader(snap[64:])
	for c := range x.lists {
		var n int32
		binary.Read(r, binary.LittleEndian, &n)
		ids := make([]int32, n)
		binary.Read(r, binary.LittleEndian, ids)
		l := &x.lists[c]
		l.ids = ids
		for _, id := range ids {
			if sq8 {
				l.codes = append(l.codes, cb.quant(vec(int(id)))...)
			} else {
				l.vecs = append(l.vecs, vec(int(id))...)
			}
			x.n++
		}
	}
	return x
}

// ---------------- deterministic HNSW ----------------

type HNSW struct {
	M, M0, EfC, EfS int
	Seed            uint64
	mL              float64
	vec             func(int) []float32
	level           []int8
	nbr             [][][]int32 // node -> layer -> neighbours (slices, never maps)
	ids             []int       // internal node -> record idx
	entry           int
	maxL            int
}

func NewHNSW(M, efC, efS int, seed uint64, vec func(int) []float32) *HNSW {
	return &HNSW{M: M, M0: 2 * M, EfC: efC, EfS: efS, Seed: seed, mL: 1 / math.Log(float64(M)), vec: vec, entry: -1}
}

func splitmix64(x uint64) uint64 {
	x += 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	return x ^ (x >> 31)
}

func (h *HNSW) levelOf(idx int) int {
	u := (float64(splitmix64(h.Seed^uint64(idx))>>11) + 0.5) / float64(1<<53)
	l := int(-math.Log(u) * h.mL)
	if l > 15 {
		l = 15
	}
	return l
}

func (h *HNSW) v(node int) []float32 { return h.vec(h.ids[node]) }

// maxHeap of candidates (best on top) for the search frontier
type candH []scored

func (c candH) Len() int           { return len(c) }
func (c candH) Less(a, b int) bool { return better(c[a], c[b]) }
func (c candH) Swap(a, b int)      { c[a], c[b] = c[b], c[a] }
func (c *candH) Push(x any)        { *c = append(*c, x.(scored)) }
func (c *candH) Pop() any          { o := *c; x := o[len(o)-1]; *c = o[:len(o)-1]; return x }

type visitSet struct {
	mark []uint32
	gen  uint32
}

func (h *HNSW) searchLayer(q []float32, eps []scored, ef, layer int, vs *visitSet) []scored {
	vs.gen++
	if len(vs.mark) < len(h.ids) {
		vs.mark = append(vs.mark, make([]uint32, len(h.ids)-len(vs.mark)+1024)...)
	}
	cand := &candH{}
	res := &topK{k: ef}
	for _, e := range eps {
		vs.mark[e.j] = vs.gen
		heap.Push(cand, e)
		res.offer(e)
	}
	for cand.Len() > 0 {
		c := heap.Pop(cand).(scored)
		if res.Len() >= ef && better(res.h[0], c) {
			break
		}
		for _, nb := range h.nbr[c.j][layer] {
			n := int(nb)
			if vs.mark[n] == vs.gen {
				continue
			}
			vs.mark[n] = vs.gen
			s := scored{n, dotR(q, h.v(n))}
			if res.Len() < ef || better(s, res.h[0]) {
				heap.Push(cand, s)
				res.offer(s)
			}
		}
	}
	return res.sorted()
}

// selectNbrs: Malkov heuristic, candidates processed in (score desc, id asc) order.
func (h *HNSW) selectNbrs(cands []scored, m int) []int32 {
	out := []int32{}
	kept := []int{}
	for _, c := range cands {
		if len(out) >= m {
			break
		}
		ok := true
		cv := h.v(c.j)
		for _, k := range kept {
			if dotR(cv, h.v(k)) > c.s {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, int32(c.j))
			kept = append(kept, c.j)
		}
	}
	// fill with skipped best candidates (keepPruned) for connectivity
	if len(out) < m {
		in := map[int]bool{}
		for _, k := range kept {
			in[k] = true
		}
		for _, c := range cands {
			if len(out) >= m {
				break
			}
			if !in[c.j] {
				out = append(out, int32(c.j))
			}
		}
	}
	return out
}

var hvs visitSet

func (h *HNSW) Add(idx int, q []float32) {
	node := len(h.ids)
	h.ids = append(h.ids, idx)
	L := h.levelOf(idx)
	h.level = append(h.level, int8(L))
	h.nbr = append(h.nbr, make([][]int32, L+1))
	if h.entry < 0 {
		h.entry, h.maxL = node, L
		return
	}
	ep := []scored{{h.entry, dotR(q, h.v(h.entry))}}
	for l := h.maxL; l > L; l-- {
		ep = h.searchLayer(q, ep, 1, l, &hvs)
	}
	for l := min(L, h.maxL); l >= 0; l-- {
		W := h.searchLayer(q, ep, h.EfC, l, &hvs)
		mmax := h.M
		if l == 0 {
			mmax = h.M0
		}
		ns := h.selectNbrs(W, h.M)
		h.nbr[node][l] = ns
		for _, nb := range ns {
			n := int(nb)
			lst := append(h.nbr[n][l], int32(node))
			if len(lst) > mmax {
				nv := h.v(n)
				cs := make([]scored, len(lst))
				for i, x := range lst {
					cs[i] = scored{int(x), dotR(nv, h.v(int(x)))}
				}
				sort.Slice(cs, func(a, b int) bool { return better(cs[a], cs[b]) })
				lst = h.selectNbrs(cs, mmax)
			}
			h.nbr[n][l] = lst
		}
		ep = W
	}
	if L > h.maxL {
		h.entry, h.maxL = node, L
	}
}

func (h *HNSW) Search(q []float32, k int) []int {
	if h.entry < 0 {
		return nil
	}
	ep := []scored{{h.entry, dotR(q, h.v(h.entry))}}
	for l := h.maxL; l > 0; l-- {
		ep = h.searchLayer(q, ep, 1, l, &hvs)
	}
	ef := h.EfS
	if ef < k {
		ef = k
	}
	W := h.searchLayer(q, ep, ef, 0, &hvs)
	if len(W) > k {
		W = W[:k]
	}
	o := make([]int, len(W))
	for i, w := range W {
		o[i] = h.ids[w.j]
	}
	return o
}

func (h *HNSW) Bytes() int64 {
	var b int64 = int64(len(h.ids))*(8+1+24) + int64(len(hvs.mark))*4
	for _, ls := range h.nbr {
		for _, l := range ls {
			b += int64(cap(l))*4 + 24
		}
	}
	return b
}

func (h *HNSW) Snapshot() []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, int64(h.entry))
	binary.Write(&b, binary.LittleEndian, int64(h.maxL))
	binary.Write(&b, binary.LittleEndian, int64(len(h.ids)))
	for i, id := range h.ids {
		binary.Write(&b, binary.LittleEndian, int64(id))
		binary.Write(&b, binary.LittleEndian, h.level[i])
		for _, l := range h.nbr[i] {
			binary.Write(&b, binary.LittleEndian, int32(len(l)))
			binary.Write(&b, binary.LittleEndian, l)
		}
	}
	return b.Bytes()
}

func RestoreHNSW(snap []byte, M, efC, efS int, seed uint64, vec func(int) []float32) *HNSW {
	h := NewHNSW(M, efC, efS, seed, vec)
	r := bytes.NewReader(snap)
	var e, ml, n int64
	binary.Read(r, binary.LittleEndian, &e)
	binary.Read(r, binary.LittleEndian, &ml)
	binary.Read(r, binary.LittleEndian, &n)
	h.entry, h.maxL = int(e), int(ml)
	for i := int64(0); i < n; i++ {
		var id int64
		var lv int8
		binary.Read(r, binary.LittleEndian, &id)
		binary.Read(r, binary.LittleEndian, &lv)
		h.ids = append(h.ids, int(id))
		h.level = append(h.level, lv)
		ls := make([][]int32, int(lv)+1)
		for l := range ls {
			var c int32
			binary.Read(r, binary.LittleEndian, &c)
			ls[l] = make([]int32, c)
			binary.Read(r, binary.LittleEndian, ls[l])
		}
		h.nbr = append(h.nbr, ls)
	}
	return h
}

func parallelFor(n, threads int, f func(i int)) {
	if threads <= 1 {
		for i := 0; i < n; i++ {
			f(i)
		}
		return
	}
	done := make(chan struct{})
	chunk := (n + threads - 1) / threads
	for t := 0; t < threads; t++ {
		go func(t int) {
			lo, hi := t*chunk, (t+1)*chunk
			if hi > n {
				hi = n
			}
			for i := lo; i < hi; i++ {
				f(i)
			}
			done <- struct{}{}
		}(t)
	}
	for t := 0; t < threads; t++ {
		<-done
	}
}

func shaB(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
