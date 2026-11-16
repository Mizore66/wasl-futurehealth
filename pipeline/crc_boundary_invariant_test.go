package main

import (
	"encoding/json"
	"testing"
)

// Real linker/core source-bound benign resolution, not the gastroenterology
// child fixture and not a manually fabricated closed decision.
func boundarySignedNormal(t *testing.T, mixed bool) ([]LogMsg, *ClosureCore) {
	t.Helper()
	L, C, loop := crcLinkedCore(t)
	var logs []LogMsg
	procedure := "Colonoscopy: advanced to the cecum. Prep adequate. No polyps. Suspicious rectal stricture."
	benign := "Microscopic examination: benign colonic mucosa."
	j := &Judge2{cache: map[string]gemmaEntry{textSha(procedure): {Caecum: "yes", Prep: "adequate", Polyps: "no", Histology: "no"}, textSha(benign): {Histology: "yes"}}}
	reviewAppendOUT(&logs, crcFeed(L, C, &Prep{Seq: 3, Kind: "event", Ev: &Event{ID: "C1", ReportID: "C1", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 10, Status: "final", Text: procedure, Ev: j.Judge(procedure, "colonoscopy")}}))
	outs := crcFeed(L, C, &Prep{Seq: 4, Kind: "event", Ev: &Event{ID: "P1", ReportID: "P1", Version: 2, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "pathology", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 12, Focus: "C1", Status: "corrected", Text: benign, Ev: j.Judge(benign, "pathology")}})
	reviewAppendOUT(&logs, outs)
	if mixed {
		path := "Diagnosis: invasive adenocarcinoma."
		j.cache[textSha(path)] = gemmaEntry{Histology: "yes"}
		reviewAppendOUT(&logs, crcFeed(L, C, &Prep{Seq: 5, Kind: "event", Ev: &Event{ID: "C2", ReportID: "C2", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "colonoscopy", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 13, Status: "final", Text: procedure, Ev: j.Judge(procedure, "colonoscopy")}}))
		outs = crcFeed(L, C, &Prep{Seq: 6, Kind: "event", Ev: &Event{ID: "P2", ReportID: "P2", Version: 1, Provider: "skmc-endo", RType: "DiagnosticReport", Category: "pathology", PatientKey: "skmc-endo|H1", Name: "Test Person", Day: 14, Focus: "C2", Status: "final", Text: path, Ev: j.Judge(path, "pathology")}})
		reviewAppendOUT(&logs, outs)
	}
	outs = C.Apply(&FactsMsg{ID: "tick", IsTick: true, TickDay: 20, InSeq: 7})
	reviewAppendOUT(&logs, outs)
	// Replay current facts to obtain the actual current-head review items. Each
	// confirmation is fed through Linker authorization and ClosureCore itself.
	for n := uint64(8); n < 12; n++ {
		var item *ReviewItem
		for _, it := range crcCurrentItems(C, loop) {
			if it.Reason == CRCDispositionPurpose && it.Verdict != "" {
				item = it
				break
			}
		}
		if item == nil {
			break
		}
		reviewAppendOUT(&logs, crcFeed(L, C, &Prep{Seq: n + 20, Kind: "signoff", So: &Signoff{ItemID: item.ItemID, LoopID: item.LoopID, Kinds: []string{"evidence"}, ReportID: item.ReportID, ReportVersion: item.ReportVersion, Evidence: item.Verdict, Role: RoleClinician, Reviewer: "synthetic-clinician", SignedHead: item.Head, ObligationID: item.ObligationID, Purpose: CRCDispositionPurpose}}))
	}
	d := C.Final()[loop]
	if d == nil || d.State != "closed" || d.CRC == nil {
		t.Fatalf("signed producer normal did not close: %+v", d)
	}
	normal := false
	for _, p := range d.CRC.Obligations {
		normal = normal || p.Disposition == "closed_normal"
	}
	if !normal || !mixed && len(C.Final()) != 1 || mixed && len(d.CRC.Obligations) < 2 {
		t.Fatal("missing real no-child normal or mixed plan")
	}
	return logs, C
}

func TestBoundaryD3SignedNormalAndMixedPlans(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal-no-child", true: "mixed"}[mixed], func(t *testing.T) {
			logs, _ := boundarySignedNormal(t, mixed)
			if got := d3Prov(logs); got != "" {
				t.Errorf("untouched real signed normal rejected: %s", got)
			}
			for _, attack := range []string{"source", "signed-head", "answer", "disposition", "chain", "signature", "terminal"} {
				t.Run(attack, func(t *testing.T) {
					attacked := cloneLog(logs)
					changed := false
					for i := range attacked {
						env := decodeEnv(attacked[i])
						d := env.Dec
						if d == nil || d.State != "closed" || d.CRC == nil || d.CRC.Parent != "" {
							continue
						}
						normal := -1
						for j, p := range d.CRC.Obligations {
							if p.Disposition == "closed_normal" {
								normal = j
								break
							}
						}
						if normal < 0 {
							continue
						}
						switch attack {
						case "chain":
							d.Chain[len(d.Chain)-1].Hash = "broken"
						case "signature":
							d.Sig = "broken"
						case "terminal":
							d.Chain = appendCRCBlock(d.Chain, "unrelated_terminal", map[string]interface{}{"loop": d.LoopID})
							d.Sig = decSig(d)
						default:
							for j, b := range d.Chain {
								if b.Type != "crc_plan_obligation" {
									continue
								}
								var nested struct {
									Plan  CRCPlanItem
									Chain []Block `json:"evidence_chain"`
								}
								if json.Unmarshal(canon(b.Body), &nested) != nil || nested.Plan.ID != d.CRC.Obligations[normal].ID {
									continue
								}
								switch attack {
								case "source":
									nested.Plan.SourceReport = "foreign-source"
									d.CRC.Obligations[normal] = nested.Plan
								case "disposition":
									nested.Plan.Disposition = "generic_closed"
									d.CRC.Obligations[normal] = nested.Plan
								case "signed-head":
									nested.Chain[len(nested.Chain)-2].Body["signed_head"] = "forged"
								case "answer":
									nested.Chain[len(nested.Chain)-2].Body["evidence"] = CRCSignGastro
								}
								nested.Chain = rehashChain(nested.Chain)
								json.Unmarshal(canon(nested), &d.Chain[j].Body)
							}
							d.Chain[len(d.Chain)-1].Body["obligations"] = d.CRC.Obligations
							d.Chain = rehashChain(d.Chain)
							d.Sig = decSig(d)
						}
						encodeEnv(&attacked[i], env)
						changed = true
						break
					}
					if !changed || d3Prov(attacked) == "" {
						t.Fatalf("normal terminal hid %s mutation (changed=%v)", attack, changed)
					}
				})
			}
		})
	}
}
