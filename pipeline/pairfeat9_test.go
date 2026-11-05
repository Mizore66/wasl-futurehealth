package main

import (
	"math/rand"
	"reflect"
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
	a, _ := buildMINamesakes(ps, 0.5, rand.New(rand.NewSource(9)))
	b, _ := buildMINamesakes(ps, 0.5, rand.New(rand.NewSource(9)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("generator is not deterministic")
	}
	regs := miRegister(nil, 9, &a[0])
	if len(regs) != 1 || regs[0].pid != 9 || regs[0].name != a[0].En || regs[0].dob != "" || regs[0].eid != "" {
		t.Fatalf("registration %+v", regs)
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
