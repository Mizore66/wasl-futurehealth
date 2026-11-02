package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	// Step 1 goldens of the given-name path, taken at BASE with -namesakes 0.1 (train) and plain (cal).
	goldenTrainNs1019   = "1f1b1722b57bd5e971036c3cb0dc9993b0c6ce9b99055222f93f7b5a73b6d80d"
	goldenTrainNs1019Sc = "8e2ed2b80c1e180e3409ada6b095d361aa365fba8c8252c92b50c445dd1e145a"
	goldenCal2011Sc     = "9c0ab119ec32be21080e15e1086c5bbc9f464eb58473d1e441917bc005653828"
	goldenTrain1019     = "6badc8881764b6c5209c9f8ffdb6ab393fdcfa00f0188161d637667fca5dba5f"
	goldenCal2011       = "7ed50419dda1df06b96091446c76c5be9ae63c1391999c5e2dc4100c98a8c9d2"
)

func TestNamesakeValidate(t *testing.T) {
	for _, c := range []struct {
		cmd  string
		rate float64
		ok   bool
	}{
		{"featdump", 0, true}, {"featdump", 0.1, true}, {"gen", 0, true},
		{"gen", 0.1, false}, {"orchestrate", 0.1, false}, {"featdump", -0.1, false}, {"featdump", 1, false},
	} {
		if err := validateNamesakes(c.cmd, c.rate); (err == nil) != c.ok {
			t.Errorf("validateNamesakes(%q, %v) = %v, want ok=%v", c.cmd, c.rate, err, c.ok)
		}
	}
}

func TestNamesakeDob(t *testing.T) {
	for _, base := range []string{"1976-03-08", "1970-01-01"} {
		r := rand.New(rand.NewSource(5))
		one, n, failed := 0, 20000, 0
		for i := 0; i < n; i++ {
			s, ok := namesakeDob(base, r)
			if !ok { // 20 invalid redraws in a row: the person then simply gets no namesake
				failed++
				continue
			}
			if len(s) != 10 || s[4] != '-' || s[7] != '-' || !realDob(s) || s[:4] < "1900" || s[:4] > "2025" {
				t.Fatalf("%s: invalid %q", base, s)
			}
			d := 0
			for k := 0; k < 10; k++ {
				if s[k] != base[k] {
					d++
				}
			}
			if d != 1 && d != 2 {
				t.Fatalf("%s: %q differs in %d positions", base, s, d)
			}
			if d == 1 {
				one++
			}
		}
		if failed > n/100 {
			t.Errorf("%s: %d of %d draws exhausted the 20 retries", base, failed, n)
		}
		if sh := float64(one) / float64(n-failed); sh < 0.58 || sh > 0.62 {
			t.Errorf("%s: share with one digit %.4f, want 0.60 +- 0.02", base, sh)
		}
	}
}

func TestNamesakeNewEID(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		e := newEID("1976-03-08", r)
		if len(e) != 15 || e[:7] != "7841976" || !luhnOK(e) {
			t.Fatalf("bad EID %q", e)
		}
		seen[e] = true
	}
	if len(seen) < 990 {
		t.Fatalf("only %d distinct EIDs", len(seen))
	}
}

func TestNamesakeBuild(t *testing.T) {
	mk := func(q, en, sex, origin string) Person {
		return Person{Qid: q, Ar: "ar-" + q, En: en, Als: []string{"als-" + q}, Dob: "1976-03-08", Sex: sex,
			Cc: "cc-" + q, Eid: "784197612345671", Spl: "train", Origin: origin, ArSrc: "src-" + q}
	}
	pool := []Person{
		mk("q1", "Mohamed Ali", "M", "arab"),
		mk("q2", "Mohamed Hassan", "M", "arab"),
		mk("q3", "Mohamed Saleh", "F", "arab"),
		mk("q4", "Mohamed Ali", "M", "arab"),
		mk("q5", "Anand Kumar", "M", "south_asian"),
		mk("q6", "Mohamed Ahmad", "M", "south_asian"),
	}
	r := rand.New(rand.NewSource(1))
	made := 0
	for i := 0; i < 200; i++ {
		ns, base := buildNamesakes(pool[:1], pool, 0.999999, r)
		if len(ns) == 0 { // DOB redraws exhausted (about 0.2% of draws): no namesake, by design
			continue
		}
		made++
		if len(ns) != 1 || len(base) != 1 || base[0] != 0 {
			t.Fatalf("draw %d: ns=%d base=%v", i, len(ns), base)
		}
		n := ns[0]
		if n.Ar != "ar-q2" || n.En != "Mohamed Hassan" || n.Sex != "M" || n.Origin != "arab" || n.Cc != "cc-q2" {
			t.Fatalf("namesake not from q2: %+v", n)
		}
		if n.Qid != "ns-q1" || n.Spl != "train" || n.Dob == pool[0].Dob || n.Eid == pool[0].Eid || !luhnOK(n.Eid) {
			t.Fatalf("bad namesake fields: %+v", n)
		}
	}
	if made < 195 {
		t.Fatalf("only %d of 200 draws produced a namesake", made)
	}
	ns, base := buildNamesakes(pool[4:5], pool, 0.999999, r)
	if len(ns) != 0 || len(base) != 0 {
		t.Fatalf("person with no donor got a namesake: %+v", ns)
	}
}

