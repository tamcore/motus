package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
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

func TestDecodeWatch_HealthMeasurements(t *testing.T) {
	t.Run("with last location", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700609403")
		last := env.seedPosition(t, 48.1, 11.5)

		before := time.Now().UTC()
		pos, _, resp, err := env.srv.decodeWatch(context.Background(), "[3G*4700609403*0013*bphrt,120,79,73,,,,]")
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp != "" {
			t.Errorf("health messages are not acknowledged, got %q", resp)
		}
		if pos == nil {
			t.Fatal("expected position at last location")
		}
		if !pos.Outdated || pos.Latitude != 48.1 || pos.Longitude != 11.5 || !pos.Timestamp.Equal(last.Timestamp) {
			t.Errorf("position: %+v", pos)
		}
		// Measurements are timestamped with the time they were received.
		if pos.DeviceTime == nil || pos.DeviceTime.Before(before) {
			t.Errorf("device time: %v", pos.DeviceTime)
		}
		want := map[string]any{"pressureHigh": "120", "pressureLow": "79", "heartRate": 73}
		for k, v := range want {
			if pos.Attributes[k] != v {
				t.Errorf("%s: got %v, want %v", k, pos.Attributes[k], v)
			}
		}
	})

	t.Run("without last location", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700609403")

		pos, _, _, err := env.srv.decodeWatch(context.Background(), "[CS*4700609403*0008*PULSE,72]")
		if err != nil || pos != nil {
			t.Fatalf("expected no position, got %+v / %v", pos, err)
		}
		if d := env.devices.get("4700609403"); d.Status != "online" {
			t.Errorf("device should be online: %+v", d)
		}
	})
}

// TestWatchServer_VoiceChunksAcknowledged sends escaped binary voice chunks
// over TCP. Each chunk must be acknowledged and the connection kept alive.
func TestWatchServer_VoiceChunksAcknowledged(t *testing.T) {
	env := newWatchTestEnv(t, "789468050042692")

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

	audio := strings.Repeat("\x0c\x0a<?\x96}\x04\xd9}\x02}\x03\xff", 2000) // ~30 KB, beyond the H02 buffer
	input := "[ZJ*789468050042692*0034*0439*JXTK,0,watch_7_20220526093954,1,2,#!AMR\n" + audio + "]" +
		"[ZJ*789468050042692*0035*0439*JXTK,0,watch_7_20220526093954,2,2," + audio + "]" +
		"[ZJ*789468050042692*0036*0009*LK,0,0,19]"
	if _, err := io.WriteString(conn, input); err != nil {
		t.Fatalf("write: %v", err)
	}

	want := "[ZJ*789468050042692*0034*0007*JXTKR,1]" +
		"[ZJ*789468050042692*0035*0007*JXTKR,1]" +
		"[ZJ*789468050042692*0036*0002*LK]"
	got := make([]byte, len(want))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read responses: %v (got %q)", err, got)
	}
	if string(got) != want {
		t.Fatalf("responses: got %q, want %q", got, want)
	}
	if n := len(env.positions.all()); n != 0 {
		t.Errorf("voice chunks must not store positions, got %d", n)
	}
}

