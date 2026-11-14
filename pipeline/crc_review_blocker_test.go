package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func reviewAppendOUT(logs *[]LogMsg, outs []OutMsg) {
	for _, out := range outs {
		seq := uint64(len(*logs) + 1)
		*logs = append(*logs, LogMsg{Seq: seq, Subject: "out.p0", Data: canon(OutEnvelope{OSeq: seq, Dec: out.Dec, Item: out.Item})})
	}
}

func TestReviewWithdrawnCRCChildCurrentCanonicalConsent(t *testing.T) {
	for _, permission := range []string{"denied", "opted_out"} {
		t.Run(permission, func(t *testing.T) {
			logs, C, f := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
			L, loop := newLoopLinker(t)
			L.Apply(revisionConsent(10, "permit", "permit", "active", 1, 1))
			f.Contact = L.factsFor(loop).Contact
			if f.Contact != "granted" {
				t.Fatal("authorized permit was not effective")
			}
			reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "permit", Facts: f, InSeq: 10}))
			childID := ""
			for id, d := range C.Final() {
				if d.CRC != nil && d.CRC.Parent != "" && d.CRC.SourceReport == "P1" {
					childID = id
				}
			}
			if childID == "" {
				t.Fatal("signed source child missing")
			}
			signed := canon(C.Final()[childID])
			f.Down[1].Version = 2
			f.Down[1].Ev = &TwoVerdict{Kind: "pathology", Agree: true, Label: "histology", CRC: extractCRCProposal("Diagnosis: hyperplastic polyp, non-neoplastic.", "pathology")}
			reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "withdraw-source", Facts: f, InSeq: 20}))
			prior := C.Final()[childID]
			if !prior.CRC.Withdrawn || prior.Contact != "granted" {
				t.Fatal("withdrawal fixture did not preserve its original permission")
			}
			historical := canon(prior)
			if outs := L.Apply(canonicalConsent(30, "unauthorized", "ahs-mushrif", "ahs-mushrif|M1", RoleAuditor, "deny", "active", 2, 2)); len(outs) != 0 || L.factsFor(loop).Contact != "granted" {
				t.Fatal("unauthorized denial changed canonical permission")
			}
			kind := "deny"
			if permission == "denied" {
				kind = "unknown"
			} // ordinary authorized unknown is not a denial; use a fresh denial state below
			if permission == "denied" {
				// A canonical ordinary denial (no prior local permit) is also a valid
				// imported current permission; obtain it through the same RBAC fold.
				denied, deniedLoop := newLoopLinker(t)
				denied.Apply(revisionConsent(31, "authorized-denial", "deny", "active", 3, 3))
				f.Contact = denied.factsFor(deniedLoop).Contact
			} else {
				L.Apply(revisionConsent(31, "authorized-denial", kind, "active", 3, 3))
				f.Contact = L.factsFor(loop).Contact
			}
			reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "current-permission", Facts: f, InSeq: 31}))
			current := C.Final()[childID]
			if current.Contact != permission {
				t.Errorf("withdrawn child permission is stale: got=%s want=%s", current.Contact, permission)
			}
			if !bytes.Equal(historical, canon(prior)) || !bytes.Contains(signed, []byte(`"contact":"granted"`)) {
				t.Fatal("permission update edited historical signed decisions")
			}
			if !current.CRC.Withdrawn || current.CRC.ParentHead != prior.CRC.ParentHead || !crcChildSourceIdentity(current) {
				t.Fatal("permission projection lost source or withdrawal provenance")
			}
			run := writeCRCMaterialization(t, logs)
			for reload := 0; reload < 2; reload++ {
				s, err := loadAPIVerified(run, "")
				if err != nil {
					t.Fatal(err)
				}
				srv := httptest.NewServer(http.HandlerFunc(s.caseRoutes))
				code, body := coord(t, srv, url.PathEscape(childID), hb(headOf(s, childID), `"action":"contact_outcome","outcome":"no_answer","channel":"phone"`))
				if code != 409 || body["conflict"] != "consent" || body["consent"] != permission {
					t.Errorf("current-head authorized simulated contact escaped consent: %d %v", code, body)
				}
				if code, body := coord(t, srv, url.PathEscape(childID), hb(headOf(s, childID), `"action":"note","text":"source withdrawal requires review"`)); code != 201 {
					t.Fatalf("non-contact action changed: %d %v", code, body)
				}
				srv.Close()
			}
		})
	}
}

