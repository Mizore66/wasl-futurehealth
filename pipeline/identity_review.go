package main

import "strings"

// regDemo is one registration's recorded demographics, read from the Patient resource contained in its FHIR events.
type regDemo struct{ Name, Dob, Sex, Eid string }

func (d *regDemo) absorb(e *Event) {
	if d.Name == "" {
		d.Name = e.Name
	}
	if d.Dob == "" {
		d.Dob = e.Dob
	}
	if d.Sex == "" || d.Sex == "U" {
		d.Sex = e.Sex
	}
	if d.Eid == "" {
		d.Eid = e.Eid
	}
}

func maskEID(e string) string {
	if len(e) < 8 {
		return strings.Repeat("*", len(e))
	}
	return e[:3] + strings.Repeat("*", len(e)-6) + e[len(e)-3:]
}

func idField(state, value string) map[string]interface{} {
	return map[string]interface{}{"state": state, "value": value}
}

func demoView(pk string, d regDemo) map[string]interface{} {
	name := idField("present", d.Name)
	if d.Name == "" {
		name = idField("missing", "")
	}
	dob := idField("present", d.Dob)
	switch {
	case d.Dob == "":
		dob = idField("missing", "")
	case placeholderDob(d.Dob):
		dob = idField("placeholder", d.Dob)
	}
	sex := idField("unknown", "")
	if d.Sex == "M" || d.Sex == "F" {
		sex = idField("present", d.Sex)
	}
	eid := idField("present", maskEID(d.Eid))
	switch {
	case d.Eid == "":
		eid = idField("missing", "")
	case dummyEID(d.Eid):
		eid = idField("placeholder", maskEID(d.Eid))
	case !luhnOK(d.Eid):
		eid = idField("invalid", maskEID(d.Eid))
	}
	return map[string]interface{}{"registration": pk, "name": name, "dob": dob, "sex": sex, "eid": eid}
}

func verdictName(a, b regDemo) string {
	switch {
	case a.Name == "" || b.Name == "":
		return "missing"
	case strings.EqualFold(a.Name, b.Name):
		return "match"
	}
	return "differ"
}

func verdictDob(a, b regDemo) string {
	switch {
	case a.Dob == "" || b.Dob == "":
		return "missing"
	case placeholderDob(a.Dob) || placeholderDob(b.Dob):
		return "placeholder"
	case a.Dob == b.Dob:
		return "match"
	}
	return "differ"
}

func verdictSex(a, b regDemo) string {
	known := func(s string) bool { return s == "M" || s == "F" }
	switch {
	case !known(a.Sex) || !known(b.Sex):
		return "unknown"
	case a.Sex == b.Sex:
		return "match"
	}
	return "differ"
}

func verdictEID(a, b regDemo) string {
	switch {
	case a.Eid == "" || b.Eid == "":
		return "missing"
	case dummyEID(a.Eid) || dummyEID(b.Eid):
		return "placeholder"
	case !luhnOK(a.Eid) || !luhnOK(b.Eid):
		return "invalid"
	case a.Eid == b.Eid:
		return "match"
	}
	return "differ"
}

// identityGuard restates the v7 second-identifier rule over the two records' own facts. The name-agreement and
// shared-ID checks of the Emirates-ID path are the linker's and are not recomputed here.
func identityGuard(dob, sex, eid string, a, b regDemo) map[string]interface{} {
	eidPath := eid == "match" && sex == "match" && dobsCompatible(a.Dob, b.Dob)
	failures := []string{}
	if dob != "match" {
		failures = append(failures, "dob_"+dob)
	}
	switch {
	case eid != "match":
		failures = append(failures, "eid_"+eid)
	case sex != "match":
		failures = append(failures, "sex_"+sex)
	case !eidPath:
		failures = append(failures, "dob_incompatible")
	}
	met := dob == "match" || eidPath
	if met {
		failures = []string{"name_or_shared_id_check"}
	}
	return map[string]interface{}{"met": met, "failures": failures}
}

// identityReview is the item-specific identifier comparison for each unsigned link item. It is returned only to roles that
// may sign a link; every other viewer, the redacted export included, gets no key at all.
func (s *apiServer) identityReview(c *apiCase, viewer apiPrincipal) []map[string]interface{} {
	if !rbac[viewer.role][ActSignLink] {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]interface{}
	for _, it := range s.queue[c.ID] {
		if !itemHasKind(it, "link") || s.itemSigned(c.ID, it.ItemID) {
			continue
		}
		entry := map[string]interface{}{"item": it.ItemID, "pair": []string{it.Pair[0], it.Pair[1]}}
		a, aok := s.demo[it.Pair[0]]
		b, bok := s.demo[it.Pair[1]]
		if !aok || !bok {
			entry["available"], entry["reason"] = false, "records_not_found"
			out = append(out, entry)
			continue
		}
		name, dob, sex, eid := verdictName(a, b), verdictDob(a, b), verdictSex(a, b), verdictEID(a, b)
		entry["available"] = true
		entry["records"] = []map[string]interface{}{demoView(it.Pair[0], a), demoView(it.Pair[1], b)}
		entry["comparison"] = []map[string]string{
			{"field": "name", "verdict": name}, {"field": "dob", "verdict": dob}, {"field": "sex", "verdict": sex}, {"field": "eid", "verdict": eid},
		}
		entry["guard"] = identityGuard(dob, sex, eid, a, b)
		out = append(out, entry)
	}
	return out
}
