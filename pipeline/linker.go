package main

// Deterministic linker core (adapted from wasl/stream/engine.go). Differences vs Wasl:
//  - one record per (provider, MRN) registration; FHIR events attach to it
//  - exact top-K cosine search instead of HNSW (HNSW insertion iterates Go maps -> non-replayable); everything iterates sorted
//  - Emirates ID: Luhn-valid equal EIDs link deterministically; Luhn-valid conflicting EIDs become cannot-link
//  - emits per-loop LoopFacts (with widest-path link strength) whenever a loop's cluster or evidence changes
// The same code runs in the live linker process and in the offline audit replay.
import (
	"sort"
	"strconv"
	"strings"
)

type Rec struct {
	Pk, Prov, Kind    string
	Name, Dob, Sex    string
	Eid               string
	eidOK             bool
	idx               int
	lat               string
	tok, skel         []string
	isAr              bool
	tid               int // featdump only: 1 + persons-file index of the registering person, 0 = none (pairfeat6 leave-own-out)
	emb               []float32
	keys              []string
	evOrder           []string
	evs               map[string]*Event
	srs               []string          // loop ids owned by this record
	reportByBody      map[string]string // report content key -> event key (cross-transport duplicates)
	reportRetired     map[string]int    // event key -> cross-report version that superseded it
	reportBodyLatest  map[string]int    // normalized body -> newest accepted version, including withdrawals
	reportLineage     map[string]string // event key -> durable logical-report group, independent of later body edits
	reportGroupLatest map[string]int    // logical-report group -> newest accepted version
}

type trackingRevision struct {
	loop     string
	event    *Event
	rec      int // -1 until a patient registration is available
	version  int
	conflict bool
	tracking bool // any revision of this provider+ID was a root tracking order
}

type LinkerCore struct {
	P              int
	K, Cap         int
	Thr, Lo        float64
	GuardThr       float64
	gbm            *GBM
	recs           []*Rec
	byPk           map[string]int
	keyIdx         map[string][]int
	eidIdx         map[string][]int
	parent         []int
	members        map[int][]int
	adj            map[int]map[int]float64
	basis          map[[2]int]string
	idConfirmed    map[[2]int]bool // second-identifier "same" from any loop, per registration pair (ReuseIDConfirm)
	cannot         map[int]map[int]bool
	pscore         map[[2]int]float64
	near           map[int]map[int]float64      // lo <= p < thr
	demoMissing    map[[2]string]bool           // demoguard: pairs emitted as near/demographic_missing
	loops          map[string]*LoopFacts        // active tracking requests only: static part (sr info)
	requests       map[string]*trackingRevision // current ServiceRequest revisions, including category changes
	loopRec        map[string]int
	tasks          map[string][]TaskFact              // loop -> tasks
	comms          map[string][]CommFact              // loop -> current Communication revisions only
	commCurrent    map[string]communicationRevision   // provider|resource ID -> logical current revision, including reassociations
	commHistory    map[string][]communicationRevision // provider|resource ID -> append-only normalized inputs and their basedOn loop
	signoffs       map[string][]Signoff
	evOver         map[string]map[string]string
	lastHash       map[string]string
	Stats          map[string]int
	provLast       map[string]float64
	provDone       map[string]bool
	lastTick       int
	streamT        float64 // HistClock: latest input arrival day seen
	streamSeen     bool
	PSeq           []uint64
	optOut         map[string]bool     // pk -> registration-local denied/opted-out projection
	contact        map[string]string   // pk -> granted|denied|opted_out (absent = unknown)
	consentJournal map[string][]*Event // pk -> authorised active Consent journal, in ingest order only for storage
	consentSeen    map[string]struct{} // stable normalized payload -> already journalled
	pcal7          map[[2]int]float64  // v7: calibrated probability of each model edge
	xref           map[string][]int    // loop -> hospital records with a rejected intake citing its referral (handoffref.go)
	ann            ANNIndex
	trace          func(LinkTrace) // diagnostics only (link_funnel.go); nil = off
	traceSeq       uint64
}

var annMode = new(string)

// HistClock (flag -histclock, default off): judge look-back history at its own arrival time. The event-time clock
// below only ticks from day 0 (lastTick starts at -1 and a provider's latest arrival starts at 0), so the closure clock
// stands at day 0 from the first message on. Lung, DR and breast cohorts carry history from before day 0 (index CTs
// from day -420), and with the clock at day 0 a loop is judged due or overdue as soon as its index report arrives,
// months before it was due and before the coordinator's consent record that follows it: the time-based outreach
// is raised early, held for consent, released once the consent arrives, and often made moot by follow-up history
// that arrives later (results/showcase at cfcf50f: 233 of its 317 consent holds; final states are unchanged).
// On: every facts message carries the arrival day of the input that produced it (a sign-off: the latest arrival
// seen), and the closure judges the loop at that day only for pre-day-0 arrivals earlier than its clock. Live-day
// arrivals always use the current clock, so a late event or a sign-off stamped at an earlier stream day cannot rewind
// a decision. Ticks are unchanged: a loop still due at go-live is raised at the next tick.
var HistClock = false
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
	return &LinkerCore{P: P, K: 10, Cap: 300, Thr: LinkThr, Lo: LinkReview, GuardThr: 0.25, gbm: gbm, byPk: map[string]int{}, keyIdx: map[string][]int{},
		eidIdx: map[string][]int{}, members: map[int][]int{}, adj: map[int]map[int]float64{}, basis: map[[2]int]string{}, idConfirmed: map[[2]int]bool{},
		cannot: map[int]map[int]bool{}, pscore: map[[2]int]float64{}, near: map[int]map[int]float64{}, demoMissing: map[[2]string]bool{}, loops: map[string]*LoopFacts{}, requests: map[string]*trackingRevision{},
		loopRec: map[string]int{}, tasks: map[string][]TaskFact{}, comms: map[string][]CommFact{}, commCurrent: map[string]communicationRevision{}, commHistory: map[string][]communicationRevision{}, signoffs: map[string][]Signoff{}, evOver: map[string]map[string]string{},
		lastHash: map[string]string{}, Stats: map[string]int{},
		provLast: map[string]float64{}, provDone: map[string]bool{}, lastTick: -1, PSeq: make([]uint64, P), optOut: map[string]bool{}, contact: map[string]string{},
		consentJournal: map[string][]*Event{}, consentSeen: map[string]struct{}{}, pcal7: map[[2]int]float64{}, xref: map[string][]int{}}
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

