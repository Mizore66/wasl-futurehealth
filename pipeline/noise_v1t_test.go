package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// noisyDobV1Ref: test-local copy of the pre-change noisyDob body (v1 corruption), for byte-identity.
func noisyDobV1Ref(dob string, r *rand.Rand, p float64) string {
	if r.Float64() > p {
		return dob
	}
	y, m, d := dob[:4], dob[5:7], dob[8:10]
	x := r.Float64()
	switch {
	case x < .25:
		return ""
	case x < .5:
		return y + "-07-01"
	case x < .75 && atoi(d) <= 12 && d != m:
		return y + "-" + d + "-" + m
	}
	dy := 1
	if r.Intn(2) == 0 {
		dy = -1
	}
	return fmt.Sprintf("%04d-%s-%s", atoi(y)+dy, m, d)
}

func TestNoisyDobUnchangedV1(t *testing.T) {
	dobs := []string{"1960-03-14", "1985-12-01", "1972-05-05", "1999-07-30", "1948-11-11"}
	r1, r2 := rand.New(rand.NewSource(77)), rand.New(rand.NewSource(77))
	for i := 0; i < 20000; i++ {
		d := dobs[i%len(dobs)]
		if a, b := noisyDob(d, r1, .3), noisyDobV1Ref(d, r2, .3); a != b {
			t.Fatalf("draw %d: %q vs %q", i, a, b)
		}
	}
	if r1.Int63() != r2.Int63() {
		t.Fatal("random streams diverged")
	}
}

func TestNoisyDobV1TDistribution(t *testing.T) {
	const n = 200000
	dob := "1968-04-23"
	r := rand.New(rand.NewSource(5))
	unc := 0
	idx := []int{0, 1, 2, 3, 5, 6, 8, 9}
	for i := 0; i < n; i++ {
		if noisyDobV1T(dob, r, .3) == dob {
			unc++
		}
	}
	if s := float64(unc) / n; s < .69 || s > .71 {
		t.Fatalf("uncorrupted share %.4f", s)
	}
	// Mirror the draw sequence to measure branch shares (keeps the test independent of v1's output collisions).
	// Real function, position 2 (decade): for a 1969 DOB v1's year shift can never produce a single-digit change
	// there (1970 changes two digits, 1968 changes position 3), so single-position-2 diffs are pure typos: .3*.2/8 of draws.
	dec := 0
	rr := rand.New(rand.NewSource(11))
	for i := 0; i < n; i++ {
		o := noisyDobV1T("1969-04-23", rr, .3)
		if len(o) == 10 && o[:2] == "19" && o[2] != '6' && o[3:] == "9-04-23" {
			dec++
		}
	}
	if s := float64(dec) / n; s < .0075*0.9 || s > .0075*1.1 {
		t.Fatalf("decade-typo share of draws %.5f want ~.0075", s)
	}
	r = rand.New(rand.NewSource(5))
	corrupted, typos := 0, 0
	pos := map[int]int{}
	for i := 0; i < n; i++ {
		if r.Float64() > .3 {
			continue
		}
		corrupted++
		if r.Float64() < .20 {
			typos++
			k := idx[r.Intn(8)]
			b := []byte(dob)
			old := b[k]
			b[k] = byte('0' + (int(b[k]-'0')+1+r.Intn(9))%10)
			if b[k] == old {
				t.Fatal("digit unchanged")
			}
			pos[k]++
		} else {
			corruptDob1(dob, r)
		}
	}
	if s := float64(typos) / float64(corrupted); s < .19 || s > .21 {
		t.Fatalf("typo share %.4f", s)
	}
	for _, k := range idx {
		if s := float64(pos[k]) / float64(typos); s < .105 || s > .145 {
			t.Fatalf("position %d share %.4f", k, s)
		}
	}
}

// Every typo output of the real function differs in exactly one digit position, dashes intact.
func TestNoisyDobV1TTypoShape(t *testing.T) {
	dob := "1968-04-23"
	r := rand.New(rand.NewSource(9))
	typo, dec := 0, 0
	for i := 0; i < 400000; i++ {
		out := noisyDobV1T(dob, r, 1)
		if len(out) != 10 || out[4] != '-' || out[7] != '-' {
			continue
		}
		diff, at := 0, -1
		for j := range dob {
			if dob[j] != out[j] {
				diff++
				at = j
			}
		}
		if diff != 1 || at == 4 || at == 7 {
			continue
		}
		typo++
		if at == 2 {
			dec++
		}
	}
	// typos (20%) plus v1 outputs that happen to differ in one digit (year +-1 without carry, 25% * ~) => lower bound only
	if typo < 80000 {
		t.Fatalf("typo-shaped outputs %d", typo)
	}
	if dec < 8000 {
		t.Fatalf("decade-position outputs %d", dec)
	}
}

func TestNoisyDobV1TDoesNotDisturbDeterminism(t *testing.T) {
	r1, r2 := rand.New(rand.NewSource(3)), rand.New(rand.NewSource(3))
	for i := 0; i < 1000; i++ {
		if noisyDobV1T("1970-01-02", r1, .3) != noisyDobV1T("1970-01-02", r2, .3) {
			t.Fatal("nondeterministic")
		}
	}
}

func TestPairFeat2PolicyGuard(t *testing.T) {
	for _, policy := range []string{"", "v6", "v7ltt"} {
		if err := validateModelPolicy("pairfeat2", policy); err == nil {
			t.Errorf("pairfeat2 accepted policy %q", policy)
		}
	}
	if err := validateModelPolicy("pairfeat2", "v7strict"); err != nil {
		t.Error(err)
	}
}

func TestValidateNoise(t *testing.T) {
	for _, cmd := range []string{"gen", "orchestrate", "encnoise", "bench", "run", ""} {
		if err := validateNoise(cmd, "v1t"); err == nil {
			t.Errorf("%q accepted v1t", cmd)
		}
		for _, n := range []string{"v1", "v2", ""} {
			if err := validateNoise(cmd, n); err != nil {
				t.Errorf("%q %q: %v", cmd, n, err)
			}
		}
	}
	for _, n := range []string{"v1", "v2", "v1t"} {
		if err := validateNoise("featdump", n); err != nil {
			t.Errorf("featdump %q: %v", n, err)
		}
	}
}

func TestFeatDumpV1TDecadeSameLabel(t *testing.T) {
	old := NoiseModel
	defer func() { NoiseModel = old }()
	NoiseModel = "v1t"
	dump := func() []byte {
		out := filepath.Join(t.TempDir(), "o.csv")
		runFeatDump("../data", "../models/encoder_weights.json", "", "train", out, 40, 1019)
		b, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	a, b := dump(), dump()
	if !bytes.Equal(a, b) {
		t.Fatal("v1t featdump not deterministic")
	}
	rows, err := csv.NewReader(bytes.NewReader(a)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	hit := 0
	for _, row := range rows {
		f24, err := strconv.ParseFloat(row[2+24], 64)
		if err != nil {
			t.Fatal(err)
		}
		if f24 == 1 && row[0] == "1" {
			hit++
		}
	}
	if hit == 0 {
		t.Fatalf("no same-person feature-24 rows in %d rows", len(rows))
	}
	t.Logf("same-person decade rows: %d of %d", hit, len(rows))
}
