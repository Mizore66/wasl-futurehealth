package main

import (
	"encoding/json"
	"fmt"
)

const materializationSchema = "wasl-materialization/1"

// Index/root counts retain the cohort/evaluation denominator, including any
// pre-existing programme follow-up rows in that cohort. CRC care children are
// independently materialized cases, not additional screening observations.
// Withdrawn children remain current review projections; they are not deleted.
type materializationCounts struct {
	Schema                   string `json:"schema"`
	RootScreening            int    `json:"root_screening"`
	DerivedCare              int    `json:"derived_care"`
	TotalCurrentMaterialized int    `json:"total_current_materialized"`
	WithdrawnDerivedCare     int    `json:"withdrawn_derived_care"`
	ScreeningComplete        int    `json:"screening_complete"`
	OngoingCareCases         int    `json:"ongoing_care_cases"`
}

func crcCareKind(disposition string) string {
	switch disposition {
	case "closed_to_mdt":
		return "crc-treatment"
	case "closed_surveillance":
		return "crc-surveillance"
	case "closed_to_gastro":
		return "crc-gastroenterology"
	case "closed_to_specialist":
		return "crc-specialist"
	}
	return ""
}

// Classification is based on typed metadata, never the spelling of a loop ID.
func materializedCaseKind(d *Decision) (string, error) {
	if d == nil {
		return "", fmt.Errorf("nil materialized decision")
	}
	cd := d.CRC
	if cd == nil {
		return "root_screening", nil
	}
	if cd.Kind == "screening" && cd.Parent == "" && cd.ParentHead == "" && cd.ObligationID == "" && !cd.Withdrawn {
		return "root_screening", nil
	}
	if d.Prog != "crc" || cd.Schema != CRCPlanSchema || cd.Kind != crcCareKind(cd.Disposition) || cd.Kind == "" ||
		cd.Parent == "" || cd.Parent == d.LoopID || cd.ParentHead == "" || cd.SourceReport == "" || cd.SourceVersion < 0 ||
		cd.ObligationID == "" || cd.Confirmation == "" || cd.SignedHead == "" || cd.Reviewer == "" {
		return "", fmt.Errorf("invalid CRC derived-care identity for %s", d.LoopID)
	}
	return "derived_care", nil
}

func screeningComplete(d *Decision) bool {
	if d.Inactive {
		return false
	}
	if d.CRC != nil {
		return d.CRC.ScreeningComplete
	}
	switch d.State {
	case "closed", "no_followup_needed", "no_referral_needed", "no_recall_needed":
		return true
	}
	return false
}

func ongoingCare(d *Decision) bool {
	return !d.Inactive && d.CRC != nil && !d.CRC.Withdrawn && d.CRC.CareState != "" && d.CRC.CareState != "complete" && d.CRC.CareState != "accepted_or_first_appointment"
}

// CRC children inherit the explicitly declared parent. Other, older programmes
// retain their existing baseLoop contract; CRC never falls back to ID guessing.
func materializedParent(d *Decision) string {
	if d != nil && d.CRC != nil {
		if d.CRC.Parent == "" {
			return d.LoopID
		}
		return d.CRC.Parent
	}
	if d == nil {
		return ""
	}
	return baseLoop(d.LoopID)
}

func countMaterializedCases(final map[string]*Decision) (materializationCounts, error) {
	counts := materializationCounts{Schema: materializationSchema}
	for id, d := range final {
		kind, err := materializedCaseKind(d)
		if err != nil {
			return counts, err
		}
		if d.LoopID != id {
			return counts, fmt.Errorf("materialized case key disagrees with decision loop %s", id)
		}
		counts.TotalCurrentMaterialized++
		if kind == "derived_care" {
			parent := final[d.CRC.Parent]
			if parent == nil || parent.Prog != "crc" || parent.CRC != nil && parent.CRC.Kind != "screening" {
				return counts, fmt.Errorf("orphan CRC derived-care case %s", id)
			}
			counts.DerivedCare++
			if d.CRC.Withdrawn {
				counts.WithdrawnDerivedCare++
			}
		} else {
			counts.RootScreening++
			if screeningComplete(d) {
				counts.ScreeningComplete++
			}
		}
		if ongoingCare(d) {
			counts.OngoingCareCases++
		}
	}
	return counts, nil
}

func (s *apiServer) materializationCounts() materializationCounts {
	final := map[string]*Decision{}
	for id, c := range s.cases {
		if c.last != nil {
			final[id] = c.last
		}
	}
	return materializationMetadata(final)
}

func materializationMetadata(final map[string]*Decision) materializationCounts {
	counts, err := countMaterializedCases(final)
	if err != nil {
		panic(err)
	}
	return counts
}

// One canonical highest-output-sequence reducer is shared by reporting and serve
// preflight. Equal-sequence duplicates must be identical, not arrival-order wins.
type materializedDecisions struct {
	final   map[string]*Decision
	seq     map[string]uint64
	history map[string][]*Decision
	seen    map[string]map[uint64]string
}

func projectMaterializedDecisions(logs []LogMsg) (*materializedDecisions, materializationCounts, error) {
	projection := newMaterializedDecisions()
	for _, m := range logs {
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return nil, materializationCounts{}, err
		}
		if err := projection.add(m, env); err != nil {
			return nil, materializationCounts{}, err
		}
	}
	counts, err := projection.validate()
	return projection, counts, err
}

func newMaterializedDecisions() *materializedDecisions {
	return &materializedDecisions{final: map[string]*Decision{}, seq: map[string]uint64{}, history: map[string][]*Decision{}, seen: map[string]map[uint64]string{}}
}

