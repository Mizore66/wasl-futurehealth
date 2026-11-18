package main

// Patient contests feed the canonical engine (#167).
//
// A verified patient contest and its independent resolution are append-only
// events on the API review ledger (api_patient_request.go). This file carries
// them into the pipeline the same way reviewer sign-offs travel: one FHIR
// Provenance per ledger event on fhir.review, with the Nats-Msg-Id
// "patient-contest/<ledger event hash>" so a re-publish is deduplicated by the
// stream. PREP turns the resource into a "patient_contest" Prep, and the linker
// re-checks identity and RBAC before folding it into LoopFacts.PatientContests
// of the contested loop's tracking root. The closure engine then holds the
// questioned decision (patient_contest_hold.go).
//
// The messages are ordinary FHIR log entries, so the audit replay of
// fhir_log.ndjson should re-derive the same held or released decisions; no
// test runs audit over contest messages yet. A run that receives no contest message produces facts and
// decisions byte-identical to before.
//
// `wsi7 patient-contest-feed -run <served run>` publishes the served run's
// verified ledger contests to a running pipeline (simulated: synthetic
// patients, no message is sent to anyone).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const patientContestExtension = "urn:wsi:patient-contest"

var patientContestHashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// PatientContestEvent is one patient contest ledger event as the pipeline receives it.
type PatientContestEvent struct {
	Kind     string   `json:"kind"`               // contest_submitted | contest_resolved
	Loop     string   `json:"loop"`               // the case (decision loop) the contest questions
	Receipt  string   `json:"receipt"`            // the contest's receipt (its submission event hash)
	Decision string   `json:"decision,omitempty"` // submission: signature of the questioned decision
	Outcome  string   `json:"outcome,omitempty"`  // resolution: decision_upheld | decision_revised
	Role     string   `json:"role"`
	Actor    string   `json:"actor"`
	Excluded []string `json:"excluded,omitempty"` // submission: original reviewers who may not resolve it
	Event    string   `json:"event"`              // this ledger event's hash
	At       string   `json:"at"`
}

// linkedPatientContest is the linker's state for one contest; Excluded never enters LoopFacts.
type linkedPatientContest struct {
	PatientContestFact
	Excluded []string
}

func (e *PatientContestEvent) validate() error {
	if e == nil {
		return errors.New("missing contest")
	}
	if e.Loop == "" || len(e.Loop) > 512 || !patientContestHashRE.MatchString(e.Receipt) || !patientContestHashRE.MatchString(e.Event) {
		return errors.New("loop, receipt and event hash are required")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.At); err != nil {
		return errors.New("bad timestamp")
	}
	switch e.Kind {
	case kindContestSubmitted:
		if e.Receipt != e.Event || e.Decision == "" || len(e.Decision) > 128 || e.Outcome != "" {
			return errors.New("a submission is its own receipt and names one decision")
		}
	case kindContestResolved:
		if e.Outcome != outcomeDecisionUpheld && e.Outcome != outcomeDecisionRevised || e.Decision != "" || len(e.Excluded) != 0 {
			return errors.New("a resolution names one outcome")
		}
	default:
		return errors.New("unknown contest kind")
	}
	return nil
}

// isPatientContestResource reports whether raw is a Provenance carrying a patient contest extension.
func isPatientContestResource(raw []byte) bool {
	var res struct {
		ResourceType string `json:"resourceType"`
		Extension    []struct {
			URL string `json:"url"`
		} `json:"extension"`
	}
	if json.Unmarshal(raw, &res) != nil || res.ResourceType != "Provenance" {
		return false
	}
	for _, x := range res.Extension {
		if x.URL == patientContestExtension {
			return true
		}
	}
	return false
}

