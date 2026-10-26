package main

// v6 live-chain tamper tests. Loads the REAL artefacts of a finished run (fhir_log, out_log as dumped from JetStream),
// applies deletion / reorder / truncation attacks to in-memory copies, and runs every detector on each copy.
// A "naive" attacker edits one thing; a "strong" attacker also renumbers sequence numbers and recomputes every hash it
// can (provenance chains, decision signatures, the v6 output ledger) - only the externally held anchors and the
// immutable FHIR input log (replay) are beyond its reach.
//
// Detectors
//   D1 stream sequence continuity (JetStream seq 1..N, strictly increasing)
//   D2 per-partition output sequence (OSeq) is exactly {1..N}, no gaps or duplicates
//   D3 provenance chain verifies + closed decisions end in closure_decision + decision Sig recomputes
//   D4 deterministic replay from the FHIR log: every expected output present, same OSeq and content, nothing extra
//   D5 v6 ledger: per-partition prev/hash links verify and the head equals the anchor held by the auditor (v6 runs)
//   D6 ingest completeness: every provider-manifest message id is in the FHIR log exactly once
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type tamperCtx struct {
	data, wasl, run string
	P               int
	fhir, out       []LogMsg
	preps           map[uint64]*Prep // by original FHIR seq
	enc             *Encoder
	gbm             *GBM
	iso             *Iso
	manifest        map[string]bool
	anchors         map[string]string // partition subject -> head hash (v6)
	v6              bool
}

func decodeEnv(m LogMsg) OutEnvelope {
	var oe OutEnvelope
	json.Unmarshal(m.Data, &oe)
	return oe
}

func encodeEnv(m *LogMsg, oe OutEnvelope) {
	b, _ := json.Marshal(oe)
	m.Data = b
}

func cloneLog(l []LogMsg) []LogMsg {
	c := make([]LogMsg, len(l))
	for i, m := range l {
		h := map[string]string{}
		for k, v := range m.Hdr {
			h[k] = v
		}
		c[i] = LogMsg{Seq: m.Seq, Subject: m.Subject, Hdr: h, Data: append([]byte{}, m.Data...), Ts: m.Ts}
	}
	return c
}

func renumber(l []LogMsg) {
	for i := range l {
		l[i].Seq = uint64(i + 1)
	}
}

// ---------------- detectors ----------------

func d1Seq(l []LogMsg) string {
	for i, m := range l {
		if m.Seq != uint64(i+1) {
			return fmt.Sprintf("seq %d at position %d", m.Seq, i+1)
		}
	}
	return ""
}

func d2OSeq(out []LogMsg) string {
	by := map[string]map[uint64]int{}
	for _, m := range out {
		oe := decodeEnv(m)
		if by[m.Subject] == nil {
			by[m.Subject] = map[uint64]int{}
		}
		by[m.Subject][oe.OSeq]++
	}
	subs := []string{}
	for s := range by {
		subs = append(subs, s)
	}
	sort.Strings(subs)
	for _, s := range subs {
		set := by[s]
		for k := uint64(1); k <= uint64(len(set)); k++ {
			if set[k] != 1 {
				return fmt.Sprintf("%s oseq %d count %d", s, k, set[k])
			}
		}
	}
	return ""
}

func decSig(d *Decision) string {
	c := *d
	c.Sig, c.Now, c.InSeq, c.ID = "", 0, 0, ""
	return sha(canon(&c))
}

func d3Prov(out []LogMsg) string {
	for _, m := range out {
		oe := decodeEnv(m)
		if oe.Dec == nil {
			if oe.Item != nil && (len(oe.Item.Chain) == 0 || !verifyChain(oe.Item.Chain) || oe.Item.Chain[len(oe.Item.Chain)-1].Hash != oe.Item.Head) {
				return "review item chain " + oe.Item.ItemID
			}
			continue
		}
		d := oe.Dec
		if len(d.Chain) > 0 && !verifyChain(d.Chain) {
			return "chain " + d.ID
		}
		if d.State == "closed" && (len(d.Chain) == 0 || d.Chain[len(d.Chain)-1].Type != "closure_decision") {
			return "closed without closure_decision " + d.ID
		}
		if decSig(d) != d.Sig {
			return "sig " + d.ID
		}
	}
	return ""
}