func (p *materializedDecisions) add(m LogMsg, env OutEnvelope) error {
	d := env.Dec
	if d == nil {
		return nil
	}
	if _, err := materializedCaseKind(d); err != nil {
		return err
	}
	seq := env.OSeq
	if seq == 0 { // legacy envelopes use the immutable stream sequence
		seq = m.Seq
	}
	if p.seen[d.LoopID] == nil {
		p.seen[d.LoopID] = map[uint64]string{}
	}
	body := sha(canon(d))
	if previous, exists := p.seen[d.LoopID][seq]; exists {
		if previous != body {
			return fmt.Errorf("conflicting OUT decisions at sequence %d for %s", seq, d.LoopID)
		}
		return nil
	}
	p.seen[d.LoopID][seq] = body
	if d.CRC != nil || d.Prog == "crc" { // include legacy CRC roots without plan metadata
		p.history[d.LoopID] = append(p.history[d.LoopID], d)
	}
	if p.final[d.LoopID] == nil || seq > p.seq[d.LoopID] {
		p.final[d.LoopID], p.seq[d.LoopID] = d, seq
	}
	return nil
}

func crcChildMatchesPlan(child, parent *Decision) bool {
	if parent == nil || parent.CRC == nil || parent.CRC.Kind != "screening" || !verifyChain(parent.Chain) {
		return false
	}
	cd := child.CRC
	for _, plan := range parent.CRC.Obligations {
		if plan.ID == cd.ObligationID && plan.ChildLoop == child.LoopID && plan.SourceReport == cd.SourceReport &&
			plan.SourceVersion == cd.SourceVersion && plan.Disposition == cd.Disposition && plan.Confirmation == cd.Confirmation &&
			plan.ExpectedHead == cd.SignedHead && plan.Reviewer == cd.Reviewer {
			for _, block := range parent.Chain {
				if block.Type != "crc_plan_obligation" {
					continue
				}
				var evidence struct {
					Plan  CRCPlanItem `json:"plan"`
					Chain []Block     `json:"evidence_chain"`
				}
				if json.Unmarshal(canon(block.Body), &evidence) == nil && string(canon(evidence.Plan)) == string(canon(plan)) &&
					len(evidence.Chain) > 0 && evidence.Chain[len(evidence.Chain)-1].Hash == cd.ParentHead && verifyChain(evidence.Chain) {
					return true
				}
			}
		}
	}
	return false
}

func crcChildSourceIdentity(d *Decision) bool {
	if len(d.Chain) == 0 || d.Chain[0].Type != "crc_child_opened" || !verifyChain(d.Chain) {
		return false
	}
	var opened struct {
		Loop, Parent  string
		ParentHead    string `json:"parent_head"`
		SourceReport  string `json:"source_report"`
		SourceVersion int    `json:"source_version"`
		Disposition   string
	}
	cd := d.CRC
	return json.Unmarshal(canon(d.Chain[0].Body), &opened) == nil && opened.Loop == d.LoopID && opened.Parent == cd.Parent && opened.ParentHead == cd.ParentHead && opened.SourceReport == cd.SourceReport && opened.SourceVersion == cd.SourceVersion && opened.Disposition == cd.Disposition
}

// Active children must match the current parent's source-bound plan. Retired
// children must match a historical signed plan, which remains in append-only OUT.
// Conversely every declared child must exist: counts alone cannot detect a lost
// child replaced by an unrelated record.
func (p *materializedDecisions) validate() (materializationCounts, error) {
	counts, err := countMaterializedCases(p.final)
	if err != nil {
		return counts, err
	}
	// Validate all child records, not only their latest projection: a later
	// correction cannot conceal forged historical child/source metadata.
	for _, history := range p.history {
		for _, d := range history {
			if !verifyChain(d.Chain) {
				return counts, fmt.Errorf("CRC case %s: original provenance digest is invalid or unsupported legacy representation", d.LoopID)
			}
			if d.CRC == nil || d.CRC.Parent == "" {
				continue
			}
			matched := false
			for _, parent := range p.history[d.CRC.Parent] {
				if crcChildMatchesPlan(d, parent) {
					matched = true
					break
				}
			}
			if !matched || !crcChildSourceIdentity(d) {
				return counts, fmt.Errorf("CRC child %s has no matching source-bound parent history", d.LoopID)
			}
		}
	}
	for _, d := range p.final {
		if cd := d.CRC; cd != nil && cd.Parent != "" && !cd.Withdrawn && !d.Inactive && !crcChildMatchesPlan(d, p.final[cd.Parent]) {
			return counts, fmt.Errorf("CRC child %s has no matching current source-bound parent plan", d.LoopID)
		}
		if d.CRC == nil || d.CRC.Parent != "" {
			continue
		}
		declared := map[string]bool{}
		for _, plan := range d.CRC.Obligations {
			if plan.ChildLoop != "" {
				declared[plan.ChildLoop] = true
				child := p.final[plan.ChildLoop]
				if child == nil || child.CRC == nil || child.CRC.Withdrawn || !crcChildMatchesPlan(child, d) {
					return counts, fmt.Errorf("missing or inconsistent CRC child decision %s", plan.ChildLoop)
				}
			}
		}
		for _, id := range d.CRC.ChildLoops {
			if !declared[id] {
				return counts, fmt.Errorf("CRC parent %s declares unknown child %s", d.LoopID, id)
			}
			delete(declared, id)
		}
		if len(declared) != 0 {
			return counts, fmt.Errorf("CRC parent %s is missing child metadata", d.LoopID)
		}
	}
	return counts, nil
}