func TestNamesakeRateZeroNoDraws(t *testing.T) {
	pool := []Person{{Qid: "a", En: "A B", Dob: "1976-03-08"}, {Qid: "b", En: "A C", Dob: "1976-03-08"}}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	ns, base := buildNamesakes(pool, pool, 0, r)
	if len(ns) != 0 || len(base) != 0 {
		t.Fatal("rate 0 produced namesakes")
	}
	if r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 consumed random draws")
	}
}

type nsSidecar struct {
	Rate       float64 `json:"rate"`
	Seed       int64   `json:"seed"`
	Added      int     `json:"namesakes_added"`
	BaseNsRows int     `json:"base_namesake_rows"`
	Rows       []int   `json:"rows"`
}

func runFD(t *testing.T, split string, seed int64, rate float64) (csvBytes []byte, sc nsSidecar, rows [][]string) {
	t.Helper()
	oldN, oldR := NoiseModel, NamesakeRate
	NoiseModel, NamesakeRate = "v1t", rate
	defer func() { NoiseModel, NamesakeRate = oldN, oldR }()
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", split, out, 400, seed)
	var err error
	if csvBytes, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out + ".namesakes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &sc); err != nil {
		t.Fatal(err)
	}
	if rows, err = csv.NewReader(bytes.NewReader(csvBytes)).ReadAll(); err != nil {
		t.Fatal(err)
	}
	return
}

func TestNamesakeFeatDumpRateZeroGolden(t *testing.T) {
	for _, g := range []struct {
		split string
		seed  int64
		hash  string
	}{{"train", 1019, goldenTrain1019}, {"cal", 2011, goldenCal2011}} {
		b, sc, _ := runFD(t, g.split, g.seed, 0)
		h := sha256.Sum256(b)
		if got := hex.EncodeToString(h[:]); got != g.hash {
			t.Errorf("%s %d: sha256 %s, want %s", g.split, g.seed, got, g.hash)
		}
		if sc.Added != 0 || len(sc.Rows) != 0 || sc.Rate != 0 {
			t.Errorf("%s: sidecar %+v", g.split, sc)
		}
	}
}

func TestNamesakeFeatDumpDeterministicLabels(t *testing.T) {
	oldN, oldR := NoiseModel, NamesakeRate
	NoiseModel, NamesakeRate = "v1t", 0.1
	defer func() { NoiseModel, NamesakeRate = oldN, oldR }()
	d := t.TempDir()
	var outs [2][2][]byte
	for i := range outs {
		out := filepath.Join(d, "o"+strconv.Itoa(i)+".csv")
		runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 400, 1019)
		outs[i][0], _ = os.ReadFile(out)
		outs[i][1], _ = os.ReadFile(out + ".namesakes.json")
	}
	if !bytes.Equal(outs[0][0], outs[1][0]) || !bytes.Equal(outs[0][1], outs[1][1]) {
		t.Fatal("featdump with namesakes is not deterministic")
	}
	var sc nsSidecar
	if err := json.Unmarshal(outs[0][1], &sc); err != nil {
		t.Fatal(err)
	}
	if sc.Added <= 0 || sc.BaseNsRows < 1 || len(sc.Rows) != sc.BaseNsRows || sc.Rate != 0.1 || sc.Seed != 1019 {
		t.Fatalf("sidecar %+v", sc)
	}
	rows, err := csv.NewReader(bytes.NewReader(outs[0][0])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range sc.Rows {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("row %d is not a label-0 row", i)
		}
	}
}

func TestPairFeat3ModelPolicy(t *testing.T) {
	for _, p := range []string{"", "v6", "v7ltt"} {
		if validateModelPolicy("pairfeat3", p) == nil {
			t.Errorf("pairfeat3 accepted policy %q", p)
		}
	}
	if err := validateModelPolicy("pairfeat3", "v7strict"); err != nil {
		t.Error(err)
	}
	for _, m := range []string{"pairfeat1", "pairfeat2"} {
		if validateModelPolicy(m, "v7ltt") == nil || validateModelPolicy(m, "v7strict") != nil {
			t.Errorf("%s behaviour changed", m)
		}
	}
	if validateModelPolicy("", "") != nil {
		t.Error("empty model set rejected")
	}
}

