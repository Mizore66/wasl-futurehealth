package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// agentCaseSpec is one synthetic CRC loop: a closed decision, one raised link item and an optional sign-off.
type agentCaseSpec struct {
	loop, item string
	pair       [2]string
	signoff    map[string]interface{} // nil = no sign-off
	state      string
	contact    string
}

func agentLoop(n string) string { return "ahs-mushrif/ServiceRequest/AHS-sr-" + n }

func agentSignoff(item, loop, link, agentRead string, reads []string) map[string]interface{} {
	return map[string]interface{}{"item": item, "loop": loop, "kinds": []string{"link"}, "verdict": "adequate", "link": link, "evidence": "",
		"wrong": false, "role": "reviewer", "reads": reads, "sim_s": 31.1, "agent_read": agentRead, "agent_s": 6.7,
		"reviewer": "sim-reviewer", "ms": 0, "t": 1}
}

func writeAgentNDJSON(t *testing.T, path string, rows []interface{}) {
	t.Helper()
	var sb strings.Builder
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// agentRun materialises a minimal run directory (out_log, fhir_log, reviewer_decisions) for the cases.
func agentRun(t *testing.T, dir string, cases []agentCaseSpec) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, so []interface{}
	for i, c := range cases {
		st, ct := c.state, c.contact
		if st == "" {
			st = "closed"
		}
		if ct == "" {
			ct = "granted"
		}
		dec := &Decision{ID: "dec-" + c.item, LoopID: c.loop, State: st, Prog: "crc", Contact: ct, Provider: "ahs-mushrif"}
		it := &ReviewItem{ItemID: c.item, LoopID: c.loop, Kinds: []string{"link"}, Pair: c.pair, P: 0.5}
		for _, env := range []OutEnvelope{{Kind: "item", OSeq: uint64(2*i + 1), Item: it}, {Kind: "dec", OSeq: uint64(2*i + 2), Dec: dec}} {
			b, _ := json.Marshal(env)
			out = append(out, LogMsg{Seq: env.OSeq, Subject: "out", Data: b})
		}
		if c.signoff != nil {
			so = append(so, c.signoff)
		}
	}
	writeAgentNDJSON(t, filepath.Join(dir, "out_log.ndjson"), out)
	writeAgentNDJSON(t, filepath.Join(dir, "fhir_log.ndjson"), nil)
	writeAgentNDJSON(t, filepath.Join(dir, "reviewer_decisions.ndjson"), so)
}

func agentRecRow(item string, pair [2]string, label string) map[string]interface{} {
	return map[string]interface{}{"item": item, "pair": pair, "label": label, "valid": true, "model": "m-1", "calls": 2, "tokens": 99, "latency_s": 1.5,
		"citations": []map[string]interface{}{{"tool": "get_record", "arg": pair[0], "field": "dob", "value": "1976-01-01"},
			{"tool": "get_record", "arg": pair[1], "field": "dob", "value": "1976-01-01"}}}
}

func writeAgentFiles(t *testing.T, dir string, recs []interface{}, replay string) {
	t.Helper()
	writeAgentNDJSON(t, filepath.Join(dir, "agent_recs.ndjson"), recs)
	if replay != "" {
		if err := os.WriteFile(filepath.Join(dir, "agent_replay.json"), []byte(replay), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const agentReplayOK = `{"agent_reads":1,"gemini":{"calls":2},"latency_s":1.2,"prompt_path":"/secret/p","provenance":{"model":"m-1","prompt_ver":"p-v4","schema_ver":"s-v1","salt":"xyz"},"cache":"/x/y"}`

func serveAgent(t *testing.T, run string) (*apiServer, *httptest.Server) {
	t.Helper()
	s := loadAPI(run, "")
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard", s.getDashboard)
	mux.HandleFunc("/api/cases", s.getCases)
	mux.HandleFunc("/api/cases/", s.caseRoutes)
	mux.HandleFunc("/api/fixtures", s.getFixtures)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, srv
}

func agentDetail(t *testing.T, srv *httptest.Server, loop string) map[string]interface{} {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/cases/" + loop)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%s: %d", loop, resp.StatusCode)
	}
	var m map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&m)
	for _, k := range []string{"agent_investigations", "agent_provenance"} {
		if m[k] == nil {
			t.Fatalf("%s must be present and not null", k)
		}
	}
	return m
}

func invs(m map[string]interface{}) []interface{} {
	v, _ := m["agent_investigations"].([]interface{})
	return v
}

var pairAB = [2]string{"ahs-mushrif|AHS-MRN1", "noor-endo|NOO-MRN2"}

