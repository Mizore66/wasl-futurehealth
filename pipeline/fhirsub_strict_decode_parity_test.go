package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

// Freeze the previous decoder's acceptance/value semantics independently of
// the optimized implementation. This grants no source or publication authority.
func canonicalStrictDecodeReference(raw []byte, out any, rejectUnknown bool) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("empty value")
	}
	if err := canonicalUniqueJSON(raw); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if rejectUnknown {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing data")
	}
	return nil
}

var canonicalStrictParityTypes = []struct {
	name string
	make func() any
}{
	{"cell", func() any { return &canonicalCell{Version: 91} }},
	{"source", func() any { return &canonicalSourceRecord{Version: 92} }},
	{"pending", func() any { return &subEvent{Num: 73, Focus: "preserved"} }},
	{"checkpoint", func() any { return &subscriptionCheckpoint{HWM: 17} }},
}

func canonicalAssertDecodeParity(t testing.TB, raw []byte) {
	t.Helper()
	for _, typ := range canonicalStrictParityTypes {
		oldValue, value := typ.make(), typ.make()
		oldErr := canonicalStrictDecodeReference(raw, oldValue, true)
		err := canonicalStrictDecode(raw, value, true)
		if (oldErr == nil) != (err == nil) {
			t.Fatalf("%s acceptance changed for %q: old=%v new=%v", typ.name, raw, oldErr, err)
		}
		if err == nil && !reflect.DeepEqual(oldValue, value) {
			t.Fatalf("%s value changed for %q: old=%#v new=%#v", typ.name, raw, oldValue, value)
		}
	}
}

func TestCanonicalStrictDecodeLegacyParity(t *testing.T) {
	for _, raw := range []string{
		`{}`, `null`, `[]`, ``, `{} {}`, `{"unknown":1}`,
		`{"num":1,"body":"AP8="}`, `{"nUm":2}`, `{"num":null}`, `{"num":1.0}`,
		`{"num":1,"NUM":2}`, `{"num":1,"num":2}`,
		`{"Phase":"one","phaſe":"two"}`, `{"phaſe":"one"}`,
		`{"Attempt":{"WireHeaders":{"X":["1"],"x":["2"]}}}`,
		`{"Attempt":{"WireHeaders":{"X":["1"],"X":["2"]}}}`,
		`{"Receipt":{"Core":{"OriginalWsi":{"Wsi-X":["1"],"wsi-x":["2"]}}}}`,
		`{"Attempt":{"FrozenApplicationHeaders":{"K":["1"],"K":["2"]}}}`,
		`{"Version":1,"vErSiOn":2}`, `{"Version":1,"version":null}`,
		`{"Incarnation":{"Stream":"FHIR","Created":"2026-01-01T00:00:00Z"}}`,
		`{"Attempt":{"WireHeaders":{"X":["` + string([]byte{0xff}) + `"]}}}`,
	} {
		canonicalAssertDecodeParity(t, []byte(raw))
	}
}

// Fast-path schemas may contain only typed objects and the existing exact-key
// header maps. A future open-ended map requires the structural folding walker.
func TestCanonicalStrictDecodeSchemasHaveOnlyExactHeaderMaps(t *testing.T) {
	var check func(reflect.Type, bool)
	check = func(typ reflect.Type, exact bool) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			typ = typ.Elem()
		}
		if typ.Kind() == reflect.Map {
			if !exact || typ.Key().Kind() != reflect.String || typ.Elem().Kind() != reflect.Slice || typ.Elem().Elem().Kind() != reflect.String {
				t.Fatalf("unsupported map in strict fast-path schema: %v (exact=%v)", typ, exact)
			}
			return
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" || field.Tag.Get("json") == "-" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			check(field.Type, exact || canonicalExactJSONMaps[canonicalJSONFold(name)])
		}
	}
	for _, typ := range canonicalStrictParityTypes {
		check(reflect.TypeOf(typ.make()), false)
	}
}

func FuzzCanonicalStrictDecodeLegacyParity(f *testing.F) {
	for _, raw := range []string{`{}`, `null`, `{"num":1}`, `{"Phase":"one","phaſe":"two"}`, `{"Attempt":{"WireHeaders":{"X":["1"],"x":["2"]}}}`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 4096 {
			return
		}
		canonicalAssertDecodeParity(t, raw)
	})
}
