package main

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// "İ" lower-cases to a longer UTF-8 form, and a middle-name initial can be a non-ASCII letter: the v2 Latin noise must
// neither panic nor cut a rune in half.
func TestNoisyLatin2DottedCapitalI(t *testing.T) {
	for _, name := range []string{"Mustafa Osman İsmail", "Mohammed Nabawi İsmail", "İbrahim Dakkak", "İsa Huso", "Mohammed ʻAbd al-Muʼadab", "José Ángel Ñúñez"} {
		for seed := int64(0); seed < 2000; seed++ {
			if out := noisyLatin2(name, rand.New(rand.NewSource(seed))); !utf8.ValidString(out) {
				t.Fatalf("%q seed %d: invalid UTF-8 %q", name, seed, out)
			}
		}
	}
	if got := foldIndex("İsmail Mousa", "ou"); got != strings.Index("İsmail Mousa", "ou") {
		t.Fatalf("foldIndex = %d", got)
	}
	if foldIndex("ABOU", "ou") != 2 || foldIndex("abc", "ou") != -1 {
		t.Fatal("foldIndex must match case-insensitively and return -1 when absent")
	}
}
