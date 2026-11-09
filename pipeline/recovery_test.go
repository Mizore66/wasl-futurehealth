package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// A restart changes broker metadata, not the logical retained messages. Including that metadata would falsely
// report a different recovered run.
func TestNATSRecoveryBundleValidatesEveryStreamBeforeRestore(t *testing.T) {
	good := recoveryMessage{Subject: "out.p0", Data: []byte("one")}
	digest, err := recoveryLogicalDigest([]recoveryMessage{good})
	if err != nil {
		t.Fatal(err)
	}
	bundle := recoveryNATSBundle{Schema: "wasl-nats-backup-v1", Streams: []recoveryStream{
		{Config: jetstream.StreamConfig{Name: "GOOD"}, Messages: []recoveryMessage{good}, Digest: digest},
		{Config: jetstream.StreamConfig{Name: "BAD"}, Messages: []recoveryMessage{good}, Digest: digest},
	}}
	bundle.Streams[1].Messages[0].Data = []byte("tampered")
	if err := validateNATSRecoveryBundle(bundle); err == nil || !strings.Contains(err.Error(), "BAD") {
		t.Fatalf("later stream corruption was not identified before restore: %v", err)
	}
	bundle.Schema = "unsupported"
	if err := validateNATSRecoveryBundle(bundle); err == nil {
		t.Fatal("unsupported bundle schema was accepted")
	}
}

func TestStructuredLogEmitsOneJSONRecord(t *testing.T) {
	var output bytes.Buffer
	writeStructuredLogTo(&output, "info", "recovery complete", "streams", 2)
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("log is not one JSON record: %q: %v", output.String(), err)
	}
	if record["msg"] != "recovery complete" || record["level"] != "INFO" || record["streams"] != float64(2) {
		t.Fatalf("unexpected structured record: %v", record)
	}
	if bytes.Count(output.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("duplicate or multiline log output: %q", output.String())
	}
}

