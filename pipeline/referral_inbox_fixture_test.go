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

var writeReferralControls = flag.Bool("write-referral-controls-v3", false, "create version3 explicitly synthetic referral integration provenance; historical version1/version2 controls are immutable")

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
			if current["inadequate_day"] != float64(119) {
				t.Fatalf("held control lost its clinical examination anchor: %v", current["inadequate_day"])
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
		"accepted_main":             "80990f16af2a4ccb7472a824c77f86889c831775",
		"contract_version":          3,
		"supersedes_record_sha256":  "013a8a109f06e3ffbc40434eb2f95f9ebf1839318e2603bbbf79d5cc76f8bd25",
		"generation_source_binding": "production_source_sha256 and helper_sha256 identify the actual integrated working source; generation HEAD is ancestry only",
		"production_source_sha256":  referralSourceHashes(t), "helper_sha256": sha(helper),
		"inputs": inputs, "input_sha256": sha(canon(inputs)), "out_log_sha256": sha(raw.Bytes()),
		"cases_sha256": sha(canon(exposed)), "currents": details, "held_control": held,
	}
	// Completed Communication revisions and inadequate_day compose into new
	// signed bytes. Record a separate software-integration generation, never
	// rewrite either historical branch's controls or claim clinical validation.
	historical, err := os.ReadFile(filepath.Join("..", "web", "data", "referral-inbox-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	generationHelper, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper.go.txt"))
	if err != nil || !referralGenerationRecordIntact(historical, generationHelper) {
		t.Fatal("immutable referral generation record or witness changed")
	}
	previous, err := os.ReadFile(filepath.Join("..", "web", "data", "referral-inbox-provenance-v2.json"))
	if err != nil || sha(previous) != "013a8a109f06e3ffbc40434eb2f95f9ebf1839318e2603bbbf79d5cc76f8bd25" {
		t.Fatal("immutable completed-Communication generation changed")
	}
	previousHelper, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper-v2.go.txt"))
	if err != nil || sha(previousHelper) != "b569fcf1cfce8450961529a7efa0393e8076acc84625ef9c91503dd5f5f4e4e1" {
		t.Fatal("immutable completed-Communication generation witness changed")
	}
	clinicalDates, err := os.ReadFile(filepath.Join("..", "web", "data", "referral-inbox-clinical-dates-provenance-v2.json"))
	if err != nil || sha(clinicalDates) != "58b4900652c3ad9a148143b6393ba287bc8d908f49b69c61ef61cce00e20ec17" {
		t.Fatal("immutable clinical-date generation changed")
	}
	provenance["schema"] = "wasl-referral-integration/3"
	provenance["previous_generation_sha256"] = sha(previous)
	provenance["clinical_dates_generation_sha256"] = sha(clinicalDates)
	// Compare the unchanged exposed v2 cases before creating either v3 file.
	for _, control := range []struct {
		name  string
		value interface{}
	}{{"referral-inbox-synthetic-v2.json", exposed}, {"referral-inbox-provenance-v3.json", provenance}} {
		name, value := control.name, control.value
		path := filepath.Join("..", "web", "data", name)
		encoded, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		encoded = append(encoded, '\n')
		if *writeReferralControls && name == "referral-inbox-provenance-v3.json" {
			witnessPath := filepath.Join("testdata", "referral-inbox-generation-helper-v3.go.txt")
			witness, err := os.OpenFile(witnessPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				t.Fatal(err)
			}
			_, witnessWriteErr := witness.Write(helper)
			witnessCloseErr := witness.Close()
			if witnessWriteErr != nil || witnessCloseErr != nil {
				t.Fatalf("record generation helper: write=%v close=%v", witnessWriteErr, witnessCloseErr)
			}
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := file.Write(encoded)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("create version3 integration control: write=%v close=%v", writeErr, closeErr)
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
			if name == "referral-inbox-provenance-v3.json" {
				generationHelper, err := os.ReadFile(filepath.Join("testdata", "referral-inbox-generation-helper-v3.go.txt"))
				if err != nil {
					t.Fatal(err)
				}
				got, recomputed := actual.(map[string]interface{}), want.(map[string]interface{})
				if got["helper_sha256"] != sha(generationHelper) {
					t.Fatal("version3 generation helper differs from its recorded witness")
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

// The complete accepted-main to PR184 api.go delta: the documented owner query parameter, the effective-owner
// predicate inside getCases, and the effectiveOwner helper. Each edit must appear exactly once.
var referralOwnerDelta = [][2]string{
	{"&owner=...&unowned=true&consent=", "&owner=...&consent="},
	{"\t\towner := s.effectiveOwner(c)\n\t\teq := func", "\t\teq := func"},
	{`eq("owner", owner) && (q.Get("unowned") != "true" || owner == "") && `, `eq("owner", c.Owner) && `},
	{"// effectiveOwner is the saved coordinator owner when one exists, else the canonical owner.\nfunc (s *apiServer) effectiveOwner(c *apiCase) string {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tif c.ov.owner != \"\" {\n\t\treturn c.ov.owner\n\t}\n\treturn c.Owner\n}\n\n", ""},
}

// ownerDeltaOnly reports whether reversing the reviewed owner edits turns src into exactly the accepted api.go.
func ownerDeltaOnly(src []byte, acceptedSHA string) bool {
	s := string(src)
	for _, e := range referralOwnerDelta {
		if strings.Count(s, e[0]) != 1 {
			return false
		}
		s = strings.Replace(s, e[0], e[1], 1)
	}
	return sha([]byte(s)) == acceptedSHA
}

// Keep the reviewed owner's four-edit transform and all adverse mutation controls
// bound to their immutable source witness, not a whitelist for today's whole API.
// Current behavior is exercised by the real API owner/filter/paging/reload tests.
// Later unrelated API changes must not invalidate historical generation evidence.
func TestReferralOwnerDeltaIsNarrow(t *testing.T) {
	const acceptedAPI = "77abf1430a4e19631d323bf5e5650e53e7635d8537d351249cf68938f2ac818c"
	const reviewedOwnerAPI = "93a64beca3ac87d8dba1f7faf8c9aaf704adff5385c30df31a742616dfc12272"
	api, err := os.ReadFile(filepath.Join("testdata", "referral-owner-reviewed-api-1955.go.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if sha(api) != reviewedOwnerAPI || !ownerDeltaOnly(api, acceptedAPI) {
		t.Fatal("reviewed owner source witness or its narrow accepted-main delta changed")
	}
	mutations := map[string]func(string) string{
		"unrelated producer line": func(s string) string {
			return strings.Replace(s, "func (s *apiServer) getCases(", "var _ = 1\nfunc (s *apiServer) getCases(", 1)
		},
		"closed predicate dropped": func(s string) string {
			return strings.Replace(s, `(q.Get("closed") == "" || q.Get("closed") == strconv.FormatBool(c.Closed)) &&`, "", 1)
		},
		"inactive guard dropped": func(s string) string { return strings.Replace(s, `q.Get("include_inactive") != "true"`, `false`, 1) },
		"helper body changed": func(s string) string {
			return strings.Replace(s, "return c.ov.owner\n\t}\n\treturn c.Owner", "return c.Owner\n\t}\n\treturn c.Owner", 1)
		},
		"extra predicate": func(s string) string {
			return strings.Replace(s, `eq("dropoff", c.Dropoff)`, `eq("dropoff", c.Dropoff) && c.Consent != "x"`, 1)
		},
		"reviewed edit missing":    func(s string) string { return strings.Replace(s, "&unowned=true", "", 1) },
		"reviewed edit duplicated": func(s string) string { return s + "\n// &owner=...&unowned=true&consent=\n" },
	}
	for name, mutate := range mutations {
		m := mutate(string(api))
		if m == string(api) {
			t.Fatalf("%s: mutation did not apply", name)
		}
		if ownerDeltaOnly([]byte(m), acceptedAPI) {
			t.Fatalf("%s: unrelated api.go change was accepted", name)
		}
	}
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
