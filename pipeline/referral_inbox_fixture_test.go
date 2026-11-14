package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var writeReferralControls = flag.Bool("write-referral-controls", false, "regenerate explicitly synthetic referral inbox controls")

// This fixture begins at the canonical facts seam, NOT the clinical ingest/linker/reader.
// Its report verdict is an explicitly synthetic input; neither a real reader result nor
// clinical validation is claimed. All decisions, signatures, ledger history and API
// projections below are produced by unchanged canonical code, not handwritten DTOs.
func referralControlInputs(t *testing.T) []FactsMsg {
	t.Helper()
	text, why := Render("fitfu-notify-v1/en", "ahs-mushrif")
	if why != "" {
		t.Fatal(why)
	}
	var inputs []FactsMsg
	for i, day := range []float64{118.25, 117.5, 116.75, 110.5, 110.5} {
		id := "ahs-mushrif/ServiceRequest/AHS-sr-" + itoa(9001+i)
		facts := &LoopFacts{LoopID: id, Provider: "ahs-mushrif", SRDay: day, SRLang: "en", Prog: "crc", Contact: "granted",
			Tasks: []TaskFact{{Owner: "Practitioner/ahs-mushrif-nurse-1"}},
			Comms: []CommFact{{ID: id + "-notify", Day: day + .05, Status: "completed", Lang: "en", Template: "fitfu-notify-v1/en", Text: text},
				{ID: id + "-ack", Day: day + .1, Category: "acknowledgement"}}}
		if i == 3 {
			// Existing missing-packet control: a genuine rejected intake with Handoff off.
			facts.Down = []DownFact{{EventID: id + "-reject", Provider: "skmc", RType: "ServiceRequest", Status: "revoked", Day: day + .5, Strength: 1, Weak: Edge{Basis: "eid"}}}
		}
		if i == 4 {
			// Contrast: a held incomplete report, never an eligible unanswered referral.
			facts.Down = []DownFact{{EventID: id + "-report", Provider: "skmc", RType: "DiagnosticReport", Status: "final", ReportID: "synthetic-incomplete-prep", Day: 119, Strength: 1, Weak: Edge{Basis: "eid"},
				Ev: &TwoVerdict{Kind: "colonoscopy", Agree: true, Label: "inadequate", Regex: ReaderVerdict{Caecum: "not_stated", Prep: "not_stated"}, Gemma: ReaderVerdict{Caecum: "not_stated", Prep: "not_stated"}}}}
		}
		inputs = append(inputs, FactsMsg{ID: "synthetic-referral-" + itoa(i), InSeq: uint64(i + 2), Facts: facts})
	}
	return inputs
}

func referralSourceHashes(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "go.mod", "go.sum")
	hashes := map[string]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		hashes[file] = sha(b)
	}
	return hashes
}

