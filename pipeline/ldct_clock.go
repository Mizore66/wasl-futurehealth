package main

// LDCTClockAnchor records an examination independently of the current report.
// Version is the actual source version, never a default assigned to legacy facts.
// Registration is internal in the signed decision and pseudonymised at the API.
type LDCTClockAnchor struct {
	Kind         string  `json:"kind"`
	Provider     string  `json:"provider"`
	Registration string  `json:"registration"`
	Report       string  `json:"report"`
	Version      int     `json:"version"`
	Day          float64 `json:"day"`
}

type ldctClockSource struct {
	provider, registration, report string
}

// ldctClockFor validates only the clock's source role. Existing index selection,
// supersession, status and reader gates remain in evalLDCT. Unlike the selection
// map, this proof is qualified by provider AND registration, validates current
// versions, and traverses through the named index to detect ancestor cycles.
func ldctClockFor(f *LoopFacts, selected row, index *row, labelOf func(row) (string, string)) (*LDCTClockAnchor, string) {
	indexLabel := ""
	if index != nil {
		indexLabel, _ = labelOf(*index)
	}
	reclassified := index != nil && selected.d.ReportID != index.d.ReportID && (indexLabel == "LR3" || indexLabel == "LR4A")
	qualified := func(d DownFact) bool {
		return f.Provider != "" && f.SRRec != "" && d.Provider == f.Provider && d.RecPk == f.SRRec && d.ReportID != "" && d.Version > 0
	}
	if !qualified(selected.d) {
		if reclassified {
			return nil, "clock_source_unqualified"
		}
		// Legacy own-examination facts retain their existing selected-report day,
		// but cannot mint a versioned anchor or borrow another examination's day.
		return nil, ""
	}
	if !diagnosticReportEvidenceEligible(selected.d.Status) {
		return nil, "clock_source_unqualified"
	}
	key := func(id string) ldctClockSource { return ldctClockSource{f.Provider, f.SRRec, id} }
	current := map[ldctClockSource]DownFact{}
	conflict := map[ldctClockSource]bool{}
	for _, d := range f.Down {
		if d.RType != "LDCTScan" || d.Prog != "lung_ldct" || !qualified(d) {
			continue
		}
		k := key(d.ReportID)
		old, exists := current[k]
		if !exists || d.Version > old.Version {
			current[k], conflict[k] = d, false
		} else if d.Version == old.Version && string(canon(d)) != string(canon(old)) {
			conflict[k] = true
		}
	}
	path, seen := []DownFact{}, map[ldctClockSource]bool{}
	for id := selected.d.ReportID; ; {
		k := key(id)
		d, exists := current[k]
		if !exists || conflict[k] || seen[k] {
			return nil, "clock_lineage_unproven"
		}
		seen[k] = true
		path = append(path, d)
		if d.Parent == "" && d.Relation == "" {
			break
		}
		if d.Parent == "" || (d.Relation != addendumRelation && d.Relation != repeatRelation) {
			return nil, "clock_lineage_unproven"
		}
		id = d.Parent
	}
	if string(canon(path[0])) != string(canon(selected.d)) {
		return nil, "clock_lineage_unproven"
	}
	if index != nil {
		actual, exists := current[key(index.d.ReportID)]
		if !exists || conflict[key(index.d.ReportID)] || string(canon(actual)) != string(canon(index.d)) {
			return nil, "clock_lineage_unproven"
		}
	}
	kind, source := "own_exam", selected.d
	if reclassified {
		found := false
		for i, d := range path {
			if d.ReportID == index.d.ReportID {
				if !diagnosticReportEvidenceEligible(d.Status) {
					return nil, "clock_source_unqualified"
				}
				source, found = d, true
				break
			}
			// The producer also uses addendum-to for LR0 completion. An unknown,
			// incomplete or repeat examination in between cannot prove same-exam
			// reclassification of the LR3/LR4A index.
			lab, _ := labelOf(row{d: d})
			if d.Relation != addendumRelation || !diagnosticReportEvidenceEligible(d.Status) || lab == "" || lab == "LR0" || (i > 0 && !ldctKnownScanLabel(lab)) {
				return nil, "clock_role_ambiguous"
			}
		}
		if !found {
			return nil, "clock_lineage_unproven"
		}
		kind = "reclassified_index_exam"
	} else if index != nil && selected.d.ReportID != index.d.ReportID && indexLabel == "LR0" {
		// Both producer forms complete LR0 from the completion's own day.
		// Still-incomplete revisions may be superseded, but a prior completed
		// classification in between is a different role, not clock proof.
		found := false
		for _, d := range path[1:] {
			if d.ReportID == index.d.ReportID {
				found = true
				break
			}
			lab, _ := labelOf(row{d: d})
			if lab != "LR0" || !diagnosticReportEvidenceEligible(d.Status) {
				return nil, "clock_role_ambiguous"
			}
		}
		if !found {
			return nil, "clock_lineage_unproven"
		}
		kind = "completion_exam"
	}
	return &LDCTClockAnchor{Kind: kind, Provider: source.Provider, Registration: source.RecPk,
		Report: source.Provider + "/DiagnosticReport/" + source.ReportID, Version: source.Version, Day: source.Day}, ""
}

func ldctKnownScanLabel(label string) bool {
	_, ok := ldctRule(label)
	return ok
}
