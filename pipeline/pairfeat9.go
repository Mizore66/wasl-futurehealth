package main

import (
	"math/rand"
	"strings"
	"unicode/utf8"
)

var MINamesakeRate float64 // featdump only, with -nameidf: middle-initial hard negatives (pairfeat9)

func miSeed(seed int64) int64 { return seed*2654435761 + 307093 }

// miName: en with one invented middle initial after the first token, case and punctuation kept (Person.En is Latin).
// A name with fewer than two tokens, or one that already has a middle initial (one rune, with or without a dot), is
// ineligible.
func miName(en string, bi int) (string, bool) {
	t := strings.Fields(en)
	if len(t) < 2 {
		return "", false
	}
	for _, x := range t[1 : len(t)-1] {
		if utf8.RuneCountInString(strings.TrimSuffix(x, ".")) == 1 {
			return "", false
		}
	}
	initial := string(rune('A' + bi%26))
	out := []string{t[0], initial}
	out = append(out, t[1:]...)
	return strings.Join(out, " "), true
}

// buildMINamesakes: one namesake per sampled base with a known sex, a non-empty DOB (placeholder dates such as Jan 01
// are allowed) and an eligible name.
func buildMINamesakes(ps []Person, rate float64, r *rand.Rand) ([]Person, []int) {
	if rate <= 0 {
		return nil, nil
	}
	var xs []Person
	var base []int
	for bi := range ps {
		b := &ps[bi]
		if r.Float64() >= rate || (b.Sex != "M" && b.Sex != "F") || b.Dob == "" {
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

// miRegister: the two registrations of a middle-initial hard negative. Every existing registration of the base
// person bpid is dropped and replaced by one exact registration (same En, unchanged DOB, known sex, drawEID), so each
// pair of the namesake with the base has the intended shape; the namesake x registers once, exactly (no DOB, no EID).
// Other persons' registrations are unchanged and keep their order.
func miRegister(regs []fdReg, bpid int, b *Person, xpid int, x *Person, r *rand.Rand) []fdReg {
	kept := regs[:0:0]
	for _, g := range regs {
		if g.pid != bpid {
			kept = append(kept, g)
		}
	}
	kept = append(kept, fdReg{bpid, b.En, b.Dob, b.Sex, drawEID(b, r), b.Idx})
	return append(kept, fdReg{xpid, x.En, "", x.Sex, "", 0})
}
