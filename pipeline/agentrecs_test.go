package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// luhn-valid EID helpers: build 15-digit strings and fix the check digit.
func agentEID(prefix string) string {
	s := prefix
	for len(s) < 14 {
		s += "0"
	}
	return s + string(luhnDigit(s))
}

func agentWrite(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent_recs.ndjson")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func agentLine(item, a, b, label string) string {
	return `{"item":"` + item + `","pair":["` + a + `","` + b + `"],"label":"` + label + `","valid":true,"latency_s":1.5}`
}

func agentRecsWith(t *testing.T, recs map[string]*Rec, lines ...string) *AgentRecs {
	t.Helper()
	ar, err := LoadAgentRecs(agentWrite(t, lines...))
	if err != nil || ar == nil {
		t.Fatalf("load: %v %v", ar, err)
	}
	ar.rec = func(pk string) *Rec { return recs[pk] }
	return ar
}

func linkItem(id, a, b string) *ReviewItem {
	return &ReviewItem{ItemID: id, LoopID: "L-" + id, Kinds: []string{"link"}, Pair: [2]string{a, b}, P: 0.6}
}

func TestAgentRecLoader(t *testing.T) {
	// empty file and empty path are equivalent to nil
	for _, p := range []string{"", agentWrite(t)} {
		ar, err := LoadAgentRecs(p)
		if err != nil || ar != nil {
			t.Fatalf("empty %q: %v %v", p, ar, err)
		}
	}
	var nilRecs *AgentRecs
	if nilRecs.read(linkItem("x", "a", "b")) != nil {
		t.Fatal("nil recs must read nothing")
	}
	bad := map[string][]string{
		"duplicate pair (unordered)": {agentLine("i1", "a", "b", "same"), agentLine("i2", "b", "a", "different")},
		"bad label":                  {agentLine("i1", "a", "b", "maybe")},
		"valid false":                {`{"item":"i1","pair":["a","b"],"label":"same","valid":false}`},
		"invalid json":               {`{"item":`},
		"missing pair":               {`{"item":"i1","pair":["a"],"label":"same","valid":true}`},
	}
	for name, ls := range bad {
		if ar, err := LoadAgentRecs(agentWrite(t, ls...)); err == nil {
			t.Errorf("%s: want error, got %+v", name, ar)
		}
	}
	if _, err := LoadAgentRecs(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file must error")
	}
	ar, err := LoadAgentRecs(agentWrite(t, agentLine("i1", "b", "a", "uncertain"), ""))
	if err != nil || len(ar.recs) != 1 {
		t.Fatalf("blank lines / unordered key: %v %+v", err, ar)
	}
}

func TestAgentRecEligibility(t *testing.T) {
	withDemoGuard(t, true)
	e1, e2 := agentEID("78419801"), agentEID("78419802")
	if !luhnOK(e1) || !luhnOK(e2) {
		t.Fatal("bad fixture eids")
	}
	mk := func(pk, dob, sex, eid string) *Rec {
		return &Rec{Pk: pk, Dob: dob, Sex: sex, Eid: eid, eidOK: eid != "" && luhnOK(eid)}
	}
	recs := map[string]*Rec{
		"a": mk("a", "", "F", ""), "b": mk("b", "1980-02-22", "F", ""),
		"c": mk("c", "1981-02-22", "F", ""), "d": mk("d", "1980-02-22", "M", ""),
		"e": mk("e", "", "F", e1), "f": mk("f", "", "F", e2), "g": mk("g", "1980-02-22", "F", ""),
	}
	lines := []string{}
	for _, p := range [][2]string{{"a", "b"}, {"b", "c"}, {"b", "d"}, {"e", "f"}, {"b", "g"}, {"a", "zz"}} {
		lines = append(lines, agentLine("r"+p[0]+p[1], p[0], p[1], "same"))
	}
	ar := agentRecsWith(t, recs, lines...)

	if r := ar.read(linkItem("i", "b", "a")); r == nil || r.Label != "same" {
		t.Fatalf("missing-DOB pair must be eligible (reversed order too): %+v", r)
	}
	for name, it := range map[string]*ReviewItem{
		"dob conflict":       linkItem("i", "b", "c"),
		"sex conflict":       linkItem("i", "b", "d"),
		"valid eid":          linkItem("i", "e", "f"),
		"both demo known":    linkItem("i", "b", "g"),
		"unknown registrant": linkItem("i", "a", "zz"),
		"no rec":             linkItem("i", "a", "c"),
		"not link":           {ItemID: "i", Kinds: []string{"evidence"}, Pair: [2]string{"a", "b"}},
		"second id":          {ItemID: "i", Kinds: []string{"link"}, Pair: [2]string{"a", "b"}, Reason: "second_identifier"},
	} {
		if r := ar.read(it); r != nil {
			t.Errorf("%s: want fallback, got %+v", name, r)
		}
	}
	withDemoGuard(t, false)
	if ar.read(linkItem("i", "a", "b")) != nil {
		t.Error("guard off must disable agent reads")
	}
	withDemoGuard(t, true)
	if got := ar.Ineligible(); got != 5 {
		t.Errorf("ineligible recs=%d want 5", got)
	}
}

