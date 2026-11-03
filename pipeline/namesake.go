package main

// Training-only synthetic "namesake" hard negatives for featdump (-namesakes). A namesake is a different, synthetic
// person who shares a real person's first name, origin and sex, with a DOB one or two digits away and a new Emirates ID.
import (
	"fmt"
	"math/rand"
	"strings"
)

var NamesakeRate float64 // flag -namesakes; training-only (featdump)

var FamilyNamesakeRate float64 // flag -familynamesakes; training-only (featdump)

func validateNamesakeRate(flag, cmd string, rate float64) error {
	if !(rate >= 0 && rate < 1) {
		return fmt.Errorf("%s must be in [0, 1) (got %v)", flag, rate)
	}
	if rate > 0 && cmd != "featdump" {
		return fmt.Errorf("%s is training-only and valid only for featdump (got command %q)", flag, cmd)
	}
	return nil
}

func validateNamesakes(cmd string, rate float64) error {
	return validateNamesakeRate("-namesakes", cmd, rate)
}

func familySeed(seed int64) int64 { return seed*2654435761 + 130363 }

func namesakeSeed(seed int64) int64 { return seed*2654435761 + 104729 }

var XSNamesakeRate float64 // flag -xsnamesakes; training-only (featdump with -nameidf: pairfeat6)

func xsSeed(seed int64) int64 { return seed*2654435761 + 155921 }

// validateXSNamesakes: -xsnamesakes in [0, 1), featdump only, and only with -nameidf (pairfeat6 training dumps).
func validateXSNamesakes(cmd string, rate float64, nameIDF string) error {
	if err := validateNamesakeRate("-xsnamesakes", cmd, rate); err != nil {
		return err
	}
	if rate > 0 && nameIDF == "" {
		return fmt.Errorf("-xsnamesakes needs -nameidf (cross-script hard negatives are pairfeat6 training data)")
	}
	return nil
}

// otherTokensDisjoint: no token after the first of either name occurs anywhere in the other name.
func otherTokensDisjoint(a, b []string) bool {
	set := func(s []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range s {
			m[x] = true
		}
		return m
	}
	sa, sb := set(a), set(b)
	for _, x := range a[1:] {
		if sb[x] {
			return false
		}
	}
	for _, x := range b[1:] {
		if sa[x] {
			return false
		}
	}
	return true
}

// buildXSNamesakes: the pairfeat6 cross-script hard negatives. For a share rate of ps, xs[i] is a different person
// whose first English token equals ps[base[i]]'s, whose other tokens share nothing with the base name (and the reverse),
// with the base person's sex and the 1 January placeholder of the base person's birth year. Donors come from pool (the
// split, file order). A base with a single-token name, no usable year or no donor gets none.
func buildXSNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	idx := map[string][]int{}
	for i := range pool {
		if t := strings.Fields(strings.ToLower(pool[i].En)); len(t) > 1 {
			idx[t[0]] = append(idx[t[0]], i)
		}
	}
	var xs []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		bt := strings.Fields(strings.ToLower(b.En))
		if len(bt) < 2 || len(b.Dob) != 10 {
			continue
		}
		dob := b.Dob[:4] + "-01-01"
		if !realDob(dob) {
			continue
		}
		var elig []int
		for _, j := range idx[bt[0]] {
			d := &pool[j]
			if d.Qid != b.Qid && d.Sex == b.Sex && otherTokensDisjoint(bt, strings.Fields(strings.ToLower(d.En))) {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		xs = append(xs, Person{Qid: "xs-" + b.Qid, Ar: d.Ar, En: d.En, Als: d.Als, Dob: dob, Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return xs, base
}

// xsRegister: the two registrations of a cross-script namesake: base person b in a drawn script and the namesake x in
// the other, both with x's placeholder DOB and their true sex (no DOB or sex noise) and a drawn Emirates ID.
func xsRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg {
	ar := r.Float64() < .5
	regs = append(regs, fdReg{bpid, fdName(b, ar, r), x.Dob, b.Sex, drawEID(b, r), b.Idx})
	return append(regs, fdReg{xpid, fdName(x, !ar, r), x.Dob, x.Sex, drawEID(x, r), x.Idx})
}

var dobPos = []int{0, 1, 2, 3, 5, 6, 8, 9}

func namesakeDob(dob string, r *rand.Rand) (string, bool) {
	if len(dob) != 10 {
		return "", false
	}
	// The digit count is drawn once, so retries do not skew the share of one-digit namesakes (two-digit changes are
	// invalid more often and would otherwise be filtered out); only the changed positions and digits are redrawn.
	n := 1
	if r.Float64() >= 0.60 {
		n = 2
	}
	for try := 0; try < 20; try++ {
		b := []byte(dob)
		for _, k := range r.Perm(8)[:n] {
			i := dobPos[k]
			b[i] = byte('0' + (int(b[i]-'0')+1+r.Intn(9))%10)
		}
		s := string(b)
		if s != dob && realDob(s) && s[:4] >= "1900" && s[:4] <= "2025" {
			return s, true
		}
	}
	return "", false
}

func newEID(dob string, r *rand.Rand) string {
	p := "784" + dob[:4]
	for len(p) < 14 {
		p += string(byte('0' + r.Intn(10)))
	}
	return p + string(luhnDigit(p))
}

func firstLastEn(en string) (string, string) {
	t := strings.Fields(strings.ToLower(en))
	if len(t) == 0 {
		return "", ""
	}
	return t[0], t[len(t)-1]
}

// buildNamesakes: ns[i] is the namesake of ps[base[i]]; pool is every person of the same split with non-empty Ar and En.
// Given-name kind: donor shares the first English token, differs in the last.
func buildNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	return buildNamesakesBy(ps, pool, rate, r, false, "ns-")
}

// buildFamilyNamesakes: the family-name kind; donor shares the last English token, differs in the first.
func buildFamilyNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	return buildNamesakesBy(ps, pool, rate, r, true, "nf-")
}

func buildNamesakesBy(ps, pool []Person, rate float64, r *rand.Rand, byLast bool, prefix string) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	key := func(en string) (shared, other string) {
		f, l := firstLastEn(en)
		if byLast {
			return l, f
		}
		return f, l
	}
	idx := map[string][]int{}
	for i := range pool {
		k, _ := key(pool[i].En)
		idx[k] = append(idx[k], i)
	}
	var ns []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		k, o := key(b.En)
		var elig []int
		for _, j := range idx[k] {
			d := &pool[j]
			_, do := key(d.En)
			if d.Qid != b.Qid && d.Origin == b.Origin && d.Sex == b.Sex && do != o {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		dob, ok := namesakeDob(b.Dob, r)
		if !ok {
			continue
		}
		ns = append(ns, Person{Qid: prefix + b.Qid, Ar: d.Ar, En: d.En, Als: d.Als, Dob: dob, Sex: d.Sex,
			Cc: d.Cc, Eid: newEID(dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return ns, base
}

// namesakePairRow: pair (a, b) is a namesake and its own base person, in either orientation.
func namesakePairRow(nsBase map[int]int, a, b int) bool {
	if x, ok := nsBase[a]; ok && x == b {
		return true
	}
	if x, ok := nsBase[b]; ok && x == a {
		return true
	}
	return false
}
