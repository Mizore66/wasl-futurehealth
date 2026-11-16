package main

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const crcProvenanceSchema = "wasl-crc-provenance/1"

// crcProvenanceBody is the FIRST-HASH boundary for new CRC provenance, not a
// migration of existing signed chains. Recursively turn typed proposals,
// contributors, plan items, nested chains and aggregate slices into independent
// JSON values before appendBlock hashes them. json.Marshal sorts every resulting
// map's keys; UseNumber retains exact integer/decimal/exponent literals. Later
// snapshots and OUT decoding therefore have exactly the same representation.
//
// The marker versions this CRC-local representation. Unrelated producers retain
// appendBlock/canon unchanged. Source events, reviews, frozen artifacts and old
// digests are never rewritten. ParentHead remains the post-signoff/disposition
// evidence head; SignedHead/ExpectedHead remain the original review head.
func crcProvenanceBody(body map[string]interface{}) map[string]interface{} {
	input := make(map[string]interface{}, len(body)+1)
	for key, value := range body {
		input[key] = value
	}
	input["crc_provenance"] = crcProvenanceSchema
	raw, err := json.Marshal(input)
	if err != nil {
		panic(fmt.Errorf("CRC provenance cannot be frozen before hashing: %w", err))
	}
	var frozen map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&frozen); err != nil {
		panic(fmt.Errorf("CRC provenance cannot be decoded before hashing: %w", err))
	}
	return frozen
}

func appendCRCBlock(chain []Block, typ string, body map[string]interface{}) []Block {
	return appendBlock(chain, typ, crcProvenanceBody(body))
}

// UnmarshalJSON preserves numeric literals in provenance bodies, including
// recursively nested evidence chains. This is wire fidelity, NOT normalization
// or a new verification rule: verifyChain still checks only the original digest
// against the current semantic Body. No raw-byte cache or alternate hash exists.
// Valid legacy JSON-shaped/root-only chains keep working. Legacy typed bodies
// whose original order was lost on serialization remain unverifiable and must
// fail closed; the decoder never reconstructs types/order or replaces hashes.
func (b *Block) UnmarshalJSON(raw []byte) error {
	type wireBlock Block
	var decoded wireBlock
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*b = Block(decoded)
	return nil
}
