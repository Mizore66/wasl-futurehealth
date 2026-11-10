package main

import (
	"fmt"
	"os"
	"syscall"
)

// lockReviewLedger serializes writers that share a run directory, including
// independent apiServer values and separate processes on Unix deployments.
// The lock covers disk revalidation, append, sync and rollback.
func lockReviewLedger(path string) (func(), error) {
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock review ledger: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
