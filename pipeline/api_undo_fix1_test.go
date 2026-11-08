package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fRev(actor, role, action string) map[string]interface{} {
	return map[string]interface{}{"case": undoCaseA, "decision": "same", "role": role, "actor": actor, "action": action}
}

func fComp(kind, target, role, actor, action, targetActor string) map[string]interface{} {
	return map[string]interface{}{"kind": kind, "case": undoCaseA, "target": target, "target_actor": targetActor,
		"role": role, "actor": actor, "action": action}
}

// writeForged writes a chain-valid ledger (correct prev links and hashes) from raw events. A target "@n" is replaced
// by the hash of event n, so tests can present semantically impossible but cryptographically intact histories.
func writeForged(t *testing.T, path string, evs []map[string]interface{}) {
	t.Helper()
	prev := "genesis"
	var sb strings.Builder
	var hashes []string
	for _, ev := range evs {
		if tg, _ := ev["target"].(string); strings.HasPrefix(tg, "@") {
			ev["target"] = hashes[int(tg[1]-'0')]
		}
		ev["prev"] = prev
		ev["hash"] = ledgerEventHash(ev)
		prev = ev["hash"].(string)
		hashes = append(hashes, prev)
		b, _ := json.Marshal(ev)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsSemanticallyInvalidChainValidLedgers(t *testing.T) {
	rev := func() map[string]interface{} { return fRev("r1", RoleReviewer, ActSignLink) }
	resolution := func(target, actor, res string) map[string]interface{} {
		e := fComp(evKindResolution, target, RoleReviewer, actor, ActSignLink, "r1")
		e["resolution"] = res
		return e
	}
	cases := map[string][]map[string]interface{}{
		"self-undo":                  {rev(), fComp("undo", "@0", RoleReviewer, "R1", ActSignLink, "r1")},
		"unauthorized role":          {rev(), fComp("undo", "@0", RoleAuditor, "a1", ActSignLink, "r1")},
		"empty role":                 {rev(), fComp("undo", "@0", "", "x", ActSignLink, "r1")},
		"unknown target":             {rev(), fComp("undo", "deadbeef", RoleReviewer, "r2", ActSignLink, "r1")},
		"resolution without contest": {rev(), resolution("@0", "r2", "upheld")},
		"empty target contest":       {rev(), fComp("contest", "", RoleReviewer, "r2", ActSignLink, "r1")},
		"empty target undo":          {rev(), fComp("undo", "", RoleReviewer, "r2", ActSignLink, "r1")},
		"unknown kind":               {rev(), fComp("erase", "@0", RoleReviewer, "r2", ActSignLink, "r1")},
		"double undo": {rev(), fComp("undo", "@0", RoleReviewer, "r2", ActSignLink, "r1"),
			fComp("undo", "@0", RoleReviewer, "r3", ActSignLink, "r1")},
		"recontest after rejection": {rev(), fComp("contest", "@0", RoleReviewer, "r2", ActSignLink, "r1"),
			resolution("@1", "r3", "rejected"), fComp("contest", "@0", RoleReviewer, "r4", ActSignLink, "r1")},
		"resolver is contester": {rev(), fComp("contest", "@0", RoleReviewer, "r2", ActSignLink, "r1"),
			resolution("@1", "r2", "upheld")},
		"forged contest target actor": {rev(), fComp("contest", "@0", RoleReviewer, "r2", ActSignLink, "nobody"),
			resolution("@1", "r1", "upheld")},
	}
	for name, evs := range cases {
		t.Run(name, func(t *testing.T) {
			r := newUndoRig(t)
			writeForged(t, r.ledgerPath(), evs)
			if _, err := readVerifiedLedger(r.ledgerPath()); err != nil {
				t.Fatalf("test ledger must be chain-valid: %v", err)
			}
			if err := validateReviewLedger(r.ledgerPath()); err == nil {
				t.Fatal("semantically invalid ledger accepted by validator")
			}
			if _, err := loadAPIVerified(r.s.run, ""); err == nil {
				t.Fatal("semantically invalid ledger loaded")
			}
		})
	}
}

func TestLoadAcceptsValidUndoContestHistory(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	_, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true}`)
	c := m["event"].(map[string]interface{})["hash"].(string)
	r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+c+`","resolution":"rejected"}`)
	r.review(undoCaseA, RoleClinician, "c1", "adequate")
	if err := validateReviewLedger(r.ledgerPath()); err != nil {
		t.Fatalf("valid history rejected: %v", err)
	}
	if _, err := loadAPIVerified(r.s.run, ""); err != nil {
		t.Fatal(err)
	}
}

func TestUndoRejectsMiddleEventMemoryMismatch(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	r.review(undoCaseA, RoleClinician, "c1", "adequate")
	r.review(undoCaseB, RoleReviewer, "r1", "different")
	// memory differs from a valid disk ledger only in a middle hash: same length, same last hash
	r.s.reviews[1] = map[string]interface{}{"hash": "ffff", "case": undoCaseA}
	before := readFile(t, r.ledgerPath())
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 503, m, "middle mismatch")
	if readFile(t, r.ledgerPath()) != before {
		t.Fatal("ledger modified")
	}
}

