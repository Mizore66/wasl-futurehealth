package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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

// Exact reviewed elapsed-slot delta relative to accepted main 4b93509. The test
// reverses these three chunks IN MEMORY, each exactly once, and then requires the
// COMPLETE restored files to equal the accepted-main checksums. Nothing is masked.
const (
	referralMainAPI           = "77abf1430a4e19631d323bf5e5650e53e7635d8537d351249cf68938f2ac818c"
	referralMainBooking       = "20ed5d55025a52d371d760e916eaa34cbc0a00d61b50b61840912e0a5371d2cf"
	referralGenerationAPI     = "9314f93a56a1c8ba0004060e8229eaefba816734b22ac67e9fb148eee2e363c4"
	referralGenerationBooking = "20ed5d55025a52d371d760e916eaa34cbc0a00d61b50b61840912e0a5371d2cf"
)

type referralElapsedChunk struct{ name, current, original string }

var (
	referralElapsedAPIChunk = referralElapsedChunk{"api live caseAction guard",
		"\t\t\tbooking, failure = s.prepareBookingLocked(c, kind, data)\n" +
			"\t\t\tif failure == nil {\n" +
			"\t\t\t\tfailure = s.elapsedSlotFailure(c, booking)\n" +
			"\t\t\t}\n",
		"\t\t\tbooking, failure = s.prepareBookingLocked(c, kind, data)\n"}
	referralElapsedHelperChunk = referralElapsedChunk{"api_booking slotElapsed/elapsedSlotFailure",
		"// A slot has elapsed when its day is before the case's latest decision run day; the run day itself is still bookable.\n" +
			"func slotElapsed(c *apiCase, slot *simulatedSlot) bool {\n" +
			"\treturn c != nil && c.last != nil && float64(slot.day) < c.last.Now\n" +
			"}\n" +
			"\n" +
			"// Live writes only. Replay must not apply this: historical reservations were valid on the run clock of their time.\n" +
			"func (s *apiServer) elapsedSlotFailure(c *apiCase, transition *bookingTransition) *bookingFailure {\n" +
			"\tif transition == nil || transition.targetID == \"\" {\n" +
			"\t\treturn nil\n" +
			"\t}\n" +
			"\tslot := s.bookingSlots[transition.targetID]\n" +
			"\tif !slotElapsed(c, slot) {\n" +
			"\t\treturn nil\n" +
			"\t}\n" +
			"\treturn bookingFail(409, \"simulated slot day has elapsed on the run clock\", \"slot_elapsed\", map[string]interface{}{\n" +
			"\t\t\"slot_id\": slot.id, \"slot_day\": slot.day, \"run_day\": c.last.Now,\n" +
			"\t})\n" +
			"}\n" +
			"\n",
		""}
	referralElapsedProjectionChunk = referralElapsedChunk{"api_booking elapsed candidate projection",
		"\t\t\t\t\tcandidate[\"availability\"] = \"reserved\"\n" +
			"\t\t\t\t} else if slotElapsed(s.cases[id], slot) {\n" +
			"\t\t\t\t\tcandidate[\"availability\"] = \"elapsed\"\n" +
			"\t\t\t\t}\n",
		"\t\t\t\t\tcandidate[\"availability\"] = \"reserved\"\n" +
			"\t\t\t\t}\n"}
)

func restoreReferralChunks(name string, src []byte, chunks ...referralElapsedChunk) ([]byte, error) {
	for _, chunk := range chunks {
		if n := bytes.Count(src, []byte(chunk.current)); n != 1 {
			return nil, fmt.Errorf("%s: elapsed chunk %q must occur exactly once, found %d", name, chunk.name, n)
		}
		src = bytes.Replace(src, []byte(chunk.current), []byte(chunk.original), 1)
	}
	return src, nil
}

// reverseReferralElapsedDelta works only on copies held in memory.
func reverseReferralElapsedDelta(apiSrc, bookingSrc []byte) ([]byte, []byte, error) {
	restoredAPI, err := restoreReferralChunks("api.go", apiSrc, referralElapsedAPIChunk)
	if err != nil {
		return nil, nil, err
	}
	restoredBooking, err := restoreReferralChunks("api_booking.go", bookingSrc, referralElapsedHelperChunk, referralElapsedProjectionChunk)
	if err != nil {
		return nil, nil, err
	}
	return restoredAPI, restoredBooking, nil
}

