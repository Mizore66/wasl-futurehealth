package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
)

type syntheticIdentity struct{ name, dob, sex, eid string }

var identityA = syntheticIdentity{"Noura Qasim", "1980-02-22", "female", luhnComplete("78419802077524")}
var identityB = syntheticIdentity{"Omar Haddad", "1995-05-19", "male", luhnComplete("78419954512367")}

const identityLoop = "ahs-mushrif/ServiceRequest/audit-SR"
const identityAdequate = "Colonoscope advanced to caecum. Good prep. No polyps."

func identityPolicy(t *testing.T) {
	t.Helper()
	d, e, v, cal, tau, policy, pn, j := DemoGuard, EIDContraGuard, V7Guard, V7Cal, TauClose, EvPolicy, PolicyName, Judge
	t.Cleanup(func() {
		DemoGuard, EIDContraGuard, V7Guard, V7Cal, TauClose, EvPolicy, PolicyName, Judge = d, e, v, cal, tau, policy, pn, j
	})
	DemoGuard, EIDContraGuard, V7Guard = true, true, true
	V7Cal, TauClose, EvPolicy = LoadCal7("../models/iso_v7.json"), TauV7Strict, "v7strict"
	PolicyName = policyName()
	Judge = &Judge2{model: "synthetic/cache", cache: map[string]gemmaEntry{textSha(identityAdequate): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no", Quotes: map[string]string{"caecum": "Colonoscope advanced to caecum", "prep": "Good prep"}}}}
	if !luhnOK(identityA.eid) || !luhnOK(identityB.eid) || identityA.eid == identityB.eid {
		t.Fatal("invalid synthetic EIDs")
	}
	if v := Judge.Judge(identityAdequate, "colonoscopy"); !v.Agree || v.Label != "adequate" {
		t.Fatalf("invalid strict cached reader fixture: %+v", v)
	}
}

type identityRuntime struct {
	t         *testing.T
	transport string
	L         *LinkerCore
	C         *ClosureCore
	fhir      []LogMsg
	preps     []*Prep
	outs      []OutMsg
	cache     sync.Map
}

func newIdentityRuntime(t *testing.T, transport string, reverse bool) *identityRuntime {
	t.Helper()
	h := &identityRuntime{t: t, transport: transport, L: NewLinkerCore(nil, 1), C: NewClosureCore(0, nil)}
	sr := clinicalFixture("ServiceRequest")
	sr["id"] = "audit-SR"
	enc := clinicalFixture("Encounter")
	enc["id"] = "audit-identity-A"
	if reverse {
		h.feed("skmc-endo", "H-A", identityA, enc)
		h.feed("ahs-mushrif", "C-A", identityA, sr)
	} else {
		h.feed("ahs-mushrif", "C-A", identityA, sr)
		h.feed("skmc-endo", "H-A", identityA, enc)
	}
	if d := h.C.Final()[identityLoop]; d == nil || d.State == "closed" {
		t.Fatalf("invalid non-closing baseline: %+v", d)
	}
	if !sameCluster(h.L, "ahs-mushrif|C-A", "skmc-endo|H-A") {
		t.Fatal("matching registrations did not link through strict identity gate")
	}
	return h
}
func (h *identityRuntime) feed(prov, mrn string, id syntheticIdentity, res map[string]interface{}) *Prep {
	h.t.Helper()
	clinicalAddPatient(res, prov, mrn)
	pt := res["contained"].([]interface{})[0].(map[string]interface{})
	pt["name"] = []interface{}{map[string]interface{}{"text": id.name}}
	pt["birthDate"], pt["gender"] = id.dob, id.sex
	pt["identifier"].([]interface{})[1].(map[string]interface{})["value"] = id.eid
	if h.transport == "hl7" {
		m, err := ParseHL7(EncodeHL7(prov, res, fmt.Sprintf("synthetic-%d", len(h.fhir)+1)))
		if err != nil {
			h.t.Fatal(err)
		}
		got, _, converted, err := m.ToResource()
		if err != nil || got != prov {
			h.t.Fatalf("HL7 conversion: %v", err)
		}
		res = converted
	}
	raw, err := json.Marshal(res)
	if err != nil {
		h.t.Fatal(err)
	}
	return h.raw(prov, raw)
}
func (h *identityRuntime) raw(prov string, raw []byte) *Prep {
	h.t.Helper()
	seq := uint64(len(h.fhir) + 1)
	subj := "fhir." + prov + ".res"
	if prov == "review" {
		subj = "fhir.review"
	}
	m := LogMsg{Seq: seq, Subject: subj, Hdr: map[string]string{"Wsi-Arr": "3"}, Data: append([]byte{}, raw...)}
	h.fhir = append(h.fhir, m)
	p := prepMsg(seq, subj, nats.Header{"Wsi-Arr": []string{"3"}}, raw, nil, &h.cache)
	if p.Skip || p.Ev == nil && p.So == nil {
		h.t.Fatalf("invalid runtime fixture: %+v", p.Reject)
	}
	h.preps = append(h.preps, p)
	for _, fm := range h.L.Apply(p) {
		h.outs = append(h.outs, h.C.Apply(&fm)...)
	}
	return p
}
func (h *identityRuntime) finishProviders() {
	h.t.Helper()
	for _, provider := range Providers {
		seq := uint64(len(h.fhir) + 1)
		m := LogMsg{Seq: seq, Subject: "fhir." + provider.ID + ".ctl", Hdr: map[string]string{"Wsi-Arr": "120"}, Data: []byte(`{"eof":true}`)}
		h.fhir = append(h.fhir, m)
		p := prepMsg(seq, m.Subject, nats.Header{"Wsi-Arr": []string{"120"}}, m.Data, nil, &h.cache)
		h.preps = append(h.preps, p)
		for _, fm := range h.L.Apply(p) {
			h.outs = append(h.outs, h.C.Apply(&fm)...)
		}
	}
}

func identityReport(id, status string, version int, text string) map[string]interface{} {
	r := clinicalFixture("DiagnosticReport")
	r["id"], r["status"] = id, status
	r["meta"].(map[string]interface{})["versionId"] = fmt.Sprint(version)
	r["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain", "data": base64.StdEncoding.EncodeToString([]byte(text))}}
	return r
}
func (h *identityRuntime) verify(materialize bool) *replayOut {
	h.t.Helper()
	// Re-read actual FHIR source bytes through the production prep path, not just
	// pre-populated facts. A second replay exercises PREP serialization as well.
	rp := replay(h.fhir, prepAll(h.fhir, nil), nil, nil, 1, false)
	serialized := replay(h.fhir, clinicalMaterializedPreps(h.t, h.preps), nil, nil, 1, false)
	for _, other := range []*replayOut{rp, serialized} {
		if !reflect.DeepEqual(h.C.Final(), other.final) || !reflect.DeepEqual(h.L.IdentityQuarantines, other.L.IdentityQuarantines) {
			h.t.Fatal("live/source/PREP replay identity disposition or clinical decision mismatch")
		}
	}
	if materialize {
		h.materialize(rp)
	}
	return rp
}
func (h *identityRuntime) materialize(rp *replayOut) *apiServer {
	h.t.Helper()
	run := h.t.TempDir()
	var out []LogMsg
	for i, o := range rp.outs {
		m := LogMsg{Seq: uint64(i + 1), Subject: "out.p0"}
		encodeEnv(&m, OutEnvelope{Kind: o.Kind, Dec: o.Dec, Item: o.Item, OSeq: o.OSeq, Prev: o.Prev, Hash: o.Hash})
		out = append(out, m)
	}
	for name, rows := range map[string][]LogMsg{"fhir_log.ndjson": h.fhir, "out_log.ndjson": out} {
		var body []byte
		for _, m := range rows {
			body = append(body, canon(m)...)
			body = append(body, '\n')
		}
		if err := os.WriteFile(filepath.Join(run, name), body, 0600); err != nil {
			h.t.Fatal(err)
		}
	}
	projection, counts, err := projectMaterializedDecisions(out)
	if err != nil {
		h.t.Fatal(err)
	}
	report := map[string]interface{}{"live_vs_replay_final_mismatch": 0, "materialized_catalog": materializedCatalogCounts(projection.final), "materialization": counts, "identity_quarantines": rp.L.identityQuarantineSnapshot()}
	for name, value := range map[string]interface{}{"dashboard.json": map[string]interface{}{"materialization": counts}, "report7.json": report, "anchors.json": map[string]interface{}{}} {
		if err := os.WriteFile(filepath.Join(run, name), canon(value), 0600); err != nil {
			h.t.Fatal(err)
		}
	}
	if err := writeIdentityClusters(run, rp); err != nil {
		h.t.Fatal(err)
	}
	s, err := loadAPIVerified(run, "")
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}
func (h *identityRuntime) assertQuarantine(p *Prep) {
	h.t.Helper()
	q := h.L.IdentityQuarantines
	if len(q) == 0 {
		h.t.Fatal("missing explicit identity-conflict disposition")
	}
	last := q[len(q)-1]
	if last.SourceSeq != p.Seq || last.SourceSHA256 != sha(canon(p.Ev)) || last.Reason != "identity_conflict" || last.Disposition != "quarantined" || len(last.Conflicts) == 0 {
		h.t.Fatalf("missing immutable source/audit proof: %+v", last)
	}
	raw := string(canon(last))
	for _, pii := range []string{identityA.name, identityB.name, identityA.dob, identityB.dob, identityA.eid, identityB.eid, "H-A"} {
		if strings.Contains(raw, pii) {
			h.t.Fatal("operational disposition leaked patient identity")
		}
	}
	if h.L.recs[h.L.byPk["skmc-endo|H-A"]].Eid != identityA.eid {
		h.t.Fatal("trusted identity overwritten")
	}
	snapshot := linkerStatus(h.L, 1, 1, 0, 0, 0, 0, 0, 0)["identity_quarantines"].([]identitySourceDisposition)
	snapshot[len(snapshot)-1].Conflicts[0] = "mutated_snapshot"
	if h.L.IdentityQuarantines[len(q)-1].Conflicts[0] == "mutated_snapshot" {
		h.t.Fatal("status document aliases mutable quarantine state")
	}
}

// The original focused red reproduction: all four independent contradictions
// falsely closed A through both FHIR and HL7 before the boundary was repaired.
func TestExistingMRNIdentityContradictions(t *testing.T) {
	identityPolicy(t)
	for _, transport := range []string{"fhir", "hl7"} {
		for _, reverse := range []bool{false, true} {
			for _, tc := range []struct {
				name, mrn string
				id        syntheticIdentity
				closed    bool
			}{
				{"matching", "H-A", identityA, true}, {"combined", "H-A", identityB, false}, {"new_MRN", "H-B", identityB, false},
				{"DOB_only", "H-A", syntheticIdentity{identityA.name, identityB.dob, identityA.sex, identityA.eid}, false},
				{"sex_only", "H-A", syntheticIdentity{identityA.name, identityA.dob, identityB.sex, identityA.eid}, false},
				{"EID_only", "H-A", syntheticIdentity{identityA.name, identityA.dob, identityA.sex, identityB.eid}, false},
			} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%v", transport, tc.name, reverse), func(t *testing.T) {
					h := newIdentityRuntime(t, transport, reverse)
					r := identityReport("audit-other-patient-report", "final", 1, identityAdequate)
					p := h.feed("skmc-endo", tc.mrn, tc.id, r)
					original := string(canon(p))
					d := h.C.Final()[identityLoop]
					if (d.State == "closed") != tc.closed {
						t.Fatalf("wrong patient report newly closed original case: state=%s", d.State)
					}
					if !tc.closed && tc.mrn == "H-A" {
						h.assertQuarantine(p)
						if len(h.L.recs) != 2 {
							t.Fatal("conflict automatically reassigned")
						}
						if h.L.recs[h.L.byPk["skmc-endo|H-A"]].evs["DiagnosticReport/"+p.Ev.ID] != nil {
							t.Fatal("conflict admitted as clinical evidence")
						}
					}
					h.raw("skmc-endo", h.fhir[len(h.fhir)-1].Data) // duplicate transport, same resource/version
					if string(canon(p)) != original {
						t.Fatal("append-only original source was mutated")
					}
					h.verify(true)
				})
			}
		}
	}
}

