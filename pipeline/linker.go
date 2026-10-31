package main

// Deterministic linker core (adapted from wasl/stream/engine.go). Differences vs Wasl:
//  - one record per (provider, MRN) registration; FHIR events attach to it
//  - exact top-K cosine search instead of HNSW (HNSW insertion iterates Go maps -> non-replayable); everything iterates sorted
//  - Emirates ID: Luhn-valid equal EIDs link deterministically; Luhn-valid conflicting EIDs become cannot-link
//  - emits per-loop LoopFacts (with widest-path link strength) whenever a loop's cluster or evidence changes
// The same code runs in the live linker process and in the offline audit replay.
import (
	"strconv"
	"sort"
	"strings"
)

type Rec struct {
	Pk, Prov, Kind string
	Name, Dob, Sex string
	Eid            string
	eidOK          bool
	idx            int
	lat            string
	tok, skel      []string
	isAr           bool
	emb            []float32
	keys           []string
	evOrder        []string
	evs            map[string]*Event
	srs            []string // loop ids owned by this record
	reportByBody   map[string]string // report content key -> event key (cross-transport duplicates)
}

type LinkerCore struct {
	P            int
	K, Cap       int
	Thr, Lo      float64
	GuardThr     float64
	gbm          *GBM
	recs         []*Rec
	byPk         map[string]int
	keyIdx       map[string][]int
	eidIdx       map[string][]int
	parent       []int
	members      map[int][]int
	adj          map[int]map[int]float64
	basis        map[[2]int]string
	idConfirmed  map[[2]int]bool // second-identifier "same" from any loop, per registration pair (ReuseIDConfirm)
	cannot       map[int]map[int]bool
	pscore       map[[2]int]float64
	near         map[int]map[int]float64 // lo <= p < thr
	loops        map[string]*LoopFacts   // static part (sr info)
	loopRec      map[string]int
	tasks        map[string][]TaskFact // loop -> tasks
	comms        map[string][]CommFact // loop -> communications
	signoffs     map[string][]Signoff
	evOver       map[string]map[string]string
	lastHash     map[string]string
	Stats        map[string]int
	provLast     map[string]float64
	provDone     map[string]bool
	lastTick     int
	PSeq         []uint64
	optOut       map[string]bool // v6: pk -> outreach consent withheld
	contact      map[string]string // stage 3: pk -> granted|denied|opted_out (absent = unknown)
	pcal7        map[[2]int]float64 // v7: calibrated probability of each model edge
	ann          ANNIndex
}

var annMode = new(string)
var annCB = new(string)

// attachANN: v6 replay-safe ANN blocking (exact remains the default so v5 runs are unchanged).
func (L *LinkerCore) attachANN() {
	vec := func(j int) []float32 { return L.recs[j].emb }
	switch *annMode {
	case "ivf", "ivfsq8":
		cb := LoadCodebook(*annCB)
		L.ann = NewIVF(cb, 16, *annMode == "ivfsq8", 100, vec)
	case "hnsw":
		L.ann = NewHNSW(16, 100, 64, 0x5EED, vec)
	}
}

func NewLinkerCore(gbm *GBM, P int) *LinkerCore {
	return &LinkerCore{P: P, K: 10, Cap: 300, Thr: LinkThr, Lo: 0.2, GuardThr: 0.25, gbm: gbm, byPk: map[string]int{}, keyIdx: map[string][]int{},
		eidIdx: map[string][]int{}, members: map[int][]int{}, adj: map[int]map[int]float64{}, basis: map[[2]int]string{}, idConfirmed: map[[2]int]bool{},
		cannot: map[int]map[int]bool{}, pscore: map[[2]int]float64{}, near: map[int]map[int]float64{}, loops: map[string]*LoopFacts{},
		loopRec: map[string]int{}, tasks: map[string][]TaskFact{}, comms: map[string][]CommFact{}, signoffs: map[string][]Signoff{}, evOver: map[string]map[string]string{},
		lastHash: map[string]string{}, Stats: map[string]int{},
		provLast: map[string]float64{}, provDone: map[string]bool{}, lastTick: -1, PSeq: make([]uint64, P), optOut: map[string]bool{}, contact: map[string]string{}, pcal7: map[[2]int]float64{}}
}

