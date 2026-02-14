package protocol_test

import (
	"sort"
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