func TestIdentityResourcePolicyControls(t *testing.T) {
	identityPolicy(t)
	for _, transport := range []string{"fhir", "hl7"} {
		for _, tc := range []struct {
			name       string
			id         syntheticIdentity
			demo, eid  bool
			quarantine bool
		}{
			{"missing", syntheticIdentity{"", "", "", ""}, true, true, false},
			{"placeholders", syntheticIdentity{identityA.name, "1980-01-01", "unknown", ""}, true, true, false},
			{"spelling", syntheticIdentity{"Noura Qassem", identityA.dob, identityA.sex, identityA.eid}, true, true, false},
			{"name_only", syntheticIdentity{identityB.name, identityA.dob, identityA.sex, identityA.eid}, true, true, false},
			{"disabled_DOB", syntheticIdentity{identityA.name, identityB.dob, identityA.sex, identityA.eid}, false, false, false},
			{"disabled_sex", syntheticIdentity{identityA.name, identityA.dob, identityB.sex, identityA.eid}, false, false, false},
			{"base_valid_EID_conflict", syntheticIdentity{identityA.name, identityA.dob, identityA.sex, identityB.eid}, false, false, true},
			{"eidguard_existing_sex", syntheticIdentity{identityA.name, identityA.dob, identityB.sex, identityA.eid}, false, true, true},
		} {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				h := newIdentityRuntime(t, transport, false)
				DemoGuard, EIDContraGuard = tc.demo, tc.eid
				defer func() { DemoGuard, EIDContraGuard = true, true }()
				p := h.feed("skmc-endo", "H-A", tc.id, identityReport("R", "final", 1, identityAdequate))
				if (len(h.L.IdentityQuarantines) > 0) != tc.quarantine {
					t.Fatalf("existing configured policy changed: %+v", h.L.IdentityQuarantines)
				}
				if tc.quarantine {
					h.assertQuarantine(p)
				} else if h.C.Final()[identityLoop].State != "closed" {
					t.Fatal("compatible identity update stopped working")
				}
				h.verify(false)
			})
		}
	}
}