func TestReferralInboxCanonicalControls(t *testing.T) {
	if !*writeReferralControls {
		// Verification must also work in a source export with no Git metadata.
		// Keep this regression guard local to this non-parallel test; generation
		// alone may inspect HEAD to record where new controls were produced.
		t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "unavailable.git"))
	}
	oldHandoff, oldOutreach := Handoff, OutreachOn
	Handoff, OutreachOn = false, false
	defer func() { Handoff, OutreachOn = oldHandoff, oldOutreach }()
	inputs := referralControlInputs(t)
	c := NewClosureCore(0, nil)
	c.Apply(&FactsMsg{ID: "synthetic-clock", InSeq: 1, IsTick: true, TickDay: 120})
	var logs []LogMsg
	currents := map[string]*Decision{}
	prev := ""
	for _, input := range inputs {
		for _, out := range c.Apply(&input) {
			if out.Dec == nil {
				t.Fatalf("unexpected review output: %+v", out)
			}
			if out.Dec.Sig == "" || out.Dec.Sig != decSig(out.Dec) {
				t.Fatal("noncanonical signature")
			}
			if out.Prev != prev || out.Hash != ledgerHash(prev, out.OSeq, out.ID, out.Kind, out.body()) {
				t.Fatal("noncanonical output ledger")
			}
			prev = out.Hash
			currents[out.Dec.LoopID] = out.Dec
			env := OutEnvelope{Kind: "dec", OSeq: out.OSeq, Dec: out.Dec, Prev: out.Prev, Hash: out.Hash, Proc: "closure-0"}
			logs = append(logs, LogMsg{Seq: out.OSeq, Subject: "out.p0", Data: canon(env)})
		}
	}
	run := t.TempDir()
	var raw bytes.Buffer
	for _, log := range logs {
		raw.Write(canon(log))
		raw.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(run, "out_log.ndjson"), raw.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	// No FHIR ingest is claimed: these controls start at the facts seam.
	if err := os.WriteFile(filepath.Join(run, "fhir_log.ndjson"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	// Actual materializer, then actual authenticated read handler; no server/broker/socket.
	s := loadAPI(run, "")
	handler, err := s.handler("referral-synthetic-test-token:reviewer:synthetic-referral-reviewer", "")
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) map[string]interface{} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer referral-synthetic-test-token")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, response.Code, response.Body.String())
		}
		var body map[string]interface{}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	list := get("/api/cases?programme=crc&provider=ahs-mushrif&limit=50")
	var exposed []interface{}
	details := map[string]interface{}{}
	var held interface{}
	for _, input := range inputs {
		id := input.Facts.LoopID
		detail := get("/api/cases/" + id)
		current := detail["current"].(map[string]interface{})
		if current["sig"] != currents[id].Sig || current["sig"] == "" {
			t.Fatal("served current/signature differs from producer")
		}
		details[id] = current
		var summary map[string]interface{}
		for _, row := range list["cases"].([]interface{}) {
			m := row.(map[string]interface{})
			if m["id"] == id {
				summary = m
			}
		}
		if summary == nil || summary["closed"] != false || summary["consent"] != "granted" || summary["owner"] == "" || summary["review"] != "none" {
			t.Fatalf("unusable synthetic control: %v", summary)
		}
		switch {
		case strings.HasSuffix(id, "9005"):
			if summary["state"] != "inadequate" || summary["dropoff"] != "inadequate" {
				t.Fatalf("held control was not held: %v", summary)
			}
			held = summary
		case strings.HasSuffix(id, "9004"):
			if summary["dropoff"] != "referral_rejected" || current["handoff"] != nil {
				t.Fatal("missing-packet control changed")
			}
			exposed = append(exposed, summary)
		default:
			if summary["state"] != "open" || summary["dropoff"] != "referral_unanswered" {
				t.Fatalf("producer cannot establish eligible unanswered referral: %v", summary)
			}
			exposed = append(exposed, summary)
		}
	}
	helper, err := os.ReadFile("referral_inbox_fixture_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var head []byte
	if *writeReferralControls {
		head, err = exec.Command("git", "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatal(err)
		}
	}
	provenance := map[string]interface{}{
		"kind":                      "explicitly synthetic facts-seam producer controls; not medical evidence",
		"producer":                  "ClosureCore.Apply -> signed OutEnvelope/out_log -> loadAPI -> authenticated GET handler (httptest recorder, no socket)",
		"source_head_at_generation": strings.TrimSpace(string(head)),
		"accepted_main":             "e3129b7efc85adf85ef10cfb212031a4908dc8c2",
		"production_source_sha256":  referralSourceHashes(t), "helper_sha256": sha(helper),
		"inputs": inputs, "input_sha256": sha(canon(inputs)), "out_log_sha256": sha(raw.Bytes()),
		"cases_sha256": sha(canon(exposed)), "currents": details, "held_control": held,
	}
	for name, value := range map[string]interface{}{"referral-inbox-synthetic.json": exposed, "referral-inbox-provenance.json": provenance} {
		path := filepath.Join("..", "web", "data", name)
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if *writeReferralControls {
			if err := os.WriteFile(path, encoded, 0644); err != nil {
				t.Fatal(err)
			}
		} else {
			saved, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var actual, want interface{}
			if err := json.Unmarshal(saved, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &want); err != nil {
				t.Fatal(err)
			}
			if name == "referral-inbox-provenance.json" {
				// Preserve the approved generation record, without regenerating fixtures.
				// Accepted PR183 changed only api.go's Closed query predicate. Pin
				// that exact new source, while retaining its historical generation hash.
				// Every other production hash and all recomputed outputs still compare.
				got, recomputed := actual.(map[string]interface{}), want.(map[string]interface{})
				sourceHashes := recomputed["production_source_sha256"].(map[string]interface{})
				const generationAPI = "9314f93a56a1c8ba0004060e8229eaefba816734b22ac67e9fb148eee2e363c4"
				const acceptedAPI = "77abf1430a4e19631d323bf5e5650e53e7635d8537d351249cf68938f2ac818c"
				if sourceHashes["api.go"] != acceptedAPI || got["production_source_sha256"].(map[string]interface{})["api.go"] != generationAPI {
					t.Fatal("unexpected API source change since approved referral generation")
				}
				sourceHashes["api.go"] = generationAPI
				// This non-executable witness is the exact whole generation helper
				// from 44f8911026dac7bbe18381144ea2596d512edab7, not a function
				// extract or this current verification helper. Check its original
				// recorded digest without needing historical Git objects at runtime.
				generationHelper, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper.go.txt"))
				if err != nil {
					t.Fatal(err)
				}
				const generationHelperSHA = "a35eb7de2c08d64683e7e84520795cc3ad0a913826a4d5d073df2fd5bacbce10"
				if sha(generationHelper) != generationHelperSHA || got["helper_sha256"] != generationHelperSHA {
					t.Fatal("approved referral generation helper identity changed")
				}
				recomputed["helper_sha256"] = sha(generationHelper)
				// HEAD advances after preparation; original generation metadata stays saved.
				delete(got, "source_head_at_generation")
				delete(recomputed, "source_head_at_generation")
			}
			if !bytes.Equal(canon(actual), canon(want)) {
				t.Fatalf("%s is stale; explicitly regenerate the synthetic controls", name)
			}
		}
	}
	t.Logf("canonical synthetic controls: inputs=%s out_log=%s cases=%s; genuine signatures verified; held stays inadequate", sha(canon(inputs)), sha(raw.Bytes()), sha(canon(exposed)))
}
