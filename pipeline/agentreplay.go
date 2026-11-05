package main

// agentreplay: offline materializer for the r1agent8 experiment (spec 2026-11-04-crc-r1agent8-evidence-design.md).
// It runs the review loop to its fixed point with the (optional) cached agent recommendations, then writes the final
// lists with the live pipeline's own serializers into OUTPUT_RUN/main so that audit, report7 and the harness read an
// ordinary run directory. It never talks to NATS or Gemini; the agent only supplies cached reads (agentrecs.go).

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// agentReplayMain is the `wsi7 agentreplay` entry point.
func agentReplayMain(data, wasl, run string, P int, cfg ReviewerCfg, recsPath string) {
	ar, err := LoadAgentRecs(recsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentreplay:", err)
		os.Exit(1)
	}
	o := loadOffline(data, wasl)
	if err := stampOfflineMsgIDs(o, data); err != nil {
		fmt.Fprintln(os.Stderr, "agentreplay:", err)
		os.Exit(1)
	}
	if err := materializeAgentRun(o, P, cfg, ar, run); err != nil {
		fmt.Fprintln(os.Stderr, "agentreplay:", err)
		os.Exit(1)
	}
}

// stampOfflineMsgIDs gives each offline FHIR message its generator Nats-Msg-Id (same order as loadOffline), so the
// materialized log passes the audit's ingest-completeness check. The cohort's truth files are not read here.
func stampOfflineMsgIDs(o *offline, data string) error {
	type ge struct {
		id      string
		arrival float64
	}
	all := []ge{}
	for _, pc := range Providers {
		f, err := os.Open(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			all = append(all, ge{g.MsgID, g.Arrival})
		}
		f.Close()
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].arrival < all[j].arrival })
	if len(o.fhir) != len(all)+len(Providers) {
		return fmt.Errorf("offline log has %d messages, cohort %d", len(o.fhir), len(all)+len(Providers))
	}
	for i, a := range all {
		o.fhir[i].Hdr["Nats-Msg-Id"] = a.id
	}
	for i, pc := range Providers {
		o.fhir[len(all)+i].Hdr["Nats-Msg-Id"] = pc.ID + "/eof"
	}
	return nil
}