func TestReviewD3AcceptsOnlySourceBoundTypedCRCTerminals(t *testing.T) {
	for _, path := range []string{"Diagnosis: invasive adenocarcinoma.", "Diagnosis: tubular adenoma. Repeat colonoscopy in 3 years.", "Microscopic examination: benign colonic mucosa."} {
		name := path
		if path == "Microscopic examination: benign colonic mucosa." {
			name = "completed-gastroenterology-child"
		}
		t.Run(name, func(t *testing.T) {
			procedure := ""
			if path == "Microscopic examination: benign colonic mucosa." {
				procedure = "Colonoscopy: advanced to the cecum. Prep adequate. Severe ulcerative colitis biopsied. Gastroenterology follow-up required."
			}
			logs, C, f := generatedCRCMaterialization(t, path, false, procedure)
			if procedure != "" {
				// Use the existing producer's gastroenterology disposition fixture; an
				// incidental benign pathology alone need not generate this care pathway.
				f = crcSyntheticFacts("Diagnosis: ulcerative colitis; significant non-neoplastic colitis.")
				f.Down[0].Ev.CRC = nil
				C = NewClosureCore(0, nil)
				logs = nil
				initial := C.Apply(&FactsMsg{ID: "initial", Facts: f, InSeq: 1})
				reviewAppendOUT(&logs, initial)
				tick := C.Apply(&FactsMsg{ID: "tick20", IsTick: true, TickDay: 20, InSeq: 2})
				reviewAppendOUT(&logs, tick)
				item := crcDispositionItem(append(initial, tick...))
				if item == nil {
					t.Fatal("no gastroenterology confirmation")
				}
				f.Signoffs = []Signoff{{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion, Evidence: CRCSignGastro, Reviewer: "synthetic-clinician", Role: RoleClinician, SignedHead: item.Head, Purpose: CRCDispositionPurpose, ObligationID: item.ObligationID}}
				reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "signed", Facts: f, InSeq: 3}))
				accepted := df("ServiceRequest", "active", 1, "eid", false)
				accepted.EventID, accepted.Day, accepted.FollowUp = "accepted", 21, "crc-gastroenterology"
				f.Down = append(f.Down, accepted)
				reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "accepted", Facts: f, InSeq: 30}))
				reviewAppendOUT(&logs, C.Apply(&FactsMsg{ID: "tick22", IsTick: true, TickDay: 22, InSeq: 31}))
				child := C.Final()["L#crc-gastroenterology"]
				if child == nil || child.State != "closed" {
					t.Fatal("producer-created completed CRC child missing")
				}
			}
			if got := d3Prov(logs); got != "" {
				t.Errorf("untouched producer-created typed CRC was detected: %s", got)
			}
			for _, attack := range []string{"chain", "signature", "source", "terminal", "signed-head"} {
				attacked := cloneLog(logs)
				changed := false
				for i := range attacked {
					env := decodeEnv(attacked[i])
					d := env.Dec
					if d == nil || d.State != "closed" || d.CRC == nil {
						continue
					}
					switch attack {
					case "chain":
						d.Chain[len(d.Chain)-1].Hash = "broken"
					case "signature":
						d.Sig = "broken"
					case "source":
						if d.CRC.Parent == "" {
							continue
						}
						d.CRC.SourceReport = "another-source"
						d.Sig = decSig(d)
					case "terminal":
						d.Chain = appendCRCBlock(d.Chain, "unrelated_terminal", map[string]interface{}{"loop": d.LoopID})
						d.Sig = decSig(d)
					case "signed-head":
						if d.CRC.Parent == "" {
							continue
						}
						d.CRC.SignedHead = "forged"
						d.Sig = decSig(d)
					}
					encodeEnv(&attacked[i], env)
					changed = true
					break
				}
				// Treatment/surveillance children are open; their parent's typed terminal
				// is still exercised above, while child-specific attacks run on completion.
				if changed && d3Prov(attacked) == "" {
					t.Errorf("typed CRC exception concealed %s attack", attack)
				}
			}
		})
	}
}

