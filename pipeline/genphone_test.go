package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The generator gives each person one synthetic UAE mobile and records it per registration with capture noise: most
// registrations carry it (some in +971 form), some carry none, a few carry a one-digit typo. The draw uses its own
// hashed stream, so every other field is unchanged (the existing generator and pipeline tests pin those).
func TestGeneratorEmitsNoisyUAEMobile(t *testing.T) {
	dir := genAdv(t, 0, 0)
	fs, _ := filepath.Glob(filepath.Join(dir, "fhir_*.ndjson"))
	mobile := regexp.MustCompile(`^05[024568][0-9]{7}$`)
	regs := map[string]string{}
	international := 0
	for _, f := range fs {
		h, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(h)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var ge GenEvent
			if json.Unmarshal(sc.Bytes(), &ge) != nil {
				continue
			}
			c, _ := ge.Res["contained"].([]interface{})
			if len(c) == 0 {
				continue
			}
			pt := c[0].(map[string]interface{})
			key := ""
			for _, idv := range pt["identifier"].([]interface{}) {
				if id := idv.(map[string]interface{}); regexp.MustCompile(`:mrn$`).MatchString(gs(id, "system")) {
					key = ge.Prov + "|" + gs(id, "value")
				}
			}
			ph := patientPhone(pt)
			if prev, seen := regs[key]; seen && prev != ph {
				t.Fatalf("registration %s recorded two phones: %q, %q", key, prev, ph)
			}
			regs[key] = ph
		}
		h.Close()
	}
	with := 0
	for _, ph := range regs {
		if ph == "" {
			continue
		}
		with++
		if ph[0] == '+' {
			international++
		}
		if n := normPhone(ph); !mobile.MatchString(n) {
			t.Fatalf("not a UAE mobile after normalising: %q -> %q", ph, n)
		}
	}
	share := float64(with) / float64(len(regs))
	if len(regs) < 100 || share < .6 || share > .9 || international == 0 {
		t.Fatalf("registrations %d, with phone %.2f, international %d", len(regs), share, international)
	}
}
