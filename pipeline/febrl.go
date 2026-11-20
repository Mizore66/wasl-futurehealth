package main

// FEBRL4 benchmark adapter (evidence only; not part of the runtime path).
//
//   wsi febrl -a dataset4a.csv -b dataset4b.csv -wasl ../models -config names_dob|names_dob_ssid -out result.json
//
// FEBRL4 (Christen, Febrl; shipped with the `recordlinkage` Python package) is two files of 5,000 synthetic Australian
// person records with a known 1:1 mapping (rec-N-org in A <-> rec-N-dup-0 in B). The team did not make it.
//
// What runs unchanged: the name encoder (models/encoder_weights.json via Encoder.Embed), the runtime registration
// identity (registrationIdentity: latinForm, tokensOf, skeleton, Luhn check), candidate generation (LinkerCore.candidates:
// exact top-K=10 cosine + skeleton rule keys capped at 300 + Emirates-ID index), the 30 pair features (pairFeats), the
// LightGBM scorer (models/lgb_stream.json), the union threshold LinkThr=0.7, the review floor LinkReview=0.2, and the
// union-find merge rules (cannot-links, the cluster guard, the Emirates-ID cluster clash) of LinkerCore.newRecord/Link.
//
// What is adapted (the field mapping, nothing in the model):
//   - name   = given_name + " " + surname (FEBRL is lower-case Latin; a missing part is dropped)
//   - DOB    = date_of_birth YYYYMMDD -> YYYY-MM-DD when it is 8 digits, else missing
//   - sex    = missing on every record (FEBRL has no sex field; both sides "" makes the sex-agreement feature 1)
//   - ID     = config names_dob: missing. FEBRL's soc_sec_id is a 7-digit number; Wasl's ID features and rules only
//              accept a 15-digit, 784-prefixed, Luhn-valid Emirates ID, so it cannot sit in that slot as it is.
//              config names_dob_ssid: soc_sec_id wrapped into a synthetic Emirates-ID-shaped number
//              ("7840000" + 7 digits + Luhn check digit), so the runtime ID rules apply to it: equal -> forced link,
//              both valid and different -> cannot-link. This is an adaptation, reported separately.
//   - address, suburb, postcode, state: not used (Wasl has no address fields).
//   - Records arrive in file order, all of A, then all of B (Wasl links incrementally in arrival order).
// The isotonic calibrators (iso.json, iso_v7.json) are not part of the union decision (they calibrate closure
// confidence), so they do not change these numbers.
import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type febrlRec struct {
	ID, Given, Surname, DOB, SSID string
}

// readFebrl reads a FEBRL CSV (header row; fields may carry a leading space).
func readFebrl(r io.Reader) ([]febrlRec, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty FEBRL file")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(h)] = i
	}
	for _, need := range []string{"rec_id", "given_name", "surname", "date_of_birth", "soc_sec_id"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("FEBRL header lacks %q", need)
		}
	}
	get := func(row []string, k string) string {
		if i := col[k]; i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	out := make([]febrlRec, 0, len(rows)-1)
	for _, row := range rows[1:] {
		out = append(out, febrlRec{ID: get(row, "rec_id"), Given: get(row, "given_name"), Surname: get(row, "surname"),
			DOB: get(row, "date_of_birth"), SSID: get(row, "soc_sec_id")})
	}
	return out, nil
}

func readFebrlFile(path string) ([]febrlRec, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readFebrl(f)
}

// febrlEntity: rec-1070-org and rec-1070-dup-0 are the same entity "rec-1070".
func febrlEntity(id string) string {
	if i := strings.Index(id, "-org"); i > 0 {
		return id[:i]
	}
	if i := strings.Index(id, "-dup"); i > 0 {
		return id[:i]
	}
	return id
}

func febrlDOB(s string) string {
	if len(s) != 8 {
		return ""
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return ""
		}
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:]
}

// luhnCheckDigit returns the digit that makes body+digit pass luhnOK (gen.go).
func luhnCheckDigit(body string) byte {
	for d := byte('0'); d <= '9'; d++ {
		if luhnOK(body + string(d)) {
			return d
		}
	}
	return '0'
}

// febrlEID wraps a FEBRL soc_sec_id into a synthetic 15-digit, 784-prefixed, Luhn-valid number (config names_dob_ssid).
// Anything that is not 1..7 digits stays missing.
func febrlEID(ssid string) string {
	if ssid == "" || len(ssid) > 7 {
		return ""
	}
	for _, c := range ssid {
		if c < '0' || c > '9' {
			return ""
		}
	}
	body := "7840000" + strings.Repeat("0", 7-len(ssid)) + ssid
	return body + string(luhnCheckDigit(body))
}

func febrlEvent(r febrlRec, side, config string) *Event {
	name := strings.TrimSpace(strings.Join(strings.Fields(r.Given+" "+r.Surname), " "))
	e := &Event{PatientKey: side + "|" + r.ID, Provider: "febrl-" + side, Name: name, Dob: febrlDOB(r.DOB)}
	if config == "names_dob_ssid" {
		e.Eid = febrlEID(r.SSID)
	}
	return e
}

