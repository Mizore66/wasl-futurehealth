package main

// Staff-recorded communications into the canonical Communication pathway (issue #197).
//
// The API records a staff-assisted result notification and its receipt acknowledgement on the review ledger
// (api_communication.go) and serves them as a projection over the signed decision. This file turns those replayed
// facts into FHIR Communication resources and writes them into a NEW cohort directory, merged by arrival into the
// referring provider's source stream. A pipeline run over that cohort ingests them exactly like any source
// Communication: normalise (fhir.go) -> commFactFromEvent (linker.go) -> the closure engine's communication layer.
// The engine, not this file, decides whether a fact counts: it re-verifies the notification wording against the
// approved template and the patient's recorded language, and only completed Communications on or before the run's
// clock are evaluated.
//
// Boundaries kept here:
//   - only facts that survived verified ledger replay are exported; an invalid ledger exports nothing;
//   - a notification or receipt is a communication fact, never clinical evidence, so nothing here can close a loop;
//   - receipt is not comprehension: no teach-back fact is written;
//   - a fact dated after the run horizon (TEnd) is listed in the manifest and NOT injected, because the run's clock
//     would otherwise advance past the horizon and re-time every other decision;
//   - the source cohort is never modified: the output directory must not exist, and untouched files are symlinked.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	staffCommManifest   = "staff_communications.json"
	staffCommIDPrefix   = "staff-"
	staffCommMsgPrefix  = "staff/"
	staffReceiptText    = "Receipt of the result notification confirmed to staff. Not a comprehension check."
	staffCommExtLedger  = "urn:wsi:ledger-event"
	staffCommExtActor   = "urn:wsi:recorded-by"
	staffCommExtIDCheck = "urn:wsi:identity-check"
)

