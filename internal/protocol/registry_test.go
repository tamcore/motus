package protocol_test

import (
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/tamcore/motus/internal/protocol"
)

func TestDeviceRegistry_OnlineDeviceIDs_Empty(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	ids := r.OnlineDeviceIDs()
	if len(ids) != 0 {
		t.Errorf("expected empty slice, got %v", ids)
	}
}

func TestDeviceRegistry_OnlineDeviceIDs_RegisteredDevices(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	ch1 := make(chan []byte, 1)
	ch2 := make(chan []byte, 1)
	r.Register("DEV001", ch1)
	r.Register("DEV002", ch2)

	ids := r.OnlineDeviceIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d: %v", len(ids), ids)
	}
	sort.Strings(ids)
	if ids[0] != "DEV001" || ids[1] != "DEV002" {
		t.Errorf("unexpected IDs: %v", ids)
	}
}

func TestDeviceRegistry_OnlineDeviceIDs_AfterDeregister(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	ch := make(chan []byte, 1)
	r.Register("DEV001", ch)
	r.Deregister("DEV001")

	ids := r.OnlineDeviceIDs()
	if len(ids) != 0 {
		t.Errorf("expected empty after deregister, got %v", ids)
	}
}

func TestDeviceRegistry_OnlineDeviceIDs_IsSnapshot(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	ch := make(chan []byte, 1)
	r.Register("DEV001", ch)

	ids := r.OnlineDeviceIDs()
	// Mutate the registry after taking snapshot.
	r.Deregister("DEV001")

	// Snapshot should be unaffected.
	if len(ids) != 1 || ids[0] != "DEV001" {
		t.Errorf("snapshot should not reflect subsequent mutations, got %v", ids)
	}
}

func TestDeviceRegistry_IsOnline(t *testing.T) {
	r := protocol.NewDeviceRegistry()

	if r.IsOnline("DEV001") {
		t.Error("expected IsOnline=false for unregistered device")
	}

	ch := make(chan []byte, 1)
	r.Register("DEV001", ch)

	if !r.IsOnline("DEV001") {
		t.Error("expected IsOnline=true after Register")
	}

	r.Deregister("DEV001")

	if r.IsOnline("DEV001") {
		t.Error("expected IsOnline=false after Deregister")
	}
}

// TestDeviceRegistry_Send_AfterChannelClosedDoesNotPanic verifies that Send
// never panics when the underlying channel has been closed. This guards against
// the send-on-closed-channel race between connection teardown and an in-flight Send.
func TestDeviceRegistry_Send_AfterChannelClosedDoesNotPanic(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	ch := make(chan []byte, 1)
	r.Register("DEV001", ch)
	close(ch) // simulate the race: channel closed while still in registry

	panicked := make(chan bool, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				panicked <- true
				return
			}
			panicked <- false
		}()
		r.Send("DEV001", []byte("cmd"))
	}()

	if <-panicked {
		t.Fatal("Send panicked on a closed channel — race condition not fixed")
	}
}

// TestDeviceRegistry_Send_ConcurrentDeregisterAndSend stress-tests Send under
// concurrent Register/Deregister activity. Must not panic under -race.
// Channels are never closed here, matching the invariant enforced in production:
// the connection goroutine abandons (never closes) outCh on exit.
func TestDeviceRegistry_Send_ConcurrentDeregisterAndSend(t *testing.T) {
	r := protocol.NewDeviceRegistry()
	const devices = 50
	var wg sync.WaitGroup

	for i := range devices {
		id := fmt.Sprintf("DEV%03d", i)
		wg.Add(2)

		// Lifecycle goroutine: register then deregister — channel is never closed.
		go func(id string) {
			defer wg.Done()
			ch := make(chan []byte, 16)
			r.Register(id, ch)
			r.Deregister(id)
		}(id)

		// Sender goroutine: hammer Send while lifecycle races above.
		go func(id string) {
			defer wg.Done()
			for range 20 {
				r.Send(id, []byte("cmd"))
			}
		}(id)
	}

	wg.Wait()
}
