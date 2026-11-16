package main

import (
	"regexp"
	"strings"
)

// Assertion scopes are source-text byte spans, not sentence-level vetoes. All
// outcome readers share them, so an unrelated historical/negative adjunct cannot
// erase a current subject or turn the adjunct into a current diagnosis. Quotes
// retain the original clause (including its context); no source event is rewritten.
type crcFinding struct {
	Start, End           int
	ScopeStart, ScopeEnd int
	Context              crcAssertionContext
	Assertion            crcAssertion
	Quote                string
}

type crcAssertionContext uint8

const (
	crcFindingContext crcAssertionContext = iota
	crcIndicationContext
	crcHistoryContext
	crcFamilyContext
)

var (
	crcCurrentContextRE    = regexp.MustCompile(`(?i)\b(?:current(?:ly)?|today(?:'s)?)\b`)
	crcSectionRE           = regexp.MustCompile(`(?i)\b(indication|reason\s+for\s+(?:procedure|examination)|clinical\s+history|past\s+medical\s+history|family\s+history|history|findings|impression|(?:final\s+)?diagnosis|microscopic\s+examination|colonoscopy)\s*:`)
	crcFamilyHistoryRE     = regexp.MustCompile(`(?i)\bfamily\s+history\b`)
	crcSubjectJoinRE       = regexp.MustCompile(`(?i)\b(and|or|with|alongside|despite|in\s+addition\s+to|in\s+the\s+(?:setting|context)\s+of|in\s+a\s+patient|against\s+a\s+background\s+of)\b`)
	crcQualifiedHeadRE     = regexp.MustCompile(`(?i)^\s*(?:(?:a|an|the)\s+)?(?:current(?:ly)?|today(?:'s)?|prior|previous(?:ly)?|histor(?:y|ical)|family\s+history|hx\s+of|no|without|negative\s+for)\b`)
	crcArticleHeadRE       = regexp.MustCompile(`(?i)^\s*(?:a|an|the)\s+`)
	crcObservationRE       = regexp.MustCompile(`(?i)\b(?:seen|identified|present|detected|confirmed|excluded|ruled\s+out)\b`)
	crcSharedObservationRE = regexp.MustCompile(`(?i)\b(?:were|are)\s+(?:not\s+)?(?:seen|identified|present|detected|confirmed|excluded|ruled\s+out)\b`)
	crcTentativeBeforeRE   = regexp.MustCompile(`(?i)\b(?:suspected|suspicious(?:\s+for)?|possible)\s+(?:(?:a|an|the|rectal|colonic|colorectal|sigmoid|invasive)\s+)*$`)
	crcSubjectNounRE       = regexp.MustCompile(`(?i)\b(?:mass|stricture|polyp|adenoma|carcinoma|adenocarcinoma|cancer|malignancy|colitis|proctitis|crohn(?:'s)?|IBD)\b`)
	crcSuspicionWordRE     = regexp.MustCompile(`(?i)\b(?:suspected|suspicious|malignant[- ]looking)\b`)
	crcPurposeBeforeRE     = regexp.MustCompile(`(?i)\bscreening\s+(?:colonoscopy\s+)?(?:(?:for|to\s+exclude)\s+)?(?:(?:a|an|the|colon|colorectal|rectal|invasive)\s+)*$`)
	crcPurposeAfterRE      = regexp.MustCompile(`(?i)^\s+screening\b`)
	// A present predicate can reassert a previously suspected finding. Require
	// the predicate on this very subject, not a "current" word elsewhere. A past
	// persistence predicate ("was still present") does not establish current care.
	crcPresentObservationRE = regexp.MustCompile(`(?i)^\s*(?:(?:a|an|the|rectal|colonic|colorectal|sigmoid|mass|stricture|and|or)\s+)*(?:(?:is|are)\s+(?:still\s+)?(?:not\s+)?(?:seen|identified|present|detected|confirmed|excluded|ruled\s+out)|(?:cannot|can't)\s+be\s+(?:confirmed|excluded|ruled\s+out))\b`)
)

func crcFindingReasserted(after string) bool {
	// Even a present-tense transcription can describe an old observation. Its
	// own historical qualifier wins; unrelated history has already been split.
	return crcPresentObservationRE.MatchString(after) && !rePriorCtx.MatchString(after)
}