func TestReviewMDTWorkProviderQualifiedThroughRuntimeAndServe(t *testing.T) {
	pathologyNamespacePolicy(t)
	text := "Colonoscope advanced to caecum. Good prep. Suspicious mass biopsied."
	Judge.cache[textSha(text)] = gemmaEntry{Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}
	h := newIdentityRuntime(t, "fhir", false)
	h.feed("noor-endo", "N-A", identityA, clinicalFixture("Encounter"))
	for _, provider := range []string{"skmc-endo", "noor-endo"} {
		mrn := "H-A"
		if provider == "noor-endo" {
			mrn = "N-A"
		}
		h.feed(provider, mrn, identityA, namespaceReport("R1", "colonoscopy", text, "", 1, "final", 10))
	}
	d := h.C.Final()[identityLoop]
	if d == nil || d.CRC == nil || len(d.CRC.Coordination) != 2 {
		t.Fatalf("two current suspicious procedures did not create two works: %+v", d)
	}
	ids := map[string]bool{}
	providers := map[string]bool{}
	for _, work := range d.CRC.Coordination {
		ids[work.ID] = true
		var wire map[string]interface{}
		json.Unmarshal(canon(work), &wire)
		provider, _ := wire["procedure_provider"].(string)
		providers[provider] = true
		if wire["procedure_report"] != "R1" || wire["procedure_version"] != float64(1) || provider == "" || wire["source_provider"] != provider {
			t.Errorf("MDT lost qualified procedure provenance: %s", canon(work))
		}
	}
	if len(ids) != 2 || !providers["skmc-endo"] || !providers["noor-endo"] {
		t.Errorf("independent provider work identities collapsed: ids=%v providers=%v", ids, providers)
	}
	for id := range ids {
		if !commTestContains(d.Pending, id) {
			t.Errorf("pending lost independent MDT %s", id)
		}
	}
	var logs []LogMsg
	reviewAppendOUT(&logs, h.outs)
	s, err := loadAPIVerified(writeCRCMaterialization(t, logs), "")
	if err != nil {
		t.Fatal(err)
	}
	obligations := personObligations(s.cases[identityLoop])
	body := string(canon(obligations))
	for id := range ids {
		if !bytes.Contains([]byte(body), []byte(id)) {
			t.Errorf("served obligations lost %s: %s", id, body)
		}
	}
	t.Logf("distinct MDT work=%d served obligations=%s", len(ids), fmt.Sprint(obligations))
	before := map[string]string{}
	for _, work := range d.CRC.Coordination {
		before[work.ProcedureProvider] = work.ID
	}
	h.feed("skmc-endo", "H-A", identityA, namespaceReport("R1", "colonoscopy", text, "", 2, "final", 10))
	for _, work := range h.C.Final()[identityLoop].CRC.Coordination {
		if work.ProcedureProvider == "noor-endo" && work.ID != before["noor-endo"] ||
			work.ProcedureProvider == "skmc-endo" && (work.ID == before["skmc-endo"] || work.ProcedureVersion != 2) {
			t.Fatal("a source correction changed the wrong namespace or lost its version")
		}
	}
}

func TestReviewMDTLegacyWireRemainsUnchanged(t *testing.T) {
	// Additive namespace fields do not rewrite old signed work objects when absent.
	raw := []byte(`{"id":"crc-mdt-old","kind":"mdt_coordinator","owner":"crc-mdt-coordinator","due_day":10,"source_report":"R1","source_version":1,"escalation":"crc_mdt_same_day","status":"open"}`)
	var legacy CRCWorkItem
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, canon(legacy)) {
		t.Fatal("legacy signed coordinator bytes were migrated on read")
	}
}
