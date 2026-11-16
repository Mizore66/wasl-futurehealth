package main

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestCanonicalFreshAckUsesOneInvocationLocalInfo(t *testing.T) {
	b := ctmStart(t, false)
	body := "fresh-ack-info-control"
	id := ctmID(body, "fresh-ack-info")
	attempt := ctmAttempt(t, b, id, nil)
	message, ack := ctmPublish(t, b, id, attempt, body)
	observer, err := b.nc.SubscribeSync("$JS.API.STREAM.INFO.FHIR")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	result := (&SubConsumer{js: b.js}).resolveCanonicalAck(b.ctx, b.inc, id, attempt, message, ack, nil)
	if !result.Returned || result.ReturnKind != canonicalReturnFresh || result.Proof == nil || result.Proof.Evidence.Kind != canonicalEvidenceFresh || result.Proof.Core.Sequence != ack.Sequence {
		t.Fatalf("actual fresh ACK lost its matching body proof: %+v", result)
	}
	if err := b.nc.Flush(); err != nil {
		t.Fatal(err)
	}
	requests, _, err := observer.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("fresh-ACK physical STREAM.INFO requests=%d, want one fresh post-ACK lookup response", requests)
	}
}

// Count real metadata RPCs, not wrapper method calls. The existing exact-q
// message response and post-read incarnation checks remain separate authorities.
func TestCanonicalOrdinaryReadUsesOneInvocationLocalInfo(t *testing.T) {
	env := startTestNATS(t)
	var sequences []uint64
	for _, body := range []string{"ordinary-read-control-one", "ordinary-read-control-two"} {
		ack, err := env.js.Publish(env.ctx, "fhir.info-test.ordinary", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		sequences = append(sequences, ack.Sequence)
	}
	observer, err := nats.Connect(NatsURL, nats.NoReconnect())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observer.Close)
	sub, err := observer.SubscribeSync("$JS.API.STREAM.INFO.FHIR")
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}
	consumer := &SubConsumer{js: env.js}
	for _, sequence := range sequences {
		read, err := consumer.readCanonicalOrdinary(env.ctx, sequence)
		if err != nil || read.Message == nil || read.Message.Sequence != sequence || read.RequestedSequence != sequence || read.Transport != canonicalTransportOrd {
			t.Fatalf("ordinary read lost its exact source: read=%+v err=%v", read, err)
		}
		if err := read.Incarnation.validate(); err != nil {
			t.Fatal(err)
		}
	}
	if err := observer.Flush(); err != nil {
		t.Fatal(err)
	}
	requests, _, err := sub.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if requests != len(sequences) {
		t.Fatalf("physical STREAM.INFO requests=%d, want %d fresh invocation-local responses", requests, len(sequences))
	}
}
