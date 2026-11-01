package main

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
)

// bruteTransport enumerates every integer allocation of row supplies to columns (plus an "unassigned" option per
// row at cost unassigned) and returns the minimum total cost. Only for tiny instances.
func bruteTransport(supply, capc []int64, cost [][]int64, unassigned int64) int64 {
	best := int64(1 << 62)
	left := append([]int64(nil), capc...)
	var rec func(r int, rem int64, acc int64)
	rec = func(r int, rem int64, acc int64) {
		if acc >= best {
			return
		}
		if r == len(supply) {
			best = acc
			return
		}
		if rem == 0 {
			if r+1 < len(supply) {
				rec(r+1, supply[r+1], acc)
			} else {
				rec(r+1, 0, acc)
			}
			return
		}
		// one unit of row r goes to unassigned or to a column with room
		rec(r, rem-1, acc+unassigned)
		for j := range left {
			if left[j] > 0 && cost[r][j] >= 0 {
				left[j]--
				rec(r, rem-1, acc+cost[r][j])
				left[j]++
			}
		}
	}
	if len(supply) == 0 {
		return 0
	}
	rec(0, supply[0], 0)
	return best
}

func TestMinCostFlowMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const unassigned = 1000
	for trial := 0; trial < 300; trial++ {
		nr, nc := 1+rng.Intn(4), 1+rng.Intn(4)
		supply := make([]int64, nr)
		capc := make([]int64, nc)
		cost := make([][]int64, nr)
		for i := range supply {
			supply[i] = int64(1 + rng.Intn(2))
			cost[i] = make([]int64, nc)
			for j := range cost[i] {
				cost[i][j] = int64(rng.Intn(50))
				if rng.Float64() < 0.25 {
					cost[i][j] = -1 // infeasible pair: no arc
				}
			}
		}
		for j := range capc {
			capc[j] = int64(rng.Intn(3))
		}
		// nodes: s=0, rows 1..nr, cols nr+1..nr+nc, t
		s, tt := 0, nr+nc+1
		var arcs []MCFArc
		var total int64
		for i := 0; i < nr; i++ {
			arcs = append(arcs, MCFArc{s, 1 + i, supply[i], 0})
			arcs = append(arcs, MCFArc{1 + i, tt, supply[i], unassigned})
			total += supply[i]
			for j := 0; j < nc; j++ {
				if cost[i][j] >= 0 {
					arcs = append(arcs, MCFArc{1 + i, 1 + nr + j, supply[i], cost[i][j]})
				}
			}
		}
		for j := 0; j < nc; j++ {
			arcs = append(arcs, MCFArc{1 + nr + j, tt, capc[j], 0})
		}
		flows, flow, c, err := MinCostFlow(nr+nc+2, s, tt, arcs, -1)
		if err != nil {
			t.Fatal(err)
		}
		if flow != total {
			t.Fatalf("trial %d: flow %d, want %d", trial, flow, total)
		}
		want := bruteTransport(supply, capc, cost, unassigned)
		if c != want {
			t.Fatalf("trial %d: cost %d, brute force %d", trial, c, want)
		}
		// flows respect capacities and conserve at every inner node
		bal := make([]int64, nr+nc+2)
		var fc int64
		for i, a := range arcs {
			if flows[i] < 0 || flows[i] > a.Cap {
				t.Fatalf("trial %d: arc %d flow %d outside [0,%d]", trial, i, flows[i], a.Cap)
			}
			bal[a.From] -= flows[i]
			bal[a.To] += flows[i]
			fc += flows[i] * a.Cost
		}
		for v := 1; v < tt; v++ {
			if bal[v] != 0 {
				t.Fatalf("trial %d: node %d imbalance %d", trial, v, bal[v])
			}
		}
		if fc != c {
			t.Fatalf("trial %d: reported cost %d, flows cost %d", trial, c, fc)
		}
	}
}

func TestMinCostFlowConvexCopies(t *testing.T) {
	// two units through parallel arcs with increasing cost must take the cheap ones first
	arcs := []MCFArc{{0, 1, 2, 0}, {1, 2, 1, 3}, {1, 2, 1, 1}, {1, 2, 1, 7}, {2, 3, 5, 0}}
	flows, flow, c, err := MinCostFlow(4, 0, 3, arcs, -1)
	if err != nil || flow != 2 || c != 4 || flows[1] != 1 || flows[2] != 1 || flows[3] != 0 {
		t.Fatalf("got flows %v flow %d cost %d err %v", flows, flow, c, err)
	}
	if _, _, _, err := MinCostFlow(2, 0, 1, []MCFArc{{0, 0, 1, 1}}, -1); err == nil {
		t.Fatal("self loop accepted")
	}
	if _, _, _, err := MinCostFlow(2, 0, 1, []MCFArc{{0, 1, 1, -1}}, -1); err == nil {
		t.Fatal("negative cost accepted")
	}
}

func TestMCFServeLines(t *testing.T) {
	in := strings.NewReader(`{"n":3,"s":0,"t":2,"arcs":[[0,1,2,0],[1,2,1,5],[0,2,1,9]]}` + "\n" + `not json` + "\n")
	var out bytes.Buffer
	runMCF(in, &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 response lines, got %q", out.String())
	}
	var r mcfResponse
	if err := json.Unmarshal([]byte(lines[0]), &r); err != nil || r.Flow != 2 || r.Cost != 14 || r.Error != "" {
		t.Fatalf("bad response %q (%v)", lines[0], err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &r); err != nil || r.Error == "" {
		t.Fatalf("malformed line not reported: %q", lines[1])
	}
}

func TestMCFProtocolValidation(t *testing.T) {
	in := strings.NewReader(
		`{"n":3,"s":0,"t":2,"arcs":[[0,1,2]]}` + "\n" +
			`{"n":3,"s":0,"t":2,"arcs":[[0,1,2,0,99]]}` + "\n" +
			`{"n":100001,"s":0,"t":1,"arcs":[]}` + "\n")
	var out bytes.Buffer
	runMCF(in, &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 error responses, got %q", out.String())
	}
	for i, line := range lines {
		var r mcfResponse
		if err := json.Unmarshal([]byte(line), &r); err != nil || r.Error == "" {
			t.Fatalf("response %d did not report malformed input: %q (%v)", i, line, err)
		}
	}
	if _, _, _, err := MinCostFlow(mcfMaxNodes+1, 0, 1, nil, -1); err == nil {
		t.Fatal("oversized node count accepted")
	}
}

func TestMCFOverlongLineReportsError(t *testing.T) {
	var out bytes.Buffer
	runMCFLimit(strings.NewReader(strings.Repeat("x", 100)+"\n"), &out, 64)
	var r mcfResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &r); err != nil {
		t.Fatalf("scanner error produced no JSON response: %q (%v)", out.String(), err)
	}
	if !strings.Contains(r.Error, "reading input") {
		t.Fatalf("scanner error not reported: %+v", r)
	}
}
