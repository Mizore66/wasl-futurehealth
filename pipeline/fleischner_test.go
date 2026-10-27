package main

import "testing"

// Every cell of the Fleischner 2017 table (MacMahon et al., Radiology 2017;284(1):228-243).
func TestFleischnerRecTable(t *testing.T) {
	m3, m6, m10, m12, m13 := months(3), months(6), months(10), months(12), months(13)
	cases := []struct {
		typ         string
		mm          int
		count, risk string
		action      string
		lo, hi      int
		optional    bool
		cell        string
	}{
		// solid, single, low risk: <6 none | 6-8 CT 6-12 | >8 CT 3 / PET / tissue
		{FlSolid, 4, FlSingle, "low", FlNone, 0, 0, false, "solid/single/low/<6"},
		{FlSolid, 7, FlSingle, "low", FlCT, m6, m12, false, "solid/single/low/6-8"},
		{FlSolid, 12, FlSingle, "low", FlCTPET, 0, m3, false, "solid/single/low/>8"},
		// solid, single, high risk: <6 optional CT 12 | 6-8 CT 6-12 then 18-24 | >8 as low risk
		{FlSolid, 5, FlSingle, "high", FlCT, m10, m13, true, "solid/single/high/<6"},
		{FlSolid, 6, FlSingle, "high", FlCT, m6, m12, false, "solid/single/high/6-8"},
		{FlSolid, 9, FlSingle, "high", FlCTPET, 0, m3, false, "solid/single/high/>8"},
		// solid, multiple, low risk: <6 none | 6-8 CT 3-6 | >8 CT 3-6
		{FlSolid, 3, FlMulti, "low", FlNone, 0, 0, false, "solid/multiple/low/<6"},
		{FlSolid, 8, FlMulti, "low", FlCT, m3, m6, false, "solid/multiple/low/6-8"},
		{FlSolid, 15, FlMulti, "low", FlCT, m3, m6, false, "solid/multiple/low/>8"},
		// solid, multiple, high risk: <6 optional CT 12 | 6-8 CT 3-6 | >8 CT 3-6
		{FlSolid, 5, FlMulti, "high", FlCT, m10, m13, true, "solid/multiple/high/<6"},
		{FlSolid, 7, FlMulti, "high", FlCT, m3, m6, false, "solid/multiple/high/6-8"},
		{FlSolid, 10, FlMulti, "high", FlCT, m3, m6, false, "solid/multiple/high/>8"},
		// subsolid (risk does not matter)
		{FlGG, 5, FlSingle, "high", FlNone, 0, 0, false, "subsolid/ground_glass/single/<6"},
		{FlGG, 6, FlSingle, "low", FlCT, m6, m12, false, "subsolid/ground_glass/single/>=6"},
		{FlPart, 5, FlSingle, "high", FlNone, 0, 0, false, "subsolid/part_solid/single/<6"},
		{FlPart, 9, FlSingle, "low", FlCT, m3, m6, false, "subsolid/part_solid/single/>=6"},
		{FlGG, 4, FlMulti, "low", FlCT, m3, m6, false, "subsolid/ground_glass/multiple/<6"},
		{FlPart, 4, FlMulti, "high", FlCT, m3, m6, false, "subsolid/part_solid/multiple/<6"},
		{FlGG, 11, FlMulti, "low", FlCT, m3, m6, false, "subsolid/ground_glass/multiple/>=6"},
		{FlPart, 14, FlMulti, "high", FlCT, m3, m6, false, "subsolid/part_solid/multiple/>=6"},
		// unknown risk is treated as high (conservative)
		{FlSolid, 4, FlSingle, "", FlCT, m10, m13, true, "solid/single/high/<6"},
	}
	for _, c := range cases {
		r := fleischnerRec(c.typ, c.mm, c.count, c.risk)
		if r.Action != c.action || r.WinLo != c.lo || r.WinHi != c.hi || r.Optional != c.optional || r.Cell != c.cell {
			t.Errorf("%s %dmm %s %q: got %+v, want action=%s [%d,%d] optional=%v cell=%s", c.typ, c.mm, c.count, c.risk, r, c.action, c.lo, c.hi, c.optional, c.cell)
		}
		if r.Action != FlNone && r.Next == "" {
			t.Errorf("%s: follow-up without a next step", c.cell)
		}
	}
}

func TestFleischnerSizeAndScope(t *testing.T) {
	for _, c := range []struct {
		l, s float64
		want int
	}{{6, 5, 6}, {5, 4, 5}, {4, 4, 4}, {8.4, 8.4, 8}, {9, 8, 9}, {7, 0, 7}} { // 5.5 rounds to 6 (6-8 band)
		if got := FlMeanMM(c.l, c.s); got != c.want {
			t.Errorf("FlMeanMM(%v,%v)=%d want %d", c.l, c.s, got, c.want)
		}
	}
	if ok, _ := FlApplies(34, "none"); ok {
		t.Error("age 34 must be out of scope")
	}
	if ok, why := FlApplies(60, "known_cancer"); ok || why != "known_cancer" {
		t.Error("known primary cancer must be out of scope")
	}
	if ok, _ := FlApplies(35, ""); !ok {
		t.Error("age 35 is in scope")
	}
	if !FlGrowth(5.5, 7.5) || FlGrowth(6, 7.5) {
		t.Error("growth is a mean-diameter increase > 1.5 mm")
	}
}