func TestIdentityResourceRevisionsAndCrossTransport(t *testing.T) {
	identityPolicy(t)
	for _, badFirst := range []bool{false, true} {
		for _, transport := range []string{"fhir", "hl7"} {
			for _, sameVersion := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/badFirst=%v/collision=%v", transport, badFirst, sameVersion), func(t *testing.T) {
					h := newIdentityRuntime(t, transport, false)
					good := identityReport("R", "final", 1, identityAdequate)
					version := 2
					if sameVersion {
						version = 1
					}
					status := "corrected"
					if sameVersion {
						status = "final"
					} // v1 corrected is invalid at normalization, not an identity-collision test
					bad := identityReport("R", status, version, identityAdequate)
					sendGood := func() { h.feed("skmc-endo", "H-A", identityA, good) }
					sendBad := func() { p := h.feed("skmc-endo", "H-A", identityB, bad); h.assertQuarantine(p) }
					if badFirst {
						sendBad()
						if h.C.Final()[identityLoop].State == "closed" {
							t.Fatal("bad revision closed before any accepted evidence")
						}
						sendGood()
					} else {
						sendGood()
						sendBad()
					}
					rec := h.L.recs[h.L.byPk["skmc-endo|H-A"]]
					if e := rec.evs["DiagnosticReport/R"]; e == nil || e.Version != 1 || e.Eid != identityA.eid {
						t.Fatal("bad raw higher version or collision selected as accepted context")
					}
					if h.C.Final()[identityLoop].State != "closed" {
						t.Fatal("legitimate accepted v1 evidence was lost")
					}
					// An alias in another transport cannot replace identity checks with body dedup.
					if transport == "fhir" {
						h.transport = "hl7"
					} else {
						h.transport = "fhir"
					}
					p := h.feed("skmc-endo", "H-A", identityB, identityReport("alias", "final", version, identityAdequate))
					h.assertQuarantine(p)
					h.verify(true)
					h.feed("skmc-endo", "H-A", identityA, identityReport("R", "entered-in-error", 3, identityAdequate))
					if h.C.Final()[identityLoop].State == "closed" {
						t.Fatal("accepted withdrawal failed to remove v1 evidence")
					}
					sendBad()
					sendGood()
					if h.C.Final()[identityLoop].State == "closed" {
						t.Fatal("stale good/bad delivery resurrected accepted withdrawal")
					}
					h.verify(true)
				})
			}
		}
	}
}