func TestAgentExactJoinAndUnconsumedExcluded(t *testing.T) {
	run := t.TempDir()
	l1, l2, l3 := agentLoop("1"), agentLoop("2"), agentLoop("3")
	// twin question: the same unordered pair raised for l1 (consumed rec) and l2 (a different item id, human only)
	rev := [2]string{pairAB[1], pairAB[0]}
	agentRun(t, run, []agentCaseSpec{
		{loop: l1, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l1, "same", "same", []string{"r1", "agent"})},
		{loop: l2, item: "rv-2", pair: rev, signoff: agentSignoff("rv-2", l2, "same", "", []string{"r1", "r2"})},
		{loop: l3, item: "rv-3", pair: [2]string{"a|1", "b|2"}, signoff: agentSignoff("rv-3", l3, "same", "", []string{"r1", "r2"})},
	})
	writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same"), agentRecRow("rv-3", [2]string{"a|1", "b|2"}, "different")}, agentReplayOK)
	_, srv := serveAgent(t, run)

	d1 := agentDetail(t, srv, l1)
	got := invs(d1)
	if len(got) != 1 {
		t.Fatalf("l1 want 1 investigation, got %v", d1["agent_investigations"])
	}
	x := got[0].(map[string]interface{})
	h, _ := x["human"].(map[string]interface{})
	reads, _ := h["reads"].([]interface{})
	if x["item"] != "rv-1" || x["label"] != "same" || len(x["citations"].([]interface{})) != 2 || h["link"] != "same" ||
		len(reads) != 2 || reads[0] != "r1" || reads[1] != "agent" {
		t.Fatalf("investigation shape: %v", x)
	}
	for _, banned := range []string{"calls", "tokens", "latency_s", "valid", "model"} {
		if _, ok := x[banned]; ok {
			t.Errorf("investigation leaks %q", banned)
		}
	}
	if got := invs(agentDetail(t, srv, l2)); len(got) != 0 {
		t.Fatalf("twin question must not get the recommendation: %v", got)
	}
	if got := invs(agentDetail(t, srv, l3)); len(got) != 0 {
		t.Fatalf("unconsumed rec attached: %v", got)
	}
}

func TestAgentOrdinaryRunEmpty(t *testing.T) {
	run := t.TempDir()
	l := agentLoop("1")
	agentRun(t, run, []agentCaseSpec{{loop: l, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l, "same", "", []string{"r1", "r2"})}})
	_, srv := serveAgent(t, run)
	d := agentDetail(t, srv, l)
	if v, ok := d["agent_investigations"].([]interface{}); !ok || len(v) != 0 {
		t.Fatalf("want [], got %v", d["agent_investigations"])
	}
	p, _ := d["agent_provenance"].(map[string]interface{})
	if p == nil || p["cached"] != false || p["model"] != nil {
		t.Fatalf("ordinary provenance: %v", d["agent_provenance"])
	}
	_, f := getJ(t, srv.URL+"/api/fixtures")
	if _, ok := f["fixtures"].(map[string]interface{})["agent_review"]; ok {
		t.Fatal("agent_review fixture without agent files")
	}
	// only one of the two files present: still ordinary
	writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same")}, "")
	_, srv2 := serveAgent(t, run)
	if got := invs(agentDetail(t, srv2, l)); len(got) != 0 {
		t.Fatal("recs without replay file must be ordinary")
	}
}

