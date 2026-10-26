package main

// Pure-Go inference for (1) the char-level dual-encoder transformer exported from PyTorch and (2) LightGBM dump_model JSON.
import (
	"encoding/json"
	"math"
	"os"
	"strconv"
)

type mat struct {
	r, c int
	w    []float32
}

type layerW struct {
	inW, outW, l1W, l2W        mat
	inB, outB, l1B, l2B        []float32
	n1w, n1b, n2w, n2b         []float32
}
type tower struct {
	emb, pos, proj mat
	projB          []float32
	lnw, lnb       []float32
	layers         []layerW
	vocab          map[rune]int
}
type Encoder struct {
	ar, lat        *tower
	L, DM, NH, OUT int
}

func toMat(v interface{}) mat {
	rows := v.([]interface{})
	r := len(rows)
	c := len(rows[0].([]interface{}))
	m := mat{r, c, make([]float32, r*c)}
	for i, row := range rows {
		for j, x := range row.([]interface{}) {
			m.w[i*c+j] = float32(x.(float64))
		}
	}
	return m
}
func toVec(v interface{}) []float32 {
	a := v.([]interface{})
	o := make([]float32, len(a))
	for i, x := range a {
		o[i] = float32(x.(float64))
	}
	return o
}

func LoadEncoder(path string) *Encoder {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j struct {
		Cfg map[string]interface{} `json:"cfg"`
		W   map[string]interface{} `json:"w"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	e := &Encoder{L: int(j.Cfg["L"].(float64)), DM: int(j.Cfg["DM"].(float64)), NH: int(j.Cfg["NH"].(float64)), OUT: int(j.Cfg["OUT"].(float64))}
	nl := int(j.Cfg["NL"].(float64))
	mk := func(pfx string, chars string) *tower {
		t := &tower{vocab: map[rune]int{}}
		i := 0
		for _, c := range chars {
			t.vocab[c] = i + 2
			i++
		}
		t.emb = toMat(j.W[pfx+".emb.weight"])
		t.pos = toMat(j.W[pfx+".pos.weight"])
		t.proj = toMat(j.W[pfx+".proj.weight"])
		t.projB = toVec(j.W[pfx+".proj.bias"])
		t.lnw = toVec(j.W[pfx+".ln.weight"])
		t.lnb = toVec(j.W[pfx+".ln.bias"])
		for l := 0; l < nl; l++ {
			p := pfx + ".layers." + strconv.Itoa(l) + "."
			t.layers = append(t.layers, layerW{
				inW: toMat(j.W[p+"self_attn.in_proj_weight"]), inB: toVec(j.W[p+"self_attn.in_proj_bias"]),
				outW: toMat(j.W[p+"self_attn.out_proj.weight"]), outB: toVec(j.W[p+"self_attn.out_proj.bias"]),
				l1W: toMat(j.W[p+"linear1.weight"]), l1B: toVec(j.W[p+"linear1.bias"]),
				l2W: toMat(j.W[p+"linear2.weight"]), l2B: toVec(j.W[p+"linear2.bias"]),
				n1w: toVec(j.W[p+"norm1.weight"]), n1b: toVec(j.W[p+"norm1.bias"]),
				n2w: toVec(j.W[p+"norm2.weight"]), n2b: toVec(j.W[p+"norm2.bias"]),
			})
		}
		return t
	}
	e.ar = mk("ar", j.Cfg["AR_CH"].(string))
	e.lat = mk("lat", j.Cfg["LAT_CH"].(string))
	return e
}

func layerNorm(x []float32, w, b []float32, out []float32) {
	n := float32(len(x))
	var mu, v float32
	for _, a := range x {
		mu += a
	}
	mu /= n
	for _, a := range x {
		v += (a - mu) * (a - mu)
	}
	v /= n
	inv := float32(1 / math.Sqrt(float64(v)+1e-5))
	for i, a := range x {
		out[i] = (a-mu)*inv*w[i] + b[i]
	}
}

// y = W x + b  (W: r x c, row-major)
func linear(W mat, b []float32, x []float32, y []float32) {
	for i := 0; i < W.r; i++ {
		row := W.w[i*W.c : (i+1)*W.c]
		s := b[i]
		for k, a := range x {
			s += row[k] * a
		}
		y[i] = s
	}
}

func gelu(x float32) float32 { return float32(0.5 * float64(x) * (1 + math.Erf(float64(x)/math.Sqrt2))) }

// Embed returns L2-normalised OUT-dim vector. Mirrors train_encoder.enc_str + Tower.forward (eval mode).
func (e *Encoder) Embed(name string) []float32 {
	var t *tower
	var s string
	if isAr(name) {
		t, s = e.ar, normAr(name)
	} else {
		t, s = e.lat, normLat(name)
	}
	ids := []int{1}
	for _, c := range s {
		if v, ok := t.vocab[c]; ok && len(ids) < e.L {
			ids = append(ids, v)
		}
	}
	n, D := len(ids), e.DM
	h := make([][]float32, n)
	for i, id := range ids {
		h[i] = make([]float32, D)
		for k := 0; k < D; k++ {
			h[i][k] = t.emb.w[id*D+k] + t.pos.w[i*D+k]
		}
	}
	nh := e.NH
	hd := D / nh
	scale := float32(1 / math.Sqrt(float64(hd)))
	tmp := make([]float32, D)
	qkv := make([][]float32, n)
	for i := range qkv {
		qkv[i] = make([]float32, 3*D)
	}
	att := make([]float32, n)
	ctx := make([]float32, D)
	ao := make([]float32, D)
	for _, L := range t.layers {
		ff := make([]float32, L.l1W.r)
		for i := 0; i < n; i++ {
			layerNorm(h[i], L.n1w, L.n1b, tmp)
			linear(L.inW, L.inB, tmp, qkv[i])
		}
		newh := make([][]float32, n)
		for i := 0; i < n; i++ {
			for hh := 0; hh < nh; hh++ {
				q := qkv[i][hh*hd : (hh+1)*hd]
				mx := float32(-1e30)
				for j := 0; j < n; j++ {
					k := qkv[j][D+hh*hd : D+(hh+1)*hd]
					var s float32
					for d := 0; d < hd; d++ {
						s += q[d] * k[d]
					}
					att[j] = s * scale
					if att[j] > mx {
						mx = att[j]
					}
				}
				var z float32
				for j := 0; j < n; j++ {
					att[j] = float32(math.Exp(float64(att[j] - mx)))
					z += att[j]
				}
				for d := 0; d < hd; d++ {
					ctx[hh*hd+d] = 0
				}
				for j := 0; j < n; j++ {
					v := qkv[j][2*D+hh*hd : 2*D+(hh+1)*hd]
					a := att[j] / z
					for d := 0; d < hd; d++ {
						ctx[hh*hd+d] += a * v[d]
					}
				}
			}
			linear(L.outW, L.outB, ctx, ao)
			nv := make([]float32, D)
			for k := 0; k < D; k++ {
				nv[k] = h[i][k] + ao[k]
			}
			newh[i] = nv
		}
		h = newh
		for i := 0; i < n; i++ {
			layerNorm(h[i], L.n2w, L.n2b, tmp)
			linear(L.l1W, L.l1B, tmp, ff)
			for k := range ff {
				ff[k] = gelu(ff[k])
			}
			linear(L.l2W, L.l2B, ff, ao)
			for k := 0; k < D; k++ {
				h[i][k] += ao[k]
			}
		}
	}
	pool := make([]float32, D)
	for i := 0; i < n; i++ {
		layerNorm(h[i], t.lnw, t.lnb, tmp)
		for k := 0; k < D; k++ {
			pool[k] += tmp[k] / float32(n)
		}
	}
	out := make([]float32, e.OUT)
	linear(t.proj, t.projB, pool, out)
	var nrm float32
	for _, a := range out {
		nrm += a * a
	}
	nrm = float32(math.Sqrt(float64(nrm)))
	for i := range out {
		out[i] /= nrm
	}
	return out
}

// ---------------- LightGBM ----------------
type gbmNode struct {
	leaf        bool
	value       float64
	feat        int
	thr         float64
	defLeft     bool
	missZero    bool
	missNaN     bool
	left, right *gbmNode
}
type GBM struct{ trees []*gbmNode }

func parseNode(m map[string]interface{}) *gbmNode {
	if v, ok := m["leaf_value"]; ok {
		return &gbmNode{leaf: true, value: v.(float64)}
	}
	n := &gbmNode{feat: int(m["split_feature"].(float64)), thr: m["threshold"].(float64), defLeft: m["default_left"].(bool)}
	mt := m["missing_type"].(string)
	n.missZero, n.missNaN = mt == "Zero", mt == "NaN"
	n.left = parseNode(m["left_child"].(map[string]interface{}))
	n.right = parseNode(m["right_child"].(map[string]interface{}))
	return n
}

func LoadGBM(path string) *GBM {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var j struct {
		TreeInfo []struct {
			TreeStructure map[string]interface{} `json:"tree_structure"`
		} `json:"tree_info"`
	}
	if err := json.Unmarshal(b, &j); err != nil {
		panic(err)
	}
	g := &GBM{}
	for _, t := range j.TreeInfo {
		g.trees = append(g.trees, parseNode(t.TreeStructure))
	}
	return g
}

func (g *GBM) Predict(x []float64) float64 {
	s := 0.0
	for _, t := range g.trees {
		n := t
		for !n.leaf {
			v := x[n.feat]
			var left bool
			if (n.missZero && math.Abs(v) <= 1e-35) || (n.missNaN && math.IsNaN(v)) {
				left = n.defLeft
			} else {
				left = v <= n.thr
			}
			if left {
				n = n.left
			} else {
				n = n.right
			}
		}
		s += n.value
	}
	return 1 / (1 + math.Exp(-s))
}
