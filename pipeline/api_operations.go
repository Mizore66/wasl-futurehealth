package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	operationsSource         = "nats_jetstream_consumer_info"
	maxBackupManifestBytes   = 64 << 10
	operationsNATSTimeout    = 750 * time.Millisecond
	operationsProbeTimeout   = 3 * time.Second
	operationsBackupV1       = "wasl-file-backup-v1"
	operationsBackupV2AESGCM = "wasl-file-backup-v2-aesgcm"
)

type operationsAPI struct {
	Status       string            `json:"status"`
	Source       string            `json:"source"`
	Dependencies map[string]string `json:"dependencies"`
}

type operationsNATS struct {
	Status string `json:"status"`
	Source string `json:"source"`
	Reason string `json:"reason,omitempty"`
}

type operationsConsumer struct {
	Stream        string  `json:"stream"`
	Consumer      string  `json:"consumer"`
	Status        string  `json:"status"`
	NumPending    *uint64 `json:"num_pending,omitempty"`
	NumAckPending *uint64 `json:"num_ack_pending,omitempty"`
	Reason        string  `json:"reason,omitempty"`
}

type operationsQueue struct {
	Status    string               `json:"status"`
	Source    string               `json:"source"`
	Coverage  string               `json:"coverage"`
	Reason    string               `json:"reason,omitempty"`
	Consumers []operationsConsumer `json:"consumers"`
}