func TestAgentMalformedAndMismatchedFailClosed(t *testing.T) {
	l1, l2 := agentLoop("1"), agentLoop("2")
	pair2 := [2]string{"a|1", "b|2"}
	base := []agentCaseSpec{
		{loop: l1, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l1, "same", "same", []string{"r1", "agent"})},
		{loop: l2, item: "rv-2", pair: pair2, signoff: agentSignoff("rv-2", l2, "same", "same", []string{"r1", "agent"})},
	}
	good := func() []interface{} {
		return []interface{}{agentRecRow("rv-1", pairAB, "same"), agentRecRow("rv-2", pair2, "same")}
	}
	noAgent := func(t *testing.T, srv *httptest.Server) {
		for _, l := range []string{l1, l2} {
			d := agentDetail(t, srv, l)
			if len(invs(d)) != 0 {
				t.Fatalf("expected no investigations: %v", d["agent_investigations"])
			}
			if p := d["agent_provenance"].(map[string]interface{}); p["cached"] != false {
				t.Fatalf("expected ordinary provenance: %v", p)
			}
		}
	}
	t.Run("sanity good", func(t *testing.T) {
		run := t.TempDir()
		agentRun(t, run, base)
		writeAgentFiles(t, run, good(), agentReplayOK)
		_, srv := serveAgent(t, run)
		if len(invs(agentDetail(t, srv, l1))) != 1 || len(invs(agentDetail(t, srv, l2))) != 1 {
			t.Fatal("good files should project both")
		}
	})
	for name, replay := range map[string]string{"bad json": `{`, "no provenance": `{"agent_reads":1}`,
		"missing model": `{"provenance":{"prompt_ver":"p","schema_ver":"s"}}`,
		"non-string":    `{"provenance":{"model":1,"prompt_ver":"p","schema_ver":"s"}}`} {
		t.Run("replay "+name, func(t *testing.T) {
			run := t.TempDir()
			agentRun(t, run, base)
			writeAgentFiles(t, run, good(), replay)
			_, srv := serveAgent(t, run)
			noAgent(t, srv)
		})
	}
	t.Run("malformed recs line later in file", func(t *testing.T) {
		run := t.TempDir()
		agentRun(t, run, base)
		writeAgentFiles(t, run, good(), agentReplayOK)
		f, _ := os.OpenFile(filepath.Join(run, "agent_recs.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString("{not json\n")
		f.Close()
		_, srv := serveAgent(t, run)
		noAgent(t, srv) // no partial projection of the first two lines
	})
	t.Run("malformed citations", func(t *testing.T) {
		run := t.TempDir()
		agentRun(t, run, base)
		bad := agentRecRow("rv-2", pair2, "same")
		bad["citations"] = "oops"
		writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same"), bad}, agentReplayOK)
		_, srv := serveAgent(t, run)
		noAgent(t, srv)
	})
	t.Run("invalid record", func(t *testing.T) {
		run := t.TempDir()
		agentRun(t, run, base)
		bad := agentRecRow("rv-2", pair2, "same")
		bad["valid"] = false
		writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same"), bad}, agentReplayOK)
		_, srv := serveAgent(t, run)
		noAgent(t, srv)
	})
	mismatch := map[string]func() ([]agentCaseSpec, []interface{}){
		"label differs from agent_read": func() ([]agentCaseSpec, []interface{}) {
			return base, []interface{}{agentRecRow("rv-1", pairAB, "different"), agentRecRow("rv-2", pair2, "same")}
		},
		"pair differs from item": func() ([]agentCaseSpec, []interface{}) {
			return base, []interface{}{agentRecRow("rv-1", [2]string{"x|1", "y|2"}, "same"), agentRecRow("rv-2", pair2, "same")}
		},
		"signoff loop differs from queue loop": func() ([]agentCaseSpec, []interface{}) {
			b := append([]agentCaseSpec{}, base...)
			b[0].signoff = agentSignoff("rv-1", l2, "same", "same", []string{"r1", "agent"})
			return b, good()
		},
		"item not in out_log": func() ([]agentCaseSpec, []interface{}) {
			b := append([]agentCaseSpec{}, base...)
			b[0].signoff = agentSignoff("rv-ghost", l1, "same", "same", []string{"r1", "agent"})
			return b, []interface{}{agentRecRow("rv-ghost", pairAB, "same"), agentRecRow("rv-2", pair2, "same")}
		},
	}
	for name, mk := range mismatch {
		t.Run("mismatch "+name, func(t *testing.T) {
			run := t.TempDir()
			cs, recs := mk()
			agentRun(t, run, cs)
			writeAgentFiles(t, run, recs, agentReplayOK)
			_, srv := serveAgent(t, run)
			if got := invs(agentDetail(t, srv, l1)); len(got) != 0 {
				t.Fatalf("mismatched investigation attached: %v", got)
			}
		})
	}
}

func TestAgentProvenanceAllowlist(t *testing.T) {
	run := t.TempDir()
	l := agentLoop("1")
	agentRun(t, run, []agentCaseSpec{{loop: l, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l, "same", "same", []string{"r1", "agent"})}})
	writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same")}, agentReplayOK)
	_, srv := serveAgent(t, run)
	resp, err := http.Get(srv.URL + "/api/cases/" + l)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&m)
	p, _ := m["agent_provenance"].(map[string]interface{})
	want := map[string]interface{}{"cached": true, "experimental": true, "model": "m-1", "prompt_ver": "p-v4", "schema_ver": "s-v1"}
	if len(p) != len(want) {
		t.Fatalf("provenance keys: %v", p)
	}
	for k, v := range want {
		if p[k] != v {
			t.Fatalf("provenance %s = %v", k, p[k])
		}
	}
	b, _ := json.Marshal(m)
	for _, leak := range []string{"salt", "xyz", "/secret", "prompt_path", "\"cache\"", "gemini", "latency_s", "tokens"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("response leaks %q", leak)
		}
	}
}

