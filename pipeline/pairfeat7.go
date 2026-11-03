package main

// Modelset pairfeat7 training-only hard negatives for featdump (with -nameidf: pairfeat7 training dumps; spec
// 2026-11-03-crc-pairfeat7-design.md). pairfeat7 reuses pairfeat6's features f30..f33 and leave-own-out rarity table
// (pairfeat6.go; its own file name_idf_pairfeat7.json) and adds two kinds of synthetic different persons. Each kind
// adds, for a share of the sampled persons, one namesake (no persons-file index: never in the rarity table, never left
// out of it) and two registrations without DOB or sex noise: the base person with their true birth date in a drawn
// script, and the namesake in the same script with probability 1/2, else the opposite (hnRegister).
//   -mdnamesakes  missing-date namesake: same sex as the base person, their given name and no other skeleton token, no
//                 family token or family-token skeleton a prefix of the other name's (givenOnly), and no birth date.
//   -rdnamesakes  same-real-date stranger: same sex, the base person's real non-placeholder birth date, no skeleton token
//                 in common (noSharedSkeleton); the donor is the eligible person whose name embedding is nearest.
import (
	"fmt"
	"math/rand"
	"strings"
)

var MDNamesakeRate float64 // flag -mdnamesakes; training-only (featdump with -nameidf: pairfeat7)

var RDNamesakeRate float64 // flag -rdnamesakes; training-only (featdump with -nameidf: pairfeat7)

func mdSeed(seed int64) int64 { return seed*2654435761 + 181081 }

func rdSeed(seed int64) int64 { return seed*2654435761 + 206233 }

// validateIDFNamesakes: a pairfeat7 hard-negative rate is in [0, 1), featdump only, and only with -nameidf.
func validateIDFNamesakes(flag, cmd string, rate float64, nameIDF string) error {
	if err := validateNamesakeRate(flag, cmd, rate); err != nil {
		return err
	}
	if rate > 0 && nameIDF == "" {
		return fmt.Errorf("%s needs -nameidf (missing-date and same-date hard negatives are pairfeat7 training data)", flag)
	}
	return nil
}

// hnName: the runtime tokens (latinForm, tokensOf) and skeleton of a persons-file English name.
type hnName struct{ tok, skel []string }

func newHNName(en string) hnName {
	t := tokensOf(latinForm(en))
	return hnName{t, skeleton(t)}
}

func strSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, x := range s {
		m[x] = true
	}
	return m
}

// prefixRelated: some x of a and y of b with x a prefix of y or y a prefix of x (equal strings count).
func prefixRelated(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.HasPrefix(x, y) || strings.HasPrefix(y, x) {
				return true
			}
		}
	}
	return false
}

// givenOnly: the missing-date namesake name rule. Both names have two or more tokens; the first tokens (the given
// names) are equal and their skeleton g is non-empty; the two skeleton-token sets share g and nothing else; no family
// token (a token after the first) of one name is a prefix of a family token of the other; and no family-token skeleton
// of one is a prefix of a family-token skeleton of the other. Initial and truncated family names ("S", "Abd",
// "Rahman" / "Rahmani", "Salman" / "Salem" by skeleton) are therefore never generated.
func givenOnly(a, b hnName) bool {
	if len(a.tok) < 2 || len(b.tok) < 2 || a.tok[0] != b.tok[0] {
		return false
	}
	g := skelTok(a.tok[0])
	if g == "" {
		return false
	}
	sb := strSet(b.skel)
	for _, x := range a.skel {
		if sb[x] && x != g {
			return false
		}
	}
	return !prefixRelated(a.tok[1:], b.tok[1:]) && !prefixRelated(skeleton(a.tok[1:]), skeleton(b.tok[1:]))
}

// noSharedSkeleton: the same-date stranger name rule: both skeletons non-empty and disjoint.
func noSharedSkeleton(a hnName, aset map[string]bool, b hnName) bool {
	if len(a.skel) == 0 || len(b.skel) == 0 {
		return false
	}
	for _, x := range b.skel {
		if aset[x] {
			return false
		}
	}
	return true
}

