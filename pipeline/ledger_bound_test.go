package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const adversarialLedgerLimit = 1 << 24

var errLedgerReadOvershot = errors.New("test reader consumed beyond the oversized prefix")

type oversizedPrefixReader struct {
	remaining int
	read      int
}

func (r *oversizedPrefixReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, errLedgerReadOvershot
	}
	if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= len(p)
	r.read += len(p)
	return len(p), nil
}

func TestReadLedgerRecordStopsAtLimitWithoutReadingWholeInput(t *testing.T) {
	source := &oversizedPrefixReader{remaining: adversarialLedgerLimit + 1}
	line, err := readLedgerRecord(bufio.NewReaderSize(source, 64*1024))
	if !errors.Is(err, errLedgerRecordTooLarge) {
		t.Fatalf("oversized record error = %v, want errLedgerRecordTooLarge", err)
	}
	if line != nil {
		t.Fatalf("oversized record returned %d retained bytes", len(line))
	}
	if source.read > adversarialLedgerLimit+1 {
		t.Fatalf("reader consumed %d bytes before rejecting a %d-byte limit", source.read, adversarialLedgerLimit)
	}
}

func writeOversizedLedger(t *testing.T, terminated bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.ndjson")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	size := int64(adversarialLedgerLimit + 1)
	if terminated {
		size++
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if terminated {
		if _, err := f.WriteAt([]byte{'\n'}, size-1); err != nil {
			f.Close()
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLedgerReadersRejectOversizedRecordsDuringBoundedReads(t *testing.T) {
	for _, terminated := range []bool{false, true} {
		shape := "unterminated"
		if terminated {
			shape = "terminated"
		}
		for _, reader := range []string{"review verification", "reader replay", "API replay"} {
			t.Run(fmt.Sprintf("%s/%s", reader, shape), func(t *testing.T) {
				path := writeOversizedLedger(t, terminated)
				var err error
				switch reader {
				case "review verification":
					_, err = readVerifiedLedger(path)
				case "reader replay":
					c := NewReaderClient("", 0, 0, path)
					err = c.ledgerErr
				case "API replay":
					s := &apiServer{run: filepath.Dir(path)}
					if err := os.Rename(path, filepath.Join(s.run, "api_reviews.ndjson")); err != nil {
						t.Fatal(err)
					}
					s.loadReviews()
					if s.ledgerErr != "" {
						err = errors.New(s.ledgerErr)
					}
				}
				if err == nil || !strings.Contains(err.Error(), "record exceeds 16 MiB") {
					t.Fatalf("oversized ledger error = %v, want bounded record-limit rejection", err)
				}
			})
		}
	}
}
