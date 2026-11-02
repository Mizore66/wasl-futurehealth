package main

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
}

type LinkTrace struct {
	Kind   string            `json:"kind"`
	Seq    uint64            `json:"seq,omitempty"`
	A      string            `json:"a,omitempty"`
	B      string            `json:"b,omitempty"`
	P      float64           `json:"p,omitempty"`
	Stage  string            `json:"stage,omitempty"`
	Reason string            `json:"reason,omitempty"`
	Config *LinkFunnelConfig `json:"config,omitempty"`
}

func (L *LinkerCore) emitLink(e LinkTrace) {
	if L.trace == nil {
		return
	}
	e.Kind = "pair"
	e.Seq = L.traceSeq
	L.trace(e)
}
