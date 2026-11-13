package main

// Synthetic patient links are provisioned locally, never sent by this command.
// The registry contains only a token hash and opaque routing metadata; the raw
// token is returned to the provisioning caller once and is never logged.

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	patientRegistryFile = "patient_links.ndjson"
	defaultPatientTTL   = 30 * time.Minute
	maxPatientTTL       = 24 * time.Hour
)

var (
	patientHashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
	patientIDRE   = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

type patientLink struct {
	TokenSHA256 string `json:"token_sha256"`
	LinkID      string `json:"link_id"`
	Case        string `json:"case"`
	ExpiresAt   string `json:"expires_at"`
}

func patientRegistryPath(run string) string { return filepath.Join(run, patientRegistryFile) }

func randomPatientID(bytesN int) (string, error) {
	b := make([]byte, bytesN)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func provisionPatientLink(run, caseID string, ttl time.Duration, now time.Time) (string, error) {
	if os.Getenv("WASL_SYNTHETIC_DATA_ONLY") != "1" {
		return "", errors.New("patient-link requires WASL_SYNTHETIC_DATA_ONLY=1")
	}
	if ttl <= 0 || ttl > maxPatientTTL {
		return "", fmt.Errorf("ttl must be greater than zero and at most 24 hours")
	}
	s, err := loadAPIVerified(run, "")
	if err != nil {
		return "", err
	}
	if s.cases[caseID] == nil {
		return "", errors.New("unknown synthetic case")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	linkID, err := randomPatientID(16)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(token))
	rec := patientLink{TokenSHA256: hex.EncodeToString(digest[:]), LinkID: linkID, Case: caseID, ExpiresAt: now.UTC().Add(ttl).Format(time.RFC3339)}
	line, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	path := patientRegistryPath(run)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return "", err
	}
	if err := appendToLedgerFile(f, append(line, '\n')); err != nil {
		return "", err
	}
	return token, nil
}

func readPatientLinks(run string) (map[string]patientLink, error) {
	out := map[string]patientLink{}
	f, err := os.Open(patientRegistryPath(run))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 64<<10)
	ids := map[string]bool{}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			return nil, errors.New("empty patient-link registry line")
		}
		if err := rejectSemanticDuplicateKeys(line); err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var rec patientLink
		if err := dec.Decode(&rec); err != nil {
			return nil, errors.New("invalid patient-link registry entry")
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, errors.New("trailing patient-link registry data")
		}
		expires, err := time.Parse(time.RFC3339, rec.ExpiresAt)
		if err != nil || !patientHashRE.MatchString(rec.TokenSHA256) || !patientIDRE.MatchString(rec.LinkID) || rec.Case == "" || strings.TrimSpace(rec.Case) != rec.Case {
			return nil, errors.New("invalid patient-link registry fields")
		}
		if expires.IsZero() || out[rec.TokenSHA256].LinkID != "" || ids[rec.LinkID] {
			return nil, errors.New("duplicate patient-link registry identity")
		}
		out[rec.TokenSHA256], ids[rec.LinkID] = rec, true
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// patientCredential resolves only the run-local patient registry. Staff bearer
// tokens therefore have no meaning on this route. Expiry is checked here, never
// while replaying historical patient events.
func (s *apiServer) patientCredential(r *http.Request) (patientLink, int) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.Contains(token, " ") {
		return patientLink{}, http.StatusUnauthorized
	}
	digest := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(digest[:])
	links, err := readPatientLinks(s.run)
	if err != nil {
		return patientLink{}, http.StatusUnauthorized
	}
	rec, found := links[key]
	if !found || s.cases[rec.Case] == nil {
		return patientLink{}, http.StatusUnauthorized
	}
	expires, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err != nil {
		return patientLink{}, http.StatusUnauthorized
	}
	if !s.clock().Before(expires) {
		return rec, http.StatusGone
	}
	return rec, 0
}