func TestNamesakeRateValidate(t *testing.T) {
	for _, flag := range []string{"-namesakes", "-familynamesakes"} {
		for _, c := range []struct {
			cmd  string
			rate float64
			ok   bool
		}{
			{"featdump", 0, true}, {"featdump", 0.1, true}, {"gen", 0, true},
			{"gen", 0.1, false}, {"featdump", -0.1, false}, {"featdump", 1, false}, {"featdump", math.NaN(), false},
		} {
			err := validateNamesakeRate(flag, c.cmd, c.rate)
			if (err == nil) != c.ok {
				t.Errorf("validateNamesakeRate(%q, %q, %v) = %v, want ok=%v", flag, c.cmd, c.rate, err, c.ok)
			}
			if err != nil && !strings.Contains(err.Error(), flag) {
				t.Errorf("error %q does not name %s", err, flag)
			}
		}
	}
	if validateNamesakes("featdump", math.NaN()) == nil {
		t.Error("validateNamesakes accepted NaN")
	}
}

func TestFamilySeed(t *testing.T) {
	if familySeed(1019) != 1019*2654435761+130363 || familySeed(1019) == namesakeSeed(1019) {
		t.Fatal("familySeed")
	}
}

func TestFamilyNamesakeBuild(t *testing.T) {
	mk := func(q, en, sex, origin string) Person {
		return Person{Qid: q, Ar: "ar-" + q, En: en, Als: []string{"als-" + q}, Dob: "1976-03-08", Sex: sex,
			Cc: "cc-" + q, Eid: "784197612345671", Spl: "train", Origin: origin, ArSrc: "src-" + q}
	}
	pool := []Person{
		mk("q1", "Mohamed Ali", "M", "arab"),
		mk("q2", "Hassan Ali", "M", "arab"),
		mk("q3", "Mohamed Ali", "M", "arab"),
		mk("q4", "Sara Ali", "F", "arab"),
		mk("q5", "Hassan Ali", "M", "south_asian"),
		mk("q6", "Omar Khan", "M", "arab"),
	}
	r := rand.New(rand.NewSource(1))
	made := 0
	for i := 0; i < 200; i++ {
		ns, base := buildFamilyNamesakes(pool[:1], pool, 0.999999, r)
		if len(ns) == 0 {
			continue
		}
		made++
		if len(ns) != 1 || len(base) != 1 || base[0] != 0 {
			t.Fatalf("draw %d: ns=%d base=%v", i, len(ns), base)
		}
		n := ns[0]
		if n.Ar != "ar-q2" || n.En != "Hassan Ali" || n.Sex != "M" || n.Origin != "arab" || n.Cc != "cc-q2" || n.ArSrc != "src-q2" {
			t.Fatalf("family namesake not from q2: %+v", n)
		}
		if n.Qid != "nf-q1" || !strings.HasPrefix(n.Qid, "nf-") || n.Spl != "train" || n.Dob == pool[0].Dob || n.Eid == pool[0].Eid || !luhnOK(n.Eid) {
			t.Fatalf("bad family namesake fields: %+v", n)
		}
	}
	if made < 195 {
		t.Fatalf("only %d of 200 draws produced a family namesake", made)
	}
	// Omar Khan (no one else shares Khan) and a lone-Ali female with only a same-first-token donor: no donor.
	if ns, _ := buildFamilyNamesakes(pool[5:6], pool, 0.999999, r); len(ns) != 0 {
		t.Fatalf("person with no donor got a family namesake: %+v", ns)
	}
	if ns, _ := buildFamilyNamesakes(pool[3:4], pool, 0.999999, r); len(ns) != 0 {
		t.Fatalf("Sara Ali has no other female arab Ali donor: %+v", ns)
	}
}

func TestFamilyNamesakeRateZeroNoDraws(t *testing.T) {
	pool := []Person{{Qid: "a", En: "A B", Dob: "1976-03-08"}, {Qid: "b", En: "C B", Dob: "1976-03-08"}}
	r, fresh := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	if ns, base := buildFamilyNamesakes(pool, pool, 0, r); len(ns) != 0 || len(base) != 0 {
		t.Fatal("rate 0 produced family namesakes")
	}
	if r.Int63() != fresh.Int63() {
		t.Fatal("rate 0 consumed random draws")
	}
}

