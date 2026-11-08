package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// featdump -persons 400 -seed 1019 -noise v1t -namesakes 0.1 -familynamesakes 0.1 (the pairfeat4/pairfeat5 train argv),
// taken with the CLI (UrduFix on) at 931a823, before pairfeat6.
const (
	goldenTrainNsFam1019   = "0b86ffd7f1082bc789b1cdfb2fc5e61af637b23ee9504b94dcd530ad1ec5b99e"
	goldenTrainNsFam1019Sc = "2e292038109d913b8d036f87bfaf8a06dd0a38246340c64914102497a3ca8c16"
)

func idf6() *NameIDFTable {
	return &NameIDFTable{Version: 1, Registrations: 100,
		Counts: map[string]int{"mhmd": 99, "ahmd": 49, "slh": 9, "zbr": 1, "cmn": 100, "cmm": 100}}
}

func setNameIDF(t *testing.T, tb *NameIDFTable) {
	t.Helper()
	old := NameIDF
	NameIDF = tb
	t.Cleanup(func() { NameIDF = old })
}

func TestNameIDFDefaultsNil(t *testing.T) {
	if NameIDF != nil || NFEAT6 != 34 || NFEAT != 30 {
		t.Fatalf("NameIDF=%v NFEAT6=%d NFEAT=%d", NameIDF, NFEAT6, NFEAT)
	}
}

func TestPairfeat6Weight(t *testing.T) {
	tb := idf6()
	for tok, want := range map[string]float64{"mhmd": 0, "ahmd": math.Log(2), "slh": math.Log(10), "zbr": math.Log(50),
		"cmn": math.Log(100.0 / 101.0), "unseen": math.Log(100)} {
		if got := tb.Weight(tok); math.Abs(got-want) > 1e-15 {
			t.Errorf("Weight(%q) = %v, want %v", tok, got, want)
		}
	}
}

func TestPairfeat6DateFeatures(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		name, da, db string
		f30, f31     float64
	}{
		{"real equal", "1980-05-17", "1980-05-17", 0, 1},
		{"real differ", "1980-05-17", "1981-05-17", 0, 0},
		{"both 1 January", "1980-01-01", "1980-01-01", 1, nan},
		{"one 1 July", "1980-07-01", "1980-05-17", 1, nan},
		{"day 00", "1980-05-00", "1980-05-17", 1, nan},
		{"both day 00", "1980-05-00", "1980-05-00", 1, nan},
		{"day 00 with invalid month", "1980-13-00", "1980-05-17", nan, nan},
		{"missing", "", "1980-05-17", nan, nan},
		{"invalid", "1980-02-30", "1980-05-17", nan, nan},
		{"sentinel placeholder", "0001-01-01", "0001-01-01", nan, nan},
		{"wrong length", "1980-5-17", "1980-05-17", nan, nan},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := &Rec{Dob: c.da, skel: []string{"slh"}}, &Rec{Dob: c.db, skel: []string{"slh"}}
			for _, f := range [][]float64{pairfeat6Feats(a, b, idf6()), pairfeat6Feats(b, a, idf6())} {
				if len(f) != 4 || !eqNaN(f[0], c.f30) || !eqNaN(f[1], c.f31) {
					t.Errorf("f30=%v f31=%v, want %v %v", f[0], f[1], c.f30, c.f31)
				}
			}
		})
	}
}

func TestPairfeat6NameFeatures(t *testing.T) {
	nan := math.NaN()
	for _, c := range []struct {
		name     string
		sa, sb   []string
		f32, f33 float64
	}{
		{"common shared, rare and unseen unmatched", []string{"mhmd", "slh"}, []string{"mhmd", "krm"}, 0, math.Log(100)},
		{"all match, repeats count once", []string{"ahmd", "ahmd", "slh"}, []string{"slh", "ahmd"}, math.Log(2) + math.Log(10), 0},
		{"disjoint", []string{"zbr"}, []string{"ahmd"}, 0, math.Log(50)},
		{"negative weights: the largest, not clamped", []string{"cmn", "mhmd"}, []string{"cmm", "mhmd"}, 0, math.Log(100.0 / 101.0)},
		{"empty skeleton", nil, []string{"slh"}, nan, nan},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, b := &Rec{Dob: "1980-05-17", skel: c.sa}, &Rec{Dob: "1980-05-17", skel: c.sb}
			ab, ba := pairfeat6Feats(a, b, idf6()), pairfeat6Feats(b, a, idf6())
			for i := 2; i < 4; i++ {
				if !eqNaN(ab[i], ba[i]) {
					t.Errorf("f%d not symmetric: %v vs %v", 30+i, ab[i], ba[i])
				}
			}
			if !eqNaN(ab[2], c.f32) || !eqNaN(ab[3], c.f33) {
				t.Errorf("f32=%v f33=%v, want %v %v", ab[2], ab[3], c.f32, c.f33)
			}
		})
	}
}

