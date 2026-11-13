package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
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
	if _, err := scanAPILog(outPath, func(m LogMsg) error { return validateOutRecord(m, loops) }); err != nil {
		return err
	}
	fhirRecords, err := scanAPILog(fhirPath, validateFHIRRecord)
	if err != nil {
		return err
	}
	return validateDeclaredCounts(objects["dashboard.json"], objects["report7.json"], len(loops), fhirRecords)
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

func validateDeclaredCounts(dashboard, report map[string]interface{}, loops, fhirRecords int) error {
	check := func(artifact, field string, value interface{}, want int, source string) error {
		n, ok := value.(float64)
		if !ok || n != float64(int(n)) || n < 0 {
			return fmt.Errorf("serve preflight: %s: invalid JSON: %s must be a non-negative integer", artifact, field)
		}
		if int(n) != want {
			return fmt.Errorf("serve preflight: %s declares %s=%d but %s holds %d", artifact, field, int(n), source, want)
		}
		return nil
	}
	if v, ok := dashboard["cases"]; ok {
		if err := check("dashboard.json", "cases", v, loops, "out_log.ndjson decisions"); err != nil {
			return err
		}
	}
	if v, ok := report["loops"]; ok {
		if err := check("report7.json", "loops", v, loops, "out_log.ndjson decisions"); err != nil {
			return err
		}
	}
	if v, ok := report["events"]; ok {
		if err := check("report7.json", "events", v, fhirRecords, "fhir_log.ndjson records"); err != nil {
			return err
		}
	}
	if programmes, ok := report["programmes_live"].(map[string]interface{}); ok {
		total, declared := 0.0, false
		for _, p := range programmes {
			programme, _ := p.(map[string]interface{})
			if v, ok := programme["cases"]; ok {
				n, isNum := v.(float64)
				if !isNum {
					return fmt.Errorf("serve preflight: report7.json: invalid JSON: programmes_live cases must be a non-negative integer")
				}
				total, declared = total+n, true
			}
		}
		if declared {
			return check("report7.json", "programmes_live cases", total, loops, "out_log.ndjson decisions")
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
