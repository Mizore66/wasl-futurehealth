package main

import (
	"math/rand"
	"strings"
)

var MINamesakeRate float64 // featdump only, with -nameidf: middle-initial hard negatives (pairfeat9)

func miSeed(seed int64) int64 { return seed*2654435761 + 307093 }

func miName(en string, bi int) (string, bool) {
	// Latin names keep their case and punctuation; only Arabic script is transliterated.
	if isAr(en) {
		en = latinForm(en)
	}
	t := strings.Fields(en)
	if len(t) < 2 {
		return "", false
	}
	for _, x := range t[1 : len(t)-1] {
		if len(strings.TrimSuffix(x, ".")) == 1 { // an existing middle initial, with or without a dot
			return "", false
		}
	}
	initial := string(rune('A' + bi%26))
	out := []string{t[0], initial}
	out = append(out, t[1:]...)
	return strings.Join(out, " "), true
}

func buildMINamesakes(ps []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	var xs []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate || (b.Sex != "M" && b.Sex != "F") {
			continue
		}
		name, ok := miName(b.En, bi)
		if !ok {
			continue
		}
		xs = append(xs, Person{Qid: "mi-" + b.Qid, En: name, Dob: "", Sex: b.Sex, Cc: b.Cc,
			Spl: b.Spl, Origin: b.Origin, ArSrc: b.ArSrc})
		base = append(base, bi)
	}
	return xs, base
}

func miRegister(regs []fdReg, xpid int, x *Person) []fdReg {
	return append(regs, fdReg{xpid, x.En, "", x.Sex, "", 0})
}
