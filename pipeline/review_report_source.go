package main

import "strings"

// These descriptors come from the immutable FHIR log, not the case's mutable
// presentation timeline. Keep every source revision needed by earlier items.
type reviewReportSourceKey struct {
	Report  string
	Version int
}

type reviewReportSource struct {
	Category  string
	BodySHA   string
	Ambiguous bool
}

func reviewSourceVersion(version int) int {
	if version <= 1 {
		return 0 // the existing unversioned/v1 review identity convention
	}
	return version
}

func (s *apiServer) rememberReviewReportSource(event *Event, raw []byte) {
	if event == nil || event.RType != "DiagnosticReport" || event.Provider == "" || event.ReportID == "" || event.Version < 0 {
		return
	}
	if s.reviewReportSources == nil {
		s.reviewReportSources = map[reviewReportSourceKey]reviewReportSource{}
	}
	key := reviewReportSourceKey{event.Provider + "/DiagnosticReport/" + event.ReportID, reviewSourceVersion(event.Version)}
	bodySHA := sha(raw)
	if previous, exists := s.reviewReportSources[key]; exists {
		if previous.BodySHA != bodySHA {
			previous.Ambiguous = true
			s.reviewReportSources[key] = previous
		}
		return
	}
	category := ""
	if diagnosticReportEvidenceEligible(event.Status) {
		category = event.Category
	}
	s.reviewReportSources[key] = reviewReportSource{Category: category, BodySHA: bodySHA}
}

// raisedReviewReport identifies only the provider-qualified report in this
// canonical item's own intact chain. It never borrows a current decision's
// evidence, another item's source or a same-short-ID timeline row.
func raisedReviewReport(item *ReviewItem) string {
	if item == nil || item.ReportID == "" || item.ReportVersion < 0 || item.Head == "" || len(item.Chain) == 0 {
		return ""
	}
	var verified []Block
	report := ""
	for _, block := range item.Chain {
		verified = appendBlock(verified, block.Type, block.Body)
		last := verified[len(verified)-1]
		if block.Prev != last.Prev || block.Hash != last.Hash {
			return ""
		}
		if block.Type != "evidence" {
			continue
		}
		ref, ok := block.Body["report"].(string)
		parts := strings.Split(ref, "/")
		if !ok || len(parts) != 3 || parts[0] == "" || parts[1] != "DiagnosticReport" || parts[2] != item.ReportID || report != "" && report != ref {
			return ""
		}
		report = ref
	}
	if verified[len(verified)-1].Hash != item.Head || report == "" {
		return ""
	}
	parts := strings.Split(report, "/")
	linked := false
	for _, block := range item.Chain {
		if block.Type != "link_decision" {
			continue
		}
		ref, ok := block.Body["downstream_event"].(string)
		event := strings.Split(ref, "/")
		if !ok || len(event) != 3 || event[0] != parts[0] || event[2] != item.ReportID || event[1] != "DiagnosticReport" && event[1] != "Pathology" {
			return ""
		}
		linked = true
	}
	if !linked {
		return ""
	}
	// Under the trusted-OUT premise, bind the declared version to an actual
	// producer identity. Reason may be a reader hint, not the identity purpose.
	// This recomputable consistency check is not authentication of the OUT file.
	for _, purpose := range []string{"", OverrideConfirm, ShadowConfirm, AutoAudit} {
		if item.ItemID == evidenceReviewItemID(item.LoopID, item.ReportID, item.ReportVersion, purpose) {
			return report
		}
	}
	return ""
}

func (s *apiServer) raisedReportCategory(caseID string, item *ReviewItem) string {
	if item == nil || item.LoopID != caseID {
		return ""
	}
	report := raisedReviewReport(item)
	if report == "" {
		return ""
	}
	source, exists := s.reviewReportSources[reviewReportSourceKey{report, reviewSourceVersion(item.ReportVersion)}]
	if !exists || source.Ambiguous {
		return ""
	}
	switch source.Category {
	case "colonoscopy", "pathology":
		return source.Category
	default:
		return ""
	}
}