func writeNDJSON(path string, lines []LogMsg) error {
	var sb strings.Builder
	for _, m := range lines {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// agentTruthWrong: does the sign-off contradict the truth (the live reviewer's "wrong" field)?
func agentTruthWrong(it *ReviewItem, so *Signoff, tr *Truth) bool {
	for _, k := range so.Kinds {
		switch k {
		case "link":
			same := tr.PKQid[so.Pair[0]] != "" && tr.PKQid[so.Pair[0]] == tr.PKQid[so.Pair[1]]
			if (so.Link == "same") != same {
				return true
			}
		case "evidence":
			ev := tr.Reports[so.ReportID]
			if ev == "" {
				ev = "inadequate"
			}
			if so.Evidence != ev {
				return true
			}
		}
	}
	return false
}

var agentAbstainReasons = map[string]bool{"": true, "model_uncertain": true, "controller_abstain": true, "ineligible_item": true}

// materializeAgentRun runs the loop and writes OUTPUT_RUN/main plus agent_need.ndjson and agent_stats.json.
func materializeAgentRun(o *offline, P int, cfg ReviewerCfg, ar *AgentRecs, run string) error {
	main := filepath.Join(run, "main")
	if err := os.RemoveAll(main); err != nil {
		return err
	}
	if err := os.MkdirAll(main, 0o755); err != nil {
		return err
	}
	if ar != nil && !strings.HasSuffix(PolicyName, "+agent") {
		defer func(p string) { PolicyName = p }(PolicyName)
		PolicyName += "+agent"
	}
	cfg.Agent = ar
	_, sos, _, list, source := reviewLoopFull(o, P, cfg)
	if ar != nil { // reviewer identity names the arm; the pointers are shared with the list's sign-off preps
		for _, so := range sos {
			if !strings.HasSuffix(so.Reviewer, "+agent") {
				so.Reviewer += "+agent"
			}
		}
	}

	// One clean final replay, traced, so earlier fixed-point iterations leave no stale funnel records.
	probe := NewLinkerCore(o.gbm, P)
	fcfg := LinkFunnelConfig{CandidateK: probe.K, CandidateCap: probe.Cap, ANNMode: *annMode, MergeThreshold: probe.Thr, ReviewFloor: probe.Lo, ModelSet: ModelSet}
	if *annMode == "hnsw" {
		fcfg.HNSWM, fcfg.HNSWEfConstruct, fcfg.HNSWEfSearch = 16, 100, 64
	}
	funnelPath := filepath.Join(main, "link_funnel.ndjson")
	trace := newLinkFunnelWriter(funnelPath, fcfg)
	ReplayTrace = trace.Append
	rp := replay(nil, list, o.gbm, o.iso, P, false)
	ReplayTrace = nil
	if err := trace.Err(); err != nil {
		return err
	}
	ar.bind(rp.L)

	// fhir_log / prep_log: one contiguous sequence. Copy original messages by source index (including forged sign-offs
	// and EOF controls); only inserted reviewer sign-offs are synthesized on fhir.review.
	fhir := make([]LogMsg, 0, len(list))
	prepLog := make([]LogMsg, 0, len(list))
	soSeq := map[string]uint64{}
	for i, p := range list {
		seq := uint64(i + 1)
		var m LogMsg
		if source[i] < 0 {
			m = LogMsg{Seq: seq, Subject: "fhir.review", Hdr: map[string]string{"Nats-Msg-Id": "signoff/" + p.So.ItemID}, Data: signoffResource(p.So)}
			soSeq[p.So.ItemID] = seq
		} else {
			m = o.fhir[source[i]]
			m.Seq = seq
		}
		fhir = append(fhir, m)
		b, err := json.Marshal(p)
		if err != nil {
			return err
		}
		prepLog = append(prepLog, LogMsg{Seq: seq, Subject: "prep", Hdr: map[string]string{"Nats-Msg-Id": "prep-" + utoa(seq)}, Data: b})
	}
	var outLog []LogMsg
	for i, om := range rp.outs {
		loop := ""
		switch {
		case om.Dec != nil:
			loop = om.Dec.LoopID
		case om.Item != nil:
			loop = om.Item.LoopID
		}
		b, err := json.Marshal(OutEnvelope{Kind: om.Kind, OSeq: om.OSeq, Dec: om.Dec, Item: om.Item, Proc: "agentreplay", Prev: om.Prev, Hash: om.Hash})
		if err != nil {
			return err
		}
		outLog = append(outLog, LogMsg{Seq: uint64(i + 1), Subject: "out.p" + itoa(partOf(loop, P)), Hdr: map[string]string{"Nats-Msg-Id": om.ID}, Data: b})
	}
	for name, l := range map[string][]LogMsg{"fhir_log.ndjson": fhir, "prep_log.ndjson": prepLog, "out_log.ndjson": outLog} {
		if err := writeNDJSON(filepath.Join(main, name), l); err != nil {
			return err
		}
	}

	// reviewer_decisions: ordered by sign-off sequence; t is that sequence, never wall-clock.
	ids := make([]string, 0, len(sos))
	for id := range sos {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return soSeq[ids[i]] < soSeq[ids[j]] })
	var db strings.Builder
	for _, id := range ids {
		so, it := sos[id], rp.items[id]
		verdict := ""
		if it != nil {
			verdict = it.Verdict
		}
		lb, err := json.Marshal(map[string]interface{}{"item": id, "loop": so.LoopID, "kinds": so.Kinds, "verdict": verdict,
			"link": so.Link, "evidence": so.Evidence, "wrong": agentTruthWrong(it, so, o.tr), "role": so.Role, "reads": so.Reads,
			"sim_s": so.DurationS, "agent_read": so.AgentRead, "agent_s": so.AgentS, "reviewer": so.Reviewer, "ms": 0, "t": soSeq[id]})
		if err != nil {
			return err
		}
		db.Write(lb)
		db.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(main, "reviewer_decisions.ndjson"), []byte(db.String()), 0o644); err != nil {
		return err
	}

	// review_queue.json (and the other analysis artifacts) come from the existing analyzer.
	analyze(fhir, list, rp, o.gbm, o.iso, P, o.tr, main)
	writeStatus(main, "linker", linkerStatus(rp.L, uint64(len(list)), 0, int64(rp.facts), 0, -1, 0, 0, 0))
	ob, _ := json.MarshalIndent(map[string]interface{}{"offline_replay": true, "fhir_msgs": len(fhir), "out_msgs": len(outLog), "P": P,
		"wall_s": 0, "throughput_ev_s": 0, "chaos": 0, "linker": readStatus(main, "linker"), "reviewer": cfg}, "", " ")
	if err := os.WriteFile(filepath.Join(main, "orchestrate.json"), ob, 0o644); err != nil {
		return err
	}

	if err := WriteAgentNeed(filepath.Join(run, "agent_need.ndjson"), rp, ar); err != nil {
		return err
	}
	return writeAgentStats(filepath.Join(run, "agent_stats.json"), o, cfg, ar, rp, sos)
}

// writeAgentStats reports the agent's coverage and cost separately from human time. human_seconds_saved compares
// each agent-read sign-off with the identical item answered by the same reviewer without the agent.
func writeAgentStats(path string, o *offline, cfg ReviewerCfg, ar *AgentRecs, rp *replayOut, sos map[string]*Signoff) error {
	st := map[string]interface{}{"eligible": 0, "covered": 0, "abstain": 0, "invalid": 0, "calls": 0, "tokens": 0, "latency_s": 0.0,
		"human_seconds_saved": 0.0, "human_seconds_delta": 0.0, "ineligible_recs": 0, "agent_reads": 0}
	if ar != nil {
		L := ar.L
		ids := make([]string, 0, len(rp.items))
		for id := range rp.items {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		seen := map[[2]string]bool{}
		eligible, covered, abstain, invalid := 0, 0, 0, 0
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
			eligible++
			if r := ar.recs[k]; r != nil {
				covered++
				if r.Label == "uncertain" {
					if agentAbstainReasons[r.Reason] {
						abstain++
					} else {
						invalid++
					}
				}
			}
		}
		calls, tokens, lat := 0, 0, 0.0
		for _, r := range ar.recs {
			calls, tokens, lat = calls+r.Calls, tokens+r.Tokens, lat+r.LatencyS
		}
		ctl := cfg
		ctl.Agent = nil
		delta, reads := 0.0, 0
		for _, so := range sos {
			if so.AgentRead == "" {
				continue
			}
			reads++
			it := rp.items[so.ItemID]
			if it == nil {
				// A sign-off inserted immediately after the event can close the item before the final replay emits it.
				// The sign-off retains everything the deterministic control reviewer needs to re-price the human reads.
				it = &ReviewItem{ItemID: so.ItemID, LoopID: so.LoopID, Kinds: so.Kinds, Pair: so.Pair,
					ReportID: so.ReportID, Verdict: so.Hold, Reason: so.Purpose, Head: so.SignedHead}
			}
			delta += ctl.Review(it, o.tr).DurationS - so.DurationS
		}
		saved := delta
		if saved < 0 {
			saved = 0
		}
		st["eligible"], st["covered"], st["abstain"], st["invalid"] = eligible, covered, abstain, invalid
		st["calls"], st["tokens"], st["latency_s"] = calls, tokens, r3(lat)
		st["human_seconds_saved"], st["human_seconds_delta"] = r3(saved), r3(delta)
		st["ineligible_recs"], st["agent_reads"] = ar.Ineligible(), reads
	}
	b, err := json.MarshalIndent(st, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