// eidClash reports whether the two clusters hold registrations with different Luhn-valid Emirates IDs. A single-digit
// typo never keeps a number Luhn-valid, so two valid, different numbers belong to two people. Candidates are linked in
// score order, so the pairwise EID conflict (cannot2) is not enough on its own: a registration without an Emirates ID
// can bridge the two clusters first, and a later cannot-link then sits inside one cluster. X10: three of the six
// failures (two wrong-patient pairs and a false closure) were such clusters.
func (L *LinkerCore) eidClash(ra, rb int) bool {
	for _, x := range L.members[ra] {
		rx := L.recs[x]
		if rx.Eid == "" {
			continue
		}
		for _, y := range L.members[rb] {
			ry := L.recs[y]
			if (rx.eidOK && ry.eidOK && rx.Eid != ry.Eid) || eidContradict(rx.Eid, ry.Eid) {
				return true
			}
		}
	}
	return false
}

func (L *LinkerCore) Link(a, b int, p float64, force bool, basis string) (merged bool) {
	guardReason := ""
	if L.trace != nil {
		defer func() {
			e := LinkTrace{A: L.recs[a].Pk, B: L.recs[b].Pk, P: p, Stage: "linked"}
			if L.Find(a) != L.Find(b) {
				e.Stage, e.Reason = "rejected_demographic", "merge_constraint"
				if guardReason != "" {
					e.Reason = guardReason
				}
			}
			L.emitLink(e)
		}()
	}
	ra, rb := L.Find(a), L.Find(b)
	if ra == rb {
		L.addEdge(a, b, p, basis)
		return false
	}
	if basis != "human" && L.eidClash(ra, rb) {
		// Record the refusal as a cannot-link, like a direct EID contradiction: a later split (a reviewer's "different")
		// can take the clashing Emirates ID out of the cluster, and the refused pair must not become mergeable then.
		// A later human "same" on this pair is refused too (mergeAllowed checks cannot-links first): fail closed.
		L.cannot2(a, b)
		L.Stats["eid_cluster_clash"]++
		return false
	}
	humanMissing := false
	if DemoGuard { // demoguard.go: reviewers bypass only the missing-data requirement, never a known conflict
		d := demoClassify(L.recs[a], L.recs[b])
		if basis != "human" && d != demoAutoOK {
			return false
		}
		if L.demoClash(ra, rb) {
			L.cannot2(a, b)
			L.Stats["demoguard_cluster_conflict"]++
			guardReason = "demographic_cluster_conflict"
			return false
		}
		humanMissing = basis == "human" && d == demoNeedsReview
	}
	if !L.mergeAllowed(ra, rb, force) {
		return false
	}
	if humanMissing {
		L.Stats["demoguard_human_missing_merge"]++
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
		L.emitUnlinked(a, b)
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
	L.emitUnlinked(a, b)
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
	return float64(dotR(a, b))
}

// reportBodyKey identifies the normalized clinical content shared by cross-transport IDs.
func reportBodyKey(e *Event) string {
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
	return e.Category + "|" + e.Focus + "|" + e.Relation + "|" + strconv.FormatFloat(e.Day, 'f', 3, 64) + "|" + e.Coded + "|" + body
}

// reportContentKey adds revision semantics to exact cross-transport dedupe.
func reportContentKey(e *Event) string {
	if body := reportBodyKey(e); body != "" {
		return strconv.Itoa(e.Version) + "|" + e.Status + "|" + body
	}
	return ""
}

// retireReport suppresses an older cross-transport ID and its body ownership.
// The raw FHIR/audit log remains append-only; this is the linker's active view.
func retireReport(rec *Rec, key string, version int) {
	if rec.reportRetired == nil {
		rec.reportRetired = map[string]int{}
	}
	rec.reportRetired[key] = version
	previous := rec.evs[key]
	delete(rec.evs, key)
	for i, existing := range rec.evOrder {
		if existing == key {
			rec.evOrder = append(rec.evOrder[:i], rec.evOrder[i+1:]...)
			break
		}
	}
	if previous != nil {
		if contentKey := reportContentKey(previous); rec.reportByBody[contentKey] == key {
			delete(rec.reportByBody, contentKey)
		}
	}
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
		if r.eidOK && o.eidOK && r.Eid != o.Eid {
			L.cannot2(r.idx, c.j)
			L.Stats["eid_conflicts"]++
			L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: c.p, Stage: "rejected_demographic", Reason: "eid_conflict"})
			continue
		}
		demoReview, equalEID := false, false // demoguard.go: the pair needs review before any automatic merge
		if DemoGuard {
			switch demoClassify(r, o) {
			case demoDOBConflict, demoSexConflict:
				L.cannot2(r.idx, c.j)
				reason := "dob_conflict"
				if demoClassify(r, o) == demoSexConflict {
					reason = "sex_conflict"
				}
				L.Stats["demoguard_"+reason]++
				if r.eidOK && o.eidOK {
					L.Stats["demoguard_eid_rejected"]++
				}
				L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: c.p, Stage: "rejected_demographic", Reason: reason})
				continue
			case demoNeedsReview:
				demoReview = true
			}
		}
		if r.eidOK && o.eidOK {
			why := ""
			if EIDContraGuard {
				why = eidContradiction(r, o)
			}
			if why == "" {
				if demoReview {
					// demoguard.go: an equal valid number alone no longer forces the link; the pair goes to review with its
					// own model score (never raised), even below Lo
					L.Stats["demoguard_missing"]++
					L.Stats["demoguard_eid_missing"]++
					L.demoteNear(r, o, c.p, aff)
					continue
				}
				L.Link(r.idx, c.j, 1.0, true, "eid")
				L.Stats["eid_links"]++
				continue
			}
			// eidguard.go: the equal number sits on contradicting people; no forced link, the model score decides. Below Lo
			// the pair would be invisible, so it is kept as a near-miss candidate and the loop asks a reviewer.
			L.Stats["eid_contradicted_"+why]++
			equalEID = true
			// demoguard.go: a pair that needs review is never raised; the clamp stays for guard-off and exact-demographic pairs
			if min := 0.2; c.p < min && !demoReview { // fixed at 0.2 regardless of -linkfloor
				c.p = min
			}
		}
		if eidContradict(r.Eid, o.Eid) { // a typo broke the check digit on a side, but the numbers are clearly different
			L.cannot2(r.idx, c.j)
			L.Stats["eid_conflicts"]++
			L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: c.p, Stage: "rejected_demographic", Reason: "eid_contradiction"})
			continue
		}
		capped := false // xscript.go: cross-script review cap, pairfeat5 only
		if XScriptCap && c.p > XScriptCapP && xscriptWeak(r, o, c.feat) {
			c.p = XScriptCapP
			capped = true
			L.Stats["xscript_capped"]++
		}
		reason := ""
		if capped {
			reason = "xscript_cap"
		}
		if c.p >= L.Thr && demoReview {
			L.Stats["demoguard_missing"]++
			if equalEID {
				L.Stats["demoguard_eid_missing"]++
			}
			L.demoteNear(r, o, c.p, aff)
		} else if c.p >= L.Thr {
			if V7Cal != nil {
				L.pcal7[pk2(r.idx, c.j)] = V7Cal.Predict(c.p, corroborated(r, o, c.feat))
			}
			L.Link(r.idx, c.j, c.p, false, "wasl")
		} else if c.p >= L.Lo {
			L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: c.p, Stage: "near", Reason: reason})
			if L.near[r.idx] == nil {
				L.near[r.idx] = map[int]float64{}
			}
			if L.near[c.j] == nil {
				L.near[c.j] = map[int]float64{}
			}
			L.near[r.idx][c.j], L.near[c.j][r.idx] = c.p, c.p
			aff[L.Find(c.j)] = true
		} else {
			L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: c.p, Stage: "below_review", Reason: reason})
		}
	}
	if L.trace != nil { // transitive merges: candidates that ended in the new record's cluster
		for _, c := range cs {
			if L.Find(c.j) == L.Find(r.idx) {
				L.emitLink(LinkTrace{A: r.Pk, B: L.recs[c.j].Pk, P: c.p, Stage: "linked"})
			}
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
		if e.RType == "DiagnosticReport" && !diagnosticReportEvidenceEligible(e.Status) {
			continue
		}
		if e.RType == "ServiceRequest" && e.Category != "referral-intake" {
			// Tracking orders govern their own loop, never another loop's
			// receiving-unit response (including retired orders kept for audit).
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
			prog = map[string]string{"colonoscopy": "crc", "pathology": "crc", "colposcopy": "cervical", "lung-ct": "lung_nodule", "lung-finding": "lung_nodule", LungPETCode: "lung_nodule", LungPathCode: "lung_nodule",
				"dr-screen": "diabetic_retinopathy", "dr-clinic": "diabetic_retinopathy", "breast-screen": "breast", "breast-assess": "breast",
				"ldct-screen": "lung_ldct", "ldct-workup": "lung_ldct"}[e.Category]
			if e.Category == "pathology" && e.Coded != "" {
				prog = "cervical"
			}
			if e.Category == "ifhas-screen" || e.Category == "ifhas-followup" { // IFHAS results | follow-up note (ifhas.go)
				prog = "ifhas"
			}
		}
		if prog == "" {
			L.Stats["unknown_programme"]++
			continue
		}
		out = append(out, DownFact{EventID: e.ID, Provider: e.Provider, RType: rt, Status: e.Status, Day: e.Day, ReportID: e.ReportID, Version: e.Version,
			Reason: e.Reason, Ev: e.Ev, Strength: round4(strength), Weak: weak, Hops: hops, NearMiss: near, Prog: prog, FollowUp: e.FollowUp})
		if e.RType == "DiagnosticReport" {
			out[len(out)-1].Parent, out[len(out)-1].Relation = e.Focus, e.Relation
		}
	}
	return out
}