type febrlRate struct {
	K     int     `json:"k"`
	N     int     `json:"n"`
	Value float64 `json:"value"`
	Low   float64 `json:"low"`
	High  float64 `json:"high"`
}

// wilson95 is the Wilson score 95% interval for k/n.
func wilson95(k, n int) febrlRate {
	if n == 0 {
		return febrlRate{K: k, N: n, Value: math.NaN(), Low: math.NaN(), High: math.NaN()}
	}
	z := 1.959963984540054
	p, nn := float64(k)/float64(n), float64(n)
	c := (p + z*z/(2*nn)) / (1 + z*z/nn)
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / (1 + z*z/nn)
	r := func(x float64) float64 { return math.Round(x*1e6) / 1e6 }
	return febrlRate{K: k, N: n, Value: r(p), Low: r(math.Max(0, c-h)), High: r(math.Min(1, c+h))}
}

type febrlMetrics struct {
	Predicted       int       `json:"predicted_pairs"`
	TP              int       `json:"true_positives"`
	FP              int       `json:"false_positives"`
	FN              int       `json:"false_negatives"`
	Precision       febrlRate `json:"precision"`
	Recall          febrlRate `json:"recall"`
	F1              float64   `json:"f1"`
	FalseMatchRate  febrlRate `json:"false_match_rate"`  // B records with at least one wrong A partner / B records
	MissedMatchRate febrlRate `json:"missed_match_rate"` // B records whose true A partner is not predicted / B records with one
}

// febrlScore turns a set of predicted cross-file pairs (A index, B index) into metrics.
func febrlScore(pred map[[2]int]bool, A, B []febrlRec) febrlMetrics {
	aByEnt := map[string]int{}
	for i, r := range A {
		aByEnt[febrlEntity(r.ID)] = i
	}
	trueA := make([]int, len(B))
	nTrue := 0
	for j, r := range B {
		trueA[j] = -1
		if i, ok := aByEnt[febrlEntity(r.ID)]; ok {
			trueA[j] = i
			nTrue++
		}
	}
	m := febrlMetrics{Predicted: len(pred)}
	wrongB, hitB := map[int]bool{}, map[int]bool{}
	for p := range pred {
		if trueA[p[1]] == p[0] {
			m.TP++
			hitB[p[1]] = true
		} else {
			m.FP++
			wrongB[p[1]] = true
		}
	}
	m.FN = nTrue - m.TP
	m.Precision = wilson95(m.TP, m.Predicted)
	m.Recall = wilson95(m.TP, nTrue)
	if m.TP > 0 {
		m.F1 = math.Round(2*m.Precision.Value*m.Recall.Value/(m.Precision.Value+m.Recall.Value)*1e6) / 1e6
	}
	m.FalseMatchRate = wilson95(len(wrongB), len(B))
	m.MissedMatchRate = wilson95(nTrue-len(hitB), nTrue)
	return m
}