func TestNamesakePairRow(t *testing.T) {
	nsBase := map[int]int{10: 3, 11: 4}
	for _, c := range []struct {
		a, b int
		want bool
	}{{10, 3, true}, {3, 10, true}, {11, 4, true}, {4, 11, true}, {10, 4, false}, {3, 4, false}, {10, 11, false}, {5, 6, false}} {
		if got := namesakePairRow(nsBase, c.a, c.b); got != c.want {
			t.Errorf("namesakePairRow(%d,%d) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
	if namesakePairRow(nil, 1, 2) {
		t.Error("nil map matched")
	}
}

type fdSidecar struct {
	nsSidecar
	FamilyRate  float64 `json:"family_rate"`
	FamilyAdded int     `json:"family_namesakes_added"`
	FamilyBase  int     `json:"family_base_namesake_rows"`
	FamilyRows  []int   `json:"family_rows"`
}

func runFD2(t *testing.T, split string, seed int64, ns, fam float64) (csvBytes, scBytes []byte) {
	t.Helper()
	oldN, oldR, oldF := NoiseModel, NamesakeRate, FamilyNamesakeRate
	NoiseModel, NamesakeRate, FamilyNamesakeRate = "v1t", ns, fam
	defer func() { NoiseModel, NamesakeRate, FamilyNamesakeRate = oldN, oldR, oldF }()
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", split, out, 400, seed)
	var err error
	if csvBytes, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	if scBytes, err = os.ReadFile(out + ".namesakes.json"); err != nil {
		t.Fatal(err)
	}
	return
}

func hex256(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestFamilyFeatDumpRateZeroGoldens(t *testing.T) {
	c, s := runFD2(t, "train", 1019, 0.1, 0)
	if hex256(c) != goldenTrainNs1019 || hex256(s) != goldenTrainNs1019Sc {
		t.Errorf("train 1019 -namesakes 0.1: csv %s sidecar %s", hex256(c), hex256(s))
	}
	c, s = runFD2(t, "cal", 2011, 0, 0)
	if hex256(c) != goldenCal2011 || hex256(s) != goldenCal2011Sc {
		t.Errorf("cal 2011: csv %s sidecar %s", hex256(c), hex256(s))
	}
}

func TestFamilyFeatDumpDeterministicLabels(t *testing.T) {
	c1, s1 := runFD2(t, "train", 1019, 0.1, 0.1)
	c2, s2 := runFD2(t, "train", 1019, 0.1, 0.1)
	if !bytes.Equal(c1, c2) || !bytes.Equal(s1, s2) {
		t.Fatal("featdump with both namesake kinds is not deterministic")
	}
	var sc fdSidecar
	if err := json.Unmarshal(s1, &sc); err != nil {
		t.Fatal(err)
	}
	if sc.FamilyAdded <= 0 || sc.FamilyBase < 1 || len(sc.FamilyRows) != sc.FamilyBase || sc.FamilyRate != 0.1 {
		t.Fatalf("sidecar %+v", sc)
	}
	if sc.Added <= 0 || len(sc.Rows) != sc.BaseNsRows {
		t.Fatalf("given-name sidecar fields %+v", sc.nsSidecar)
	}
	rows, err := csv.NewReader(bytes.NewReader(c1)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, i := range sc.Rows {
		seen[i] = true
	}
	for _, i := range append(append([]int{}, sc.Rows...), sc.FamilyRows...) {
		if i < 0 || i >= len(rows) || rows[i][0] != "0" {
			t.Fatalf("row %d is not a label-0 row", i)
		}
	}
	for _, i := range sc.FamilyRows {
		if seen[i] {
			t.Fatalf("row %d in both rows and family_rows", i)
		}
	}
	// the given-name kind is unchanged by adding the family kind only if its sidecar keys match the family-off run
	_, s0 := runFD2(t, "train", 1019, 0.1, 0)
	var sc0 map[string]any
	if err := json.Unmarshal(s0, &sc0); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"family_rate", "family_namesakes_added", "family_base_namesake_rows", "family_rows"} {
		if _, ok := sc0[k]; ok {
			t.Errorf("family-off sidecar has key %s", k)
		}
	}
}

func TestPairFeat4ModelPolicy(t *testing.T) {
	for _, p := range []string{"", "v6", "v7ltt"} {
		if validateModelPolicy("pairfeat4", p) == nil {
			t.Errorf("pairfeat4 accepted policy %q", p)
		}
	}
	if err := validateModelPolicy("pairfeat4", "v7strict"); err != nil {
		t.Error(err)
	}
	for _, m := range []string{"pairfeat1", "pairfeat2", "pairfeat3"} {
		if validateModelPolicy(m, "v7ltt") == nil || validateModelPolicy(m, "v7strict") != nil {
			t.Errorf("%s behaviour changed", m)
		}
	}
}