func NewLinkerCoreANN(gbm *GBM, P int) *LinkerCore {
	L := NewLinkerCore(gbm, P)
	L.attachANN()
	return L
}

// LinkThr: union threshold for the retrained mixed-origin scorer (v3 operating point, -thr 0.7).
var LinkThr = 0.7

func provKind(p string) string {
	for _, c := range Providers {
		if c.ID == p {
			return c.Kind
		}
	}
	return "other"
}

func pk2(a, b int) [2]int {
	if a > b {
		a, b = b, a
	}
	return [2]int{a, b}
}

func (L *LinkerCore) Find(x int) int {
	for L.parent[x] != x {
		L.parent[x] = L.parent[L.parent[x]]
		x = L.parent[x]
	}
	return x
}

func sortedKeys(m map[int]float64) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

func (L *LinkerCore) addEdge(a, b int, p float64, basis string) {
	if L.adj[a] == nil {
		L.adj[a] = map[int]float64{}
	}
	if L.adj[b] == nil {
		L.adj[b] = map[int]float64{}
	}
	if old, ok := L.adj[a][b]; ok && old >= p && basis != "human" {
		return
	}
	L.adj[a][b], L.adj[b][a] = p, p
	L.basis[pk2(a, b)] = basis
}

func (L *LinkerCore) isCannot(ra, rb int) bool {
	ma, mb := L.members[ra], L.members[rb]
	if len(ma) > len(mb) {
		ma, rb = mb, ra
	}
	for _, x := range ma {
		for y := range L.cannot[x] {
			if L.Find(y) == rb {
				return true
			}
		}
	}
	return false
}

func (L *LinkerCore) mergeAllowed(ra, rb int, force bool) bool {
	if L.isCannot(ra, rb) {
		L.Stats["cannot_rejects"]++
		return false
	}
	ma, mb := L.members[ra], L.members[rb]
	if force || (len(ma) == 1 && len(mb) == 1) {
		return true
	}
	s, n := 0.0, 0
	for _, x := range ma {
		for _, y := range mb {
			s += L.pscore[pk2(x, y)]
			n++
		}
	}
	if n > 0 && s/float64(n) < L.GuardThr {
		L.Stats["guard_rejects"]++
		return false
	}
	return true
}

func (L *LinkerCore) Link(a, b int, p float64, force bool, basis string) bool {
	ra, rb := L.Find(a), L.Find(b)
	if ra == rb {
		L.addEdge(a, b, p, basis)
		return false
	}
	if !L.mergeAllowed(ra, rb, force) {
		return false
	}
	L.addEdge(a, b, p, basis)
	if ra > rb { // deterministic root = smaller index
		ra, rb = rb, ra
	}
	L.parent[rb] = ra
	L.members[ra] = append(L.members[ra], L.members[rb]...)
	sort.Ints(L.members[ra])
	delete(L.members, rb)
	L.Stats["merges"]++
	return true
}

func (L *LinkerCore) bfsPath(a, b int) []int {
	prev := map[int]int{a: -1}
	q := []int{a}
	for len(q) > 0 {
		x := q[0]
		q = q[1:]
		if x == b {
			break
		}
		for _, y := range sortedKeys(L.adj[x]) {
			if _, ok := prev[y]; !ok {
				prev[y] = x
				q = append(q, y)
			}
		}
	}
	if _, ok := prev[b]; !ok {
		return nil
	}
	path := []int{}
	for x := b; x != -1; x = prev[x] {
		path = append(path, x)
	}
	return path
}

