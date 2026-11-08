package main

// Adversarial generator cases (both off by default; the cohort is byte-identical when off).
//
// B8, -wrongeid p: a share p of new registrations carries a family member's Emirates ID instead of the patient's own
// (a relative's card typed at registration, or a relative registered under the patient's card: either way one
// registration of person X carries the real, Luhn-valid number of person Y). No public rate exists for this
// (docs/research/eid-second-identifier.md); p is an assumption. The relative is drawn from persons already registered in
// the cohort, so every wrong number can meet a registration that rightly carries it (worst case for the linker):
//   - 60% blood relative (parent, child, sibling): same name origin and same last Latin name token when such a person
//     is registered, else same origin;
//   - 40% spouse: opposite recorded sex, same origin; outside Arab names (where a wife usually keeps her own family
//     name) the same last name token when such a person is registered.
// The split and the "already registered" draw are assumptions, not measured shares.
//
// B9, -wrongref p: a share p of referral intakes cite the wrong referral in basedOn ("external referral <loop>"):
//   - 40% typo: one digit of the referral number replaced, or two adjacent digits swapped (may hit another real loop);
//   - 40% another patient's referral from the same referring provider (a wrong row picked from a list);
//   - 20% a number that does not exist (an extra digit typed).
// The rate and split are assumptions. Only the cited text changes; the intake still goes to the right patient's
// registration and the loop truth is unchanged.
//
// Each case draws from its own hashed stream per registration / per intake, so nothing else in the cohort moves.

