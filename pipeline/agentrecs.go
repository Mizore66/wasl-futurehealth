package main

// Agent recommendations (spec 2026-11-04-crc-r1agent8-evidence-design.md): a cached evidence-investigator label is
// one extra read for an eligible missing-demographic link question. It never signs: the simulated human still reads
// first and signs every item, and only human reads count towards DurationS.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// AgentRec is one terminal record from agent_recs.ndjson.
type AgentRec struct {
	Item      string          `json:"item"`
	Pair      [2]string       `json:"pair"`
	Label     string          `json:"label"` // same | different | uncertain
	Citations json.RawMessage `json:"citations,omitempty"`
	Valid     bool            `json:"valid"`
	Reason    string          `json:"reason,omitempty"`
	Model     string          `json:"model,omitempty"`
	Calls     int             `json:"calls,omitempty"`
	Tokens    int             `json:"tokens,omitempty"`
	LatencyS  float64         `json:"latency_s,omitempty"`
}

// AgentRecs holds terminal records keyed by unordered registration pair (not by loop-specific item id).
type AgentRecs struct {
	recs map[[2]string]*AgentRec
	L    *LinkerCore // current replay linker, refreshed on each iteration for trace/cannot-link eligibility
}

// LoadAgentRecs reads agent_recs.ndjson. An empty path or an empty file returns nil (identical to no agent).
func LoadAgentRecs(path string) (*AgentRecs, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ar := &AgentRecs{recs: map[[2]string]*AgentRec{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r AgentRec
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, n, err)
		}
		if r.Pair[0] == "" || r.Pair[1] == "" || r.Pair[0] == r.Pair[1] {
			return nil, fmt.Errorf("%s:%d: bad pair %v", path, n, r.Pair)
		}
		if !r.Valid {
			return nil, fmt.Errorf("%s:%d: record not valid", path, n)
		}
		if r.Label != "same" && r.Label != "different" && r.Label != "uncertain" {
			return nil, fmt.Errorf("%s:%d: bad label %q", path, n, r.Label)
		}
		k := pairKey(r.Pair[0], r.Pair[1])
		if _, dup := ar.recs[k]; dup {
			return nil, fmt.Errorf("%s:%d: duplicate pair %v", path, n, k)
		}
		rr := r
		ar.recs[k] = &rr
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(ar.recs) == 0 {
		return nil, nil
	}
	return ar, nil
}

// bind points eligibility at the latest replay's registrations and trace/cannot-link state.
func (a *AgentRecs) bind(L *LinkerCore) {
	if a == nil || L == nil {
		return
	}
	a.L = L
}

// agentEligiblePair requires the exact demographic_missing trace and no cannot-link, in addition to demographic/EID checks.
// This deliberately excludes xscript_cap and ordinary near pairs, and pairs later unlinked by a reviewer.
func agentEligiblePair(L *LinkerCore, a, b string) bool {
	if !DemoGuard || L == nil || !L.demoMissing[pairKey(a, b)] {
		return false
	}
	ia, oka := L.byPk[a]
	ib, okb := L.byPk[b]
	if !oka || !okb {
		return false
	}
	x, y := L.recs[ia], L.recs[ib]
	if demoClassify(x, y) != demoNeedsReview {
		return false
	}
	if x.Eid != "" && y.Eid != "" && ((x.eidOK && y.eidOK && x.Eid != y.Eid) || eidContradict(x.Eid, y.Eid)) {
		return false
	}
	if L.cannot[ia] != nil && L.cannot[ia][ib] {
		return false
	}
	return true
}

// read returns the terminal record for an eligible link question, else nil (ordinary human double-read).
func (a *AgentRecs) read(it *ReviewItem) *AgentRec {
	if a == nil || !isLinkQuestion(it) || !agentEligiblePair(a.L, it.Pair[0], it.Pair[1]) {
		return nil
	}
	return a.recs[pairKey(it.Pair[0], it.Pair[1])]
}

// Ineligible counts recommendations whose pair is not eligible under the current registrations (ignored by read).
func (a *AgentRecs) Ineligible() int {
	if a == nil {
		return 0
	}
	n := 0
	for k := range a.recs {
		if !agentEligiblePair(a.L, k[0], k[1]) {
			n++
		}
	}
	return n
}

// WriteAgentNeed writes agent_need.ndjson: one {item,pair,p} per eligible pair of the replay's link questions that
// has no terminal record yet (the smallest item id represents a pair). A terminal uncertain record is a record and
// is never requested again. An optional recs argument supplies the terminal records; without it every eligible pair
// is needed. The file is always written, empty when nothing is needed.
func WriteAgentNeed(path string, rp *replayOut, recs ...*AgentRecs) error {
	var ar *AgentRecs
	if len(recs) > 0 {
		ar = recs[0]
	}
	var L *LinkerCore
	switch {
	case ar != nil && ar.L != nil:
		L = ar.L
	case rp.L != nil:
		L = rp.L
	}
	ids := make([]string, 0, len(rp.items))
	for id := range rp.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	seen := map[[2]string]bool{}
	var sb strings.Builder
	for _, id := range ids {
		it := rp.items[id]
		if !isLinkQuestion(it) {
			continue
		}
		k := pairKey(it.Pair[0], it.Pair[1])
		if seen[k] || !agentEligiblePair(L, k[0], k[1]) {
			continue
		}
		seen[k] = true
		if ar != nil && ar.recs[k] != nil {
			continue
		}
		b, err := json.Marshal(struct {
			Item string    `json:"item"`
			Pair [2]string `json:"pair"`
			P    float64   `json:"p"`
		}{it.ItemID, it.Pair, it.P})
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
