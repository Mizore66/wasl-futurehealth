package main

import "encoding/json"

// Typed CRC terminals replace neither the original chain/signature checks nor
// source-bound parent validation. They describe the producer's existing signed
// plan/completed-follow-up contract, not a generic exemption for closed CRC.
func crcClosedTerminal(d *Decision) bool {
	if d == nil || d.Prog != "crc" || d.CRC == nil || d.CRC.Schema != CRCPlanSchema || d.CRC.Withdrawn || len(d.Chain) == 0 {
		return false
	}
	cd := d.CRC
	last := d.Chain[len(d.Chain)-1]
	if cd.Parent != "" {
		if _, err := materializedCaseKind(d); err != nil {
			return false
		}
		if !crcChildSourceIdentity(d) || last.Type != "crc_followup_completed" || cd.FollowUpState != "accepted_or_first_appointment" || cd.CareState != "accepted_or_first_appointment" || (cd.Kind != "crc-gastroenterology" && cd.Kind != "crc-specialist") {
			return false
		}
		var completed struct {
			Loop, Category string
			Day            float64
		}
		return json.Unmarshal(canon(last.Body), &completed) == nil && completed.Loop == d.LoopID && completed.Category == cd.Kind && completed.Day == d.ClosedDay
	}
	if cd.Kind != "screening" || !cd.ScreeningComplete || last.Type != "crc_plan" || len(cd.Evidence) == 0 {
		return false
	}
	var terminal struct {
		Schema            string
		ScreeningComplete bool   `json:"screening_complete"`
		CareState         string `json:"care_state"`
		Obligations       []CRCPlanItem
		Evidence          []CRCEvidenceState
	}
	if json.Unmarshal(canon(last.Body), &terminal) != nil || terminal.Schema != cd.Schema || terminal.ScreeningComplete != cd.ScreeningComplete || terminal.CareState != cd.CareState || string(canon(terminal.Obligations)) != string(canon(cd.Obligations)) || string(canon(terminal.Evidence)) != string(canon(cd.Evidence)) {
		return false
	}
	// Bind every declared signed need to the actual nested evidence chain and
	// original review head. Rehashing the public plan is not a source signature.
	for _, plan := range cd.Obligations {
		if plan.ID == "" || plan.SourceReport == "" || plan.ProcedureReport == "" || plan.SourceVersion < 0 || plan.ProcedureVersion < 0 || plan.Confirmation == "" || plan.Reviewer == "" || plan.ExpectedHead == "" || crcCareKind(plan.Disposition) == "" {
			return false
		}
		matched := false
		for _, block := range d.Chain {
			if block.Type != "crc_plan_obligation" {
				continue
			}
			var nested struct {
				Plan  CRCPlanItem
				Chain []Block `json:"evidence_chain"`
			}
			if json.Unmarshal(canon(block.Body), &nested) != nil || string(canon(nested.Plan)) != string(canon(plan)) {
				continue
			}
			ch := nested.Chain
			if len(ch) < 3 || !verifyChain(ch) || ch[len(ch)-3].Hash != plan.ExpectedHead || ch[len(ch)-2].Type != "human_signoff" || ch[len(ch)-1].Type != "crc_disposition" {
				return false
			}
			var signed struct {
				Item, Reviewer, Evidence string
				Head                     string `json:"signed_head"`
			}
			var disposition struct{ Obligation, Disposition string }
			if json.Unmarshal(canon(ch[len(ch)-2].Body), &signed) != nil || json.Unmarshal(canon(ch[len(ch)-1].Body), &disposition) != nil || signed.Item != plan.Confirmation || signed.Reviewer != plan.Reviewer || signed.Head != plan.ExpectedHead || crcDispositionFromAnswer(signed.Evidence) != plan.Disposition || disposition.Obligation != plan.ID || disposition.Disposition != plan.Disposition {
				return false
			}
			for _, evidence := range ch {
				if evidence.Type != "crc_proposal" {
					continue
				}
				var source struct {
					Loop, Report     string
					Version          int    `json:"report_version"`
					Procedure        string `json:"procedure_report"`
					ProcedureVersion int    `json:"procedure_version"`
				}
				if json.Unmarshal(canon(evidence.Body), &source) == nil && source.Loop == d.LoopID && source.Report == plan.SourceReport && source.Version == plan.SourceVersion && source.Procedure == plan.ProcedureReport && source.ProcedureVersion == plan.ProcedureVersion {
					matched = true
				}
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