func TestNATSBackupRestoreReadsRetainedMessagesOnEmptyCluster(t *testing.T) {
	urls := strings.Split(os.Getenv("WASL_TEST_RECOVERY_NATS"), ",")
	if len(urls) != 2 {
		t.Skip("set WASL_TEST_RECOVERY_NATS=sourceURL,emptyTargetURL for the real JetStream drill")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connect := func(raw string) (*nats.Conn, jetstream.JetStream) {
		nc, err := nats.Connect(raw)
		if err != nil {
			t.Fatal(err)
		}
		js, err := jetstream.New(nc)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nc.Close() })
		return nc, js
	}
	_, source := connect(urls[0])
	_, err := source.CreateStream(ctx, jetstream.StreamConfig{Name: "RECOVERY_TEST", Subjects: []string{"recovery.>"}, Storage: jetstream.FileStorage, AllowDirect: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		msg := nats.NewMsg("recovery." + id)
		msg.Header.Set("Nats-Msg-Id", id)
		msg.Data = []byte(`{"retained":"` + id + `"}`)
		if _, err := source.PublishMsg(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}
	kv, err := source.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "RECOVERY_TEST_KV"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(ctx, "checkpoint", []byte("sequence-2")); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(t.TempDir(), "nats.json")
	before, err := backupNATS(ctx, source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if before.Streams != 2 || before.Messages != 3 {
		t.Fatalf("backup retained %d streams/%d messages, want 2/3", before.Streams, before.Messages)
	}
	_, target := connect(urls[1])
	after, err := restoreNATS(ctx, target, bundle)
	if err != nil {
		infos, listErr := listRecoveryStreams(ctx, target)
		if listErr == nil {
			for _, info := range infos {
				stream, _ := target.Stream(ctx, info.Config.Name)
				messages, _ := readRecoveryStream(ctx, stream)
				for _, message := range messages {
					t.Logf("restored %s subject=%s header=%v data=%q", info.Config.Name, message.Subject, message.Header, message.Data)
				}
			}
		}
		t.Fatal(err)
	}
	for name, digest := range before.Digests {
		if after.Digests[name] != digest {
			t.Errorf("restored stream %s logical digest differs", name)
		}
	}
	verified, err := verifyNATS(ctx, target, bundle)
	if err != nil {
		t.Fatalf("independent post-restart readback failed: %v", err)
	}
	if verified.Streams != before.Streams || verified.Messages != before.Messages {
		t.Fatalf("verification evidence = %d streams/%d messages, want %d/%d", verified.Streams, verified.Messages, before.Streams, before.Messages)
	}
	restoredKV, err := target.KeyValue(ctx, "RECOVERY_TEST_KV")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := restoredKV.Get(ctx, "checkpoint")
	if err != nil || string(entry.Value()) != "sequence-2" {
		t.Fatalf("restored checkpoint: %q, %v", entry.Value(), err)
	}
	if _, err := restoreNATS(ctx, target, bundle); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("second restore must refuse nonempty JetStream, got %v", err)
	}
}

func TestNATSBackupRestorePreservesRepeatedMessageIDs(t *testing.T) {
	urls := strings.Split(os.Getenv("WASL_TEST_RECOVERY_NATS"), ",")
	if len(urls) != 2 {
		t.Skip("set WASL_TEST_RECOVERY_NATS=sourceURL,emptyTargetURL for the real JetStream drill")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	connect := func(raw string) (*nats.Conn, jetstream.JetStream) {
		nc, err := nats.Connect(raw)
		if err != nil {
			t.Fatal(err)
		}
		js, err := jetstream.New(nc)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { nc.Close() })
		return nc, js
	}
	_, source := connect(urls[0])
	_, err := source.CreateStream(ctx, jetstream.StreamConfig{
		Name: "RECOVERY_DUPLICATES", Subjects: []string{"recovery.duplicate"},
		Storage: jetstream.FileStorage, Duplicates: time.Second, AllowDirect: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, payload := range []string{"first", "second"} {
		if index > 0 {
			time.Sleep(1100 * time.Millisecond) // retain both source messages despite their shared deduplication ID
		}
		msg := nats.NewMsg("recovery.duplicate")
		msg.Header.Set("Nats-Msg-Id", "shared-id")
		msg.Data = []byte(payload)
		ack, err := source.PublishMsg(ctx, msg)
		if err != nil {
			t.Fatal(err)
		}
		if ack.Duplicate {
			t.Fatal("source test setup unexpectedly suppressed a retained message")
		}
	}
	bundle := filepath.Join(t.TempDir(), "nats.json")
	before, err := backupNATS(ctx, source, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if before.Streams != 1 || before.Messages != 2 {
		t.Fatalf("backup retained %d streams/%d messages, want 1/2", before.Streams, before.Messages)
	}
	_, target := connect(urls[1])
	after, err := restoreNATS(ctx, target, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if after.Streams != 1 || after.Messages != 2 {
		t.Fatalf("restore retained %d streams/%d messages, want 1/2", after.Streams, after.Messages)
	}
	stream, err := target.Stream(ctx, "RECOVERY_DUPLICATES")
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.Duplicates != time.Second {
		t.Fatalf("restored duplicate window = %s, want original 1s", info.Config.Duplicates)
	}
	verified, err := verifyNATS(ctx, target, bundle)
	if err != nil {
		t.Fatalf("independent repeated-ID readback failed: %v", err)
	}
	if verified.Digests["RECOVERY_DUPLICATES"] != before.Digests["RECOVERY_DUPLICATES"] {
		t.Fatal("restored repeated-ID stream logical digest differs from backup")
	}
}

func TestRecoveryLogicalDigestExcludesPhysicalMetadata(t *testing.T) {
	a := []recoveryMessage{{Sequence: 8, Time: time.Unix(10, 0), Subject: "fhir.a.res", Header: map[string][]string{"Nats-Msg-Id": {"a/1"}}, Data: []byte(`{"id":"a"}`)}}
	b := []recoveryMessage{{Sequence: 91, Time: time.Unix(9999, 0), Subject: "fhir.a.res", Header: map[string][]string{"Nats-Msg-Id": {"a/1"}}, Data: []byte(`{"id":"a"}`)}}
	da, err := recoveryLogicalDigest(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := recoveryLogicalDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatalf("physical metadata changed logical digest: %s != %s", da, db)
	}
	// Independently derive the one-message digest over stable wire fields.
	h := sha256.New()
	h.Write([]byte("wasl-recovery-v1\x00fhir.a.res\x00Nats-Msg-Id\x00a/1\x00\x00{\"id\":\"a\"}"))
	want := hex.EncodeToString(h.Sum(nil))
	if da != want {
		t.Fatalf("logical digest = %s, want stable wire digest %s", da, want)
	}
	b[0].Data = []byte(`{"id":"tampered"}`)
	db, _ = recoveryLogicalDigest(b)
	if da == db {
		t.Fatal("message payload tampering did not change logical digest")
	}
}

func TestNATSFileSecretAndTLSOptions(t *testing.T) {
	dir := t.TempDir()
	password := filepath.Join(dir, "password")
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(password, []byte("correct horse\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, []byte("test ca"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASL_NATS_USER", "pipeline")
	t.Setenv("WASL_NATS_PASSWORD_FILE", password)
	t.Setenv("WASL_NATS_CA_FILE", ca)
	opts, err := natsSecurityOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts) != 3 { // UserInfo, RootCAs, Secure
		t.Fatalf("got %d NATS security options, want user/password and TLS", len(opts))
	}
}

func TestNATSClientCertificateFilesAreRequiredTogether(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"password", "ca", "cert", "key"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WASL_NATS_USER", "pipeline")
	t.Setenv("WASL_NATS_PASSWORD_FILE", filepath.Join(dir, "password"))
	t.Setenv("WASL_NATS_CA_FILE", filepath.Join(dir, "ca"))
	t.Setenv("WASL_NATS_CERT_FILE", filepath.Join(dir, "cert"))
	t.Setenv("WASL_NATS_KEY_FILE", filepath.Join(dir, "key"))
	opts, err := natsSecurityOptions()
	if err != nil || len(opts) != 4 {
		t.Fatalf("mutual TLS options = %d, %v; want 4", len(opts), err)
	}
	t.Setenv("WASL_NATS_KEY_FILE", "")
	if _, err := natsSecurityOptions(); err == nil {
		t.Fatal("client certificate without its key was accepted")
	}
}

func TestNATSMissingSecretFailsBeforeConnection(t *testing.T) {
	t.Setenv("WASL_NATS_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := natsSecurityOptions(); err == nil {
		t.Fatal("missing password file was accepted")
	}
}

func TestAPIAuthenticationDerivesRoleAndActor(t *testing.T) {
	users := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(users, []byte(`{"dr.sara":{"password":"secret","role":"reviewer"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASL_API_USERS_FILE", users)
	auth, err := apiAuthFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	var role, actor string
	h := auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, actor = r.Header.Get("X-Wasl-Role"), r.Header.Get("X-Wasl-Actor")
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/cases/x/review", nil)
	req.Header.Set("X-Wasl-Role", "coordinator") // forged caller assertions must be replaced
	req.Header.Set("X-Wasl-Actor", "mallory")
	req.SetBasicAuth("dr.sara", "secret")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent || role != RoleReviewer || actor != "dr.sara" {
		t.Fatalf("authenticated identity: code=%d role=%q actor=%q", resp.Code, role, actor)
	}
}

func TestAPIAuthenticationRejectsMissingAndWrongCredentials(t *testing.T) {
	users := filepath.Join(t.TempDir(), "users.json")
	if err := os.WriteFile(users, []byte(`{"dr.sara":{"password":"secret","role":"reviewer"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASL_API_USERS_FILE", users)
	auth, err := apiAuthFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	h := auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("unauthenticated request reached handler") }))
	for _, password := range []string{"", "wrong"} {
		req := httptest.NewRequest(http.MethodGet, "/api/cases", nil)
		if password != "" {
			req.SetBasicAuth("dr.sara", password)
		}
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("password %q: got %d, want 401", password, resp.Code)
		}
	}
}
