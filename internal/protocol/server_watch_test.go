package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// memDeviceRepo is an in-memory DeviceRepo for WATCH server tests. Methods
// not used by the protocol server panic via the nil embedded interface.
type memDeviceRepo struct {
	repository.DeviceRepo
	mu      sync.Mutex
	nextID  int64
	devices map[string]*model.Device
}

func newMemDeviceRepo(devices ...*model.Device) *memDeviceRepo {
	r := &memDeviceRepo{devices: map[string]*model.Device{}}
	for _, d := range devices {
		r.nextID++
		d.ID = r.nextID
		r.devices[d.UniqueID] = d
	}
	return r
}

func (r *memDeviceRepo) GetByUniqueID(_ context.Context, uniqueID string) (*model.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[uniqueID]
	if !ok {
		return nil, fmt.Errorf("device %s not found", uniqueID)
	}
	c := *d
	return &c, nil
}

func (r *memDeviceRepo) GetByID(_ context.Context, id int64) (*model.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.ID == id {
			c := *d
			return &c, nil
		}
	}
	return nil, fmt.Errorf("device %d not found", id)
}

func (r *memDeviceRepo) Update(_ context.Context, d *model.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *d
	r.devices[d.UniqueID] = &c
	return nil
}

func (r *memDeviceRepo) UpdateProtocol(_ context.Context, id int64, protocol string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.ID == id {
			d.Protocol = protocol
		}
	}
	return nil
}

func (r *memDeviceRepo) get(uniqueID string) *model.Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[uniqueID]
	if !ok {
		return nil
	}
	c := *d
	return &c
}

// memPositionRepo is an in-memory PositionRepo for WATCH server tests.
type memPositionRepo struct {
	repository.PositionRepo
	mu        sync.Mutex
	positions []*model.Position
}

func (r *memPositionRepo) Create(_ context.Context, p *model.Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = int64(len(r.positions) + 1)
	r.positions = append(r.positions, p)
	return nil
}

func (r *memPositionRepo) GetLatestByDevice(_ context.Context, deviceID int64) (*model.Position, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *model.Position
	for _, p := range r.positions {
		if p.DeviceID == deviceID && (latest == nil || !p.Timestamp.Before(latest.Timestamp)) {
			latest = p
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("no positions")
	}
	return latest, nil
}

func (r *memPositionRepo) all() []*model.Position {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*model.Position(nil), r.positions...)
}

type watchTestEnv struct {
	srv       *Server
	devices   *memDeviceRepo
	positions *memPositionRepo
	device    *model.Device
}

// newWatchTestEnv creates a WATCH server with one known, offline device.
func newWatchTestEnv(t *testing.T, uniqueID string) *watchTestEnv {
	t.Helper()
	device := &model.Device{UniqueID: uniqueID, Name: "Watch", Protocol: "watch", Status: "offline"}
	devices := newMemDeviceRepo(device)
	positions := &memPositionRepo{}
	hub := websocket.NewHub(nil, nil, func(*http.Request) int64 { return 0 })
	handler := NewPositionHandler(positions, devices, hub, nil)
	return &watchTestEnv{
		srv:       NewWatchServer("0", devices, handler),
		devices:   devices,
		positions: positions,
		device:    device,
	}
}

func (e *watchTestEnv) seedPosition(t *testing.T, lat, lon float64) *model.Position {
	t.Helper()
	speed, course, alt := 3.0, 90.0, 120.0
	p := &model.Position{
		DeviceID:  e.device.ID,
		Protocol:  "watch",
		Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Valid:     true,
		Latitude:  lat,
		Longitude: lon,
		Speed:     &speed,
		Course:    &course,
		Altitude:  &alt,
	}
	_ = e.positions.Create(context.Background(), p)
	return p
}