type nearPath struct {
	p            float64
	mid          int
	weakA, weakB int
	hops         int
}

// nearHospitalPaths returns the strongest one-hop near path to each hospital record. With demoguard on, it also
// considers a two-hop path through any intermediate registration: demoted missing-demographic links must not make an
// otherwise reviewable patient path invisible. The weakest pair is the one reviewers need to resolve first.
func (L *LinkerCore) nearHospitalPaths(ms []int, inC map[int]bool) map[int]nearPath {
	nm := map[int]nearPath{}
	better := func(z int, c nearPath) {
		old, ok := nm[z]
		if !ok || c.p > old.p || (c.p == old.p && (c.hops < old.hops ||
			(c.hops == old.hops && (c.mid < old.mid || (c.mid == old.mid && (c.weakA < old.weakA ||
				(c.weakA == old.weakA && c.weakB < old.weakB))))))) {
			nm[z] = c
		}
	}
	for _, x := range ms {
		for _, mid := range sortedKeys(L.near[x]) {
			if inC[mid] || L.Find(x) == L.Find(mid) || L.isCannot(L.Find(x), L.Find(mid)) {
				continue
			}
			p1 := L.near[x][mid]
			if L.recs[mid].Kind == "hospital" {
				better(mid, nearPath{p: p1, mid: x, weakA: x, weakB: mid, hops: 1})
			}
			if !DemoGuard {
				continue
			}
			for _, z := range sortedKeys(L.near[mid]) {
				if z == x || inC[z] || L.recs[z].Kind != "hospital" || L.isCannot(L.Find(x), L.Find(z)) {
					continue
				}
				if L.Find(mid) == L.Find(z) {
					// The final edge is already linked, so the unresolved first edge is the reviewable one.
					better(z, nearPath{p: p1, mid: mid, weakA: x, weakB: mid, hops: 2})
					continue
				}
				p2, p := L.near[mid][z], p1
				weakA, weakB := x, mid
				if p2 < p {
					p, weakA, weakB = p2, mid, z
				}
				better(z, nearPath{p: p, mid: mid, weakA: weakA, weakB: weakB, hops: 2})
			}
			for _, z := range sortedKeys(L.adj[mid]) {
				if z == x || inC[z] || L.recs[z].Kind != "hospital" || L.isCannot(L.Find(x), L.Find(z)) {
					continue
				}
				// An already-linked final edge is traversable even when it was never in the near map.
				better(z, nearPath{p: p1, mid: mid, weakA: x, weakB: mid, hops: 2})
			}
		}
	}
	return nm
}