func TestAgentNeedFile(t *testing.T) {
	withDemoGuard(t, true)
	recs := map[string]*Rec{
		"a": {Pk: "a", Sex: "F"}, "b": {Pk: "b", Dob: "1980-02-22", Sex: "F"},
		"c": {Pk: "c", Sex: "F"}, "d": {Pk: "d", Sex: "F"}, "x": {Pk: "x", Dob: "1980-02-22", Sex: "M"}, "y": {Pk: "y", Dob: "1980-02-22", Sex: "F"},
	}
	ar := agentRecsWith(t, recs, agentLine("r1", "c", "d", "uncertain"))
	rp := &replayOut{items: map[string]*ReviewItem{
		"i2": linkItem("i2", "a", "b"), "i1": linkItem("i1", "b", "a"), // same pair twice: one need
		"i3": linkItem("i3", "c", "d"), // terminal uncertain: not requested again
		"i4": linkItem("i4", "x", "y"), // sex conflict: ineligible
		"i5": {ItemID: "i5", Kinds: []string{"evidence"}},
	}}
	p := filepath.Join(t.TempDir(), "need.ndjson")
	if err := WriteAgentNeed(p, rp, ar); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("want one need, got %q", b)
	}
	var got map[string]interface{}
	json.Unmarshal([]byte(lines[0]), &got)
	if got["item"] != "i1" || got["p"].(float64) != 0.6 || !reflect.DeepEqual(got["pair"], []interface{}{"b", "a"}) {
		t.Fatalf("need=%v", got)
	}
	// nothing needed -> empty file still written
	if err := WriteAgentNeed(p, &replayOut{items: map[string]*ReviewItem{}}, ar); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); len(b) != 0 {
		t.Fatalf("want empty file, got %q", b)
	}
}

// findAgentItem searches item ids for one whose human r1/r3 reads match the wanted outcomes.
func findAgentItem(t *testing.T, cfg ReviewerCfg, tr *Truth, a, b, r1, r3 string) *ReviewItem {
	t.Helper()
	for i := 0; i < 5000; i++ {
		it := linkItem("it"+itoa(i), a, b)
		x := cfg.oneRead(it, tr, "r1")
		y := cfg.oneRead(it, tr, "r3")
		if x.link == r1 && (r3 == "" || y.link == r3) {
			return it
		}
	}
	t.Fatalf("no item with r1=%s r3=%s", r1, r3)
	return nil
}

func agentCfg(t *testing.T, label string) (ReviewerCfg, *Truth, *AgentRecs) {
	withDemoGuard(t, true)
	// Per-item draws (so the search can vary item ids); the pair-keyed reuse path is tested separately.
	oldR := ReuseLinkAnswer
	ReuseLinkAnswer = false
	t.Cleanup(func() { ReuseLinkAnswer = oldR })
	recs := map[string]*Rec{"a": {Pk: "a", Sex: "F"}, "b": {Pk: "b", Dob: "1980-02-22", Sex: "F"}}
	ar := agentRecsWith(t, recs, agentLine("r", "a", "b", label))
	cfg := DefaultReviewer
	cfg.Err, cfg.Double, cfg.Seed = 0.5, "merge", 7
	tr := &Truth{PKQid: map[string]string{"a": "Q1", "b": "Q1"}, Reports: map[string]string{}}
	return cfg, tr, ar
}

func TestAgentReadFourCells(t *testing.T) {
	// agent same + human same: agent replaces human r2
	cfg, tr, ar := agentCfg(t, "same")
	ctl := cfg
	it := findAgentItem(t, cfg, tr, "a", "b", "same", "")
	cfg.Agent = ar
	so, base := cfg.Review(it, tr), ctl.Review(it, tr)
	r1 := cfg.oneRead(it, tr, "r1")
	if so.Link != "same" || !reflect.DeepEqual(so.Reads, []string{"r1", "agent"}) || so.AgentRead != "same" || so.AgentS != 1.5 {
		t.Fatalf("same/same: %+v", so)
	}
	if so.DurationS != round1(r1.secs) || so.Role != RoleReviewer {
		t.Fatalf("human seconds must be r1 only: %v vs %v", so.DurationS, r1.secs)
	}
	if so.DurationS >= base.DurationS && len(base.Reads) > 1 {
		t.Errorf("agent should save the r2 read: %v >= %v", so.DurationS, base.DurationS)
	}
	if err := CheckSignoff(so, true); err != nil {
		t.Errorf("dual merge with agent read must pass: %v", err)
	}
	so.Role = ""
	if CheckSignoff(so, true) == nil {
		t.Error("CheckSignoff must still require the reviewer role")
	}

	// agent same + human different: senior human r3 decides, agent does not sign
	it = findAgentItem(t, cfg, tr, "a", "b", "different", "same")
	so = cfg.Review(it, tr)
	r1, r3 := cfg.oneRead(it, tr, "r1"), cfg.oneRead(it, tr, "r3")
	if so.Link != r3.link || !reflect.DeepEqual(so.Reads, []string{"r1", "agent", "r3"}) {
		t.Fatalf("same/different: %+v", so)
	}
	if so.DurationS != round1(r1.secs+r3.secs) {
		t.Fatalf("human seconds r1+r3 only: %v", so.DurationS)
	}
	it = findAgentItem(t, cfg, tr, "a", "b", "different", "different")
	if so = cfg.Review(it, tr); so.Link != "different" {
		t.Fatalf("senior human rejection must stand: %+v", so)
	}

	// agent different/uncertain: existing human path unchanged, both human r1 outcomes
	for _, label := range []string{"different", "uncertain"} {
		cfg, tr, ar := agentCfg(t, label)
		ctl := cfg
		cfg.Agent = ar
		for _, r1 := range []string{"same", "different"} {
			it := findAgentItem(t, cfg, tr, "a", "b", r1, "")
			a, c := cfg.Review(it, tr), ctl.Review(it, tr)
			if a.Link != c.Link || a.DurationS != c.DurationS || !reflect.DeepEqual(a.Reads, c.Reads) || a.Reviewer != c.Reviewer {
				t.Errorf("%s/%s diverged from control: %+v vs %+v", label, r1, a, c)
			}
			for _, rd := range a.Reads {
				if rd == "agent" {
					t.Errorf("%s must not appear as a read", label)
				}
			}
		}
	}
}

