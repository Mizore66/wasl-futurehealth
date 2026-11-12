package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func operationsTestServer(t *testing.T) *apiServer {
	t.Helper()
	s, _ := contractRun(t, []contractCase{{suffix: "operations", item: "rv-operations"}})
	s.report = map[string]interface{}{
		"latency_ms": map[string]interface{}{"n": float64(4), "p50": 12.5, "p90": 30.25, "p99": 48.75, "max": 50.0},
		"crash_recovery": map[string]interface{}{
			"generated_resources": float64(100), "fhir_msgs": float64(120), "expected_outputs": float64(80), "live_outputs": float64(79),
			"ingest_lost": float64(1), "ingest_duplicates": float64(2), "lost_outputs": float64(3), "duplicate_outputs": float64(4),
		},
	}
	t.Setenv("WASL_BACKUP_MANIFEST", "")
	return s
}

func availableOperationsProbe(t *testing.T) {
	t.Helper()
	old := operationsQueueProbe
	pending, ackPending := uint64(7), uint64(2)
	otherPending, otherAckPending := uint64(11), uint64(3)
	operationsQueueProbe = func(context.Context) (operationsNATS, operationsQueue) {
		return operationsNATS{Status: "reachable", Source: "nats_jetstream_consumer_info"}, operationsQueue{
			Status: "available", Source: "nats_jetstream_consumer_info", Coverage: "known_durable_consumers",
			Consumers: []operationsConsumer{
				{Stream: "FHIRIN", Consumer: "notifications", Status: "available", NumPending: &pending, NumAckPending: &otherAckPending},
				{Stream: "FHIR", Consumer: "prep", Status: "available", NumPending: &otherPending, NumAckPending: &ackPending},
			},
		}
	}
	t.Cleanup(func() { operationsQueueProbe = old })
}

func operationsRequest(t *testing.T, s *apiServer) (*httptest.ResponseRecorder, operationsResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/operations", nil)
	apiHandler(s).ServeHTTP(rec, req)
	var body operationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("operations response is not valid contract JSON: %v: %s", err, rec.Body.String())
	}
	return rec, body
}

func TestOperationsHealthyUsesObservedQueueRunAndBundleValues(t *testing.T) {
	s := operationsTestServer(t)
	availableOperationsProbe(t)
	oldNow := operationsNow
	operationsNow = func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { operationsNow = oldNow })
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	raw := `{"schema":"wasl-file-backup-v2-aesgcm","created_at":"2026-11-13T11:59:57Z","key_id":"secret-key-id","wrapped_dek":"secret-wrapped-key","files":[{"path":"/private/run"}]}`
	if err := os.WriteFile(manifest, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASL_BACKUP_MANIFEST", manifest)

	rec, body := operationsRequest(t, s)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET operations: %d %s", rec.Code, rec.Body.String())
	}
	if body.Schema != "wasl-operations/1" || body.ObservedAt != "2026-11-13T12:00:00Z" {
		t.Fatalf("observation identity = %q %q", body.Schema, body.ObservedAt)
	}
	if body.API.Status != "ok" || body.API.Source != "/healthz" || !reflect.DeepEqual(body.API.Dependencies, s.healthBody()["dependencies"]) {
		t.Fatalf("API health diverged from healthz: %+v", body.API)
	}
	if body.NATS.Status != "reachable" || body.Queue.Status != "available" || len(body.Queue.Consumers) != 2 {
		t.Fatalf("queue was not measured: %+v %+v", body.NATS, body.Queue)
	}
	if got := *body.Queue.Consumers[0].NumPending; got != 7 {
		t.Errorf("notifications num_pending = %d, want 7", got)
	}
	if got := *body.Queue.Consumers[1].NumPending; got != 11 {
		t.Errorf("prep num_pending = %d, want 11", got)
	}
	if body.Run.Status != "available" || body.Run.Scope != "completed_synthetic_run" || body.Run.Source != "report7.json" {
		t.Fatalf("run provenance = %+v", body.Run)
	}
	if body.Run.LatencyMS == nil || body.Run.LatencyMS.N != 4 || body.Run.LatencyMS.P50 != 12.5 || body.Run.LatencyMS.P90 != 30.25 || body.Run.LatencyMS.P99 != 48.75 || body.Run.LatencyMS.Max != 50 {
		t.Errorf("latency = %+v", body.Run.LatencyMS)
	}
	if body.Run.Recovery == nil || body.Run.Recovery.GeneratedResources != 100 || body.Run.Recovery.FHIRMsgs != 120 || body.Run.Recovery.ExpectedOutputs != 80 || body.Run.Recovery.LiveOutputs != 79 || body.Run.Recovery.IngestLost != 1 || body.Run.Recovery.IngestDuplicates != 2 || body.Run.Recovery.LostOutputs != 3 || body.Run.Recovery.DuplicateOutputs != 4 {
		t.Errorf("recovery = %+v", body.Run.Recovery)
	}
	if body.APIMeasurements.Latency.Status != "unmeasured" || body.APIMeasurements.Latency.Reason != "api_request_latency_not_recorded" || body.APIMeasurements.ErrorRate.Status != "unmeasured" || body.APIMeasurements.ErrorRate.Reason != "api_request_outcomes_not_recorded" {
		t.Errorf("current API measurements invented: %+v", body.APIMeasurements)
	}
	if body.Backup.Status != "available" || body.Backup.BundleSchema != "wasl-file-backup-v2-aesgcm" || body.Backup.CreatedAt != "2026-11-13T11:59:57Z" || body.Backup.AgeSeconds == nil || *body.Backup.AgeSeconds != 3 || body.Backup.VerificationStatus != "not_verified_by_api" {
		t.Errorf("backup = %+v", body.Backup)
	}
	for _, secret := range []string{"secret-key-id", "secret-wrapped-key", "/private/run", manifest} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("operations response leaked manifest value %q", secret)
		}
	}
}

