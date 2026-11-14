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
	"strconv"
	"strings"
)

const identityClusterSchema = "wasl-identity-clusters/1"
const retainedIdentitySchema = "wasl-identity-clusters/2"

type identityCluster struct {
	Root          string   `json:"root"`
	Registrations []string `json:"registrations"`
}

type identityCase struct {
	CaseID       string `json:"case_id"`
	Registration string `json:"registration"`
}

type identityClusterSnapshot struct {
	Schema       string                 `json:"schema"`
	Policy       string                 `json:"policy"`
	FHIRSHA256   string                 `json:"fhir_sha256"`
	OutLogSHA256 string                 `json:"out_log_sha256"`
	Clusters     []identityCluster      `json:"clusters"`
	Cases        []identityCase         `json:"cases,omitempty"`
	Catalog      []retainedIdentityCase `json:"catalog,omitempty"`
}

// /2 separates membership, source history and clinical eligibility. An unavailable
// association contains no registration or selected request body. Decision references
// are case-local, immutable OUT identities, not a second clinical decision stream.
type retainedAssociation struct {
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Registration   string `json:"registration,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	RequestVersion int    `json:"request_version,omitempty"`
	RequestSHA256  string `json:"request_sha256,omitempty"`
	RequestSeq     uint64 `json:"request_seq,omitempty"`
	BindingSeq     uint64 `json:"binding_seq,omitempty"`
}

type retainedDecisionContext struct {
	DecisionID       string              `json:"decision_id"`
	DecisionSig      string              `json:"decision_sig"`
	FactsSeq         uint64              `json:"facts_seq"`
	Association      retainedAssociation `json:"association"`
	Transition       retainedAssociation `json:"transition"`
	OriginDecisionID string              `json:"origin_decision_id,omitempty"`
	ParentDecisionID string              `json:"parent_decision_id,omitempty"`
}

type retainedIdentityCase struct {
	CaseID        string                    `json:"case_id"`
	ParentCaseID  string                    `json:"parent_case_id,omitempty"`
	Lifecycle     string                    `json:"lifecycle"`
	Participation string                    `json:"participation"`
	Current       retainedAssociation       `json:"current"`
	History       []retainedDecisionContext `json:"history"`
}

// This passive observer is called AFTER the canonical request fold, before the
// next input can overwrite its selected revision. It never scores, unions or
// selects a request. Source verification below reuses foldTrackingRequest itself;
// there is deliberately no alternate request-version/conflict resolver.
type retainedEvidence struct {
	selected  map[string]retainedAssociation
	conflicts map[string]bool
}

func newRetainedEvidence() *retainedEvidence {
	return &retainedEvidence{selected: map[string]retainedAssociation{}, conflicts: map[string]bool{}}
}

func unavailableAssociation(reason string) retainedAssociation {
	return retainedAssociation{Status: "unavailable", Reason: reason}
}

func requestContentSHA(e *Event) string {
	normalized := *e
	normalized.Version = requestVersion(e)
	return sha(canon(&normalized))
}

func requestConflictKey(loop string, version int) string {
	return fmt.Sprintf("%s@%d", loop, version)
}

func (e *retainedEvidence) observe(L *LinkerCore, p *Prep) {
	if p == nil || p.Ev == nil || p.Ev.RType != "ServiceRequest" {
		return
	}
	loop := p.Ev.Provider + "/ServiceRequest/" + p.Ev.ID
	r := L.requests[loop]
	if r == nil {
		return
	}
	if r.conflict {
		e.conflicts[requestConflictKey(loop, r.version)] = true
		e.selected[loop] = unavailableAssociation("conflicting_request")
		return
	}
	digest := requestContentSHA(r.event)
	old := e.selected[loop]
	if old.RequestSHA256 == digest {
		return
	}
	// A stale or quarantined input cannot be evidence for the selected revision.
	if requestContentSHA(p.Ev) != digest {
		return
	}
	a := retainedAssociation{Status: "resolved", RequestID: loop, RequestVersion: r.version, RequestSHA256: digest, RequestSeq: p.Seq}
	switch {
	case r.rec < 0:
		a = unavailableAssociation("unbound_request")
	case r.event.Prog == "":
		a = unavailableAssociation("unknown_programme")
	default:
		a.Registration = L.recs[r.rec].Pk
		if r.event.PatientKey == a.Registration {
			a.BindingSeq = p.Seq
		} else if r.event.PatientKey == "" && old.Status == "resolved" && old.Registration == a.Registration && !e.conflicts[requestConflictKey(loop, old.RequestVersion)] {
			a.BindingSeq = old.BindingSeq
		} else {
			a = unavailableAssociation("unproven_binding")
		}
	}
	e.selected[loop] = a
}

func (e *retainedEvidence) association(loop string) retainedAssociation {
	a, ok := e.selected[loop]
	if !ok {
		return unavailableAssociation("unproven_binding")
	}
	if a.Status == "resolved" && e.conflicts[requestConflictKey(loop, a.RequestVersion)] {
		return unavailableAssociation("conflicting_request")
	}
	return a
}

func (e *retainedEvidence) historical(a retainedAssociation) retainedAssociation {
	if a.Status == "resolved" && e.conflicts[requestConflictKey(a.RequestID, a.RequestVersion)] {
		return unavailableAssociation("conflicting_request")
	}
	return a
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
	if rp.retained != nil {
		doc.Schema = retainedIdentitySchema
		doc.Cases = nil
		doc.Catalog, err = buildRetainedCatalog(run, rp, canonical)
		if err != nil {
			return nil, err
		}
		return doc, nil
	}
	// Preserve the legacy in-memory active-only exporter contract. Canonical
	// replay always supplies retained evidence; it may never downgrade to /1.
	if len(rp.outs) != 0 {
		return nil, fmt.Errorf("missing retained replay context")
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

// retainedOutDecisions reads the actual materialized OUT, including inactive
// roots and children. Active Final() is not a completeness denominator.
func retainedOutDecisions(run string) (map[string][]*Decision, error) {
	cases := map[string][]*Decision{}
	seen := map[string]bool{}
	_, err := scanAPILog(filepath.Join(run, "out_log.ndjson"), func(m LogMsg) error {
		if err := validateOutRecord(m, map[string]bool{}); err != nil {
			return err
		}
		d := decodeEnv(m).Dec
		if d == nil {
			return nil
		}
		if seen[d.ID] {
			return fmt.Errorf("duplicate OUT decision")
		}
		seen[d.ID] = true
		cases[d.LoopID] = append(cases[d.LoopID], d)
		return nil
	})
	return cases, err
}

// A head can recur in equivalent signed decisions. Resolve it to the latest
// actual immediate-parent emission at or before the child's emission, never a
// future decision or an arbitrary same-root signature. The head may be a signed
// prefix: the clinical producer appends further blocks after opening a child.
func retainedSignedParent(d *Decision, cases map[string][]*Decision) (*Decision, error) {
	// CRC care uses its typed, source-bound signed plan through this same
	// resolver; no ID spelling, candidate matching or second identity inference.
	if d.CRC != nil && d.CRC.Parent != "" {
		if !crcChildSourceIdentity(d) {
			return nil, fmt.Errorf("invalid CRC child source identity")
		}
		var parent *Decision
		for _, pd := range cases[d.CRC.Parent] {
			if !pd.Inactive && pd.InSeq <= d.InSeq && crcChildMatchesPlan(d, pd) && (parent == nil || pd.InSeq >= parent.InSeq) {
				parent = pd
			}
		}
		if parent == nil {
			return nil, fmt.Errorf("missing source-bound CRC parent")
		}
		return parent, nil
	}
	var parent *Decision
	opened := 0
	for _, b := range d.Chain {
		if b.Type != "surveillance_opened" {
			continue
		}
		opened++
		p, _ := b.Body["parent"].(string)
		head, _ := b.Body["parent_head"].(string)
		if p == d.LoopID || baseLoop(p) != baseLoop(d.LoopID) || head == "" {
			return nil, fmt.Errorf("invalid canonical surveillance parent")
		}
		for _, pd := range cases[p] {
			if pd.Inactive || pd.InSeq > d.InSeq {
				continue
			}
			for _, pb := range pd.Chain {
				if pb.Hash == head && (parent == nil || pd.InSeq >= parent.InSeq) {
					parent = pd
				}
			}
		}
	}
	if opened != 1 || parent == nil {
		return nil, fmt.Errorf("missing or ambiguous signed surveillance parent")
	}
	return parent, nil
}

func retainedRoot(id string, cases map[string][]*Decision) string {
	seen := map[string]bool{}
	for !seen[id] {
		seen[id] = true
		history := cases[id]
		if len(history) == 0 {
			return ""
		}
		parent := materializedParent(history[len(history)-1])
		if parent == id {
			return id
		}
		id = parent
	}
	return ""
}

func retainedParent(id string, cases map[string][]*Decision) (string, error) {
	history := cases[id]
	if len(history) == 0 {
		return "", fmt.Errorf("missing case history")
	}
	if materializedParent(history[len(history)-1]) == id {
		return "", nil
	}
	parent := ""
	for _, d := range cases[id] {
		if d.Inactive {
			continue
		}
		pd, err := retainedSignedParent(d, cases)
		if err != nil || (parent != "" && parent != pd.LoopID) {
			return "", fmt.Errorf("invalid canonical surveillance parent")
		}
		parent = pd.LoopID
	}
	if parent == "" {
		return "", fmt.Errorf("missing canonical surveillance parent")
	}
	return parent, nil
}

// Participation is independent of clinical lifecycle. A stored active child
// participates only if its own producing context and every signed immediate
// parent context lead to the FINAL trusted cluster of the final selected root.
// This function is also used AFTER source validation by the reader; missing or
// forged contexts are errors, never permission to omit an active mapping.
func retainedParticipation(row retainedIdentityCase, rows map[string]retainedIdentityCase, contexts map[string]retainedDecisionContext, canonical map[string]string, selected retainedAssociation) (string, retainedAssociation, error) {
	if row.Lifecycle == "retained" {
		return "reference_only", unavailableAssociation("inactive_request"), nil
	}
	if row.ParentCaseID == "" {
		if selected.Status != "resolved" || canonical[selected.Registration] == "" {
			return "", retainedAssociation{}, fmt.Errorf("active root has no proven current association")
		}
		return "current", selected, nil
	}
	last := row.History[len(row.History)-1]
	origin := last.Association
	rootCluster := canonical[selected.Registration]
	continuous := selected.Status == "resolved" && rootCluster != ""
	seen := map[string]bool{}
	ctx := last
	id := row.CaseID
	for {
		if ctx.Association.Status != "resolved" || canonical[ctx.Association.Registration] != rootCluster || rootCluster == "" {
			continuous = false
		}
		if row, ok := rows[id]; ok && row.ParentCaseID == "" {
			break
		}
		if seen[id] || ctx.ParentDecisionID == "" {
			return "", retainedAssociation{}, fmt.Errorf("missing or cyclic child lineage")
		}
		seen[id] = true
		parent, ok := rows[id]
		if !ok || parent.ParentCaseID == "" {
			return "", retainedAssociation{}, fmt.Errorf("missing child catalog lineage")
		}
		id = parent.ParentCaseID
		var exists bool
		ctx, exists = contexts[ctx.ParentDecisionID]
		if !exists {
			return "", retainedAssociation{}, fmt.Errorf("missing signed parent context")
		}
	}
	if !continuous {
		return "reference_only", unavailableAssociation("detached_child"), nil
	}
	return "current", origin, nil
}

func buildRetainedCatalog(run string, rp *replayOut, canonical map[string]string) ([]retainedIdentityCase, error) {
	actual, err := retainedOutDecisions(run)
	if err != nil {
		return nil, err
	}
	expected := map[string]*Decision{}
	for _, o := range rp.outs {
		if o.Dec != nil {
			if expected[o.Dec.ID] != nil {
				return nil, fmt.Errorf("duplicate replay decision identity")
			}
			expected[o.Dec.ID] = o.Dec
		}
	}
	catalog := []retainedIdentityCase{}
	matched := 0
	for id, history := range actual {
		last := history[len(history)-1]
		row := retainedIdentityCase{CaseID: id, Lifecycle: "active", Current: rp.retained.association(retainedRoot(id, actual)), History: []retainedDecisionContext{}}
		parent, err := retainedParent(id, actual)
		if err != nil {
			return nil, err
		}
		row.ParentCaseID = parent
		if last.Inactive {
			row.Lifecycle = "retained"
			row.Current = unavailableAssociation("inactive_request")
		}
		if row.Current.Status == "resolved" && canonical[row.Current.Registration] == "" {
			return nil, fmt.Errorf("current association missing canonical membership")
		}
		if !last.Inactive && parent == "" && row.Current.Status != "resolved" {
			return nil, fmt.Errorf("active root %q has no proven current association", id)
		}
		for _, d := range history {
			want := expected[d.ID]
			if want == nil || string(canon(want)) != string(canon(d)) || d.Sig != decSig(d) || !verifyChain(d.Chain) {
				return nil, fmt.Errorf("retained live/replay decision mismatch: %q", d.ID)
			}
			context, ok := rp.contexts[d.ID]
			if !ok || context.FactsSeq == 0 || context.DecisionSig != d.Sig {
				return nil, fmt.Errorf("missing same-case replay context: %q", d.ID)
			}
			if parent != "" {
				origin := expected[context.OriginDecisionID]
				if origin == nil || origin.Inactive || origin.LoopID != id {
					return nil, fmt.Errorf("missing child clinical origin")
				}
				pd, err := retainedSignedParent(origin, actual)
				if err != nil {
					return nil, err
				}
				context.ParentDecisionID = pd.ID
			}
			context.Association = rp.retained.historical(context.Association)
			context.Transition = rp.retained.historical(context.Transition)
			if context.Association.Status == "resolved" && canonical[context.Association.Registration] == "" {
				return nil, fmt.Errorf("history association missing canonical membership")
			}
			row.History = append(row.History, context)
			matched++
		}
		catalog = append(catalog, row)
	}
	if matched != len(expected) {
		return nil, fmt.Errorf("materialized retained catalog differs from replay")
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].CaseID < catalog[j].CaseID })
	rows := map[string]retainedIdentityCase{}
	contexts := map[string]retainedDecisionContext{}
	for _, row := range catalog {
		rows[row.CaseID] = row
		for _, ctx := range row.History {
			contexts[ctx.DecisionID] = ctx
		}
	}
	for i := range catalog {
		row := &catalog[i]
		row.Participation, row.Current, err = retainedParticipation(*row, rows, contexts, canonical, rp.retained.association(retainedRoot(row.CaseID, actual)))
		if err != nil {
			return nil, err
		}
	}
	if err := validateRetainedSources(run, catalog, actual, canonical); err != nil {
		return nil, err
	}
	return catalog, nil
}

// validateRetainedSources checks provenance with the EXISTING canonical request
// fold, not raw-last-arrival maps, candidate links or identity inference. The
// temporary linker has no scoring/ANN/union state and is used only by that fold.
// Present person membership remains exclusively the snapshot's trusted Clusters.
func validateRetainedSources(run string, catalog []retainedIdentityCase, actual map[string][]*Decision, canonical map[string]string) error {
	type sourceState struct {
		inactive  bool
		status    string
		version   int
		conflict  bool
		programme string
	}
	states := map[uint64]map[string]sourceState{}
	wanted := map[uint64]map[string]retainedAssociation{}
	for _, row := range catalog {
		for _, ctx := range row.History {
			if wanted[ctx.FactsSeq] == nil {
				wanted[ctx.FactsSeq] = map[string]retainedAssociation{}
			}
			wanted[ctx.FactsSeq][retainedRoot(row.CaseID, actual)] = retainedAssociation{}
		}
	}
	L := &LinkerCore{requests: map[string]*trackingRevision{}, loops: map[string]*LoopFacts{}, loopRec: map[string]int{}, Stats: map[string]int{}}
	evidence := newRetainedEvidence()
	registrations := map[string]int{}
	lastSeq := uint64(0)
	_, err := scanAPILog(filepath.Join(run, "fhir_log.ndjson"), func(m LogMsg) error {
		if m.Seq <= lastSeq {
			return fmt.Errorf("non-monotonic FHIR source sequence")
		}
		lastSeq = m.Seq
		if strings.HasSuffix(m.Subject, ".res") {
			parts := strings.Split(m.Subject, ".")
			if len(parts) != 3 {
				return fmt.Errorf("invalid FHIR source subject")
			}
			arr, parseErr := strconv.ParseFloat(m.Hdr["Wsi-Arr"], 64)
			if parseErr != nil {
				arr = 0
			}
			ev, _, normErr := normaliseChecked(parts[1], arr, m.Data)
			if normErr == nil && ev != nil && ev.RType == "ServiceRequest" {
				loop := ev.Provider + "/ServiceRequest/" + ev.ID
				rec := -1
				if ev.PatientKey != "" {
					var ok bool
					rec, ok = registrations[ev.PatientKey]
					if !ok {
						rec = len(L.recs)
						registrations[ev.PatientKey] = rec
						L.recs = append(L.recs, &Rec{Pk: ev.PatientKey, evs: map[string]*Event{}})
					}
				} else if prior := L.requests[loop]; prior != nil {
					rec = prior.rec
				}
				L.foldTrackingRequest(ev, rec)
				evidence.observe(L, &Prep{Seq: m.Seq, Ev: ev})
			}
		}
		for loop := range wanted[m.Seq] {
			wanted[m.Seq][loop] = evidence.association(loop)
			if states[m.Seq] == nil {
				states[m.Seq] = map[string]sourceState{}
			}
			if r := L.requests[loop]; r != nil {
				state := sourceState{inactive: L.loops[loop] == nil, status: r.event.Status, version: r.version, conflict: r.conflict, programme: r.event.Prog}
				if r.conflict {
					state.status = "conflict"
					state.programme = ""
				}
				states[m.Seq][loop] = state
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	rows := map[string]retainedIdentityCase{}
	contexts := map[string]retainedDecisionContext{}
	for _, row := range catalog {
		rows[row.CaseID] = row
		for _, ctx := range row.History {
			contexts[ctx.DecisionID] = ctx
		}
	}
	for _, row := range catalog {
		root := retainedRoot(row.CaseID, actual)
		if row.Lifecycle == "active" && L.loops[root] == nil {
			return fmt.Errorf("active catalog lacks an active source request")
		}
		if row.Lifecycle == "retained" && root == row.CaseID && L.loops[root] != nil {
			return fmt.Errorf("retired catalog contradicts current source request")
		}
		history := actual[row.CaseID]
		var origin *Decision
		for i, ctx := range row.History {
			d := history[i]
			source, ok := wanted[ctx.FactsSeq][root]
			if !ok || source.Status == "" || ctx.FactsSeq > d.InSeq || ctx.Transition != evidence.historical(source) {
				return fmt.Errorf("invalid transition request provenance: %q", ctx.DecisionID)
			}
			association := evidence.historical(source)
			if root == row.CaseID {
				if ctx.OriginDecisionID != "" || ctx.ParentDecisionID != "" {
					return fmt.Errorf("root cannot borrow child provenance")
				}
			} else {
				if !d.Inactive && !(d.CRC != nil && d.CRC.Withdrawn) {
					origin = d
				}
				if origin == nil || ctx.OriginDecisionID != origin.ID {
					return fmt.Errorf("invalid child clinical origin")
				}
				pd, err := retainedSignedParent(origin, actual)
				if err != nil || ctx.ParentDecisionID != pd.ID {
					return fmt.Errorf("invalid signed immediate-parent context")
				}
				originContext, exists := contexts[origin.ID]
				if !exists {
					return fmt.Errorf("missing child origin context")
				}
				association = evidence.historical(wanted[originContext.FactsSeq][root])
			}
			if ctx.Association != association {
				return fmt.Errorf("invalid clinical-origin association: %q", ctx.DecisionID)
			}
			state := states[ctx.FactsSeq][root]
			if state.version == 0 || d.Inactive != state.inactive || (d.Prog != state.programme && !(d.Inactive && row.ParentCaseID != "" && d.CRC != nil && d.Prog == "crc")) || (d.Inactive && (d.RequestStatus != state.status || d.RequestVersion != state.version || d.RequestConflict != state.conflict)) {
				return fmt.Errorf("decision lifecycle contradicts source context")
			}
			if strings.Contains(d.ID, "@"+root+"@") && !strings.HasSuffix(d.ID, "@"+utoa(ctx.FactsSeq)) {
				return fmt.Errorf("wrong decision facts sequence")
			}
		}
	}
	// Only positively source- and signed-lineage-validated rows reach this step.
	for _, row := range catalog {
		participation, current, err := retainedParticipation(row, rows, contexts, canonical, evidence.association(retainedRoot(row.CaseID, actual)))
		if err != nil || row.Current != current || row.Participation != participation {
			return fmt.Errorf("invalid current participation/provenance: %q", row.CaseID)
		}
	}
	return nil
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
