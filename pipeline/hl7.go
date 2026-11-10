package main

// Stage 4: production-shaped HL7 v2 ingest (Malaffi's public interface is HL7 v2 ADT/ORU; FHIR stays as the mock tier).
//
//   hl7send  (mock HIE sender)  --MLLP/TCP-->  hl7mllp (listener)  --NATS JetStream fhir.<prov>.res-->  prep/linker/closure
//
// Sender: turns each cohort resource into one HL7 v2.5.1 message (ADT^A04 patient registration/encounter, ORM^O01 referral
// order, ORU^R01 procedure/pathology result, SIU^S12 appointment, MDM^T02 communication/consent/sign-off), framed with MLLP
// (0x0B ... 0x1C 0x0D). Faults: duplicate re-send, adjacent reorder, connection drop mid-stream (resumes from last ACK).
// Listener: parses MSH/PID/ORC/OBR/OBX/SCH/TXA, maps to the SAME internal event schema the FHIR path produces, and publishes
// with Nats-Msg-Id = <prov>/<Type>/<id>/v<vid>, so JetStream dedupe absorbs duplicates and replays across reconnects.
// Only after the publish is acknowledged does it return MSA|AA (the ACK is the durability point). Raw-message sha256,
// source system (MSH-4) and control id (MSH-10) are attached as headers for provenance.
//
// Mapping note: an HL7 v2 message carries the clinical payload in segments; Wasl-specific fields with no standard HL7 home
// (template id, consent actor role, sign-off JSON) travel in a Z-segment (ZWS). The full FHIR resource is also carried,
// base64, in ZWS-2 so the mock round-trips losslessly — the listener rebuilds from segments first and falls back to ZWS-2
// only for fields it cannot express in v2. hl7_test.go asserts segment-derived fields equal the original resource.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	mllpStart = 0x0b
	mllpEnd1  = 0x1c
	mllpEnd2  = 0x0d

	maxMLLPFrameBytes    = 1 << 20 // one MiB of clinical payload, excluding MLLP delimiters
	maxMLLPPreambleBytes = 64 << 10
)

// ---------- encoding: FHIR resource -> HL7 v2 message ----------

