package main

// Shadow-private lane staging for the evidence investigator (docs/AGENT-SHADOW.md).
//
// A lane is the directory layout scripts/agent_evidence.py EvidenceStore.from_lane reads: run/main/{review_queue.json,
// link_funnel.ndjson,prep_log.ndjson}, an empty cohort/, and agent_need.ndjson. It is built only from the two
// registrations of one eligible link question, captured at the moment the item was raised (point in time):
// demographics already in the linker, nothing else. It never contains live review quotes, report text, chain blocks,
// truth, reviewer answers, donor maps, or anything from the live run directory. The lane dir is shadow-private and is
// removed as soon as the investigation ends.

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type shadowReg struct {
	Pk, Prov, Name, Dob, Sex, Eid string
}

type shadowLaneData struct {
	Item string
	Pair [2]string
	P    float64
	Regs [2]shadowReg
}

// snapshotLane captures the two registrations of c at production time; nil when either is unknown to the linker.
func snapshotLane(L *LinkerCore, c *shadowCand) *shadowLaneData {
	d := &shadowLaneData{Item: c.ID, Pair: c.Pair, P: c.P}
	for i, pk := range c.Pair {
		idx, ok := L.byPk[pk]
		if !ok {
			return nil
		}
		r := L.recs[idx]
		d.Regs[i] = shadowReg{Pk: r.Pk, Prov: r.Prov, Name: r.Name, Dob: r.Dob, Sex: r.Sex, Eid: r.Eid}
	}
	return d
}

func writeJSONFile(path string, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func writeNDJSONRows(path string, rows []interface{}) error {
	var out []byte
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		out = append(append(out, b...), '\n')
	}
	return os.WriteFile(path, out, 0o644)
}

// stageLane writes the lane for one candidate into dir (created fresh).
func stageLane(dir string, c *shadowCand) error {
	d := c.lane
	if d == nil {
		return os.ErrInvalid
	}
	os.RemoveAll(dir)
	main := filepath.Join(dir, "run", "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "cohort"), 0o755); err != nil {
		return err
	}
	// kinds/pair/p only: no quote, no chain, no head, no verdict, no report id
	if err := writeJSONFile(filepath.Join(main, "review_queue.json"), []map[string]interface{}{
		{"item": d.Item, "kinds": []string{"link"}, "pair": []string{d.Pair[0], d.Pair[1]}, "p": d.P, "report": "", "reason": ""}}); err != nil {
		return err
	}
	if err := writeNDJSONRows(filepath.Join(main, "link_funnel.ndjson"), []interface{}{
		map[string]interface{}{"kind": "config", "config": map[string]interface{}{}},
		map[string]interface{}{"kind": "pair", "a": d.Pair[0], "b": d.Pair[1], "p": d.P, "stage": "near", "reason": "demographic_missing"}}); err != nil {
		return err
	}
	rows := []interface{}{}
	for _, r := range d.Regs {
		rows = append(rows, map[string]interface{}{"data": map[string]interface{}{"kind": "event", "ev": map[string]interface{}{
			"pk": r.Pk, "provider": r.Prov, "name": r.Name, "dob": r.Dob, "sex": r.Sex, "eid": r.Eid}}})
	}
	if err := writeNDJSONRows(filepath.Join(main, "prep_log.ndjson"), rows); err != nil {
		return err
	}
	return writeNDJSONRows(filepath.Join(dir, "agent_need.ndjson"), []interface{}{
		map[string]interface{}{"item": d.Item, "pair": []string{d.Pair[0], d.Pair[1]}, "p": d.P}})
}