func TestCRCIdentityTemporalMaterializationIntegration(t *testing.T) {
	identityPolicy(t)
	text := "Colonoscopy: advanced to the cecum. Prep adequate. Suspicious mass biopsied. Histology confirms invasive adenocarcinoma."
	Judge.cache[textSha(text)] = gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "yes", Histology: "yes"}
	h := newIdentityRuntime(t, "fhir", false)
	h.feed("skmc-endo", "H-A", identityA, identityReport("C1", "final", 1, text))
	h.finishProviders() // persist the canonical clock before a day-20 clinician confirmation
	items := crcCurrentItems(h.C, identityLoop)
	if len(items) != 1 || items[0].Verdict != CRCSignMDT {
		t.Fatalf("embedded-histology fixture did not require source-bound MDT confirmation: %+v", items)
	}
	it := items[0]
	day := 20.0
	so := Signoff{ItemID: it.ItemID, LoopID: identityLoop, Kinds: []string{"evidence"}, ReportID: it.ReportID, ReportVersion: it.ReportVersion, Evidence: it.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: it.Head, ObligationID: it.ObligationID, Purpose: CRCDispositionPurpose, ClinicalDay: &day}
	h.raw("review", canon(map[string]interface{}{"resourceType": "Provenance", "extension": []interface{}{map[string]interface{}{"url": "urn:wsi:signoff", "valueString": string(canon(so))}}}))
	childID := identityLoop + "#crc-treatment"
	child := h.C.Final()[childID]
	if child == nil || child.CRC == nil || child.CRC.Parent != identityLoop || child.CRC.Owner == "" {
		t.Fatal("confirmed independent treatment child lost")
	}
	originalChild := string(canon(child))
	rp := h.verify(false)
	s := h.materialize(rp)
	doc := retainedCatalog(t, s)
	for _, row := range doc.Catalog {
		if row.CaseID == childID {
			ctx := row.History[len(row.History)-1]
			if row.ParentCaseID != identityLoop || row.Participation != "current" || ctx.FactsSeq == 0 || ctx.OriginDecisionID == "" || ctx.ParentDecisionID == "" {
				t.Fatal("CRC child borrowed or lost producing/signed parent context")
			}
		}
	}
	bad := clinicalFixture("ServiceRequest")
	bad["id"] = "audit-SR"
	bad["meta"].(map[string]interface{})["versionId"] = "9"
	p := h.feed("ahs-mushrif", "C-A", identityB, bad)
	h.assertQuarantine(p)
	if string(canon(child)) != originalChild {
		t.Fatal("quarantine rewrote original child provenance")
	}
	h.verify(true)
	retired := clinicalFixture("ServiceRequest")
	retired["id"], retired["status"] = "audit-SR", "revoked"
	retired["meta"].(map[string]interface{})["versionId"] = "2"
	h.feed("ahs-mushrif", "C-A", identityA, retired)
	rp = h.verify(false)
	s = h.materialize(rp)
	doc = retainedCatalog(t, s)
	if len(h.C.Final()) != 0 || len(doc.Catalog) != 2 {
		t.Fatal("retirement lost retained root/child or left actionable care")
	}
	for _, row := range doc.Catalog {
		if row.Lifecycle != "retained" || row.Participation != "reference_only" {
			t.Fatal("retired child kept current identity authority")
		}
	}
	active := clinicalFixture("ServiceRequest")
	active["id"] = "audit-SR"
	active["meta"].(map[string]interface{})["versionId"] = "3"
	h.feed("ahs-mushrif", "C-A", identityA, active)
	h.verify(true)
	h.feed("skmc-endo", "H-A", identityA, identityReport("C1", "entered-in-error", 2, text))
	withdrawn := h.C.Final()[childID]
	if withdrawn == nil || !withdrawn.CRC.Withdrawn || withdrawn.State != "review" {
		t.Fatal("source withdrawal lost independent care review")
	}
	h.verify(true)
}

