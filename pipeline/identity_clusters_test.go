package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonIDUsesSortedRegistrationsNotRoot(t *testing.T) {
	a := personIDForRegistrations([]string{"p|2", "p|1"})
	b := personIDForRegistrations([]string{"p|1", "p|2"})
	if a != b || !personIDPattern.MatchString(a) {
		t.Fatalf("unstable or malformed person id: %q %q", a, b)
	}
	if a == personIDForRegistrations([]string{"p|1"}) {
		t.Fatal("different clusters received one person id")
	}
}

func TestWriteIdentityClustersCanonicalTrustedMembership(t *testing.T) {
	run := t.TempDir()
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), []byte("fhir\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte("out\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	L := NewLinkerCore(nil, 1)
	L.recs = []*Rec{{Pk: "b|2"}, {Pk: "a|1"}, {Pk: "z|9"}}
	L.parent = []int{1, 1, 2}
	L.loops = map[string]*LoopFacts{
		"b/ServiceRequest/2": {LoopID: "b/ServiceRequest/2", SRRec: "b|2"},
		"a/ServiceRequest/1": {LoopID: "a/ServiceRequest/1", SRRec: "a|1"},
	}
	rp := &replayOut{L: L}
	if err := writeIdentityClusters(run, rp); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(run, "identity_clusters.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	raw, _ := os.ReadFile(filepath.Join(run, "identity_clusters.json"))
	var doc identityClusterSnapshot
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != identityClusterSchema || len(doc.Clusters) != 2 || len(doc.Cases) != 2 {
		t.Fatalf("unexpected document: %+v", doc)
	}
	if got := strings.Join(doc.Clusters[0].Registrations, ","); got != "a|1,b|2" {
		t.Fatalf("registrations not sorted: %s", got)
	}
	if doc.Clusters[0].Root != "a|1" || doc.Cases[0].CaseID != "a/ServiceRequest/1" {
		t.Fatalf("clusters/cases not sorted: %+v", doc)
	}
	for _, forbidden := range []string{"name", "dob", "eid", "candidate", "score", "review"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("raw snapshot contains %q: %s", forbidden, raw)
		}
	}
}

func TestWriteIdentityClustersFailureDoesNotReplaceSnapshot(t *testing.T) {
	run := t.TempDir()
	path := filepath.Join(run, "identity_clusters.json")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), []byte("fhir"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), []byte("out"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeIdentityClusters(run, &replayOut{}); err == nil {
		t.Fatal("missing canonical linker did not fail materialisation")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "previous" {
		t.Fatalf("failed write replaced prior snapshot: %q", got)
	}
}
