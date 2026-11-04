package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoisyDob3OnlyBlankOrOriginal(t *testing.T) {
	const dob = "1980-02-22"
	for seed := int64(0); seed < 2000; seed++ {
		got := noisyDob3(dob, rand.New(rand.NewSource(seed)), .3)
		if got != dob && got != "" {
			t.Fatalf("seed %d: got %q", seed, got)
		}
	}
}

func TestNoisyDob3BlankRate(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	blank := 0
	for i := 0; i < 20000; i++ {
		if noisyDob3("1980-02-22", r, .3) == "" {
			blank++
		}
	}
	if got := float64(blank) / 20000; got < .04 || got > .05 {
		t.Fatalf("blank rate %.4f outside [0.04, 0.05]", got)
	}
}

func setNoise(t *testing.T, n string) {
	t.Helper()
	saved := NoiseModel
	NoiseModel = n
	t.Cleanup(func() { NoiseModel = saved })
}

func fdDobs(t *testing.T, model, dob string) (distinctReal map[string]bool, blanks int) {
	setNoise(t, model)
	p := &Person{Qid: "Q1", Ar: "محمد احمد", En: "Mohammed Ahmed", Als: []string{"Muhammad Ahmad"}, Dob: dob, Sex: "M", Eid: "784198012345678", Idx: 1}
	distinctReal = map[string]bool{}
	for seed := int64(0); seed < 200; seed++ {
		r := rand.New(rand.NewSource(seed))
		for _, g := range fdRegister(nil, 1, p, r) {
			if g.dob == "" {
				blanks++
			} else {
				distinctReal[g.dob] = true
			}
		}
	}
	return
}

func TestNoise3FDRegisterInvariant(t *testing.T) {
	const dob = "1980-02-22"
	real, blanks := fdDobs(t, "v3", dob)
	for d := range real {
		if d != dob {
			t.Fatalf("v3 produced real DOB %q != %q", d, dob)
		}
	}
	if blanks == 0 || !real[dob] {
		t.Fatalf("v3 should cover both branches: blanks=%d real=%v", blanks, real)
	}
	real2, _ := fdDobs(t, "v2", dob)
	if len(real2) < 2 {
		t.Fatalf("v2 control produced no distinct real DOB: %v", real2)
	}
}

func TestNoise3GeneratedCohortInvariant(t *testing.T) {
	if _, err := os.Stat("../data/persons3.json"); err != nil {
		t.Skip("persons3.json not available")
	}
	setNoise(t, "v3")
	dir := genAdv(t, 0, 0)
	tr, _ := readTruth(t, dir)
	fs, _ := filepath.Glob(filepath.Join(dir, "fhir_*.ndjson"))
	if len(fs) == 0 {
		t.Fatal("no feeds")
	}
	real := map[string]string{}
	blanks, nonblank := 0, 0
	for _, f := range fs {
		b, _ := os.ReadFile(f)
		for _, ln := range strings.Split(string(b), "\n") {
			if ln == "" {
				continue
			}
			var ev struct {
				Res struct {
					Contained []struct {
						ResourceType string                           `json:"resourceType"`
						BirthDate    string                           `json:"birthDate"`
						Identifier   []struct{ System, Value string } `json:"identifier"`
					} `json:"contained"`
				} `json:"res"`
			}
			if json.Unmarshal([]byte(ln), &ev) != nil {
				continue
			}
			for _, c := range ev.Res.Contained {
				if c.ResourceType != "Patient" {
					continue
				}
				for _, id := range c.Identifier {
					if !strings.HasSuffix(id.System, ":mrn") {
						continue
					}
					prov := strings.TrimSuffix(strings.TrimPrefix(id.System, "urn:ae:"), ":mrn")
					q := tr.PKQid[prov+"|"+id.Value]
					if q == "" {
						t.Fatalf("no QID for %s|%s", prov, id.Value)
					}
					if c.BirthDate == "" {
						blanks++
						continue
					}
					nonblank++
					if old, ok := real[q]; ok && old != c.BirthDate {
						t.Fatalf("QID %s has DOBs %s and %s", q, old, c.BirthDate)
					}
					real[q] = c.BirthDate
				}
			}
		}
	}
	if blanks == 0 || nonblank == 0 {
		t.Fatalf("both branches needed: blanks=%d nonblank=%d", blanks, nonblank)
	}
}

// Golden digest of v1/v1t/v2 DOB outputs and stream positions for fixed seeds; recorded before v3 was added.
func TestNoiseOldProfilesGolden(t *testing.T) {
	dobs := []string{"1960-03-14", "1985-12-01", "1972-05-05", "1999-07-30", "1948-11-11"}
	fns := map[string]func(string, *rand.Rand, float64) string{"v1": noisyDob, "v1t": noisyDobV1T, "v2": noisyDob2}
	want := map[string]string{"v1": "bfc6e14765e10bbd", "v1t": "9daf2000d7bcf1ad", "v2": "da0b3e96b95e5f2e"}
	for name, fn := range fns {
		h := sha256.New()
		for seed := int64(0); seed < 50; seed++ {
			r := rand.New(rand.NewSource(seed))
			for i := 0; i < 40; i++ {
				fmt.Fprintf(h, "%s|", fn(dobs[i%len(dobs)], r, .3))
			}
			fmt.Fprintf(h, "%d;", r.Int63())
		}
		if got := fmt.Sprintf("%x", h.Sum(nil))[:16]; got != want[name] {
			t.Errorf("%s golden %s, want %s", name, got, want[name])
		}
	}
}

// Golden digest of fdRegister output (names, DOBs, EIDs, stream position) per old profile.
func TestNoiseOldProfilesFDRegisterGolden(t *testing.T) {
	want := map[string]string{"v1": "27bd40d97614ec47", "v1t": "3771fa2c1be6a0b2", "v2": "f1f170e6198ec158"}
	for _, m := range []string{"v1", "v1t", "v2"} {
		setNoise(t, m)
		p := &Person{Qid: "Q1", Ar: "محمد احمد", En: "Mohammed Ahmed", Als: []string{"Muhammad Ahmad"}, Dob: "1980-02-22", Sex: "M", Eid: "784198012345678", Idx: 1}
		h := sha256.New()
		for seed := int64(0); seed < 100; seed++ {
			r := rand.New(rand.NewSource(seed))
			fmt.Fprintf(h, "%v|%d;", fdRegister(nil, 1, p, r), r.Int63())
		}
		got := fmt.Sprintf("%x", h.Sum(nil))[:16]
		if want[m] == "" {
			t.Logf("RECORD %s %s", m, got)
			continue
		}
		if got != want[m] {
			t.Errorf("%s fdRegister golden %s, want %s", m, got, want[m])
		}
	}
}
