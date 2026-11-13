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
	linked := func(d DownFact) bool {
		// The evaluator additionally gates a DR/breast repeat on U/TR.
		return d.Relation == addendumRelation ||
			(d.Relation == repeatRelation && (prog == "diabetic_retinopathy" || prog == "breast"))
	}
	// Discover descendants without admitting disconnected cyclic islands. Reaching
	// index establishes membership only, not validity of the index's own lineage.
	for _, id := range ids {
		seen := map[string]bool{}
		cur := id
		for cur != index && !seen[cur] {
			seen[cur] = true
			d, ok := current[cur]
			if !ok || !linked(d) || d.Parent == "" {
				break
			}
			cur = d.Parent
		}
		if cur == index {
			included[id] = true
		}
	}
	// Validate complete applicable paths before any grade/supersession selection.
	// Unlike membership discovery, this must traverse THROUGH index to a genuine
	// root: index -> child -> index, an ancestor cycle, and self-edges all hold.
	// Ancestors are validated, not added as independently selectable episode rows.
	validated := map[string]bool{}
	for _, id := range append([]string{index}, ids...) {
		if !included[id] {
			continue
		}
		path, seen := []string{}, map[string]bool{}
		for cur := id; !validated[cur]; {
			if seen[cur] {
				return nil, "cyclic_index_evidence_lineage"
			}
			seen[cur] = true
			path = append(path, cur)
			d, ok := current[cur]
			if !ok {
				return nil, "invalid_index_evidence_lineage"
			}
			if conflict[cur] {
				return nil, "conflicting_index_evidence_version"
			}
			if d.Relation == "" && d.Parent == "" {
				break
			}
			if !linked(d) || d.Parent == "" {
				return nil, "invalid_index_evidence_lineage"
			}
			cur = d.Parent
		}
		for _, cur := range path {
			validated[cur] = true
		}
	}
	// A linked correction may itself correct the clinical date. Check every
	// validated edge, including the index's, before chronological sorting can
	// silently restore a superseded ancestor or select a normal descendant.
	for id := range validated {
		d := current[id]
		if d.Parent != "" && d.Day < current[d.Parent].Day {
			return nil, "contradictory_index_evidence_chronology"
		}
	}
	out := make([]row, 0, len(included))
	for _, r := range in {
		if included[r.d.ReportID] && string(canon(r.d)) == string(canon(current[r.d.ReportID])) {
			out = append(out, r)
		}
	}
	// Clinical time orders separate observations, but a same-time child must
	// follow its ancestors. Resource IDs and versions across distinct resources
	// establish no clinical precedence. Every included path already reaches index.
	depth := map[string]int{index: 0}
	for id := range included {
		for cur := id; cur != index; cur = current[cur].Parent {
			depth[id]++
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].d.Day != out[j].d.Day {
			return out[i].d.Day < out[j].d.Day
		}
		return depth[out[i].d.ReportID] < depth[out[j].d.ReportID]
	})
	// Same-time siblings (including branches at different depths) have no
	// explicit precedence. Do not let their order choose a closing grade. In a
	// totally ordered lineage each adjacent same-time row descends from the last.
	for i := 1; i < len(out); i++ {
		prev, next := out[i-1].d, out[i].d
		if prev.Day != next.Day {
			continue
		}
		cur := next.ReportID
		for cur != index && cur != prev.ReportID {
			cur = current[cur].Parent
		}
		if cur != prev.ReportID {
			return nil, "ambiguous_same_time_index_evidence_lineage"
		}
	}
	return out, ""
}

func episodeRepeatAllowed(prog, label string) bool {
	return (prog == "diabetic_retinopathy" && label == "U") || (prog == "breast" && label == "TR")
}
