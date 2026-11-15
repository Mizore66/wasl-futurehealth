package main

import (
	"encoding/json"
	"strings"
	"testing"
)

type identityScenario struct {
	chains    [][]Block
	outreach  [][][2]string
	different [][2]string
}

func linkBlocks(pairs ...[2]string) []Block {
	var ch []Block
	for _, p := range pairs {
		ch = appendBlock(ch, "link_decision", map[string]interface{}{"weakest_edge": map[string]interface{}{"a": p[0], "b": p[1]}, "loop": exportLoop})
	}
	return ch
}

func identityServer(t *testing.T, sc identityScenario) (*apiServer, map[string]interface{}) {
	t.Helper()
	s, _ := exportRun(t)
	c := s.cases[exportLoop]
	c.decisions = nil
	for i, ch := range sc.chains {
		d := &Decision{ID: "dec-" + string(rune('a'+i)), LoopID: exportLoop, Prog: "crc", Provider: "exp-prov", State: "open", Policy: "v7strict", Contact: "granted", Chain: ch}
		if i < len(sc.outreach) {
			for _, p := range sc.outreach[i] {
				d.Outreach = append(d.Outreach, Outreach{Pair: p})
			}
		}
		d.Sig = decSig(d)
		c.decisions = append(c.decisions, d)
		c.last = d
	}
	regs := []string{"provA|1", "provB|2", "provC|3"}
	s.ids[exportLoop], s.facts[exportLoop] = nil, nil
	for i, r := range regs {
		role := "downstream"
		if i == 0 {
			role = "referral"
		}
		prov := strings.Split(r, "|")[0]
		s.ids[exportLoop] = append(s.ids[exportLoop], map[string]string{"registration": r, "provider": prov, "role": role})
		s.facts[exportLoop] = append(s.facts[exportLoop], map[string]interface{}{"seq": uint64(i + 1), "type": "Observation", "id": "ev-" + prov, "provider": prov, "patient_key": r})
	}
	for _, p := range sc.different {
		s.signoffs[exportLoop] = append(s.signoffs[exportLoop], map[string]interface{}{"loop": exportLoop, "link": "different", "pair": []interface{}{p[0], p[1]}})
	}
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop)
	return s, decodeMap(t, raw)
}

func jsonKey(detail map[string]interface{}, key string) string {
	b, _ := json.Marshal(detail[key])
	return string(b)
}

func TestRejectedLinkDroppedFromCurrentIdentity(t *testing.T) {
	_, detail := identityServer(t, identityScenario{
		chains:    [][]Block{linkBlocks([2]string{"provA|1", "provB|2"}), nil},
		different: [][2]string{{"provA|1", "provB|2"}},
	})
	if ids := jsonKey(detail, "identities"); strings.Contains(ids, "provB") || !strings.Contains(ids, "provA") {
		t.Fatalf("identities = %s", ids)
	}
	if tl := jsonKey(detail, "timeline"); strings.Contains(tl, "ev-provB") || !strings.Contains(tl, "ev-provA") {
		t.Fatalf("timeline = %s", tl)
	}
	if links := detail["links"].([]interface{}); len(links) != 0 {
		t.Fatalf("current links = %v", links)
	}
	hist, _ := detail["link_history"].([]interface{})
	if len(hist) != 1 || hist[0].(map[string]interface{})["history"] == nil {
		t.Fatalf("link_history = %v", detail["link_history"])
	}
	if h := jsonKey(detail, "identity_history"); !strings.Contains(h, "provB") {
		t.Fatalf("identity_history = %s", h)
	}
}

func TestTrustedLinkKeepsBothRegistrations(t *testing.T) {
	_, detail := identityServer(t, identityScenario{chains: [][]Block{linkBlocks([2]string{"provA|1", "provB|2"})}})
	if ids := jsonKey(detail, "identities"); !strings.Contains(ids, "provB") || !strings.Contains(ids, "provA") {
		t.Fatalf("identities = %s", ids)
	}
	if tl := jsonKey(detail, "timeline"); !strings.Contains(tl, "ev-provB") {
		t.Fatalf("timeline = %s", tl)
	}
	if links := detail["links"].([]interface{}); len(links) != 1 {
		t.Fatalf("links = %v", links)
	}
}

func TestRejectedComponentSplitDropsAllMembers(t *testing.T) {
	_, detail := identityServer(t, identityScenario{
		chains:    [][]Block{linkBlocks([2]string{"provA|1", "provB|2"}, [2]string{"provB|2", "provC|3"})},
		different: [][2]string{{"provB|2", "provA|1"}},
	})
	ids := jsonKey(detail, "identities")
	if strings.Contains(ids, "provB") || strings.Contains(ids, "provC") || !strings.Contains(ids, "provA") {
		t.Fatalf("identities = %s", ids)
	}
	if tl := jsonKey(detail, "timeline"); strings.Contains(tl, "provB") || strings.Contains(tl, "provC") {
		t.Fatalf("timeline = %s", tl)
	}
}

func TestHistoricalOutreachCannotReintroduceRegistration(t *testing.T) {
	_, detail := identityServer(t, identityScenario{
		chains:   [][]Block{nil, nil},
		outreach: [][][2]string{{{"provA|1", "provB|2"}}, nil},
	})
	if ids := jsonKey(detail, "identities"); strings.Contains(ids, "provB") {
		t.Fatalf("identities = %s", ids)
	}
	_, detail = identityServer(t, identityScenario{
		chains:   [][]Block{nil},
		outreach: [][][2]string{{{"provA|1", "provB|2"}}},
	})
	if ids := jsonKey(detail, "identities"); !strings.Contains(ids, "provB") {
		t.Fatalf("current outreach pair lost: %s", ids)
	}
}

func TestExportAgreesOnCurrentVersusHistoricalIdentity(t *testing.T) {
	s, detail := identityServer(t, identityScenario{
		chains:    [][]Block{linkBlocks([2]string{"provA|1", "provB|2"}), nil},
		different: [][2]string{{"provA|1", "provB|2"}},
	})
	_, raw := fetchBody(t, exportRoutes(s), "GET", "/api/cases/"+exportLoop+"/export")
	exp := decodeMap(t, raw)["detail"].(map[string]interface{})
	rd := redactExportDetail(detail)
	for _, key := range []string{"identities", "timeline", "links", "link_history", "identity_history"} {
		if got, want := string(canon(exp[key])), string(canon(rd[key])); got != want {
			t.Fatalf("export %s = %s, want %s", key, got, want)
		}
	}
	if h, _ := exp["link_history"].([]interface{}); len(h) != 1 {
		t.Fatalf("export link_history = %v", exp["link_history"])
	}
}
