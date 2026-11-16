package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Separate from the generation helper so recording immutable output hashes does
// not change the helper bytes that actually produced these software controls.
func referralGenerationV2RecordIntact(record, witness, cases []byte) bool {
	return sha(record) == "013a8a109f06e3ffbc40434eb2f95f9ebf1839318e2603bbbf79d5cc76f8bd25" &&
		sha(witness) == "b569fcf1cfce8450961529a7efa0393e8076acc84625ef9c91503dd5f5f4e4e1" &&
		sha(cases) == "c56a8815bfbc3e4933ee6ed757307f1d83ecc1d68e229300eb84748433567e59"
}

func TestReferralGenerationV2RecordIntegrity(t *testing.T) {
	read := func(parts ...string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(parts...))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	record := read("..", "web", "data", "referral-inbox-provenance-v2.json")
	witness := read("testdata", "referral-inbox-generation-helper-v2.go.txt")
	cases := read("..", "web", "data", "referral-inbox-synthetic-v2.json")
	if !referralGenerationV2RecordIntact(record, witness, cases) {
		t.Fatal("immutable version2 software generation record, witness or cases changed")
	}
	for _, field := range []string{
		"kind", "producer", "source_head_at_generation", "accepted_main",
		"production_source_sha256", "helper_sha256", "inputs", "input_sha256",
		"out_log_sha256", "cases_sha256", "currents", "held_control",
		"contract_version", "supersedes_record_sha256", "generation_source_binding",
	} {
		t.Run(field, func(t *testing.T) {
			var changed map[string]interface{}
			if err := json.Unmarshal(record, &changed); err != nil {
				t.Fatal(err)
			}
			if _, ok := changed[field]; !ok {
				t.Fatalf("missing version2 mutation target: %s", field)
			}
			changed[field] = "tampered version2 generation value"
			if referralGenerationV2RecordIntact(canon(changed), witness, cases) {
				t.Fatalf("altered version2 generation field accepted: %s", field)
			}
		})
	}
	t.Run("generation_witness", func(t *testing.T) {
		changed := append([]byte(nil), witness...)
		changed[0] ^= 1
		if referralGenerationV2RecordIntact(record, changed, cases) {
			t.Fatal("altered version2 generation witness accepted")
		}
	})
	t.Run("synthetic_cases", func(t *testing.T) {
		changed := append([]byte(nil), cases...)
		changed[0] ^= 1
		if referralGenerationV2RecordIntact(record, witness, changed) {
			t.Fatal("altered version2 synthetic cases accepted")
		}
	})
}