func TestAgentMainLayoutReadsParentDir(t *testing.T) {
	root := t.TempDir()
	run := filepath.Join(root, "main")
	l := agentLoop("1")
	spec := []agentCaseSpec{{loop: l, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l, "same", "same", []string{"r1", "agent"})}}
	agentRun(t, run, spec)
	writeAgentFiles(t, root, []interface{}{agentRecRow("rv-1", pairAB, "same")}, agentReplayOK)
	_, srv := serveAgent(t, run)
	if len(invs(agentDetail(t, srv, l))) != 1 {
		t.Fatal("main run must read ../agent_recs.ndjson")
	}
	// a non-main run directory must not reach into its parent
	other := filepath.Join(root, "other")
	agentRun(t, other, spec)
	_, srv2 := serveAgent(t, other)
	if len(invs(agentDetail(t, srv2, l))) != 0 {
		t.Fatal("non-main run must not read parent files")
	}
}

func TestAgentReviewFixtureDeterministic(t *testing.T) {
	run := t.TempDir()
	ids := []string{agentLoop("1"), agentLoop("2"), agentLoop("3"), agentLoop("4"), agentLoop("5")}
	pairs := [][2]string{{"a|1", "b|1"}, {"a|2", "b|2"}, {"a|3", "b|3"}, {"a|4", "b|4"}, {"a|5", "b|5"}}
	mk := func(i int, link, st, ct string) agentCaseSpec {
		item := "rv-" + ids[i][len(ids[i])-1:]
		return agentCaseSpec{loop: ids[i], item: item, pair: pairs[i], state: st, contact: ct,
			signoff: agentSignoff(item, ids[i], link, "same", []string{"r1", "agent"})}
	}
	cs := []agentCaseSpec{
		mk(0, "same", "open", "granted"),        // not closed
		mk(1, "same", "closed", "unknown"),      // not consented
		mk(2, "different", "closed", "granted"), // final human link differs
		mk(3, "same", "closed", "granted"),      // citations removed below
		mk(4, "same", "closed", "granted"),      // eligible
	}
	agentRun(t, run, cs)
	var recs []interface{}
	for i := range cs {
		r := agentRecRow(cs[i].item, pairs[i], "same")
		if i == 3 {
			r["citations"] = []map[string]interface{}{}
		}
		recs = append(recs, r)
	}
	writeAgentFiles(t, run, recs, agentReplayOK)
	s1, _ := serveAgent(t, run)
	if s1.fixtures["agent_review"] != ids[4] {
		t.Fatalf("agent_review = %q want %q (fixtures %v)", s1.fixtures["agent_review"], ids[4], s1.fixtures)
	}
	s2, _ := serveAgent(t, run)
	if s2.fixtures["agent_review"] != s1.fixtures["agent_review"] {
		t.Fatal("fixture selection not deterministic")
	}
	if s1.cases[ids[4]].Fixture != "agent_review" {
		t.Fatalf("case tag: %q", s1.cases[ids[4]].Fixture)
	}
}

// An existing fixture's ID is never given to agent_review, and its single tag stays.
func TestAgentReviewFixtureExcludesExistingFixtureIDs(t *testing.T) {
	run := t.TempDir()
	l1, l2 := agentLoop("1"), agentLoop("2")
	cs := []agentCaseSpec{
		{loop: l1, item: "rv-1", pair: pairAB, signoff: agentSignoff("rv-1", l1, "same", "same", []string{"r1", "agent"})},
		{loop: l2, item: "rv-2", pair: [2]string{"a|2", "b|2"}, signoff: agentSignoff("rv-2", l2, "same", "same", []string{"r1", "agent"})},
	}
	agentRun(t, run, cs)
	writeAgentFiles(t, run, []interface{}{agentRecRow("rv-1", pairAB, "same"), agentRecRow("rv-2", [2]string{"a|2", "b|2"}, "same")}, agentReplayOK)
	s := loadAPI(run, "")
	if s.fixtures["agent_review"] != l1 {
		t.Fatalf("baseline: %v", s.fixtures)
	}
	// pretend l1 is already another fixture: agent_review must move to l2 and l1 keeps its tag
	s.fixtures = map[string]string{"evidence_closure": l1}
	s.cases[l1].Fixture, s.cases[l2].Fixture = "evidence_closure", ""
	s.pickAgentFixture()
	if s.fixtures["agent_review"] != l2 || s.cases[l1].Fixture != "evidence_closure" || s.cases[l2].Fixture != "agent_review" {
		t.Fatalf("fixtures %v tags %q %q", s.fixtures, s.cases[l1].Fixture, s.cases[l2].Fixture)
	}
}