func TestUndoAuthorizationBeforeTargetLookup(t *testing.T) {
	r := newUndoRig(t)
	tB := r.review(undoCaseB, RoleReviewer, "r1", "same")
	tA := r.review(undoCaseA, RoleReviewer, "r1", "same")
	for _, role := range []string{"", "nobody", RoleAuditor} {
		for _, tg := range []string{tA, tB, "deadbeef"} {
			code, m := r.undo(undoCaseA, role, "x9", `{"target":"`+tg+`"}`)
			if code != 403 {
				t.Errorf("role %q target %.8s: want 403, got %d", role, tg, code)
			}
			if b, _ := json.Marshal(m); strings.Contains(string(b), tg) {
				t.Errorf("role %q: response echoes the target: %s", role, b)
			}
		}
	}
	if n := len(r.ledgerLines()); n != 2 {
		t.Errorf("appended: %d", n)
	}
}

func TestContestRejectedCannotBeContestedAgain(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	_, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true}`)
	c := m["event"].(map[string]interface{})["hash"].(string)
	code, m := r.undo(undoCaseA, RoleReviewer, "r3", `{"target":"`+c+`","resolution":"rejected"}`)
	r.wantCode(code, 201, m, "reject")
	n := len(r.ledgerLines())
	code, m = r.undo(undoCaseA, RoleReviewer, "r4", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 409, m, "re-contest after rejection")
	if len(r.ledgerLines()) != n {
		t.Error("re-contest appended")
	}
	code, m = r.undo(undoCaseA, RoleReviewer, "r4", `{"target":"`+target+`"}`)
	r.wantCode(code, 201, m, "plain undo still allowed after rejected contest")
}

func TestUndoAppendFailureLeavesNoTraceAndReloadRejectsTornLine(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	before := readFile(t, r.ledgerPath())
	nMem := len(r.s.reviews)

	orig := undoAppendLedgerLine
	defer func() { undoAppendLedgerLine = orig }()
	undoAppendLedgerLine = func(path string, line []byte) error {
		os.WriteFile(path, []byte(before+string(line[:len(line)/2])), 0644) // torn half line, then failure
		return io.ErrShortWrite
	}
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 500, m, "append failure")
	if len(r.s.reviews) != nMem {
		t.Fatal("memory mutated on failed append")
	}
	// reload failure: the torn line is not accepted, and later writes are refused
	if _, err := loadAPIVerified(r.s.run, ""); err == nil {
		t.Fatal("torn line accepted on reload")
	}
	undoAppendLedgerLine = orig
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 503, m, "write after torn ledger")
}

func TestLedgerWriteInputBounds(t *testing.T) {
	r := newUndoRig(t)
	longNote := strings.Repeat("n", maxNoteRunes+1)
	code, m := r.do("POST", "/api/cases/"+undoCaseA+"/review", RoleReviewer, "r1", `{"decision":"same","note":"`+longNote+`"}`)
	r.wantCode(code, 400, m, "oversized review note")
	longActor := strings.Repeat("a", maxActorRunes+1)
	code, m = r.do("POST", "/api/cases/"+undoCaseA+"/review", RoleReviewer, longActor, `{"decision":"same"}`)
	r.wantCode(code, 400, m, "oversized review actor")
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","note":"`+longNote+`"}`)
	r.wantCode(code, 400, m, "oversized undo note")
	code, m = r.undo(undoCaseA, RoleReviewer, longActor, `{"target":"`+target+`"}`)
	r.wantCode(code, 400, m, "oversized undo actor")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}{}`)
	r.wantCode(code, 400, m, "trailing undo body")
	if len(r.ledgerLines()) != 1 {
		t.Fatal("invalid input reached the ledger")
	}
}

func TestUndoRejectsUnterminatedFinalRecord(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	before := strings.TrimSuffix(readFile(t, r.ledgerPath()), "\n")
	if err := os.WriteFile(r.ledgerPath(), []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 503, m, "unterminated ledger")
	if after := readFile(t, r.ledgerPath()); after != before {
		t.Fatalf("unterminated ledger was modified: %q", after)
	}
}

func TestUndoIndeterminateAppendBlocksEveryWriter(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	orig := undoAppendLedgerLine
	undoAppendLedgerLine = func(string, []byte) error { return errLedgerIndeterminate }
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 500, m, "indeterminate undo append")
	undoAppendLedgerLine = orig
	if r.s.ledgerErr == "" {
		t.Fatal("indeterminate undo append did not mark the shared ledger failed")
	}
	code, m = r.do("POST", "/api/cases/"+undoCaseA+"/review", RoleReviewer, "r3", `{"decision":"same"}`)
	r.wantCode(code, 500, m, "review after indeterminate undo")
	code, m = r.do("POST", "/api/cases/"+undoCaseA+"/actions", RoleCoordinator, "n1", `{"expected_head":"head-`+undoCaseA+`","action":"note","text":"handoff"}`)
	r.wantCode(code, 500, m, "action after indeterminate undo")
	code, m = r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`"}`)
	r.wantCode(code, 503, m, "undo after indeterminate undo")
	if len(r.ledgerLines()) != 1 {
		t.Fatal("a failed writer appended to the ledger")
	}
}