func TestPairFeatsPairfeat6Appended(t *testing.T) {
	a := &Rec{Eid: "784198020775241", Dob: "1980-01-01", Sex: "F", isAr: true, lat: "mona saleh", tok: []string{"mona", "saleh"}, skel: []string{"mn", "slh"}}
	b := &Rec{Eid: "784198020775231", Dob: "1980-01-01", Sex: "F", lat: "mona salih", tok: []string{"mona", "salih"}, skel: []string{"mn", "slh"}}
	setNameIDF(t, nil)
	base := pairFeats(a, b, 0.5, 0.25)
	if len(base) != NFEAT {
		t.Fatalf("NameIDF nil: len %d, want %d", len(base), NFEAT)
	}
	setNameIDF(t, idf6())
	f := pairFeats(a, b, 0.5, 0.25)
	if len(f) != NFEAT6 {
		t.Fatalf("NameIDF set: len %d, want %d", len(f), NFEAT6)
	}
	for i := range base {
		if !eqNaN(f[i], base[i]) {
			t.Errorf("f%d=%v changed (was %v)", i, f[i], base[i])
		}
	}
	want := []float64{1, math.NaN(), math.Log(100) + math.Log(10), 0}
	for i, w := range want {
		if !eqNaN(f[NFEAT+i], w) {
			t.Errorf("f%d=%v want %v", NFEAT+i, f[NFEAT+i], w)
		}
	}
}

func TestNameIDFLoad(t *testing.T) {
	d := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.json", `{"version":1,"split":"train","persons":5000,"seeds":[1019],"noise":"v1t","registrations":10,"counts":{"mhmd":10,"slh":1}}`)
	tb := LoadNameIDF(good)
	if tb.Registrations != 10 || tb.Counts["slh"] != 1 || tb.Weight("x") != math.Log(10) || tb.Split != "train" {
		t.Fatalf("loaded %+v", tb)
	}
	for name, body := range map[string]string{
		"version":        `{"version":3,"registrations":10,"counts":{}}`,
		"v2 without own": `{"version":2,"registrations":10,"counts":{"a":1}}`,
		"zero n":         `{"version":1,"registrations":0,"counts":{}}`,
		"no counts":      `{"version":1,"registrations":10}`,
		"count above n":  `{"version":1,"registrations":10,"counts":{"a":11}}`,
		"zero count":     `{"version":1,"registrations":10,"counts":{"a":0}}`,
		"empty token":    `{"version":1,"registrations":10,"counts":{"":1}}`,
		"malformed":      `{"version":1,`,
	} {
		if _, err := readNameIDF(write(strings.ReplaceAll(name, " ", "_")+".json", body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	missing := filepath.Join(d, "missing.json")
	if _, err := readNameIDF(missing); err == nil {
		t.Error("missing file accepted")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("LoadNameIDF did not panic on a missing file")
			}
		}()
		LoadNameIDF(missing)
	}()
}

func TestNameIDFFor(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "name_idf_pairfeat6.json")
	if err := os.WriteFile(p, []byte(`{"version":1,"registrations":5,"counts":{"a":2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cmd, path, ms string
		load, err     bool
	}{
		{"featdump", p, "", true, false},
		{"featdump", "", "", false, false},
		{"featdump", "", "pairfeat6", false, false},
		{"orchestrate", "", "pairfeat6", true, false},
		{"audit", "", "pairfeat6", true, false},
		{"report7", "", "pairfeat6", true, false},
		{"orchestrate", "", "pairfeat5", false, false},
		{"orchestrate", "", "", false, false},
		{"orchestrate", p, "pairfeat6", false, true},
		{"gen", p, "", false, true},
	} {
		tb, err := nameIDFFor(c.cmd, c.path, c.ms, d)
		if (err != nil) != c.err || (tb != nil) != c.load {
			t.Errorf("nameIDFFor(%q, %q, %q) = %v, %v; want load=%v err=%v", c.cmd, c.path, c.ms, tb, err, c.load, c.err)
		}
	}
	if _, err := nameIDFFor("orchestrate", "", "pairfeat6", t.TempDir()); err == nil {
		t.Error("pairfeat6 without a table in -wasl accepted")
	}
}

func TestPairfeat6PolicyAndCap(t *testing.T) {
	for _, p := range []string{"", "v6", "v7ltt"} {
		if err := validateModelPolicy("pairfeat6", p); err == nil || !strings.Contains(err.Error(), "pairfeat6") {
			t.Errorf("validateModelPolicy(pairfeat6, %q) = %v, want an error naming pairfeat6", p, err)
		}
	}
	if err := validateModelPolicy("pairfeat6", "v7strict"); err != nil {
		t.Error(err)
	}
	for ms, want := range map[string]bool{"": false, "pairfeat1": false, "pairfeat2": false, "pairfeat3": false,
		"pairfeat4": false, "pairfeat5": true, "pairfeat6": true} {
		if got := xscriptCapFor(ms); got != want {
			t.Errorf("xscriptCapFor(%q) = %v, want %v", ms, got, want)
		}
	}
}

func TestPairfeat6OffFeatDumpGolden(t *testing.T) {
	setNameIDF(t, nil)
	old := UrduFix
	UrduFix = true // the golden was taken with the CLI, which turns the Urdu fix on
	defer func() { UrduFix = old }()
	// earlier tests may leave the lung/DR/breast/LDCT/IFHAS programme providers registered; the CLI golden (no
	// programme data in ../data) has none, so run with the base providers only.
	prog := map[string]bool{}
	for _, l := range [][]ProvCfg{lungProviders, drProviders, breastProviders, ldctProviders, ifhasProviders} {
		for _, p := range l {
			prog[p.ID] = true
		}
	}
	savedP := Providers
	defer func() { Providers = savedP }()
	Providers = nil
	for _, p := range savedP {
		if !prog[p.ID] {
			Providers = append(Providers, p)
		}
	}
	c, s := runFD2(t, "train", 1019, 0.1, 0.1)
	if hex256(c) != goldenTrainNsFam1019 || hex256(s) != goldenTrainNsFam1019Sc {
		t.Errorf("train 1019 -namesakes 0.1 -familynamesakes 0.1: csv %s sidecar %s", hex256(c), hex256(s))
	}
}