func hl7Esc(s string) string {
	r := strings.NewReplacer(`\`, `\E\`, `|`, `\F\`, `^`, `\S\`, `&`, `\T\`, `~`, `\R\`, "\r", `\X0D\`, "\n", `\X0A\`)
	return r.Replace(s)
}

func hl7Unesc(s string) string {
	r := strings.NewReplacer(`\F\`, `|`, `\S\`, `^`, `\T\`, `&`, `\R\`, `~`, `\X0D\`, "\r", `\X0A\`, "\n", `\E\`, `\`)
	return r.Replace(s)
}

func hl7TS(iso string) string {
	t, err := parseClinicalTime(iso)
	if err != nil {
		return ""
	}
	return t.UTC().Format("20060102150405")
}

func hl7TimeToFHIR(value string) (string, error) {
	base := value
	fraction := ""
	if len(value) > 14 && value[14] == '.' {
		base, fraction = value[:14], value[14:]
		for _, r := range value[15:] {
			if r < '0' || r > '9' {
				return "", fmt.Errorf("invalid_timestamp: invalid HL7 timestamp fraction")
			}
		}
		if len(fraction) == 1 {
			return "", fmt.Errorf("invalid_timestamp: empty HL7 timestamp fraction")
		}
	}
	var out string
	switch len(base) {
	case 4:
		out = base
	case 6:
		out = base[:4] + "-" + base[4:6]
	case 8:
		out = base[:4] + "-" + base[4:6] + "-" + base[6:8]
	case 14:
		out = base[:4] + "-" + base[4:6] + "-" + base[6:8] + "T" + base[8:10] + ":" + base[10:12] + ":" + base[12:14] + fraction + "Z"
	default:
		return "", fmt.Errorf("invalid_timestamp: unsupported HL7 timestamp precision")
	}
	if _, err := tsToDay(out); err != nil {
		return "", fmt.Errorf("invalid_timestamp: %v", err)
	}
	return out, nil
}

func hl7ClinicalStatus(value string) (string, bool) {
	if diagnosticReportStatuses[value] {
		return value, true
	}
	status, ok := map[string]string{
		"F": "final", "C": "corrected", "P": "preliminary", "S": "partial",
		"O": "registered", "X": "cancelled", "E": "entered-in-error", "U": "unknown",
	}[value]
	return status, ok
}

// applyClinicalOBR makes OBR authoritative before any specialised converter or
// ZWS fallback can preserve a conflicting, clinically safer-looking value.
func (m *HL7Msg) applyClinicalOBR(res map[string]interface{}) error {
	rt := gs(res, "resourceType")
	if rt != "DiagnosticReport" && rt != "ServiceRequest" {
		return nil
	}
	obr := m.seg("OBR")
	if obr == nil {
		return normalizationError(rt, gs(res, "id"), "invalid_resource", "OBR", "clinical HL7 resource requires OBR")
	}
	code := strings.Split(fld(obr, 4), "^")
	if rt == "DiagnosticReport" && (len(code) == 0 || code[0] == "") {
		return normalizationError(rt, gs(res, "id"), "unknown_programme", "OBR-4", "clinical code is required")
	}
	if rt == "ServiceRequest" && len(code) < 2 && (len(code) == 0 || code[0] == "") {
		return normalizationError(rt, gs(res, "id"), "unknown_programme", "OBR-4", "clinical programme text is required")
	}
	if rt == "DiagnosticReport" {
		timestamp, err := hl7TimeToFHIR(fld(obr, 7))
		if err != nil {
			return normalizationError(rt, gs(res, "id"), "invalid_timestamp", "OBR-7", err.Error())
		}
		status, ok := hl7ClinicalStatus(fld(obr, 25))
		if !ok {
			return normalizationError(rt, gs(res, "id"), "invalid_report_status", "OBR-25", "missing or unsupported report status")
		}
		res["effectiveDateTime"] = timestamp
		res["status"] = status
		res["code"] = map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": code[0]}}}
		return nil
	}
	timeField := 6
	timestamp, err := hl7TimeToFHIR(fld(obr, timeField))
	if err != nil {
		return normalizationError(rt, gs(res, "id"), "invalid_timestamp", fmt.Sprintf("OBR-%d", timeField), err.Error())
	}
	res["authoredOn"] = timestamp
	text := code[0]
	if len(code) > 1 {
		text = code[1]
	}
	res["code"] = map[string]interface{}{"text": text}
	return nil
}

var hl7Types = map[string]string{"Encounter": "ADT^A04", "ServiceRequest": "ORM^O01", "DiagnosticReport": "ORU^R01",
	"Appointment": "SIU^S12", "Task": "ORM^O01", "Communication": "MDM^T02", "Consent": "MDM^T02", "Provenance": "MDM^T02"}

// EncodeHL7 builds one HL7 v2.5.1 message for a cohort resource. ctl = MSH-10 control id.
func EncodeHL7(prov string, res map[string]interface{}, ctl string) []byte {
	rt := gs(res, "resourceType")
	mt := hl7Types[rt]
	if mt == "" {
		mt = "MDM^T02"
	}
	meta, _ := res["meta"].(map[string]interface{})
	segs := []string{fmt.Sprintf("MSH|^~\\&|%s|%s|WASL|ADHIE|%s||%s|%s|P|2.5.1", hl7Esc(prov), hl7Esc(prov), hl7TS(gs(meta, "lastUpdated")), mt, hl7Esc(ctl))}
	if c, ok := res["contained"].([]interface{}); ok && len(c) > 0 {
		pt := c[0].(map[string]interface{})
		ids := []string{}
		for _, idv := range pt["identifier"].([]interface{}) {
			id := idv.(map[string]interface{})
			sys := gs(id, "system")
			typ := "MR"
			if strings.Contains(sys, "emirates-id") {
				typ = "NI"
			}
			ids = append(ids, hl7Esc(gs(id, "value"))+"^^^"+hl7Esc(prov)+"^"+typ)
		}
		name := ""
		if ns, ok := pt["name"].([]interface{}); ok && len(ns) > 0 {
			name = hl7Esc(gs(ns[0].(map[string]interface{}), "text"))
		}
		sex := map[string]string{"male": "M", "female": "F"}[gs(pt, "gender")]
		if sex == "" {
			sex = "U"
		}
		lang := ""
		if cm, ok := pt["communication"].([]interface{}); ok && len(cm) > 0 {
			if lg, ok := cm[0].(map[string]interface{})["language"].(map[string]interface{}); ok {
				if cd, ok := lg["coding"].([]interface{}); ok && len(cd) > 0 {
					lang = gs(cd[0].(map[string]interface{}), "code")
				}
			}
		}
		dob := strings.ReplaceAll(gs(pt, "birthDate"), "-", "")
		segs = append(segs, fmt.Sprintf("PID|1||%s||%s||%s|%s|||||||%s", strings.Join(ids, "~"), name, dob, sex, hl7Esc(lang)))
	}
	switch rt {
	case "ServiceRequest", "Task":
		if dc := hl7DRCode(res); dc != "" { // diabetic retinopathy tracking order: local-coded ORM (hl7dr.go)
			segs = append(segs, hl7DRSegs(res, dc)...)
			break
		}
		if bc := hl7BreastCode(res); bc != "" { // breast screening tracking order: local-coded ORM (hl7breast.go)
			segs = append(segs, hl7BreastSegs(res, bc)...)
			break
		}
		segs = append(segs, fmt.Sprintf("ORC|NW|%s||||%s", hl7Esc(gs(res, "id")), hl7Esc(gs(res, "status"))))
		segs = append(segs, fmt.Sprintf("OBR|1|%s||^%s|%s|%s", hl7Esc(gs(res, "id")), hl7Esc(gs(res, "code", "text")), hl7Esc(gs(res, "priority")), hl7TS(gs(res, "authoredOn"))))
	case "DiagnosticReport":
		if dc := hl7DRCode(res); dc != "" { // diabetic retinopathy screen / eye-clinic note: local-coded ORU (hl7dr.go)
			segs = append(segs, hl7DRSegs(res, dc)...)
			break
		}
		if bc := hl7BreastCode(res); bc != "" { // breast screening mammogram / assessment document: local-coded ORU (hl7breast.go)
			segs = append(segs, hl7BreastSegs(res, bc)...)
			break
		}
		if lc := hl7LungCode(res); lc != "" { // lung-nodule reports: LOINC-coded ORU (hl7lung.go)
			segs = append(segs, hl7LungORU(res, lc)...)
			break
		}
		code := ""
		if cs, ok := res["code"].(map[string]interface{}); ok {
			if cd, ok := cs["coding"].([]interface{}); ok && len(cd) > 0 {
				code = gs(cd[0].(map[string]interface{}), "code")
			}
		}
		status := hl7Status[gs(res, "status")]
		if status == "" {
			status = gs(res, "status")
		}
		obr := make([]string, 26)
		obr[0], obr[1], obr[3], obr[4] = "OBR", "1", hl7Esc(gs(res, "id")), hl7Esc(code)+"^^LN"
		obr[7], obr[25] = hl7TS(gs(res, "effectiveDateTime")), hl7Esc(status)
		segs = append(segs, strings.Join(obr, "|"))
		if pf, ok := res["presentedForm"].([]interface{}); ok && len(pf) > 0 {
			if d := gs(pf[0].(map[string]interface{}), "data"); d != "" {
				if raw, err := base64.StdEncoding.DecodeString(d); err == nil {
					segs = append(segs, "OBX|1|TX|"+hl7Esc(code)+"^^LN||"+hl7Esc(string(raw))+"||||||F")
				}
			}
		}
		if cc, ok := res["conclusionCode"].([]interface{}); ok && len(cc) > 0 {
			segs = append(segs, "OBX|2|CE|conclusion||"+hl7Esc(gs(cc[0].(map[string]interface{}), "text"))+"||||||F")
		}
	case "Appointment":
		segs = append(segs, fmt.Sprintf("SCH|%s||||||||||^^^%s||||||||||||||%s", hl7Esc(gs(res, "id")), hl7TS(gs(res, "start")), hl7Esc(gs(res, "status"))))
	case "Encounter":
		segs = append(segs, fmt.Sprintf("PV1|1|O|||||||||||||||||%s||||||||||||||||||||||||%s", hl7Esc(gs(res, "id")), hl7TS(gs(res, "period", "start"))))
	default: // Communication, Consent, Provenance: document notification
		segs = append(segs, fmt.Sprintf("TXA|1|%s|TX||||||||||%s||||||%s", hl7Esc(rt), hl7Esc(gs(res, "id")), hl7Esc(gs(res, "status"))))
	}
	full, _ := json.Marshal(res)
	segs = append(segs, fmt.Sprintf("ZWS|%s|%s|%s", hl7Esc(rt+"/"+gs(res, "id")+"/v"+gs(meta, "versionId")), base64.StdEncoding.EncodeToString(full), hl7Esc(gs(meta, "versionId"))))
	return []byte(strings.Join(segs, "\r") + "\r")
}

// ---------- decoding: HL7 v2 message -> resource bytes for the existing normaliser ----------

type HL7Msg struct {
	Segs    map[string][][]string
	Control string
	Sender  string
	Type    string
}

func ParseHL7(b []byte) (*HL7Msg, error) {
	m := &HL7Msg{Segs: map[string][][]string{}}
	singleton := map[string]bool{"MSH": true, "PID": true, "ORC": true, "OBR": true, "SCH": true, "PV1": true, "TXA": true, "ZWS": true}
	segment := 0
	for _, line := range strings.Split(strings.TrimRight(string(b), "\r\n"), "\r") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "|")
		if f[0] == "" {
			return nil, fmt.Errorf("hl7: empty segment name")
		}
		if segment == 0 && f[0] != "MSH" {
			return nil, fmt.Errorf("hl7: MSH must be the first segment")
		}
		if singleton[f[0]] && len(m.Segs[f[0]]) != 0 {
			return nil, fmt.Errorf("hl7: duplicate %s segment", f[0])
		}
		m.Segs[f[0]] = append(m.Segs[f[0]], f)
		segment++
	}
	msh := m.seg("MSH")
	if msh == nil || len(msh) < 12 {
		return nil, fmt.Errorf("hl7: missing/short MSH")
	}
	m.Sender, m.Type, m.Control = hl7Unesc(msh[3]), msh[8], hl7Unesc(msh[9])
	if m.Sender == "" || m.Type == "" || m.Control == "" {
		return nil, fmt.Errorf("hl7: MSH sender, message type and control id are required")
	}
	return m, nil
}

func (m *HL7Msg) seg(name string) []string {
	if s := m.Segs[name]; len(s) > 0 {
		return s[0]
	}
	return nil
}

func fld(s []string, i int) string {
	if i < len(s) {
		return hl7Unesc(s[i])
	}
	return ""
}

// ToResource rebuilds the internal resource: patient identity/demographics from PID, clinical payload from OBR/OBX/SCH/PV1,
// and the remaining Wasl-specific fields from ZWS. Segment values override the ZWS copy (they are the v2 source of truth).
func (m *HL7Msg) ToResource() (prov string, msgID string, res map[string]interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			prov, msgID, res = "", "", nil
			err = fmt.Errorf("hl7: malformed embedded resource shape")
			return
		}
		if err != nil {
			return
		}
		raw, marshalErr := json.Marshal(res)
		if marshalErr != nil {
			prov, msgID, res = "", "", nil
			err = fmt.Errorf("hl7: embedded resource cannot be encoded: %v", marshalErr)
			return
		}
		if event, signoff, normErr := normaliseChecked(prov, 0, raw); event == nil && signoff == nil {
			prov, msgID, res = "", "", nil
			if normErr != nil {
				err = fmt.Errorf("hl7: %w", normErr)
			} else {
				err = fmt.Errorf("hl7: embedded resource is not normalisable")
			}
		}
	}()
	z := m.seg("ZWS")
	if z == nil || len(z) < 4 {
		return "", "", nil, fmt.Errorf("hl7: missing ZWS identity, body or version")
	}
	raw, err := base64.StdEncoding.DecodeString(z[2])
	if err != nil {
		return "", "", nil, fmt.Errorf("hl7: ZWS-2: %v", err)
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return "", "", nil, fmt.Errorf("hl7: embedded resource: %v", err)
	}
	if err := json.Unmarshal(raw, &res); err != nil || res == nil || gs(res, "resourceType") == "" {
		return "", "", nil, fmt.Errorf("hl7: embedded resource must be a typed FHIR object")
	}
	prov = m.Sender
	identity := strings.Split(fld(z, 1), "/")
	if len(identity) != 3 || identity[0] == "" || identity[1] == "" || !strings.HasPrefix(identity[2], "v") {
		return "", "", nil, fmt.Errorf("hl7: ZWS-1 must be ResourceType/id/vversion")
	}
	resourceType, resourceID, version, _, parseErr := parseResourceVersion(raw)
	if parseErr != nil {
		return "", "", nil, fmt.Errorf("hl7: embedded resource metadata: %v", parseErr)
	}
	if identity[0] != resourceType || identity[1] != resourceID || identity[2] != "v"+version {
		return "", "", nil, fmt.Errorf("hl7: ZWS-1 and embedded resource identity differ")
	}
	if fld(z, 3) != version {
		return "", "", nil, fmt.Errorf("hl7: ZWS-1/ZWS-3 and embedded resource versions differ")
	}
	if resourceType == "DiagnosticReport" && fld(m.seg("OBR"), 3) != resourceID {
		return "", "", nil, fmt.Errorf("hl7: OBR-3 and embedded DiagnosticReport id differ")
	}
	msgID = prov + "/" + fld(z, 1)
	if pid := m.seg("PID"); pid != nil {
		contained, ok := res["contained"].([]interface{})
		if !ok || len(contained) == 0 {
			return "", "", nil, fmt.Errorf("hl7: PID requires a contained Patient")
		}
		pt, ok := contained[0].(map[string]interface{})
		if !ok || pt == nil {
			return "", "", nil, fmt.Errorf("hl7: contained Patient must be an object")
		}
		ids := []interface{}{}
		for _, rep := range strings.Split(fld(pid, 3), "~") {
			c := strings.Split(rep, "^")
			if len(c) < 5 {
				continue
			}
			sys := "urn:ae:" + prov + ":mrn"
			if c[4] == "NI" {
				sys = "http://doh.gov.ae/fhir/sid/emirates-id"
			}
			ids = append(ids, map[string]interface{}{"system": sys, "value": hl7Unesc(c[0])})
		}
		pt["identifier"] = ids
		if n := fld(pid, 5); n != "" {
			pt["name"] = []interface{}{map[string]interface{}{"text": n}}
		}
		if d := fld(pid, 7); len(d) == 8 {
			pt["birthDate"] = d[:4] + "-" + d[4:6] + "-" + d[6:]
		}
		pt["gender"] = map[string]string{"M": "male", "F": "female"}[fld(pid, 8)]
		if pt["gender"] == "" {
			pt["gender"] = "unknown"
		}
	}
	if err := m.applyClinicalOBR(res); err != nil {
		return "", "", nil, fmt.Errorf("hl7: %w", err)
	}
	if rt := gs(res, "resourceType"); (rt == "DiagnosticReport" || rt == "ServiceRequest") && m.hl7DRRebuild(res) { // hl7dr.go: OBR-4 decides
		return prov, msgID, res, nil
	}
	if rt := gs(res, "resourceType"); rt == "DiagnosticReport" || rt == "ServiceRequest" {
		if m.hl7BreastRebuild(res) { // hl7breast.go
			return prov, msgID, res, nil
		}
		if hl7BreastRebuildOn && hl7BreastCode(res) != "" {
			return "", "", nil, fmt.Errorf("hl7: invalid breast segments")
		}
	}
	if gs(res, "resourceType") == "DiagnosticReport" && m.hl7LungRebuild(res) {
		return prov, msgID, res, nil
	}
	if gs(res, "resourceType") == "DiagnosticReport" {
		delete(res, "presentedForm")
		delete(res, "conclusion")
		delete(res, "conclusionCode")
		for _, obx := range m.Segs["OBX"] {
			switch fld(obx, 2) {
			case "TX":
				res["presentedForm"] = []interface{}{map[string]interface{}{"contentType": "text/plain; charset=utf-8", "data": base64.StdEncoding.EncodeToString([]byte(fld(obx, 5)))}}
			case "CE", "CWE":
				res["conclusionCode"] = []interface{}{map[string]interface{}{"text": strings.Split(fld(obx, 5), "^")[0]}}
			}
		}
	}
	return prov, msgID, res, nil
}

func mllpFrame(b []byte) []byte {
	out := append([]byte{mllpStart}, b...)
	return append(out, mllpEnd1, mllpEnd2)
}

// readMLLPUntil is the bounded replacement for ReadBytes: ErrBufferFull fragments
// are accumulated only while the complete delimiter-terminated value fits limit.
func readMLLPUntil(r *bufio.Reader, delim byte, limit int) ([]byte, error) {
	out := make([]byte, 0, 4096)
	for {
		frag, err := r.ReadSlice(delim)
		if len(out)+len(frag) > limit {
			return nil, fmt.Errorf("mllp: value exceeds %d bytes", limit)
		}
		out = append(out, frag...)
		switch err {
		case nil:
			return out, nil
		case bufio.ErrBufferFull:
			continue
		default:
			return nil, err
		}
	}
}

// readMLLP reads one bounded framed message. Oversized preambles and unterminated
// frames are rejected without growing memory with the peer's input.
func readMLLP(r *bufio.Reader) ([]byte, error) {
	if _, err := readMLLPUntil(r, mllpStart, maxMLLPPreambleBytes); err != nil {
		return nil, err
	}
	b, err := readMLLPUntil(r, mllpEnd1, maxMLLPFrameBytes+1)
	if err != nil {
		return nil, err
	}
	if c, err := r.ReadByte(); err != nil || c != mllpEnd2 {
		return nil, fmt.Errorf("mllp: bad trailer")
	}
	return b[:len(b)-1], nil
}

func hl7Ack(m *HL7Msg, code, text string) []byte {
	return []byte(fmt.Sprintf("MSH|^~\\&|WASL|ADHIE|%s|%s|%s||ACK|%s|P|2.5.1\rMSA|%s|%s|%s\r",
		hl7Esc(m.Sender), hl7Esc(m.Sender), time.Now().UTC().Format("20060102150405"), hl7Esc(m.Control)+"-ack", code, hl7Esc(m.Control), hl7Esc(text)))
}

// ---------- listener ----------

func runHL7Listener(addr, runDir string) {
	nc, js := connect("hl7mllp")
	defer nc.Close()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(err)
	}
	var msgs, dups, naks, conns, replays int64
	seen := openSeen(js)
	seen.CatchUp(js, "FHIR", 4*seenQueue)
	start := time.Now()
	go func() {
		for {
			time.Sleep(300 * time.Millisecond)
			writeStatus(runDir, "hl7mllp", map[string]interface{}{"msgs": atomic.LoadInt64(&msgs), "dedup_absorbed": atomic.LoadInt64(&dups),
				"nak": atomic.LoadInt64(&naks), "connections": atomic.LoadInt64(&conns), "already_ingested_dropped": atomic.LoadInt64(&replays),
				"secs": time.Since(start).Seconds()})
		}
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		atomic.AddInt64(&conns, 1)
		go func(c net.Conn) {
			defer c.Close()
			r := bufio.NewReaderSize(c, 1<<20)
			for {
				b, err := readMLLP(r)
				if err != nil {
					return
				}
				m, err := ParseHL7(b)
				if err != nil {
					atomic.AddInt64(&naks, 1)
					c.Write(mllpFrame([]byte("MSH|^~\\&|WASL|ADHIE||||||ACK|x|P|2.5.1\rMSA|AE|x|" + hl7Esc(err.Error()) + "\r")))
					continue
				}
				if m.Type == "EOF" { // mock end-of-feed marker -> per-provider EOF control messages
					for _, pc := range Providers {
						em := nats.NewMsg("fhir." + pc.ID + ".ctl")
						em.Data = []byte(`{"eof":true}`)
						em.Header.Set(jetstream.MsgIDHeader, pc.ID+"/eof")
						em.Header.Set("Wsi-Arr", strconv.FormatFloat(TEnd, 'f', 6, 64))
						pubSync(js, em)
					}
					c.Write(mllpFrame(hl7Ack(m, "AA", "eof")))
					continue
				}
				prov, mid, res, err := m.ToResource()
				if err != nil {
					atomic.AddInt64(&naks, 1)
					c.Write(mllpFrame(hl7Ack(m, "AE", err.Error())))
					continue
				}
				body, _ := json.Marshal(res)
				nm := nats.NewMsg("fhir." + prov + ".res")
				nm.Data = body
				nm.Header.Set(jetstream.MsgIDHeader, mid)
				h := sha256.Sum256(b)
				nm.Header.Set("Wsi-Raw-Sha", hex.EncodeToString(h[:]))
				nm.Header.Set("Wsi-Src", "hl7v2:"+m.Sender)
				nm.Header.Set("Wsi-Ctl", m.Control)
				nm.Header.Set("Wsi-Arr", m.arrival())
				nm.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
				dup, replay := seen.Publish(js, nm)
				if dup {
					atomic.AddInt64(&dups, 1)
				}
				if replay {
					atomic.AddInt64(&replays, 1)
				}
				atomic.AddInt64(&msgs, 1)
				c.Write(mllpFrame(hl7Ack(m, "AA", ""))) // ACK only after JetStream acknowledged the publish
			}
		}(c)
	}
}

// arrival: HIE receipt time in cohort days, carried in MSH-7.
func (m *HL7Msg) arrival() string {
	t, err := time.Parse("20060102150405.000", fld(m.seg("MSH"), 6))
	if err != nil {
		t, _ = time.Parse("20060102150405", fld(m.seg("MSH"), 6))
	}
	return strconv.FormatFloat(t.Sub(baseTime).Seconds()/86400, 'f', 6, 64)
}

// ---------- mock HIE sender ----------

type hl7Out struct {
	msgID string
	arr   float64
	b     []byte
}

func loadHL7Feed(dataDir string) []hl7Out {
	type ge struct {
		g    GenEvent
		prov string
	}
	all := []ge{}
	for _, pc := range Providers {
		f, err := os.Open(filepath.Join(dataDir, "fhir_"+pc.ID+".ndjson"))
		if err != nil {
			panic(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var g GenEvent
			json.Unmarshal(sc.Bytes(), &g)
			all = append(all, ge{g, pc.ID})
		}
		f.Close()
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].g.Arrival < all[j].g.Arrival })
	out := make([]hl7Out, len(all))
	for i, a := range all {
		b := EncodeHL7(a.prov, a.g.Res, fmt.Sprintf("%s-%07d", a.prov, i+1))
		// MSH-7 = HIE receipt time (cohort day) with ms precision, so arrival order survives the v2 round trip
		ts := baseTime.Add(time.Duration(a.g.Arrival * 86400 * float64(time.Second))).UTC().Format("20060102150405.000")
		parts := strings.SplitN(string(b), "|", 8)
		parts[6] = ts
		out[i] = hl7Out{a.g.MsgID, a.g.Arrival, []byte(strings.Join(parts, "|"))}
	}
	return out
}

// runHL7Sender streams the cohort over MLLP with faults; resumes after a dropped connection from the last ACKed message.
func runHL7Sender(dataDir, addr string, seed int64, dup, reorder float64, drops int, runDir string) {
	feed := loadHL7Feed(dataDir)
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i+1 < len(feed); i++ { // adjacent reorders within a provider-independent window
		if rng.Float64() < reorder {
			feed[i], feed[i+1] = feed[i+1], feed[i]
			i++
		}
	}
	dropAt := map[int]bool{}
	for k := 0; k < drops; k++ {
		dropAt[len(feed)*(k+1)/(drops+1)] = true
	}
	acked, sent, resent, reconnects := 0, 0, 0, 0
	start := time.Now()
	status := func(done bool) {
		writeStatus(runDir, "hl7send", map[string]interface{}{"messages": len(feed), "acked": acked, "sent": sent, "dup_resent": resent,
			"reconnects": reconnects, "done": done, "secs": time.Since(start).Seconds()})
	}
	var conn net.Conn
	var r *bufio.Reader
	dial := func() {
		for {
			c, err := net.Dial("tcp", addr)
			if err == nil {
				conn, r = c, bufio.NewReaderSize(c, 1<<16)
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	send := func(b []byte) bool {
		if _, err := conn.Write(mllpFrame(b)); err != nil {
			return false
		}
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		ack, err := readMLLP(r)
		if err != nil {
			return false
		}
		return bytes.Contains(ack, []byte("MSA|AA|"))
	}
	dial()
	for acked < len(feed) {
		if dropAt[acked] { // simulate a network drop: close without warning, re-dial, re-send the unACKed message
			delete(dropAt, acked)
			conn.Close()
			reconnects++
			dial()
			if acked > 0 { // a sender that lost the ACK re-sends the previous message too (at-least-once)
				send(feed[acked-1].b)
				resent++
			}
		}
		if !send(feed[acked].b) {
			conn.Close()
			reconnects++
			dial()
			continue
		}
		sent++
		if rng.Float64() < dup { // duplicate delivery of an already-ACKed message
			send(feed[acked].b)
			resent++
		}
		acked++
		if acked%500 == 0 {
			status(false)
		}
	}
	eof := []byte(fmt.Sprintf("MSH|^~\\&|ADHIE|ADHIE|WASL|ADHIE|%s||EOF|eof|P|2.5.1\r", time.Now().UTC().Format("20060102150405")))
	send(eof)
	conn.Close()
	status(true)
	fmt.Fprintf(os.Stderr, "hl7send: %d messages ACKed, %d duplicate re-sends, %d reconnects (%.1fs)\n", acked, resent, reconnects, time.Since(start).Seconds())
	_ = io.EOF
}
