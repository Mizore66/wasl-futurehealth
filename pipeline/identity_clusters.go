package main

// Read-only materialisation of the canonical linker's final trusted clusters.
// This file deliberately contains no candidate scoring, review, or merge logic.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const identityClusterSchema = "wasl-identity-clusters/1"

type identityCluster struct {
	Root          string   `json:"root"`
	Registrations []string `json:"registrations"`
}

type identityCase struct {
	CaseID       string `json:"case_id"`
	Registration string `json:"registration"`
}

type identityClusterSnapshot struct {
	Schema       string            `json:"schema"`
	Policy       string            `json:"policy"`
	FHIRSHA256   string            `json:"fhir_sha256"`
	OutLogSHA256 string            `json:"out_log_sha256"`
	Clusters     []identityCluster `json:"clusters"`
	Cases        []identityCase    `json:"cases"`
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// personIDForRegistrations is stable when union-find chooses a different root.
func personIDForRegistrations(registrations []string) string {
	sorted := append([]string{}, registrations...)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte("wasl-person-v1\x00" + strings.Join(sorted, "\x00")))
	return "person_" + hex.EncodeToString(h[:])
}

func buildIdentityClusters(run string, rp *replayOut) (*identityClusterSnapshot, error) {
	if rp == nil || rp.L == nil {
		return nil, fmt.Errorf("canonical replay linker is unavailable")
	}
	fhirHash, err := fileSHA256(filepath.Join(run, "fhir_log.ndjson"))
	if err != nil {
		return nil, fmt.Errorf("hash FHIR log: %w", err)
	}
	outHash, err := fileSHA256(filepath.Join(run, "out_log.ndjson"))
	if err != nil {
		return nil, fmt.Errorf("hash OUT log: %w", err)
	}
	canonical := rp.L.Clusters()
	byRoot := map[string][]string{}
	for registration, root := range canonical {
		if registration == "" || root == "" {
			return nil, fmt.Errorf("canonical linker returned an empty registration or root")
		}
		byRoot[root] = append(byRoot[root], registration)
	}
	roots := make([]string, 0, len(byRoot))
	for root := range byRoot {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	doc := &identityClusterSnapshot{Schema: identityClusterSchema, Policy: PolicyName, FHIRSHA256: fhirHash, OutLogSHA256: outHash,
		Clusters: []identityCluster{}, Cases: []identityCase{}}
	for _, root := range roots {
		registrations := byRoot[root]
		sort.Strings(registrations)
		doc.Clusters = append(doc.Clusters, identityCluster{Root: root, Registrations: registrations})
	}
	for caseID, loop := range rp.L.loops {
		if loop == nil || loop.SRRec == "" {
			return nil, fmt.Errorf("loop %q has no referral registration", caseID)
		}
		if _, ok := canonical[loop.SRRec]; !ok {
			return nil, fmt.Errorf("loop %q referral registration is absent from canonical clusters", caseID)
		}
		doc.Cases = append(doc.Cases, identityCase{CaseID: caseID, Registration: loop.SRRec})
	}
	sort.Slice(doc.Cases, func(i, j int) bool { return doc.Cases[i].CaseID < doc.Cases[j].CaseID })
	return doc, nil
}

// writeIdentityClusters writes one complete snapshot or leaves any previous snapshot untouched.
func writeIdentityClusters(run string, rp *replayOut) error {
	doc, err := buildIdentityClusters(run, rp)
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	tmp, err := os.CreateTemp(run, ".identity_clusters-*.tmp")
	if err != nil {
		return fmt.Errorf("create identity snapshot: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(run, "identity_clusters.json")); err != nil {
		return fmt.Errorf("install identity snapshot: %w", err)
	}
	return nil
}
