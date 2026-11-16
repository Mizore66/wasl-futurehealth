package main

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonicalHex64MatchesLegacyForEveryByteAndPosition(t *testing.T) {
	legacy := func(value string) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
	}
	for position := range 64 {
		for character := range 256 {
			value := []byte(strings.Repeat("a", 64))
			value[position] = byte(character)
			s := string(value)
			if got, want := canonicalIsHex64(s), legacy(s); got != want {
				t.Fatalf("position=%d byte=%02x got=%v want=%v", position, character, got, want)
			}
		}
	}
	for _, value := range []string{"", "0", strings.Repeat("f", 63), strings.Repeat("f", 65), strings.Repeat("K", 32), strings.Repeat("ａ", 32)} {
		if canonicalIsHex64(value) != legacy(value) {
			t.Fatalf("length/Unicode acceptance changed for %q", value)
		}
	}
}

func TestCanonicalHex64ValidationAllocatesNoDecodeBuffer(t *testing.T) {
	value := strings.Repeat("a", 64)
	allocations := testing.AllocsPerRun(100, func() {
		if !canonicalIsHex64(value) {
			t.Fatal("valid lowercase digest rejected")
		}
	})
	if allocations != 0 {
		t.Fatalf("boolean digest validation allocated a decoded buffer: allocations=%v", allocations)
	}
}
