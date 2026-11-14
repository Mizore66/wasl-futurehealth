package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	loops := map[string]bool{}
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
	fhirRecords, err := scanAPILog(fhirPath, validateFHIRRecord)
	if err != nil {
		return err
	}
	return validateDeclaredCounts(objects["dashboard.json"], objects["report7.json"], counts, projection.final, fhirRecords)
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

func validateOutRecord(m LogMsg, loops map[string]bool) error {
	var env OutEnvelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		return fmt.Errorf("invalid out envelope: %w", err)
	}
	switch {
	case env.Dec != nil:
		if env.Dec.ID == "" || env.Dec.LoopID == "" {
			return fmt.Errorf("invalid out envelope: decision requires id and loop")
		}
		loops[env.Dec.LoopID] = true
	case env.Item != nil:
		if env.Item.ItemID == "" || env.Item.LoopID == "" {
			return fmt.Errorf("invalid out envelope: review item requires item and loop")
		}
	default:
		return fmt.Errorf("invalid out envelope: decision or review item payload is required")
	}
	return nil
}

func validateFHIRRecord(m LogMsg) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(m.Data, &payload); err != nil || len(payload) == 0 {
		return fmt.Errorf("invalid FHIR record: data must be a non-empty object")
	}
	if strings.HasSuffix(m.Subject, ".res") || m.Subject == "fhir.review" {
		var resourceType string
		if json.Unmarshal(payload["resourceType"], &resourceType) != nil || resourceType == "" {
			return fmt.Errorf("invalid FHIR record: resourceType is required")
		}
	}
	return nil
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

func validateDeclaredCounts(dashboard, report map[string]interface{}, counts materializationCounts, final map[string]*Decision, fhirRecords int) error {
	check := func(artifact, field string, value interface{}, want int, source string) error {
		n, ok := value.(float64)
		if !ok || math.IsInf(n, 0) || math.IsNaN(n) || n != math.Trunc(n) || n < 0 {
			return fmt.Errorf("serve preflight: %s: invalid JSON: %s must be a non-negative integer", artifact, field)
		}
		if n != float64(want) {
			return fmt.Errorf("serve preflight: %s declares %s=%g but %s holds %d", artifact, field, n, source, want)
		}
		return nil
	}
	// Metadata is additive for legacy root-only artifacts, mandatory for runs
	// with independent CRC care. Every field is checked against actual OUT.
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
			if err := check(name, "materialization."+field, declared[field], int(want.(float64)), "out_log.ndjson decisions"); err != nil {
				return err
			}
		}
	}
	if v, ok := dashboard["cases"]; ok {
		if err := check("dashboard.json", "cases", v, counts.TotalCurrentMaterialized, "out_log.ndjson decisions"); err != nil {
			return err
		}
	}
	clinicalLoops := counts.RootScreening
	catalogCounts := materializedCatalogCounts(final)
	if raw, present := report["materialized_catalog"]; present {
		catalog, ok := raw.(map[string]interface{})
		if !ok || catalog["schema"] != "wasl-materialized-catalog/1" || len(catalog) != 4 {
			return fmt.Errorf("serve preflight: invalid materialized catalog counts")
		}
		for field, want := range map[string]int{"retained_cases": len(final), "active_cases": catalogCounts["active_cases"].(int), "retained_live_replay_mismatch": 0} {
			if err := check("report7.json", "materialized_catalog."+field, catalog[field], want, "out_log.ndjson decisions"); err != nil {
				return err
			}
		}
		// New reports explicitly distinguish retained OUT completeness from their
		// unchanged clinical truth denominator. Validate both, never substitute one
		// for the other or silently drop an invalid old declaration.
		if _, declared := report["loops"]; declared {
			byProgramme, ok := report["loops_by_programme"].(map[string]interface{})
			if !ok {
				return fmt.Errorf("serve preflight: materialized catalog requires clinical loops_by_programme")
			}
			clinicalLoops = 0
			for programme, value := range byProgramme {
				n, ok := value.(float64)
				if !ok || n < 0 || n != float64(int(n)) {
					return fmt.Errorf("serve preflight: invalid clinical loop count for %s", programme)
				}
				clinicalLoops += int(n)
			}
		}
	}
	if v, ok := report["loops"]; ok {
		if err := check("report7.json", "loops", v, clinicalLoops, "declared clinical cohort (legacy: out_log.ndjson root screening decisions)"); err != nil {
			return err
		}
	}
	if v, ok := report["events"]; ok {
		if err := check("report7.json", "events", v, fhirRecords, "fhir_log.ndjson records"); err != nil {
			return err
		}
	}
	byProgramme := map[string]int{}
	for _, d := range final {
		kind, _ := materializedCaseKind(d)
		if kind == "derived_care" {
			continue
		}
		pg := d.Prog
		if pg == "" {
			pg = "crc"
		}
		byProgramme[pg]++
	}
	if _, catalog := report["materialized_catalog"]; catalog {
		// The native clinical denominator is independently declared by the cohort;
		// materialization and catalog metadata above prove actual retained OUT.
		byProgramme = map[string]int{}
		if declared, ok := report["loops_by_programme"].(map[string]interface{}); ok {
			for pg, value := range declared {
				n, ok := value.(float64)
				if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n != float64(int(n)) {
					return fmt.Errorf("serve preflight: invalid clinical loop count for %s", pg)
				}
				byProgramme[pg] = int(n)
			}
		}
	}
	if raw, present := report["loops_by_programme"]; present {
		declared, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("serve preflight: report7.json: invalid loops_by_programme object")
		}
		for pg, value := range declared {
			if err := check("report7.json", "loops_by_programme."+pg, value, byProgramme[pg], "out_log.ndjson root screening decisions"); err != nil {
				return err
			}
		}
		for pg := range byProgramme {
			if _, present := declared[pg]; !present {
				return fmt.Errorf("serve preflight: report7.json: loops_by_programme.%s missing root count for out_log.ndjson", pg)
			}
		}
	}
	if raw, present := report["programmes_live"]; present {
		programmes, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("serve preflight: report7.json: invalid programmes_live object")
		}
		for pg, raw := range programmes {
			programme, ok := raw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("serve preflight: report7.json: invalid programmes_live.%s", pg)
			}
			for _, field := range []string{"loops", "cases"} {
				if value, present := programme[field]; present {
					if err := check("report7.json", "programmes_live."+pg+"."+field, value, byProgramme[pg], "out_log.ndjson root screening decisions"); err != nil {
						return err
					}
				}
			}
		}
		for pg := range byProgramme {
			programme, _ := programmes[pg].(map[string]interface{})
			_, loops := programme["loops"]
			_, cases := programme["cases"]
			if !loops && !cases {
				return fmt.Errorf("serve preflight: report7.json: programmes_live.%s missing root count for out_log.ndjson", pg)
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

func apiJSONErrorKind(err error) string {
	if strings.Contains(err.Error(), "unexpected end of JSON input") {
		return "truncated"
	}
	return "malformed"
}