// TestWatchServer_ConcatenatedFramesWithoutNewlines reproduces real device
// traffic (Traccar's WatchFrameDecoderTest vector): several frames in one TCP
// segment with no line terminators. Every frame must be decoded, the position
// stored, the device marked online, and acks sent without CRLF.
func TestWatchServer_ConcatenatedFramesWithoutNewlines(t *testing.T) {
	env := newWatchTestEnv(t, "9705141740")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	env.srv.listener = listener
	go env.srv.acceptLoop(t.Context())

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	input := "[3G*9705141740*0009*LK,0,0,53]" +
		"[3G*9705141740*005A*UD,190723,190707,A,36.815109,N,10.1792312,E,8.24,127.9,21.0,5,100,53,0,0,00000000,0,0,58.0]" +
		"[3G*9705141740*0003*TKQ]" +
		"[3G*9705141740*0009*LK,0,0,53]"
	if _, err := io.WriteString(conn, input); err != nil {
		t.Fatalf("write: %v", err)
	}

	// UD is not acknowledged. The second LK carries battery data and is
	// stored at the last known location (the UD fix).
	want := "[3G*9705141740*0002*LK][3G*9705141740*0003*TKQ][3G*9705141740*0002*LK]"
	got := make([]byte, len(want))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read responses: %v (got %q)", err, got)
	}
	if string(got) != want {
		t.Fatalf("responses: got %q, want %q", got, want)
	}

	positions := env.positions.all()
	if len(positions) != 2 {
		t.Fatalf("stored %d positions, want 2: %+v", len(positions), positions)
	}
	ud := positions[0]
	if !ud.Valid || ud.Latitude != 36.815109 || ud.Longitude != 10.1792312 || ud.Outdated {
		t.Errorf("UD position: %+v", ud)
	}
	if want := time.Date(2023, 7, 19, 19, 7, 7, 0, time.UTC); !ud.Timestamp.Equal(want) {
		t.Errorf("UD timestamp: got %v, want %v", ud.Timestamp, want)
	}
	lk := positions[1]
	if !lk.Outdated || lk.Latitude != ud.Latitude || lk.Longitude != ud.Longitude {
		t.Errorf("LK position should reuse last location: %+v", lk)
	}
	if lk.Attributes["batteryLevel"] != 53 {
		t.Errorf("LK battery: got %v", lk.Attributes["batteryLevel"])
	}

	if d := env.devices.get("9705141740"); d.Status != "online" || d.LastUpdate == nil {
		t.Errorf("device should be online: %+v", d)
	}
}

func TestWatchServer_HandleConnectionUsesWatchFraming(t *testing.T) {
	srv := NewWatchServer("0", nil, nil)
	if srv.scannerSplit == nil || !srv.rawFrames || srv.maxFrameSize < 64*1024 {
		t.Errorf("watch server framing not configured: split=%v raw=%v max=%d",
			srv.scannerSplit != nil, srv.rawFrames, srv.maxFrameSize)
	}
}

func TestDecodeWatch_HeartbeatMarksKnownDeviceOnline(t *testing.T) {
	env := newWatchTestEnv(t, "8800000015")

	pos, devID, resp, err := env.srv.decodeWatch(context.Background(), "[SG*8800000015*0002*LK]")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pos != nil {
		t.Errorf("expected no position, got %+v", pos)
	}
	if devID != "8800000015" || resp != "[SG*8800000015*0002*LK]" {
		t.Errorf("got devID %q resp %q", devID, resp)
	}
	if d := env.devices.get("8800000015"); d.Status != "online" || d.LastUpdate == nil {
		t.Errorf("device should be online: %+v", d)
	}
}

func TestDecodeWatch_HeartbeatWithBattery(t *testing.T) {
	t.Run("with last location", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		last := env.seedPosition(t, 48.1, 11.5)

		pos, _, resp, err := env.srv.decodeWatch(context.Background(), "[3G*4700186508*000B*LK,0,10,100]")
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp != "[3G*4700186508*0002*LK]" {
			t.Errorf("resp: %q", resp)
		}
		if pos == nil {
			t.Fatal("expected position at last location")
		}
		if !pos.Outdated || pos.Latitude != 48.1 || pos.Longitude != 11.5 || !pos.Timestamp.Equal(last.Timestamp) {
			t.Errorf("position: %+v", pos)
		}
		if pos.Attributes["batteryLevel"] != 100 || pos.Attributes["steps"] != 0 {
			t.Errorf("attributes: %v", pos.Attributes)
		}
		if pos.DeviceID != env.device.ID || pos.Protocol != "watch" || pos.ServerTime == nil || pos.DeviceTime == nil {
			t.Errorf("metadata: %+v", pos)
		}
	})

	t.Run("without last location", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")

		pos, _, _, err := env.srv.decodeWatch(context.Background(), "[3G*4700186508*000B*LK,0,10,100]")
		if err != nil || pos != nil {
			t.Fatalf("expected no position, got %+v / %v", pos, err)
		}
		if d := env.devices.get("4700186508"); d.Status != "online" {
			t.Errorf("device should be online: %+v", d)
		}
	})
}

