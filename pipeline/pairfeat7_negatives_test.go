package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// pf7Dump: pf6Dump with missing-date and same-date hard negatives at rates md and rd, parsed.
func pf7Dump(t *testing.T, split string, seed int64, xs, md, rd float64, tb *NameIDFTable) (rows [][]string, sc map[string]any, csvB, scB []byte) {
	t.Helper()
	oldM, oldD := MDNamesakeRate, RDNamesakeRate
	defer func() { MDNamesakeRate, RDNamesakeRate = oldM, oldD }()
	MDNamesakeRate, RDNamesakeRate = md, rd
	csvB, scB = pf6Dump(t, split, seed, xs, tb)
	if err := json.Unmarshal(scB, &sc); err != nil {
		t.Fatal(err)
	}
	var err error
	if rows, err = csv.NewReader(bytes.NewReader(csvB)).ReadAll(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPairfeat7Validate(t *testing.T) {
	for _, flag := range []string{"-mdnamesakes", "-rdnamesakes"} {
		for _, c := range []struct {
			cmd  string
			rate float64
			idf  string
			ok   bool
		}{
			{"featdump", 0, "", true}, {"featdump", 0.1, "t.json", true}, {"featdump", 0.1, "", false},
			{"gen", 0.1, "t.json", false}, {"featdump", 1, "t.json", false}, {"featdump", -0.1, "t.json", false},
			{"orchestrate", 0, "", true},
		} {
			err := validateIDFNamesakes(flag, c.cmd, c.rate, c.idf)
			if (err == nil) != c.ok || (err != nil && !strings.Contains(err.Error(), flag)) {
				t.Errorf("validateIDFNamesakes(%s, %q, %v, %q) = %v, want ok=%v", flag, c.cmd, c.rate, c.idf, err, c.ok)
			}
		}
	}
	seeds := map[int64]bool{}
	for _, s := range []int64{namesakeSeed(1019), familySeed(1019), xsSeed(1019), mdSeed(1019), rdSeed(1019)} {
		seeds[s] = true
	}
	if len(seeds) != 5 {
		t.Error("hard-negative kinds share a generator seed")
	}
}

func TestGivenOnly(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Mohammed Salem", "Mohammed Karim Nasser", true},
		{"Fatima Ali", "Fatima Hassan", true},
		{"Mohammed Al Salem", "Mohammed Nasser", true},    // the article is not a token
		{"Mohammed Salem", "Mohammed Salem Hamad", false}, // shares Salem
		{"Mohammed Salem", "Mohammed Hamad Salem", false}, // in any position
		{"Mohammed Selim", "Mohammed Salem", false},       // same skeleton slm
		{"Mohammed Salem", "Ahmed Karim", false},          // different given name
		{"Mohammed Salem", "Mohammed", false},             // single token
		{"Mohammed Salem", "Mohammed S", false},           // initial
		{"Abdullah Rahman", "Abdullah Rahmani", false},    // truncation (and the same skeleton)
		{"Mohammed Salman", "Mohammed Salem", false},      // skeleton slm is a prefix of slmn
		{"Mohammed Abd Karim", "Mohammed Abdullah", false},
		{"Mohammed A", "Mohammed Ali", false}, // initial with an empty skeleton: only the token-prefix check rejects it
	} {
		a, b := newHNName(c.a), newHNName(c.b)
		if givenOnly(a, b) != c.want || givenOnly(b, a) != c.want {
			t.Errorf("givenOnly(%q, %q) = %v / %v, want %v", c.a, c.b, givenOnly(a, b), givenOnly(b, a), c.want)
		}
	}
}

func TestNoSharedSkeleton(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"Mohammed Salem", "Karim Nasser", true},
		{"Mohammed Salem", "Ahmed Karim", false}, // mohammed and ahmed share the skeleton md
		{"Mohammed Salem", "Karim Salim", false}, // salem and salim: slm
		{"Fatima Ali", "Noor Hassan", true},
		{"Mohammed Salem", "", false},
	} {
		a, b := newHNName(c.a), newHNName(c.b)
		if noSharedSkeleton(a, strSet(a.skel), b) != c.want || noSharedSkeleton(b, strSet(b.skel), a) != c.want {
			t.Errorf("noSharedSkeleton(%q, %q), want %v", c.a, c.b, c.want)
		}
	}
}

