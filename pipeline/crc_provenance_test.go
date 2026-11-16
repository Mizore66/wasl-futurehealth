package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func requireCRCChain(t *testing.T, chain []Block) {
	t.Helper()
	if len(chain) == 0 || !verifyChain(chain) {
		t.Fatal("original provenance digest does not verify")
	}
	for _, b := range chain {
		if b.Type != "crc_plan_obligation" {
			continue
		}
		var nested struct {
			Plan  CRCPlanItem `json:"plan"`
			Chain []Block     `json:"evidence_chain"`
		}
		if err := json.Unmarshal(canon(b.Body), &nested); err != nil {
			t.Fatal(err)
		}
		requireCRCChain(t, nested.Chain)
		if nested.Plan.ExpectedHead == "" {
			t.Fatal("plan lost original review head")
		}
		found := false
		for _, evidence := range nested.Chain {
			found = found || evidence.Hash == nested.Plan.ExpectedHead
		}
		if !found {
			t.Fatal("review head is not part of the preserved evidence chain")
		}
		if nested.Plan.Confirmation != "" && nested.Plan.ExpectedHead == nested.Chain[len(nested.Chain)-1].Hash {
			t.Fatal("review head collapsed into post-disposition evidence head")
		}
	}
}

func TestCRCProvenanceVerifiesInMemoryAndAcrossOUTSnapshots(t *testing.T) {
	logs, core, facts := generatedCRCMaterialization(t, "Diagnosis: invasive adenocarcinoma.", true)
	if _, err := materializationProjection(t, logs).validate(); err != nil {
		t.Fatalf("valid baseline: %v", err)
	}
	var chains [][]Block
	for _, d := range core.Final() {
		requireCRCChain(t, d.Chain)
		chains = append(chains, d.Chain)
		if d.CRC.Parent != "" && d.CRC.ParentHead == d.CRC.SignedHead {
			t.Fatal("child lost distinct review and post-disposition heads")
		}
	}
	for _, state := range core.loops {
		for _, item := range state.raised {
			if item.Reason != CRCDispositionPurpose {
				continue
			}
			requireCRCChain(t, item.Chain)
			if item.Head != item.Chain[len(item.Chain)-1].Hash {
				t.Fatal("retained in-memory review snapshot changed its signed digest")
			}
			chains = append(chains, item.Chain)
		}
	}
	for _, m := range logs {
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			t.Fatal(err)
		}
		if env.Dec != nil {
			requireCRCChain(t, env.Dec.Chain)
			chains = append(chains, env.Dec.Chain)
		}
		if env.Item != nil && env.Item.Reason == CRCDispositionPurpose {
			requireCRCChain(t, env.Item.Chain)
			if env.Item.Head != env.Item.Chain[len(env.Item.Chain)-1].Hash {
				t.Fatal("review snapshot changed the signed digest")
			}
			chains = append(chains, env.Item.Chain)
		}
	}
	// Mutating source pointers after emission must not edit any retained body.
	before := canon(chains)
	facts.Down[0].Ev.CRC.Outcome = "changed-source-after-emission"
	if !bytes.Equal(before, canon(chains)) {
		t.Fatal("CRC provenance still aliases mutable source facts")
	}
	for _, chain := range chains {
		requireCRCChain(t, chain)
		var decoded []Block
		if err := json.Unmarshal(canon(chain), &decoded); err != nil {
			t.Fatal(err)
		}
		requireCRCChain(t, decoded)
		decoded[0].Body["changed-semantic-body"] = true
		if verifyChain(decoded) {
			t.Fatal("decoded body mutation accepted using stale raw bytes")
		}
	}
	// Nested evidence is independently verifiable, not just protected by the
	// outer plan hash. Alter its body without replacing its original digest.
	parent := core.Final()["L"]
	for _, block := range parent.Chain {
		if block.Type != "crc_plan_obligation" {
			continue
		}
		var nested struct {
			Chain []Block `json:"evidence_chain"`
		}
		if err := json.Unmarshal(canon(block.Body), &nested); err != nil {
			t.Fatal(err)
		}
		requireCRCChain(t, nested.Chain)
		nested.Chain[0].Body["referral_patient"] = "forged-registration"
		if verifyChain(nested.Chain) {
			t.Fatal("nested evidence mutation accepted")
		}
	}
}

