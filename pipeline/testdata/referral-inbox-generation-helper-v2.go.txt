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

var writeReferralControls = flag.Bool("write-referral-controls-v2", false, "create version2 explicitly synthetic referral inbox controls; historical version1 is immutable")

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

// Version1 inputs and their historical generation remain unchanged above. The
// current reducer requires a completed Communication revision before an
// acknowledgement can count; version2 supplies that explicit synthetic status.
func referralControlInputsV2(t *testing.T) []FactsMsg {
	inputs := referralControlInputs(t)
	for i := range inputs {
		for j := range inputs[i].Facts.Comms {
			if inputs[i].Facts.Comms[j].Category == "acknowledgement" {
				inputs[i].Facts.Comms[j].Status = "completed"
			}
		}
	}
	return inputs
}

func TestReferralInboxAcknowledgementRequiresCompletedStatus(t *testing.T) {
	oldHandoff, oldOutreach := Handoff, OutreachOn
	Handoff, OutreachOn = false, false
	t.Cleanup(func() { Handoff, OutreachOn = oldHandoff, oldOutreach })
	for _, status := range []string{"", "in-progress", "completed"} {
		t.Run("status="+status, func(t *testing.T) {
			input := referralControlInputsV2(t)[0]
			input.Facts.Comms[1].Status = status
			core := NewClosureCore(0, nil)
			core.Apply(&FactsMsg{ID: "synthetic-clock", InSeq: 1, IsTick: true, TickDay: 120})
			var current *Decision
			for _, out := range core.Apply(&input) {
				if out.Dec != nil {
					current = out.Dec
				}
			}
			want := "no_acknowledgement"
			if status == "completed" {
				want = "referral_unanswered"
			}
			if current == nil || current.Sig == "" || current.Sig != decSig(current) || current.Dropoff != want {
				t.Fatalf("acknowledgement status %q did not preserve the canonical guard: want %s, got %+v", status, want, current)
			}
		})
	}
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
	inputs := referralControlInputsV2(t)
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
		"accepted_main":             "ac815cb868f4fbbcea0b6ed2233c1296306d9b38",
		"contract_version":          2,
		"supersedes_record_sha256":  "9b7616a9a44e8cd65f9f17325255b301cc9a8d1f37e46d5ea2b4c7fb9d396200",
		"generation_source_binding": "production_source_sha256 and helper_sha256 identify the actual integrated working source; generation HEAD is ancestry only",
		"production_source_sha256":  referralSourceHashes(t), "helper_sha256": sha(helper),
		"inputs": inputs, "input_sha256": sha(canon(inputs)), "out_log_sha256": sha(raw.Bytes()),
		"cases_sha256": sha(canon(exposed)), "currents": details, "held_control": held,
	}
	if *writeReferralControls {
		witnessPath := filepath.Join("testdata", "referral-inbox-generation-helper-v2.go.txt")
		witness, err := os.OpenFile(witnessPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := witness.Write(helper)
		closeErr := witness.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("record generation helper: write=%v close=%v", writeErr, closeErr)
		}
	}
	for name, value := range map[string]interface{}{"referral-inbox-synthetic-v2.json": exposed, "referral-inbox-provenance-v2.json": provenance} {
		path := filepath.Join("..", "web", "data", name)
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if *writeReferralControls {
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.Write(encoded)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("create version2 software control: write=%v close=%v", writeErr, closeErr)
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
			if name == "referral-inbox-provenance-v2.json" {
				generationHelper, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper-v2.go.txt"))
				if err != nil {
					t.Fatal(err)
				}
				got, recomputed := actual.(map[string]interface{}), want.(map[string]interface{})
				if got["helper_sha256"] != sha(generationHelper) {
					t.Fatal("version2 generation helper differs from its recorded witness")
				}
				t.Logf("current verification source manifest: %s; separate version1 generation record preserved", sha(canon(recomputed["production_source_sha256"])))
				// Only generation-description fields vary across source revisions.
				// Inputs, signed OUT hash, cases, currents, held control and the
				// version/superseded-record binding still compare exactly below.
				for _, field := range []string{"source_head_at_generation", "production_source_sha256", "helper_sha256"} {
					delete(got, field)
					delete(recomputed, field)
				}
			}
			if !bytes.Equal(canon(actual), canon(want)) {
				t.Fatalf("%s is stale; explicitly regenerate the synthetic controls", name)
			}
		}
	}
	t.Logf("canonical synthetic controls: inputs=%s out_log=%s cases=%s; genuine signatures verified; held stays inadequate", sha(canon(inputs)), sha(raw.Bytes()), sha(canon(exposed)))
}

func referralGenerationRecordIntact(record, witness []byte) bool {
	return sha(record) == "9b7616a9a44e8cd65f9f17325255b301cc9a8d1f37e46d5ea2b4c7fb9d396200" &&
		sha(witness) == "a35eb7de2c08d64683e7e84520795cc3ad0a913826a4d5d073df2fd5bacbce10"
}

func TestReferralGenerationRecordIntegrity(t *testing.T) {
	record, err := os.ReadFile(filepath.Join("..", "web", "data", "referral-inbox-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	witness, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !referralGenerationRecordIntact(record, witness) {
		t.Fatal("approved generation record rejected")
	}
	for _, field := range []string{"production_source_sha256", "source_head_at_generation", "helper_sha256", "inputs", "input_sha256", "out_log_sha256", "cases_sha256", "currents", "held_control"} {
		t.Run(field, func(t *testing.T) {
			var changed map[string]interface{}
			if err := json.Unmarshal(record, &changed); err != nil {
				t.Fatal(err)
			}
			if _, exists := changed[field]; !exists {
				t.Fatalf("mutation target absent: %s", field)
			}
			changed[field] = "tampered historical generation value"
			if referralGenerationRecordIntact(canon(changed), witness) {
				t.Fatalf("generation field mutation accepted: %s", field)
			}
		})
	}
	t.Run("generation_witness", func(t *testing.T) {
		changed := append([]byte(nil), witness...)
		if len(changed) == 0 {
			t.Fatal("empty witness")
		}
		changed[0] ^= 1
		if referralGenerationRecordIntact(record, changed) {
			t.Fatal("altered historical generation helper accepted")
		}
	})
}