func pf7Pool() []Person {
	return []Person{
		{Qid: "b1", Ar: "محمد سالم", En: "Mohammed Salem", Dob: "1975-06-14", Sex: "M", Spl: "train", Origin: "ae", Idx: 1},
		{Qid: "d1", Ar: "محمد كريم ناصر", En: "Mohammed Karim Nasser", Als: []string{"M Karim"}, Dob: "1980-02-02", Sex: "M", Spl: "train", Origin: "eg", Idx: 2},
		{Qid: "d2", Ar: "محمد سالم حمد", En: "Mohammed Salem Hamad", Dob: "1981-03-03", Sex: "M", Spl: "train", Idx: 3},
		{Qid: "d3", Ar: "محمد كريم", En: "Mohammed Karim", Dob: "1982-04-04", Sex: "F", Spl: "train", Idx: 4},
		{Qid: "d4", Ar: "محمد س", En: "Mohammed S", Dob: "1983-05-05", Sex: "M", Spl: "train", Idx: 5},
		{Qid: "d5", Ar: "محمد سلمان", En: "Mohammed Salman", Dob: "1984-06-06", Sex: "M", Spl: "train", Idx: 6},
		{Qid: "s1", Ar: "عمر فارس", En: "Omar Faris", Dob: "1990-01-02", Sex: "M", Spl: "train", Origin: "jo", Idx: 7},
		{Qid: "s2", Ar: "كريم ناصر", En: "Karim Nasser", Dob: "1991-02-03", Sex: "M", Spl: "train", Origin: "eg", Idx: 8},
		{Qid: "s3", Ar: "نور حسن", En: "Noor Hassan", Dob: "1992-03-04", Sex: "F", Spl: "train", Idx: 9},
	}
}

func TestBuildMDNamesakes(t *testing.T) {
	pool := pf7Pool()
	md, base := buildMDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)))
	if len(md) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("md %+v base %v", md, base)
	}
	x := md[0] // d1 is the only donor: d2 shares Salem, d3 is female, d4 an initial, d5 a skeleton prefix
	if x.Qid != "md-b1" || x.En != "Mohammed Karim Nasser" || x.Ar != "محمد كريم ناصر" || x.Dob != "" || x.Sex != "M" ||
		x.Als != nil || x.Origin != "eg" || x.Spl != "train" || x.Idx != 0 {
		t.Fatalf("namesake %+v", x)
	}
	if !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841980") {
		t.Fatalf("namesake Emirates ID %q", x.Eid)
	}
	for _, ps := range [][]Person{pool[6:7], pool[3:4]} { // no other Omar; no other female Mohammed
		if got, _ := buildMDNamesakes(ps, pool, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
			t.Errorf("%s got namesake %+v", ps[0].Qid, got)
		}
	}
	// a sampled person is never a donor: with d1 sampled too, b1 has no donor left (d1's own donor is d5)
	for _, x := range func() []Person {
		g, _ := buildMDNamesakes(pool[:2], pool, 0.999999, rand.New(rand.NewSource(1)))
		return g
	}() {
		if x.Qid == "md-b1" || x.En == pool[0].En || x.En == pool[1].En {
			t.Errorf("sampled person used as donor: %+v", x)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildMDNamesakes(pool, pool, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built namesakes or consumed random draws")
	}
	a, _ := buildMDNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	b, _ := buildMDNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("buildMDNamesakes is not deterministic")
	}
}

func TestBuildRDNamesakes(t *testing.T) {
	pool := pf7Pool()
	vec := map[string][]float32{"Mohammed Salem": {1, 0}, "Omar Faris": {0.9, 0.1}, "Karim Nasser": {0.2, 0.9}}
	emb := func(s string) []float32 {
		if v, ok := vec[s]; ok {
			return v
		}
		return []float32{0, 0}
	}
	rd, base := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb)
	if len(rd) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("rd %+v base %v", rd, base)
	}
	x := rd[0] // eligible: s1, s2 (same sex, no shared skeleton); s1 is nearest
	if x.Qid != "rd-b1" || x.En != "Omar Faris" || x.Dob != "1975-06-14" || x.Sex != "M" || x.Als != nil ||
		x.Origin != "jo" || x.Idx != 0 || !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841975") {
		t.Fatalf("stranger %+v", x)
	}
	vec["Karim Nasser"] = []float32{0.9, 0.7} // a tie with s1 keeps the first in pool order (s1); above it, s2 wins
	if rd, _ := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb); rd[0].En != "Omar Faris" {
		t.Errorf("tie went to %s", rd[0].En)
	}
	vec["Karim Nasser"] = []float32{1, 0.5}
	if rd, _ := buildRDNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)), emb); rd[0].En != "Karim Nasser" {
		t.Errorf("nearest is %s", rd[0].En)
	}
	for _, dob := range []string{"1975-01-01", "1975-07-01", "1975-06-00", "", "1975-02-30"} { // placeholder, missing, invalid
		b := pool[0]
		b.Dob = dob
		if got, _ := buildRDNamesakes([]Person{b}, pool, 0.999999, rand.New(rand.NewSource(1)), emb); len(got) != 0 {
			t.Errorf("DOB %q got stranger %+v", dob, got)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildRDNamesakes(pool, pool, 0, r, emb); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built strangers or consumed random draws")
	}
}