// CannotLink: cut weakest edge on each a-b path until separated, rebuild components. Returns roots touched.
func (L *LinkerCore) CannotLink(a, b int) []int {
	if L.cannot[a] == nil {
		L.cannot[a] = map[int]bool{}
	}
	if L.cannot[b] == nil {
		L.cannot[b] = map[int]bool{}
	}
	L.cannot[a][b], L.cannot[b][a] = true, true
	root := L.Find(a)
	if root != L.Find(b) {
		return nil
	}
	for {
		path := L.bfsPath(a, b)
		if path == nil {
			break
		}
		wi, wv := 0, 2.0
		for i := 0; i+1 < len(path); i++ {
			if v := L.adj[path[i]][path[i+1]]; v < wv {
				wv, wi = v, i
			}
		}
		x, y := path[wi], path[wi+1]
		delete(L.adj[x], y)
		delete(L.adj[y], x)
	}
	old := L.members[root]
	allowed := map[int]bool{}
	for _, x := range old {
		allowed[x] = true
	}
	delete(L.members, root)
	assigned := map[int]bool{}
	roots := []int{}
	for _, x := range old {
		if assigned[x] {
			continue
		}
		comp := []int{x}
		assigned[x] = true
		for q := 0; q < len(comp); q++ {
			for _, y := range sortedKeys(L.adj[comp[q]]) {
				if allowed[y] && !assigned[y] {
					assigned[y] = true
					comp = append(comp, y)
				}
			}
		}
		sort.Ints(comp)
		for _, y := range comp {
			L.parent[y] = comp[0]
		}
		L.members[comp[0]] = comp
		roots = append(roots, comp[0])
	}
	L.Stats["splits"]++
	return roots
}

type cand struct {
	j    int
	feat []float64
	p    float64
}

func (L *LinkerCore) candidates(r *Rec) []cand {
	seen := map[int]bool{}
	cs := []int{}
	if r.emb != nil && len(L.recs) > 0 && L.ann != nil {
		for _, j := range L.ann.Search(r.emb, L.K) {
			seen[j] = true
			cs = append(cs, j)
		}
	} else if r.emb != nil && len(L.recs) > 0 {
		type sc struct {
			j int
			s float64
		}
		top := make([]sc, 0, L.K+1)
		for j, o := range L.recs {
			if o.emb == nil {
				continue
			}
			s := dot(r.emb, o.emb)
			if len(top) < L.K || s > top[len(top)-1].s {
				top = append(top, sc{j, s})
				sort.SliceStable(top, func(a, b int) bool { return top[a].s > top[b].s })
				if len(top) > L.K {
					top = top[:L.K]
				}
			}
		}
		for _, t := range top {
			seen[t.j] = true
			cs = append(cs, t.j)
		}
	}
	for _, key := range r.keys {
		lst := L.keyIdx[key]
		if len(lst) > L.Cap {
			continue
		}
		for _, j := range lst {
			if !seen[j] {
				seen[j] = true
				cs = append(cs, j)
			}
		}
	}
	if r.eidOK {
		for _, j := range L.eidIdx[r.Eid] {
			if !seen[j] {
				seen[j] = true
				cs = append(cs, j)
			}
		}
	}
	out := make([]cand, len(cs))
	best := -2.0
	cos := make([]float64, len(cs))
	for i, j := range cs {
		if r.emb != nil && L.recs[j].emb != nil {
			cos[i] = dot(r.emb, L.recs[j].emb)
		}
		if cos[i] > best {
			best = cos[i]
		}
	}
	for i, j := range cs {
		out[i] = cand{j: j, feat: pairFeats(L.recs[j], r, cos[i], best-cos[i])}
	}
	return out
}

func dot(a, b []float32) float64 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return float64(s)
}

// reportContentKey: identity of a report's content for cross-transport de-duplication ("" for other events).
func reportContentKey(e *Event) string {
	if e.RType != "DiagnosticReport" {
		return ""
	}
	body := e.TextSha
	if body == "" {
		if e.Text == "" {
			return ""
		}
		body = sha([]byte(e.Text))
	}
	return e.Category + "|" + e.Focus + "|" + strconv.FormatFloat(e.Day, 'f', 3, 64) + "|" + e.Coded + "|" + body
}

func (L *LinkerCore) clusterOf(i int) []int { return L.members[L.Find(i)] }

