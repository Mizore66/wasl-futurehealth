package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestXSNamesakeValidate(t *testing.T) {
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
		if err := validateXSNamesakes(c.cmd, c.rate, c.idf); (err == nil) != c.ok {
			t.Errorf("validateXSNamesakes(%q, %v, %q) = %v, want ok=%v", c.cmd, c.rate, c.idf, err, c.ok)
		}
	}
	if xsSeed(1019) == namesakeSeed(1019) || xsSeed(1019) == familySeed(1019) {
		t.Error("cross-script namesakes share a generator seed with another kind")
	}
}

func TestValidateNoiseNameIDF(t *testing.T) {
	if err := validateNoise("nameidf", "v1t"); err != nil {
		t.Error(err)
	}
	if validateNoise("gen", "v1t") == nil || validateNoise("orchestrate", "v1t") == nil {
		t.Error("v1t accepted outside featdump/nameidf")
	}
}

func TestParseIDFSeeds(t *testing.T) {
	if got := parseIDFSeeds("1019, 1021,,1031"); !reflect.DeepEqual(got, []int64{1019, 1021, 1031}) {
		t.Fatalf("got %v", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("bad seed list accepted")
		}
	}()
	parseIDFSeeds("1019,x")
}

func xsTestPool() []Person {
	return []Person{
		{Qid: "b1", Ar: "محمد سالم", En: "Mohammed Salem", Dob: "1975-06-14", Sex: "M", Spl: "train", Origin: "ae"},
		{Qid: "d1", Ar: "محمد كريم ناصر", En: "Mohammed Karim Nasser", Dob: "1980-02-02", Sex: "M", Spl: "train", Origin: "eg"},
		{Qid: "d2", Ar: "محمد سالم حمد", En: "Mohammed Salem Hamad", Dob: "1981-03-03", Sex: "M", Spl: "train"},
		{Qid: "d3", Ar: "محمد علي", En: "Mohammed Ali", Dob: "1982-04-04", Sex: "F", Spl: "train"},
		{Qid: "b2", Ar: "سارة", En: "Sara", Dob: "1990-05-05", Sex: "F", Spl: "train"},
		{Qid: "b3", Ar: "فاطمة علي", En: "Fatima Ali", Dob: "1991-05-05", Sex: "F", Spl: "train"},
	}
}

func TestOtherTokensDisjoint(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"mohammed salem", "mohammed karim nasser", true},
		{"mohammed salem", "mohammed salem hamad", false},
		{"mohammed salem", "mohammed hamad salem", false},
		{"mohammed salem mohammed", "mohammed karim", false},
		{"mohammed karim", "mohammed salem mohammed", false},
	} {
		if got := otherTokensDisjoint(strings.Fields(c.a), strings.Fields(c.b)); got != c.want {
			t.Errorf("otherTokensDisjoint(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestBuildXSNamesakes(t *testing.T) {
	pool := xsTestPool()
	xs, base := buildXSNamesakes(pool[:1], pool, 0.999999, rand.New(rand.NewSource(1)))
	if len(xs) != 1 || !reflect.DeepEqual(base, []int{0}) {
		t.Fatalf("xs %+v base %v", xs, base)
	}
	x := xs[0]
	if x.Qid != "xs-b1" || x.En != "Mohammed Karim Nasser" || x.Ar != "محمد كريم ناصر" || x.Dob != "1975-01-01" ||
		x.Sex != "M" || x.Origin != "eg" || x.Spl != "train" {
		t.Fatalf("namesake %+v", x)
	}
	if !luhnOK(x.Eid) || !strings.HasPrefix(x.Eid, "7841975") {
		t.Fatalf("namesake Emirates ID %q", x.Eid)
	}
	// single-token name; given name with no other bearer; only donors of the other sex
	for _, ps := range [][]Person{pool[4:5], pool[5:6], pool[3:4]} {
		if got, _ := buildXSNamesakes(ps, pool, 0.999999, rand.New(rand.NewSource(1))); len(got) != 0 {
			t.Errorf("%s got namesake %+v", ps[0].Qid, got)
		}
	}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if got, _ := buildXSNamesakes(pool, pool, 0, r); len(got) != 0 || r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 built namesakes or consumed random draws")
	}
	a, _ := buildXSNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	b, _ := buildXSNamesakes(pool, pool, 0.5, rand.New(rand.NewSource(7)))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("buildXSNamesakes is not deterministic")
	}
}

func TestXSRegister(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	defer func() { NoiseModel = old }()
	pool := xsTestPool()
	b := pool[0]
	b.Eid = newEID(b.Dob, rand.New(rand.NewSource(5))) // drawEID reads the person's ID
	x := Person{Qid: "xs-b1", Ar: pool[1].Ar, En: pool[1].En, Dob: "1975-01-01", Sex: "M", Eid: newEID("1975-01-01", rand.New(rand.NewSource(2)))}
	for seed := int64(1); seed <= 20; seed++ {
		regs := xsRegister(nil, 3, &b, 10, &x, rand.New(rand.NewSource(seed)))
		if len(regs) != 2 || regs[0].pid != 3 || regs[1].pid != 10 {
			t.Fatalf("seed %d: regs %+v", seed, regs)
		}
		if regs[0].dob != "1975-01-01" || regs[1].dob != "1975-01-01" || regs[0].sex != "M" || regs[1].sex != "M" {
			t.Fatalf("seed %d: date or sex noise %+v", seed, regs)
		}
		if isAr(regs[0].name) == isAr(regs[1].name) {
			t.Fatalf("seed %d: same script %q / %q", seed, regs[0].name, regs[1].name)
		}
	}
}

