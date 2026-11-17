package main

// Streamed NATS recovery payload (wasl-nats-backup-v2-stream).
//
// The v1 payload is one JSON document that backup, verify and restore each hold in memory in full. The v2 stream is
// newline-delimited JSON written and read one record at a time, so resident memory is bounded by the largest single
// record (one message), not by the stream:
//
//	{"schema":"wasl-nats-backup-v2-stream","created_at":...,"nondeterministic_fields":[...],"streams":N}
//	{"record":"stream","index":0,"config":{...},"consumers":[...]}
//	{"record":"message","sequence":S,"time":...,"subject":...,"header":{...},"data":"<base64>"}   (zero or more)
//	{"record":"stream_end","index":0,"messages":M,"bytes":B,"logical_sha256":"<v1 logical digest>"}
//	... one stream / message* / stream_end group per stream, in strictly ascending stream-name order ...
//	{"record":"end","streams":N,"messages":TOTAL}
//
// The per-stream logical digest is exactly the v1 digest of the stored (raw) headers. Every reader validates record
// grammar, ordering (stream index and name, strictly increasing message sequence), explicit counts and byte totals,
// header shape, and the original digest before it normalises broker headers; a missing trailer, any byte after the
// trailer, or an oversized record fails closed. Encryption and chunk authentication are the job of the encrypted file
// bundle (scripts/recovery.py, wasl-file-backup-v3-chunked); this format only carries the logical content.
import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const recoveryStreamSchema = "wasl-nats-backup-v2-stream"

// recoveryStreamMaxRecordBytes bounds one NDJSON record, and therefore the reader's working set. 96 MiB covers a
// base64-encoded message at the 64 MiB NATS max_payload ceiling with its JSON envelope; the production broker
// configuration uses the 1 MiB default. Tests lower it.
var recoveryStreamMaxRecordBytes = 96 << 20

// restoreDuplicateWindow is the shortest explicit deduplication window NATS accepts.
const restoreDuplicateWindow = 100 * time.Millisecond

var recoveryNondeterministicFields = []string{"created_at", "nats_sequence", "nats_time", "consumer_delivered", "consumer_ack_floor"}

type recoveryStreamHeader struct {
	Schema                 string    `json:"schema"`
	CreatedAt              time.Time `json:"created_at"`
	NondeterministicFields []string  `json:"nondeterministic_fields"`
	Streams                int       `json:"streams"`
}

type recoveryStreamStart struct {
	Record    string                     `json:"record"`
	Index     int                        `json:"index"`
	Config    jetstream.StreamConfig     `json:"config"`
	Consumers []jetstream.ConsumerConfig `json:"consumers"`
}

type recoveryStreamMessage struct {
	Record   string              `json:"record"`
	Sequence uint64              `json:"sequence"`
	Time     time.Time           `json:"time"`
	Subject  string              `json:"subject"`
	Header   map[string][]string `json:"header,omitempty"`
	Data     []byte              `json:"data"`
}

type recoveryStreamEnd struct {
	Record   string `json:"record"`
	Index    int    `json:"index"`
	Messages int    `json:"messages"`
	Bytes    int64  `json:"bytes"`
	Digest   string `json:"logical_sha256"`
}

type recoveryStreamTrailer struct {
	Record   string `json:"record"`
	Streams  int    `json:"streams"`
	Messages int    `json:"messages"`
}

// recoveryDigester computes recoveryLogicalDigest incrementally, one message at a time.
type recoveryDigester struct{ h hash.Hash }

func newRecoveryDigester() *recoveryDigester {
	h := sha256.New()
	h.Write([]byte("wasl-recovery-v1\x00"))
	return &recoveryDigester{h: h}
}

func (d *recoveryDigester) add(subject string, header map[string][]string, data []byte) {
	d.h.Write([]byte(subject))
	d.h.Write([]byte{0})
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		d.h.Write([]byte(k))
		d.h.Write([]byte{0})
		for _, v := range header[k] {
			d.h.Write([]byte(v))
			d.h.Write([]byte{0})
		}
	}
	d.h.Write([]byte{0})
	d.h.Write(data)
}