func round1(x float64) float64 { return float64(int64(x*10+0.5)) / 10 }

func TestAgentReadControlIdentity(t *testing.T) {
	cfg, tr, ar := agentCfg(t, "same")
	ctl := cfg
	// nil Agent, and an Agent whose recs match nothing, are byte-identical to control
	cfg.Agent = ar
	for i := 0; i < 300; i++ {
		other := linkItem("o"+itoa(i), "p"+itoa(i), "q"+itoa(i))
		tr.PKQid["p"+itoa(i)], tr.PKQid["q"+itoa(i)] = "Z"+itoa(i), "Z"+itoa(i)
		x, _ := json.Marshal(cfg.Review(other, tr))
		y, _ := json.Marshal(ctl.Review(other, tr))
		if string(x) != string(y) {
			t.Fatalf("non-matching pair diverged:\n%s\n%s", x, y)
		}
		ev := &ReviewItem{ItemID: "e" + itoa(i), Kinds: []string{"evidence"}, ReportID: "R"}
		x, _ = json.Marshal(cfg.Review(ev, tr))
		y, _ = json.Marshal(ctl.Review(ev, tr))
		if string(x) != string(y) {
			t.Fatal("evidence item diverged")
		}
	}
	// draws are keyed to the item, not the agent: r1 and r3 reads under an agent equal control's
	it := findAgentItem(t, cfg, tr, "a", "b", "different", "same")
	so := cfg.Review(it, tr)
	if so.Link != ctl.oneRead(it, tr, "r3").link {
		t.Error("r3 draw changed under agent")
	}
	// ReuseLinkAnswer keys draws to the pair: twins stay identical, with and without the agent
	ReuseLinkAnswer = true
	twin := *it
	twin.ItemID = "twin"
	x, _ := json.Marshal(cfg.Review(it, tr))
	var xs Signoff
	json.Unmarshal(x, &xs)
	ys := cfg.Review(&twin, tr)
	if ys.Link != xs.Link || !reflect.DeepEqual(ys.Reads, xs.Reads) || ys.DurationS != xs.DurationS {
		t.Error("link twins diverged under agent")
	}
	cc := ctl
	if a, b := cc.Review(it, tr), cc.Review(&twin, tr); a.Link != b.Link || a.DurationS != b.DurationS {
		t.Error("control twins diverged")
	}
	// DurationS is human-only: AgentS is separate and never summed in
	s2 := *so
	s2.AgentS = 9999
	if s2.DurationS != so.DurationS {
		t.Fatal("DurationS must be independent of AgentS")
	}
}

func TestAgentRecsBindUsesLinkerRegistrations(t *testing.T) {
	withDemoGuard(t, true)
	L := &LinkerCore{byPk: map[string]int{"a": 0, "b": 1}, recs: []*Rec{{Pk: "a", Sex: "F"}, {Pk: "b", Dob: "1980-02-22", Sex: "F"}}}
	ar, err := LoadAgentRecs(agentWrite(t, agentLine("r", "a", "b", "same")))
	if err != nil {
		t.Fatal(err)
	}
	if ar.read(linkItem("i", "a", "b")) != nil {
		t.Fatal("unbound recs must not be eligible")
	}
	ar.bind(L)
	if ar.read(linkItem("i", "a", "b")) == nil {
		t.Fatal("bound recs must be eligible")
	}
	L.recs[0].Dob = "1981-01-02" // a later replay finds a DOB conflict
	if ar.read(linkItem("i", "a", "b")) != nil {
		t.Fatal("conflict must fall back")
	}
	(*AgentRecs)(nil).bind(L) // nil-safe
}
