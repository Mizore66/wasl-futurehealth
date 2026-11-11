package main

// Recovery and externally authenticated transport helpers. The demo remains usable without these settings;
// the production Compose file requires all of them and therefore fails before opening a listener when one is absent.
import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type recoveryMessage struct {
	Sequence uint64              `json:"sequence"`
	Time     time.Time           `json:"time"`
	Subject  string              `json:"subject"`
	Header   map[string][]string `json:"header,omitempty"`
	Data     []byte              `json:"data"`
}

type recoveryStream struct {
	Config    jetstream.StreamConfig     `json:"config"`
	Consumers []jetstream.ConsumerConfig `json:"consumers,omitempty"`
	Messages  []recoveryMessage          `json:"messages"`
	Digest    string                     `json:"logical_sha256"`
}

type recoveryNATSBundle struct {
	Schema                 string           `json:"schema"`
	CreatedAt              time.Time        `json:"created_at"`
	NondeterministicFields []string         `json:"nondeterministic_fields"`
	Streams                []recoveryStream `json:"streams"`
}

type recoverySummary struct {
	Streams  int               `json:"streams"`
	Messages int               `json:"messages"`
	Digests  map[string]string `json:"logical_sha256_by_stream"`
}

func recoveryApplicationHeader(header nats.Header) map[string][]string {
	// Direct-get adds broker read metadata. Republishing it makes the server append a second value and changes the
	// logical header set; application headers such as Nats-Msg-Id and KV rollup markers must be retained.
	brokerOnly := map[string]bool{"Nats-Stream": true, "Nats-Subject": true, "Nats-Sequence": true,
		"Nats-Time-Stamp": true, "Nats-Num-Pending": true, "Nats-Last-Sequence": true, "Nats-Msg-Size": true,
		"Nats-Expected-Last-Subject-Sequence": true, "Nats-Expected-Last-Sequence": true,
		"Nats-Expected-Last-Msg-Id": true, "Nats-Expected-Stream": true}
	out := map[string][]string{}
	for key, values := range header {
		if !brokerOnly[key] {
			out[key] = append([]string(nil), values...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readRecoveryStream(ctx context.Context, stream jetstream.Stream) ([]recoveryMessage, error) {
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, err
	}
	messages := make([]recoveryMessage, 0, info.State.Msgs)
	if info.State.Msgs == 0 {
		return messages, nil
	}
	for sequence := info.State.FirstSeq; sequence <= info.State.LastSeq; sequence++ {
		raw, err := stream.GetMsg(ctx, sequence)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue // retained streams may contain deleted-sequence gaps
		}
		if err != nil {
			return nil, fmt.Errorf("read %s sequence %d: %w", info.Config.Name, sequence, err)
		}
		messages = append(messages, recoveryMessage{Sequence: raw.Sequence, Time: raw.Time, Subject: raw.Subject,
			Header: recoveryApplicationHeader(raw.Header), Data: raw.Data})
	}
	return messages, nil
}

func listRecoveryStreams(ctx context.Context, js jetstream.JetStream) ([]*jetstream.StreamInfo, error) {
	lister := js.ListStreams(ctx)
	var infos []*jetstream.StreamInfo
	for info := range lister.Info() {
		infos = append(infos, info)
	}
	if err := lister.Err(); err != nil {
		return nil, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Config.Name < infos[j].Config.Name })
	return infos, nil
}

func backupNATS(ctx context.Context, js jetstream.JetStream, path string) (recoverySummary, error) {
	infos, err := listRecoveryStreams(ctx, js)
	if err != nil {
		return recoverySummary{}, err
	}
	bundle := recoveryNATSBundle{Schema: "wasl-nats-backup-v1", CreatedAt: time.Now().UTC(),
		NondeterministicFields: []string{"created_at", "nats_sequence", "nats_time", "consumer_delivered", "consumer_ack_floor"},
		Streams:                []recoveryStream{}}
	summary := recoverySummary{Digests: map[string]string{}}
	for _, info := range infos {
		stream, err := js.Stream(ctx, info.Config.Name)
		if err != nil {
			return summary, err
		}
		messages, err := readRecoveryStream(ctx, stream)
		if err != nil {
			return summary, err
		}
		digest, err := recoveryLogicalDigest(messages)
		if err != nil {
			return summary, err
		}
		consumers := []jetstream.ConsumerConfig{}
		lister := stream.ListConsumers(ctx)
		for consumer := range lister.Info() {
			if consumer.Config.Durable != "" {
				consumers = append(consumers, consumer.Config)
			}
		}
		if err := lister.Err(); err != nil {
			return summary, err
		}
		sort.Slice(consumers, func(i, j int) bool { return consumers[i].Durable < consumers[j].Durable })
		bundle.Streams = append(bundle.Streams, recoveryStream{Config: info.Config, Consumers: consumers, Messages: messages, Digest: digest})
		summary.Streams++
		summary.Messages += len(messages)
		summary.Digests[info.Config.Name] = digest
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return summary, err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(encoded, '\n'), 0600); err != nil {
		return summary, err
	}
	if err := os.Rename(temporary, path); err != nil {
		return summary, err
	}
	return summary, nil
}

func validateNATSRecoveryBundle(bundle recoveryNATSBundle) error {
	if bundle.Schema != "wasl-nats-backup-v1" {
		return fmt.Errorf("unsupported NATS recovery bundle schema")
	}
	for _, saved := range bundle.Streams {
		digest, err := recoveryLogicalDigest(saved.Messages)
		if err != nil || digest != saved.Digest {
			return fmt.Errorf("NATS backup logical hash mismatch for %s", saved.Config.Name)
		}
	}
	return nil
}

func loadNATSRecoveryBundle(path string) (recoveryNATSBundle, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return recoveryNATSBundle{}, err
	}
	var bundle recoveryNATSBundle
	if err := json.Unmarshal(encoded, &bundle); err != nil {
		return recoveryNATSBundle{}, fmt.Errorf("invalid NATS recovery bundle")
	}
	if err := validateNATSRecoveryBundle(bundle); err != nil {
		return recoveryNATSBundle{}, err
	}
	return bundle, nil
}

