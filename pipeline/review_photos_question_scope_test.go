package main

import (
	"bytes"
	"net/http"
	"testing"
)

// These are serialization/authorization controls over the existing synthetic
// photoEnv, not clinical image or source-association acceptance. They deliberately
// use the real authenticated claim/read handler rather than inventing read state.
func photoQuestionStoredSnapshot(e *photoEnv) []byte {
	return canon(map[string]interface{}{
		"current": e.s.cases[ledgerLoop].last,
		"queue":   e.s.queue[ledgerLoop],
	})
}

func TestImageModelFirstReadDoesNotUnlockOtherQuestions(t *testing.T) {
	e := newPhotoEnv(t)
	before := photoQuestionStoredSnapshot(e)
	if !e.itemHasVerdict(t, photoTokRev, "rv-other") {
		t.Fatal("positive control: the other report has no model verdict to conceal")
	}
	for _, item := range []string{"rv-one", "rv-comp", "rv-other"} {
		if e.itemHasVerdict(t, photoTokClin, item) {
			t.Fatalf("before reading, evidence question %s exposes its image model", item)
		}
	}

	r1 := e.must(t, photoTokClin, irBody("rv-one", "adequate", "r1", ""), 201)
	if !e.itemHasVerdict(t, photoTokClin, "rv-one") {
		t.Error("the first reader must retain access to the model for their own answered question")
	}
	// rv-comp is another question on the same report; rv-other is another
	// report's question. Neither received a read by this clinician.
	for _, item := range []string{"rv-comp", "rv-other"} {
		if e.itemHasVerdict(t, photoTokClin, item) {
			t.Errorf("reading rv-one exposed the model of unread evidence question %s", item)
		}
	}

	// Revealed agreement on the older question must not become a case-wide
	// permission for either reader to inspect a different unread question.
	e.must(t, irClinB, irBody("rv-one", "adequate", "r2", stateOf(r1)["head"].(string)), 201)
	for _, token := range []string{photoTokClin, irClinB} {
		if !e.itemHasVerdict(t, token, "rv-one") {
			t.Error("agreed-question positive control lost its model visibility")
		}
		for _, item := range []string{"rv-comp", "rv-other"} {
			if e.itemHasVerdict(t, token, item) {
				t.Errorf("resolved rv-one exposed the model of unread evidence question %s", item)
			}
		}
	}
	if !bytes.Equal(before, photoQuestionStoredSnapshot(e)) {
		t.Fatal("derived model concealment or protocol reads changed canonical decisions/item chains")
	}
}

// An unmatched claimed version is intentionally INVALID source context. It
// must not acquire pixels from the legacy report index. This negative boundary
// control does not claim a genuine version-2 producer or clinical photo result.
func TestReviewPhotoUnmatchedVersionDoesNotAuthorizePixels(t *testing.T) {
	e := newPhotoEnv(t)
	if response := e.get(photoTokClin, e.photoURL("rv-comp", e.caecum)); response.Code != http.StatusOK {
		t.Fatalf("positive software-authorization control: status=%d", response.Code)
	}
	found := false
	for index, item := range e.s.queue[ledgerLoop] {
		if item.ItemID == "rv-comp" {
			invalid := *item
			invalid.ReportID = "R1"
			invalid.ReportVersion = 2 // only the legacy source was filed by photoEnv
			e.s.queue[ledgerLoop][index] = &invalid
			found = true
			break
		}
	}
	if !found {
		t.Fatal("completion question is missing")
	}
	before := photoQuestionStoredSnapshot(e)
	response := e.get(photoTokClin, e.photoURL("rv-comp", e.caecum))
	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte("PNG")) {
		t.Errorf("unmatched source version authorized legacy pixels: status=%d", response.Code)
	}
	if !bytes.Equal(before, photoQuestionStoredSnapshot(e)) {
		t.Fatal("photo refusal changed canonical decisions or item chains")
	}
}

func TestImageModelUnrelatedTextReadDoesNotUnlockPhotoQuestion(t *testing.T) {
	e := newPhotoEnv(t)
	before := photoQuestionStoredSnapshot(e)
	// irEvItem is the existing text-only evidence question from irRun, not one
	// of the photo questions appended by newPhotoEnv.
	if !e.itemHasVerdict(t, photoTokRev, "rv-comp") {
		t.Fatal("positive control: completion item has no model verdict")
	}
	e.must(t, photoTokClin, irBody(irEvItem, "adequate", "r1", ""), 201)
	for _, item := range []string{"rv-one", "rv-comp", "rv-other"} {
		if e.itemHasVerdict(t, photoTokClin, item) {
			t.Errorf("text-only read exposed the model of unread photo question %s", item)
		}
	}
	if !bytes.Equal(before, photoQuestionStoredSnapshot(e)) {
		t.Fatal("text-only protocol read changed canonical decisions/item chains")
	}
}