func patientContestFromResource(raw []byte) (*PatientContestEvent, error) {
	if rejectDuplicateKeys(raw) != nil {
		return nil, errors.New("duplicate keys")
	}
	var res struct {
		ResourceType string `json:"resourceType"`
		Extension    []struct {
			URL         string `json:"url"`
			ValueString string `json:"valueString"`
		} `json:"extension"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || res.ResourceType != "Provenance" {
		return nil, errors.New("not a Provenance")
	}
	var found *PatientContestEvent
	for _, x := range res.Extension {
		if x.URL != patientContestExtension {
			continue
		}
		if found != nil || rejectDuplicateKeys([]byte(x.ValueString)) != nil {
			return nil, errors.New("one patient contest per resource")
		}
		dec := json.NewDecoder(strings.NewReader(x.ValueString))
		dec.DisallowUnknownFields()
		var e PatientContestEvent
		if err := dec.Decode(&e); err != nil {
			return nil, errors.New("malformed patient contest")
		}
		found = &e
	}
	if err := found.validate(); err != nil {
		return nil, err
	}
	return found, nil
}

// patientContestResource encodes one contest event as the Provenance PREP reads.
func patientContestResource(e *PatientContestEvent) []byte {
	b, _ := json.Marshal(e)
	res := map[string]interface{}{"resourceType": "Provenance", "id": "patient-contest-" + e.Event[:32], "recorded": e.At,
		"target":    []interface{}{map[string]string{"reference": e.Loop}},
		"agent":     []interface{}{map[string]interface{}{"who": map[string]string{"display": e.Actor}}},
		"extension": []interface{}{map[string]string{"url": patientContestExtension, "valueString": string(b)}}}
	out, _ := json.Marshal(res)
	return out
}

// patientContestFromLedger maps one replay-validated API ledger event to its feed form.
func patientContestFromLedger(ev map[string]interface{}) (*PatientContestEvent, bool) {
	if sstr(ev, "type") != patientRequestEventType {
		return nil, false
	}
	e := &PatientContestEvent{Kind: sstr(ev, "kind"), Loop: sstr(ev, "case"), Role: sstr(ev, "role"), Actor: sstr(ev, "actor"), Event: sstr(ev, "hash"), At: sstr(ev, "at")}
	switch e.Kind {
	case kindContestSubmitted:
		e.Receipt, e.Decision = e.Event, sstr(ev, "decision")
		if list, ok := ev["excluded_reviewers"].([]interface{}); ok {
			for _, v := range list {
				e.Excluded = append(e.Excluded, fmt.Sprint(v))
			}
		}
	case kindContestResolved:
		e.Receipt, e.Outcome = sstr(ev, "contest"), sstr(ev, "outcome")
	default:
		return nil, false
	}
	return e, e.validate() == nil
}

// patientContestFeedMessages lists the fhir.review messages for every contest event in ledger order.
func patientContestFeedMessages(events []map[string]interface{}) []LogMsg {
	out := []LogMsg{}
	for _, ev := range events {
		e, ok := patientContestFromLedger(ev)
		if !ok {
			continue
		}
		out = append(out, LogMsg{Subject: "fhir.review", Hdr: map[string]string{"Nats-Msg-Id": "patient-contest/" + e.Event}, Data: patientContestResource(e)})
	}
	return out
}

// applyPatientContest folds one contest event into the linker with the same rules the API enforces. It returns
// the tracking root whose facts changed, or false when the event is refused or a duplicate.
func (L *LinkerCore) applyPatientContest(e *PatientContestEvent) (string, bool) {
	if e.validate() != nil {
		L.Stats["invalid_patient_contest"]++
		return "", false
	}
	if L.patientContests == nil {
		L.patientContests = map[string][]*linkedPatientContest{}
	}
	root := baseLoop(e.Loop)
	var match *linkedPatientContest
	for _, c := range L.patientContests[root] {
		if c.Receipt == e.Receipt {
			match = c
		}
	}
	switch e.Kind {
	case kindContestSubmitted:
		if e.Role != patientRole || e.Actor == "" || !strings.HasPrefix(e.Actor, "patient-link/") {
			L.Stats["rbac_denied_patient_contest"]++
			return "", false
		}
		if match != nil {
			L.Stats["duplicate_patient_contest"]++
			return "", false
		}
		for _, c := range L.patientContests[root] {
			if c.Outcome == "" && c.Loop == e.Loop && c.Decision == e.Decision {
				L.Stats["duplicate_patient_contest"]++
				return "", false
			}
		}
		L.patientContests[root] = append(L.patientContests[root], &linkedPatientContest{
			PatientContestFact: PatientContestFact{Receipt: e.Receipt, Decision: e.Decision, Loop: e.Loop}, Excluded: append([]string(nil), e.Excluded...)})
	case kindContestResolved:
		if Authorize(e.Role, ActResolvePatientContest) != nil || e.Actor == "" {
			L.Stats["rbac_denied_patient_contest"]++
			return "", false
		}
		if match == nil || match.Loop != e.Loop || match.Outcome != "" {
			L.Stats["unmatched_patient_contest_resolution"]++
			return "", false
		}
		for _, x := range match.Excluded {
			if x == e.Actor {
				L.Stats["rbac_denied_patient_contest"]++
				return "", false
			}
		}
		match.Outcome = e.Outcome
	}
	L.Stats["patient_contests"]++
	return root, true
}

// patientContestFacts is the fact copy for one tracking root; nil (absent from the facts) without a contest.
func (L *LinkerCore) patientContestFacts(root string) []PatientContestFact {
	list := L.patientContests[root]
	if len(list) == 0 {
		return nil
	}
	out := make([]PatientContestFact, len(list))
	for i, c := range list {
		out[i] = c.PatientContestFact
		if c.Loop == root {
			out[i].Loop = "" // a root contest keeps the historical fact shape
		}
	}
	return out
}

// runPatientContestFeed publishes a served run's verified ledger contests to fhir.review.
func runPatientContestFeed(run, data string) {
	s, err := loadAPIVerified(run, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "patient-contest-feed:", err)
		os.Exit(1)
	}
	if s.ledgerErr != "" {
		fmt.Fprintln(os.Stderr, "patient-contest-feed: review ledger failed validation; nothing published:", s.ledgerErr)
		os.Exit(1)
	}
	msgs := patientContestFeedMessages(s.reviews)
	nc, js := connect("patient-contest-feed")
	defer nc.Close()
	published, duplicate := publishPatientContestFeed(js, msgs)
	fmt.Printf("patient-contest-feed: %d contest events, %d published, %d already in the stream (simulated; no message dispatched)\n", len(msgs), published, duplicate)
}

// publishPatientContestFeed publishes the feed messages with their ledger-derived Nats-Msg-Id, so a re-run is a
// stream-level no-op. It returns how many the stream stored and how many it already had.
func publishPatientContestFeed(js jetstream.JetStream, msgs []LogMsg) (published, duplicate int) {
	for _, m := range msgs {
		out := nats.NewMsg(m.Subject)
		out.Data = m.Data
		out.Header.Set(jetstream.MsgIDHeader, m.Hdr["Nats-Msg-Id"])
		out.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
		if pubSync(js, out) {
			duplicate++
		} else {
			published++
		}
	}
	return published, duplicate
}
