package protocol

import "sync"

// DeviceSession holds protocol details of a live device connection that are
// needed to encode commands in the format the device speaks.
type DeviceSession struct {
	// Manufacturer is the WATCH manufacturer code the device uses (e.g. "3G", "SG", "ZJ").
	Manufacturer string
	// Indexed reports whether the WATCH device uses indexed frames.
	Indexed bool
}

// DeviceRegistry tracks live device TCP connections by unique device ID.
// It allows the HTTP command handler to deliver bytes to a connected device.
type DeviceRegistry struct {
	mu    sync.RWMutex
	conns map[string]registeredConn
}

type registeredConn struct {
	ch      chan<- []byte
	session *DeviceSession
}

// NewDeviceRegistry creates an empty DeviceRegistry.
func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{conns: make(map[string]registeredConn)}
}

// Register associates uniqueID with an outbound write channel.
// The server calls this as soon as it knows the device identity.
func (r *DeviceRegistry) Register(uniqueID string, ch chan<- []byte) {
	r.mu.Lock()
	c := r.conns[uniqueID]
	c.ch = ch
	r.conns[uniqueID] = c
	r.mu.Unlock()
}

// Deregister removes the mapping for uniqueID (called on disconnect).
func (r *DeviceRegistry) Deregister(uniqueID string) {
	r.mu.Lock()
	delete(r.conns, uniqueID)
	r.mu.Unlock()
}

// SetSession records the protocol session details of a registered device.
// It is a no-op for devices without a registered connection.
func (r *DeviceRegistry) SetSession(uniqueID string, s DeviceSession) {
	r.mu.Lock()
	if c, ok := r.conns[uniqueID]; ok {
		c.session = &s
		r.conns[uniqueID] = c
	}
	r.mu.Unlock()
}

// Session returns the protocol session details of a connected device.
func (r *DeviceRegistry) Session(uniqueID string) (DeviceSession, bool) {
	r.mu.RLock()
	c := r.conns[uniqueID]
	r.mu.RUnlock()
	if c.session == nil {
		return DeviceSession{}, false
	}
	return *c.session, true
}

// Send writes data to the outbound channel for uniqueID.
// Returns true if the device is online and the send succeeded, false otherwise.
//
// Channels stored in the registry are owned by the connection goroutine and are
// never closed while registered. The recover below is a belt-and-braces guard
// against a future regression that violates that invariant.
func (r *DeviceRegistry) Send(uniqueID string, data []byte) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	r.mu.RLock()
	c, found := r.conns[uniqueID]
	r.mu.RUnlock()
	if !found {
		return false
	}
	select {
	case c.ch <- data:
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
