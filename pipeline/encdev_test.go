package main

import (
	"bytes"
	"encoding/csv"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// The default chain must keep the original file names; -modelset X must switch all four files together.
func TestModelSetFiles(t *testing.T) {
	defer func() { ModelSet = "" }()
	ModelSet = ""
	for _, f := range []string{"encoder_weights.json", "lgb_stream.json", "iso.json", "iso_v7.json"} {
		if mf(f) != f {
			t.Fatalf("default chain renamed %s -> %s", f, mf(f))
		}
	}
	ModelSet = "real_v2"
	want := map[string]string{"encoder_weights.json": "encoder_real_v2.json", "lgb_stream.json": "lgb_stream_real_v2.json",
		"iso.json": "iso_real_v2.json", "iso_v7.json": "iso_v7_real_v2.json"}
	for f, w := range want {
		if mf(f) != w {
			t.Fatalf("modelset real_v2: %s -> %s, want %s", f, mf(f), w)
		}
	}
	ModelSet = "pairfeat1"
	want = map[string]string{"encoder_weights.json": "encoder_pairfeat1.json", "lgb_stream.json": "lgb_stream_pairfeat1.json",
		"iso.json": "iso_pairfeat1.json", "iso_v7.json": "iso_v7_pairfeat1.json"}
	for f, w := range want {
		if mf(f) != w {
			t.Fatalf("modelset pairfeat1: %s -> %s, want %s", f, mf(f), w)
		}
	}
}

func TestPairFeatRejectsStaleLTTSelection(t *testing.T) {
	for _, policy := range []string{"v7ltt", "v6", ""} {
		if err := validateModelPolicy("pairfeat1", policy); err == nil {
			t.Errorf("pairfeat1 accepted policy %q", policy)
		}
	}
	for _, tc := range [][2]string{{"pairfeat1", "v7strict"}, {"", "v7ltt"}, {"real_v2", "v7ltt"}} {
		if err := validateModelPolicy(tc[0], tc[1]); err != nil {
			t.Errorf("%v: %v", tc, err)
		}
	}
}

// drawEID: 62% valid / 26% missing / 12% one-digit-mistyped; never mutates the Person.
func TestDrawEID(t *testing.T) {
	const eid = "784199012345671"
	p := &Person{Eid: eid}
	r := rand.New(rand.NewSource(7))
	var valid, missing, typo int
	for i := 0; i < 10000; i++ {
		g := drawEID(p, r)
		switch {
		case g == eid:
			valid++
		case g == "":
			missing++
		default:
			typo++
			if len(g) != len(eid) {
				t.Fatalf("mistyped length %d", len(g))
			}
			d := 0
			for k := range g {
				if g[k] != eid[k] {
					d++
				}
			}
			if d != 1 {
				t.Fatalf("mistyped EID differs at %d digits: %s", d, g)
			}
		}
	}
	if p.Eid != eid {
		t.Fatal("Person.Eid mutated")
	}
	if valid == 0 || missing == 0 || typo == 0 {
		t.Fatalf("outcomes valid=%d missing=%d typo=%d", valid, missing, typo)
	}
	t.Logf("valid=%d missing=%d typo=%d", valid, missing, typo)
}

// drawEID must reproduce the original inline runGen switch exactly (same draws, same order).
func TestDrawEIDMatchesOriginalSwitch(t *testing.T) {
	orig := func(p *Person, r *rand.Rand) string {
		x := r.Float64()
		switch {
		case x < .62:
			return p.Eid
		case x < .88:
			return ""
		default:
			bs := []byte(p.Eid)
			i := 3 + r.Intn(12)
			bs[i] = byte('0' + (int(bs[i]-'0')+1+r.Intn(9))%10)
			return string(bs)
		}
	}
	p := &Person{Eid: "784199012345671"}
	r1, r2 := rand.New(rand.NewSource(99)), rand.New(rand.NewSource(99))
	for i := 0; i < 5000; i++ {
		if a, b := drawEID(p, r1), orig(p, r2); a != b {
			t.Fatalf("draw %d: %q vs %q", i, a, b)
		}
	}
	if r1.Int63() != r2.Int63() {
		t.Fatal("random streams diverged")
	}
}

func TestFeatDumpPairFeatSchemaDeterministic(t *testing.T) {
	a := filepath.Join(t.TempDir(), "a.csv")
	b := filepath.Join(t.TempDir(), "b.csv")
	for _, out := range []string{a, b} {
		runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 40, 1001)
	}
	ba, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ba, bb) {
		t.Fatal("featdump is not deterministic")
	}
	rows, err := csv.NewReader(bytes.NewReader(ba)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("no candidate rows")
	}
	finiteEID := false
	for n, row := range rows {
		if len(row) != 37 {
			t.Fatalf("row %d has %d fields", n, len(row))
		}
		f := func(i int) float64 {
			v, err := strconv.ParseFloat(row[2+i], 64)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
		sl := func(i int) int {
			v, err := strconv.Atoi(row[32+i])
			if err != nil || (v != 0 && v != 1) {
				t.Fatalf("row %d slice %d = %q", n, i, row[32+i])
			}
			return v
		}
		f23 := f(23)
		finiteEID = finiteEID || !math.IsNaN(f23)
		if want := boolInt(f23 == 1 || f23 == 2); sl(0) != want {
			t.Fatalf("row %d near_eid %d want %d", n, sl(0), want)
		}
		if want := boolInt(f(24) == 1); sl(1) != want {
			t.Fatalf("row %d decade_dob", n)
		}
		if want := boolInt(f(6) == 1); sl(2) != want {
			t.Fatalf("row %d same_surname", n)
		}
		sl(3)
		if want := boolInt(f(20) == 0); sl(4) != want {
			t.Fatalf("row %d cross_script", n)
		}
	}
	if !finiteEID {
		t.Fatal("all training EID features are NaN")
	}
}

func TestModelPolicyArgsForwardedToChildren(t *testing.T) {
	args := modelPolicyArgs("pairfeat1", "v7strict")
	got := map[string]string{}
	for i := 0; i+1 < len(args); i += 2 {
		got[args[i]] = args[i+1]
	}
	if got["-modelset"] != "pairfeat1" || got["-policy"] != "v7strict" || len(args) != 4 {
		t.Fatalf("child args %v", args)
	}
	if err := validateModelPolicy(got["-modelset"], got["-policy"]); err != nil {
		t.Fatalf("child would reject its own args: %v", err)
	}
}

func TestModelPolicyArgsDefaultsAddNothing(t *testing.T) {
	if a := modelPolicyArgs("", ""); len(a) != 0 {
		t.Fatalf("default parent produced child args %v", a)
	}
	if a := modelPolicyArgs("real_v2", ""); len(a) != 2 || a[0] != "-modelset" {
		t.Fatalf("modelset-only args %v", a)
	}
	for _, a := range modelPolicyArgs("", "") {
		if a == "-policy" {
			t.Fatal("-policy forwarded for empty policy")
		}
	}
}