func TestIdentitySourceRequestContextRejectsQuarantinedRevisions(t *testing.T) {
	identityPolicy(t)
	for _, transport := range []string{"fhir", "hl7"} {
		for _, badFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/badFirst=%v", transport, badFirst), func(t *testing.T) {
				h := newIdentityRuntime(t, transport, false)
				good := clinicalFixture("ServiceRequest")
				good["id"] = "audit-SR"
				good["meta"].(map[string]interface{})["versionId"] = "2"
				bad := clinicalFixture("ServiceRequest")
				bad["id"] = "audit-SR"
				bad["meta"].(map[string]interface{})["versionId"] = "9"
				if badFirst {
					p := h.feed("ahs-mushrif", "C-A", identityB, bad)
					h.assertQuarantine(p)
					h.feed("ahs-mushrif", "C-A", identityA, good)
				} else {
					h.feed("ahs-mushrif", "C-A", identityA, good)
					p := h.feed("ahs-mushrif", "C-A", identityB, bad)
					h.assertQuarantine(p)
				}
				h.feed("skmc-endo", "H-A", identityA, identityReport("matching", "final", 1, identityAdequate))
				rp := h.verify(false)
				s := h.materialize(rp)
				doc := retainedCatalog(t, s)
				for _, row := range doc.Catalog {
					if row.CaseID == identityLoop && row.Current.RequestVersion != 2 {
						t.Fatal("last raw bad higher version became retained selected context")
					}
				}
				if current := h.L.requests[identityLoop]; current == nil || current.version != 2 || current.event.Eid != identityA.eid {
					t.Fatal("quarantined order overwrote accepted source")
				}
			})
		}
	}
}

