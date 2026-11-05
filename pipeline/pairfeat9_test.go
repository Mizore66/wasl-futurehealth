package main

import (
	"math"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestBuildMINamesakesExactPattern(t *testing.T) {
	ps := []Person{
		{Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F", Spl: "train", Origin: "ie", Idx: 1},
		{Qid: "b", En: "Kate L. O'Brien", Dob: "1964-02-02", Sex: "F", Spl: "train", Origin: "ie", Idx: 2},
		{Qid: "c", En: "Kate O'Brien", Dob: "1965-03-03", Sex: "U", Spl: "train", Origin: "ie", Idx: 3},
	}
	xs, base := buildMINamesakes(ps[:1], 0.999999, rand.New(rand.NewSource(1)))
	if len(xs) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("xs=%+v base=%v", xs, base)
	}
	x := xs[0]
	if x.Qid != "mi-a" || x.Dob != "" || x.Eid != "" || x.Sex != "F" || x.Idx != 0 {
		t.Fatalf("namesake %+v", x)
	}
	got := strings.Fields(x.En)
	if len(got) != 3 || got[0] != "Kate" || got[1] != "A" || got[2] != "O'Brien" {
		t.Fatalf("name pattern %q", x.En)
	}
	for _, ps2 := range [][]Person{ps[1:2], ps[2:3]} {
		if got, _ := buildMINamesakes(ps2, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
			t.Errorf("ineligible base produced %+v", got)
		}
	}
}

func TestMINamesakesDeterministicAndZeroRate(t *testing.T) {
	ps := []Person{{Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F"}}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildMINamesakes(ps, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate zero generated a person or consumed randomness")
	}
	a, _ := buildMINamesakes(ps, 0.999999, rand.New(rand.NewSource(9)))
	b, _ := buildMINamesakes(ps, 0.999999, rand.New(rand.NewSource(9)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("generator is not deterministic")
	}
}

func TestPairfeat9Registration(t *testing.T) {
	if !usesNameIDF("pairfeat9") || !xscriptCapFor("pairfeat9") {
		t.Fatal("pairfeat9 must inherit rarity features and cross-script cap")
	}
	if err := validateModelPolicy("pairfeat9", "v7strict"); err != nil {
		t.Fatal(err)
	}
	if err := validateIDFNamesakes("-minamesakes", "featdump", 0.002, "idf.json"); err != nil {
		t.Fatal(err)
	}
	if err := validateIDFNamesakes("-minamesakes", "gen", 0.002, "idf.json"); err == nil {
		t.Fatal("training-only flag accepted on gen")
	}
}

func TestMINameUTF8Initial(t *testing.T) {
	if _, ok := miName("Anna Ö. Müller", 0); ok {
		t.Error("non-ASCII dotted initial not detected")
	}
	if _, ok := miName("Anna Ö Müller", 0); ok {
		t.Error("non-ASCII initial not detected")
	}
	if got, ok := miName("Zoë Müller", 1); !ok || got != "Zoë B Müller" {
		t.Errorf("got %q %v", got, ok)
	}
}

func TestMIRegisterExactPair(t *testing.T) {
	b := Person{Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F", Idx: 4, Eid: "784-1963-1234567-1"}
	x := Person{En: "Kate A O'Brien", Sex: "F"}
	regs := []fdReg{{0, "other", "1990-01-01", "M", "", 1}, {2, "كيت", "1963-05-05", "U", "", 4}, {2, "Kate O'Brien", "", "", "", 4}, {3, "z", "", "M", "", 2}}
	got := miRegister(regs, 2, &b, 9, &x, rand.New(rand.NewSource(1)))
	if len(got) != 4 || got[0] != regs[0] || got[1] != regs[3] {
		t.Fatalf("other registrations changed: %+v", got)
	}
	if g := got[2]; g.pid != 2 || g.name != b.En || g.dob != b.Dob || g.sex != "F" || g.tid != 4 {
		t.Fatalf("base %+v", g)
	}
	if g := got[3]; g.pid != 9 || g.name != x.En || g.dob != "" || g.sex != "F" || g.eid != "" || g.tid != 0 {
		t.Fatalf("namesake %+v", g)
	}
}

func TestPairfeat9FeatDump(t *testing.T) {
	cliState(t)
	tb, _ := pf7Table(t)
	old := MINamesakeRate
	defer func() { MINamesakeRate = old }()
	MINamesakeRate = 0.1
	rows, sc, c1, s1 := pf7Dump(t, "cal", 2011, 0, 0, 0, tb)
	_, _, c2, s2 := pf7Dump(t, "cal", 2011, 0, 0, 0, tb)
	if string(c1) != string(c2) || string(s1) != string(s2) {
		t.Fatal("mi featdump is not deterministic")
	}
	xr := intList(t, sc["mi_rows"])
	added := int(sc["mi_namesakes_added"].(float64))
	if sc["mi_rate"].(float64) != 0.1 || added <= 0 || len(xr) == 0 || int(sc["mi_base_namesake_rows"].(float64)) != len(xr) {
		t.Fatalf("mi sidecar %v", sc)
	}
	col := func(r []string, i int) float64 {
		if r[2+i] == "NaN" {
			return math.NaN()
		}
		v, err := strconv.ParseFloat(r[2+i], 64)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, i := range xr {
		r := rows[i]
		if r[0] != "0" || col(r, 12) != 0 || col(r, 19) != 1 || col(r, 20) != 1 || !math.IsNaN(col(r, 23)) {
			t.Fatalf("mi row %d shape %v", i, r)
		}
	}
	// zero rate: sidecar has no mi keys and the dump equals one with the flag untouched
	MINamesakeRate = 0
	_, sc0, c0, _ := pf7Dump(t, "cal", 2011, 0, 0, 0, tb)
	for k := range sc0 {
		if len(k) > 3 && k[:3] == "mi_" {
			t.Errorf("rate 0, sidecar carries %s", k)
		}
	}
	regs := []fdReg{{0, "a b", "", "M", "", 0}}
	out, kinds := pairfeat7Negatives(regs, nil, nil, 1, 1, "", nil)
	if len(kinds) != 0 || len(out) != 1 || out[0] != regs[0] {
		t.Fatal("rate 0 changed registrations")
	}
	if string(c0) == string(c1) {
		t.Error("mi rate 0.1 left the dump unchanged")
	}
}

func TestMIDobEligibility(t *testing.T) {
	ps := []Person{
		{Qid: "e", En: "Kate O'Brien", Dob: "", Sex: "F"},
		{Qid: "p", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F"},
	}
	xs, base := buildMINamesakes(ps, 0.999999, rand.New(rand.NewSource(1)))
	if len(xs) != 1 || base[0] != 1 || xs[0].Qid != "mi-p" {
		t.Fatalf("xs=%+v base=%v", xs, base)
	}
}

func TestPairfeat7NegativesMIFirstOffsetsAndOverlap(t *testing.T) {
	old := MINamesakeRate
	defer func() { MINamesakeRate = old }()
	MINamesakeRate = 0.999999
	ps := []Person{
		{Qid: "a", En: "Kate O'Brien", Dob: "1963-01-01", Sex: "F", Idx: 1, Eid: "784-1963-1234567-1"},
		{Qid: "b", En: "John Smith", Dob: "1970-02-02", Sex: "M", Idx: 2, Eid: "784-1970-1234567-1"},
		{Qid: "c", En: "Mary Jones", Dob: "1980-03-03", Sex: "F", Idx: 3, Eid: "784-1980-1234567-1"},
	}
	var regs []fdReg
	for i := range ps {
		regs = append(regs, fdReg{i, ps[i].En, ps[i].Dob, ps[i].Sex, "", ps[i].Idx})
	}
	out, kinds := pairfeat7Negatives(regs, ps, nil, 5, 10, "", map[int]bool{1: true})
	if len(kinds) != 1 || kinds[0].key != "mi" || kinds[0].added != 2 {
		t.Fatalf("kinds %+v", kinds)
	}
	if !reflect.DeepEqual(kinds[0].base, map[int]int{10: 0, 11: 2}) {
		t.Fatalf("first offsets/bases %v", kinds[0].base)
	}
	n := map[int]int{}
	for _, g := range out {
		n[g.pid]++
	}
	if len(out) != 5 || n[0] != 1 || n[1] != 1 || n[2] != 1 || n[10] != 1 || n[11] != 1 {
		t.Fatalf("registrations %+v", out)
	}
	for _, g := range out {
		if g.pid == 1 && g != regs[1] {
			t.Fatalf("overlapping base registration changed: %+v", g)
		}
	}
}