type expOut struct {
	oseq uint64
	sum  string
}

func outSum(o OutMsg) string {
	if o.Dec != nil {
		return o.Dec.Sig
	}
	return o.Item.Head
}

func (t *tamperCtx) expected(fhir []LogMsg) map[string]expOut {
	preps := make([]*Prep, 0, len(fhir))
	for i, m := range fhir {
		p := *t.preps[origSeq(m)]
		p.Seq = uint64(i + 1)
		preps = append(preps, &p)
	}
	rp := replay(nil, preps, t.gbm, t.iso, t.P, false)
	exp := map[string]expOut{}
	for _, o := range rp.outs {
		exp[o.ID] = expOut{o.OSeq, outSum(o)}
	}
	return exp
}

// origSeq: tampered FHIR copies keep the original seq in a private header so the (expensive, stateless) prep is reused.
func origSeq(m LogMsg) uint64 {
	var s uint64
	fmt.Sscan(m.Hdr["X-Orig-Seq"], &s)
	return s
}

func d4Replay(exp map[string]expOut, out []LogMsg) string {
	seen := map[string]int{}
	for _, m := range out {
		id := m.Hdr["Nats-Msg-Id"]
		seen[id]++
		oe := decodeEnv(m)
		e, ok := exp[id]
		if !ok {
			return "unexpected output " + id
		}
		sum := ""
		if oe.Dec != nil {
			sum = oe.Dec.Sig
		} else if oe.Item != nil {
			sum = oe.Item.Head
		}
		if e.oseq != oe.OSeq || e.sum != sum {
			return "content/order mismatch " + id
		}
	}
	ids := make([]string, 0, len(exp))
	for id := range exp {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lost := 0
	first := ""
	for _, id := range ids {
		if seen[id] == 0 {
			if lost == 0 {
				first = id
			}
			lost++
		}
	}
	if lost > 0 {
		return fmt.Sprintf("%d expected outputs missing (first %s)", lost, first)
	}
	for id, c := range seen {
		if c > 1 {
			return "duplicate " + id
		}
	}
	return ""
}

func d5Ledger(out []LogMsg, anchors map[string]string) string {
	if anchors == nil {
		return "n/a"
	}
	by := map[string][]OutEnvelope{}
	for _, m := range out {
		by[m.Subject] = append(by[m.Subject], decodeEnv(m))
	}
	subs := []string{}
	for s := range anchors {
		if s != "fhir" {
			subs = append(subs, s)
		}
	}
	sort.Strings(subs)
	for _, s := range subs {
		es := by[s]
		sort.SliceStable(es, func(i, j int) bool { return es[i].OSeq < es[j].OSeq })
		prev := ""
		for _, e := range es {
			id, kind, body := "", e.Kind, []byte(nil)
			if e.Dec != nil {
				id, body = e.Dec.ID, canon(e.Dec)
			} else {
				id, body = e.Item.ItemID, canon(e.Item)
			}
			if e.Prev != prev || e.Hash != ledgerHash(prev, e.OSeq, id, kind, body) {
				return fmt.Sprintf("%s ledger break at oseq %d", s, e.OSeq)
			}
			prev = e.Hash
		}
		if prev != anchors[s] {
			return fmt.Sprintf("%s head != anchor", s)
		}
	}
	return ""
}

// fhirAnchor: hash chain over the FHIR input log (seq, subject, message id, body).
func fhirAnchor(l []LogMsg) string {
	h := ""
	for _, m := range l {
		h = sha(append([]byte(h+"|"+utoa(m.Seq)+"|"+m.Subject+"|"+m.Hdr["Nats-Msg-Id"]+"|"), m.Data...))
	}
	return h
}

func (t *tamperCtx) d6Ingest(fhir []LogMsg) string {
	got := map[string]int{}
	for _, m := range fhir {
		if id := m.Hdr["Nats-Msg-Id"]; id != "" {
			got[id]++
		}
	}
	missing := 0
	for id := range t.manifest {
		if got[id] != 1 {
			missing++
		}
	}
	if missing > 0 {
		return fmt.Sprintf("%d manifest ids missing/duplicated", missing)
	}
	return ""
}

func stripOrig(l []LogMsg) []LogMsg {
	c := cloneLog(l)
	for i := range c {
		delete(c[i].Hdr, "X-Orig-Seq")
	}
	return c
}

// synthOut regenerates a self-consistent OUT log (with a recomputed ledger) from a FHIR log via deterministic replay.
func (t *tamperCtx) synthOut(fhir []LogMsg) []LogMsg {
	preps := make([]*Prep, 0, len(fhir))
	for i, m := range fhir {
		p := *t.preps[origSeq(m)]
		p.Seq = uint64(i + 1)
		preps = append(preps, &p)
	}
	rp := replay(nil, preps, t.gbm, t.iso, t.P, false)
	out := []LogMsg{}
	for _, o := range rp.outs {
		loop := ""
		if o.Dec != nil {
			loop = o.Dec.LoopID
		} else {
			loop = o.Item.LoopID
		}
		e := OutEnvelope{Kind: o.Kind, OSeq: o.OSeq, Dec: o.Dec, Item: o.Item, Prev: o.Prev, Hash: o.Hash}
		b, _ := json.Marshal(e)
		out = append(out, LogMsg{Seq: uint64(len(out) + 1), Subject: "out.p" + itoa(partOf(loop, t.P)), Hdr: map[string]string{"Nats-Msg-Id": o.ID}, Data: b})
	}
	return out
}

// ---------------- strong-attacker helpers ----------------

func rehashChain(ch []Block) []Block {
	var out []Block
	for _, b := range ch {
		out = appendBlock(out, b.Type, b.Body)
	}
	return out
}

func reSig(d *Decision) { d.Sig = decSig(d) }

// releger recomputes the whole v6 ledger of every partition (attacker without the anchors).
func releger(out []LogMsg) {
	by := map[string][]int{}
	for i, m := range out {
		by[m.Subject] = append(by[m.Subject], i)
	}
	for _, idx := range by {
		sort.SliceStable(idx, func(a, b int) bool { return decodeEnv(out[idx[a]]).OSeq < decodeEnv(out[idx[b]]).OSeq })
		prev := ""
		for k, i := range idx {
			e := decodeEnv(out[i])
			e.OSeq = uint64(k + 1)
			id, body := "", []byte(nil)
			if e.Dec != nil {
				id, body = e.Dec.ID, canon(e.Dec)
			} else {
				id, body = e.Item.ItemID, canon(e.Item)
			}
			e.Prev, e.Hash = prev, ledgerHash(prev, e.OSeq, id, e.Kind, body)
			prev = e.Hash
			encodeEnv(&out[i], e)
		}
	}
}

// ---------------- driver ----------------

func tamper(data, wasl, run string, P int, role string, seed int64) {
	if err := Authorize(role, ActReadAudit); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	t := &tamperCtx{data: data, wasl: wasl, run: run, P: P}
	t.fhir = readLog(filepath.Join(run, "fhir_log.ndjson"))
	t.out = readLog(filepath.Join(run, "out_log.ndjson"))
	t.v6 = len(t.out) > 0 && decodeEnv(t.out[0]).Hash != ""
	if !t.v6 { // verify a v5 run with v5 semantics (no outreach intents, v5 sign-offs carry no role)
		OutreachOn, RBACEnforce = false, false
	}
	if b, err := os.ReadFile(filepath.Join(run, "anchors.json")); err == nil && t.v6 {
		json.Unmarshal(b, &t.anchors)
	}
	t.enc = LoadEncoder(filepath.Join(wasl, "encoder_weights.json"))
	t.gbm = LoadGBM(filepath.Join(wasl, "lgb_stream.json"))
	t.iso = LoadIso(filepath.Join(wasl, "iso.json"))
	Judge = LoadJudge2(filepath.Join(wasl, "gemma_cache.json"))
	Parse = LoadParser(filepath.Join(wasl, "parse_cache.json")) // v7 merge fix: v6-impact prep parses replies via this cache
	for i := range t.fhir {
		t.fhir[i].Hdr["X-Orig-Seq"] = utoa(t.fhir[i].Seq)
	}
	pl := prepAll(t.fhir, t.enc)
	t.preps = map[uint64]*Prep{}
	for i, p := range pl {
		t.preps[t.fhir[i].Seq] = p
	}
	t.manifest = map[string]bool{}
	for _, pc := range Providers {
		b, _ := os.ReadFile(filepath.Join(data, "fhir_"+pc.ID+".ndjson"))
		for _, line := range strings.Split(string(b), "\n") {
			var g struct {
				MsgID string `json:"msgid"`
			}
			if json.Unmarshal([]byte(line), &g) == nil && g.MsgID != "" {
				t.manifest[g.MsgID] = true
			}
		}
	}
	expBase := t.expected(t.fhir)

	// target: the first closed, human-signed loop in the live OUT log (its final decision and the sign-off that closed it)
	finalIdx := map[string]int{}
	decIdx := map[string][]int{}
	for i, m := range t.out {
		oe := decodeEnv(m)
		if oe.Dec != nil {
			decIdx[oe.Dec.LoopID] = append(decIdx[oe.Dec.LoopID], i)
			if j, ok := finalIdx[oe.Dec.LoopID]; !ok || decodeEnv(t.out[j]).OSeq < oe.OSeq {
				finalIdx[oe.Dec.LoopID] = i
			}
		}
	}
	loops := []string{}
	for l := range finalIdx {
		loops = append(loops, l)
	}
	sort.Strings(loops)
	target, tIdx, soItem := "", -1, ""
	for _, l := range loops {
		d := decodeEnv(t.out[finalIdx[l]]).Dec
		if d.State != "closed" || len(decIdx[l]) < 2 {
			continue
		}
		for _, b := range d.Chain {
			if b.Type == "human_signoff" {
				target, tIdx, soItem = l, finalIdx[l], fmt.Sprint(b.Body["item"])
			}
		}
		if target != "" {
			break
		}
	}
	soPos := -1
	for i, m := range t.fhir {
		if m.Hdr["Nats-Msg-Id"] == "signoff/"+soItem {
			soPos = i
		}
	}
	editDec := func(out []LogMsg, i int, f func(d *Decision)) {
		oe := decodeEnv(out[i])
		f(oe.Dec)
		encodeEnv(&out[i], oe)
	}

	type attack struct {
		name, target, variant string
		fhir, out             []LogMsg
	}
	attacks := []attack{{name: "none (baseline)", target: "-", variant: "-", fhir: t.fhir, out: t.out}}
	add := func(name, tgt, variant string, fh, out []LogMsg) {
		attacks = append(attacks, attack{name, tgt, variant, fh, out})
	}
	strongOut := func(out []LogMsg) {
		renumber(out)
		if t.v6 {
			releger(out)
		}
	}
	// --- provenance chain of the target's closing decision
	for _, strong := range []bool{false, true} {
		v := map[bool]string{false: "naive", true: "strong"}[strong]
		for _, a := range []string{"delete", "reorder", "truncate"} {
			o := cloneLog(t.out)
			editDec(o, tIdx, func(d *Decision) {
				ch := append([]Block{}, d.Chain...)
				switch a {
				case "delete": // drop the evidence block
					ch = append(ch[:1:1], ch[2:]...)
				case "reorder":
					ch[0], ch[1] = ch[1], ch[0]
				case "truncate": // drop human_signoff + closure_decision
					ch = ch[:len(ch)-2]
				}
				if strong {
					ch = rehashChain(ch)
					if a == "truncate" {
						ch = appendBlock(ch, "closure_decision", d.Chain[len(d.Chain)-1].Body)
					}
				}
				d.Chain = ch
				if strong {
					reSig(d)
				}
			})
			if strong {
				strongOut(o)
			}
			add("provenance chain: "+a, "closing decision of "+target, v, t.fhir, o)
		}
	}
	// --- OUT log (decisions)
	for _, strong := range []bool{false, true} {
		v := map[bool]string{false: "naive", true: "strong"}[strong]
		o := cloneLog(t.out)
		o = append(o[:tIdx:tIdx], o[tIdx+1:]...)
		if strong {
			// close the OSeq gap in that partition
			sub := t.out[tIdx].Subject
			cut := decodeEnv(t.out[tIdx]).OSeq
			for i := range o {
				if o[i].Subject == sub {
					oe := decodeEnv(o[i])
					if oe.OSeq > cut {
						oe.OSeq--
						encodeEnv(&o[i], oe)
					}
				}
			}
			strongOut(o)
		}
		add("decision log: delete", "closing decision of "+target, v, t.fhir, o)
		o = cloneLog(t.out)
		ds := decIdx[target]
		a, b := ds[len(ds)-2], ds[len(ds)-1]
		ea, eb := decodeEnv(o[a]), decodeEnv(o[b])
		ea.OSeq, eb.OSeq = eb.OSeq, ea.OSeq // stale decision now claims to be the latest
		encodeEnv(&o[a], ea)
		encodeEnv(&o[b], eb)
		o[a], o[b] = o[b], o[a] // positions swapped; naive: stream seq numbers travel with the messages
		if strong {
			strongOut(o)
		}
		add("decision log: reorder", "last two decisions of "+target, v, t.fhir, o)
	}
	o := cloneLog(t.out)
	add("decision log: truncate 5% tail", "OUT stream", "strong", t.fhir, o[:len(o)*95/100])
	// --- FHIR input log
	for _, strong := range []bool{false, true} {
		v := map[bool]string{false: "naive", true: "strong"}[strong]
		f := cloneLog(t.fhir)
		f = append(f[:soPos:soPos], f[soPos+1:]...)
		if strong {
			renumber(f)
		}
		add("FHIR log: delete sign-off", "Provenance signoff/"+soItem, v, f, t.out)
		f = cloneLog(t.fhir)
		f[soPos], f[soPos-1] = f[soPos-1], f[soPos]
		if strong {
			renumber(f)
		}
		add("FHIR log: reorder", "sign-off vs previous event", v, f, t.out)
	}
	f := cloneLog(t.fhir)
	add("FHIR log: truncate 2% tail", "FHIR stream", "strong", f[:len(f)*98/100], t.out)

	// --- full consistent rewrite (insider with write access to both streams): delete the sign-off from the FHIR log,
	// re-run the deterministic pipeline on the doctored log and re-emit a self-consistent OUT log + ledger.
	if t.v6 {
		f := cloneLog(t.fhir)
		f = append(f[:soPos:soPos], f[soPos+1:]...)
		renumber(f)
		add("full rewrite: delete sign-off, re-derive OUT", "both streams", "insider", f, t.synthOut(f))
	}

	rows := []map[string]interface{}{}
	for _, at := range attacks {
		exp := expBase
		if &at.fhir[0] != &t.fhir[0] || len(at.fhir) != len(t.fhir) {
			exp = t.expected(at.fhir)
		}
		r := map[string]string{
			"D1_seq_fhir": d1Seq(at.fhir), "D1_seq_out": d1Seq(at.out), "D2_oseq": d2OSeq(at.out), "D3_provenance": d3Prov(at.out),
			"D4_replay": d4Replay(exp, at.out), "D5_ledger_anchor": d5Ledger(at.out, t.anchors), "D6_ingest": t.d6Ingest(at.fhir)}
		if t.anchors != nil && t.anchors["fhir"] != "" {
			r["D5_fhir_anchor"] = ""
			if fhirAnchor(stripOrig(at.fhir)) != t.anchors["fhir"] {
				r["D5_fhir_anchor"] = "fhir log != anchor"
			}
		}
		if at.name != "none (baseline)" && !strings.HasPrefix(at.name, "FHIR") {
			r["D6_ingest"] = "" // OUT-side attacks leave the input untouched
		}
		fired := []string{}
		keys := []string{}
		for k := range r {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if r[k] != "" && r[k] != "n/a" {
				fired = append(fired, k)
			}
		}
		rows = append(rows, map[string]interface{}{"attack": at.name, "target": at.target, "attacker": at.variant, "detectors": r,
			"fired": fired, "detected": len(fired) > 0})
		fmt.Fprintf(os.Stderr, "%-34s %-7s fired=%v\n", at.name, at.variant, fired)
	}
	res := map[string]interface{}{"run": run, "v6_ledger": t.v6, "fhir_msgs": len(t.fhir), "out_msgs": len(t.out), "target_loop": target,
		"target_signoff": soItem, "anchors": t.anchors, "attacks": rows}
	b, _ := json.MarshalIndent(res, "", " ")
	fmt.Println(string(b))
}