// verifyNATS replays every retained message from the current cluster and compares stable logical hashes to the
// backup. It does not create, delete, publish, or acknowledge anything.
func verifyNATS(ctx context.Context, js jetstream.JetStream, path string) (recoverySummary, error) {
	summary := recoverySummary{Digests: map[string]string{}}
	bundle, err := loadNATSRecoveryBundle(path)
	if err != nil {
		return summary, err
	}
	infos, err := listRecoveryStreams(ctx, js)
	if err != nil {
		return summary, err
	}
	if len(infos) != len(bundle.Streams) {
		return summary, fmt.Errorf("retained stream count differs: backup=%d cluster=%d", len(bundle.Streams), len(infos))
	}
	for _, saved := range bundle.Streams {
		stream, err := js.Stream(ctx, saved.Config.Name)
		if err != nil {
			return summary, err
		}
		messages, err := readRecoveryStream(ctx, stream)
		if err != nil {
			return summary, err
		}
		digest, err := recoveryLogicalDigest(messages)
		if err != nil || digest != saved.Digest {
			return summary, fmt.Errorf("retained stream %s differs from backup logical hash", saved.Config.Name)
		}
		summary.Streams++
		summary.Messages += len(messages)
		summary.Digests[saved.Config.Name] = digest
	}
	return summary, nil
}

func restoreNATS(ctx context.Context, js jetstream.JetStream, path string) (recoverySummary, error) {
	bundle, err := loadNATSRecoveryBundle(path)
	if err != nil {
		return recoverySummary{}, err
	}
	infos, err := listRecoveryStreams(ctx, js)
	if err != nil {
		return recoverySummary{}, err
	}
	if len(infos) != 0 {
		return recoverySummary{}, fmt.Errorf("restore target JetStream is not empty: %d streams already exist", len(infos))
	}
	summary := recoverySummary{Digests: map[string]string{}}
	for _, saved := range bundle.Streams {
		const restoreDuplicateWindow = 100 * time.Millisecond // NATS rejects shorter explicit windows.
		config := saved.Config
		seenIDs := map[string]bool{}
		repeatedIDs := false
		for _, message := range saved.Messages {
			id := nats.Header(message.Header).Get("Nats-Msg-Id")
			if id == "" {
				continue
			}
			if seenIDs[id] {
				repeatedIDs = true
				break
			}
			seenIDs[id] = true
		}
		if repeatedIDs {
			config.Duplicates = restoreDuplicateWindow
		}
		if _, err := js.CreateStream(ctx, config); err != nil {
			return summary, fmt.Errorf("create restored stream %s: %w", saved.Config.Name, err)
		}
		seenIDs = map[string]bool{}
		for _, message := range saved.Messages {
			msg := nats.NewMsg(message.Subject)
			msg.Header = nats.Header(message.Header)
			msg.Data = message.Data
			if repeatedIDs {
				id := msg.Header.Get("Nats-Msg-Id")
				if id != "" && seenIDs[id] {
					time.Sleep(2 * restoreDuplicateWindow)
				}
				if id != "" {
					seenIDs[id] = true
				}
			}
			ack, err := js.PublishMsg(ctx, msg)
			if err != nil {
				return summary, fmt.Errorf("restore stream %s message %d: %w", saved.Config.Name, message.Sequence, err)
			}
			if ack.Duplicate {
				return summary, fmt.Errorf("restore stream %s message %d was suppressed as a duplicate", saved.Config.Name, message.Sequence)
			}
		}
		if repeatedIDs {
			if _, err := js.UpdateStream(ctx, saved.Config); err != nil {
				return summary, fmt.Errorf("restore stream %s duplicate window: %w", saved.Config.Name, err)
			}
		}
		stream, err := js.Stream(ctx, saved.Config.Name)
		if err != nil {
			return summary, err
		}
		messages, err := readRecoveryStream(ctx, stream)
		if err != nil {
			return summary, err
		}
		restoredDigest, err := recoveryLogicalDigest(messages)
		if err != nil || restoredDigest != saved.Digest {
			return summary, fmt.Errorf("restored stream %s failed logical readback", saved.Config.Name)
		}
		for _, consumer := range saved.Consumers {
			if _, err := stream.CreateOrUpdateConsumer(ctx, consumer); err != nil {
				return summary, fmt.Errorf("restore consumer %s/%s: %w", saved.Config.Name, consumer.Durable, err)
			}
		}
		summary.Streams++
		summary.Messages += len(messages)
		summary.Digests[saved.Config.Name] = restoredDigest
	}
	return summary, nil
}

