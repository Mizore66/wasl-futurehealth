package main

// Name normalisation, rule transliteration, skeletons, Jaro-Winkler and pair features (Go port of src/common.py).
import (
	"math"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func isAr(s string) bool {
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

func joinPrefixTokens(toks []string, pref map[string]string) []string {
	out := []string{}
	carry := ""
	for _, t := range toks {
		if rep, ok := pref[t]; ok {
			carry += rep
			continue
		}
		out = append(out, carry+t)
		carry = ""
	}
	if carry != "" {
		out = append(out, carry)
	}
	return out
}

var arPref = map[string]string{"عبد": "عبد", "ابو": "ابو"}
var latPref = map[string]string{"abd": "abdal", "abdel": "abdal", "abdul": "abdal", "abdal": "abdal", "abdol": "abdal", "abu": "abu", "abou": "abu"}

// UrduFix: v3 fix. v2 mapped Urdu/Persian letters (پ چ گ ڤ ژ ٹ ڈ ڑ ں ہ ے) to spaces, deleting letters from
// South Asian Arabic-script names. With the fix they fold to their nearest Arabic letter (same table as common3.py).
var UrduFix = false
var urduMap = map[rune]rune{'پ': 'ب', 'چ': 'ج', 'ژ': 'ز', 'ڤ': 'ف', 'گ': 'غ', 'ٹ': 'ت', 'ڈ': 'د', 'ڑ': 'ر', 'ں': 'ن', 'ہ': 'ه',
	'ۀ': 'ه', 'ۃ': 'ه', 'ے': 'ي', 'ۓ': 'ي', 'ڭ': 'غ', 'ڨ': 'ق', 'ݣ': 'غ'}

func normAr(s string) string {
	var b strings.Builder
	for _, r := range s {
		if UrduFix {
			if m, ok := urduMap[r]; ok {
				r = m
			}
		}
		if (r >= 0x064B && r <= 0x0652) || r == 0x0670 || r == 0x0640 {
			continue
		}
		switch r {
		case 'أ', 'إ', 'آ', 'ٱ':
			r = 'ا'
		case 'ى':
			r = 'ي'
		case 'ة':
			r = 'ه'
		case 'ؤ':
			r = 'و'
		case 'ئ', 'ی':
			r = 'ي'
		case 'ک':
			r = 'ك'
		case 'ھ':
			r = 'ه'
		}
		if !((r >= 0x0621 && r <= 0x064A) || r == ' ') {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.Join(joinPrefixTokens(strings.Fields(b.String()), arPref), " ")
}

func normLat(s string) string {
	s = norm.NFKD.String(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		r = unicode.ToLower(r)
		if r == '\'' || r == '`' || r == 'ʿ' || r == 'ʾ' {
			continue
		}
		if !((r >= 'a' && r <= 'z') || r == ' ') {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.Join(joinPrefixTokens(strings.Fields(b.String()), latPref), " ")
}

var ar2lat = map[rune]string{'ا': "a", 'ب': "b", 'ت': "t", 'ث': "th", 'ج': "j", 'ح': "h", 'خ': "kh", 'د': "d", 'ذ': "dh", 'ر': "r", 'ز': "z",
	'س': "s", 'ش': "sh", 'ص': "s", 'ض': "d", 'ط': "t", 'ظ': "z", 'ع': "a", 'غ': "gh", 'ف': "f", 'ق': "q", 'ك': "k",
	'ل': "l", 'م': "m", 'ن': "n", 'ه': "h", 'و': "u", 'ي': "i"}

func translit(sAr string) string {
	out := []string{}
	for _, tok := range strings.Fields(sAr) {
		var b strings.Builder
		for _, r := range tok {
			b.WriteString(ar2lat[r])
		}
		t := b.String()
		if rt := []rune(tok); len(rt) > 2 && rt[len(rt)-1] == 'ه' && len(t) > 0 {
			t = t[:len(t)-1] + "a"
		}
		if strings.HasPrefix(tok, "ال") && len([]rune(tok)) > 3 && len(t) >= 2 {
			t = "al " + t[2:]
		}
		out = append(out, t)
	}
	return strings.Join(strings.Fields(strings.Join(out, " ")), " ")
}

func latinForm(name string) string {
	if isAr(name) {
		return translit(normAr(name))
	}
	return normLat(name)
}

var articles = map[string]bool{"al": true, "el": true, "bin": true, "ben": true, "ibn": true, "bint": true, "ould": true}
var alKeep = map[string]bool{"ali": true, "alia": true, "alaa": true, "alya": true}

func tokensOf(lat string) []string {
	out := []string{}
	for _, t := range strings.Fields(lat) {
		if strings.HasPrefix(t, "al") && len(t) > 4 && !alKeep[t] {
			t = t[2:]
		}
		if articles[t] {
			continue
		}
		out = append(out, t)
	}
	return out
}

var vow = regexp.MustCompile(`[aeiouyw]`)

func skelTok(t string) string {
	for _, p := range [][2]string{{"kh", "K"}, {"sh", "S"}, {"th", "T"}, {"dh", "D"}, {"gh", "G"}, {"ph", "f"},
		{"q", "k"}, {"c", "k"}, {"x", "ks"}, {"v", "f"}, {"p", "b"}, {"j", "g"},
		{"T", "t"}, {"D", "d"}, {"z", "d"}, {"G", "g"}, {"K", "k"}, {"S", "s"}, {"e", "a"}, {"o", "u"}, {"h", ""}} {
		t = strings.ReplaceAll(t, p[0], p[1])
	}
	t = vow.ReplaceAllString(t, "")
	var b strings.Builder
	var prev rune = -1
	for _, r := range t {
		if r != prev {
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

func skeleton(toks []string) []string {
	out := []string{}
	for _, t := range toks {
		if s := skelTok(t); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func jaroWinkler(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	md := la
	if lb > md {
		md = lb
	}
	md = md/2 - 1
	if md < 0 {
		md = 0
	}
	ma := make([]bool, la)
	mb := make([]bool, lb)
	m := 0
	for i := 0; i < la; i++ {
		lo, hi := i-md, i+md+1
		if lo < 0 {
			lo = 0
		}
		if hi > lb {
			hi = lb
		}
		for j := lo; j < hi; j++ {
			if !mb[j] && ra[i] == rb[j] {
				ma[i], mb[j] = true, true
				m++
				break
			}
		}
	}
	if m == 0 {
		return 0
	}
	tr, k := 0, 0
	for i := 0; i < la; i++ {
		if ma[i] {
			for !mb[k] {
				k++
			}
			if ra[i] != rb[k] {
				tr++
			}
			k++
		}
	}
	fm := float64(m)
	j := (fm/float64(la) + fm/float64(lb) + (fm-float64(tr)/2)/fm) / 3
	if j > 0.7 {
		p := 0
		for p < 4 && p < la && p < lb && ra[p] == rb[p] {
			p++
		}
		j += float64(p) * 0.1 * (1 - j)
	}
	return j
}

func softTok(ta, tb []string) float64 {
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	s := 0.0
	for _, x := range ta {
		best := 0.0
		for _, y := range tb {
			if v := jaroWinkler(x, y); v > best {
				best = v
			}
		}
		s += best
	}
	return s / float64(len(ta))
}

func sortedJoin(t []string) string {
	c := append([]string{}, t...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j] < c[j-1]; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	return strings.Join(c, " ")
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func dobFeats(da, db string) []float64 {
	if da == "" || db == "" {
		return []float64{0, 0, 0, 0, 0, -1, 0}
	}
	ya, ma, dda := da[:4], da[5:7], da[8:10]
	yb, mb, ddb := db[:4], db[5:7], db[8:10]
	b := func(x bool) float64 {
		if x {
			return 1
		}
		return 0
	}
	yd := math.Abs(float64(atoi(ya) - atoi(yb)))
	if yd > 10 {
		yd = 10
	}
	return []float64{1, b(da == db), b(ya == yb), b(ya == yb && ma == ddb && dda == mb), b(ma+dda == "0701" || mb+ddb == "0701"), yd, b(ma == mb && dda == ddb)}
}

const NFEAT = 23 // 21 classical + cos + bestgap

func pairFeats(a, b *Rec, cos, gap float64) []float64 {
	ta, tb, sa, sb := a.tok, b.tok, a.skel, b.skel
	f := make([]float64, 0, NFEAT)
	b2f := func(x bool) float64 {
		if x {
			return 1
		}
		return 0
	}
	first, last := 0.0, 0.0
	if len(ta) > 0 && len(tb) > 0 {
		first = jaroWinkler(ta[0], tb[0])
		last = jaroWinkler(ta[len(ta)-1], tb[len(tb)-1])
	}
	inter, uni := 0, map[string]bool{}
	sbSet := map[string]bool{}
	for _, x := range sb {
		sbSet[x] = true
		uni[x] = true
	}
	saSet := map[string]bool{}
	for _, x := range sa {
		if !saSet[x] && sbSet[x] {
			inter++
		}
		saSet[x] = true
		uni[x] = true
	}
	jac := 0.0
	if len(uni) > 0 {
		jac = float64(inter) / float64(len(uni))
	}
	nd := math.Abs(float64(len(ta) - len(tb)))
	f = append(f, jaroWinkler(a.lat, b.lat), jaroWinkler(sortedJoin(ta), sortedJoin(tb)), first, last, softTok(ta, tb),
		b2f(len(sa) > 0 && len(sb) > 0 && sa[0] == sb[0]), b2f(len(sa) > 0 && len(sb) > 0 && sa[len(sa)-1] == sb[len(sb)-1]),
		jac, jaroWinkler(strings.Join(sa, ""), strings.Join(sb, "")), float64(len(ta)), float64(len(tb)), nd)
	f = append(f, dobFeats(a.Dob, b.Dob)...)
	f = append(f, b2f(a.Sex == b.Sex), b2f(a.isAr == b.isAr), cos, gap)
	return f
}

func ruleKeys(r *Rec) []string {
	set := map[string]bool{}
	for _, x := range r.skel {
		set[x] = true
	}
	sk := []string{}
	for x := range set {
		sk = append(sk, x)
	}
	sk = strings.Fields(sortedJoin(sk))
	ks := []string{}
	for i := 0; i < len(sk); i++ {
		for j := i + 1; j < len(sk); j++ {
			ks = append(ks, "p|"+sk[i]+"|"+sk[j])
		}
		if r.Dob != "" {
			ks = append(ks, "y|"+sk[i]+"|"+r.Dob[:4], "d|"+sk[i]+"|"+r.Dob[5:])
		}
	}
	if len(sk) == 1 {
		ks = append(ks, "s|"+sk[0])
	}
	return ks
}