// markNearDependents invalidates loops with a near edge into the newly changed cluster. Forward near propagation alone
// misses a clinic whose lab registration just merged with a hospital registration.
func (L *LinkerCore) markNearDependents(i int, aff map[int]bool) {
	members := L.members[L.Find(i)]
	for x, ns := range L.near {
		for _, m := range members {
			if _, ok := ns[m]; ok {
				aff[L.Find(x)] = true
				break
			}
		}
	}
}

func requestVersion(e *Event) int {
	if e.Version == 0 {
		return 1 // legacy materialized tracking requests
	}
	return e.Version
}

func sameRequestContent(a, b *Event) bool {
	ac, bc := *a, *b
	ac.Version, bc.Version = requestVersion(a), requestVersion(b)
	return string(canon(&ac)) == string(canon(&bc))
}

func removeLoopRegistration(rec *Rec, loop string) {
	for i, lp := range rec.srs {
		if lp == loop {
			rec.srs = append(rec.srs[:i], rec.srs[i+1:]...)
			return
		}
	}
}

func addLoopRegistration(rec *Rec, loop string) {
	for _, lp := range rec.srs {
		if lp == loop {
			return
		}
	}
	rec.srs = append(rec.srs, loop)
	sort.Strings(rec.srs)
}

func removeCurrentRequestEvent(rec *Rec, e *Event) {
	if rec == nil || e == nil {
		return
	}
	key := e.RType + "/" + e.ID
	delete(rec.evs, key)
	for i, k := range rec.evOrder {
		if k == key {
			rec.evOrder = append(rec.evOrder[:i], rec.evOrder[i+1:]...)
			break
		}
	}
}

func trackingBase(e *Event, rec *Rec) *LoopFacts {
	return &LoopFacts{LoopID: e.Provider + "/ServiceRequest/" + e.ID, Provider: e.Provider, SRDay: e.Day, SRRec: rec.Pk, SRLang: e.PrefLang,
		Prog: e.Prog, Priority: e.Priority, Cat: e.Cat, IndexDay: e.IndexDay, Lung: e.Lung, DR: e.DR, Breast: e.Breast, LDCT: e.LDCT, IFHAS: e.IFHAS}
}

// foldTrackingRequest folds every ServiceRequest revision before category
// routing. An intake-only ID never owns a loop, but an intake category correction
// must retire the same ID's old tracking order, even if delivered first.
func (L *LinkerCore) foldTrackingRequest(e *Event, recIdx int) (loop string, store, changed bool) {
	loop = e.Provider + "/ServiceRequest/" + e.ID
	version := requestVersion(e)
	tracking := e.Category != "referral-intake"
	if version <= 0 || (tracking && !trackingRequestStatuses[e.Status]) {
		// normaliseChecked rejects this before Prep; a direct/materialized caller
		// gets the same no-op quarantine and cannot supersede a valid revision.
		if version <= 0 {
			L.Stats["invalid_tracking_request_version"]++
		} else {
			L.Stats["invalid_tracking_request_status"]++
		}
		return loop, false, false
	}
	incoming := *e
	incoming.Version = version
	current, seen := L.requests[loop]
	var currentRec *Rec
	if seen && current.rec >= 0 {
		currentRec = L.recs[current.rec]
	}
	newTracking := seen && tracking && !current.tracking
	if seen {
		// A delayed old tracking revision establishes the resource's tracking
		// history, not its current category. Emit the current intake retirement.
		current.tracking = current.tracking || tracking
		tracking = current.tracking
		if newTracking && currentRec != nil {
			addLoopRegistration(currentRec, loop)
			L.loopRec[loop] = current.rec
		}
		if version < current.version {
			L.Stats["stale_tracking_request_version"]++
			return loop, false, newTracking
		}
		if version == current.version {
			if current.conflict {
				return loop, false, newTracking
			}
			if sameRequestContent(current.event, &incoming) {
				return loop, false, newTracking
			}
			// Same-version disagreement is a durable tombstone. Neither arrival
			// order nor another copy of either revision can select a winner.
			current.conflict = true
			delete(L.loops, loop)
			removeCurrentRequestEvent(currentRec, current.event)
			L.Stats["conflicting_tracking_request_version"]++
			return loop, false, tracking
		}
	}
	if currentRec != nil {
		removeLoopRegistration(currentRec, loop)
		removeCurrentRequestEvent(currentRec, current.event)
	}
	L.requests[loop] = &trackingRevision{loop: loop, event: &incoming, rec: recIdx, version: version, tracking: tracking}
	if recIdx < 0 {
		// Preserve an unbound revision by resource identity alone, including
		// active orders. Never let delayed older revisions bind or activate it.
		delete(L.loops, loop)
		delete(L.loopRec, loop)
		return loop, false, tracking
	}
	if !tracking {
		return loop, true, false // distinct intake responses retain downstream semantics
	}
	rec := L.recs[recIdx]
	addLoopRegistration(rec, loop)
	L.loopRec[loop] = recIdx
	if e.Prog == "" {
		L.Stats["unknown_programme"]++
		delete(L.loops, loop)
		return loop, true, true
	}
	if e.Status == "active" && e.Category != "referral-intake" {
		L.loops[loop] = trackingBase(&incoming, rec)
	} else {
		delete(L.loops, loop)
	}
	return loop, true, true
}