// recoveryLogicalDigest hashes stable wire fields in retained stream order. Broker sequence and timestamps are
// explicitly excluded: a clean restore may assign new physical metadata without changing the logical run.
func recoveryLogicalDigest(messages []recoveryMessage) (string, error) {
	h := sha256.New()
	if _, err := h.Write([]byte("wasl-recovery-v1\x00")); err != nil {
		return "", err
	}
	for _, m := range messages {
		if _, err := h.Write([]byte(m.Subject)); err != nil {
			return "", err
		}
		h.Write([]byte{0})
		keys := make([]string, 0, len(m.Header))
		for k := range m.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte{0})
			for _, v := range m.Header[k] {
				h.Write([]byte(v))
				h.Write([]byte{0})
			}
		}
		h.Write([]byte{0})
		h.Write(m.Data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readRequiredSecret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	value := strings.TrimRight(string(b), "\r\n")
	if value == "" {
		return "", fmt.Errorf("secret file %s is empty", path)
	}
	return value, nil
}

func natsSecurityOptions() ([]nats.Option, error) {
	user, passwordFile, caFile := os.Getenv("WASL_NATS_USER"), os.Getenv("WASL_NATS_PASSWORD_FILE"), os.Getenv("WASL_NATS_CA_FILE")
	var opts []nats.Option
	if user != "" || passwordFile != "" {
		if user == "" || passwordFile == "" {
			return nil, fmt.Errorf("WASL_NATS_USER and WASL_NATS_PASSWORD_FILE must be set together")
		}
		password, err := readRequiredSecret(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("NATS authentication: %w", err)
		}
		opts = append(opts, nats.UserInfo(user, password))
	}
	if caFile != "" {
		if _, err := os.ReadFile(caFile); err != nil {
			return nil, fmt.Errorf("NATS TLS CA: %w", err)
		}
		opts = append(opts, nats.RootCAs(caFile), nats.Secure())
	}
	certFile, keyFile := os.Getenv("WASL_NATS_CERT_FILE"), os.Getenv("WASL_NATS_KEY_FILE")
	if (certFile == "") != (keyFile == "") {
		return nil, fmt.Errorf("WASL_NATS_CERT_FILE and WASL_NATS_KEY_FILE must be set together")
	}
	if certFile != "" {
		if _, err := os.ReadFile(certFile); err != nil {
			return nil, fmt.Errorf("NATS client certificate: %w", err)
		}
		if _, err := os.ReadFile(keyFile); err != nil {
			return nil, fmt.Errorf("NATS client key: %w", err)
		}
		opts = append(opts, nats.ClientCert(certFile, keyFile))
	}
	return opts, nil
}

type apiUser struct {
	Password string `json:"password"`
	Role     string `json:"role"`
}

// apiAuthFromEnvironment returns pass-through middleware only for the explicitly unauthenticated local demo.
// Setting WASL_API_USERS_FILE enables HTTP Basic authentication over the configured TLS listener and replaces all
// caller-supplied identity headers with the authenticated server-side identity.
func apiAuthFromEnvironment() (func(http.Handler) http.Handler, error) {
	path := os.Getenv("WASL_API_USERS_FILE")
	if path == "" {
		return func(next http.Handler) http.Handler { return next }, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read API users: %w", err)
	}
	var users map[string]apiUser
	if err := json.Unmarshal(b, &users); err != nil || len(users) == 0 {
		return nil, fmt.Errorf("API users file is empty or invalid")
	}
	for name, user := range users {
		if name == "" || user.Password == "" {
			return nil, fmt.Errorf("API users file contains an empty identity or password")
		}
		if user.Role != RoleCoordinator && user.Role != RoleClinician && user.Role != RoleReviewer && user.Role != RoleAuditor {
			return nil, fmt.Errorf("API user %s has invalid role", name)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			name, password, ok := r.BasicAuth()
			user, exists := users[name]
			expected := user.Password
			if !exists {
				expected = password // equal-length dummy comparison; exists still fails closed below
			}
			passwordOK := subtle.ConstantTimeCompare([]byte(password), []byte(expected))
			if !ok || !exists || passwordOK != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="wasl", charset="UTF-8"`)
				writeStructuredLog("warn", "api authentication failed", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"authentication required"}` + "\n"))
				return
			}
			r.Header.Set("X-Wasl-Role", user.Role)
			r.Header.Set("X-Wasl-Actor", name)
			next.ServeHTTP(w, r)
		})
	}, nil
}
