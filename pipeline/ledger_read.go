package main

import (
	"bufio"
	"errors"
)

const maxLedgerRecordBytes = 1 << 24

var errLedgerRecordTooLarge = errors.New("record exceeds 16 MiB")

// readLedgerRecord accumulates delimiter fragments only while the complete
// newline-terminated record fits the ledger limit. A partial record is returned
// with io.EOF so callers can distinguish truncation from a clean end of file.
func readLedgerRecord(r *bufio.Reader) ([]byte, error) {
	out := make([]byte, 0, 64*1024)
	for {
		frag, err := r.ReadSlice('\n')
		if len(frag) > maxLedgerRecordBytes-len(out) {
			return nil, errLedgerRecordTooLarge
		}
		out = append(out, frag...)
		if err == bufio.ErrBufferFull {
			continue
		}
		return out, err
	}
}