// newRecord indexes a registration and links it. Returns affected roots.
func (L *LinkerCore) newRecord(e *Event, emb []float32) (int, []int) {
	r := &Rec{Pk: e.PatientKey, Prov: e.Provider, Kind: provKind(e.Provider), Name: e.Name, Dob: e.Dob, Sex: e.Sex, Eid: e.Eid, evs: map[string]*Event{}}
	r.eidOK = luhnOK(r.Eid)
	r.lat = latinForm(r.Name)
	r.tok = tokensOf(r.lat)
	r.skel = skeleton(r.tok)
	r.isAr = isAr(r.Name)
	r.keys = ruleKeys(r)
	r.emb = emb
	cs := L.candidates(r)
	if L.gbm != nil {
		for i := range cs {
			cs[i].p = L.gbm.Predict(cs[i].feat)
		}
	}
	r.idx = len(L.recs)
	L.recs = append(L.recs, r)
	L.byPk[r.Pk] = r.idx
	L.parent = append(L.parent, r.idx)
	L.members[r.idx] = []int{r.idx}
	for _, k := range r.keys {
		L.keyIdx[k] = append(L.keyIdx[k], r.idx)
	}
	if r.eidOK {
		L.eidIdx[r.Eid] = append(L.eidIdx[r.Eid], r.idx)
	}
	if L.ann != nil && r.emb != nil {
		L.ann.Add(r.idx, r.emb)
	}
	sort.SliceStable(cs, func(a, b int) bool {
		if cs[a].p != cs[b].p {
			return cs[a].p > cs[b].p
		}
		return cs[a].j < cs[b].j
	})
	aff := map[int]bool{}
	for _, c := range cs {
		L.pscore[pk2(r.idx, c.j)] = c.p
	}
	for _, c := range cs {
		o := L.recs[c.j]
		if r.eidOK && o.eidOK {
			if r.Eid == o.Eid {
				L.Link(r.idx, c.j, 1.0, true, "eid")
				L.Stats["eid_links"]++
				continue
			}
			L.cannot2(r.idx, c.j)
			L.Stats["eid_conflicts"]++
			continue
		}
		if c.p >= L.Thr {
			if V7Cal != nil {
				L.pcal7[pk2(r.idx, c.j)] = V7Cal.Predict(c.p, corroborated(r, o, c.feat))
			}
			L.Link(r.idx, c.j, c.p, false, "wasl")
		} else if c.p >= L.Lo {
			if L.near[r.idx] == nil {
				L.near[r.idx] = map[int]float64{}
			}
			if L.near[c.j] == nil {
				L.near[c.j] = map[int]float64{}
			}
			L.near[r.idx][c.j], L.near[c.j][r.idx] = c.p, c.p
			aff[L.Find(c.j)] = true
		}
	}
	aff[L.Find(r.idx)] = true
	return r.idx, keysOf(aff)
}

func (L *LinkerCore) cannot2(a, b int) {
	if L.cannot[a] == nil {
		L.cannot[a] = map[int]bool{}
	}
	if L.cannot[b] == nil {
		L.cannot[b] = map[int]bool{}
	}
	L.cannot[a][b], L.cannot[b][a] = true, true
}