func (d *recoveryDigester) sum() string { return hex.EncodeToString(d.h.Sum(nil)) }

// eachRecoveryMessage visits every retained message of a stream in sequence order without holding the stream.
func eachRecoveryMessage(ctx context.Context, stream jetstream.Stream, visit func(recoveryMessage) error) error {
	info, err := stream.Info(ctx)
	if err != nil {
		return err
	}
	if info.State.Msgs == 0 {
		return nil
	}
	for sequence := info.State.FirstSeq; sequence <= info.State.LastSeq; sequence++ {
		raw, err := stream.GetMsg(ctx, sequence)
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			continue // retained streams may contain deleted-sequence gaps
		}
		if err != nil {
			return fmt.Errorf("read %s sequence %d: %w", info.Config.Name, sequence, err)
		}
		if err := visit(recoveryMessage{Sequence: raw.Sequence, Time: raw.Time, Subject: raw.Subject,
			Header: recoveryApplicationHeader(raw.Header), Data: raw.Data}); err != nil {
			return err
		}
	}
	return nil
}

// recoveryStreamDigest rereads a retained stream from the cluster and returns its message count and logical digest
// with bounded memory.
func recoveryStreamDigest(ctx context.Context, stream jetstream.Stream) (int, string, error) {
	digester, count := newRecoveryDigester(), 0
	err := eachRecoveryMessage(ctx, stream, func(m recoveryMessage) error {
		digester.add(m.Subject, m.Header, m.Data)
		count++
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	return count, digester.sum(), nil
}

func writeRecoveryRecord(w *bufio.Writer, value interface{}) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(encoded)+1 > recoveryStreamMaxRecordBytes {
		return fmt.Errorf("NATS recovery record of %d bytes exceeds the %d-byte record limit", len(encoded)+1, recoveryStreamMaxRecordBytes)
	}
	if _, err := w.Write(encoded); err != nil {
		return err
	}
	return w.WriteByte('\n')
}

// backupNATSStream writes every retained stream as a v2 stream to out, one message at a time.
func backupNATSStream(ctx context.Context, js jetstream.JetStream, out io.Writer) (recoverySummary, error) {
	summary := recoverySummary{Digests: map[string]string{}}
	infos, err := listRecoveryStreams(ctx, js)
	if err != nil {
		return summary, err
	}
	w := bufio.NewWriterSize(out, 1<<20)
	if err := writeRecoveryRecord(w, recoveryStreamHeader{Schema: recoveryStreamSchema, CreatedAt: time.Now().UTC(),
		NondeterministicFields: recoveryNondeterministicFields, Streams: len(infos)}); err != nil {
		return summary, err
	}
	for index, info := range infos {
		stream, err := js.Stream(ctx, info.Config.Name)
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
		if err := writeRecoveryRecord(w, recoveryStreamStart{Record: "stream", Index: index, Config: info.Config, Consumers: consumers}); err != nil {
			return summary, err
		}
		digester, count, size := newRecoveryDigester(), 0, int64(0)
		err = eachRecoveryMessage(ctx, stream, func(m recoveryMessage) error {
			digester.add(m.Subject, m.Header, m.Data)
			count++
			size += int64(len(m.Data))
			return writeRecoveryRecord(w, recoveryStreamMessage{Record: "message", Sequence: m.Sequence, Time: m.Time,
				Subject: m.Subject, Header: m.Header, Data: m.Data})
		})
		if err != nil {
			return summary, err
		}
		digest := digester.sum()
		if err := writeRecoveryRecord(w, recoveryStreamEnd{Record: "stream_end", Index: index, Messages: count, Bytes: size, Digest: digest}); err != nil {
			return summary, err
		}
		summary.Streams++
		summary.Messages += count
		summary.Digests[info.Config.Name] = digest
	}
	if err := writeRecoveryRecord(w, recoveryStreamTrailer{Record: "end", Streams: summary.Streams, Messages: summary.Messages}); err != nil {
		return summary, err
	}
	return summary, w.Flush()
}

// recoveryStreamPipe reports whether path names an inherited pipe/descriptor that must be written in place.
func recoveryStreamPipe(path string) bool {
	return path == "-" || strings.HasPrefix(path, "/dev/fd/") || path == "/dev/stdout" || path == "/dev/stdin"
}

// backupNATSStreamToPath writes a v2 stream to an inherited descriptor ("-" is stdout, "/dev/fd/N" an inherited pipe)
// in place, or to a new regular file through a private temporary and rename. An existing regular file is refused.
func backupNATSStreamToPath(ctx context.Context, js jetstream.JetStream, path string) (recoverySummary, error) {
	if path == "-" {
		return backupNATSStream(ctx, js, os.Stdout)
	}
	if recoveryStreamPipe(path) {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return recoverySummary{}, err
		}
		summary, err := backupNATSStream(ctx, js, file)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		return summary, err
	}
	if _, err := os.Lstat(path); err == nil {
		return recoverySummary{}, fmt.Errorf("NATS backup destination already exists: %s", path)
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return recoverySummary{}, err
	}
	temporary := file.Name()
	summary, err := backupNATSStream(ctx, js, file)
	if err == nil {
		err = file.Chmod(0600)
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(temporary, path) // never replaces an existing file
	}
	os.Remove(temporary)
	return summary, err
}

