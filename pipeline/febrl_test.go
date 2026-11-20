package main

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
)

// Inline FEBRL-shaped fixture (header and leading spaces as in the recordlinkage package files; values invented).
const febrlFixtureA = `rec_id, given_name, surname, street_number, address_1, address_2, suburb, postcode, state, date_of_birth, soc_sec_id
rec-1-org, michaela, neumann, 8, stanley street, miami, winston hills, 4223, nsw, 19151111, 5304218
rec-2-org, courtney, painter, 12, pinkerton circuit, bega flats, richlands, 4560, vic, 19161214, 4066625
rec-3-org, jayden, oconnell, 5, lyne place, , tarrawanna, 2617, nsw, 19800707, 1234567
`

const febrlFixtureB = `rec_id, given_name, surname, street_number, address_1, address_2, suburb, postcode, state, date_of_birth, soc_sec_id
rec-2-dup-0, courtney, paintr, 12, pinkerton circuit, bega flats, richlands, 4560, vic, 19161214, 4066625
rec-1-dup-0, micheala, neumann, 8, stanley st, miami, winston hills, 4223, nsw, 19151111, 5304218
rec-3-dup-0, jayden, oconnell, 5, lyne place, , tarrawanna, 2617, nsw, , 1234568
`

func febrlFixture(t *testing.T) ([]febrlRec, []febrlRec) {
	t.Helper()
	A, err := readFebrl(strings.NewReader(febrlFixtureA))
	if err != nil {
		t.Fatal(err)
	}
	B, err := readFebrl(strings.NewReader(febrlFixtureB))
	if err != nil {
		t.Fatal(err)
	}
	return A, B
}

func TestFebrlRead(t *testing.T) {
	A, B := febrlFixture(t)
	if len(A) != 3 || len(B) != 3 {
		t.Fatalf("rows %d %d", len(A), len(B))
	}
	if A[0] != (febrlRec{ID: "rec-1-org", Given: "michaela", Surname: "neumann", DOB: "19151111", SSID: "5304218"}) {
		t.Fatalf("A[0] = %+v", A[0])
	}
	if B[2].DOB != "" || febrlEntity(B[2].ID) != "rec-3" || febrlEntity(A[2].ID) != "rec-3" {
		t.Fatalf("B[2] = %+v", B[2])
	}
	if _, err := readFebrl(strings.NewReader("rec_id, given_name\nx, y\n")); err == nil {
		t.Fatal("missing columns must be an error")
	}
}

func TestFebrlFieldMapping(t *testing.T) {
	for in, want := range map[string]string{"19151111": "1915-11-11", "": "", "1915111": "", "1915x111": ""} {
		if got := febrlDOB(in); got != want {
			t.Fatalf("febrlDOB(%q) = %q, want %q", in, got, want)
		}
	}
	e := febrlEID("5304218")
	if len(e) != 15 || !strings.HasPrefix(e, "7840000") || !luhnOK(e) || dummyEID(e) {
		t.Fatalf("febrlEID = %q: must be a 15-digit 784 Luhn-valid non-dummy number", e)
	}
	if s := febrlEID("42"); len(s) != 15 || !strings.HasPrefix(s, "78400000000042") || !luhnOK(s) {
		t.Fatalf("short ssid must be zero-padded and Luhn-valid: %q", s)
	}
	if febrlEID("") != "" || febrlEID("12345678") != "" || febrlEID("12a") != "" {
		t.Fatal("unusable soc_sec_id must stay missing")
	}
	A, _ := febrlFixture(t)
	ev := febrlEvent(A[0], "a", "names_dob")
	if ev.Name != "michaela neumann" || ev.Dob != "1915-11-11" || ev.Eid != "" || ev.Sex != "" || ev.PatientKey != "a|rec-1-org" {
		t.Fatalf("names_dob event = %+v", ev)
	}
	if ev := febrlEvent(A[0], "a", "names_dob_ssid"); ev.Eid != febrlEID("5304218") {
		t.Fatalf("names_dob_ssid must carry the wrapped id, got %q", ev.Eid)
	}
	if ev := febrlEvent(febrlRec{ID: "x", Surname: "smith"}, "b", "names_dob"); ev.Name != "smith" {
		t.Fatalf("missing given name: %q", ev.Name)
	}
}