func TestIdentityResourceChecksAuxiliaryClinicalResourcesAndRightsWithdrawal(t *testing.T) {
	identityPolicy(t)
	for _, rt := range []string{"Encounter", "Appointment", "ServiceRequest", "Task", "Communication", "Consent"} {
		t.Run(rt, func(t *testing.T) {
			h := newIdentityRuntime(t, "fhir", false)
			r := clinicalFixture(rt)
			r["id"] = "contradicted-" + rt
			if rt == "Communication" {
				r["category"] = []interface{}{map[string]interface{}{"text": "prep-reply"}}
				r["status"] = "completed"
			}
			p := h.feed("skmc-endo", "H-A", identityB, r)
			h.assertQuarantine(p)
			if h.C.Final()[identityLoop].State == "closed" {
				t.Fatal("auxiliary resource admitted a hard identity conflict")
			}
			h.verify(false)
		})
	}
	h := newIdentityRuntime(t, "fhir", false)
	r := clinicalFixture("Consent")
	r["provision"] = map[string]interface{}{"type": "deny"}
	r["extension"] = []interface{}{map[string]interface{}{"url": "urn:wsi:actor-role", "valueString": RoleCoordinator}}
	h.feed("skmc-endo", "H-A", identityB, r)
	if len(h.L.IdentityQuarantines) != 0 {
		t.Fatal("rights withdrawal blocked by identity quarantine")
	}
	if contact, _ := h.L.clusterConsent(h.L.byPk["skmc-endo|H-A"]); contact != "denied" && contact != "opted_out" {
		t.Fatal("authorized rights withdrawal lost")
	}
	h.verify(false)
}

func TestIdentityResourceProviderIsolationAndNoOrdinaryOverride(t *testing.T) {
	identityPolicy(t)
	h := newIdentityRuntime(t, "fhir", false)
	p := h.feed("skmc-endo", "H-A", identityB, identityReport("bad", "final", 1, identityAdequate))
	h.assertQuarantine(p)
	so := Signoff{ItemID: "synthetic-ordinary", LoopID: identityLoop, Kinds: []string{"evidence"}, ReportID: "bad", ReportVersion: 1, Evidence: "adequate", Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: "synthetic-original-head"}
	if err := CheckSignoff(&so, DualMerge); err != nil {
		t.Fatalf("invalid ordinary-review negative control: %v", err)
	}
	raw := canon(map[string]interface{}{"resourceType": "Provenance", "extension": []interface{}{map[string]interface{}{"url": "urn:wsi:signoff", "valueString": string(canon(so))}}})
	h.raw("review", raw)
	if h.C.Final()[identityLoop].State == "closed" {
		t.Fatal("ordinary evidence approval blessed a hard identity conflict")
	}
	// Same MRN and resource ID under another provider remain separately scoped.
	h.feed("noor-endo", "H-A", identityB, identityReport("bad", "final", 1, identityAdequate))
	if len(h.L.recs) != 3 || h.L.byPk["noor-endo|H-A"] == h.L.byPk["skmc-endo|H-A"] {
		t.Fatal("provider scope collapsed")
	}
	if h.C.Final()[identityLoop].State == "closed" {
		t.Fatal("cross-provider identity contaminated A")
	}
	h.feed("skmc-endo", "H-A", identityA, identityReport("matching", "final", 1, identityAdequate))
	if h.C.Final()[identityLoop].State != "closed" {
		t.Fatal("matching follow-up blocked after quarantine")
	}
	h.verify(true)
}