// buildMDNamesakes: md[i] is the missing-date namesake of ps[base[i]]: prefix "md-", DOB "", the base person's sex, the
// donor's names without aliases, a drawn Emirates ID with the donor's birth year. Donors: pool (the split, file order),
// same sex, givenOnly with the base name, a 10-character DOB; one is drawn uniformly. A base with no donor gets none.
func buildMDNamesakes(ps, pool []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	names := make([]hnName, len(pool))
	idx := map[string][]int{}
	for i := range pool {
		names[i] = newHNName(pool[i].En)
		if len(names[i].tok) > 1 {
			idx[names[i].tok[0]] = append(idx[names[i].tok[0]], i)
		}
	}
	var md []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		bn := newHNName(b.En)
		if len(bn.tok) < 2 {
			continue
		}
		var elig []int
		for _, j := range idx[bn.tok[0]] {
			d := &pool[j]
			if d.Qid != b.Qid && d.Sex == b.Sex && len(d.Dob) == 10 && givenOnly(bn, names[j]) {
				elig = append(elig, j)
			}
		}
		if len(elig) == 0 {
			continue
		}
		d := pool[elig[r.Intn(len(elig))]]
		md = append(md, Person{Qid: "md-" + b.Qid, Ar: d.Ar, En: d.En, Dob: "", Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(d.Dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return md, base
}

// buildRDNamesakes: rd[i] is the same-real-date stranger of ps[base[i]]: prefix "rd-", the base person's DOB and sex,
// the donor's names without aliases, a drawn Emirates ID. Bases need a real, non-placeholder DOB (realDob and not
// dobPlaceholder6). Donors: pool, same sex, noSharedSkeleton with the base name; the donor is the eligible person whose
// English-name embedding emb(En) has the largest dot product with the base person's (the first in pool order on a tie),
// so that the pair can reach the linker's candidate list. A base with no donor gets none.
func buildRDNamesakes(ps, pool []Person, rate float64, r *rand.Rand, emb func(string) []float32) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	names := make([]hnName, len(pool))
	for i := range pool {
		names[i] = newHNName(pool[i].En)
	}
	vecs := make([][]float32, len(pool)) // embedded on first use
	var rd []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate {
			continue
		}
		if !realDob(b.Dob) || dobPlaceholder6(b.Dob) {
			continue
		}
		bn := newHNName(b.En)
		bset, bv := strSet(bn.skel), emb(b.En)
		best, bestS := -1, 0.0
		for j := range pool {
			d := &pool[j]
			if d.Qid == b.Qid || d.Sex != b.Sex || !noSharedSkeleton(bn, bset, names[j]) {
				continue
			}
			if vecs[j] == nil {
				vecs[j] = emb(d.En)
			}
			if s := dot(bv, vecs[j]); best < 0 || s > bestS {
				best, bestS = j, s
			}
		}
		if best < 0 {
			continue
		}
		d := pool[best]
		rd = append(rd, Person{Qid: "rd-" + b.Qid, Ar: d.Ar, En: d.En, Dob: b.Dob, Sex: b.Sex, Cc: d.Cc,
			Eid: newEID(b.Dob, r), Spl: b.Spl, Origin: d.Origin, ArSrc: d.ArSrc})
		base = append(base, bi)
	}
	return rd, base
}

// hnRegister: the two registrations of a pairfeat7 hard negative: base person b with their true DOB in a drawn script,
// namesake x with its own DOB in the same script with probability 1/2, else the opposite; true sex, no noise.
func hnRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg {
	ar := r.Float64() < .5
	xar := ar
	if r.Float64() >= .5 {
		xar = !ar
	}
	regs = append(regs, fdReg{bpid, fdName(b, ar, r), b.Dob, b.Sex, drawEID(b, r), b.Idx})
	return append(regs, fdReg{xpid, fdName(x, xar, r), x.Dob, x.Sex, drawEID(x, r), x.Idx})
}

// hnKind: one pairfeat7 hard-negative kind in a featdump: sidecar key prefix, rate, namesake pid -> base pid, the
// number added, and the rows pairing a namesake with a registration of its base person.
type hnKind struct {
	key   string
	rate  float64
	base  map[int]int
	added int
	rows  []int
}

// pairfeat7Negatives: the md then rd hard negatives, each only when its rate is > 0 and with its own generator
// (mdSeed, rdSeed); their registrations are appended to regs with person indices from first on. With both rates 0 it
// returns regs unchanged and no kind (no draw, no encoder load).
func pairfeat7Negatives(regs []fdReg, ps, pool []Person, seed int64, first int, encPath string) ([]fdReg, []*hnKind) {
	var kinds []*hnKind
	for _, k := range []struct {
		key  string
		rate float64
		seed int64
	}{{"md", MDNamesakeRate, mdSeed(seed)}, {"rd", RDNamesakeRate, rdSeed(seed)}} {
		if k.rate <= 0 {
			continue
		}
		r := rand.New(rand.NewSource(k.seed))
		var xs []Person
		var base []int
		if k.key == "md" {
			xs, base = buildMDNamesakes(ps, pool, k.rate, r)
		} else {
			xs, base = buildRDNamesakes(ps, pool, k.rate, r, LoadEncoder(encPath).Embed)
		}
		hk := &hnKind{key: k.key, rate: k.rate, base: map[int]int{}, added: len(xs), rows: []int{}}
		for i := range xs {
			regs = hnRegister(regs, base[i], &ps[base[i]], first+i, &xs[i], r)
			hk.base[first+i] = base[i]
		}
		first += len(xs)
		kinds = append(kinds, hk)
	}
	return regs, kinds
}