type febrlSweep struct {
	Thr       float64 `json:"thr"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

type febrlResult struct {
	Config         string            `json:"config"`
	Records        [2]int            `json:"records_a_b"`
	TruePairs      int               `json:"true_pairs"`
	LinkThr        float64           `json:"link_threshold"`
	ReviewFloor    float64           `json:"review_floor"`
	Models         map[string]string `json:"models"`
	BlockingRecall febrlRate         `json:"blocking_recall"`  // true pairs that reached the scorer as candidates
	Auto           febrlMetrics      `json:"auto_link"`        // final clusters: cross-file pairs in one cluster
	AutoDirect     febrlMetrics      `json:"auto_link_direct"` // scored cross-file candidate pairs with p >= LinkThr
	AutoOrReview   febrlMetrics      `json:"auto_or_review"`   // scored cross-file candidate pairs with p >= LinkReview
	OracleSweep    []febrlSweep      `json:"oracle_threshold_sweep"`
	Stats          map[string]int    `json:"linker_stats"`
	MissingDOB     [2]int            `json:"missing_dob_a_b"`
	MissingEID     [2]int            `json:"missing_id_a_b"`
	Unchanged      []string          `json:"unchanged"`
	Adapted        []string          `json:"adapted"`
}

// runFebrlCore links A then B through the runtime LinkerCore with the given embedder and scorer.
func runFebrlCore(A, B []febrlRec, config string, embed func(string) []float32, gbm *GBM) (*febrlResult, error) {
	if config != "names_dob" && config != "names_dob_ssid" {
		return nil, fmt.Errorf("unknown -config %q (names_dob | names_dob_ssid)", config)
	}
	L := NewLinkerCore(gbm, 1)
	res := &febrlResult{Config: config, Records: [2]int{len(A), len(B)}, LinkThr: L.Thr, ReviewFloor: L.Lo}
	side := []string{}
	local := []int{}
	add := func(r febrlRec, s string, k int) {
		e := febrlEvent(r, s, config)
		var emb []float32
		if e.Name != "" {
			emb = embed(e.Name)
		}
		L.newRecord(e, emb)
		side = append(side, s)
		local = append(local, k)
		if e.Dob == "" {
			res.MissingDOB[map[string]int{"a": 0, "b": 1}[s]]++
		}
		if e.Eid == "" {
			res.MissingEID[map[string]int{"a": 0, "b": 1}[s]]++
		}
	}
	for i, r := range A {
		add(r, "a", i)
	}
	for j, r := range B {
		add(r, "b", j)
	}
	cross := func(x, y int) ([2]int, bool) {
		if side[x] == side[y] {
			return [2]int{}, false
		}
		if side[x] == "b" {
			x, y = y, x
		}
		return [2]int{local[x], local[y]}, true
	}
	// clusters
	auto := map[[2]int]bool{}
	for _, ms := range L.members {
		for _, x := range ms {
			if side[x] != "a" {
				continue
			}
			for _, y := range ms {
				if p, ok := cross(x, y); ok {
					auto[p] = true
				}
			}
		}
	}
	res.Auto = febrlScore(auto, A, B)
	// direct scored candidate pairs
	type sp struct {
		ab [2]int
		p  float64
	}
	scored := []sp{}
	for k, p := range L.pscore {
		if ab, ok := cross(k[0], k[1]); ok {
			scored = append(scored, sp{ab, p})
		}
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].ab[0] != scored[j].ab[0] {
			return scored[i].ab[0] < scored[j].ab[0]
		}
		return scored[i].ab[1] < scored[j].ab[1]
	})
	at := func(thr float64) map[[2]int]bool {
		m := map[[2]int]bool{}
		for _, s := range scored {
			if s.p >= thr {
				m[s.ab] = true
			}
		}
		return m
	}
	res.AutoDirect = febrlScore(at(L.Thr), A, B)
	res.AutoOrReview = febrlScore(at(L.Lo), A, B)
	all := map[[2]int]bool{}
	for _, s := range scored {
		all[s.ab] = true
	}
	blk := febrlScore(all, A, B)
	res.TruePairs = blk.TP + blk.FN
	res.BlockingRecall = blk.Recall
	for _, t := range []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 0.95} {
		m := febrlScore(at(t), A, B)
		res.OracleSweep = append(res.OracleSweep, febrlSweep{Thr: t, Precision: m.Precision.Value, Recall: m.Recall.Value, F1: m.F1})
	}
	res.Stats = L.Stats
	res.Unchanged = []string{
		"name encoder models/encoder_weights.json (Encoder.Embed)",
		"registration identity: latinForm, tokensOf, skeleton, Luhn check (registrationIdentity)",
		"candidate generation: exact top-10 cosine + skeleton rule keys (cap 300) + ID index (LinkerCore.candidates)",
		"30 pair features (pairFeats) and LightGBM scorer models/lgb_stream.json",
		fmt.Sprintf("union threshold %.2f (LinkThr) and review floor %.2f (LinkReview)", L.Thr, L.Lo),
		"merge rules: cannot-links, cluster guard, ID cluster clash (LinkerCore.newRecord / Link)",
	}
	res.Adapted = []string{
		"name = given_name + ' ' + surname",
		"date_of_birth YYYYMMDD -> YYYY-MM-DD (else missing)",
		"sex missing on every record (FEBRL has none)",
		"address, suburb, postcode, state unused (Wasl has no address fields)",
		"arrival order: all of A, then all of B",
	}
	if config == "names_dob_ssid" {
		res.Adapted = append(res.Adapted, "soc_sec_id wrapped as synthetic '7840000'+7 digits+Luhn digit so Emirates-ID rules apply")
	} else {
		res.Adapted = append(res.Adapted, "soc_sec_id unused: Wasl's ID slot only accepts a 15-digit 784-prefixed Luhn-valid Emirates ID")
	}
	return res, nil
}

func runFebrl(aPath, bPath, models, config, outPath string) error {
	UrduFix = true // as main.go sets it for every runtime command (the encoder was trained with it)
	A, err := readFebrlFile(aPath)
	if err != nil {
		return err
	}
	B, err := readFebrlFile(bPath)
	if err != nil {
		return err
	}
	encPath, gbmPath := filepath.Join(models, mf("encoder_weights.json")), filepath.Join(models, mf("lgb_stream.json"))
	enc := LoadEncoder(encPath)
	gbm := LoadGBM(gbmPath)
	res, err := runFebrlCore(A, B, config, enc.Embed, gbm)
	if err != nil {
		return err
	}
	res.Models = map[string]string{}
	for _, p := range []string{encPath, gbmPath, aPath, bPath} {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		res.Models[filepath.Base(p)] = sha(b)
	}
	b, _ := json.MarshalIndent(res, "", " ")
	if outPath == "" || outPath == "-" {
		_, err = os.Stdout.Write(append(b, '\n'))
		return err
	}
	return os.WriteFile(outPath, append(b, '\n'), 0644)
}
