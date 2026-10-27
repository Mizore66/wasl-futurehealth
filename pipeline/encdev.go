package main

// Encoder development harnesses (used by encoder/*.py; not part of the runtime path).
//   wsi encembed -encfile <encoder.json> -in names.txt -out emb.json   Go embeddings + normalised strings, one per input line
//   wsi encnoise -wasl <data dir> -split eval -seed 7 -out pairs.json   synthetic eval protocol: one noisy Arabic + one noisy
//                                                                        Latin registration per person (gen.go noise model v1, or v2 with -noise v2)
import (
	"bufio"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
)

func runEncEmbed(encPath, inPath, outPath string) {
	enc := LoadEncoder(encPath)
	f, err := os.Open(inPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	type row struct {
		Name string    `json:"name"`
		Norm string    `json:"norm"`
		Emb  []float32 `json:"emb"`
	}
	out := []row{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		s := sc.Text()
		n := normLat(s)
		if isAr(s) {
			n = normAr(s)
		}
		out = append(out, row{s, n, enc.Embed(s)})
	}
	b, _ := json.Marshal(out)
	if err := os.WriteFile(outPath, b, 0644); err != nil {
		panic(err)
	}
}

func runEncNoise(wasl, split, outPath string, seed int64) {
	b, err := os.ReadFile(filepath.Join(wasl, "persons3.json"))
	if err != nil {
		panic(err)
	}
	var ps []Person
	if err := json.Unmarshal(b, &ps); err != nil {
		panic(err)
	}
	r := rand.New(rand.NewSource(seed))
	type pair struct {
		Qid string `json:"qid"`
		Ar  string `json:"ar"`
		Lat string `json:"lat"`
	}
	out := []pair{}
	for _, p := range ps {
		if p.Spl != split || p.Ar == "" || p.En == "" {
			continue
		}
		opts := append([]string{p.En}, p.Als...)
		src := opts[r.Intn(len(opts))]
		var a, l string
		if NoiseModel == "v2" {
			a, l = noisyArabic2(p.Ar, r), noisyLatin2(src, r)
		} else {
			a, l = noisyArabic(p.Ar, r), noisyLatin(src, r)
		}
		out = append(out, pair{p.Qid, a, l})
	}
	bb, _ := json.Marshal(out)
	if err := os.WriteFile(outPath, bb, 0644); err != nil {
		panic(err)
	}
}
