package main

// Ungraded-prep escalation (DoH CRC closure rule, docs/WRITEUP.md "Closure rule"): a colonoscopy report that documents
// the caecum but never grades the bowel preparation does not close the loop and is not sent to a reviewer who cannot
// grade it either. The loop becomes done_quality_unverified, Wasl queries the endoscopy-unit lead, escalates to the
// programme coordinator after 5 working days, and at day 15 a clinician decides (closed with a documentation flag, or
// repeat required).
//
// The unit's answers are simulated. No published data measures how endoscopy units answer a retrospective prep-grade
// query, so the rates are proxies and labelled assumptions (sources in pipeline/RESULTS.md, "Ungraded-prep escalation"):
//   - answer within 5 working days 55% (data-query turnaround in community oncology, discounted: not mandated);
//   - answer only after the coordinator escalation 20% (assumption, sized by the published escalation effect);
//   - no answer by day 15 25% (remainder);
//   - an answer is a usable grade 50%, else "cannot determine" (assumption: grading prep afterwards from stored images
//     is unreliable);
//   - the day-15 clinician decision has a 25% per-read error (assumption, same reason). It is a stress-test rate, not
//     evidence that clinicians err.

var (
	PrepQuery         = true // detect ungraded prep and run the query/escalation flow (flag -prepquery)
	PrepP1            = 0.55 // P(unit answers within 5 working days)
	PrepP2            = 0.20 // P(unit answers only after the coordinator escalation, before day 15)
	PrepUsable        = 0.50 // P(an answer is a usable grade rather than "cannot determine")
	PrepEscalateDays  = 7.0  // 5 working days, in calendar days
	PrepDecideDays    = 15.0 // calendar days from the query to the clinician decision
	ErrUngraded       = 0.25 // per-read error of the day-15 clinician decision (assumption)
	PrepDay15Purpose  = "prep_ungraded_day15"
)

// prepThread is the simulated unit-lead thread for one ungraded-prep report. It uses its own hashed stream (u01), so the
// rest of the cohort is byte-identical with or without it. truthAdequate: the prep was in fact adequate.
type prepQMsg struct {
	Day   float64
	Cat   string // prep-query | prep-escalation | prep-reply
	Grade string // prep-reply only: adequate | inadequate | cannot_determine
}

func prepThread(seed int64, reportID string, t0 float64, truthAdequate bool) []prepQMsg {
	msgs := []prepQMsg{{Day: t0, Cat: "prep-query"}}
	u := u01(seed, reportID, "prep", "group")
	ln := func(med float64, salt string) float64 { // lognormal(median, 0.6) from two hashed uniforms
		a, b := u01(seed, reportID, "prep", salt, "a"), u01(seed, reportID, "prep", salt, "b")
		return lognormSecs(med, 0.6, a, b)
	}
	reply := -1.0
	switch {
	case u < PrepP1:
		reply = t0 + minf(ln(3.5, "d1"), PrepEscalateDays-0.05)
	case u < PrepP1+PrepP2:
		reply = t0 + PrepEscalateDays + minf(ln(3.5, "d2"), PrepDecideDays-PrepEscalateDays-0.05)
	}
	if reply < 0 || reply > t0+PrepEscalateDays {
		msgs = append(msgs, prepQMsg{Day: t0 + PrepEscalateDays, Cat: "prep-escalation"})
	}
	if reply >= 0 {
		g := "cannot_determine"
		if u01(seed, reportID, "prep", "usable") < PrepUsable {
			g = map[bool]string{true: "adequate", false: "inadequate"}[truthAdequate]
		}
		msgs = append(msgs, prepQMsg{Day: reply, Cat: "prep-reply", Grade: g})
	}
	return msgs
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// prepState: what the closure engine knows at `now` about the query thread for one report.
type prepState struct {
	queryDay float64 // -1 = no query seen
	escalated bool
	grade    string // adequate | inadequate | cannot_determine | ""
}

func prepStates(comms []CommFact, now float64) map[string]*prepState {
	m := map[string]*prepState{}
	for _, cm := range comms {
		if cm.Report == "" || cm.Day > now+1e-9 {
			continue
		}
		st := m[cm.Report]
		if st == nil {
			st = &prepState{queryDay: -1}
			m[cm.Report] = st
		}
		switch cm.Category {
		case "prep-query":
			if st.queryDay < 0 || cm.Day < st.queryDay {
				st.queryDay = cm.Day
			}
		case "prep-escalation":
			st.escalated = true
		case "prep-reply":
			st.grade = cm.Grade
		}
	}
	return m
}

// errFor: the day-15 ungraded-prep decision is made without a documented grade, so it carries ErrUngraded (assumed)
// instead of the reviewer's usual per-read error.
func (c ReviewerCfg) errFor(it *ReviewItem) float64 {
	if it.Reason == PrepDay15Purpose {
		return ErrUngraded
	}
	return c.Err
}