import (
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

var WrongEIDRate float64 // flag -wrongeid
var WrongRefRate float64 // flag -wrongref

// WrongEIDTruth: one registration that carries a relative's Emirates ID.
type WrongEIDTruth struct {
	Qid           string `json:"qid"`
	RelQid        string `json:"rel_qid"`
	Relation      string `json:"relation"` // blood | spouse
	SharedSurname bool   `json:"shared_surname"`
	Own           string `json:"own"` // what the registration would have carried: valid | missing | mistyped
}

// WrongRefTruth: one intake that cites the wrong referral.
type WrongRefTruth struct {
	Loop      string `json:"loop"`  // the referral the intake really answers
	Cited     string `json:"cited"` // what basedOn says
	Kind      string `json:"kind"`  // typo | other_patient | nonexistent
	Status    string `json:"status"`
	CitedLoop bool   `json:"cited_loop"`          // the cited text is an existing loop (another patient's)
	CitedQid  string `json:"cited_qid,omitempty"` // that loop's patient
}

type advGen struct {
	seed     int64
	tr       *Truth
	seen     map[string]bool
	bySur    map[string][]*Person // origin|last token -> registered persons, in registration order
	byOrigin map[string][]*Person
}

var adv *advGen

func newAdvGen(seed int64, tr *Truth) *advGen {
	if WrongEIDRate <= 0 && WrongRefRate <= 0 {
		return nil
	}
	if WrongEIDRate > 0 {
		tr.WrongEID = map[string]*WrongEIDTruth{}
	}
	if WrongRefRate > 0 {
		tr.WrongRef = map[string]*WrongRefTruth{}
	}
	return &advGen{seed: seed, tr: tr, seen: map[string]bool{}, bySur: map[string][]*Person{}, byOrigin: map[string][]*Person{}}
}

func (a *advGen) stream(key string) *rand.Rand {
	return rand.New(rand.NewSource(a.seed ^ int64(fnv32(key))))
}

// surKey: origin and the last Latin name token, lowercased, without a leading "al-".
func surKey(p *Person) string {
	t := strings.Fields(strings.ReplaceAll(strings.ToLower(p.En), "-", " "))
	if len(t) == 0 {
		return p.Origin + "|"
	}
	return p.Origin + "|" + t[len(t)-1]
}

// register: called for every new registration, after its own identity draw (x.eid already set).
func (a *advGen) register(x *reg, p *Person) {
	if a == nil {
		return
	}
	if WrongEIDRate > 0 {
		a.wrongEID(x, p)
	}
	if !a.seen[p.Qid] {
		a.seen[p.Qid] = true
		q := *p // a copy: callers may pass the address of a reused loop variable
		a.bySur[surKey(&q)] = append(a.bySur[surKey(&q)], &q)
		a.byOrigin[q.Origin] = append(a.byOrigin[q.Origin], &q)
	}
}

func pickOther(r *rand.Rand, ps []*Person, self string, keep func(*Person) bool) *Person {
	var c []*Person
	for _, q := range ps {
		if q.Qid != self && q.Eid != "" && keep(q) {
			c = append(c, q)
		}
	}
	if len(c) == 0 {
		return nil
	}
	return c[r.Intn(len(c))]
}

func (a *advGen) wrongEID(x *reg, p *Person) {
	r := a.stream(x.pk + "|wrongeid")
	if r.Float64() >= WrongEIDRate {
		return
	}
	any := func(*Person) bool { return true }
	opp := map[string]string{"M": "F", "F": "M"}[p.Sex]
	sk := surKey(p)
	var rel *Person
	relation := "blood"
	if r.Float64() < .4 && opp != "" {
		relation = "spouse"
		isOpp := func(q *Person) bool { return q.Sex == opp }
		if p.Origin != "arab" {
			rel = pickOther(r, a.bySur[sk], p.Qid, isOpp)
		}
		if rel == nil {
			rel = pickOther(r, a.byOrigin[p.Origin], p.Qid, isOpp)
		}
	} else {
		rel = pickOther(r, a.bySur[sk], p.Qid, any)
		if rel == nil {
			rel = pickOther(r, a.byOrigin[p.Origin], p.Qid, any)
		}
	}
	if rel == nil || rel.Eid == p.Eid {
		return
	}
	own := "valid"
	switch {
	case x.eid == "":
		own = "missing"
	case x.eid != p.Eid:
		own = "mistyped"
	}
	x.eid = rel.Eid
	a.tr.WrongEID[x.pk] = &WrongEIDTruth{Qid: p.Qid, RelQid: rel.Qid, Relation: relation, SharedSurname: surKey(rel) == sk, Own: own}
}

// citeRef: the referral an intake cites (key = provider/ServiceRequest/<intake id>, loop = the referral it answers).
func (a *advGen) citeRef(key, loop, status string, p *Person) string {
	if a == nil || WrongRefRate <= 0 || loop == "" {
		return loop
	}
	r := a.stream(key + "|wrongref")
	if r.Float64() >= WrongRefRate {
		return loop
	}
	i := strings.LastIndexByte(loop, '-')
	if i < 0 {
		return loop
	}
	head, num := loop[:i+1], loop[i+1:]
	t := &WrongRefTruth{Loop: loop, Status: status}
	switch x := r.Float64(); {
	case x < .4:
		t.Kind = "typo"
		bs := []byte(num)
		for k := 0; k < 20 && string(bs) == num; k++ {
			bs = []byte(num)
			j := r.Intn(len(bs))
			if len(bs) > 1 && r.Float64() < .3 {
				j = r.Intn(len(bs) - 1)
				bs[j], bs[j+1] = bs[j+1], bs[j]
			} else {
				bs[j] = byte('0' + (int(bs[j]-'0')+1+r.Intn(9))%10)
			}
			if bs[0] == '0' && len(bs) > 1 {
				bs = []byte(num) // no leading zero in a referral number
			}
		}
		t.Cited = head + string(bs)
	case x < .8:
		t.Kind = "other_patient"
		prov := loop[:strings.IndexByte(loop, '/')+1]
		var c []string
		for id, lt := range a.tr.Loops {
			if lt.Qid != p.Qid && strings.HasPrefix(id, prov) && !strings.Contains(id, "#") {
				c = append(c, id)
			}
		}
		if len(c) == 0 {
			t.Kind, t.Cited = "nonexistent", head+extraDigits(r, num)
			break
		}
		sort.Strings(c)
		t.Cited = c[r.Intn(len(c))]
	default:
		t.Kind, t.Cited = "nonexistent", head+extraDigits(r, num)
	}
	if t.Cited == loop {
		return loop
	}
	a.tr.WrongRef[key] = t
	return t.Cited
}

// extraDigits: the number with digits typed after it until it is beyond any issued referral number (>= 10000; the
// largest per-provider counter in a frozen-eval cohort is in the low thousands).
func extraDigits(r *rand.Rand, num string) string {
	s := num + strconv.Itoa(r.Intn(10))
	for len(s) < 5 {
		s += strconv.Itoa(r.Intn(10))
	}
	return s
}

// finish: resolve what each cited text points at once every loop exists (a typo can hit a loop generated later).
func (a *advGen) finish() {
	if a == nil {
		return
	}
	for _, t := range a.tr.WrongRef {
		if lt, ok := a.tr.Loops[t.Cited]; ok {
			t.CitedLoop, t.CitedQid = true, lt.Qid
		}
	}
}

// summary: counts for cohort_summary.json (only when a case is on).
func (a *advGen) summary() map[string]interface{} {
	out := map[string]interface{}{}
	if a.tr.WrongEID != nil {
		m := map[string]int{"registrations": len(a.tr.WrongEID)}
		for _, t := range a.tr.WrongEID {
			m["relation_"+t.Relation]++
			m["own_"+t.Own]++
			if t.SharedSurname {
				m["shared_surname"]++
			}
		}
		out["wrong_eid"] = m
	}
	if a.tr.WrongRef != nil {
		m := map[string]int{"intakes": len(a.tr.WrongRef)}
		for _, t := range a.tr.WrongRef {
			m["kind_"+t.Kind]++
			m["status_"+t.Status]++
			if t.CitedLoop {
				m["cites_existing_loop"]++
			}
		}
		out["wrong_ref"] = m
	}
	return out
}
