package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pairfeat6 training-dump protocol at 400 persons, with the CLI (UrduFix on); hashes include explicit dot-product rounding:
// wsi nameidf -split train -persons 400 -noise v1t -idfseeds 1019; featdump -split train -persons 400 -noise v1t
// -seed 1019 -namesakes 0.1 -familynamesakes 0.1 -xsnamesakes 0.1 -nameidf <that table>; featdump -split cal
// -persons 400 -noise v1t -seed 2011 -nameidf <that table>.
const (
	goldenPF6IDF1019     = "c6491f0ae65573e417cf9539684afcdfe74ad497643b32637ec2484fca987512"
	goldenPF6Train1019   = "2c251234dfde7fb42ce016904bcc0cb5a6e2c20f722a69a382ee19334d776b58"
	goldenPF6Train1019Sc = "d08510f57fd43b124a8d84b0bd089a5600817665b81188a26455edd0398bd79b"
	goldenPF6Cal2011     = "8d03727f6d17e51bf6cf19af2bd3155926934257df8ecd7f6c126e065914069d"
)

// cliState: the CLI's UrduFix and noise v1t, with only the base providers (earlier tests may leave the lung/DR/
// breast/LDCT/IFHAS programme providers registered; the CLI goldens, with no programme data in ../data, have none).
func cliState(t *testing.T) {
	t.Helper()
	oldU, oldP, oldN := UrduFix, Providers, NoiseModel
	t.Cleanup(func() { UrduFix, Providers, NoiseModel = oldU, oldP, oldN })
	UrduFix, NoiseModel = true, "v1t"
	prog := map[string]bool{}
	for _, l := range [][]ProvCfg{lungProviders, drProviders, breastProviders, ldctProviders, ifhasProviders} {
		for _, p := range l {
			prog[p.ID] = true
		}
	}
	Providers = nil
	for _, p := range oldP {
		if !prog[p.ID] {
			Providers = append(Providers, p)
		}
	}
}

// pf7Table: wsi nameidf -split train -persons 400 -idfseeds 1019, in process.
func pf7Table(t *testing.T) (*NameIDFTable, []byte) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "name_idf_pairfeat7.json")
	runNameIDF("../data", "train", p, 400, []int64{1019})
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return LoadNameIDF(p), b
}

// pf6Dump: an in-process featdump of 400 persons with name-rarity table tb: on the train split with given-name and
// family namesakes 0.1, on any other split without them, and cross-script namesakes at rate xs.
func pf6Dump(t *testing.T, split string, seed int64, xs float64, tb *NameIDFTable) (csvB, scB []byte) {
	t.Helper()
	oldR, oldF, oldX, oldI := NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF
	defer func() { NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = oldR, oldF, oldX, oldI }()
	ns := 0.0
	if split == "train" {
		ns = 0.1
	}
	NamesakeRate, FamilyNamesakeRate, XSNamesakeRate, NameIDF = ns, ns, xs, tb
	out := filepath.Join(t.TempDir(), "o.csv")
	runFeatDump("../data", "../models/encoder_weights.json", "", split, out, 400, seed)
	var err error
	if csvB, err = os.ReadFile(out); err != nil {
		t.Fatal(err)
	}
	if scB, err = os.ReadFile(out + ".namesakes.json"); err != nil {
		t.Fatal(err)
	}
	return
}

func TestPairfeat6DumpsUnchangedGolden(t *testing.T) {
	cliState(t)
	tb, raw := pf7Table(t)
	if hex256(raw) != goldenPF6IDF1019 {
		t.Errorf("name-rarity table %s", hex256(raw))
	}
	c, s := pf6Dump(t, "train", 1019, 0.1, tb)
	if hex256(c) != goldenForArch(t, goldenPF6Train1019, goldenPF6Train1019AMD64) || hex256(s) != goldenPF6Train1019Sc {
		t.Errorf("pairfeat6 train dump: csv %s sidecar %s", hex256(c), hex256(s))
	}
	c, _ = pf6Dump(t, "cal", 2011, 0, tb)
	if hex256(c) != goldenForArch(t, goldenPF6Cal2011, goldenPF6Cal2011AMD64) {
		t.Errorf("pairfeat6 cal dump: csv %s", hex256(c))
	}
}

func TestPairfeat7ModelsetTables(t *testing.T) {
	for ms, want := range map[string]bool{"": false, "pairfeat1": false, "pairfeat5": false, "pairfeat6": true, "pairfeat7": true, "pairfeat8": true} {
		if usesNameIDF(ms) != want {
			t.Errorf("usesNameIDF(%q) = %v", ms, !want)
		}
	}
	for ms, want := range map[string]bool{"": false, "pairfeat4": false, "pairfeat5": true, "pairfeat6": true, "pairfeat7": true, "pairfeat8": true} {
		if xscriptCapFor(ms) != want {
			t.Errorf("xscriptCapFor(%q) = %v", ms, !want)
		}
	}
	for _, p := range []string{"", "v6", "v7ltt"} {
		err := validateModelPolicy("pairfeat7", p)
		if err == nil || !strings.Contains(err.Error(), "modelset pairfeat7 supports only -policy v7strict") {
			t.Errorf("validateModelPolicy(pairfeat7, %q) = %v", p, err)
		}
		if err := validateModelPolicy("pairfeat6", p); err == nil ||
			err.Error() != `modelset pairfeat6 supports only -policy v7strict (got "`+p+`"); it keeps pairfeat5's cross-script review cap, selected under v7strict` {
			t.Errorf("pairfeat6 policy message changed: %v", err)
		}
	}
	if err := validateModelPolicy("pairfeat7", "v7strict"); err != nil {
		t.Error(err)
	}
	d := t.TempDir()
	for ms, n := range map[string]int{"pairfeat6": 6, "pairfeat7": 7} {
		body := fmt.Sprintf(`{"version":1,"registrations":%d,"counts":{"a":1}}`, n)
		if err := os.WriteFile(filepath.Join(d, "name_idf_"+ms+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for ms, n := range map[string]int{"pairfeat6": 6, "pairfeat7": 7} {
		for _, cmd := range []string{"orchestrate", "audit", "report7"} {
			tb, err := nameIDFFor(cmd, "", ms, d)
			if err != nil || tb == nil || tb.Registrations != n {
				t.Errorf("nameIDFFor(%s, %s) = %+v, %v; want its own table (N=%d)", cmd, ms, tb, err, n)
			}
		}
	}
	if tb, err := nameIDFFor("featdump", "", "pairfeat7", d); tb != nil || err != nil {
		t.Errorf("featdump without -nameidf loaded %+v, %v", tb, err)
	}
	if _, err := nameIDFFor("orchestrate", "", "pairfeat7", t.TempDir()); err == nil {
		t.Error("pairfeat7 without a table in -wasl accepted")
	}
}
