package protocol

import "sync"

// DeviceRegistry tracks live device TCP connections by unique device ID.
// It allows the HTTP command handler to deliver bytes to a connected device.
type DeviceRegistry struct {
	mu    sync.RWMutex
	conns map[string]chan<- []byte
}

// NewDeviceRegistry creates an empty DeviceRegistry.
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{conns: make(map[string]chan<- []byte)}
}

// Register associates uniqueID with an outbound write channel.
// The server calls this as soon as it knows the device identity.
func (r *DeviceRegistry) Register(uniqueID string, ch chan<- []byte) {
	r.mu.Lock()
	r.conns[uniqueID] = ch
	r.mu.Unlock()
}

// Deregister removes the mapping for uniqueID (called on disconnect).
func (r *DeviceRegistry) Deregister(uniqueID string) {
	r.mu.Lock()
	delete(r.conns, uniqueID)
	r.mu.Unlock()
}

// Send writes data to the outbound channel for uniqueID.
// Returns true if the device is online and the send succeeded, false otherwise.
func (r *DeviceRegistry) Send(uniqueID string, data []byte) bool {
	r.mu.RLock()
	ch, ok := r.conns[uniqueID]
	r.mu.RUnlock()
	if !ok {
		return false
	}
	select {
	case ch <- data:
		return true
	default:
		// Channel full — drop the message rather than block.
		return false
	}
}

// IsOnline reports whether the device with uniqueID has an active connection.
func (r *DeviceRegistry) IsOnline(uniqueID string) bool {
	r.mu.RLock()
	_, ok := r.conns[uniqueID]
	r.mu.RUnlock()
	return ok
}

// OnlineDeviceIDs returns a snapshot of all currently registered device unique IDs.
func (r *DeviceRegistry) OnlineDeviceIDs() []string {
	r.mu.RLock()
	ids := make([]string, 0, len(r.conns))
	for id := range r.conns {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	return ids
}
