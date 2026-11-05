package main

import "testing"

func TestFairPrepIsInadequate(t *testing.T) {
	v := RegexReader("The colonoscope was advanced to the cecum. Preparation was fair allowing examination of 85% of mucosa after washing.", "colonoscopy")
	if v.Prep != "inadequate" || v.State != "not_closed" || v.Reason != "inadequate_prep" {
		t.Fatalf("fair prep verdict = %+v", v)
	}
}
