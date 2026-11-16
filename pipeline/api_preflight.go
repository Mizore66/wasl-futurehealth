package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const apiPreflightMaxNDJSONRecord = 1 << 24

// validateAPIPreflight rejects incomplete materialized runs before the serve API
// can turn read or scanner failures into apparently healthy empty projections.
func validateAPIPreflight(run string) error {
	objects := map[string]map[string]interface{}{}
	for _, name := range []string{"dashboard.json", "report7.json", "anchors.json"} {
		object, err := validateRequiredAPIJSON(filepath.Join(run, name))
		if err != nil {
			return err
		}
		objects[name] = object
	}
	for _, name := range []string{"out_log.ndjson", "fhir_log.ndjson"} {
		if err := validateRequiredAPINDJSON(filepath.Join(run, name)); err != nil {
			return err
		}
	}
	// No-review runs do not start the reviewer, so they have no reviewer ledger.
	if err := validateRequiredAPINDJSON(filepath.Join(run, "reviewer_decisions.ndjson")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// booking.json is optional (booking/suggest.py may not have run), but a present document must be one strict
	// top-level object; otherwise booking and capacity would report not_generated for corrupted evidence.
	if _, err := readBookingArtifact(filepath.Join(run, "booking.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	loops := map[string]string{}
	outPath, fhirPath := filepath.Join(run, "out_log.ndjson"), filepath.Join(run, "fhir_log.ndjson")
	projection := newMaterializedDecisions()
	if _, err := scanAPILog(outPath, func(m LogMsg) error {
		if err := validateOutRecord(m, loops); err != nil {
			return err
		}
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return err
		}
		return projection.add(m, env)
	}); err != nil {
		return err
	}
	counts, err := projection.validate()
	if err != nil {
		return fmt.Errorf("serve preflight: out_log.ndjson: %w", err)
	}
	fhirRecords, err := scanAPILog(fhirPath, func(m LogMsg) error {
		err := validateFHIRRecord(m)
		if err != nil && preservedAPIRejection(run, m) {
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	if err := validateMaterializationMetadata(objects["dashboard.json"], objects["report7.json"], counts); err != nil {
		return err
	}
	// Root screening/index cases (not independently derived care children) are the
	// legacy clinical denominator; every retained case is the OUT completeness set.
	roots := map[string]string{}
	for id, d := range projection.final {
		if kind, _ := materializedCaseKind(d); kind != "derived_care" {
			roots[id] = firstNonEmpty(d.Prog, "crc")
		}
	}
	active := materializedCatalogCounts(projection.final)["active_cases"].(int)
	return validateDeclaredCountsRoots(objects["dashboard.json"], objects["report7.json"], loops, roots, fhirRecords, active, true)
}

func validateRequiredAPIJSON(path string) (map[string]interface{}, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("serve preflight: required JSON %s: %w", path, err)
	}
	var value interface{}
	if err := json.Unmarshal(contents, &value); err != nil {
		return nil, fmt.Errorf("serve preflight: %s: %s JSON: %w", path, apiJSONErrorKind(err), err)
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("serve preflight: %s: invalid JSON: expected top-level object", path)
	}
	return object, nil
}

// scanAPILog decodes each already syntax-checked record as the canonical log envelope and hands it to check.
// A log with no records is rejected because every required log carries at least one materialized message.
func scanAPILog(path string, check func(LogMsg) error) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("serve preflight: required NDJSON %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), apiPreflightMaxNDJSONRecord)
	records := 0
	for scanner.Scan() {
		records++
		var m LogMsg
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			return 0, fmt.Errorf("serve preflight: %s:%d: invalid log envelope: %w", path, records, err)
		}
		if m.Seq == 0 || m.Subject == "" || len(m.Data) == 0 {
			return 0, fmt.Errorf("serve preflight: %s:%d: invalid log envelope: seq, subj and data are required", path, records)
		}
		if err := check(m); err != nil {
			return 0, fmt.Errorf("serve preflight: %s:%d: %w", path, records, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("serve preflight: %s:%d: NDJSON scanner completion: %w", path, records+1, err)
	}
	if records == 0 {
		return 0, fmt.Errorf("serve preflight: %s: log has no records", path)
	}
	return records, nil
}

var apiOUTSubject = regexp.MustCompile(`^out\.p[0-9]+$`)
var apiFHIRSubject = regexp.MustCompile(`^fhir\.[A-Za-z0-9_-]+\.(res|ctl)$`)
var apiLegacyObservationSubject = regexp.MustCompile(`^[A-Za-z0-9_-]+\.obs$`)

func validateOutRecord(m LogMsg, loops map[string]string) error {
	if !apiOUTSubject.MatchString(m.Subject) {
		return fmt.Errorf("invalid out envelope: unknown subject %q", m.Subject)
	}
	var env OutEnvelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		return fmt.Errorf("invalid out envelope: %w", err)
	}
	switch env.Kind {
	case "dec", "decision": // both the legacy fixtures and canonical closure envelopes
		if env.Dec == nil || env.Item != nil {
			return fmt.Errorf("invalid out envelope: decision payload required, without review item")
		}
		d := env.Dec
		if d.ID == "" || d.LoopID == "" || d.Provider == "" || d.State == "" || d.Sig == "" {
			return fmt.Errorf("invalid out envelope: decision requires id, loop, prov, state and sig")
		}
		programme := firstNonEmpty(d.Prog, "crc") // pre-multiprogramme logs were CRC
		if personProgrammeRank(programme) == len(personProgrammes) {
			return fmt.Errorf("invalid out envelope: unsupported decision prog %q", programme)
		}
		// OUT is a history. Canonical tracking revisions can correct a loop's
		// programme; source-derived counts use its latest retained decision.
		loops[d.LoopID] = programme
	case "item", "review":
		if env.Item == nil || env.Dec != nil {
			return fmt.Errorf("invalid out envelope: review item payload required, without decision")
		}
		if env.Item.ItemID == "" || env.Item.LoopID == "" || len(env.Item.Kinds) == 0 {
			return fmt.Errorf("invalid out envelope: review item requires item, loop and kinds")
		}
		for _, kind := range env.Item.Kinds {
			if kind != "link" && kind != "evidence" {
				return fmt.Errorf("invalid out envelope: unsupported review item kind %q", kind)
			}
		}
	default:
		return fmt.Errorf("invalid out envelope: unknown kind %q", env.Kind)
	}
	return nil
}

func validateFHIRRecord(m LogMsg) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(m.Data, &payload); err != nil || len(payload) == 0 {
		return fmt.Errorf("invalid FHIR record: data must be a non-empty object")
	}
	canonical := apiFHIRSubject.MatchString(m.Subject)
	legacy := apiLegacyObservationSubject.MatchString(m.Subject)
	if !canonical && !legacy && m.Subject != "fhir.review" {
		return fmt.Errorf("invalid FHIR record: unknown subject %q", m.Subject)
	}
	if canonical && strings.HasSuffix(m.Subject, ".ctl") {
		var eof bool
		if json.Unmarshal(payload["eof"], &eof) != nil || !eof {
			return fmt.Errorf("invalid FHIR control: eof must be true")
		}
		return nil
	}
	var rt, id, status string
	if json.Unmarshal(payload["resourceType"], &rt) != nil || rt == "" {
		return fmt.Errorf("invalid FHIR record: resourceType is required")
	}
	if legacy {
		if rt != "Observation" || json.Unmarshal(payload["id"], &id) != nil || id == "" ||
			json.Unmarshal(payload["status"], &status) != nil || status == "" {
			return fmt.Errorf("invalid legacy observation: resourceType Observation, id and status required")
		}
		return nil
	}
	if m.Subject == "fhir.review" && rt != "Provenance" {
		return fmt.Errorf("invalid FHIR review: resourceType must be Provenance")
	}
	prov := strings.Split(m.Subject, ".")[1]
	event, signoff, normErr := normaliseChecked(prov, 0, m.Data)
	if normErr != nil {
		return fmt.Errorf("invalid FHIR payload: %w", normErr)
	}
	if signoff != nil {
		// Authorization/reviewer answers may deliberately be rejected by the linker.
		// Validate the sign-off shape, not whether it would be accepted clinically.
		if signoff.ItemID == "" || signoff.LoopID == "" || len(signoff.Kinds) == 0 {
			return fmt.Errorf("invalid FHIR signoff: item, loop and kinds required")
		}
		return nil
	}
	if event == nil || event.ID == "" || event.Status == "" {
		return fmt.Errorf("invalid FHIR resource: supported operational resource with id and status required")
	}
	return nil
}

// An immutable source log can intentionally contain input quarantined by PREP.
// Only a matching structured normalization rejection (not a naked skip, arbitrary
// header or unknown subject/control) permits such a record to remain as evidence.
func preservedAPIRejection(run string, m LogMsg) bool {
	if !(apiFHIRSubject.MatchString(m.Subject) && strings.HasSuffix(m.Subject, ".res") || m.Subject == "fhir.review") {
		return false
	}
	prov := strings.Split(m.Subject, ".")[1]
	event, signoff, normErr := normaliseChecked(prov, 0, m.Data)
	if normErr == nil || event != nil || signoff != nil {
		return false
	}
	matched := false
	_, err := scanAPILog(filepath.Join(run, "prep_log.ndjson"), func(row LogMsg) error {
		var prep Prep
		if row.Subject != "prep" || json.Unmarshal(row.Data, &prep) != nil {
			return fmt.Errorf("invalid PREP rejection evidence")
		}
		if prep.Seq == m.Seq && prep.Skip && prep.Reject != nil && prep.Ev == nil && prep.So == nil && *prep.Reject == *normErr {
			matched = true
		}
		return nil
	})
	return err == nil && matched
}

func materializedCatalogCounts(final map[string]*Decision) map[string]interface{} {
	active := 0
	for _, d := range final {
		if !d.Inactive {
			active++
		}
	}
	return map[string]interface{}{"schema": "wasl-materialized-catalog/1", "retained_cases": len(final), "active_cases": active, "retained_live_replay_mismatch": 0}
}

// checkDeclaredCount validates one declared integer artifact counter against
// the value derived from the actual retained logs.
func checkDeclaredCount(artifact, field string, value interface{}, want int, source string) error {
	n, ok := value.(float64)
	if !ok || math.IsInf(n, 0) || math.IsNaN(n) || n != math.Trunc(n) || n < 0 {
		return fmt.Errorf("serve preflight: %s: invalid JSON: %s must be a non-negative integer", artifact, field)
	}
	if n != float64(want) {
		return fmt.Errorf("serve preflight: %s declares %s=%g but %s holds %d", artifact, field, n, source, want)
	}
	return nil
}

// validateMaterializationMetadata checks the additive materialization schema.
// Metadata is optional for legacy root-only artifacts, mandatory for runs with
// independent CRC care. Every field is checked against actual OUT.
func validateMaterializationMetadata(dashboard, report map[string]interface{}, counts materializationCounts) error {
	var expected map[string]interface{}
	_ = json.Unmarshal(canon(counts), &expected)
	for name, artifact := range map[string]map[string]interface{}{"dashboard.json": dashboard, "report7.json": report} {
		value, present := artifact["materialization"]
		if !present && counts.DerivedCare == 0 {
			continue
		}
		declared, ok := value.(map[string]interface{})
		if !ok || declared["schema"] != materializationSchema {
			return fmt.Errorf("serve preflight: %s: missing or invalid materialization metadata", name)
		}
		for field, want := range expected {
			if field == "schema" {
				continue
			}
			if err := checkDeclaredCount(name, "materialization."+field, declared[field], int(want.(float64)), "out_log.ndjson decisions"); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateDeclaredCounts treats every retained OUT case as root-screening, the
// shape of a run with no independent CRC care children.
func validateDeclaredCounts(dashboard, report map[string]interface{}, loops map[string]string, fhirRecords int, activeCounts ...int) error {
	active := len(loops)
	if len(activeCounts) == 1 {
		active = activeCounts[0]
	}
	return validateDeclaredCountsRoots(dashboard, report, loops, loops, fhirRecords, active, len(activeCounts) == 1)
}

// validateDeclaredCountsRoots validates declared counters. loops maps every
// retained case (root and derived care) to its latest programme; roots maps only
// the root screening/index cases. Native clinical cohort denominators default to
// roots (main's explicit retained-case/active-case versus clinical-cohort split),
// and an explicit materialized catalog separately declares retained and active
// totals without replacing the clinical cohort.
func validateDeclaredCountsRoots(dashboard, report map[string]interface{}, loops, roots map[string]string, fhirRecords, active int, catalogAllowed bool) error {
	check := checkDeclaredCount
	byProgramme := map[string]int{}
	for _, programme := range loops {
		byProgramme[programme]++
	}
	clinicalLoops := len(roots)
	clinicalByProgramme := map[string]int{}
	for _, programme := range roots {
		clinicalByProgramme[programme]++
	}
	if raw, present := report["materialized_catalog"]; present {
		catalog, ok := raw.(map[string]interface{})
		if !ok || catalog["schema"] != "wasl-materialized-catalog/1" || len(catalog) != 4 || !catalogAllowed {
			return fmt.Errorf("serve preflight: invalid materialized catalog counts")
		}
		for field, want := range map[string]int{"retained_cases": len(loops), "active_cases": active, "retained_live_replay_mismatch": 0} {
			if err := check("report7.json", "materialized_catalog."+field, catalog[field], want, "out_log.ndjson decisions"); err != nil {
				return err
			}
		}
		// Retained OUT completeness and the clinical cohort are distinct. Preserve
		// the explicit cohort denominator while checking every native counter.
		if _, declared := report["loops"]; declared {
			declaredByProgramme, ok := report["loops_by_programme"].(map[string]interface{})
			if !ok {
				return fmt.Errorf("serve preflight: materialized catalog requires clinical loops_by_programme")
			}
			clinicalLoops = 0
			clinicalByProgramme = map[string]int{}
			for programme, value := range declaredByProgramme {
				n, ok := value.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n != math.Trunc(n) {
					return fmt.Errorf("serve preflight: invalid clinical loop count for %s", programme)
				}
				clinicalByProgramme[programme] = int(n)
				clinicalLoops += int(n)
			}
		}
	}
	if raw, present := report["loops_by_programme"]; present {
		declared, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("serve preflight: report7.json: invalid loops_by_programme object")
		}
		for pg, value := range declared {
			if err := check("report7.json", "loops_by_programme."+pg, value, clinicalByProgramme[pg], "declared clinical cohort (legacy: out_log.ndjson root screening decisions)"); err != nil {
				return err
			}
		}
		for pg := range clinicalByProgramme {
			if _, present := declared[pg]; !present {
				return fmt.Errorf("serve preflight: report7.json: loops_by_programme.%s missing root count for out_log.ndjson", pg)
			}
		}
	}
	for _, artifact := range []struct {
		name string
		body map[string]interface{}
	}{{"dashboard.json", dashboard}, {"report7.json", report}} {
		for _, field := range []string{"cases", "loops", "events", "fhir_msgs"} {
			want, source := len(loops), "out_log.ndjson decisions"
			if artifact.name == "report7.json" && field == "loops" {
				want, source = clinicalLoops, "declared clinical cohort (legacy: out_log.ndjson root screening decisions)"
			}
			if field == "events" || field == "fhir_msgs" {
				want, source = fhirRecords, "fhir_log.ndjson records"
			}
			if v, present := artifact.body[field]; present {
				if err := check(artifact.name, field, v, want, source); err != nil {
					return err
				}
			}
		}
		raw, present := artifact.body["programmes_live"]
		if !present {
			continue
		}
		programmes, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("serve preflight: %s: programmes_live must be an object", artifact.name)
		}
		expectedByProgramme, expectedTotal := byProgramme, len(loops)
		if artifact.name == "report7.json" {
			expectedByProgramme, expectedTotal = clinicalByProgramme, clinicalLoops
		}
		keys := make([]string, 0, len(programmes))
		for name := range programmes {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		total, declared := 0, false
		for _, name := range keys {
			programme, ok := programmes[name].(map[string]interface{})
			if !ok {
				return fmt.Errorf("serve preflight: %s: programmes_live.%s must be an object", artifact.name, name)
			}
			counted := false
			for _, field := range []string{"loops", "cases"} {
				if v, present := programme[field]; present {
					if err := check(artifact.name, "programmes_live."+name+"."+field, v, expectedByProgramme[name], "declared programme cohort (legacy: out_log.ndjson root screening decisions)"); err != nil {
						return err
					}
					counted = true
				}
			}
			if counted {
				total, declared = total+expectedByProgramme[name], true
			}
		}
		if artifact.name == "report7.json" {
			for pg := range expectedByProgramme {
				programme, _ := programmes[pg].(map[string]interface{})
				_, hasLoops := programme["loops"]
				_, hasCases := programme["cases"]
				if !hasLoops && !hasCases {
					return fmt.Errorf("serve preflight: report7.json: programmes_live.%s missing root count for out_log.ndjson", pg)
				}
			}
		}
		if declared {
			if err := check(artifact.name, "programmes_live declared cases/loops", float64(total), expectedTotal, "declared cohort (legacy: out_log.ndjson root screening decisions)"); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRequiredAPINDJSON(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("serve preflight: required NDJSON %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), apiPreflightMaxNDJSONRecord)
	line := 1
	for scanner.Scan() {
		var record interface{}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("serve preflight: %s:%d: %s NDJSON record: %w", path, line, apiJSONErrorKind(err), err)
		}
		if _, ok := record.(map[string]interface{}); !ok {
			return fmt.Errorf("serve preflight: %s:%d: invalid NDJSON record: expected object", path, line)
		}
		line++
	}
	if err := scanner.Err(); err != nil {
		if strings.Contains(err.Error(), "token too long") {
			return fmt.Errorf("serve preflight: %s:%d: NDJSON record exceeds scanner limit: %w", path, line, err)
		}
		return fmt.Errorf("serve preflight: %s:%d: NDJSON scanner completion: %w", path, line, err)
	}
	return nil
}

// readBookingArtifact strictly decodes the optional booking decision-support document. The returned error wraps
// os.ErrNotExist only when the file is genuinely absent. Any other read failure, malformed or truncated JSON,
// trailing content after the first value, or a top-level value other than an object is rejected and names the path.
// Numbers stay json.Number so generated spellings (notably Python's 2.0) keep exact cross-language version hashes.
func readBookingArtifact(path string) (map[string]interface{}, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("serve preflight: optional JSON %s is present but unreadable: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(contents))
	dec.UseNumber()
	var value interface{}
	if err := dec.Decode(&value); err != nil {
		kind := "malformed"
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			kind = "truncated"
		}
		return nil, fmt.Errorf("serve preflight: %s: %s JSON: %v", path, kind, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("serve preflight: %s: malformed JSON: unexpected content after the top-level value", path)
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("serve preflight: %s: invalid JSON: expected top-level object", path)
	}
	return object, nil
}

func apiJSONErrorKind(err error) string {
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		return "truncated"
	}
	return "malformed"
}
