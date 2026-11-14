package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests drive the production loadRecovery and candidate evaluation with
// producer-shaped, correctly fingerprinted unit input. They do not run the
// Python generator; they cover only the Go consumer layer.

func absoluteTargetCandidate(t *testing.T, s *apiServer, slot *recoverySlot, caseID string) map[string]interface{} {
	t.Helper()
	for _, c := range s.recoveryCandidatesLocked(slot) {
		if c["case_id"] == caseID {
			return c
		}
	}
	t.Fatalf("no candidate %s", caseID)
	return nil
}

func absoluteTargetHasWarning(c map[string]interface{}, reason string) bool {
	checks, _ := c["checks"].([]map[string]interface{})
	for _, check := range checks {
		if check["reason_code"] == reason && check["result"] == "warning" {
			return true
		}
	}
	return false
}

func TestRecoveryAbsoluteDeadlineBeforeSyntheticRequestStaysReady(t *testing.T) {
	run, ids := recoveryTestRun(t, 2)
	rewriteRecoveryTestInput(t, run, func(in *recoveryInput) {
		for i := range in.Waitlist {
			if in.Waitlist[i].CaseID == ids[2] {
				in.Waitlist[i].RequestedDay, in.Waitlist[i].DeadlineDay = 42, 31
			}
		}
	})
	s := loadAPI(run, "")
	if s.recovery.Status != "ready" {
		t.Fatalf("overdue absolute target invalidated recovery: %+v", s.recovery.Status)
	}
	if len(s.recovery.Slots) != 4 || len(s.recovery.Waitlist) != 4 {
		t.Fatalf("unrelated recovery work lost: slots=%d waitlist=%d", len(s.recovery.Slots), len(s.recovery.Waitlist))
	}
	w := s.recovery.Waitlist[ids[2]]
	if w == nil || w.DeadlineDay != 31 || w.RequestedDay != 42 {
		t.Fatalf("authoritative target shifted: %+v", w)
	}
	slot := recoveryTestSlot(s, 1)
	late := absoluteTargetCandidate(t, s, slot, ids[2])
	if late["eligible"] != true || late["deadline_day"] != 31 || !absoluteTargetHasWarning(late, "past_target") {
		t.Fatalf("late candidate must stay eligible with soft past_target: %v", late)
	}
	normal := absoluteTargetCandidate(t, s, slot, ids[3])
	if normal["eligible"] != true || absoluteTargetHasWarning(normal, "past_target") {
		t.Fatalf("normal control changed: %v", normal)
	}
}

func TestRecoveryAbsoluteDeadlineControlsAndTamperUnchanged(t *testing.T) {
	// Normal and equal deadline controls remain ready.
	run, ids := recoveryTestRun(t, 1)
	rewriteRecoveryTestInput(t, run, func(in *recoveryInput) {
		in.Waitlist[2].RequestedDay, in.Waitlist[2].DeadlineDay = 30, 30
	})
	if s := loadAPI(run, ""); s.recovery.Status != "ready" {
		t.Fatalf("equal deadline control: %s", s.recovery.Status)
	}
	invalid := func(name string, mutate func(*recoveryInput), raw func(string) string) {
		t.Helper()
		run, ids := recoveryTestRun(t, 1)
		_ = ids
		if mutate != nil {
			rewriteRecoveryTestInput(t, run, func(in *recoveryInput) {
				in.Waitlist[2].RequestedDay, in.Waitlist[2].DeadlineDay = 42, 31
				mutate(in)
			})
		}
		if raw != nil {
			p := filepath.Join(run, "recovery.json")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(raw(string(b))), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		s := loadAPI(run, "")
		if s.recovery.Status != "invalid" || len(s.recovery.Slots) != 0 || len(s.recovery.Waitlist) != 0 {
			t.Fatalf("%s: tamper accepted: %s", name, s.recovery.Status)
		}
	}
	invalid("unknown waitlist source signature", func(in *recoveryInput) { in.Waitlist[2].SourceDecisionSig = "forged" }, nil)
	invalid("unknown slot source signature", func(in *recoveryInput) { in.Slots[0].SourceDecisionSig = "forged" }, nil)
	invalid("past slot after as_of", func(in *recoveryInput) { in.AsOfDay = 60 }, nil)
	invalid("slot base head", func(in *recoveryInput) { in.Slots[0].BaseHead = "forged" }, nil)
	invalid("programme mismatch", func(in *recoveryInput) { in.Waitlist[2].Programme = "cervical" }, nil)
	invalid("negative prep lead", func(in *recoveryInput) { in.Waitlist[2].PrepLead = -1 }, nil)
	invalid("input fingerprint tamper", func(in *recoveryInput) {}, func(b string) string {
		return strings.Replace(b, `"deadline_day":31`, `"deadline_day":30`, 1)
	})
	_ = ids
}