func TestOperationsUnavailableNATSReturnsNoBacklogButKeepsAPIHealth(t *testing.T) {
	s := operationsTestServer(t)
	oldURL, oldProbe := NatsURL, operationsQueueProbe
	NatsURL = "nats://127.0.0.1:1"
	operationsQueueProbe = probeOperationsQueue
	t.Cleanup(func() { NatsURL, operationsQueueProbe = oldURL, oldProbe })

	rec, body := operationsRequest(t, s)
	if rec.Code != http.StatusOK || body.NATS.Status != "unreachable" || body.NATS.Reason != "jetstream_probe_failed" {
		t.Fatalf("NATS failure = %d %+v", rec.Code, body.NATS)
	}
	if body.Queue.Status != "unavailable" || body.Queue.Reason != "nats_unreachable" || body.Queue.Consumers == nil || len(body.Queue.Consumers) != 0 {
		t.Fatalf("failed queue observation invented backlog: %+v", body.Queue)
	}
	if body.API.Status != "ok" || body.API.Dependencies["materialised_run"] != "loaded" {
		t.Errorf("materialised API health lost during NATS failure: %+v", body.API)
	}
	if strings.Contains(rec.Body.String(), "127.0.0.1:1") || strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("internal NATS error leaked: %s", rec.Body.String())
	}
}

func TestOperationsPartialConsumerKeepsMeasuredCounts(t *testing.T) {
	s := operationsTestServer(t)
	old := operationsQueueProbe
	pending, ackPending := uint64(5), uint64(1)
	operationsQueueProbe = func(context.Context) (operationsNATS, operationsQueue) {
		return operationsNATS{Status: "reachable", Source: "nats_jetstream_consumer_info"}, operationsQueue{
			Status: "partial", Source: "nats_jetstream_consumer_info", Coverage: "known_durable_consumers", Reason: "consumer_info_unavailable",
			Consumers: []operationsConsumer{
				{Stream: "FHIRIN", Consumer: "notifications", Status: "available", NumPending: &pending, NumAckPending: &ackPending},
				{Stream: "FHIR", Consumer: "prep", Status: "unavailable", Reason: "consumer_not_found"},
			},
		}
	}
	t.Cleanup(func() { operationsQueueProbe = old })
	_, body := operationsRequest(t, s)
	if body.Queue.Status != "partial" || body.Queue.Consumers[0].NumPending == nil || *body.Queue.Consumers[0].NumPending != 5 {
		t.Fatalf("partial queue discarded measured consumer: %+v", body.Queue)
	}
	if body.Queue.Consumers[1].NumPending != nil || body.Queue.Consumers[1].NumAckPending != nil {
		t.Fatalf("missing consumer has synthetic zero counts: %+v", body.Queue.Consumers[1])
	}
}

