package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// This matrix owns one ephemeral loopback child at a time. Broker wire logs,
// configs and JSON observations (including raw []byte as base64) outlive TempDir.
// No production publisher/provider/queue or durable metadata is connected here.
type ctmBroker struct {
	ctx      context.Context
	nc       *nats.Conn
	js       jetstream.JetStream
	stream   jetstream.Stream
	inc      canonicalIncarnation
	evidence string
	spy      *nats.Subscription
}

func ctmSave(t *testing.T, name string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("WASL_CRC_TRANSPORT_EVIDENCE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	name = strings.NewReplacer("/", "_", " ", "_").Replace(t.Name() + "-" + name)
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("observation=%s sha256=%x", path, sha256.Sum256(append(raw, '\n')))
}

func ctmStart(t *testing.T, domain bool) *ctmBroker {
	t.Helper()
	binary := os.Getenv("WASL_TEST_NATS_SERVER")
	if binary == "" {
		binary, _ = filepath.Abs("../bin/nats-server")
	}
	bin, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	// Version is asserted from the actual owned connection below. Record the
	// platform-specific executable hash as evidence, not a Darwin-only policy.
	t.Logf("owned broker binary=%s sha256=%x", binary, sha256.Sum256(bin))
	dir := os.Getenv("WASL_CRC_TRANSPORT_EVIDENCE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	store := t.TempDir()
	if err := os.Chmod(store, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("listen: 127.0.0.1:-1\njetstream { store_dir: %q, max_memory_store: 64MB, max_file_store: 64MB", store)
	if domain {
		cfg += ", domain: CRC"
	}
	cfg += " }\n"
	if domain {
		cfg += "mappings { 'CRC.CUSTOM.API.>': '$JS.API.>' }\n"
	}
	stem := filepath.Join(dir, t.Name())
	if err := os.WriteFile(stem+".conf", []byte(cfg), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(stem+".broker.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-c", stem+".conf", "-DV")
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = log.Close() })
	re := regexp.MustCompile(`Listening for client connections on (127\.0\.0\.1:[0-9]+)`)
	var endpoint string
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); {
		raw, _ := os.ReadFile(stem + ".broker.log")
		if match := re.FindSubmatch(raw); len(match) == 2 {
			endpoint = "nats://" + string(match[1])
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if endpoint == "" {
		t.Fatal("owned broker did not start; preserve broker log")
	}
	nc, err := nats.Connect(endpoint, nats.NoReconnect(), nats.Timeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	if nc.ConnectedServerVersion() != "2.12.6" || strings.HasSuffix(endpoint, ":4222") {
		t.Fatal("not pinned random-port broker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	t.Cleanup(cancel)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: canonicalStream, Subjects: []string{"fhir.ctm.>"}, AllowDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	spy, err := nc.SubscribeSync(">")
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	b := &ctmBroker{ctx: ctx, nc: nc, js: js, stream: stream, inc: canonicalIncarnation{Stream: info.Config.Name, Created: info.Created}, evidence: dir, spy: spy}
	ctmSave(t, "broker", map[string]any{"pid": cmd.Process.Pid, "argv": cmd.Args, "endpoint": endpoint, "version": nc.ConnectedServerVersion(), "binary_sha256": fmt.Sprintf("%x", sha256.Sum256(bin)), "config": cfg, "info": info})
	return b
}

func ctmID(body, name string) canonicalIdentity {
	return canonicalIdentity{Subject: "fhir.ctm.resource", FullID: "p1/DiagnosticReport/" + name + "/v1", BodySHA256: cfkHex(body), BodyLength: uint64(len(body))}
}

// Prefix authority here comes from the actual approved history helper, not a
// constructor-supplied digest. The helper result is not an absence grant for K
// in duplicate tests: those use the real generic producer and no attempt.
func ctmAttempt(t *testing.T, b *ctmBroker, id canonicalIdentity, extra nats.Header) *canonicalAttempt {
	t.Helper()
	history := &SubConsumer{js: b.js}
	result, err := history.canonicalHistory(b.ctx, id, 0)
	if err != nil || result.State != canonicalHistoryAbsent || result.Prefix == nil {
		t.Fatalf("actual prefix: %+v %v", result, err)
	}
	h := cfkFrozen(id)
	h["Nats-Crc-Provenance"] = []string{"keep", "also", "also"}
	h["x-Case"] = []string{"lower"}
	h["X-Case"] = []string{"upper", "twice"}
	h["X-Signature"] = []string{"original-signature-raw"}
	for k, v := range extra {
		h[k] = append([]string(nil), v...)
	}
	a := &canonicalAttempt{ID: id.FullID + "/attempt", Number: 1, Incarnation: b.inc, ExpectedLast: result.Prefix.End, Prefix: *result.Prefix, FrozenApplicationHeaders: h, WireHeaders: canonicalWireFor(h, canonicalStream, result.Prefix.End), SourceKey: canonicalSourcePrefix + cfkHex(id.FullID)}
	ctmSave(t, "actual-prefix-"+id.FullID, result)
	return a
}

func ctmPublish(t *testing.T, b *ctmBroker, id canonicalIdentity, a *canonicalAttempt, body string) (*nats.Msg, *jetstream.PubAck) {
	t.Helper()
	m := &nats.Msg{Subject: id.Subject, Header: canonicalCopyHeader(a.FrozenApplicationHeaders), Data: []byte(body)}
	ack, err := b.js.PublishMsg(b.ctx, m, jetstream.WithExpectStream(canonicalStream), jetstream.WithExpectLastSequence(a.ExpectedLast))
	if err != nil || ack == nil || ack.Duplicate || ack.Sequence != a.ExpectedLast+1 {
		t.Fatalf("real fresh publish: %+v %v", ack, err)
	}
	ctmSave(t, "actual-submit-ack-"+id.FullID, map[string]any{"submitted": m, "ack": ack})
	return m, ack
}

func ctmInfo(t *testing.T, b *ctmBroker) *jetstream.StreamInfo {
	t.Helper()
	info, err := b.stream.Info(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
func ctmUnchanged(t *testing.T, b *ctmBroker, before *jetstream.StreamInfo) {
	t.Helper()
	after := ctmInfo(t, b)
	if !reflect.DeepEqual(before.Config, after.Config) || !before.Created.Equal(after.Created) || before.State.Msgs != after.State.Msgs || before.State.LastSeq != after.State.LastSeq {
		t.Fatalf("lookup mutated store/config: before=%+v after=%+v", before, after)
	}
}
func ctmRequests(t *testing.T, b *ctmBroker) (ordinary, direct, publish int) {
	t.Helper()
	if err := b.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	var requests []string
	for {
		n, _, err := b.spy.Pending()
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		m, err := b.spy.NextMsg(time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(m.Subject, "STREAM.MSG.GET.FHIR") {
			ordinary++
			requests = append(requests, m.Subject)
		}
		if strings.Contains(m.Subject, "DIRECT.GET.FHIR") {
			direct++
			requests = append(requests, m.Subject)
		}
		if m.Subject == "fhir.ctm.resource" {
			publish++
		}
	}
	ctmSave(t, "requests", map[string]any{"ordinary": ordinary, "direct": direct, "publication_requests": publish, "subjects": requests, "stored": ctmInfo(t, b).State})
	return
}

func TestCanonicalTransportMatrixFreshReadConvergence(t *testing.T) {
	b := ctmStart(t, false)
	body := "{ \"resourceType\":\"DiagnosticReport\",\"raw\":\"signed\\nbytes\" }\n"
	id := ctmID(body, "fresh")
	a := ctmAttempt(t, b, id, nil)
	m, ack := ctmPublish(t, b, id, a, body)
	before := ctmInfo(t, b)
	c := &SubConsumer{js: b.js}
	fresh := c.resolveCanonicalAck(b.ctx, b.inc, id, a, m, ack, nil)
	if !fresh.Returned || fresh.Wire != "fresh" || fresh.ReturnKind != "fresh" || fresh.Proof == nil || fresh.Proof.Evidence.Kind != canonicalEvidenceFresh {
		t.Fatalf("fresh observation must prove submitted frozen bytes: %+v", fresh)
	}
	direct, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, a)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if direct.Transport != canonicalTransportDir || direct.Provenance.Envelope == nil || ordinary.Transport != canonicalTransportOrd || ordinary.Provenance.Envelope != nil {
		t.Fatal("transport provenance not explicit")
	}
	for _, read := range []canonicalRead{direct, ordinary} {
		core, err := canonicalCoreFromRead(read, a, id)
		if err != nil {
			t.Fatal(err)
		}
		fp, _ := canonicalCoreFingerprint(core)
		if fp != fresh.Proof.Fingerprint {
			t.Fatal("fresh/direct/ordinary cores diverged")
		}
		if !bytes.Equal(read.Message.Data, m.Data) {
			t.Fatal("raw body changed")
		}
	}
	if !reflect.DeepEqual(ordinary.Message.Header, m.Header) || !reflect.DeepEqual(fresh.Proof.Core.OriginalWsi, canonicalOriginalWsi(m.Header)) {
		t.Fatal("original headers/guards/Wsi not preserved")
	}
	if fresh.ObservedAck == ack || fresh.Proof.Evidence.ObservedAck == ack {
		t.Fatal("retain an immutable ACK copy")
	}
	ctmSave(t, "convergence", map[string]any{"fresh": fresh, "direct": direct, "ordinary": ordinary, "body_sha256": cfkHex(body)})
	ctmUnchanged(t, b, before)
	o, d, p := ctmRequests(t, b)
	if o != 1 || d != 1 || p != 1 {
		t.Fatalf("requests ordinary/direct/publish=%d/%d/%d", o, d, p)
	}
}

func TestCanonicalTransportMatrixOriginalHeadersFallback(t *testing.T) {
	b := ctmStart(t, false)
	c := &SubConsumer{js: b.js}
	cases := []struct {
		name  string
		extra nats.Header
	}{
		{"stream", nats.Header{jetstream.StreamHeader: {"original-stream", "again"}}},
		{"sequence", nats.Header{jetstream.SequenceHeader: {"original-sequence"}}},
		{"subject", nats.Header{jetstream.SubjectHeader: {"original-subject"}}},
		{"timestamp", nats.Header{jetstream.TimeStampHeaer: {"original-time"}}},
		{"case-collision", nats.Header{"nats-stream": {"lower-original"}}},
		{"wire-trimming", nats.Header{"X-Whitespace": {" leading "}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := "{\"raw\":\"" + tc.name + "\"}\n"
			id := ctmID(body, tc.name)
			a := ctmAttempt(t, b, id, tc.extra)
			m, ack := ctmPublish(t, b, id, a, body)
			before := ctmInfo(t, b)
			result := c.resolveCanonicalAck(b.ctx, b.inc, id, a, m, ack, nil)
			if !result.Returned || result.Wire != "fresh" || result.Proof == nil || result.Proof.Evidence.Kind != canonicalEvidenceRead || result.Proof.Evidence.ReadTransport != canonicalTransportOrd {
				t.Fatalf("ambiguous ACK needs actual original retrieval: %+v", result)
			}
			read, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, a)
			if err != nil {
				t.Fatal(err)
			}
			if read.Transport != canonicalTransportOrd {
				t.Fatal("collision must use ordinary without probing direct")
			}
			for key, values := range tc.extra {
				want := values
				if tc.name == "wire-trimming" {
					want = []string{"leading"}
				}
				if !reflect.DeepEqual(read.Message.Header[key], want) {
					t.Fatalf("stored original %s=%q want=%q", key, read.Message.Header[key], want)
				}
			}
			core, err := canonicalCoreFromRead(read, nil, id)
			if err != nil {
				t.Fatal(err)
			}
			fp, _ := canonicalCoreFingerprint(core)
			if fp != result.Proof.Fingerprint {
				t.Fatal("ordinary/core disagreement")
			}
			if !bytes.Equal(read.Message.Data, m.Data) || !reflect.DeepEqual(read.Message.Header["X-Signature"], m.Header["X-Signature"]) {
				t.Fatal("raw body/signature altered")
			}
			ctmSave(t, "fallback", map[string]any{"submitted": m, "ack": ack, "result": result, "ordinary": read})
			ctmUnchanged(t, b, before)
		})
	}
	// Unconstrained historical headers are ordinary even when the envelope-looking
	// originals happen to contain plausible values. Nothing is stripped.
	body := "legacy raw\x00bytes\n"
	id := ctmID(body, "legacy")
	h := nats.Header{jetstream.MsgIDHeader: {id.FullID}, jetstream.StreamHeader: {"FHIR"}, "Wsi-Evn": {"old", "old-again"}, "Nats-Crc-Provenance": {"one", "two"}, "x-Key": {"a"}, "X-Key": {"B"}}
	m := &nats.Msg{Subject: id.Subject, Header: h, Data: []byte(body)}
	ack, err := b.js.PublishMsg(b.ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	read, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read.Transport != canonicalTransportOrd || !reflect.DeepEqual(read.Message.Header, h) || !bytes.Equal(read.Message.Data, m.Data) {
		t.Fatal("legacy originals changed")
	}
	ctmSave(t, "legacy", map[string]any{"ack": ack, "original": read})
	_, direct, p := ctmRequests(t, b)
	if direct != 0 || p != len(cases)+1 {
		t.Fatalf("lookup issued direct/publication request: direct=%d publish=%d", direct, p)
	}
}

func TestCanonicalTransportMatrixAckDispositionAndReplay(t *testing.T) {
	b := ctmStart(t, false)
	c := &SubConsumer{js: b.js}
	body := "original signed raw\n"
	id := ctmID(body, "old")
	original := &nats.Msg{Subject: id.Subject, Header: cfkFrozen(id), Data: []byte(body)}
	oldAck, err := b.js.PublishMsg(b.ctx, original)
	if err != nil || oldAck == nil || oldAck.Duplicate {
		t.Fatal("real original required", err)
	}
	ctmSave(t, "original", map[string]any{"submitted": original, "ack": oldAck})
	for _, different := range []bool{false, true} {
		t.Run(fmt.Sprint("different-body-", different), func(t *testing.T) {
			retryBody := body
			if different {
				retryBody = "DIFFERENT current pending bytes"
			}
			retryID := ctmID(retryBody, "old")
			retry := &nats.Msg{Subject: retryID.Subject, Header: cfkFrozen(retryID), Data: []byte(retryBody)}
			retry.Header["Wsi-Evn"] = []string{"999"}
			retry.Header["Wsi-Ots"] = []string{"new-occurrence"}
			ack, err := b.js.PublishMsg(b.ctx, retry)
			if err != nil || ack == nil || !ack.Duplicate || ack.Sequence != oldAck.Sequence {
				t.Fatalf("real duplicate required: %+v %v", ack, err)
			}
			before := ctmInfo(t, b)
			result := c.resolveCanonicalAck(b.ctx, b.inc, retryID, nil, retry, ack, nil)
			if !result.Returned || result.Wire != "duplicate" || result.ReturnKind != "duplicate" || result.ObservedAck == nil || !result.ObservedAck.Duplicate {
				t.Fatalf("lost real duplicate disposition %+v", result)
			}
			if different {
				if result.Proof != nil || result.Blocked == "" {
					t.Fatal("different incoming body is not original proof")
				}
			} else {
				if result.Proof == nil || result.Proof.Evidence.ObservedAck != nil || result.Proof.Core.OriginalWsi["Wsi-Evn"][0] != "1" {
					t.Fatalf("replay restamped original Wsi/ACK: %+v", result)
				}
			}
			ctmSave(t, "real-duplicate", map[string]any{"submitted": retry, "ack": ack, "result": result})
			ctmUnchanged(t, b, before)
		})
	}
	recovered := c.recoverCanonicalRead(b.ctx, b.inc, oldAck.Sequence, id, nil)
	if !recovered.Returned || recovered.ReturnKind != "replay" || recovered.Wire != "not-sent" || recovered.ObservedAck != nil || recovered.Proof == nil || recovered.Proof.Evidence.ObservedAck != nil {
		t.Fatalf("positive retrieval must not invent ACK: %+v", recovered)
	}
	// Real eligible fresh observation is the source for deliberately negative copies.
	freshBody := "fresh second"
	freshID := ctmID(freshBody, "fresh-second")
	a := ctmAttempt(t, b, freshID, nil)
	submitted, ack := ctmPublish(t, b, freshID, a, freshBody)
	negatives := []struct {
		name string
		ack  *jetstream.PubAck
		err  error
	}{{"nil", nil, nil}, {"error-with-real-ack", ack, errors.New("lost outcome")}}
	for _, name := range []string{"zero", "stream", "impossible", "overflow", "duplicate-zero"} {
		copy := *ack
		switch name {
		case "zero":
			copy.Sequence = 0
		case "stream":
			copy.Stream = "OTHER"
		case "impossible":
			copy.Sequence++
		case "overflow":
			copy.Sequence = math.MaxUint64
		case "duplicate-zero":
			copy.Duplicate = true
			copy.Sequence = 0
		}
		negatives = append(negatives, struct {
			name string
			ack  *jetstream.PubAck
			err  error
		}{name, &copy, nil})
	}
	before := ctmInfo(t, b)
	for _, tc := range negatives {
		t.Run(tc.name, func(t *testing.T) {
			r := c.resolveCanonicalAck(b.ctx, b.inc, freshID, a, submitted, tc.ack, tc.err)
			if r.Returned || r.Proof != nil || r.Wire != "unknown" || r.Blocked == "" {
				t.Fatalf("invalid ACK became success: %+v", r)
			}
			ctmSave(t, "negative-copy-of-real-ack", r)
		})
	}
	ctmUnchanged(t, b, before)
	t.Run("actual-fresh-return-proof-unavailable", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(b.ctx)
		cancel()
		r := c.resolveCanonicalAck(cancelled, b.inc, freshID, a, submitted, ack, nil)
		if !r.Returned || r.ReturnKind != canonicalReturnFresh || r.ObservedAck == nil || r.Proof != nil || r.Blocked == "" {
			t.Fatalf("real fresh disposition must survive unavailable proof: %+v", r)
		}
		ctmSave(t, "actual-fresh-proof-unavailable", r)
	})
	// Delete only this owned test original, then obtain another REAL duplicate ACK.
	if err := b.stream.DeleteMsg(b.ctx, oldAck.Sequence); err != nil {
		t.Fatal(err)
	}
	retry := &nats.Msg{Subject: id.Subject, Header: cfkFrozen(id), Data: []byte(body)}
	dup, err := b.js.PublishMsg(b.ctx, retry)
	if err != nil || dup == nil || !dup.Duplicate {
		t.Fatalf("real unavailable-original duplicate: %+v %v", dup, err)
	}
	before = ctmInfo(t, b)
	unavailable := c.resolveCanonicalAck(b.ctx, b.inc, id, nil, retry, dup, nil)
	if !unavailable.Returned || unavailable.ReturnKind != "duplicate" || unavailable.Proof != nil || unavailable.Blocked == "" {
		t.Fatalf("duplicate without original lost distinction: %+v", unavailable)
	}
	ctmSave(t, "recovery-and-unavailable", map[string]any{"recovered": recovered, "actual_unavailable_ack": dup, "unavailable": unavailable})
	ctmUnchanged(t, b, before)
	_, _, p := ctmRequests(t, b)
	if p != 5 {
		t.Fatalf("helper sent a publication: requests=%d want=5", p)
	}
}

// Response faults below mutate copies of ACTUAL broker responses, not receipt,
// prefix or positive authority. Every corrupted response is explicitly negative.
type ctmFaultJS struct {
	jetstream.JetStream
	mutate func(*jetstream.RawStreamMsg)
	info   func(*jetstream.StreamInfo)
	last   *jetstream.RawStreamMsg
}
type ctmFaultStream struct {
	jetstream.Stream
	owner *ctmFaultJS
}

func (j *ctmFaultJS) Stream(ctx context.Context, name string) (jetstream.Stream, error) {
	s, e := j.JetStream.Stream(ctx, name)
	return &ctmFaultStream{Stream: s, owner: j}, e
}
func (s *ctmFaultStream) GetMsg(ctx context.Context, q uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	m, e := s.Stream.GetMsg(ctx, q, opts...)
	if e != nil || m == nil {
		return m, e
	}
	copy := *m
	copy.Header = canonicalCopyHeader(m.Header)
	copy.Data = bytes.Clone(m.Data)
	if s.owner.mutate != nil {
		s.owner.mutate(&copy)
	}
	s.owner.last = &copy
	return &copy, nil
}
func (s *ctmFaultStream) Info(ctx context.Context, opts ...jetstream.StreamInfoOpt) (*jetstream.StreamInfo, error) {
	i, e := s.Stream.Info(ctx, opts...)
	if e != nil || i == nil {
		return i, e
	}
	copy := *i
	if s.owner.info != nil {
		s.owner.info(&copy)
	}
	return &copy, nil
}

func TestCanonicalTransportMatrixEnvelopeAndIncarnation(t *testing.T) {
	b := ctmStart(t, false)
	body := "strict raw bytes"
	id := ctmID(body, "strict")
	a := ctmAttempt(t, b, id, nil)
	_, ack := ctmPublish(t, b, id, a, body)
	actual, err := b.stream.GetMsg(b.ctx, ack.Sequence)
	if err != nil {
		t.Fatal(err)
	}
	ctmSave(t, "actual-direct-response-before-negative-copies", actual)
	cases := []struct {
		name   string
		mutate func(*jetstream.RawStreamMsg)
	}{
		{"envelope-stream", func(m *jetstream.RawStreamMsg) { m.Header[jetstream.StreamHeader] = []string{"OTHER"} }},
		{"envelope-q", func(m *jetstream.RawStreamMsg) { m.Header[jetstream.SequenceHeader] = []string{"999"} }},
		{"envelope-subject", func(m *jetstream.RawStreamMsg) { m.Header[jetstream.SubjectHeader] = []string{"wrong"} }},
		{"envelope-time", func(m *jetstream.RawStreamMsg) {
			m.Header[jetstream.TimeStampHeaer] = []string{m.Time.Add(time.Second).Format(time.RFC3339Nano)}
		}},
		{"missing-time", func(m *jetstream.RawStreamMsg) { delete(m.Header, jetstream.TimeStampHeaer) }},
		{"returned-q", func(m *jetstream.RawStreamMsg) { m.Sequence++ }},
		{"returned-subject", func(m *jetstream.RawStreamMsg) {
			m.Subject = "wrong"
			m.Header[jetstream.SubjectHeader] = []string{"wrong"}
		}},
		{"returned-body", func(m *jetstream.RawStreamMsg) { m.Data = []byte("different") }},
		{"returned-id", func(m *jetstream.RawStreamMsg) { m.Header[jetstream.MsgIDHeader] = []string{"other/full/id"} }},
		{"multiple-id", func(m *jetstream.RawStreamMsg) {
			m.Header[jetstream.MsgIDHeader] = append(m.Header[jetstream.MsgIDHeader], id.FullID)
		}},
		{"zero-time", func(m *jetstream.RawStreamMsg) {
			m.Time = time.Time{}
			m.Header[jetstream.TimeStampHeaer] = []string{m.Time.Format(time.RFC3339Nano)}
		}},
	}
	for _, name := range canonicalEnvelopeNames() {
		key := name
		cases = append(cases, struct {
			name   string
			mutate func(*jetstream.RawStreamMsg)
		}{"multiple-" + key, func(m *jetstream.RawStreamMsg) { m.Header[key] = append(m.Header[key], m.Header[key][0]) }})
	}
	before := ctmInfo(t, b)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fault := &ctmFaultJS{JetStream: b.js, mutate: tc.mutate}
			c := &SubConsumer{js: fault}
			r, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, a)
			if err == nil || r.Message != nil {
				t.Fatalf("negative response accepted: %+v %v", r, err)
			}
			ctmSave(t, "deliberately-negative-response-copy", map[string]any{"negative_response": fault.last, "error": err.Error()})
		})
	}
	// Wrong expected full ID and subject must fail through actual ordinary reads too.
	for _, field := range []string{"id", "subject", "body"} {
		wrong := id
		switch field {
		case "id":
			wrong.FullID += "wrong"
		case "subject":
			wrong.Subject += "wrong"
		case "body":
			wrong.BodySHA256 = cfkHex("wrong")
		}
		if _, err := (&SubConsumer{js: b.js}).readCanonical(b.ctx, b.inc, ack.Sequence, wrong, nil); err == nil {
			t.Fatal("ordinary identity mismatch accepted", field)
		}
	}
	changed := b.inc
	changed.Created = changed.Created.Add(time.Second)
	if _, err := (&SubConsumer{js: b.js}).readCanonical(b.ctx, changed, ack.Sequence, id, a); err == nil {
		t.Fatal("wrong protected Created accepted")
	}
	infos := 0
	fault := &ctmFaultJS{JetStream: b.js, info: func(i *jetstream.StreamInfo) {
		infos++
		if infos >= 2 {
			i.Created = i.Created.Add(time.Second)
		}
	}}
	if _, err := (&SubConsumer{js: fault}).readCanonical(b.ctx, b.inc, ack.Sequence, id, a); err == nil {
		t.Fatal("observed mid-read Created change accepted")
	}
	ctmUnchanged(t, b, before)
	ctmRequests(t, b)
}

func TestCanonicalTransportMatrixConfiguredPrefixes(t *testing.T) {
	b := ctmStart(t, true)
	body := "prefix original signed bytes"
	id := ctmID(body, "prefix")
	a := ctmAttempt(t, b, id, nil)
	_, ack := ctmPublish(t, b, id, a, body)
	domain, err := jetstream.NewWithDomain(b.nc, "CRC")
	if err != nil {
		t.Fatal(err)
	}
	custom, err := jetstream.NewWithAPIPrefix(b.nc, "CRC.CUSTOM.API")
	if err != nil {
		t.Fatal(err)
	}
	before := ctmInfo(t, b)
	var fingerprint string
	for _, tc := range []struct {
		name   string
		js     jetstream.JetStream
		prefix string
	}{{"default", b.js, "$JS.API."}, {"domain", domain, "$JS.CRC.API."}, {"custom", custom, "CRC.CUSTOM.API."}} {
		t.Run(tc.name, func(t *testing.T) {
			if canonicalAPIPrefix(tc.js.Options()) != tc.prefix {
				t.Fatal("wrong options prefix")
			}
			c := &SubConsumer{js: tc.js}
			read, err := c.readCanonical(b.ctx, b.inc, ack.Sequence, id, nil)
			if err != nil {
				t.Fatal(err)
			}
			core, err := canonicalCoreFromRead(read, nil, id)
			if err != nil {
				t.Fatal(err)
			}
			fp, _ := canonicalCoreFingerprint(core)
			if fingerprint != "" && fp != fingerprint {
				t.Fatal("prefix changed original core")
			}
			fingerprint = fp
			if read.Transport != canonicalTransportOrd || !bytes.Equal(read.Message.Data, []byte(body)) {
				t.Fatal("not real ordinary original")
			}
			ctmSave(t, "actual-prefix-read", map[string]any{"options": tc.js.Options(), "ordinary_request_subject": tc.prefix + "STREAM.MSG.GET.FHIR", "read": read, "core": core})
		})
	}
	ctmUnchanged(t, b, before)
	ordinary, direct, p := ctmRequests(t, b)
	if ordinary != 3 || direct != 0 || p != 1 {
		t.Fatalf("ordinary/direct/publish=%d/%d/%d", ordinary, direct, p)
	}
}