// proveReferralElapsedCompatibility is the only way the saved generation hashes for
// api.go and api_booking.go may be compared with the current source. It never
// accepts a new hash by itself: the complete reversed bytes must equal accepted main.
func proveReferralElapsedCompatibility(apiSrc, bookingSrc []byte, saved, recomputed map[string]interface{}) error {
	if saved["api.go"] != referralGenerationAPI || saved["api_booking.go"] != referralGenerationBooking {
		return fmt.Errorf("saved generation api/booking hashes are not the approved generation hashes")
	}
	if recomputed["api.go"] != sha(apiSrc) || recomputed["api_booking.go"] != sha(bookingSrc) {
		return fmt.Errorf("recomputed source hashes do not describe the inspected bytes")
	}
	restoredAPI, restoredBooking, err := reverseReferralElapsedDelta(apiSrc, bookingSrc)
	if err != nil {
		return err
	}
	if sha(restoredAPI) != referralMainAPI {
		return fmt.Errorf("api.go reversed elapsed delta does not equal accepted main")
	}
	if sha(restoredBooking) != referralMainBooking {
		return fmt.Errorf("api_booking.go reversed elapsed delta does not equal accepted main")
	}
	return nil
}

// Executed as subtests of TestReferralInboxCanonicalControls. Every mutation starts
// from the actual current bytes and must change them; the honest recomputed hashes of the
// mutated bytes are supplied, so only the in-memory reversal proof can reject them.
func runReferralSourceCompatibilityControls(t *testing.T, saved map[string]interface{}) {
	apiSrc, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	bookingSrc, err := os.ReadFile("api_booking.go")
	if err != nil {
		t.Fatal(err)
	}
	prove := func(apiBytes, bookingBytes []byte) error {
		return proveReferralElapsedCompatibility(apiBytes, bookingBytes, saved, map[string]interface{}{"api.go": sha(apiBytes), "api_booking.go": sha(bookingBytes)})
	}
	mutate := func(t *testing.T, src []byte, old, replacement string) []byte {
		t.Helper()
		if n := bytes.Count(src, []byte(old)); n != 1 {
			t.Fatalf("mutation anchor %q must occur exactly once, found %d", old, n)
		}
		out := bytes.Replace(src, []byte(old), []byte(replacement), 1)
		if bytes.Equal(out, src) {
			t.Fatalf("mutation did not change bytes: %q", old)
		}
		return out
	}
	t.Run("positive_complete_byte_reversal", func(t *testing.T) {
		if err := prove(apiSrc, bookingSrc); err != nil {
			t.Fatal(err)
		}
		restoredAPI, restoredBooking, err := reverseReferralElapsedDelta(apiSrc, bookingSrc)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(restoredAPI, apiSrc) || bytes.Equal(restoredBooking, bookingSrc) {
			t.Fatal("reversal did not change the current bytes")
		}
		if sha(restoredAPI) != referralMainAPI || sha(restoredBooking) != referralMainBooking {
			t.Fatal("reversal does not reproduce accepted main bytes")
		}
	})
	cases := []struct {
		name   string
		mutate func(t *testing.T) (apiBytes, bookingBytes []byte)
	}{
		{"missing_live_guard", func(t *testing.T) ([]byte, []byte) {
			return mutate(t, apiSrc, referralElapsedAPIChunk.current, referralElapsedAPIChunk.original), bookingSrc
		}},
		{"duplicated_live_guard", func(t *testing.T) ([]byte, []byte) {
			return mutate(t, apiSrc, referralElapsedAPIChunk.current, referralElapsedAPIChunk.current+referralElapsedAPIChunk.current), bookingSrc
		}},
		{"missing_helper_block", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, referralElapsedHelperChunk.current, "")
		}},
		{"duplicated_helper_block", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, referralElapsedHelperChunk.current, referralElapsedHelperChunk.current+referralElapsedHelperChunk.current)
		}},
		{"missing_projection", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, referralElapsedProjectionChunk.current, referralElapsedProjectionChunk.original)
		}},
		{"duplicated_projection", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, referralElapsedProjectionChunk.current, referralElapsedProjectionChunk.current+referralElapsedProjectionChunk.current)
		}},
		{"changed_less_than_to_less_or_equal", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "float64(slot.day) < c.last.Now", "float64(slot.day) <= c.last.Now")
		}},
		{"changed_helper_nil_guard", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "return c != nil && c.last != nil && float64", "return c.last != nil && float64")
		}},
		{"changed_failure_target_guard", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "if transition == nil || transition.targetID == \"\" {", "if transition == nil {")
		}},
		{"unrelated_live_api_mutation", func(t *testing.T) ([]byte, []byte) {
			return mutate(t, apiSrc, "if !guard.Allowed {", "if guard.Allowed {"), bookingSrc
		}},
		{"dropped_readiness_guard", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "if reason := bookingReadiness(c); reason != \"\" {", "if false {")
		}},
		{"dropped_closed_guard", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "if c.Closed || !bookingOpen[c.State] {", "if !bookingOpen[c.State] {")
		}},
		{"dropped_inactive_reservation_guard", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "if status != \"unreserved\" && status != \"cancelled\" {", "if false {")
		}},
		{"elapsed_guard_inserted_into_replay", func(t *testing.T) ([]byte, []byte) {
			return mutate(t, apiSrc, "\t\tbooking, failure = s.prepareBookingLocked(c, kind, norm)\n",
				"\t\tbooking, failure = s.prepareBookingLocked(c, kind, norm)\n\t\tif failure == nil {\n\t\t\tfailure = s.elapsedSlotFailure(c, booking)\n\t\t}\n"), bookingSrc
		}},
		{"dropped_signed_catalogue_clone", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "cloneBookingJSON(raw).(map[string]interface{})", "raw")
		}},
		{"unrelated_booking_mutation", func(t *testing.T) ([]byte, []byte) {
			return apiSrc, mutate(t, bookingSrc, "case is not open and booking-eligible", "case is not open or booking-eligible")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiBytes, bookingBytes := tc.mutate(t)
			if bytes.Equal(apiBytes, apiSrc) && bytes.Equal(bookingBytes, bookingSrc) {
				t.Fatal("control did not change the actual current bytes")
			}
			if err := prove(apiBytes, bookingBytes); err == nil {
				t.Fatal("elapsed source compatibility accepted a mutated source")
			}
		})
	}
	t.Run("hash_only_blessing_rejected", func(t *testing.T) {
		stale := map[string]interface{}{"api.go": sha(apiSrc), "api_booking.go": "0000000000000000000000000000000000000000000000000000000000000000"}
		if err := proveReferralElapsedCompatibility(apiSrc, bookingSrc, saved, stale); err == nil {
			t.Fatal("accepted recomputed hashes that do not describe the inspected bytes")
		}
	})
	t.Run("altered_saved_generation_rejected", func(t *testing.T) {
		for _, name := range []string{"api.go", "api_booking.go"} {
			altered := map[string]interface{}{"api.go": saved["api.go"], "api_booking.go": saved["api_booking.go"]}
			altered[name] = sha(apiSrc)
			if err := proveReferralElapsedCompatibility(apiSrc, bookingSrc, altered, map[string]interface{}{"api.go": sha(apiSrc), "api_booking.go": sha(bookingSrc)}); err == nil {
				t.Fatalf("accepted altered saved %s generation hash", name)
			}
		}
	})
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
				// Approved generation hashes stay saved. The current elapsed-slot source is
				// compared only after its reviewed delta is reversed in memory and the complete
				// api.go/api_booking.go bytes equal accepted main. All other hashes and outputs compare.
				got, recomputed := actual.(map[string]interface{}), want.(map[string]interface{})
				sourceHashes := recomputed["production_source_sha256"].(map[string]interface{})
				savedHashes := got["production_source_sha256"].(map[string]interface{})
				currentAPI, err := os.ReadFile("api.go")
				if err != nil {
					t.Fatal(err)
				}
				currentBooking, err := os.ReadFile("api_booking.go")
				if err != nil {
					t.Fatal(err)
				}
				if err := proveReferralElapsedCompatibility(currentAPI, currentBooking, savedHashes, sourceHashes); err != nil {
					t.Fatalf("unexpected API/booking source change since approved referral generation: %v", err)
				}
				runReferralSourceCompatibilityControls(t, savedHashes)
				sourceHashes["api.go"] = referralGenerationAPI
				sourceHashes["api_booking.go"] = referralGenerationBooking
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