// TestWatchServer_RegistersConnectionForCommands verifies that a WATCH device
// connection is registered for command dispatch together with its session
// (manufacturer and frame indexing), and that commands reach the device.
func TestWatchServer_RegistersConnectionForCommands(t *testing.T) {
	for _, tt := range []struct {
		name, frame string
		session     DeviceSession
		command     string
	}{
		{"3G", "[3G*4700186508*0002*LK]", DeviceSession{Manufacturer: "3G"}, "[SG*4700186508*0002*CR]"},
		{"indexed ZJ", "[ZJ*4700186508*0034*0009*LK,0,0,19]", DeviceSession{Manufacturer: "ZJ", Indexed: true}, "[ZJ*4700186508*0001*0002*CR]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := newWatchTestEnv(t, "4700186508")
			registry := NewDeviceRegistry()
			env.srv.SetRegistry(registry)

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
			if _, err := io.WriteString(conn, tt.frame); err != nil {
				t.Fatalf("write: %v", err)
			}
			ack := make([]byte, len("[..*4700186508*0002*LK]"))
			if strings.Contains(tt.frame, "*0034*") {
				ack = make([]byte, len("[ZJ*4700186508*0034*0002*LK]"))
			}
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, err := io.ReadFull(conn, ack); err != nil {
				t.Fatalf("read ack: %v", err)
			}

			session, ok := registry.Session("4700186508")
			if !registry.IsOnline("4700186508") || !ok || session != tt.session {
				t.Fatalf("registered=%v session=%+v (%v), want %+v", registry.IsOnline("4700186508"), session, ok, tt.session)
			}

			payload, err := NewWatchCommandEncoder(registry).EncodeCommand(&model.Command{Type: model.CommandPositionSingle}, "4700186508")
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !registry.Send("4700186508", payload) {
				t.Fatal("send failed")
			}
			got := make([]byte, len(tt.command))
			if _, err := io.ReadFull(conn, got); err != nil || string(got) != tt.command {
				t.Fatalf("command: got %q err %v, want %q", got, err, tt.command)
			}
		})
	}
}

// memCommandRepo is an in-memory CommandRepo for WATCH reply tests.
type memCommandRepo struct {
	repository.CommandRepo
	mu       sync.Mutex
	commands []*model.Command // oldest first
	listed   int              // ListByDevice calls
}

func (r *memCommandRepo) ListByDevice(_ context.Context, deviceID int64, limit int) ([]*model.Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listed++
	var out []*model.Command
	for i := len(r.commands) - 1; i >= 0 && len(out) < limit; i-- {
		if r.commands[i].DeviceID == deviceID {
			c := *r.commands[i]
			out = append(out, &c)
		}
	}
	return out, nil
}

func (r *memCommandRepo) AppendResult(_ context.Context, id int64, chunk string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if c.ID == id {
			if c.Result == nil {
				c.Result = &chunk
				c.Status = "executed"
			} else {
				joined := *c.Result + "\n" + chunk
				c.Result = &joined
			}
		}
	}
	return nil
}

func (r *memCommandRepo) get(id int64) *model.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.commands {
		if c.ID == id {
			cp := *c
			return &cp
		}
	}
	return nil
}

func TestWatchCommandKeyword(t *testing.T) {
	tests := []struct {
		cmd  *model.Command
		want string
	}{
		{&model.Command{Type: model.CommandPositionPeriodic, Attributes: map[string]any{"frequency": 60}}, "UPLOAD"},
		{&model.Command{Type: model.CommandPositionSingle}, "CR"},
		{&model.Command{Type: model.CommandRebootDevice}, "RESET"},
		{&model.Command{Type: model.CommandSosNumber, Attributes: map[string]any{"phoneNumber": "123"}}, "SOS1"},
		{&model.Command{Type: model.CommandSosNumber, Attributes: map[string]any{"phoneNumber": "123", "index": 2}}, "SOS2"},
		{&model.Command{Type: model.CommandCustom, Attributes: map[string]any{"text": "LZ,1,+1"}}, "LZ"},
		{&model.Command{Type: model.CommandCustom, Attributes: map[string]any{"text": "POWEROFF"}}, "POWEROFF"},
		{&model.Command{Type: model.CommandSetSpeedAlarm}, ""},
		{&model.Command{Type: model.CommandPositionPeriodic}, ""},
	}
	for _, tt := range tests {
		if got := watchCommandKeyword(tt.cmd); got != tt.want {
			t.Errorf("%s %v: got %q, want %q", tt.cmd.Type, tt.cmd.Attributes, got, tt.want)
		}
	}
}

