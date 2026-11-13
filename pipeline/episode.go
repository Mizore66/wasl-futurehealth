package main

import "sort"

const (
	addendumRelation = "urn:wsi:addendum-to"
	repeatRelation   = "urn:wsi:repeat-of"
)

// episodeRows selects only the index report's current same-ID revision and
// documents with an explicit lineage back to that index. It never infers
// episode membership from registration, date, arrival order, or content.
func episodeRows(in []row, index, prog string) ([]row, string) {
	if index == "" {
		return nil, "missing_or_ambiguous_index_reference"
	}
	current := map[string]DownFact{}
	conflict := map[string]bool{}
	for _, r := range in {
		d := r.d
		if d.ReportID == "" {
			continue
		}
		old, ok := current[d.ReportID]
		if !ok || d.Version > old.Version || (d.Version == old.Version && d.EventID < old.EventID) {
			if ok && d.Version == old.Version && string(canon(d)) != string(canon(old)) {
				conflict[d.ReportID] = true
			}
			current[d.ReportID] = d
		} else if d.Version == old.Version && string(canon(d)) != string(canon(old)) {
			conflict[d.ReportID] = true
		}
	}
	// Revision/withdrawal selection precedes evidence eligibility: a current
	// withdrawal must not reveal an older corrected or final version.
	for id, d := range current {
		if d.Status != "" && !diagnosticReportEvidenceEligible(d.Status) {
			delete(current, id)
		}
	}
	if _, ok := current[index]; !ok {
		return nil, "index_report_withdrawn_or_not_current"
	}
	included := map[string]bool{index: true}
	ids := make([]string, 0, len(current))
	for id := range current {
		if id != index {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		seen := map[string]bool{}
		cur := id
		valid := false
		for cur != index {
			if seen[cur] {
				// A cycle is ambiguous only when it captures the index lineage;
				// an unrelated cyclic island is not this loop's evidence.
				if seen[index] {
					return nil, "cyclic_index_evidence_lineage"
				}
				valid = false
				break
			}
			seen[cur] = true
			d, ok := current[cur]
			if !ok {
				break
			}
			if d.Relation == addendumRelation {
				// Corrections and addenda are allowed for all three programmes.
			} else if d.Relation == repeatRelation && (prog == "diabetic_retinopathy" || prog == "breast") {
				// The evaluator additionally gates a repeat on U/TR at its turn.
			} else {
				break
			}
			if d.Parent == "" {
				break
			}
			cur = d.Parent
			if cur == index {
				valid = true
			}
		}
		if valid {
			included[id] = true
		}
	}
	for id := range included {
		if conflict[id] {
			return nil, "conflicting_index_evidence_version"
		}
	}
	out := make([]row, 0, len(included))
	for _, r := range in {
		if included[r.d.ReportID] && string(canon(r.d)) == string(canon(current[r.d.ReportID])) {
			out = append(out, r)
		}
	}
	return out, ""
}

func episodeRepeatAllowed(prog, label string) bool {
	return (prog == "diabetic_retinopathy" && label == "U") || (prog == "breast" && label == "TR")
}