type operationsLatency struct {
	N   float64 `json:"n"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

type operationsRecovery struct {
	GeneratedResources uint64 `json:"generated_resources"`
	FHIRMsgs           uint64 `json:"fhir_msgs"`
	ExpectedOutputs    uint64 `json:"expected_outputs"`
	LiveOutputs        uint64 `json:"live_outputs"`
	IngestLost         uint64 `json:"ingest_lost"`
	IngestDuplicates   uint64 `json:"ingest_duplicates"`
	LostOutputs        uint64 `json:"lost_outputs"`
	DuplicateOutputs   uint64 `json:"duplicate_outputs"`
}

type operationsRun struct {
	Status    string              `json:"status"`
	Scope     string              `json:"scope"`
	Source    string              `json:"source"`
	Reason    string              `json:"reason,omitempty"`
	LatencyMS *operationsLatency  `json:"latency_ms,omitempty"`
	Recovery  *operationsRecovery `json:"recovery,omitempty"`
}

type operationsUnmeasured struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type operationsAPIMeasurements struct {
	Latency   operationsUnmeasured `json:"latency"`
	ErrorRate operationsUnmeasured `json:"error_rate"`
}

type operationsBackup struct {
	Status             string `json:"status"`
	Source             string `json:"source"`
	BundleSchema       string `json:"bundle_schema,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	AgeSeconds         *int64 `json:"age_seconds,omitempty"`
	VerificationStatus string `json:"verification_status,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type operationsResponse struct {
	Schema          string                    `json:"schema"`
	Meta            map[string]interface{}    `json:"meta"`
	ObservedAt      string                    `json:"observed_at"`
	API             operationsAPI             `json:"api"`
	NATS            operationsNATS            `json:"nats"`
	Queue           operationsQueue           `json:"queue"`
	Run             operationsRun             `json:"run"`
	APIMeasurements operationsAPIMeasurements `json:"api_measurements"`
	Backup          operationsBackup          `json:"backup"`
}

var (
	operationsNow        = time.Now
	operationsQueueProbe = probeOperationsQueue
)

func unavailableOperationsQueue(reason string) operationsQueue {
	return operationsQueue{Status: "unavailable", Source: operationsSource, Coverage: "known_durable_consumers", Reason: reason, Consumers: []operationsConsumer{}}
}

// probeOperationsQueue performs one bounded, authenticated JetStream observation. It deliberately does not use the
// pipeline connect helper, whose reconnect loop is appropriate for workers but not for a point-in-time API read.
func probeOperationsQueue(ctx context.Context) (operationsNATS, operationsQueue) {
	natsObservation := operationsNATS{Status: "unreachable", Source: operationsSource, Reason: "jetstream_probe_failed"}
	queue := unavailableOperationsQueue("nats_unreachable")
	security, err := natsSecurityOptions()
	if err != nil {
		return natsObservation, queue
	}
	security = append(security, nats.Timeout(operationsNATSTimeout), nats.NoReconnect(), nats.RetryOnFailedConnect(false))
	connection, err := nats.Connect(NatsURL, security...)
	if err != nil {
		return natsObservation, queue
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		return natsObservation, queue
	}

	natsObservation = operationsNATS{Status: "reachable", Source: operationsSource}
	queue = operationsQueue{Status: "available", Source: operationsSource, Coverage: "known_durable_consumers", Consumers: []operationsConsumer{}}
	hardFailures := 0
	targets := []struct{ stream, consumer string }{{"FHIRIN", "notifications"}, {"FHIR", "prep"}, {"OUT", "reviewer"}}
	for _, target := range targets {
		observation := operationsConsumer{Stream: target.stream, Consumer: target.consumer, Status: "available"}
		consumer, err := js.Consumer(ctx, target.stream, target.consumer)
		if err == nil {
			var info *jetstream.ConsumerInfo
			info, err = consumer.Info(ctx)
			if err == nil {
				if info.NumAckPending < 0 {
					err = errors.New("negative acknowledgement count")
				} else {
					pending, ackPending := info.NumPending, uint64(info.NumAckPending)
					observation.NumPending, observation.NumAckPending = &pending, &ackPending
				}
			}
		}
		if err != nil {
			observation.Status = "unavailable"
			observation.NumPending, observation.NumAckPending = nil, nil
			if errors.Is(err, jetstream.ErrConsumerNotFound) {
				observation.Reason = "consumer_not_found"
			} else {
				observation.Reason = "consumer_info_failed"
				if !errors.Is(err, jetstream.ErrStreamNotFound) {
					hardFailures++
				}
			}
		}
		queue.Consumers = append(queue.Consumers, observation)
	}
	if hardFailures == len(targets) {
		return operationsNATS{Status: "unreachable", Source: operationsSource, Reason: "jetstream_probe_failed"}, unavailableOperationsQueue("nats_unreachable")
	}
	available := 0
	for _, consumer := range queue.Consumers {
		if consumer.Status == "available" {
			available++
		}
	}
	switch available {
	case 0:
		queue.Status = "unavailable"
	case len(queue.Consumers):
		queue.Status = "available"
	default:
		queue.Status = "partial"
	}
	if queue.Status != "available" {
		queue.Reason = "consumer_info_unavailable"
	}
	return natsObservation, queue
}

func operationsNumber(value interface{}, integer bool) (float64, bool) {
	number, ok := value.(float64)
	if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > float64(math.MaxInt64) || (integer && math.Trunc(number) != number) {
		return 0, false
	}
	return number, true
}

func operationsRunFromReport(report map[string]interface{}) operationsRun {
	run := operationsRun{Status: "unmeasured", Scope: "completed_synthetic_run", Source: "report7.json", Reason: "run_metrics_not_recorded"}
	latency, latencyOK := report["latency_ms"].(map[string]interface{})
	recovery, recoveryOK := report["crash_recovery"].(map[string]interface{})
	if !latencyOK || !recoveryOK {
		return run
	}
	latencyValues := map[string]float64{}
	for _, key := range []string{"n", "p50", "p90", "p99", "max"} {
		value, ok := operationsNumber(latency[key], key == "n")
		if !ok {
			return run
		}
		latencyValues[key] = value
	}
	recoveryValues := map[string]uint64{}
	for _, key := range []string{"generated_resources", "fhir_msgs", "expected_outputs", "live_outputs", "ingest_lost", "ingest_duplicates", "lost_outputs", "duplicate_outputs"} {
		value, ok := operationsNumber(recovery[key], true)
		if !ok {
			return run
		}
		recoveryValues[key] = uint64(value)
	}
	run.Status, run.Reason = "available", ""
	run.LatencyMS = &operationsLatency{N: latencyValues["n"], P50: latencyValues["p50"], P90: latencyValues["p90"], P99: latencyValues["p99"], Max: latencyValues["max"]}
	run.Recovery = &operationsRecovery{
		GeneratedResources: recoveryValues["generated_resources"], FHIRMsgs: recoveryValues["fhir_msgs"],
		ExpectedOutputs: recoveryValues["expected_outputs"], LiveOutputs: recoveryValues["live_outputs"],
		IngestLost: recoveryValues["ingest_lost"], IngestDuplicates: recoveryValues["ingest_duplicates"],
		LostOutputs: recoveryValues["lost_outputs"], DuplicateOutputs: recoveryValues["duplicate_outputs"],
	}
	return run
}

type operationsBackupFile struct {
	Path             string `json:"path"`
	Size             *int64 `json:"size"`
	Mode             *int   `json:"mode"`
	SHA256           string `json:"sha256"`
	Nonce            string `json:"nonce"`
	CiphertextSHA256 string `json:"ciphertext_sha256"`
}

type operationsBackupDirectory struct {
	Path string `json:"path"`
	Mode *int   `json:"mode"`
}

type operationsBackupManifestV2 struct {
	Schema                string                      `json:"schema"`
	CreatedAt             string                      `json:"created_at"`
	KeyID                 string                      `json:"key_id"`
	NondeterministicField []string                    `json:"nondeterministic_fields"`
	WrappedDEK            string                      `json:"wrapped_dek"`
	WrapNonce             string                      `json:"wrap_nonce"`
	Dirs                  []operationsBackupDirectory `json:"dirs"`
	Files                 []operationsBackupFile      `json:"files"`
	HMAC                  string                      `json:"hmac_sha256"`
}

type operationsBackupManifestV1 struct {
	Files []struct {
		Path   string `json:"path"`
		Size   *int64 `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
	HMAC string `json:"hmac_sha256"`
}

