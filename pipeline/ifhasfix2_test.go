package main

import "testing"

// Fix round 2: deferral scope around the cue, negation that must govern the start, appointments versus plan arrangements.

type ifhasStatusCase struct {
	text string
	want map[string]string
}

func ifhasStatusCases(t *testing.T, cases []ifhasStatusCase) {
	t.Helper()
	for _, c := range cases {
		got := regexIfhasNote(c.text).Status
		for d, w := range c.want {
			if got[d] != w {
				t.Errorf("%q\n  %s = %q, want %q (all %v)", c.text, d, got[d], w, got)
			}
		}
	}
}

func TestIfhasDeferralScopeAroundCue(t *testing.T) {
	ifhasStatusCases(t, []ifhasStatusCase{
		{"Diabetes confirmed. Metformin started today, lifestyle review postponed.", map[string]string{"glyc": "managed"}},
		{"Hypertension confirmed. Amlodipine started, follow-up deferred.", map[string]string{"bp": "managed"}},
		{"LDL 180 mg/dL confirmed. Statin started; treatment review deferred.", map[string]string{"lipid": "managed"}},
		{"LDL 180 mg/dL confirmed. Statin started, treatment review deferred.", map[string]string{"lipid": "managed"}},
		{"Hypertension confirmed. Amlodipine started, education deferred.", map[string]string{"bp": "managed"}},
		{"Statin started, BP decision deferred.", map[string]string{"lipid": "managed", "bp": "pending"}},
		{"Amlodipine started, metformin decision deferred.", map[string]string{"bp": "managed", "glyc": "pending"}},
		{"Hypertension confirmed. Amlodipine started, BP decision deferred.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed, treatment decision deferred, amlodipine to be considered.", map[string]string{"bp": "pending"}},
	})
}

func TestIfhasNegationNeedsToGovernTheStart(t *testing.T) {
	ifhasStatusCases(t, []ifhasStatusCase{
		{"Hypertension confirmed. BP not at target after amlodipine started.", map[string]string{"bp": "managed"}},
		{"Hypertension confirmed. BP not controlled since amlodipine commenced.", map[string]string{"bp": "managed"}},
		{"Hypertension confirmed. No medication commenced today.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed. Amlodipine not started.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed. Treatment not started; decision deferred to next visit.", map[string]string{"bp": "pending"}},
	})
}

func TestIfhasAppointmentIsNotAStartedPlan(t *testing.T) {
	ifhasStatusCases(t, []ifhasStatusCase{
		{"Hypertension confirmed. Follow-up arranged. Home BP monitoring requested.", map[string]string{"bp": "pending"}},
		{"Diabetes confirmed. Appointment booked to discuss. Home glucose monitoring requested.", map[string]string{"glyc": "pending"}},
		{"Hypertension confirmed. Review appointment booked in two weeks. Home BP monitoring requested.", map[string]string{"bp": "pending"}},
		{"Hypertension confirmed. Specialist referral booked. Home BP monitoring requested.", map[string]string{"bp": "managed"}},
		{"Obesity confirmed. Dietitian referral booked.", map[string]string{"weight": "managed"}},
		{"Hypertension confirmed. Amlodipine commenced. Follow-up arranged.", map[string]string{"bp": "managed"}},
		{"Diabetes confirmed. Metformin started; follow-up appointment booked.", map[string]string{"glyc": "managed"}},
	})
}