func keysOf(m map[int]bool) []int {
	ks := []int{}
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

// widest-path (max bottleneck) from src within the cluster; returns strength, weakest edge, hops.
func (L *LinkerCore) widest(src int, members []int) (map[int]float64, map[int][2]int, map[int]int) {
	b, w, h, _ := L.widestP(src, members)
	return b, w, h
}

func (L *LinkerCore) widestP(src int, members []int) (map[int]float64, map[int][2]int, map[int]int, map[int]int) {
	pred := map[int]int{src: -1}
	in := map[int]bool{}
	for _, m := range members {
		in[m] = true
	}
	best := map[int]float64{src: 2}
	weak := map[int][2]int{}
	hops := map[int]int{src: 0}
	done := map[int]bool{}
	for {
		u, bu := -1, -1.0
		for _, m := range members {
			if v, ok := best[m]; ok && !done[m] && v > bu {
				u, bu = m, v
			}
		}
		if u < 0 {
			break
		}
		done[u] = true
		for _, y := range sortedKeys(L.adj[u]) {
			if !in[y] || done[y] {
				continue
			}
			w := L.adj[u][y]
			s := bu
			we := weak[u]
			if w < s {
				s = w
				we = [2]int{u, y}
			}
			if s > best[y] {
				best[y] = s
				weak[y] = we
				hops[y] = hops[u] + 1
				pred[y] = u
			}
		}
	}
	return best, weak, hops, pred
}

// pathCal (v7): min calibrated edge probability and all-human flag along the widest path src->y.
func (L *LinkerCore) pathCal(pred map[int]int, y int) (float64, bool) {
	pc, allHuman := 1.0, true
	for y >= 0 && pred[y] >= 0 {
		u := pred[y]
		k := pk2(u, y)
		e := 1.0
		switch L.basis[k] {
		case "human":
		case "eid":
			allHuman = false
		default:
			allHuman = false
			if v, ok := L.pcal7[k]; ok {
				e = v
			} else if V7Cal != nil {
				e = V7Cal.Predict(L.adj[u][y], false)
			}
		}
		if e < pc {
			pc = e
		}
		y = u
	}
	return pc, allHuman
}

func (L *LinkerCore) edgeOf(we [2]int) Edge {
	if we[0] == we[1] {
		return Edge{}
	}
	return Edge{A: L.recs[we[0]].Pk, B: L.recs[we[1]].Pk, P: round4(L.adj[we[0]][we[1]]), Basis: L.basis[pk2(we[0], we[1])]}
}

func round4(x float64) float64 { return float64(int64(x*10000+0.5)) / 10000 }

func (L *LinkerCore) downOf(rec *Rec, strength float64, weak Edge, hops int, near bool) []DownFact {
	out := []DownFact{}
	for _, k := range rec.evOrder {
		e := rec.evs[k]
		if e.RType != "Appointment" && e.RType != "DiagnosticReport" && e.RType != "ServiceRequest" {
			continue
		}
		if near && e.RType != "DiagnosticReport" {
			continue
		}
		rt := e.RType
		if rt == "DiagnosticReport" && e.Category == "pathology" {
			rt = "Pathology"
		}
		prog := e.Prog
		if e.RType == "DiagnosticReport" {
			prog = map[string]string{"colposcopy": "cervical", "lung-ct": "lung_nodule", "lung-finding": "lung_nodule", LungPETCode: "lung_nodule", LungPathCode: "lung_nodule"}[e.Category]
			if e.Category == "pathology" && e.Coded != "" {
				prog = "cervical"
			}
		}
		if prog == "" {
			prog = "crc"
		}
		out = append(out, DownFact{EventID: e.ID, Provider: e.Provider, RType: rt, Status: e.Status, Day: e.Day, ReportID: e.ReportID,
			Reason: e.Reason, Ev: e.Ev, Strength: round4(strength), Weak: weak, Hops: hops, NearMiss: near, Prog: prog})
		if rt == "Pathology" {
			out[len(out)-1].Parent = e.Focus
		}
	}
	return out
}

func (L *LinkerCore) factsFor(loop string) *LoopFacts {
	base := L.loops[loop]
	f := &LoopFacts{LoopID: loop, Provider: base.Provider, SRDay: base.SRDay, SRRec: base.SRRec, SRLang: base.SRLang, Tasks: L.tasks[loop], Comms: L.comms[loop], Down: []DownFact{},
		Prog: base.Prog, Priority: base.Priority, Cat: base.Cat, IndexDay: base.IndexDay,
		Signoffs: L.signoffs[loop], EvOver: L.evOver[loop], MsgOptOut: L.optOut[base.SRRec], Contact: contactOf(L.contact, base.SRRec)}
	s := L.loopRec[loop]
	ms := L.clusterOf(s)
	best, weak, hops, pred := L.widestP(s, ms)
	if V7Cal != nil || V7Guard {
		f.SRDob = L.recs[s].Dob
	}
	if base.Lung != nil {
		f.Lung = base.Lung
		f.Down = append(f.Down, lungFindingFacts(L.recs[s])...) // index CT on the referral's own registration
	}
	inC := map[int]bool{}
	for _, y := range ms {
		inC[y] = true
		if y == s || L.recs[y].Kind != "hospital" {
			continue
		}
		st, ok := best[y]
		if !ok {
			continue
		}
		ds := L.downOf(L.recs[y], st, L.edgeOf(weak[y]), hops[y], false)
		if V7Cal != nil || V7Guard {
			pc, ah := L.pathCal(pred, y)
			ah = ah || L.basis[pk2(s, y)] == "human"
			if V7Cal == nil {
				pc = 0
			}
			for i := range ds {
				ds[i].PCal, ds[i].AllHuman, ds[i].RecPk, ds[i].RecDob = round4(pc), ah, L.recs[y].Pk, L.recs[y].Dob
				ds[i].EIDSecond = V7Guard && L.eidSecond(s, y)
				ds[i].IDConfirmed = V7Guard && ReuseIDConfirm && L.idConfirmed[pk2(s, y)]
			}
		}
		f.Down = append(f.Down, ds...)
	}
	// near-miss hospital records (scored lo<=p<thr against any cluster member): candidates for a missed link
	nm := map[int][2]float64{}
	for _, x := range ms {
		for _, z := range sortedKeys(L.near[x]) {
			if inC[z] || L.recs[z].Kind != "hospital" || L.isCannot(L.Find(x), L.Find(z)) {
				continue
			}
			p := L.near[x][z]
			if cur, ok := nm[z]; !ok || p > cur[0] {
				nm[z] = [2]float64{p, float64(x)}
			}
		}
	}
	zs := []int{}
	for z := range nm {
		zs = append(zs, z)
	}
	sort.Ints(zs)
	for _, z := range zs {
		x := int(nm[z][1])
		e := Edge{A: L.recs[x].Pk, B: L.recs[z].Pk, P: round4(nm[z][0]), Basis: "wasl-subthreshold"}
		ds := L.downOf(L.recs[z], nm[z][0], e, 1, true)
		if V7Cal != nil || V7Guard {
			for i := range ds {
				if V7Cal != nil {
					ds[i].PCal = round4(V7Cal.Predict(nm[z][0], false))
				}
				ds[i].RecPk, ds[i].RecDob = L.recs[z].Pk, L.recs[z].Dob
			}
		}
		f.Down = append(f.Down, ds...)
	}
	return f
}

// Apply consumes one prepared FHIR message in sequence order and returns the facts messages to emit (deterministic order).
func (L *LinkerCore) Apply(p *Prep) []FactsMsg {
	out := []FactsMsg{}
	if p.Skip {
		return out
	}
	affRoots := map[int]bool{}
	affLoops := map[string]bool{}
	switch p.Kind {
	case "eof":
		L.provDone[p.Ev.Provider] = true
	case "signoff":
		so := p.So
		if err := CheckSignoff(so, DualMerge); err != nil {
			L.Stats["rbac_denied_signoff"]++
			break
		}
		L.Stats["signoffs"]++
		if so.Purpose == "second_identifier" {
			// messaging confirmation: "same" is recorded on the sign-off (outreach released), "different" keeps the
			// message held and is escalated to identity review; neither changes the clusters
			L.signoffs[so.LoopID] = append(L.signoffs[so.LoopID], *so)
			affLoops[so.LoopID] = true
			a, okA := L.byPk[so.Pair[0]]
			b, okB := L.byPk[so.Pair[1]]
			if ReuseIDConfirm && okA && okB {
				// the answer is about the two registrations, not the loop: it holds for the patient's other loops too
				if so.Link == "same" {
					L.idConfirmed[pk2(a, b)] = true
				} else if so.Link == "different" {
					delete(L.idConfirmed, pk2(a, b))
				}
				for _, i := range []int{a, b} {
					for _, lp := range L.recs[i].srs {
						affLoops[lp] = true
					}
				}
			}
			break
		}
		if so.Link != "" {
			a, okA := L.byPk[so.Pair[0]]
			b, okB := L.byPk[so.Pair[1]]
			if okA && okB {
				if so.Link == "same" {
					L.Link(a, b, 1.0, true, "human")
					affRoots[L.Find(a)] = true
				} else {
					for _, r := range L.CannotLink(a, b) {
						affRoots[r] = true
					}
					affRoots[L.Find(a)], affRoots[L.Find(b)] = true, true
				}
			}
		}
		if so.Evidence != "" && so.ReportID != "" {
			if L.evOver[so.LoopID] == nil {
				L.evOver[so.LoopID] = map[string]string{}
			}
			key := so.ReportID
			if so.Purpose == OverrideConfirm { // confirmation of an override against the report (closure.go)
				key += "#confirm"
			} else if k := pilotKey(so.Purpose); k != "" { // pilot shadow confirmation / post-closure audit (pilot.go)
				key += k
			}
			L.evOver[so.LoopID][key] = so.Evidence
		}
		L.signoffs[so.LoopID] = append(L.signoffs[so.LoopID], *so)
		affLoops[so.LoopID] = true
	case "event":
		e := p.Ev
		if p.Tick > L.provLast[e.Provider] {
			L.provLast[e.Provider] = p.Tick
		}
		if e.RType == "Task" {
			loop := e.Provider + "/" + strings.Replace(e.Focus, "ServiceRequest/", "ServiceRequest/", 1)
			L.tasks[loop] = append(L.tasks[loop], TaskFact{Day: e.Day, Status: e.Status, Owner: e.Owner})
			if _, ok := L.loops[loop]; ok {
				affLoops[loop] = true
			}
			break
		}
		if e.RType == "Communication" {
			loop := e.Provider + "/" + e.Focus
			cf := CommFact{ID: e.ID, Day: e.Day, Category: e.Category, Template: e.Template, Lang: e.CommLang, Channel: e.Channel, Status: e.Status, Parse: e.Parse, Sha: e.TextSha}
			if strings.HasPrefix(e.Category, "prep-") { // ungraded-prep query thread (closure.go): report + structured grade
				cf.Report = e.ReportID
				if e.Category == "prep-reply" {
					cf.Grade = e.Reason
				}
			}
			if e.Category == "econsult-question" { // why the e-consult was asked and which unit (handoff.go: no identity link needed)
				cf.Trigger, cf.Unit = e.Reason, e.Recipient
			}
			if e.Category == "handoff-reply" { // referrer's structured answer to a hand-off packet (handoff.go)
				cf.Action = e.Reason
			}
			if e.Category == "result-notification" || e.Category == "teachback-prompt" {
				cf.Text = e.Text // outbound: re-verified against the approved template; inbound free text stays in the FHIR log only
			}
			L.comms[loop] = append(L.comms[loop], cf)
			if _, ok := L.loops[loop]; ok {
				affLoops[loop] = true
			}
			break
		}
		if e.RType == "Consent" {
			if Authorize(e.Owner, ActRecordConsent) != nil {
				L.Stats["rbac_denied_consent"]++
				break
			}
			L.Stats["consent_records"]++
			L.optOut[e.PatientKey] = e.Status == "deny"
			prev := L.contact[e.PatientKey]
			switch e.Status { // idempotent: re-applying the same record (duplicate/replay) is a no-op
			case "permit":
				if prev != "opted_out" { // a stale permit never overrides a later withdrawal
					L.contact[e.PatientKey] = "granted"
				}
			case "deny":
				if prev == "granted" || prev == "opted_out" {
					L.contact[e.PatientKey] = "opted_out" // withdrawal after granting
				} else {
					L.contact[e.PatientKey] = "denied"
				}
			}
			if L.contact[e.PatientKey] != prev {
				L.Stats["contact_"+L.contact[e.PatientKey]]++
			}
			if i, ok := L.byPk[e.PatientKey]; ok {
				for _, lp := range L.recs[i].srs {
					affLoops[lp] = true
				}
			}
			break
		}
		if e.PatientKey == "" {
			break
		}
		i, ok := L.byPk[e.PatientKey]
		if !ok {
			var roots []int
			i, roots = L.newRecord(e, p.Emb)
			for _, r := range roots {
				affRoots[r] = true
			}
		}
		rec := L.recs[i]
		key := e.RType + "/" + e.ID
		if ck := reportContentKey(e); ck != "" {
			// the same result sent twice under two ids (a FHIR DiagnosticReport and an HL7 ORU with its own filler
			// number): same registration, kind, effective day and text. Keep the first; a correction keeps its id.
			if rec.reportByBody == nil {
				rec.reportByBody = map[string]string{}
			}
			if prev, ok := rec.reportByBody[ck]; ok && prev != key {
				L.Stats["duplicate_report_other_id"]++
				break
			}
			rec.reportByBody[ck] = key
		}
		if _, seen := rec.evs[key]; !seen {
			rec.evOrder = append(rec.evOrder, key)
		}
		rec.evs[key] = e
		if e.RType == "ServiceRequest" && e.Category != "referral-intake" {
			loop := e.Provider + "/ServiceRequest/" + e.ID
			if _, ok := L.loops[loop]; !ok {
				prog := e.Prog
				if prog == "" {
					prog = "crc"
				}
				L.loops[loop] = &LoopFacts{LoopID: loop, Provider: e.Provider, SRDay: e.Day, SRRec: rec.Pk, SRLang: e.PrefLang,
					Prog: prog, Priority: e.Priority, Cat: e.Cat, IndexDay: e.IndexDay, Lung: e.Lung}
				L.loopRec[loop] = i
				rec.srs = append(rec.srs, loop)
			}
		}
		affRoots[L.Find(i)] = true
		for z := range L.near[i] {
			affRoots[L.Find(z)] = true
		}
	}
	for _, r := range keysOf(affRoots) {
		for _, m := range L.members[r] {
			for _, lp := range L.recs[m].srs {
				affLoops[lp] = true
			}
		}
	}
	lps := []string{}
	for lp := range affLoops {
		lps = append(lps, lp)
	}
	sort.Strings(lps)
	for _, lp := range lps {
		if _, ok := L.loops[lp]; !ok {
			continue
		}
		f := L.factsFor(lp)
		h := sha(canon(f))
		if L.lastHash[lp] == h {
			continue
		}
		L.lastHash[lp] = h
		f.InSeq = p.Seq
		out = append(out, FactsMsg{ID: lp + "@" + utoa(p.Seq), InSeq: p.Seq, Facts: f, OriginTs: p.OriginTs, Part: partOf(lp, L.P)})
	}
	// event-time clock: watermark = min latest arrival over providers still streaming; one tick per whole day crossed
	wm, all := TEnd, true
	for _, pc := range Providers {
		if !L.provDone[pc.ID] {
			all = false
			if L.provLast[pc.ID] < wm {
				wm = L.provLast[pc.ID]
			}
		}
	}
	target := int(wm)
	if all {
		target = int(TEnd)
	}
	for d := L.lastTick + 1; d <= target; d++ {
		for part := 0; part < L.P; part++ {
			out = append(out, FactsMsg{ID: "tick-d" + itoa(d) + "-p" + itoa(part), InSeq: p.Seq, TickDay: float64(d), IsTick: true, Part: part})
		}
		L.lastTick = d
	}
	for i := range out {
		L.PSeq[out[i].Part]++
		out[i].PSeq = L.PSeq[out[i].Part]
	}
	return out
}

// Clusters returns pk -> root pk (for linkage metrics).
func (L *LinkerCore) Clusters() map[string]string {
	m := map[string]string{}
	for i, r := range L.recs {
		m[r.Pk] = L.recs[L.Find(i)].Pk
	}
	return m
}

func contactOf(m map[string]string, pk string) string {
	if s, ok := m[pk]; ok {
		return s
	}
	return "unknown"
}
