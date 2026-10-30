package main

// Stage 7: independent noise model ("v2"), written separately from the training-time noise (gen.go noisyLatin/noisyArabic,
// a port of the encoder's training corruptions). Used ONLY for held-out evaluation (-noise v2), never for training,
// calibration or rule design. Different error families and rates, modelled on registration-desk failure modes:
//   Latin : phonetic respellings (ou/u, ee/i, kh/k, q/k/g, j/g, sh/ch, dh/z/d, th/t), doubled/undoubled consonants,
//           name-order swap (family name first), dropped "bin/bint/ibn" particles, initials, OCR confusions (rn/m, l/1, O/0),
//           extra whitespace / hyphenation.
//   Arabic: hamza seat changes (ئ/ؤ/ء), alif maqsura vs ya (ى/ي), ta marbuta, dropped "ال" on the family name,
//           "بن/بنت" insertion or removal, given-name-only truncation.
//   DOB   : different mix than training: 15% missing, month/day transposition, year off by one or ten, 1st-of-month default.
import (
	"fmt"
	"math/rand"
	"strings"
	"unicode/utf8"
)

var NoiseModel = "v1" // v1 = training-time noise (gen.go); v2 = independent held-out noise (this file)

var phon2 = [][2]string{{"ou", "u"}, {"u", "ou"}, {"ee", "i"}, {"i", "ee"}, {"kh", "k"}, {"q", "k"}, {"q", "g"}, {"j", "g"},
	{"sh", "ch"}, {"dh", "z"}, {"dh", "d"}, {"th", "t"}, {"ai", "ay"}, {"ei", "ay"}, {"oo", "u"}, {"ph", "f"}, {"y", "i"}}
var ocr2 = [][2]string{{"rn", "m"}, {"m", "rn"}, {"l", "1"}, {"o", "0"}, {"I", "l"}, {"cl", "d"}}

func noisyLatin2(name string, r *rand.Rand) string {
	toks := strings.Fields(name)
	keep := toks[:0:0]
	for _, t := range toks { // drop genealogical particles
		lt := strings.ToLower(t)
		if (lt == "bin" || lt == "bint" || lt == "ibn" || lt == "ben") && r.Float64() < .6 {
			continue
		}
		keep = append(keep, t)
	}
	toks = keep
	if len(toks) >= 2 && r.Float64() < .15 { // family name first
		toks = append([]string{toks[len(toks)-1]}, toks[:len(toks)-1]...)
	}
	if len(toks) >= 3 && r.Float64() < .12 { // middle name as initial
		k := 1 + r.Intn(len(toks)-2)
		_, n := utf8.DecodeRuneInString(toks[k]) // first letter, not first byte: a non-ASCII initial stays valid text
		toks[k] = toks[k][:n] + "."
	}
	s := strings.Join(toks, " ")
	for n := 0; n < 2; n++ {
		if r.Float64() < .35 {
			p := phon2[r.Intn(len(phon2))]
			if i := foldIndex(s, p[0]); i >= 0 {
				s = s[:i] + p[1] + s[i+len(p[0]):]
			}
		}
	}
	if r.Float64() < .15 { // (un)double a consonant
		rs := []rune(s)
		for k := 0; k < 6 && len(rs) > 3; k++ {
			i := 1 + r.Intn(len(rs)-2)
			if strings.ContainsRune("bcdfghjklmnprstvwz", rs[i]) {
				if rs[i] == rs[i+1] {
					rs = append(rs[:i:i], rs[i+1:]...)
				} else {
					rs = append(rs[:i+1:i+1], append([]rune{rs[i]}, rs[i+1:]...)...)
				}
				break
			}
		}
		s = string(rs)
	}
	if r.Float64() < .06 {
		p := ocr2[r.Intn(len(ocr2))]
		s = strings.Replace(s, p[0], p[1], 1)
	}
	if r.Float64() < .08 {
		s = strings.Replace(s, " ", "-", 1)
	}
	if r.Float64() < .05 {
		s = strings.ReplaceAll(s, " ", "  ")
	}
	return s
}

// foldIndex: byte offset in s of the first case-insensitive match of the lower-case ASCII pattern p, or -1. The offset
// is taken in s itself: an index into strings.ToLower(s) is wrong when a rune's lower-case form has a different UTF-8
// length (Go lower-cases "İ" to "i̇", one byte longer), which sliced s out of range.
func foldIndex(s, p string) int {
	for i := range s {
		if i+len(p) <= len(s) && strings.EqualFold(s[i:i+len(p)], p) {
			return i
		}
	}
	return -1
}

func noisyArabic2(name string, r *rand.Rand) string {
	toks := strings.Fields(name)
	if r.Float64() < .25 {
		for i, t := range toks {
			toks[i] = strings.NewReplacer("ئ", "ء", "ؤ", "ء").Replace(t)
		}
	}
	if r.Float64() < .3 {
		for i, t := range toks {
			toks[i] = strings.ReplaceAll(t, "ى", "ي")
		}
	}
	if r.Float64() < .15 {
		for i, t := range toks {
			toks[i] = strings.ReplaceAll(t, "ة", "ت")
		}
	}
	if len(toks) >= 2 && strings.HasPrefix(toks[len(toks)-1], "ال") && r.Float64() < .3 {
		toks[len(toks)-1] = strings.TrimPrefix(toks[len(toks)-1], "ال")
	}
	if len(toks) >= 2 && r.Float64() < .1 {
		toks = append(toks[:1:1], append([]string{"بن"}, toks[1:]...)...)
	} else {
		keep := toks[:0:0]
		for _, t := range toks {
			if (t == "بن" || t == "بنت") && r.Float64() < .5 {
				continue
			}
			keep = append(keep, t)
		}
		toks = keep
	}
	if len(toks) >= 3 && r.Float64() < .08 {
		toks = toks[:2]
	}
	return strings.Join(toks, " ")
}

func noisyDob2(dob string, r *rand.Rand, p float64) string {
	if r.Float64() > p {
		return dob
	}
	y, m, d := dob[:4], dob[5:7], dob[8:10]
	x := r.Float64()
	switch {
	case x < .15:
		return ""
	case x < .35 && atoi(d) <= 12 && d != m:
		return y + "-" + d + "-" + m
	case x < .55:
		return y + "-" + m + "-01"
	case x < .75:
		return fmt.Sprintf("%04d-%s-%s", atoi(y)+10*(1-2*r.Intn(2)), m, d)
	}
	return fmt.Sprintf("%04d-%s-%s", atoi(y)+1-2*r.Intn(2), m, d)
}