// A conjunction may share modifiers across a finding list, or introduce another
// subject. Split only at a newly qualified subject or its own observation. In
// particular, "history of mass and a stricture" and "mass and stricture were not
// seen" share scope; "history of colitis and a mass was seen" does not.
func crcSubjectBoundary(left, join, right string) (split, ambiguous bool) {
	join = strings.ToLower(join)
	if join != "and" && join != "or" && join != "with" {
		return true, false // adjunct introduces its own subject
	}
	if crcQualifiedHeadRE.MatchString(right) {
		return true, false
	}
	if !crcObservationRE.MatchString(right) {
		return false, false
	}
	if crcCurrentContextRE.MatchString(right) {
		return true, false
	}
	if crcSharedObservationRE.MatchString(right) && !crcObservationRE.MatchString(left) {
		return false, false
	}
	if crcArticleHeadRE.MatchString(right) {
		return true, false
	}
	l, r := crcSubjectNounRE.FindString(left), crcSubjectNounRE.FindString(right)
	if l != "" && r != "" && !strings.EqualFold(l, r) {
		// Bare coordinated findings can share a predicate. An observation of a
		// different subject alongside history has ambiguous attachment: hold it,
		// but never assert a diagnosis or benign clearance from that ambiguity.
		if rePriorCtx.MatchString(left) {
			return true, true
		}
		return crcObservationRE.MatchString(left), false
	}
	return false, false
}

func crcScopedFindings(text string, re *regexp.Regexp) []crcFinding {
	clauses := crcClauseBoundaryRE.FindAllStringIndex(text, -1)
	sections := crcSectionRE.FindAllStringSubmatchIndex(text, -1)
	var findings []crcFinding
	for _, m := range re.FindAllStringIndex(text, -1) {
		f := crcFinding{Start: m[0], End: m[1], ScopeEnd: len(text)}
		for _, b := range clauses {
			if b[1] <= f.Start {
				f.ScopeStart = b[1]
			} else if b[0] >= f.End {
				f.ScopeEnd = b[0]
				break
			}
		}
		for _, s := range sections {
			label := strings.ToLower(strings.Join(strings.Fields(text[s[2]:s[3]]), " "))
			if label == "colonoscopy" {
				// "Not today's current colonoscopy: mass" is not a new section.
				prefix := strings.TrimRight(text[:s[0]], " \t")
				if prefix != "" && !strings.ContainsAny(prefix[len(prefix)-1:], ".;\n") {
					continue
				}
			}
			if s[1] <= f.Start {
				f.Context = crcFindingContext
				switch label {
				case "indication", "reason for procedure", "reason for examination":
					f.Context = crcIndicationContext
				case "family history":
					f.Context = crcFamilyContext
				case "history", "clinical history", "past medical history":
					f.Context = crcHistoryContext
				}
				if s[1] > f.ScopeStart {
					f.ScopeStart = s[1]
				}
			} else if s[0] >= f.End && s[0] < f.ScopeEnd {
				f.ScopeEnd = s[0]
				break
			}
		}
		f.Quote = strings.TrimSpace(text[f.ScopeStart:f.ScopeEnd])
		start, end := f.ScopeStart, f.ScopeEnd
		joins := crcSubjectJoinRE.FindAllStringIndex(text[start:end], -1)
		ambiguous := false
		for i, j := range joins {
			a, b := start+j[0], start+j[1]
			rightEnd := end
			if i+1 < len(joins) {
				rightEnd = start + joins[i+1][0]
			}
			split, unclear := crcSubjectBoundary(text[f.ScopeStart:a], text[a:b], text[b:rightEnd])
			if !split {
				continue
			}
			if b <= f.Start {
				f.ScopeStart, ambiguous = b, unclear
			} else if a >= f.End {
				f.ScopeEnd = a
				break
			}
		}
		before, after := text[f.ScopeStart:f.Start], text[f.End:f.ScopeEnd]
		f.Assertion = crcFindingAssertion(before, after)
		scope := text[f.ScopeStart:f.ScopeEnd]
		switch {
		case f.Context == crcFamilyContext || crcFamilyHistoryRE.MatchString(scope):
			f.Assertion = crcContextOnly
		case f.Context == crcHistoryContext && !crcCurrentContextRE.MatchString(scope) && !crcFindingReasserted(after):
			f.Assertion = crcHistorical
		case f.Assertion == crcAffirmative && (ambiguous || re != crcSuspiciousRE && crcTentativeBeforeRE.MatchString(before)):
			// Suspected disease is not a confirmed diagnosis. The suspicion reader
			// may retain it for coordination, never for automatic diagnosed care.
			f.Assertion = crcUnresolved
		case f.Assertion == crcAffirmative && (f.Context == crcIndicationContext || crcPurposeBeforeRE.MatchString(before) || crcPurposeAfterRE.MatchString(after)):
			// Purpose/header words are not finding assertions. Explicit suspicion
			// and observations remain clinical evidence even in an indication.
			if !crcTentativeBeforeRE.MatchString(before) && !crcSuspicionWordRE.MatchString(text[f.Start:f.End]) && !crcPostObservationRE.MatchString(after) {
				f.Assertion = crcContextOnly
			}
		}
		findings = append(findings, f)
	}
	return findings
}