// Revisions can remove facts from a different registration than the incoming
// one. Invalidate both sides, including near-link and explicit-reference
// dependents, even for intake-only IDs and same-version conflicts. Rejected
// stale/duplicate revisions remain no-ops through the facts hash guard.
func (L *LinkerCore) foldRequestAndInvalidate(e *Event, recIdx int, roots map[int]bool, loops map[string]bool) (string, bool, bool) {
	invalidate := func(rec int, event *Event) {
		if rec >= 0 {
			roots[L.Find(rec)] = true
			L.markNearDependents(rec, roots)
			for near := range L.near[rec] {
				roots[L.Find(near)] = true
			}
		}
		if XRefLinkQ && event != nil && event.XRef != "" {
			loops[event.XRef] = true
		}
	}
	if previous := L.requests[e.Provider+"/ServiceRequest/"+e.ID]; previous != nil {
		invalidate(previous.rec, previous.event)
	}
	loop, store, changed := L.foldTrackingRequest(e, recIdx)
	invalidate(recIdx, e)
	return loop, store, changed
}

func (L *LinkerCore) retirementFacts(loop string) *LoopFacts {
	r := L.requests[loop]
	if r == nil || !r.tracking {
		return nil
	}
	f := &LoopFacts{LoopID: loop, Provider: r.event.Provider, Inactive: true, ReqStatus: r.event.Status, ReqVersion: r.version, ReqConflict: r.conflict}
	if !r.conflict {
		f.SRDay, f.Prog = r.event.Day, r.event.Prog
		if r.rec >= 0 {
			f.SRRec = L.recs[r.rec].Pk
		}
	} else {
		// Conflicts have no clinically selected request body or status.
		f.ReqStatus = "conflict"
	}
	return f
}