func TestOperationsRealNATSMeasuresPreparationBacklogAndMissingConsumer(t *testing.T) {
	env := startTestNATS(t)
	s := operationsTestServer(t)
	oldProbe := operationsQueueProbe
	operationsQueueProbe = probeOperationsQueue
	t.Cleanup(func() { operationsQueueProbe = oldProbe })

	_, before := operationsRequest(t, s)
	if before.NATS.Status != "reachable" || before.Queue.Status != "available" || *before.Queue.Consumers[1].NumPending != 0 {
		t.Fatalf("initial real NATS observation: %+v %+v", before.NATS, before.Queue)
	}
	for i := 0; i < 2; i++ {
		if _, err := env.js.Publish(env.ctx, "fhir.synthetic", []byte(`{"synthetic":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	_, after := operationsRequest(t, s)
	if got := *after.Queue.Consumers[1].NumPending; got != 2 {
		t.Fatalf("FHIR/prep num_pending after publishing = %d, want 2", got)
	}
	if err := env.js.DeleteConsumer(env.ctx, "FHIR", "prep"); err != nil {
		t.Fatal(err)
	}
	_, partial := operationsRequest(t, s)
	if partial.Queue.Status != "partial" || partial.Queue.Consumers[0].Status != "available" || partial.Queue.Consumers[1].Reason != "consumer_not_found" {
		t.Fatalf("missing durable consumer: %+v", partial.Queue)
	}
	if _, err := env.js.CreateOrUpdateConsumer(env.ctx, "FHIR", jetstream.ConsumerConfig{Durable: "prep", AckPolicy: jetstream.AckExplicitPolicy}); err != nil {
		t.Fatal(err)
	}
	if err := env.js.DeleteStream(env.ctx, "FHIR"); err != nil {
		t.Fatal(err)
	}
	if err := env.js.DeleteStream(env.ctx, "FHIRIN"); err != nil {
		t.Fatal(err)
	}
	_, noStreams := operationsRequest(t, s)
	if noStreams.NATS.Status != "reachable" || noStreams.Queue.Status != "unavailable" || noStreams.Queue.Reason != "consumer_info_unavailable" {
		t.Fatalf("missing streams were misreported as a NATS outage: %+v %+v", noStreams.NATS, noStreams.Queue)
	}
}

func TestOperationsMissingRunMetricsAreUnmeasuredNotZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		report map[string]interface{}
	}{
		{name: "no report", report: nil},
		{name: "latency missing", report: map[string]interface{}{"crash_recovery": map[string]interface{}{}}},
		{name: "recovery missing", report: map[string]interface{}{"latency_ms": map[string]interface{}{"n": float64(1), "p50": float64(1), "p90": float64(1), "p99": float64(1), "max": float64(1)}}},
		{name: "negative recovery", report: map[string]interface{}{"latency_ms": map[string]interface{}{"n": float64(1), "p50": float64(1), "p90": float64(1), "p99": float64(1), "max": float64(1)}, "crash_recovery": map[string]interface{}{"generated_resources": float64(-1)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := operationsTestServer(t)
			s.report = tc.report
			_, body := operationsRequest(t, s)
			if body.Run.Status != "unmeasured" || body.Run.Reason != "run_metrics_not_recorded" || body.Run.LatencyMS != nil || body.Run.Recovery != nil {
				t.Fatalf("missing run fields became zero or partial metrics: %+v", body.Run)
			}
		})
	}
}

func TestOperationsBackupUnavailableAndUnmeasuredStates(t *testing.T) {
	for _, tc := range []struct {
		name, contents, status, reason string
		set                            bool
	}{
		{name: "unset", status: "unmeasured", reason: "backup_manifest_not_configured"},
		{name: "missing", set: true, status: "unavailable", reason: "backup_manifest_unreadable"},
		{name: "malformed", set: true, contents: `{`, status: "unavailable", reason: "backup_manifest_invalid"},
		{name: "oversized", set: true, contents: strings.Repeat(" ", maxBackupManifestBytes+1), status: "unavailable", reason: "backup_manifest_invalid"},
		{name: "schema", set: true, contents: `{"schema":"other","created_at":"2026-11-13T00:00:00Z"}`, status: "unavailable", reason: "backup_manifest_invalid"},
		{name: "timestamp", set: true, contents: `{"schema":"wasl-file-backup-v1","created_at":"2026-11-13"}`, status: "unavailable", reason: "backup_created_at_invalid"},
		{name: "future", set: true, contents: `{"schema":"wasl-file-backup-v1","created_at":"2026-11-13T12:00:01Z"}`, status: "unavailable", reason: "backup_created_at_in_future"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := operationsTestServer(t)
			path := filepath.Join(t.TempDir(), "manifest.json")
			if tc.set {
				t.Setenv("WASL_BACKUP_MANIFEST", path)
			}
			if tc.contents != "" {
				if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			oldNow := operationsNow
			operationsNow = func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }
			t.Cleanup(func() { operationsNow = oldNow })
			rec, body := operationsRequest(t, s)
			if body.Backup.Status != tc.status || body.Backup.Reason != tc.reason || body.Backup.AgeSeconds != nil {
				t.Fatalf("backup = %+v, want %s/%s without age", body.Backup, tc.status, tc.reason)
			}
			if strings.Contains(rec.Body.String(), path) {
				t.Fatal("backup path leaked in response")
			}
		})
	}
}

func TestOperationsOnlyGET(t *testing.T) {
	s := operationsTestServer(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/operations", nil)
		apiHandler(s).ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodGet {
			t.Errorf("%s operations = %d Allow=%q", method, rec.Code, rec.Header().Get("Allow"))
		}
	}
}