func TestHNRegister(t *testing.T) {
	cliState(t)
	pool := pf7Pool()
	b := pool[0]
	b.Eid = newEID(b.Dob, rand.New(rand.NewSource(5)))
	x := Person{Qid: "md-b1", Ar: pool[1].Ar, En: pool[1].En, Dob: "", Sex: "M", Eid: newEID("1980-02-02", rand.New(rand.NewSource(2)))}
	same := 0
	for seed := int64(1); seed <= 40; seed++ {
		regs := hnRegister(nil, 3, &b, 10, &x, rand.New(rand.NewSource(seed)))
		if len(regs) != 2 || regs[0].pid != 3 || regs[1].pid != 10 || regs[0].tid != 1 || regs[1].tid != 0 {
			t.Fatalf("seed %d: regs %+v", seed, regs)
		}
		if regs[0].dob != "1975-06-14" || regs[1].dob != "" || regs[0].sex != "M" || regs[1].sex != "M" {
			t.Fatalf("seed %d: date or sex noise %+v", seed, regs)
		}
		if strings.Contains(regs[0].name, "Karim") || strings.Contains(regs[1].name, "Salem") {
			t.Fatalf("seed %d: a registration carries the other person's name: %q / %q", seed, regs[0].name, regs[1].name)
		}
		if isAr(regs[0].name) == isAr(regs[1].name) {
			same++
		}
	}
	if same < 10 || same > 30 {
		t.Fatalf("same script in %d of 40 draws, want about half", same)
	}
}

// checkKind: the rows of hard-negative kind key (md or rd) are label-0 rows of no other kind (seen, updated), the sidecar
// counts match, and at least half of them have the kind's feature pattern.
func checkKind(t *testing.T, rows [][]string, sc map[string]any, key string, seen map[int]string, want func(f []string) bool) {
	t.Helper()
	added := int(sc[key+"_namesakes_added"].(float64))
	xr := intList(t, sc[key+"_rows"])
	if sc[key+"_rate"].(float64) != 0.1 || added <= 0 || len(xr) == 0 || int(sc[key+"_base_namesake_rows"].(float64)) != len(xr) {
		t.Fatalf("%s sidecar %v", key, sc)
	}
	hit := 0
	for _, i := range xr {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("%s row %d is not a label-0 row", key, i)
		}
		if k, ok := seen[i]; ok {
			t.Fatalf("%s row %d is also a %s row", key, i, k)
		}
		seen[i] = key
		if want(rows[i][2:]) {
			hit++
		}
	}
	t.Logf("%s: %d added, %d rows, %d with the pattern", key, added, len(xr), hit)
	if 2*hit < len(xr) {
		t.Errorf("%s: only %d of %d rows have the hard-negative pattern", key, hit, len(xr))
	}
}

// mdPattern: one DOB missing (f12 0, f30 NaN) and the same sex (f19). rdPattern: equal real dates (f13, f31), the same
// sex and no shared skeleton token (f7, skeleton Jaccard, 0).
func mdPattern(f []string) bool { return f[12] == "0" && f[30] == "NaN" && f[19] == "1" }
func rdPattern(f []string) bool { return f[13] == "1" && f[31] == "1" && f[19] == "1" && f[7] == "0" }

func TestPF7FeatDump(t *testing.T) {
	cliState(t)
	tb, _ := pf7Table(t)
	// train: the missing-date kind next to the given-name, family and cross-script kinds; the same-date kind's donor
	// search embeds the split pool, so it runs on the smaller cal split
	rows, sc, _, _ := pf7Dump(t, "train", 1019, 0.1, 0.1, 0, tb)
	seen := map[int]string{}
	for _, k := range []string{"rows", "family_rows", "xs_rows"} {
		for _, i := range intList(t, sc[k]) {
			seen[i] = k
		}
	}
	checkKind(t, rows, sc, "md", seen, mdPattern)
	for _, k := range []string{"rd_rate", "rd_namesakes_added", "rd_base_namesake_rows", "rd_rows"} {
		if _, ok := sc[k]; ok {
			t.Errorf("rd rate 0, sidecar carries %s", k)
		}
	}
	rows, sc, c1, s1 := pf7Dump(t, "cal", 2011, 0, 0.1, 0.1, tb)
	_, _, c2, s2 := pf7Dump(t, "cal", 2011, 0, 0.1, 0.1, tb)
	if !bytes.Equal(c1, c2) || !bytes.Equal(s1, s2) {
		t.Fatal("featdump with -mdnamesakes and -rdnamesakes is not deterministic")
	}
	for i, r := range rows {
		if len(r) != 2+NFEAT6+5 {
			t.Fatalf("row %d has %d columns, want %d", i, len(r), 2+NFEAT6+5)
		}
	}
	seen = map[int]string{}
	checkKind(t, rows, sc, "md", seen, mdPattern)
	checkKind(t, rows, sc, "rd", seen, rdPattern)
	_, sc, _, _ = pf7Dump(t, "cal", 2011, 0, 0, 0, tb)
	for k := range sc {
		if strings.HasPrefix(k, "md_") || strings.HasPrefix(k, "rd_") {
			t.Errorf("rates 0, sidecar carries %s", k)
		}
	}
}