func TestUndoAppendFailureNoOverlayMutation(t *testing.T) {
	r := newUndoRig(t)
	target := r.review(undoCaseA, RoleReviewer, "r1", "same")
	orig := undoAppendLedgerLine
	defer func() { undoAppendLedgerLine = orig }()
	undoAppendLedgerLine = func(string, []byte) error { return os.ErrPermission }
	code, m := r.undo(undoCaseA, RoleReviewer, "r2", `{"target":"`+target+`","contest":true}`)
	r.wantCode(code, 500, m, "append permission failure")
	if r.status(undoCaseA, target) != "active" || len(r.ledgerLines()) != 1 {
		t.Fatal("state changed after failed append")
	}
}

func TestAppendLedgerLineRealFilesystem(t *testing.T) {
	dir := t.TempDir()
	if err := undoAppendLedgerLine(dir, []byte("x\n")); err == nil { // a directory cannot be opened for append
		t.Fatal("expected open failure")
	}
	p := filepath.Join(dir, "l.ndjson")
	for _, l := range []string{"a\n", "b\n"} {
		if err := undoAppendLedgerLine(p, []byte(l)); err != nil {
			t.Fatal(err)
		}
	}
	if got := readFile(t, p); got != "a\nb\n" {
		t.Fatalf("got %q", got)
	}
}
