package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const apiPreflightMaxNDJSONRecord = 1 << 24

// validateAPIPreflight rejects incomplete materialized runs before the serve API
// can turn read or scanner failures into apparently healthy empty projections.
func validateAPIPreflight(run string) error {
	for _, name := range []string{"dashboard.json", "report7.json", "anchors.json"} {
		if err := validateRequiredAPIJSON(filepath.Join(run, name)); err != nil {
			return err
		}
	}
	for _, name := range []string{"out_log.ndjson", "fhir_log.ndjson", "reviewer_decisions.ndjson"} {
		if err := validateRequiredAPINDJSON(filepath.Join(run, name)); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredAPIJSON(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("serve preflight: required JSON %s: %w", path, err)
	}
	var value interface{}
	if err := json.Unmarshal(contents, &value); err != nil {
		return fmt.Errorf("serve preflight: %s: %s JSON: %w", path, apiJSONErrorKind(err), err)
	}
	if _, ok := value.(map[string]interface{}); !ok {
		return fmt.Errorf("serve preflight: %s: invalid JSON: expected top-level object", path)
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
