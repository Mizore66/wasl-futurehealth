package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// An acceptance-barrier failure must remain a failure, but teardown must still
// join the real producer before closing its publisher queue. The child fails
// deliberately so testing.T's real Fatalf/Goexit cleanup path is exercised.
func TestPendingOwnerCleanupJoinsProducerAfterBarrierFailure(t *testing.T) {
	const child = "WASL_TEST_PENDING_OWNER_CLEANUP_CHILD"
	if os.Getenv(child) == "1" {
		c, _, js := newPendingOwnerConsumer(t)
		done, release := make(chan struct{}), make(chan struct{})
		// Registered first: inspect after both producer and publisher cleanup.
		t.Cleanup(func() {
			select {
			case <-done:
				fmt.Println("cleanup joined late producer")
			default:
				fmt.Println("cleanup escaped before producer exit")
			}
		})
		pendingOwnerStartPublisher(t, c, js)
		body := durableSubscriptionNotification(t, "pending-owner", "p1", 1)
		go func() {
			defer close(done)
			<-release
			_ = c.processInboxMessage(&pendingOwnerMsg{body: body})
		}()
		t.Cleanup(func() {
			pendingOwnerCleanupInbox(t, js, func() {
				// Longer than the original 1500ms barrier, never an increased
				// acceptance budget: the child must report that exact failure.
				time.AfterFunc(2*time.Second, func() { close(release) })
			}, done)
		})
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPendingOwnerCleanupJoinsProducerAfterBarrierFailure$", "-test.timeout=15s", "-test.v")
	cmd.Env = append(os.Environ(), child+"=1")
	output, err := cmd.CombinedOutput()
	text := string(output)
	if err == nil || !strings.Contains(text, "timed out waiting for inbox exit") {
		t.Fatalf("original cleanup barrier failure was hidden: err=%v\n%s", err, text)
	}
	if !strings.Contains(text, "cleanup joined late producer") || strings.Contains(text, "cleanup escaped before producer exit") || strings.Contains(text, "DATA RACE") || strings.Contains(text, "panic:") {
		t.Fatalf("failed acceptance barrier escaped producer ownership:\n%s", text)
	}
}