// recoveryStreamVisitor receives validated, normalised content. message receives headers with broker metadata and
// write preconditions already removed; streamEnd runs only after the stream's original digest, count and size
// matched its stream_end record and receives the digest of the normalised messages.
type recoveryStreamVisitor struct {
	header    func(recoveryStreamHeader) error
	stream    func(recoveryStreamStart) error
	message   func(recoveryMessage) error
	streamEnd func(start recoveryStreamStart, messages int, normalizedDigest string) error
}

func readRecoveryRecord(r *bufio.Reader) ([]byte, error) {
	var record []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(record)+len(chunk) > recoveryStreamMaxRecordBytes {
			return nil, fmt.Errorf("NATS recovery record exceeds the %d-byte limit", recoveryStreamMaxRecordBytes)
		}
		record = append(record, chunk...)
		switch {
		case err == nil:
			return record[:len(record)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(record) == 0 {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("NATS recovery stream is truncated inside a record")
		default:
			return nil, err
		}
	}
}

// decodeRecoveryRecord decodes one record, refusing unknown fields and requiring every field in required.
func decodeRecoveryRecord(raw []byte, required []string, optional []string, target interface{}) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("NATS recovery record is not a JSON object")
	}
	allowed := map[string]bool{}
	for _, name := range optional {
		allowed[name] = true
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("NATS recovery record is missing %q", name)
		}
		allowed[name] = true
	}
	for name := range fields {
		if !allowed[name] {
			return fmt.Errorf("NATS recovery record has unexpected field %q", name)
		}
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("NATS recovery record is malformed")
	}
	return nil
}