func TestCRCProvenancePreservesExactNumericLiterals(t *testing.T) {
	input := map[string]interface{}{
		"proposal": &CRCProposal{Outcome: CRCOutcomeMalignant},
		"contributors": []interface{}{map[string]interface{}{
			"large": json.Number("9007199254740993"), "decimal": json.Number("1.2300"), "exponent": json.Number("1e+09"),
		}},
	}
	chain := appendCRCBlock(nil, "crc_proposal", input)
	requireCRCChain(t, chain)
	original := canon(chain)
	for _, literal := range []string{"9007199254740993", "1.2300", "1e+09"} {
		if !bytes.Contains(original, []byte(literal)) {
			t.Fatalf("lost original numeric literal %s", literal)
		}
	}
	snapshot := snapshotCRCChain(chain)
	requireCRCChain(t, snapshot)
	var decoded []Block
	if err := json.Unmarshal(original, &decoded); err != nil {
		t.Fatal(err)
	}
	requireCRCChain(t, decoded)
	if !bytes.Equal(original, canon(snapshot)) || !bytes.Equal(original, canon(decoded)) {
		t.Fatal("snapshot or OUT decoding changed signed bytes")
	}
	input["proposal"].(*CRCProposal).Outcome = CRCOutcomeNormal
	input["contributors"].([]interface{})[0].(map[string]interface{})["large"] = json.Number("0")
	if !bytes.Equal(original, canon(chain)) {
		t.Fatal("first hash retained mutable input pointers")
	}
	decoded[0].Body["contributors"].([]interface{})[0].(map[string]interface{})["large"] = json.Number("9007199254740992")
	if verifyChain(decoded) {
		t.Fatal("one-unit numeric mutation accepted")
	}
}

func TestCRCProvenanceLegacyCompatibilityUsesOnlyOriginalDigest(t *testing.T) {
	legacy := appendBlock(nil, "closure_decision", map[string]interface{}{"loop": "legacy", "state": "closed"})
	requireCRCChain(t, legacy)
	original := canon(legacy)
	var decoded []Block
	if err := json.Unmarshal(original, &decoded); err != nil {
		t.Fatal(err)
	}
	requireCRCChain(t, decoded)
	p := newMaterializedDecisions()
	d := &Decision{LoopID: "legacy", Prog: "crc", State: "closed", Chain: decoded}
	if err := p.add(LogMsg{Seq: 1}, OutEnvelope{OSeq: 1, Dec: d}); err != nil {
		t.Fatal(err)
	}
	if counts, err := p.validate(); err != nil || counts.RootScreening != 1 || counts.DerivedCare != 0 {
		t.Fatalf("valid legacy root-only data rejected: %+v %v", counts, err)
	}
	if !bytes.Equal(original, canon(snapshotCRCChain(decoded))) {
		t.Fatal("valid legacy chain was rewritten")
	}

	// An old typed nested body can verify in its original in-memory form but
	// cannot be proved from its generic decoded representation. Do not guess
	// field order, rehash the signed body, or fabricate replacement signatures.
	// Reuse the synthetic adenoma's three-year interval: the typed proposal emits
	// outcome before interval_days, while a decoded map sorts them the other way.
	unsupported := appendBlock(nil, "evidence", map[string]interface{}{"proposal": &CRCProposal{
		Outcome: CRCOutcomeSurveillance, IntervalDays: 1095,
	}})
	requireCRCChain(t, unsupported)
	raw := canon(unsupported)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(canon(unsupported[0].Body), canon(decoded[0].Body)) {
		t.Fatal("fixture did not change body field order across serialization")
	}
	if decoded[0].Hash != unsupported[0].Hash || decoded[0].Prev != unsupported[0].Prev {
		t.Fatal("decoding rewrote the original legacy digest or predecessor")
	}
	if verifyChain(decoded) {
		t.Fatal("fixture did not expose the legacy representation mismatch")
	}
	decodedRaw := canon(decoded)
	for _, metadata := range []*CRCDecision{nil, {Kind: "screening"}} {
		p = newMaterializedDecisions()
		d = &Decision{LoopID: "legacy", Prog: "crc", CRC: metadata, Chain: legacy}
		if err := p.add(LogMsg{Seq: 1}, OutEnvelope{OSeq: 1, Dec: d}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.validate(); err != nil {
			t.Fatalf("valid legacy baseline: %v", err)
		}
		d.Chain = decoded
		if _, err := p.validate(); err == nil || !strings.Contains(err.Error(), "original provenance digest") {
			t.Fatalf("unverifiable legacy provenance not explicitly rejected: %v", err)
		}
	}
	if !bytes.Equal(raw, canon(unsupported)) || !bytes.Equal(decodedRaw, canon(decoded)) {
		t.Fatal("validation rewrote original or decoded legacy evidence")
	}
	for _, chain := range [][]Block{unsupported, decoded} {
		t.Run("snapshot-rejects-unverifiable-representation", func(t *testing.T) {
			before := canon(chain)
			defer func() {
				if recover() == nil {
					t.Error("snapshot accepted a representation whose original digest cannot survive copying")
				}
				if !bytes.Equal(before, canon(chain)) {
					t.Error("failed snapshot mutated signed evidence")
				}
			}()
			snapshotCRCChain(chain)
		})
	}
}