// staffComm is one replayed staff fact ready for the source stream, or a fact held back with its reason.
type staffComm struct {
	Case     string                 `json:"case"`
	Kind     string                 `json:"kind"`
	Event    string                 `json:"event"`
	Provider string                 `json:"provider"`
	Day      float64                `json:"day"`
	Resource string                 `json:"resource,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
	res      map[string]interface{} // the FHIR Communication, never in the manifest (the stream carries it)
}

func staffCommID(event string) string {
	if len(event) > 24 {
		event = event[:24]
	}
	return staffCommIDPrefix + event
}

// staffServiceRequest is the ServiceRequest id a case's loop id names (provider/ServiceRequest/<id>).
func staffServiceRequest(c *apiCase) (string, error) {
	prefix := c.Provider + "/ServiceRequest/"
	if !strings.HasPrefix(c.ID, prefix) || len(c.ID) == len(prefix) {
		return "", fmt.Errorf("case %s is not a ServiceRequest loop of %s", c.ID, c.Provider)
	}
	return strings.TrimPrefix(c.ID, prefix), nil
}

// staffCommunicationResource is the FHIR Communication for one replayed staff fact (an overlay entry of
// api_communication.go): a notification is an outbound result-notification carrying the approved wording the staff
// member read; a receipt is an inbound acknowledgement answering it.
func staffCommunicationResource(c *apiCase, kind string, entry map[string]interface{}) (map[string]interface{}, float64, error) {
	sr, err := staffServiceRequest(c)
	if err != nil {
		return nil, 0, err
	}
	event, _ := entry["event"].(string)
	occurred, _ := entry["occurred_at"].(string)
	channel, _ := entry["channel"].(string)
	if event == "" || occurred == "" || !communicationChannels[channel] {
		return nil, 0, errors.New("staff communication entry is incomplete")
	}
	day, err := tsToDay(occurred)
	if err != nil {
		return nil, 0, err
	}
	ext := []interface{}{}
	res := map[string]interface{}{"resourceType": "Communication", "id": staffCommID(event), "status": "completed",
		"basedOn": []interface{}{map[string]string{"reference": "ServiceRequest/" + sr}},
		"medium":  []interface{}{map[string]string{"text": channel}}}
	switch kind {
	case notificationKind:
		tpl, _ := entry["template"].(string)
		text, why := Render(tpl, c.Provider)
		if why != "" {
			return nil, 0, fmt.Errorf("notification template %q: %s", tpl, why)
		}
		res["category"] = []interface{}{map[string]interface{}{"text": "result-notification"}}
		res["payload"] = []interface{}{map[string]string{"contentString": text}}
		res["language"] = Templates[tpl].Lang
		res["sent"] = occurred
		ext = append(ext, map[string]string{"url": "urn:wsi:template", "valueString": tpl})
	case receiptKind:
		res["category"] = []interface{}{map[string]interface{}{"text": "acknowledgement"}}
		res["payload"] = []interface{}{map[string]string{"contentString": staffReceiptText}}
		res["received"] = occurred
		if ref, _ := entry["notification_event"].(string); ref != "" && ref != canonicalNotification {
			res["inResponseTo"] = []interface{}{map[string]string{"reference": "Communication/" + staffCommID(ref)}}
		}
	default:
		return nil, 0, fmt.Errorf("not a staff communication kind: %s", kind)
	}
	actor, _ := entry["actor"].(string)
	check, _ := entry["identity_check"].(string)
	ext = append(ext, map[string]string{"url": staffCommExtLedger, "valueString": event},
		map[string]string{"url": staffCommExtActor, "valueString": actor},
		map[string]string{"url": staffCommExtIDCheck, "valueString": check})
	res["extension"] = ext
	return res, day, nil
}

// staffCommunications lists every replayed staff fact of a verified API load, in case order then notification first.
// Facts after the run horizon are returned with a reason and no resource.
func (s *apiServer) staffCommunications() ([]staffComm, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ledgerErr != "" {
		return nil, errors.New("review ledger failed validation; nothing exported: " + s.ledgerErr)
	}
	var out []staffComm
	for _, id := range s.order {
		c := s.cases[id]
		for _, k := range []struct {
			kind  string
			entry map[string]interface{}
		}{{notificationKind, c.ov.notification}, {receiptKind, c.ov.receipt}} {
			if k.entry == nil {
				continue
			}
			res, day, err := staffCommunicationResource(c, k.kind, k.entry)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", c.ID, k.kind, err)
			}
			sc := staffComm{Case: c.ID, Kind: k.kind, Event: k.entry["event"].(string), Provider: c.Provider, Day: day}
			if day > TEnd {
				sc.Reason = fmt.Sprintf("after the run horizon (day %.0f, %s); a run at this horizon does not evaluate it", TEnd, dayDate(TEnd).Format(calendarDayLayout))
			} else {
				sc.Resource, sc.res = "Communication/"+res["id"].(string), res
			}
			out = append(out, sc)
		}
	}
	return out, nil
}

// writeStaffCommunicationCohort writes a NEW cohort directory: every file of `data` symlinked, except the provider
// streams that receive staff Communications, which are rewritten with them merged in by arrival (stable: a staff fact
// follows source events of the same arrival). Returns the manifest it wrote.
func writeStaffCommunicationCohort(run, data, out string, comms []staffComm) (map[string]interface{}, error) {
	return writeLedgerCohort(run, data, out, staffCommManifest, "staff-recorded notification and receipt facts from the verified review ledger, as FHIR Communications on the referring provider's stream. "+
		"The closure engine re-verifies them; they are communication facts, never clinical evidence, and receipt is not comprehension.", comms)
}

// writeLedgerCohort writes a NEW cohort directory holding ledger-derived source resources (staff communications, or
// retrieved reports in api_retrieved_report.go) merged by arrival into their provider streams, and its manifest.
func writeLedgerCohort(run, data, out, manifestName, note string, comms []staffComm) (map[string]interface{}, error) {
	absData, err := filepath.Abs(data)
	if err != nil {
		return nil, err
	}
	absOut, err := filepath.Abs(out)
	if err != nil {
		return nil, err
	}
	if absOut == absData {
		return nil, errors.New("-out must be a new directory, not the source cohort")
	}
	if _, err := os.Stat(absOut); err == nil {
		return nil, fmt.Errorf("%s already exists; staffcomms writes a NEW cohort directory", absOut)
	}
	byProv := map[string][]staffComm{}
	var injected, held []staffComm
	for _, sc := range comms {
		if sc.res == nil {
			held = append(held, sc)
			continue
		}
		injected = append(injected, sc)
		byProv[sc.Provider] = append(byProv[sc.Provider], sc)
	}
	for prov := range byProv {
		if _, err := os.Stat(filepath.Join(absData, "fhir_"+prov+".ndjson")); err != nil {
			return nil, fmt.Errorf("provider stream fhir_%s.ndjson: %w", prov, err)
		}
		sort.SliceStable(byProv[prov], func(i, j int) bool { return byProv[prov][i].Day < byProv[prov][j].Day })
	}
	entries, err := os.ReadDir(absData)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absOut, 0700); err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if name == manifestName {
			continue
		}
		prov := strings.TrimSuffix(strings.TrimPrefix(name, "fhir_"), ".ndjson")
		if staff, ok := byProv[prov]; ok && name == "fhir_"+prov+".ndjson" {
			if err := mergeStaffStream(filepath.Join(absData, name), filepath.Join(absOut, name), prov, staff); err != nil {
				return nil, err
			}
			continue
		}
		if err := os.Symlink(filepath.Join(absData, name), filepath.Join(absOut, name)); err != nil {
			return nil, err
		}
	}
	if injected == nil {
		injected = []staffComm{}
	}
	if held == nil {
		held = []staffComm{}
	}
	manifest := map[string]interface{}{
		"source_run": run, "source_cohort": absData, "horizon_day": TEnd, "horizon_date": dayDate(TEnd).Format(calendarDayLayout),
		"injected": injected, "after_horizon": held, "note": note,
	}
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return manifest, os.WriteFile(filepath.Join(absOut, manifestName), append(b, '\n'), 0600)
}

// mergeStaffStream copies a provider stream line for line and inserts each staff Communication before the first source
// event that arrives strictly later.
func mergeStaffStream(src, dst, prov string, staff []staffComm) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	var buf bytes.Buffer
	emit := func(sc staffComm) error {
		prefix := staffCommMsgPrefix
		if sc.Kind == retrievedReportKind {
			prefix = "retrieved/"
		}
		b, err := json.Marshal(GenEvent{Arrival: sc.Day, Prov: prov, MsgID: prefix + sc.Event, Res: sc.res})
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		return nil
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	i := 0
	for sc.Scan() {
		line := sc.Bytes()
		var ge struct {
			Arrival float64 `json:"arrival"`
		}
		if err := json.Unmarshal(line, &ge); err != nil {
			return fmt.Errorf("%s: %w", src, err)
		}
		for i < len(staff) && staff[i].Day < ge.Arrival {
			if err := emit(staff[i]); err != nil {
				return err
			}
			i++
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for ; i < len(staff); i++ {
		if err := emit(staff[i]); err != nil {
			return err
		}
	}
	return os.WriteFile(dst, buf.Bytes(), 0600)
}

// runStaffComms is `wsi7 staffcomms -run <run> -data <cohort> -out <new cohort>`.
func runStaffComms(run, data, out string) error {
	s, err := loadAPIVerified(run, data)
	if err != nil {
		return fmt.Errorf("verified API load: %w", err)
	}
	comms, err := s.staffCommunications()
	if err != nil {
		return err
	}
	m, err := writeStaffCommunicationCohort(run, data, out, comms)
	if err != nil {
		return err
	}
	fmt.Printf("staffcomms: %d injected, %d after the run horizon; cohort %s\n", len(m["injected"].([]staffComm)),
		len(m["after_horizon"].([]staffComm)), out)
	return nil
}
