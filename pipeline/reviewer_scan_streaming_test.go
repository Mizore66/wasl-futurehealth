package main

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Recovery must scan the whole retained signoff prefix using one streaming
// consumer, not reset/delete/create a server consumer for every stored message.
// The first message is the match; later retained messages must still be visited.
func TestReviewerRecoveryScansRetainedSignoffsWithoutPerMessageConsumerChurn(t *testing.T) {
	env := startTestNATS(t)
	var first reviewerRecord
	var firstSequence uint64
	for i := 0; i < 32; i++ {
		rec := preparedReviewerRecord("rv-streaming-"+strconv.Itoa(i), uint64(i+1))
		sequence, err := publishReviewerSignoff(env.js, rec)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first, firstSequence = rec, sequence
		}
	}
	var creates, deletes int64
	js, err := jetstream.New(env.nc, jetstream.WithClientTrace(&jetstream.ClientTrace{RequestSent: func(subject string, _ []byte) {
		if strings.Contains(subject, ".CONSUMER.CREATE.FHIR.") {
			atomic.AddInt64(&creates, 1)
		}
		if strings.Contains(subject, ".CONSUMER.DELETE.FHIR.") {
			atomic.AddInt64(&deletes, 1)
		}
	}}))
	if err != nil {
		t.Fatal(err)
	}
	sequence, found, err := findReviewerSignoff(js, first.ItemID, first.BodyHash)
	if err != nil || !found || sequence != firstSequence {
		t.Fatalf("retained proof: seq=%d found=%v err=%v", sequence, found, err)
	}
	if got := atomic.LoadInt64(&creates); got != 1 {
		t.Fatalf("one retained scan recreated %d server consumers, want 1", got)
	}
	if got := atomic.LoadInt64(&deletes); got > 1 {
		t.Fatalf("one retained scan deleted %d consumers, want at most owned cleanup", got)
	}
}

func TestReviewerRecoveryStreamingRejectsLaterConflictingSignoff(t *testing.T) {
	env := startTestNATS(t)
	stream, err := env.js.Stream(env.ctx, "FHIR")
	if err != nil {
		t.Fatal(err)
	}
	cfg := stream.CachedInfo().Config
	cfg.Duplicates = dedupeTestWindow
	if _, err := env.js.UpdateStream(env.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	rec := preparedReviewerRecord("rv-streaming-late-conflict", 1)
	first, err := publishReviewerSignoff(env.js, rec)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(dedupeTestWindow + 150*time.Millisecond)
	// Same semantic JSON but different actual source bytes must still conflict.
	msg := &nats.Msg{Subject: "fhir.review", Data: append(signoffResource(rec.Signoff), '\n'), Header: nats.Header{jetstream.MsgIDHeader: []string{"signoff/" + rec.ItemID}}}
	ack, err := env.js.PublishMsg(env.ctx, msg)
	if err != nil || ack == nil || ack.Duplicate || ack.Sequence <= first {
		t.Fatalf("late conflict was not actually retained: ack=%+v err=%v", ack, err)
	}
	if _, _, err := findReviewerSignoff(env.js, rec.ItemID, rec.BodyHash); err == nil {
		t.Fatal("scan returned early at matching first signoff and missed later raw-body conflict")
	}
}
