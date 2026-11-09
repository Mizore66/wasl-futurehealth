package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	maxReviewBodyBytes  = 1 << 20
	maxConsentBodyBytes = 16 << 10
	maxUndoBodyBytes    = 16 << 10
)

// decodeAPIBody enforces the same trust-boundary rules on every JSON mutation:
// a real byte limit, duplicate-key rejection, an object with known fields only,
// and no trailing JSON or data hidden beyond an artificial EOF.
func decodeAPIBody(w http.ResponseWriter, r *http.Request, limit int64, dst interface{}) error {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		return fmt.Errorf("body unreadable or exceeds %d bytes", limit)
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("body must be one JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("body must contain exactly one JSON object and no trailing data")
	}
	return nil
}
