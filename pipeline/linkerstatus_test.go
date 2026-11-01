package main

import (
	"encoding/json"
	"sync"
	"testing"
)

// X10 (80 linker restarts): the status goroutine built a document that aliased L.Stats under the linker mutex, then
// serialised it after unlocking while the apply loop kept writing to the map ("concurrent map iteration and map write").
// This mirrors that exact protocol: apply under the mutex, build under the mutex, marshal outside it.
func TestLinkerStatusSurvivesConcurrentApply(t *testing.T) {
	L := NewLinkerCore(nil, 4)
	var mu sync.Mutex
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			mu.Lock()
			L.Stats["merges"]++
			L.Stats["k"+itoa(i%97)]++ // new keys force map growth, as contact_* / eid_* counters do
			mu.Unlock()
		}
	}()
	for i := 0; i < 3000; i++ {
		mu.Lock()
		st := linkerStatus(L, uint64(i), 0, 0, 0, -1, 0, 0, 0)
		mu.Unlock()
		if _, err := json.Marshal(st); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}

// The status document is a snapshot: later linker writes must not appear in it.
func TestLinkerStatusIsASnapshot(t *testing.T) {
	L := NewLinkerCore(nil, 2)
	L.Stats["merges"] = 1
	L.PSeq[0] = 5
	st := linkerStatus(L, 1, 0, 0, 0, -1, 0, 0, 0)
	L.Stats["merges"] = 2
	L.Stats["splits"] = 1
	L.PSeq[0] = 6
	b, _ := json.Marshal(st)
	var back struct {
		Stats map[string]int `json:"stats"`
		PSeq  []uint64       `json:"pseq"`
	}
	json.Unmarshal(b, &back)
	if back.Stats["merges"] != 1 || len(back.Stats) != 1 || back.PSeq[0] != 5 {
		t.Fatalf("status aliases linker state: %+v", back)
	}
}

// The real document, produced the way runLinker produces it (build under the mutex that serialises Apply, marshal
// after releasing it) while real Apply calls mutate every counter and registration. The first X10 fix was tested
// only through hand-written counter increments; the rerun showed that is not the production path.
func TestRealLinkerStatusDocumentDuringApply(t *testing.T) {
	e1, e2 := luhnComplete("78419802077524"), luhnComplete("78419807358085")
	L := NewLinkerCore(constGBM(), 4)
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 1; i <= 120; i++ {
			p := eidEvent("e"+itoa(i), "ahs-mushrif", "M"+itoa(i), "Lamia Eddinari", "1999-07-19", []string{"", e1, e2}[i%3])
			p.Seq = uint64(i)
			if i%5 == 0 {
				p = consentPrep(uint64(i), "ahs-mushrif|M"+itoa(i-1), RoleCoordinator, []string{"permit", "deny"}[i%2])
			}
			mu.Lock()
			L.Apply(p)
			mu.Unlock()
		}
	}()
	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
		}
		mu.Lock()
		st := linkerStatus(L, 1, 0, 0, 0, -1, 0, 0, 0)
		mu.Unlock()
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		var back map[string]interface{}
		if err := json.Unmarshal(b, &back); err != nil || back["stats"] == nil {
			t.Fatalf("status document unreadable: %v %s", err, b)
		}
	}
}
