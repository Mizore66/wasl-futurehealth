package main

import (
	"encoding/json"
	"os"
	"sort"
)

// Diagnostics-only candidate tracing for the linker. Callbacks must never write linker state.

var LinkFunnelPath string
var LinkFunnelArgs []string

type LinkFunnelConfig struct {
	CandidateK      int     `json:"candidate_k"`
	CandidateCap    int     `json:"candidate_cap"`
	ANNMode         string  `json:"ann_mode"`
	HNSWM           int     `json:"hnsw_m,omitempty"`
	HNSWEfConstruct int     `json:"hnsw_ef_construction,omitempty"`
	HNSWEfSearch    int     `json:"hnsw_ef_search,omitempty"`
	MergeThreshold  float64 `json:"merge_threshold"`
	ReviewFloor     float64 `json:"review_floor"`
	ModelSet        string  `json:"modelset"`
}

type LinkTrace struct {
	Kind    string            `json:"kind"`
	Seq     uint64            `json:"seq,omitempty"`
	A       string            `json:"a,omitempty"`
	B       string            `json:"b,omitempty"`
	P       float64           `json:"p,omitempty"`
	Stage   string            `json:"stage,omitempty"`
	Reason  string            `json:"reason,omitempty"`
	Root    string            `json:"root,omitempty"`
	Members []string          `json:"members,omitempty"`
	Config  *LinkFunnelConfig `json:"config,omitempty"`
}

func (L *LinkerCore) emitLink(e LinkTrace) {
	if L.trace == nil {
		return
	}
	e.Kind = "pair"
	e.Seq = L.traceSeq
	L.trace(e)
}

type linkFunnelWriter struct {
	enc *json.Encoder
	err error
}

func newLinkFunnelWriter(path string, cfg LinkFunnelConfig) *linkFunnelWriter {
	if path == "" {
		return nil
	}
	w := &linkFunnelWriter{}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		w.err = err
		return w
	}
	w.enc = json.NewEncoder(f)
	w.err = w.enc.Encode(LinkTrace{Kind: "config", Config: &cfg})
	return w
}

func (w *linkFunnelWriter) Append(e LinkTrace) {
	if w == nil || w.err != nil {
		return
	}
	w.err = w.enc.Encode(e)
}

func (w *linkFunnelWriter) Err() error {
	if w == nil {
		return nil
	}
	return w.err
}

// emitClusters snapshots every affected cluster at the end of Apply (root PK and sorted member PKs), so an offline
// reader can tell which expected pairs were joined transitively. Diagnostics only.
func (L *LinkerCore) emitClusters(roots map[int]bool) {
	if L.trace == nil {
		return
	}
	seen := map[int]bool{}
	for _, r := range keysOf(roots) {
		if r < 0 || r >= len(L.recs) {
			continue
		}
		r = L.Find(r)
		if seen[r] {
			continue
		}
		seen[r] = true
		pks := []string{}
		for _, m := range L.members[r] {
			pks = append(pks, L.recs[m].Pk)
		}
		sort.Strings(pks)
		L.trace(LinkTrace{Kind: "cluster", Seq: L.traceSeq, Root: L.recs[r].Pk, Members: pks})
	}
}
