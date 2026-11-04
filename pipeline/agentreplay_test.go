package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	arPkA = "ahs-mushrif|A1"
	arPkB = "skmc-endo|H1"
)

// agentOffline is a tiny offline cohort: two registrations of one person (the second lacks a DOB, so the guarded
// linker raises a missing-demographic link question) plus a colonoscopy report that needs the link.
func agentOffline(t *testing.T) *offline {
	t.Helper()
	withDemoGuard(t, true)
	oldW, oldR := ReplayWorklist, ReuseLinkAnswer
	ReplayWorklist, ReuseLinkAnswer = true, true
	t.Cleanup(func() { ReplayWorklist, ReuseLinkAnswer = oldW, oldR })
	ps := []*Prep{
		{Kind: "event", Ev: &Event{ID: "SR1", Provider: "ahs-mushrif", RType: "ServiceRequest", PatientKey: arPkA, Name: "Lamia Eddinari", Dob: "1999-07-19", Sex: "F", Day: 1, Prog: "crc", IndexDay: 1, Category: "colonoscopy"}},
		{Kind: "event", Ev: &Event{ID: "AP1", Provider: "skmc-endo", RType: "Appointment", Status: "booked", PatientKey: arPkB, Name: "Lamia Eddinari", Sex: "F", Day: 5, Prog: "crc"}},
		{Kind: "event", Ev: &Event{ID: "DR1", Provider: "skmc-endo", RType: "DiagnosticReport", Status: "final", PatientKey: arPkB, Name: "Lamia Eddinari", Sex: "F", Day: 10, Category: "colonoscopy", ReportID: "DR1"}},
	}
	for _, pc := range Providers {
		ps = append(ps, &Prep{Kind: "eof", Tick: 120, Ev: &Event{Provider: pc.ID}})
	}
	o := &offline{gbm: constGBM(), tr: &Truth{PKQid: map[string]string{arPkA: "Q1", arPkB: "Q1"}, Reports: map[string]string{}, Loops: map[string]*LoopTruth{}},
		evPK: map[string]string{}, dob: map[string]string{}, optT: map[string]bool{}}
	for i, p := range ps {
		p.Seq = uint64(i + 1)
		o.fhir = append(o.fhir, LogMsg{Seq: p.Seq, Subject: "fhir.x.res", Hdr: map[string]string{"Wsi-Arr": "1.000000", "Nats-Msg-Id": "gen/" + itoa(i)}, Data: json.RawMessage(`{"resourceType":"Basic","id":"b` + itoa(i) + `"}`)})
	}
	o.preps = ps
	return o
}

func agentCfgFor(seed int64) ReviewerCfg {
	cfg := DefaultReviewer
	cfg.Err, cfg.Double, cfg.Seed = 0.2, "merge", seed
	return cfg
}

func agentRecsFile(t *testing.T, label string) *AgentRecs {
	t.Helper()
	ar, err := LoadAgentRecs(agentWrite(t, agentLine("x", arPkA, arPkB, label)))
	if err != nil || ar == nil {
		t.Fatal(err)
	}
	return ar
}