func (L *LinkerCore) factsFor(loop string) *LoopFacts {
	base := L.loops[loop]
	s := L.loopRec[loop]
	contact, optedOut := L.clusterConsent(s)
	f := &LoopFacts{LoopID: loop, Provider: base.Provider, SRDay: base.SRDay, SRRec: base.SRRec, SRLang: base.SRLang, Tasks: L.tasks[loop], Comms: L.comms[loop], Down: []DownFact{},
		Prog: base.Prog, Priority: base.Priority, Cat: base.Cat, IndexDay: base.IndexDay,
		Signoffs: L.signoffs[loop], EvOver: L.evOver[loop], MsgOptOut: optedOut, Contact: contact}
	ms := L.clusterOf(s)
	best, weak, hops, pred := L.widestP(s, ms)
	if V7Cal != nil || V7Guard {
		f.SRDob = L.recs[s].Dob
	}
	if base.Lung != nil {
		f.Lung = base.Lung
		f.Down = append(f.Down, lungFindingFacts(L.recs[s])...) // index CT on the referral's own registration
	}
	if base.DR != nil {
		f.DR = base.DR
		var screens []DownFact
		screens, f.EpisodeError = drScreenFacts(L.recs[s], base.DR.Screen)
		f.Down = append(f.Down, screens...)
	}
	if base.Breast != nil {
		f.Breast = base.Breast
		var screens []DownFact
		screens, f.EpisodeError = breastScreenFacts(L.recs[s], base.Breast.Screen)
		f.Down = append(f.Down, screens...)
	}
	if base.LDCT != nil {
		f.LDCT = base.LDCT
		f.Down = append(f.Down, ldctScanFacts(L.recs[s])...) // LDCT reports (index, completion, follow-up) on the scan's own registration
	}
	if base.Prog == "ifhas" {
		f.IFHAS = base.IFHAS
		index := ""
		if base.IFHAS != nil {
			index = base.IFHAS.Screen
		}
		var screens []DownFact
		screens, f.EpisodeError = ifhasScreenFacts(L.recs[s], index)
		f.Down = append(f.Down, screens...)
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
		if EIDContraGuard {
			if why, x := L.eidPathContradiction(s, y, pred); why != "" {
				// the question pair is a registration the path passes through, not necessarily a graph edge: its P is the
				// pair's real model score (0 when never scored) and the provenance block says it is substituted
				we := Edge{A: L.recs[s].Pk, B: L.recs[x].Pk, P: round4(L.pscore[pk2(s, x)]), Basis: "eid-contradiction"}
				for i := range ds {
					ds[i].EIDContra, ds[i].Weak = why, we
				}
			}
		}
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
	if XRefLinkQ {
		f.Down = append(f.Down, L.xrefFacts(loop, s, inC)...)
	}
	// near-miss hospital records (scored lo<=p<thr against any cluster member): candidates for a missed link
	nm := L.nearHospitalPaths(ms, inC)
	zs := []int{}
	for z := range nm {
		zs = append(zs, z)
	}
	sort.Ints(zs)
	for _, z := range zs {
		np := nm[z]
		e := Edge{A: L.recs[np.weakA].Pk, B: L.recs[np.weakB].Pk, P: round4(np.p), Basis: "wasl-subthreshold"}
		ds := L.downOf(L.recs[z], np.p, e, np.hops, true)
		if V7Cal != nil || V7Guard {
			for i := range ds {
				if V7Cal != nil {
					ds[i].PCal = round4(V7Cal.Predict(np.p, false))
				}
				ds[i].RecPk, ds[i].RecDob = L.recs[z].Pk, L.recs[z].Dob
			}
		}
		f.Down = append(f.Down, ds...)
	}
	return f
}

type communicationRevision struct {
	Loop string
	Fact CommFact
}

// Fold at provider/resource identity, never at basedOn membership. A newer
// reassociation or withdrawal retracts the old loop's effective revision too.
// Missing versions and v1 are equivalent. Conflicting same-version payloads
// remain inert until a greater version; every original input stays append-only.
func (L *LinkerCore) foldCommunication(loop string, cf CommFact) []string {
	provider, _, _ := strings.Cut(loop, "/")
	key := provider + "|" + cf.ID
	next := communicationRevision{Loop: loop, Fact: cf}
	L.commHistory[key] = append(L.commHistory[key], next)
	version := func(v int) int {
		if v <= 1 {
			return 1
		}
		return v
	}
	next.Fact.Version = version(cf.Version)
	current, exists := L.commCurrent[key]
	if exists {
		cv, nv := version(current.Fact.Version), version(cf.Version)
		if nv < cv {
			L.Stats["stale_communication_version"]++
			return nil
		}
		if nv == cv {
			a, b := current, next
			a.Fact.Version, b.Fact.Version = 0, 0
			if string(canon(a)) == string(canon(b)) {
				L.Stats["duplicate_communication_version"]++
				return nil
			}
			// The tombstone's loop is the lexically first conflicting membership,
			// so reverse arrival and repeated conflicts yield the same projection.
			if current.Loop < next.Loop {
				next.Loop = current.Loop
			}
			next.Fact = CommFact{ID: cf.ID, Version: nv, Status: "conflicting_revision"}
			L.Stats["conflicting_communication_version"]++
		}
	}
	// Copy slices rather than editing previously emitted LoopFacts in place.
	if exists {
		remaining := []CommFact{}
		for _, old := range L.comms[current.Loop] {
			if old.ID != cf.ID {
				remaining = append(remaining, old)
			}
		}
		if len(remaining) == 0 {
			delete(L.comms, current.Loop)
		} else {
			L.comms[current.Loop] = remaining
		}
	}
	updated := append([]CommFact{}, L.comms[next.Loop]...)
	updated = append(updated, next.Fact)
	sort.Slice(updated, func(i, j int) bool {
		a, b := updated[i], updated[j]
		if a.Day != b.Day {
			return a.Day < b.Day
		}
		if a.Category != b.Category {
			return a.Category < b.Category // opening questions/prompts precede same-day replies
		}
		return a.ID < b.ID
	})
	L.comms[next.Loop], L.commCurrent[key] = updated, next
	affected := []string{loop, next.Loop}
	if exists {
		affected = append(affected, current.Loop)
	}
	return uniqueSorted(affected)
}

// Apply consumes one prepared FHIR message in sequence order and returns the facts messages to emit (deterministic order).
func (L *LinkerCore) Apply(p *Prep) []FactsMsg {
	out := []FactsMsg{}
	L.traceSeq = p.Seq
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
		if secondIDPurpose(so.Purpose) {
			// messaging confirmation: "same" is recorded on the sign-off (outreach released), "different" keeps the
			// message held and is escalated to identity review; neither changes the clusters
			a, okA := L.byPk[so.Pair[0]]
			b, okB := L.byPk[so.Pair[1]]
			if okA && okB && so.Link == "same" && LinkContradictionConfirm && SecondIDContradictionConfirm && so.Purpose == "second_identifier" {
				if c := linkContradiction(L.recs[a], L.recs[b]); c != "" {
					// the records contradict: the confirmation counts only after a confirming review (closure raises IDConfirm)
					sc := *so
					sc.Hold = c
					so = &sc
					L.Stats["second_id_same_held_contradiction"]++
				}
			}
			L.signoffs[so.LoopID] = append(L.signoffs[so.LoopID], *so)
			affLoops[so.LoopID] = true
			if ReuseIDConfirm && okA && okB {
				// the answer is about the two registrations, not the loop: it holds for the patient's other loops too
				// (a held "same" neither confirms nor withdraws an earlier confirmation of the pair)
				if so.Link == "same" && so.Hold == "" {
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
			if okA && okB && so.Link == "same" && LinkContradictionConfirm && so.Purpose != LinkConfirm {
				if c := linkContradiction(L.recs[a], L.recs[b]); c != "" {
					// the records contradict: hold the merge until a confirming review (closure raises LinkConfirm)
					sc := *so
					sc.Hold = c
					so = &sc
					L.Stats["link_same_held_contradiction"]++
					okA = false
				}
			}
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
			if so.Purpose == CRCDispositionPurpose {
				// CRC dispositions never become generic EvOver adequacy labels.
				if so.ItemID != crcDispositionItemID(so.LoopID, so.ReportID, so.ReportVersion, so.SignedHead) || !crcReviewDecisionLabel(so.Evidence) {
					L.Stats["mismatched_crc_disposition_signoff"]++
					break
				}
			} else {
				if so.ReportVersion >= 2 && so.ItemID != evidenceReviewItemID(so.LoopID, so.ReportID, so.ReportVersion, so.Purpose) {
					L.Stats["mismatched_evidence_signoff"]++
					break
				}
				if L.evOver[so.LoopID] == nil {
					L.evOver[so.LoopID] = map[string]string{}
				}
				key := reportVersionKey(so.ReportID, so.ReportVersion)
				if so.Purpose == OverrideConfirm { // confirmation of an override against the report (closure.go)
					key += "#confirm"
				} else if k := pilotKey(so.Purpose); k != "" { // pilot shadow confirmation / post-closure audit (pilot.go)
					key += k
				}
				L.evOver[so.LoopID][key] = so.Evidence
			}
		}
		L.signoffs[so.LoopID] = append(L.signoffs[so.LoopID], *so)
		affLoops[so.LoopID] = true
	case "event":
		e := p.Ev
		if p.Tick > L.provLast[e.Provider] {
			L.provLast[e.Provider] = p.Tick
		}
		if !L.streamSeen || p.Tick > L.streamT {
			L.streamT, L.streamSeen = p.Tick, true
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
			cf := CommFact{ID: e.ID, Version: e.Version, Day: e.Day, Category: e.Category, Template: e.Template, Lang: e.CommLang, Channel: e.Channel, Status: e.Status, Parse: e.Parse, Sha: e.TextSha, ReportVersion: e.ReportVersion}
			revision := *e
			revision.Version = 0 // legacy missing version and explicit v1 share a revision identity
			cf.RevisionSha = sha(canon(&revision))
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
			for _, affected := range L.foldCommunication(loop, cf) {
				if _, ok := L.loops[affected]; ok {
					affLoops[affected] = true
				}
			}
			break
		}
		if e.RType == "Consent" {
			if Authorize(e.Owner, ActRecordConsent) != nil {
				L.Stats["rbac_denied_consent"]++
				break
			}
			if e.ConsentStatus == "" {
				// Historical normalized events carry no lifecycle proof. They remain
				// visible in the raw log but cannot authorize an outreach transition.
				L.Stats["consent_legacy_inert"]++
				break
			}
			if e.ConsentStatus != "active" || !supportedConsentProvision(e.Status) {
				L.Stats["consent_inert"]++
				break
			}
			added, previous, current := L.addConsent(e)
			if !added {
				break
			}
			L.Stats["consent_records"]++
			if current != previous {
				L.Stats["contact_"+current]++
			}
			if i, ok := L.byPk[e.PatientKey]; ok {
				// A changed record can affect every loop in the trusted cluster,
				// including when its clinical time predates the current arrival.
				affRoots[L.Find(i)] = true
			}
			break
		}
		if e.PatientKey == "" {
			if e.RType == "ServiceRequest" {
				loop := e.Provider + "/ServiceRequest/" + e.ID
				recIdx := -1
				if current := L.requests[loop]; current != nil {
					recIdx = current.rec
				}
				// Fold every revision even before any patient is known, so a
				// delayed older active request cannot supply stale scope or bind it.
				// Activation still requires an established registration.
				_, store, changed := L.foldRequestAndInvalidate(e, recIdx, affRoots, affLoops)
				if recIdx >= 0 {
					affRoots[L.Find(recIdx)] = true
				}
				if changed {
					affLoops[loop] = true
				}
				if store {
					rec := L.recs[recIdx]
					key := e.RType + "/" + e.ID
					if _, exists := rec.evs[key]; !exists {
						rec.evOrder = append(rec.evOrder, key)
					}
					rec.evs[key] = e
				}
			}
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
		if e.RType == "ServiceRequest" {
			loop, store, changed := L.foldRequestAndInvalidate(e, i, affRoots, affLoops)
			affRoots[L.Find(i)] = true
			if changed {
				affLoops[loop] = true
			}
			if !store {
				break
			}
		}
		var previousReport *Event
		if e.RType == "DiagnosticReport" {
			if retired := rec.reportRetired[key]; retired > 0 && e.Version <= retired {
				L.Stats["stale_report_version"]++
				break
			}
			if current, seen := rec.evs[key]; seen {
				previousReport = current
				switch {
				case e.Version < current.Version:
					L.Stats["stale_report_version"]++
					break
				case e.Version == current.Version:
					if string(canon(e)) != string(canon(current)) {
						retireReport(rec, key, e.Version)
						L.Stats["conflicting_report_version"]++
						affRoots[L.Find(i)] = true
					}
					break
				}
				if e.Version <= current.Version {
					break
				}
			}
		}
		contentKey := reportContentKey(e)
		if contentKey != "" {
			// Exact same-generation cross-transport copies dedupe. A newer revision of
			// an already stored ID must instead reach the supersession logic below.
			if rec.reportByBody == nil {
				rec.reportByBody = map[string]string{}
			}
			if prev, ok := rec.reportByBody[contentKey]; ok && prev != key && previousReport == nil {
				if group := rec.reportLineage[prev]; group != "" {
					if rec.reportLineage == nil {
						rec.reportLineage = map[string]string{}
					}
					// An exact duplicate has no active facts of its own, but its ID remains an
					// alias of the logical report so a later changed-body revision supersedes it.
					rec.reportLineage[key] = group
				}
				L.Stats["duplicate_report_other_id"]++
				break
			}
		}
		bodyKey := reportBodyKey(e)
		group := ""
		if bodyKey != "" {
			for _, otherKey := range rec.evOrder {
				other := rec.evs[otherKey]
				if otherKey != key && other != nil && reportBodyKey(other) == bodyKey {
					group = rec.reportLineage[otherKey]
					if group != "" {
						break
					}
				}
			}
		}
		if group == "" {
			group = rec.reportLineage[key]
		}
		if group == "" && e.RType == "DiagnosticReport" {
			group = key
		}
		if group != "" {
			if rec.reportLineage == nil {
				rec.reportLineage = map[string]string{}
			}
			if rec.reportGroupLatest == nil {
				rec.reportGroupLatest = map[string]int{}
			}
			latest, seen := rec.reportGroupLatest[group]
			if seen && e.Version < latest {
				retireReport(rec, key, latest)
				L.Stats["stale_report_version"]++
				affRoots[L.Find(i)] = true
				break
			}
			if seen && e.Version == latest && previousReport == nil {
				conflict := false
				for _, otherKey := range rec.evOrder {
					other := rec.evs[otherKey]
					if otherKey != key && other != nil && rec.reportLineage[otherKey] == group && other.Version == latest {
						conflict = true
						break
					}
				}
				if conflict {
					for _, otherKey := range append([]string(nil), rec.evOrder...) {
						other := rec.evs[otherKey]
						if otherKey != key && other != nil && rec.reportLineage[otherKey] == group && other.Version <= latest {
							retireReport(rec, otherKey, latest)
						}
					}
					retireReport(rec, key, latest)
					L.Stats["conflicting_report_version"]++
					affRoots[L.Find(i)] = true
					break
				}
			}
			rec.reportLineage[key] = group
			if !seen || e.Version > latest {
				rec.reportGroupLatest[group] = e.Version
			}
			for _, otherKey := range append([]string(nil), rec.evOrder...) {
				if otherKey == key || rec.reportLineage[otherKey] != group {
					continue
				}
				other := rec.evs[otherKey]
				if other == nil {
					continue
				}
				if e.Version > other.Version || previousReport != nil && e.Version >= other.Version {
					retireReport(rec, otherKey, e.Version)
				}
			}
		}
		if bodyKey != "" {
			latest, seen := rec.reportBodyLatest[bodyKey]
			if seen && e.Version < latest {
				retireReport(rec, key, latest)
				L.Stats["stale_report_version"]++
				affRoots[L.Find(i)] = true
				break
			}
			if seen && e.Version == latest && previousReport == nil {
				// Exact duplicates already took the idempotent branch above. Only an
				// active same-generation other ID makes this a semantic conflict.
				conflict := false
				for _, otherKey := range rec.evOrder {
					other := rec.evs[otherKey]
					if otherKey != key && other != nil && reportBodyKey(other) == bodyKey && other.Version == latest {
						conflict = true
						break
					}
				}
				if conflict {
					for _, otherKey := range append([]string(nil), rec.evOrder...) {
						other := rec.evs[otherKey]
						if otherKey != key && other != nil && reportBodyKey(other) == bodyKey && other.Version <= latest {
							retireReport(rec, otherKey, latest)
						}
					}
					retireReport(rec, key, latest)
					L.Stats["conflicting_report_version"]++
					affRoots[L.Find(i)] = true
					break
				}
			}
		}
		if previousReport != nil {
			if oldKey := reportContentKey(previousReport); oldKey != "" && oldKey != contentKey && rec.reportByBody[oldKey] == key {
				delete(rec.reportByBody, oldKey)
			}
		}
		delete(rec.reportRetired, key)
		if bodyKey != "" {
			if rec.reportBodyLatest == nil {
				rec.reportBodyLatest = map[string]int{}
			}
			if latest, seen := rec.reportBodyLatest[bodyKey]; !seen || e.Version > latest {
				rec.reportBodyLatest[bodyKey] = e.Version
			}
			for _, otherKey := range append([]string(nil), rec.evOrder...) {
				if otherKey == key {
					continue
				}
				other := rec.evs[otherKey]
				if other == nil || reportBodyKey(other) != bodyKey {
					continue
				}
				if e.Version > other.Version || previousReport != nil && e.Version >= other.Version {
					retireReport(rec, otherKey, e.Version)
				}
			}
		}
		if contentKey != "" {
			rec.reportByBody[contentKey] = key
		}
		if _, seen := rec.evs[key]; !seen {
			rec.evOrder = append(rec.evOrder, key)
		}
		rec.evs[key] = e
		if XRefLinkQ && L.noteXRef(e, i) { // a rejected intake citing a referral: that loop may need a link question
			affLoops[e.XRef] = true
		}
		affRoots[L.Find(i)] = true
		L.markNearDependents(i, affRoots)
		for z := range L.near[i] {
			affRoots[L.Find(z)] = true
		}
	}
	L.emitClusters(affRoots)
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
		var f *LoopFacts
		if _, ok := L.loops[lp]; ok {
			f = L.factsFor(lp)
		} else {
			f = L.retirementFacts(lp)
		}
		if f == nil {
			continue
		}
		h := sha(canon(f))
		if L.lastHash[lp] == h {
			continue
		}
		L.lastHash[lp] = h
		f.InSeq = p.Seq
		out = append(out, FactsMsg{ID: lp + "@" + utoa(p.Seq), InSeq: p.Seq, Facts: f, OriginTs: p.OriginTs, Part: partOf(lp, L.P)})
		if HistClock && L.streamSeen {
			t := L.streamT // a sign-off has no arrival of its own: the stream time
			if p.Kind == "event" {
				t = p.Tick
			}
			out[len(out)-1].Arr = &t
		}
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

// eidContradict reports whether two captured Emirates ID numbers cannot be one person's number. Capture noise is a
// one-digit slip per registration, so two copies of one number differ in at most two positions (a transposition is the
// largest realistic slip); three or more differences mean two numbers. This covers pairs the Luhn rule cannot judge
// because a typo broke the check digit on a side. Missing, truncated or default/dummy numbers say nothing.
func eidContradict(a, b string) bool {
	if len(a) != 15 || len(b) != 15 || dummyEID(a) || dummyEID(b) {
		return false
	}
	d := 0
	for i := 0; i < 15; i++ {
		if a[i] != b[i] {
			d++
		}
	}
	return d >= 3
}

func (L *LinkerCore) emitUnlinked(a, b int) {
	if L.trace == nil {
		return
	}
	L.emitLink(LinkTrace{A: L.recs[a].Pk, B: L.recs[b].Pk, P: L.pscore[pk2(a, b)], Stage: "unlinked"})
}