var (
	operationsBackupKeyIDPattern  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	operationsBackupSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func operationsBackupBase64(value string, size int) bool {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	return err == nil && len(decoded) == size && base64.StdEncoding.EncodeToString(decoded) == value
}

func operationsBackupPath(path string) bool {
	if path == "" || len(path) > 1024 || strings.Contains(path, `\`) {
		return false
	}
	for _, char := range path {
		if char < 32 {
			return false
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func operationsBackupPathSet(paths []string, ciphertext bool) bool {
	for _, suffix := range []string{"", ".aead"} {
		if suffix != "" && !ciphertext {
			continue
		}
		seen := map[string]bool{}
		for _, path := range paths {
			path += suffix
			if !operationsBackupPath(path) || seen[path] {
				return false
			}
			seen[path] = true
		}
		for path := range seen {
			parts := strings.Split(path, "/")
			for i := 1; i < len(parts); i++ {
				if seen[strings.Join(parts[:i], "/")] {
					return false
				}
			}
		}
	}
	return true
}

func operationsBackupDirsAndFilesValid(dirs, files []string) bool {
	known := map[string]bool{}
	for _, path := range dirs {
		if !operationsBackupPath(path) || known[path] {
			return false
		}
		known[path] = true
	}
	for _, path := range files {
		if known[path] {
			return false
		}
	}
	for _, path := range append(append([]string{}, dirs...), files...) {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			if !known[strings.Join(parts[:i], "/")] {
				return false
			}
		}
	}
	return true
}

func operationsBackupV2Valid(raw []byte, fields map[string]json.RawMessage) bool {
	required := map[string]bool{"schema": true, "key_id": true, "created_at": true, "nondeterministic_fields": true, "wrapped_dek": true, "wrap_nonce": true, "dirs": true, "files": true, "hmac_sha256": true}
	if len(fields) != len(required) {
		return false
	}
	for field := range fields {
		if !required[field] {
			return false
		}
	}
	var manifest operationsBackupManifestV2
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Files == nil || manifest.Dirs == nil {
		return false
	}
	if !operationsBackupKeyIDPattern.MatchString(manifest.KeyID) || !operationsBackupSHA256Pattern.MatchString(manifest.HMAC) ||
		!operationsBackupBase64(manifest.WrappedDEK, 48) || !operationsBackupBase64(manifest.WrapNonce, 12) {
		return false
	}
	wantNondeterministic := []string{"created_at", "file_mtime", "nats_sequence", "nats_time"}
	if len(manifest.NondeterministicField) != len(wantNondeterministic) {
		return false
	}
	for i := range wantNondeterministic {
		if manifest.NondeterministicField[i] != wantNondeterministic[i] {
			return false
		}
	}
	files, dirs, nonces := []string{}, []string{}, map[string]bool{}
	for _, record := range manifest.Files {
		if record.Size == nil || *record.Size < 0 || record.Mode == nil || *record.Mode < 0 || *record.Mode > 0o777 ||
			!operationsBackupSHA256Pattern.MatchString(record.SHA256) || !operationsBackupSHA256Pattern.MatchString(record.CiphertextSHA256) ||
			!operationsBackupBase64(record.Nonce, 12) || nonces[record.Nonce] {
			return false
		}
		nonces[record.Nonce] = true
		files = append(files, record.Path)
	}
	for _, directory := range manifest.Dirs {
		if directory.Mode == nil || *directory.Mode < 0 || *directory.Mode > 0o777 {
			return false
		}
		dirs = append(dirs, directory.Path)
	}
	return operationsBackupPathSet(files, true) && operationsBackupDirsAndFilesValid(dirs, files)
}

func operationsBackupV1Valid(raw []byte, fields map[string]json.RawMessage) bool {
	if _, ok := fields["files"]; !ok {
		return false
	}
	if _, ok := fields["hmac_sha256"]; !ok {
		return false
	}
	var manifest operationsBackupManifestV1
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Files == nil || !operationsBackupSHA256Pattern.MatchString(manifest.HMAC) {
		return false
	}
	paths := []string{}
	for _, record := range manifest.Files {
		if record.Size == nil || *record.Size < 0 || !operationsBackupSHA256Pattern.MatchString(record.SHA256) {
			return false
		}
		paths = append(paths, record.Path)
	}
	return operationsBackupPathSet(paths, false)
}

// parseOperationsBackupManifest validates the same required metadata shapes as recovery.py without authenticating the
// manifest or opening payload files. Only the schema and creation timestamp are returned for the public response.
func parseOperationsBackupManifest(raw []byte) (string, string, bool) {
	if err := rejectDuplicateKeys(raw); err != nil {
		return "", "", false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return "", "", false
	}
	var schema, createdAt string
	if err := json.Unmarshal(fields["schema"], &schema); err != nil {
		return "", "", false
	}
	if rawCreatedAt, ok := fields["created_at"]; ok {
		if err := json.Unmarshal(rawCreatedAt, &createdAt); err != nil {
			return "", "", false
		}
	}
	switch schema {
	case operationsBackupV2AESGCM:
		if !operationsBackupV2Valid(raw, fields) {
			return "", "", false
		}
	case operationsBackupV1:
		if !operationsBackupV1Valid(raw, fields) {
			return "", "", false
		}
	default:
		return "", "", false
	}
	return schema, createdAt, true
}

// operationsBackupFromManifest reads only configured manifest metadata. It never reads keys or payload files, and does
// not claim that the bundle or a restore was verified.
func operationsBackupFromManifest(path string, observedAt time.Time) operationsBackup {
	backup := operationsBackup{Status: "unmeasured", Source: "recovery_bundle_manifest", Reason: "backup_manifest_not_configured"}
	if path == "" {
		return backup
	}
	backup.Status, backup.Reason = "unavailable", "backup_manifest_unreadable"
	file, err := os.Open(path)
	if err != nil {
		return backup
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBackupManifestBytes+1))
	if err != nil {
		return backup
	}
	if len(raw) > maxBackupManifestBytes {
		backup.Reason = "backup_manifest_invalid"
		return backup
	}
	schema, createdAtRaw, ok := parseOperationsBackupManifest(raw)
	if !ok {
		backup.Reason = "backup_manifest_invalid"
		return backup
	}
	backup.BundleSchema = schema
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		backup.Reason = "backup_created_at_invalid"
		return backup
	}
	backup.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	if createdAt.After(observedAt) {
		backup.Reason = "backup_created_at_in_future"
		return backup
	}
	age := int64(observedAt.Sub(createdAt) / time.Second)
	backup.Status, backup.Reason = "available", ""
	backup.AgeSeconds = &age
	backup.VerificationStatus = "not_verified_by_api"
	return backup
}

func (s *apiServer) getOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONResp(w, http.StatusMethodNotAllowed, s.envelope(map[string]interface{}{"error": "method not allowed", "allow": http.MethodGet}))
		return
	}
	observedAt := operationsNow().UTC()
	ctx, cancel := context.WithTimeout(r.Context(), operationsProbeTimeout)
	defer cancel()
	natsObservation, queue := operationsQueueProbe(ctx)
	health := s.healthBody()
	dependencies, _ := health["dependencies"].(map[string]string)
	response := operationsResponse{
		Schema: "wasl-operations/1", Meta: s.meta, ObservedAt: observedAt.Format(time.RFC3339Nano),
		API:  operationsAPI{Status: health["status"].(string), Source: "/healthz", Dependencies: dependencies},
		NATS: natsObservation, Queue: queue, Run: operationsRunFromReport(s.report),
		APIMeasurements: operationsAPIMeasurements{
			Latency:   operationsUnmeasured{Status: "unmeasured", Reason: "api_request_latency_not_recorded"},
			ErrorRate: operationsUnmeasured{Status: "unmeasured", Reason: "api_request_outcomes_not_recorded"},
		},
		Backup: operationsBackupFromManifest(os.Getenv("WASL_BACKUP_MANIFEST"), observedAt),
	}
	writeJSONResp(w, http.StatusOK, response)
}