func TestDecodeWatch_NoFixPositions(t *testing.T) {
	const noFixUD = "[3G*4700222306*0077*UD,120316,140610,V,0.000000,N,0.000000,E,0.00,0.0,0.0,0,25,83,0,0,00000000,2,255,262,1,21041,9067,121,21041,5981,116]"
	const noFixSOS = "[3G*4700222306*003e*AL_LTE,170525,214118,V,0,N,0,E,0,0,0,0,0,22,0,0,00010000,0,0,0]"

	t.Run("0,0 uses last location", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700222306")
		env.seedPosition(t, 48.779045, 9.1574736)

		pos, _, _, err := env.srv.decodeWatch(context.Background(), noFixUD)
		if err != nil || pos == nil {
			t.Fatalf("expected position, got %+v / %v", pos, err)
		}
		if !pos.Outdated || pos.Latitude != 48.779045 || pos.Longitude != 9.1574736 {
			t.Errorf("position should be at last location: %+v", pos)
		}
		if pos.Attributes["batteryLevel"] != 83 || pos.Attributes["mcc"] != 262 {
			t.Errorf("fresh attributes lost: %v", pos.Attributes)
		}
		if pos.Network == nil {
			t.Error("network lost")
		}
	})

	t.Run("0,0 without last location is not stored", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700222306")

		pos, _, _, err := env.srv.decodeWatch(context.Background(), noFixUD)
		if err != nil || pos != nil {
			t.Fatalf("expected no position, got %+v / %v", pos, err)
		}
		if d := env.devices.get("4700222306"); d.Status != "online" {
			t.Errorf("device should be online: %+v", d)
		}
	})

	t.Run("0,0 alarm without last location is stored", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700222306")

		pos, _, resp, err := env.srv.decodeWatch(context.Background(), noFixSOS)
		if err != nil || pos == nil {
			t.Fatalf("expected position, got %+v / %v", pos, err)
		}
		if pos.Attributes["alarm"] != "sos" || resp != "[3G*4700222306*0002*AL]" {
			t.Errorf("alarm %v resp %q", pos.Attributes["alarm"], resp)
		}
	})

	t.Run("invalid fix with coordinates is stored as-is", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700222306")

		pos, _, _, err := env.srv.decodeWatch(context.Background(),
			"[3G*4700222306*0077*UD,120316,140610,V,48.779045,N, 9.1574736,E,0.00,0.0,0.0,0,25,83,0,0,00000000,0]")
		if err != nil || pos == nil {
			t.Fatalf("expected position, got %+v / %v", pos, err)
		}
		if pos.Valid || pos.Outdated || pos.Latitude != 48.779045 || pos.Longitude != 9.1574736 {
			t.Errorf("position: %+v", pos)
		}
	})
}

func TestDecodeWatch_AlarmWithUnparseableContentIsAcknowledged(t *testing.T) {
	env := newWatchTestEnv(t, "1234567890")

	pos, devID, resp, err := env.srv.decodeWatch(context.Background(), "[3G*1234567890*0006*AL,foo]")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if pos != nil || devID != "1234567890" || resp != "[3G*1234567890*0002*AL]" {
		t.Errorf("got pos %+v devID %q resp %q", pos, devID, resp)
	}
}

func TestDecodeWatch_UnknownDeviceWithoutAutoCreate(t *testing.T) {
	env := newWatchTestEnv(t, "1111111111")

	// Position reports from unknown devices are rejected.
	_, devID, _, err := env.srv.decodeWatch(context.Background(),
		"[3G*2222222222*004E*UD2,220322,055105,A,22.761162,N,114.360192,E,0,0,47,14,100,64,0,0,00000008,0,0]")
	if err == nil || devID != "2222222222" {
		t.Errorf("expected unknown device error, got devID %q err %v", devID, err)
	}

	// Heartbeats are still acknowledged, without creating anything.
	pos, _, resp, err := env.srv.decodeWatch(context.Background(), "[3G*2222222222*0002*LK]")
	if err != nil || pos != nil || resp != "[3G*2222222222*0002*LK]" {
		t.Errorf("got pos %+v resp %q err %v", pos, resp, err)
	}
	if env.devices.get("2222222222") != nil {
		t.Error("heartbeat must not create a device")
	}
}

func TestRelay_WatchForwardsFramesVerbatim(t *testing.T) {
	relayAddr, relayLines := startMockRelay(t)

	srv := &Server{
		name:         "watch",
		port:         "0",
		relayTarget:  relayAddr,
		decoder:      noopDecoder(nil),
		scannerSplit: NewWatchServer("0", nil, nil).scannerSplit,
		rawFrames:    true,
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	go srv.acceptLoop(t.Context())

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	// A trailing newline lets the line-based mock relay emit what it got.
	if _, err := io.WriteString(conn, "[3G*1*0002*LK][3G*1*0003*TKQ]\n"); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Response ACKs must not be CRLF terminated.
	got := make([]byte, 6)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil || string(got) != "ACKACK" {
		t.Fatalf("responses: got %q err %v", got, err)
	}

	// Close the device conn so the relay conn is closed and the mock flushes.
	_ = conn.Close()
	select {
	case line := <-relayLines:
		if line != "[3G*1*0002*LK][3G*1*0003*TKQ]" {
			t.Errorf("relayed %q, want frames without CRLF", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for relay")
	}
}