func TestFebrlWilson(t *testing.T) {
	r := wilson95(0, 10)
	if r.Value != 0 || r.Low != 0 || math.Abs(r.High-0.277533) > 1e-5 {
		t.Fatalf("wilson95(0,10) = %+v", r)
	}
	r = wilson95(5, 10)
	if math.Abs(r.Low-0.236590) > 1e-5 || math.Abs(r.High-0.763410) > 1e-5 {
		t.Fatalf("wilson95(5,10) = %+v", r)
	}
}

func TestFebrlScore(t *testing.T) {
	A, B := febrlFixture(t)
	// B[0] is rec-2 (A[1]); B[1] is rec-1 (A[0]); B[2] is rec-3 (A[2])
	m := febrlScore(map[[2]int]bool{{1, 0}: true, {0, 1}: true, {0, 2}: true}, A, B)
	if m.TP != 2 || m.FP != 1 || m.FN != 1 || m.Predicted != 3 {
		t.Fatalf("counts %+v", m)
	}
	if m.Precision.K != 2 || m.Precision.N != 3 || m.Recall.K != 2 || m.Recall.N != 3 {
		t.Fatalf("rates %+v %+v", m.Precision, m.Recall)
	}
	if m.FalseMatchRate.K != 1 || m.MissedMatchRate.K != 1 || math.Abs(m.F1-2.0/3) > 1e-5 {
		t.Fatalf("fmr %+v mmr %+v f1 %v", m.FalseMatchRate, m.MissedMatchRate, m.F1)
	}
}

// With the ID configuration and no scorer, only the runtime Emirates-ID rule links: equal wrapped IDs link, and the
// corrupted soc_sec_id of rec-3 becomes a different valid number (a cannot-link), so rec-3 is missed.
func TestFebrlCoreIDRule(t *testing.T) {
	A, B := febrlFixture(t)
	stub := func(s string) []float32 { // deterministic unit vector; content irrelevant without a scorer
		v := make([]float32, 4)
		for i, c := range s {
			v[i%4] += float32(c)
		}
		n := float32(math.Sqrt(float64(v[0]*v[0] + v[1]*v[1] + v[2]*v[2] + v[3]*v[3])))
		for i := range v {
			v[i] /= n
		}
		return v
	}
	res, err := runFebrlCore(A, B, "names_dob_ssid", stub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Auto.TP != 2 || res.Auto.FP != 0 || res.Auto.FN != 1 || res.Stats["eid_links"] != 2 {
		t.Fatalf("auto %+v stats %v", res.Auto, res.Stats)
	}
	if res.MissingDOB != [2]int{0, 1} || res.MissingEID != [2]int{0, 0} {
		t.Fatalf("missing dob %v id %v", res.MissingDOB, res.MissingEID)
	}
	res, err = runFebrlCore(A, B, "names_dob", stub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Auto.Predicted != 0 || res.MissingEID != [2]int{3, 3} {
		t.Fatalf("names_dob without a scorer must link nothing: %+v", res.Auto)
	}
	if _, err := runFebrlCore(A, B, "address", stub, nil); err == nil {
		t.Fatal("unknown config must be an error")
	}
}

// End to end with the shipped encoder and scorer on the fixture: the typo pairs link at the runtime threshold.
func TestFebrlCoreShippedModels(t *testing.T) {
	enc, gbm := filepath.Join("..", "models", "encoder_weights.json"), filepath.Join("..", "models", "lgb_stream.json")
	old := UrduFix
	UrduFix = true
	defer func() { UrduFix = old }()
	A, B := febrlFixture(t)
	res, err := runFebrlCore(A, B, "names_dob", LoadEncoder(enc).Embed, LoadGBM(gbm))
	if err != nil {
		t.Fatal(err)
	}
	if res.Auto.FP != 0 || res.Auto.TP < 2 || res.BlockingRecall.K != 3 {
		t.Fatalf("auto %+v blocking %+v", res.Auto, res.BlockingRecall)
	}
}
