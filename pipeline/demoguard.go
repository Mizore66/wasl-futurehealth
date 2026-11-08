package main

// Strict demographic guard (flag -demoguard, default off; spec 2026-11-04-crc-r1photo8-demoguard-design.md).
// Under the reliable-DOB assumption two different real birth dates, or two different known sexes, are hard
// contradictions. Missing, invalid or placeholder dates and unknown sex block automatic merging but a reviewer can
// resolve them. With the flag off none of this code runs.

type demoDecision uint8

const (
	demoNeedsReview demoDecision = iota
	demoAutoOK
	demoDOBConflict
	demoSexConflict
)

// DemoGuard: flag -demoguard.
var DemoGuard bool

func usableDemoDOB(d string) bool { return realDob(d) && !placeholderDob(d) }
func knownDemoSex(s string) bool  { return s == "M" || s == "F" }

// demoClassify: DOB conflict takes precedence over sex conflict.
func demoClassify(a, b *Rec) demoDecision {
	if usableDemoDOB(a.Dob) && usableDemoDOB(b.Dob) && a.Dob != b.Dob {
		return demoDOBConflict
	}
	if knownDemoSex(a.Sex) && knownDemoSex(b.Sex) && a.Sex != b.Sex {
		return demoSexConflict
	}
	if usableDemoDOB(a.Dob) && usableDemoDOB(b.Dob) && a.Dob == b.Dob &&
		knownDemoSex(a.Sex) && knownDemoSex(b.Sex) && a.Sex == b.Sex {
		return demoAutoOK
	}
	return demoNeedsReview
}

// demoClash reports whether any cross-cluster registration pair is a DOB or sex conflict, so a record with missing
// demographics can never bridge two conflicting clusters.
func (L *LinkerCore) demoClash(ra, rb int) bool {
	for _, x := range L.members[ra] {
		for _, y := range L.members[rb] {
			if d := demoClassify(L.recs[x], L.recs[y]); d == demoDOBConflict || d == demoSexConflict {
				return true
			}
		}
	}
	return false
}

// demoteNear routes a needs-review pair to the near/review path with its own model score (never raised).
func (L *LinkerCore) demoteNear(r, o *Rec, p float64, aff map[int]bool) {
	if L.demoMissing == nil {
		L.demoMissing = map[[2]string]bool{}
	}
	L.demoMissing[pairKey(r.Pk, o.Pk)] = true
	L.emitLink(LinkTrace{A: r.Pk, B: o.Pk, P: p, Stage: "near", Reason: "demographic_missing"})
	if L.near[r.idx] == nil {
		L.near[r.idx] = map[int]float64{}
	}
	if L.near[o.idx] == nil {
		L.near[o.idx] = map[int]float64{}
	}
	L.near[r.idx][o.idx], L.near[o.idx][r.idx] = p, p
	aff[L.Find(o.idx)] = true
}

// demoGuardPolicySuffix: stamped on PolicyName so guarded and unguarded lanes are never mixed ("" when off).
func demoGuardPolicySuffix() string {
	if DemoGuard {
		return "+demoguard"
	}
	return ""
}

// demoGuardArgs: the child-process argv, only when on (default argv is unchanged).
func demoGuardArgs() []string {
	if DemoGuard {
		return []string{"-demoguard"}
	}
	return nil
}