func recoveryRecordKind(raw []byte) string {
	var probe struct {
		Record string `json:"record"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return ""
	}
	return probe.Record
}

// scanNATSStream validates a complete v2 stream from r and feeds normalised content to visitor. Any grammar, order,
// count, size, digest or trailer violation stops the scan with an error.
func scanNATSStream(r io.Reader, visitor recoveryStreamVisitor) (recoverySummary, error) {
	summary := recoverySummary{Digests: map[string]string{}}
	reader, ok := r.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReaderSize(r, 1<<20)
	}
	raw, err := readRecoveryRecord(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return summary, fmt.Errorf("NATS recovery stream is empty")
		}
		return summary, err
	}
	var header recoveryStreamHeader
	if err := decodeRecoveryRecord(raw, []string{"schema", "created_at", "nondeterministic_fields", "streams"}, nil, &header); err != nil {
		return summary, err
	}
	if header.Schema != recoveryStreamSchema {
		return summary, fmt.Errorf("unsupported NATS recovery stream schema")
	}
	if header.Streams < 0 || strings.Join(header.NondeterministicFields, ",") != strings.Join(recoveryNondeterministicFields, ",") {
		return summary, fmt.Errorf("NATS recovery stream header is malformed")
	}
	if visitor.header != nil {
		if err := visitor.header(header); err != nil {
			return summary, err
		}
	}
	previousName, total := "", 0
	for index := 0; index < header.Streams; index++ {
		raw, err := readRecoveryRecord(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return summary, fmt.Errorf("NATS recovery stream is truncated: %d of %d streams present", index, header.Streams)
			}
			return summary, err
		}
		if recoveryRecordKind(raw) != "stream" {
			return summary, fmt.Errorf("NATS recovery stream record %d is out of order", index)
		}
		var start recoveryStreamStart
		if err := decodeRecoveryRecord(raw, []string{"record", "index", "config", "consumers"}, nil, &start); err != nil {
			return summary, err
		}
		name := start.Config.Name
		if start.Index != index || name == "" || (index > 0 && name <= previousName) {
			return summary, fmt.Errorf("NATS recovery stream %q is duplicated, reordered or misnumbered", name)
		}
		previousName = name
		if visitor.stream != nil {
			if err := visitor.stream(start); err != nil {
				return summary, err
			}
		}
		original, normalized := newRecoveryDigester(), newRecoveryDigester()
		count, size, lastSequence := 0, int64(0), uint64(0)
		for {
			raw, err := readRecoveryRecord(reader)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return summary, fmt.Errorf("NATS recovery stream %s is truncated", name)
				}
				return summary, err
			}
			kind := recoveryRecordKind(raw)
			if kind == "stream_end" {
				var end recoveryStreamEnd
				if err := decodeRecoveryRecord(raw, []string{"record", "index", "messages", "bytes", "logical_sha256"}, nil, &end); err != nil {
					return summary, err
				}
				if end.Index != index || end.Messages != count || end.Bytes != size {
					return summary, fmt.Errorf("NATS recovery stream %s count or size differs from its stream_end record", name)
				}
				// Original-digest validation before any normalised content is acted on as a whole stream.
				if end.Digest != original.sum() {
					return summary, fmt.Errorf("NATS backup logical hash mismatch for %s", name)
				}
				digest := normalized.sum()
				if visitor.streamEnd != nil {
					if err := visitor.streamEnd(start, count, digest); err != nil {
						return summary, err
					}
				}
				summary.Streams++
				summary.Messages += count
				summary.Digests[name] = digest
				total += count
				break
			}
			if kind != "message" {
				return summary, fmt.Errorf("NATS recovery stream %s has an out-of-order %q record", name, kind)
			}
			var message recoveryStreamMessage
			if err := decodeRecoveryRecord(raw, []string{"record", "sequence", "time", "subject", "data"}, []string{"header"}, &message); err != nil {
				return summary, err
			}
			if message.Sequence == 0 || message.Sequence <= lastSequence || message.Subject == "" {
				return summary, fmt.Errorf("NATS recovery stream %s sequence %d is duplicated, reordered or malformed", name, message.Sequence)
			}
			lastSequence = message.Sequence
			if err := validateRecoveryHeaderShape(message.Header); err != nil {
				return summary, fmt.Errorf("NATS backup stream %s sequence %d: %w", name, message.Sequence, err)
			}
			original.add(message.Subject, message.Header, message.Data)
			clean := recoveryApplicationHeader(nats.Header(message.Header))
			normalized.add(message.Subject, clean, message.Data)
			count++
			size += int64(len(message.Data))
			if visitor.message != nil {
				if err := visitor.message(recoveryMessage{Sequence: message.Sequence, Time: message.Time, Subject: message.Subject,
					Header: clean, Data: message.Data}); err != nil {
					return summary, err
				}
			}
		}
	}
	raw, err = readRecoveryRecord(reader)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return summary, fmt.Errorf("NATS recovery stream is truncated: trailer missing")
		}
		return summary, err
	}
	if recoveryRecordKind(raw) != "end" {
		return summary, fmt.Errorf("NATS recovery stream has records after its last declared stream")
	}
	var trailer recoveryStreamTrailer
	if err := decodeRecoveryRecord(raw, []string{"record", "streams", "messages"}, nil, &trailer); err != nil {
		return summary, err
	}
	if trailer.Streams != header.Streams || trailer.Messages != total {
		return summary, fmt.Errorf("NATS recovery stream trailer totals differ from its content")
	}
	if _, err := reader.ReadByte(); err == nil {
		return summary, fmt.Errorf("NATS recovery stream has data after its trailer")
	} else if !errors.Is(err, io.EOF) {
		return summary, err
	}
	return summary, nil
}

// openRecoveryPayload opens a NATS payload ("-" is stdin) and reports whether it is a v2 stream and whether it can
// be reread (a regular file).
func openRecoveryPayload(path string) (*bufio.Reader, io.Closer, bool, bool, error) {
	var file *os.File
	if path == "-" {
		file = os.Stdin
	} else {
		opened, err := os.Open(path)
		if err != nil {
			return nil, nil, false, false, err
		}
		file = opened
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, false, false, err
	}
	reader := bufio.NewReaderSize(file, 1<<20)
	prefix := []byte(`{"schema":"` + recoveryStreamSchema + `"`)
	peek, _ := reader.Peek(len(prefix))
	return reader, file, bytes.Equal(peek, prefix), info.Mode().IsRegular(), nil
}

// loadNATSRecoveryPayloadV1 reads a whole v1 payload from an already opened reader.
func loadNATSRecoveryPayloadV1(reader io.Reader) (recoveryNATSBundle, error) {
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return recoveryNATSBundle{}, err
	}
	return parseNATSRecoveryBundle(encoded)
}

// checkNATS validates a payload offline (no broker connection) and returns its normalised summary.
func checkNATS(path string) (recoverySummary, error) {
	reader, closer, stream, _, err := openRecoveryPayload(path)
	if err != nil {
		return recoverySummary{}, err
	}
	defer closer.Close()
	if stream {
		return scanNATSStream(reader, recoveryStreamVisitor{})
	}
	bundle, err := loadNATSRecoveryPayloadV1(reader)
	if err != nil {
		return recoverySummary{}, err
	}
	summary := recoverySummary{Digests: map[string]string{}}
	for _, saved := range bundle.Streams {
		summary.Streams++
		summary.Messages += len(saved.Messages)
		summary.Digests[saved.Config.Name] = saved.Digest
	}
	return summary, nil
}

// recoveryRepublisher republishes messages into a stream created with the shortest deduplication window and waits
// out that window only when an ID was published very recently, so a retained repeated Nats-Msg-Id is neither
// suppressed nor tracked for longer than two windows (bounded memory).
type recoveryRepublisher struct {
	recent  map[string]time.Time
	publish int
}

func (p *recoveryRepublisher) publishMsg(ctx context.Context, js jetstream.JetStream, stream string, m recoveryMessage) error {
	msg := nats.NewMsg(m.Subject)
	msg.Header = nats.Header(m.Header)
	msg.Data = m.Data
	id := msg.Header.Get("Nats-Msg-Id")
	if id != "" {
		if previous, ok := p.recent[id]; ok {
			if wait := 2*restoreDuplicateWindow - time.Since(previous); wait > 0 {
				time.Sleep(wait)
			}
		}
	}
	ack, err := js.PublishMsg(ctx, msg)
	if err != nil {
		return fmt.Errorf("restore stream %s message %d: %w", stream, m.Sequence, err)
	}
	if ack.Duplicate {
		return fmt.Errorf("restore stream %s message %d was suppressed as a duplicate", stream, m.Sequence)
	}
	if id != "" {
		p.recent[id] = time.Now()
	}
	if p.publish++; p.publish%1024 == 0 {
		for key, at := range p.recent {
			if time.Since(at) >= 2*restoreDuplicateWindow {
				delete(p.recent, key)
			}
		}
	}
	return nil
}

// restoreNATSStream restores a v2 stream into an empty JetStream. A rereadable regular file is validated completely
// (scan pass) before the first write. A pipe cannot be reread, so it is accepted only with prechecked=true, after the
// caller has validated an identical authenticated copy with check-nats; the restore pass still re-validates every
// stream's digest and readback and stops at the first difference.
func restoreNATSStream(ctx context.Context, js jetstream.JetStream, path string, reader *bufio.Reader, rereadable, prechecked bool) (recoverySummary, error) {
	if !prechecked {
		if !rereadable {
			return recoverySummary{}, fmt.Errorf("a streamed NATS payload that cannot be reread must be validated with check-nats first (-prechecked)")
		}
		if _, err := checkNATS(path); err != nil {
			return recoverySummary{}, err
		}
	}
	infos, err := listRecoveryStreams(ctx, js)
	if err != nil {
		return recoverySummary{}, err
	}
	if len(infos) != 0 {
		return recoverySummary{}, fmt.Errorf("restore target JetStream is not empty: %d streams already exist", len(infos))
	}
	publisher, current := &recoveryRepublisher{}, ""
	return scanNATSStream(reader, recoveryStreamVisitor{
		stream: func(start recoveryStreamStart) error {
			current = start.Config.Name
			config := start.Config
			config.Duplicates = restoreDuplicateWindow
			publisher.recent = map[string]time.Time{}
			if _, err := js.CreateStream(ctx, config); err != nil {
				return fmt.Errorf("create restored stream %s: %w", start.Config.Name, err)
			}
			return nil
		},
		message: func(m recoveryMessage) error {
			return publisher.publishMsg(ctx, js, current, m)
		},
		streamEnd: func(start recoveryStreamStart, messages int, digest string) error {
			name := start.Config.Name
			if _, err := js.UpdateStream(ctx, start.Config); err != nil {
				return fmt.Errorf("restore stream %s duplicate window: %w", name, err)
			}
			stream, err := js.Stream(ctx, name)
			if err != nil {
				return err
			}
			count, restored, err := recoveryStreamDigest(ctx, stream)
			if err != nil {
				return err
			}
			if count != messages || restored != digest {
				return fmt.Errorf("restored stream %s failed logical readback", name)
			}
			for _, consumer := range start.Consumers {
				if _, err := stream.CreateOrUpdateConsumer(ctx, consumer); err != nil {
					return fmt.Errorf("restore consumer %s/%s: %w", name, consumer.Durable, err)
				}
			}
			return nil
		},
	})
}

// verifyNATSStream compares every retained cluster stream to a v2 stream payload. It never writes to the cluster.
func verifyNATSStream(ctx context.Context, js jetstream.JetStream, reader *bufio.Reader) (recoverySummary, error) {
	return scanNATSStream(reader, recoveryStreamVisitor{
		header: func(header recoveryStreamHeader) error {
			infos, err := listRecoveryStreams(ctx, js)
			if err != nil {
				return err
			}
			if len(infos) != header.Streams {
				return fmt.Errorf("retained stream count differs: backup=%d cluster=%d", header.Streams, len(infos))
			}
			return nil
		},
		streamEnd: func(start recoveryStreamStart, messages int, digest string) error {
			stream, err := js.Stream(ctx, start.Config.Name)
			if err != nil {
				return err
			}
			count, retained, err := recoveryStreamDigest(ctx, stream)
			if err != nil {
				return err
			}
			if count != messages || retained != digest {
				return fmt.Errorf("retained stream %s differs from backup logical hash", start.Config.Name)
			}
			return nil
		},
	})
}