func TestDecodeWatch_RecordsCommandReply(t *testing.T) {
	sent := func(id int64, deviceID int64, typ string, attrs map[string]any) *model.Command {
		return &model.Command{ID: id, DeviceID: deviceID, Type: typ, Attributes: attrs, Status: model.CommandStatusSent}
	}

	t.Run("reply to set reporting interval", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		cmds := &memCommandRepo{commands: []*model.Command{
			sent(1, env.device.ID, model.CommandPositionPeriodic, map[string]any{"frequency": 60}),
		}}
		env.srv.SetCommandRepo(cmds)

		pos, _, resp, err := env.srv.decodeWatch(context.Background(), "[3G*4700186508*0006*UPLOAD]")
		if err != nil || pos != nil || resp != "" {
			t.Fatalf("got pos %+v resp %q err %v", pos, resp, err)
		}
		c := cmds.get(1)
		if c.Status != "executed" || c.Result == nil || *c.Result != "UPLOAD" {
			t.Errorf("command: status %q result %v", c.Status, c.Result)
		}
		if d := env.devices.get("4700186508"); d.Status != "online" {
			t.Errorf("device should be online: %+v", d)
		}
	})

	t.Run("reply content and case-insensitive keyword", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		cmds := &memCommandRepo{commands: []*model.Command{
			sent(1, env.device.ID, model.CommandCustom, map[string]any{"text": "PowerOff"}),
		}}
		env.srv.SetCommandRepo(cmds)

		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*4700186508*000a*POWEROFF,1]")
		if c := cmds.get(1); c.Result == nil || *c.Result != "POWEROFF,1" {
			t.Errorf("result: %v", c.Result)
		}
	})

	t.Run("matches the newest sent command with the same keyword", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		cmds := &memCommandRepo{commands: []*model.Command{
			sent(1, env.device.ID, model.CommandPositionPeriodic, map[string]any{"frequency": 300}),
			sent(2, env.device.ID, model.CommandPositionPeriodic, map[string]any{"frequency": 60}),
			sent(3, env.device.ID, model.CommandPositionSingle, nil),
		}}
		env.srv.SetCommandRepo(cmds)

		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*4700186508*0006*UPLOAD]")
		if c := cmds.get(2); c.Result == nil {
			t.Error("newest UPLOAD command should get the reply")
		}
		if c := cmds.get(1); c.Result != nil {
			t.Error("older UPLOAD command must not get the reply")
		}
		if c := cmds.get(3); c.Result != nil {
			t.Error("CR command must not get an UPLOAD reply")
		}
	})

	t.Run("ignores executed, pending and unrelated commands", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		executed := "RESET"
		cmds := &memCommandRepo{commands: []*model.Command{
			{ID: 1, DeviceID: env.device.ID, Type: model.CommandRebootDevice, Status: "executed", Result: &executed},
			{ID: 2, DeviceID: env.device.ID, Type: model.CommandRebootDevice, Status: model.CommandStatusPending},
			sent(3, env.device.ID, model.CommandPositionSingle, nil),
		}}
		env.srv.SetCommandRepo(cmds)

		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*4700186508*0005*RESET]")
		if c := cmds.get(1); *c.Result != "RESET" {
			t.Errorf("executed command changed: %v", *c.Result)
		}
		for _, id := range []int64{2, 3} {
			if c := cmds.get(id); c.Result != nil {
				t.Errorf("command %d must not get the reply: %v", id, *c.Result)
			}
		}
	})

	t.Run("device messages do not look up commands", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		cmds := &memCommandRepo{commands: []*model.Command{
			sent(1, env.device.ID, model.CommandCustom, map[string]any{"text": "LK"}),
		}}
		env.srv.SetCommandRepo(cmds)

		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*4700186508*0002*LK]")
		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*4700186508*0003*TKQ]")
		if cmds.listed != 0 || cmds.get(1).Result != nil {
			t.Errorf("heartbeats must not be treated as replies (listed %d)", cmds.listed)
		}
	})

	t.Run("unknown device and missing command repo", func(t *testing.T) {
		env := newWatchTestEnv(t, "4700186508")
		// No command repository configured: replies are ignored.
		if _, _, _, err := env.srv.decodeWatch(context.Background(), "[3G*4700186508*0006*UPLOAD]"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		cmds := &memCommandRepo{}
		env.srv.SetCommandRepo(cmds)
		_, _, _, _ = env.srv.decodeWatch(context.Background(), "[3G*9999999999*0006*UPLOAD]")
		if cmds.listed != 0 {
			t.Error("replies from unknown devices must not look up commands")
		}
	})
}