func TestNameIDFBuild(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	defer func() { NoiseModel = old }()
	d := t.TempDir()
	p, q := filepath.Join(d, "a.json"), filepath.Join(d, "b.json")
	seeds := []int64{1019, 1021}
	runNameIDF("../data", "train", p, 400, seeds)
	runNameIDF("../data", "train", q, 400, seeds)
	pb, _ := os.ReadFile(p)
	qb, _ := os.ReadFile(q)
	if len(pb) == 0 || !bytes.Equal(pb, qb) {
		t.Fatal("nameidf is empty or not deterministic")
	}
	tb := LoadNameIDF(p)
	all := readPersons("../data")
	n, counts := 0, map[string]int{}
	for _, seed := range seeds {
		ps, _, r := fdSample(all, "train", 400, seed)
		var regs []fdReg
		for pid := range ps {
			regs = fdRegister(regs, pid, &ps[pid], r)
		}
		n += len(regs)
		for _, g := range regs {
			seen := map[string]bool{}
			for _, s := range skeleton(tokensOf(latinForm(g.name))) {
				if !seen[s] {
					seen[s] = true
					counts[s]++
				}
			}
		}
	}
	if tb.Registrations != n || !reflect.DeepEqual(tb.Counts, counts) {
		t.Fatalf("table N=%d (%d tokens), recount N=%d (%d tokens)", tb.Registrations, len(tb.Counts), n, len(counts))
	}
	if tb.Version != 1 || tb.Split != "train" || tb.Persons != 400 || tb.Noise != "v1t" || !reflect.DeepEqual(tb.Seeds, seeds) {
		t.Fatalf("table header %+v", tb)
	}
	if n < 1200 {
		t.Fatalf("only %d base registrations for 2 x 400 persons", n)
	}
}

func intList(t *testing.T, v any) []int {
	t.Helper()
	xs, ok := v.([]any)
	if !ok {
		t.Fatalf("not a list: %v", v)
	}
	out := make([]int, len(xs))
	for i, x := range xs {
		out[i] = int(x.(float64))
	}
	return out
}

func xsFD(t *testing.T, xs float64, tb *NameIDFTable) (rows [][]string, sc map[string]any, csvB, scB []byte) {
	t.Helper()
	oldN, oldR, oldF, oldX, oldI := NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF
	NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = "v1t", 0.1, 0.1, xs, tb
	defer func() {
		NoiseModel, NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = oldN, oldR, oldF, oldX, oldI
	}()
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 400, 1019)
	var err error
	if csvB, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	if scB, err = os.ReadFile(out + ".namesakes.json"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(scB, &sc); err != nil {
		t.Fatal(err)
	}
	if rows, err = csv.NewReader(bytes.NewReader(csvB)).ReadAll(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestXSFeatDump(t *testing.T) {
	old := NoiseModel
	NoiseModel = "v1t"
	p := filepath.Join(t.TempDir(), "name_idf_pairfeat6.json")
	runNameIDF("../data", "train", p, 400, []int64{1019})
	NoiseModel = old
	tb := LoadNameIDF(p)
	rows, sc, c1, s1 := xsFD(t, 0.1, tb)
	_, _, c2, s2 := xsFD(t, 0.1, tb)
	if !bytes.Equal(c1, c2) || !bytes.Equal(s1, s2) {
		t.Fatal("featdump with -xsnamesakes is not deterministic")
	}
	for i, r := range rows {
		if len(r) != 2+NFEAT6+5 {
			t.Fatalf("row %d has %d columns, want %d", i, len(r), 2+NFEAT6+5)
		}
	}
	added := int(sc["xs_namesakes_added"].(float64))
	xr := intList(t, sc["xs_rows"])
	if sc["xs_rate"].(float64) != 0.1 || added <= 0 || len(xr) == 0 || int(sc["xs_base_namesake_rows"].(float64)) != len(xr) {
		t.Fatalf("sidecar %v", sc)
	}
	other := map[int]bool{}
	for _, i := range append(intList(t, sc["rows"]), intList(t, sc["family_rows"])...) {
		other[i] = true
	}
	pattern := 0
	for _, i := range xr {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("xs row %d is not a label-0 row", i)
		}
		if other[i] {
			t.Fatalf("row %d is also a given- or family-name namesake row", i)
		}
		f := rows[i][2:] // f0..f33
		if f[20] == "0" && f[13] == "1" && f[19] == "1" && f[30] == "1" {
			pattern++
		}
	}
	if 2*pattern < added {
		t.Fatalf("only %d of %d cross-script namesakes gave the opposite-script, same-sex, same-placeholder-date row", pattern, added)
	}
	_, sc0, _, _ := xsFD(t, 0, tb)
	for _, k := range []string{"xs_rate", "xs_namesakes_added", "xs_base_namesake_rows", "xs_rows"} {
		if _, ok := sc0[k]; ok {
			t.Errorf("rate-0 sidecar carries %s", k)
		}
	}
}