func readNDJSONLines(t *testing.T, p string) []map[string]interface{} {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]interface{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var m map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

func TestMaterializeAgentRunLanes(t *testing.T) {
	o := agentOffline(t)
	run := t.TempDir()
	ar := agentRecsFile(t, "same")
	if err := materializeAgentRun(o, 1, agentCfgFor(3), ar, run); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(run, "main")
	fhir, prep := readLog(filepath.Join(main, "fhir_log.ndjson")), readLog(filepath.Join(main, "prep_log.ndjson"))
	if len(fhir) == 0 || len(fhir) != len(prep) {
		t.Fatalf("fhir %d prep %d", len(fhir), len(prep))
	}
	signs := 0
	for i := range fhir {
		if fhir[i].Seq != uint64(i+1) || prep[i].Seq != fhir[i].Seq {
			t.Fatalf("sequence break at %d: %d %d", i, fhir[i].Seq, prep[i].Seq)
		}
		var p Prep
		json.Unmarshal(prep[i].Data, &p)
		if p.Seq != fhir[i].Seq {
			t.Fatalf("prep body seq %d vs %d", p.Seq, fhir[i].Seq)
		}
		if p.Kind == "signoff" {
			signs++
			if fhir[i].Subject != "fhir.review" || fhir[i].Hdr["Nats-Msg-Id"] != "signoff/"+p.So.ItemID {
				t.Fatalf("sign-off message %+v", fhir[i])
			}
			if string(fhir[i].Data) != string(signoffResource(p.So)) {
				t.Fatal("sign-off not serialized with signoffResource")
			}
		}
	}
	if signs == 0 {
		t.Fatal("fixture raised no review item")
	}
	rq := []ReviewItem{}
	b, _ := os.ReadFile(filepath.Join(main, "review_queue.json"))
	if err := json.Unmarshal(b, &rq); err != nil || len(rq) == 0 {
		t.Fatalf("review_queue: %v %d", err, len(rq))
	}
	dec := readNDJSONLines(t, filepath.Join(main, "reviewer_decisions.ndjson"))
	if len(dec) != signs {
		t.Fatalf("decisions %d signoffs %d", len(dec), signs)
	}
	seqOf := map[string]float64{}
	for i := range fhir {
		if strings.HasPrefix(fhir[i].Hdr["Nats-Msg-Id"], "signoff/") {
			seqOf[strings.TrimPrefix(fhir[i].Hdr["Nats-Msg-Id"], "signoff/")] = float64(fhir[i].Seq)
		}
	}
	agentLinks := 0
	for _, d := range dec {
		if d["t"] != seqOf[d["item"].(string)] {
			t.Fatalf("decision t %v != sign-off seq %v", d["t"], seqOf[d["item"].(string)])
		}
		if d["sim_s"] == nil || d["reads"] == nil {
			t.Fatalf("decision fields %v", d)
		}
		if k := d["kinds"].([]interface{}); k[0] == "link" {
			agentLinks++
			if d["agent_read"] != "same" || d["agent_s"].(float64) != 1.5 {
				t.Fatalf("agent fields %v", d)
			}
		}
		if !strings.HasSuffix(d["reviewer"].(string), "+agent") {
			t.Fatalf("reviewer %v", d["reviewer"])
		}
	}
	if agentLinks != 1 {
		t.Fatalf("agent link decisions %d", agentLinks)
	}
	// replaying the materialized log reproduces the written outputs: no lost, extra or mismatched decisions
	var preps []*Prep
	for _, m := range prep {
		p := &Prep{}
		json.Unmarshal(m.Data, p)
		preps = append(preps, p)
	}
	defer func(p string) { PolicyName = p }(PolicyName)
	PolicyName += "+agent"
	rp := replay(nil, preps, o.gbm, o.iso, 1, false)
	want := map[string]int{}
	for _, id := range rp.ids {
		want[id]++
	}
	got := map[string]int{}
	liveFinal := map[string]*Decision{}
	for _, m := range readLog(filepath.Join(main, "out_log.ndjson")) {
		got[m.Hdr["Nats-Msg-Id"]]++
		var oe OutEnvelope
		json.Unmarshal(m.Data, &oe)
		if oe.Dec != nil {
			liveFinal[oe.Dec.LoopID] = oe.Dec
		}
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("out_log ids differ from replay: want %d got %d", len(want), len(got))
	}
	for k, d := range rp.final {
		if liveFinal[k] == nil || liveFinal[k].Sig != d.Sig {
			t.Fatalf("final signature mismatch for %s", k)
		}
		if !strings.HasSuffix(d.Policy, "+agent") {
			t.Fatalf("policy %q not stamped", d.Policy)
		}
	}
	for _, n := range []string{"orchestrate.json", "status-linker.json", "link_funnel.ndjson"} {
		if _, err := os.Stat(filepath.Join(main, n)); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"forged.json", "truth.json", "consent_detail.json"} {
		if _, err := os.Stat(filepath.Join(main, n)); err == nil {
			t.Fatalf("%s must not be copied into the run", n)
		}
	}

}

func TestAgentReplayDeterministicAndControl(t *testing.T) {
	o := agentOffline(t)
	a, b := t.TempDir(), t.TempDir()
	cfg := agentCfgFor(5)
	for _, d := range []string{a, b} {
		if err := materializeAgentRun(o, 1, cfg, agentRecsFile(t, "same"), d); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"fhir_log.ndjson", "prep_log.ndjson", "out_log.ndjson", "reviewer_decisions.ndjson", "review_queue.json", "link_funnel.ndjson", "status-linker.json", "orchestrate.json"}
	for _, f := range files {
		x, _ := os.ReadFile(filepath.Join(a, "main", f))
		y, _ := os.ReadFile(filepath.Join(b, "main", f))
		if len(x) == 0 || !bytes.Equal(x, y) {
			t.Fatalf("%s not byte-identical (len %d/%d)", f, len(x), len(y))
		}
	}
	for _, f := range []string{"agent_need.ndjson", "agent_stats.json"} {
		x, _ := os.ReadFile(filepath.Join(a, f))
		y, _ := os.ReadFile(filepath.Join(b, f))
		if !bytes.Equal(x, y) {
			t.Fatalf("%s not byte-identical", f)
		}
	}
	var st map[string]interface{}
	x, _ := os.ReadFile(filepath.Join(a, "agent_stats.json"))
	json.Unmarshal(x, &st)
	for _, k := range []string{"eligible", "covered", "abstain", "invalid", "calls", "tokens", "latency_s", "human_seconds_saved"} {
		if _, ok := st[k].(float64); !ok {
			t.Fatalf("stat %s missing: %v", k, st)
		}
	}
	if st["eligible"].(float64) != 1 || st["covered"].(float64) != 1 {
		t.Fatalf("stats %v", st)
	}
	// control (no recs): no +agent stamp, full need file, zeros
	c := t.TempDir()
	if err := materializeAgentRun(o, 1, cfg, nil, c); err != nil {
		t.Fatal(err)
	}
	for _, d := range readNDJSONLines(t, filepath.Join(c, "main", "reviewer_decisions.ndjson")) {
		if strings.Contains(d["reviewer"].(string), "+agent") || d["agent_read"] != "" {
			t.Fatalf("control decision carries agent fields: %v", d)
		}
	}
	need, _ := os.ReadFile(filepath.Join(c, "agent_need.ndjson"))
	if strings.Count(string(need), "\n") != 1 {
		t.Fatalf("control need %q", need)
	}
	// need file with the loaded recs is empty: the pair has its terminal record
	if n, _ := os.ReadFile(filepath.Join(a, "agent_need.ndjson")); len(n) != 0 {
		t.Fatalf("covered pair still needed: %q", n)
	}
}

// ReplayTrace is nil unless set, and a set hook sees exactly what a live-style L.trace sees.
func TestAgentReplayTraceHook(t *testing.T) {
	o := agentOffline(t)
	if ReplayTrace != nil {
		t.Fatal("ReplayTrace must be nil by default")
	}
	var live []LinkTrace
	L := NewLinkerCoreANN(o.gbm, 1)
	L.trace = func(e LinkTrace) { live = append(live, e) }
	for _, p := range o.preps {
		L.Apply(p)
	}
	var hooked []LinkTrace
	ReplayTrace = func(e LinkTrace) { hooked = append(hooked, e) }
	replay(nil, o.preps, o.gbm, o.iso, 1, false)
	ReplayTrace = nil
	if len(live) == 0 || !reflect.DeepEqual(live, hooked) {
		t.Fatalf("hook differs from live-style trace:\n%+v\n%+v", live, hooked)
	}
	// a replay without the hook leaves it untouched and untraced
	replay(nil, o.preps, o.gbm, o.iso, 1, false)
	if ReplayTrace != nil {
		t.Fatal("ReplayTrace set after replay")
	}
}

// The agent flag is a parent-command flag only: it is never part of the live children's argv.
func TestAgentReplayFlagNotForwarded(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, bad := range []string{`"-agentrecs"`, `agentRecsPath)`} {
		for _, line := range strings.Split(src, "\n") {
			if strings.Contains(line, bad) && (strings.Contains(line, "Args = append") || strings.Contains(line, "Args =")) {
				t.Fatalf("agent flag appended to child argv: %s", line)
			}
		}
	}
	if !strings.Contains(src, `cmd != "agentreplay"`) || !strings.Contains(src, `case "agentreplay":`) {
		t.Fatal("agentreplay not registered or flag not restricted")
	}
	for _, a := range append(append([]string{}, govArgs...), RevArgs...) {
		if strings.Contains(a, "agent") || strings.Contains(strings.ToLower(a), "gemini") {
			t.Fatalf("live child argv leaks %q", a)
		}
	}
}

// writeTinyCohort writes a three-resource CRC cohort (a referral, a booked appointment and a closing colonoscopy
// report at a second provider, the second registration missing its DOB) plus truth.json, and returns the cohort dir.
func writeTinyCohort(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pt := func(prov, mrn, dob string) []interface{} {
		p := map[string]interface{}{"resourceType": "Patient", "id": "pt", "gender": "female",
			"identifier": []interface{}{map[string]string{"system": "urn:ae:" + prov + ":mrn", "value": mrn}},
			"name":       []interface{}{map[string]string{"text": "Lamia Eddinari"}}}
		if dob != "" {
			p["birthDate"] = dob
		}
		return []interface{}{p}
	}
	subj := map[string]string{"reference": "#pt"}
	repDay := 20.0
	events := map[string][]GenEvent{
		"ahs-mushrif": {{Arrival: 1, Prov: "ahs-mushrif", MsgID: "ahs-mushrif/ServiceRequest/SR1/v1", Res: map[string]interface{}{
			"resourceType": "ServiceRequest", "id": "SR1", "status": "active", "intent": "order", "authoredOn": dayTS(1),
			"code": map[string]string{"text": "Colonoscopy"}, "contained": pt("ahs-mushrif", "A1", "1999-07-19"), "subject": subj}}},
		"skmc-endo": {
			{Arrival: 5, Prov: "skmc-endo", MsgID: "skmc-endo/Appointment/AP1/v1", Res: map[string]interface{}{
				"resourceType": "Appointment", "id": "AP1", "status": "booked", "start": dayTS(5),
				"serviceType": []interface{}{map[string]string{"text": "Colonoscopy"}}, "contained": pt("skmc-endo", "H1", ""), "subject": subj}},
			{Arrival: repDay, Prov: "skmc-endo", MsgID: "skmc-endo/DiagnosticReport/DR1/v1", Res: map[string]interface{}{
				"resourceType": "DiagnosticReport", "id": "DR1", "status": "final", "effectiveDateTime": dayTS(repDay),
				"conclusion": "Complete colonoscopy to the caecum, adequate bowel preparation.", "contained": pt("skmc-endo", "H1", ""), "subject": subj}},
		},
	}
	for _, pc := range Providers {
		var sb strings.Builder
		for _, g := range events[pc.ID] {
			b, _ := json.Marshal(g)
			sb.Write(b)
			sb.WriteByte('\n')
		}
		if err := os.WriteFile(filepath.Join(dir, "fhir_"+pc.ID+".ndjson"), []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tr := Truth{Loops: map[string]*LoopTruth{"ahs-mushrif/ServiceRequest/SR1": {Loop: "ahs-mushrif/ServiceRequest/SR1", Qid: "Q1", Prog: "crc", State: "closed", ClosedDay: repDay, Origin: "x", NotifDay: -1, AckDay: -1}},
		PKQid: map[string]string{arPkA: "Q1", arPkB: "Q1"}, Reports: map[string]string{"DR1": "adequate"}}
	b, _ := json.Marshal(tr)
	os.WriteFile(filepath.Join(dir, "truth.json"), b, 0o644)
	return dir
}

func dayTS(d float64) string {
	return baseTime.Add(time.Duration(d * 86400 * float64(time.Second))).Format(time.RFC3339)
}

// The materialized directory is accepted by audit() (ingest, prep, replay and final-state checks all clean) and by
// scripts/link_funnel.py.
func TestMaterializedRunAcceptedByAuditAndLinkFunnel(t *testing.T) {
	if testing.Short() {
		t.Skip("loads the real models")
	}
	withDemoGuard(t, true)
	defer func(g *Judge2, p *Parser) { Judge, Parse = g, p }(Judge, Parse)
	data, wasl, run := writeTinyCohort(t), "../models", t.TempDir()
	o := loadOffline(data, wasl)
	if err := stampOfflineMsgIDs(o, data); err != nil {
		t.Fatal(err)
	}
	if err := materializeAgentRun(o, 1, agentCfgFor(3), nil, run); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(run, "main")
	audit(data, wasl, main, 1)
	var res map[string]interface{}
	b, err := os.ReadFile(filepath.Join(main, "audit.json"))
	if err != nil || json.Unmarshal(b, &res) != nil {
		t.Fatalf("audit.json: %v", err)
	}
	cr := res["crash_recovery"].(map[string]interface{})
	for _, k := range []string{"ingest_lost", "ingest_duplicates", "duplicate_outputs", "lost_outputs", "unexpected_outputs",
		"logical_duplicate_decisions", "final_state_mismatch_loops", "prep_duplicates", "prep_lost", "prep_content_mismatch"} {
		if v, ok := cr[k].(float64); !ok || v != 0 {
			t.Errorf("audit crash_recovery.%s = %v, want 0", k, cr[k])
		}
	}
	if cr["fhir_msgs"].(float64) != cr["prep_live_msgs"].(float64) || cr["generated_resources"].(float64) != 3 {
		t.Errorf("audit counts %v", cr)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	out := filepath.Join(t.TempDir(), "funnel.json")
	cmd := exec.Command(py, "-I", "../scripts/link_funnel.py", main, data, out)
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("link_funnel.py: %v\n%s", err, msg)
	}
	var sum map[string]interface{}
	fb, _ := os.ReadFile(out)
	if err := json.Unmarshal(fb, &sum); err != nil || sum["analysis_errors"].(float64) != 0 {
		t.Fatalf("funnel summary %v err %v", sum, err)
	}
}
