package main

// Min-cost flow for the booking lookahead (booking/booking/lookahead.py).
//
// The booking stage is Python; its optional multi-period lookahead optimiser builds a flow network per day
// (pending patients and forecast future arrivals -> endoscopy/colposcopy sessions -> sink) and asks this solver
// for a min-cost maximum flow. `wsi7 mcf` reads one JSON problem per line on stdin and writes one JSON answer
// per line on stdout, so one process serves a whole booking run.
//
// Method: successive shortest paths with Dijkstra on reduced costs (Johnson potentials), stopped once the sink is
// settled. Costs are non-negative int64, so the initial potentials are 0 and every reduced cost stays >= 0.
// The result is an exact minimum-cost flow among maximum flows. Ties are broken deterministically by arc order.
// Pure Go, no external solver.

import (
	"bufio"
	"container/heap"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const (
	mcfMaxNodes = 100000
	mcfMaxLine  = 64 << 20 // bound request memory; normal booking networks are far below this
)

// MCFArc is one directed arc with integer capacity and non-negative integer cost.
type MCFArc struct {
	From, To  int
	Cap, Cost int64
}

type mcfEdge struct {
	to, rev   int
	cap, cost int64
}

type mcfItem struct {
	d    int64
	node int
}

type mcfHeap []mcfItem

func (h mcfHeap) Len() int { return len(h) }
func (h mcfHeap) Less(i, j int) bool {
	if h[i].d != h[j].d {
		return h[i].d < h[j].d
	}
	return h[i].node < h[j].node
}
func (h mcfHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *mcfHeap) Push(x interface{}) { *h = append(*h, x.(mcfItem)) }
func (h *mcfHeap) Pop() interface{} {
	o := *h
	x := o[len(o)-1]
	*h = o[:len(o)-1]
	return x
}

// MinCostFlow returns the flow on each input arc, the total flow from s to t and its total cost.
// It sends as much flow as possible (up to maxFlow; maxFlow < 0 means unlimited) at minimum cost.
func MinCostFlow(n, s, t int, arcs []MCFArc, maxFlow int64) (flows []int64, flow, cost int64, err error) {
	if n < 0 || n > mcfMaxNodes {
		return nil, 0, 0, fmt.Errorf("mcf: node count %d outside [0,%d]", n, mcfMaxNodes)
	}
	if s < 0 || s >= n || t < 0 || t >= n || s == t {
		return nil, 0, 0, fmt.Errorf("mcf: bad source/sink %d/%d for %d nodes", s, t, n)
	}
	g := make([][]mcfEdge, n)
	pos := make([][2]int, len(arcs)) // arc -> (node, edge index) of its forward edge
	for i, a := range arcs {
		if a.From < 0 || a.From >= n || a.To < 0 || a.To >= n || a.From == a.To || a.Cap < 0 || a.Cost < 0 {
			return nil, 0, 0, fmt.Errorf("mcf: bad arc %d: %+v", i, a)
		}
		pos[i] = [2]int{a.From, len(g[a.From])}
		g[a.From] = append(g[a.From], mcfEdge{a.To, len(g[a.To]), a.Cap, a.Cost})
		g[a.To] = append(g[a.To], mcfEdge{a.From, len(g[a.From]) - 1, 0, -a.Cost})
	}
	const inf = int64(math.MaxInt64 / 4)
	pot := make([]int64, n)
	dist := make([]int64, n)
	prevN := make([]int, n)
	prevE := make([]int, n)
	for maxFlow < 0 || flow < maxFlow {
		for i := range dist {
			dist[i], prevN[i], prevE[i] = inf, -1, -1
		}
		dist[s] = 0
		h := &mcfHeap{{0, s}}
		for h.Len() > 0 {
			it := heap.Pop(h).(mcfItem)
			u := it.node
			if it.d > dist[u] {
				continue
			}
			if u == t { // nodes not yet settled keep distance >= dist[t]; the potential update below caps at dist[t]
				break
			}
			for ei, e := range g[u] {
				if e.cap <= 0 {
					continue
				}
				nd := dist[u] + e.cost + pot[u] - pot[e.to]
				if nd < dist[e.to] {
					dist[e.to], prevN[e.to], prevE[e.to] = nd, u, ei
					heap.Push(h, mcfItem{nd, e.to})
				}
			}
		}
		if dist[t] >= inf {
			break
		}
		for i := 0; i < n; i++ { // pot += min(dist, dist[t]) keeps every residual reduced cost >= 0
			if dist[i] < dist[t] {
				pot[i] += dist[i]
			} else {
				pot[i] += dist[t]
			}
		}
		push := inf
		if maxFlow >= 0 {
			push = maxFlow - flow
		}
		for v := t; v != s; v = prevN[v] {
			if c := g[prevN[v]][prevE[v]].cap; c < push {
				push = c
			}
		}
		for v := t; v != s; v = prevN[v] {
			e := &g[prevN[v]][prevE[v]]
			e.cap -= push
			g[v][e.rev].cap += push
			cost += push * e.cost
		}
		flow += push
	}
	flows = make([]int64, len(arcs))
	for i, a := range arcs {
		flows[i] = a.Cap - g[pos[i][0]][pos[i][1]].cap
	}
	return flows, flow, cost, nil
}

type mcfRequest struct {
	N       int       `json:"n"`
	S       int       `json:"s"`
	T       int       `json:"t"`
	Arcs    [][]int64 `json:"arcs"` // each must be exactly [from, to, cap, cost]
	MaxFlow *int64    `json:"max_flow,omitempty"`
}

type mcfResponse struct {
	Flows []int64 `json:"flows"`
	Flow  int64   `json:"flow"`
	Cost  int64   `json:"cost"`
	Error string  `json:"error,omitempty"`
}

// runMCF serves `wsi7 mcf`: one JSON request per input line, one JSON response per output line.
func runMCF(in io.Reader, out io.Writer) {
	runMCFLimit(in, out, mcfMaxLine)
}

func runMCFLimit(in io.Reader, out io.Writer, maxLine int) {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, min(64*1024, maxLine)), maxLine)
	w := bufio.NewWriter(out)
	enc := json.NewEncoder(w)
	for sc.Scan() {
		var req mcfRequest
		var resp mcfResponse
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			resp.Error = err.Error()
		} else {
			for i, a := range req.Arcs {
				if len(a) != 4 {
					resp.Error = fmt.Sprintf("mcf: arc %d has %d fields, want 4 [from,to,cap,cost]", i, len(a))
					break
				}
			}
			if resp.Error == "" {
				arcs := make([]MCFArc, len(req.Arcs))
				for i, a := range req.Arcs {
					arcs[i] = MCFArc{int(a[0]), int(a[1]), a[2], a[3]}
				}
				mf := int64(-1)
				if req.MaxFlow != nil {
					mf = *req.MaxFlow
				}
				fl, f, c, err := MinCostFlow(req.N, req.S, req.T, arcs, mf)
				if err != nil {
					resp.Error = err.Error()
				} else {
					resp.Flows, resp.Flow, resp.Cost = fl, f, c
				}
			}
		}
		if err := enc.Encode(resp); err != nil {
			return
		}
		if err := w.Flush(); err != nil {
			return
		}
	}
	if err := sc.Err(); err != nil {
		// Scanner errors (including an over-long line) are protocol errors, not clean EOF.
		_ = enc.Encode(mcfResponse{Error: "mcf: reading input: " + err.Error()})
		_ = w.Flush()
	}
}
