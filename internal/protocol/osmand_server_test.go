package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/websocket"
)

// osmandDeviceRepo is an in-memory DeviceRepo for OsmAnd server tests.
// Methods not used by the server panic via the nil embedded interface.
type osmandDeviceRepo struct {
	repository.DeviceRepo
	mu      sync.Mutex
	nextID  int64
	devices map[string]*model.Device
}

func (r *osmandDeviceRepo) add(d *model.Device) *model.Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	d.ID = r.nextID
	r.devices[d.UniqueID] = d
	return d
}

func (r *osmandDeviceRepo) GetByUniqueID(_ context.Context, uniqueID string) (*model.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.devices[uniqueID]; ok {
		c := *d
		return &c, nil
	}
	return nil, fmt.Errorf("device %s not found", uniqueID)
}

func (r *osmandDeviceRepo) GetByID(_ context.Context, id int64) (*model.Device, error) {
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

func (r *osmandDeviceRepo) Create(_ context.Context, d *model.Device, _ int64) error {
	r.add(d)
	return nil
}

func (r *osmandDeviceRepo) Update(_ context.Context, d *model.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *d
	r.devices[d.UniqueID] = &c
	return nil
}

func (r *osmandDeviceRepo) UpdateProtocol(_ context.Context, id int64, protocol string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.ID == id {
			d.Protocol = protocol
		}
	}
	return nil
}

func (r *osmandDeviceRepo) get(uniqueID string) *model.Device {
	d, err := r.GetByUniqueID(context.Background(), uniqueID)
	if err != nil {
		return nil
	}
	return d
}

// osmandPositionRepo is an in-memory PositionRepo for OsmAnd server tests.
type osmandPositionRepo struct {
	repository.PositionRepo
	mu        sync.Mutex
	positions []*model.Position
}

func (r *osmandPositionRepo) Create(_ context.Context, p *model.Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = int64(len(r.positions) + 1)
	r.positions = append(r.positions, p)
	return nil
}

func (r *osmandPositionRepo) GetLatestByDevice(_ context.Context, deviceID int64) (*model.Position, error) {
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

func (r *osmandPositionRepo) all() []*model.Position {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*model.Position(nil), r.positions...)
}

// osmandUserRepo resolves the default user for device auto-creation.
type osmandUserRepo struct{ repository.UserRepo }

func (osmandUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	return &model.User{ID: 1, Email: email}, nil
}

type osmandTestEnv struct {
	srv       *OsmAndServer
	devices   *osmandDeviceRepo
	positions *osmandPositionRepo
}

// newOsmAndTestEnv creates an OsmAnd server with one known, offline device "123456".
func newOsmAndTestEnv(t *testing.T) *osmandTestEnv {
	t.Helper()
	devices := &osmandDeviceRepo{devices: map[string]*model.Device{}}
	devices.add(&model.Device{UniqueID: "123456", Name: "Phone", Protocol: "osmand", Status: "offline"})
	positions := &osmandPositionRepo{}
	hub := websocket.NewHub(nil, nil, func(*http.Request) int64 { return 0 })
	handler := NewPositionHandler(positions, devices, hub, nil)
	return &osmandTestEnv{
		srv:       NewOsmAndServer("0", devices, handler),
		devices:   devices,
		positions: positions,
	}
}

func (e *osmandTestEnv) do(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.srv.ServeHTTP(rec, req)
	return rec
}

func TestOsmAndServer_QueryGET(t *testing.T) {
	env := newOsmAndTestEnv(t)

	rec := env.do(t, httptest.NewRequest(http.MethodGet,
		"/?id=123456&timestamp=1504763810&lat=40.7232948571&lon=-74.0061408571&bearing=7.2&speed=40&altitude=12&accuracy=8&batt=87&ignition=true", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
	}

	positions := env.positions.all()
	if len(positions) != 1 {
		t.Fatalf("stored %d positions, want 1", len(positions))
	}
	p := positions[0]
	if p.Protocol != "osmand" || p.DeviceID != 1 || !p.Valid || p.Outdated {
		t.Errorf("position: %+v", p)
	}
	if want := time.Date(2017, 9, 7, 5, 56, 50, 0, time.UTC); !p.Timestamp.Equal(want) || p.DeviceTime == nil || !p.DeviceTime.Equal(want) || p.ServerTime == nil {
		t.Errorf("times: fix %v device %v server %v", p.Timestamp, p.DeviceTime, p.ServerTime)
	}
	if p.Latitude != 40.7232948571 || p.Longitude != -74.0061408571 {
		t.Errorf("location: %f,%f", p.Latitude, p.Longitude)
	}
	if p.Speed == nil || *p.Speed != 40*1.852 || p.Course == nil || *p.Course != 7.2 ||
		p.Altitude == nil || *p.Altitude != 12 || p.Accuracy != 8 {
		t.Errorf("speed/course/alt/acc: %v %v %v %v", p.Speed, p.Course, p.Altitude, p.Accuracy)
	}
	if p.Attributes["batteryLevel"] != 87.0 || p.Attributes["ignition"] != true {
		t.Errorf("attributes: %v", p.Attributes)
	}
	if d := env.devices.get("123456"); d.Status != "online" || d.LastUpdate == nil {
		t.Errorf("device should be online: %+v", d)
	}
}

func TestOsmAndServer_QueryFormPOST(t *testing.T) {
	env := newOsmAndTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("id=123456&lat=60.0&lon=30.0&timestamp=1377177267"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if rec := env.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
	}
	if positions := env.positions.all(); len(positions) != 1 || positions[0].Latitude != 60 || positions[0].Longitude != 30 {
		t.Errorf("positions: %+v", positions)
	}
}

func TestOsmAndServer_QueryPOSTWithURLParams(t *testing.T) {
	// Traccar Client (older versions) POSTs with the parameters in the URL.
	env := newOsmAndTestEnv(t)

	if rec := env.do(t, httptest.NewRequest(http.MethodPost, "/?id=123456&lat=60.0&lon=30.0", nil)); rec.Code != http.StatusOK {
		t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
	}
	if n := len(env.positions.all()); n != 1 {
		t.Errorf("stored %d positions, want 1", n)
	}
}

func TestOsmAndServer_JSON(t *testing.T) {
	env := newOsmAndTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"location":{"event":"motionchange","timestamp":"2021-07-21T08:06:34.444Z","coords":{"latitude":-6.1148096,"longitude":106.6837015,"accuracy":3.8,"speed":10,"heading":63,"altitude":35.7},"battery":{"is_charging":false,"level":0.79}},"device_id":"123456"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if rec := env.do(t, req); rec.Code != http.StatusOK {
		t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
	}

	positions := env.positions.all()
	if len(positions) != 1 {
		t.Fatalf("stored %d positions, want 1", len(positions))
	}
	p := positions[0]
	if p.Latitude != -6.1148096 || p.Speed == nil || *p.Speed != 36 || p.Attributes["batteryLevel"] != 79 {
		t.Errorf("position: %+v speed %v", p, p.Speed)
	}
}

func TestOsmAndServer_UnknownDevice(t *testing.T) {
	env := newOsmAndTestEnv(t)

	// Traccar answers 400 for the query format and 404 for JSON.
	if rec := env.do(t, httptest.NewRequest(http.MethodGet, "/?id=999&lat=1&lon=2", nil)); rec.Code != http.StatusBadRequest {
		t.Errorf("query: got %d, want 400", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"device_id":"999","location":{"timestamp":"2021-07-21T08:06:34Z","coords":{"latitude":1,"longitude":2}}}`))
	req.Header.Set("Content-Type", "application/json")
	if rec := env.do(t, req); rec.Code != http.StatusNotFound {
		t.Errorf("json: got %d, want 404", rec.Code)
	}
	if n := len(env.positions.all()); n != 0 {
		t.Errorf("stored %d positions for unknown devices", n)
	}
	if env.devices.get("999") != nil {
		t.Error("device must not be created without auto-create")
	}
}

func TestOsmAndServer_AutoCreate(t *testing.T) {
	env := newOsmAndTestEnv(t)
	env.srv.SetAutoCreate(AutoCreateConfig{Enabled: true, DefaultUserEmail: "admin@motus.local"}, osmandUserRepo{})

	if rec := env.do(t, httptest.NewRequest(http.MethodGet, "/?id=555&lat=1&lon=2", nil)); rec.Code != http.StatusOK {
		t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
	}
	d := env.devices.get("555")
	if d == nil || d.Protocol != "osmand" {
		t.Fatalf("device not auto-created: %+v", d)
	}
	if positions := env.positions.all(); len(positions) != 1 || positions[0].DeviceID != d.ID {
		t.Errorf("positions: %+v", positions)
	}
}

func TestOsmAndServer_BadRequests(t *testing.T) {
	env := newOsmAndTestEnv(t)

	jsonReq := func(body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	for name, req := range map[string]*http.Request{
		"missing id":     httptest.NewRequest(http.MethodGet, "/?lat=60.0&lon=30.0", nil),
		"no parameters":  httptest.NewRequest(http.MethodGet, "/", nil),
		"bad latitude":   httptest.NewRequest(http.MethodGet, "/?id=123456&lat=abc&lon=1", nil),
		"bad json":       jsonReq(`{`),
		"json no device": jsonReq(`{"location":{"timestamp":"2021-07-21T08:06:34Z"}}`),
	} {
		if rec := env.do(t, req); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, rec.Code)
		}
	}
	if n := len(env.positions.all()); n != 0 {
		t.Errorf("stored %d positions for bad requests", n)
	}
}

func TestOsmAndServer_MethodNotAllowed(t *testing.T) {
	env := newOsmAndTestEnv(t)

	rec := env.do(t, httptest.NewRequest(http.MethodPut, "/?id=123456&lat=1&lon=2", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Errorf("got %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestOsmAndServer_BodyTooLarge(t *testing.T) {
	env := newOsmAndTestEnv(t)

	body := "id=123456&lat=1&lon=2&pad=" + strings.Repeat("x", osmandMaxBodySize)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if rec := env.do(t, req); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got %d, want 413", rec.Code)
	}
}

func TestOsmAndServer_WithoutCoordinates(t *testing.T) {
	t.Run("uses last location", func(t *testing.T) {
		env := newOsmAndTestEnv(t)
		speed := 5.0
		last := &model.Position{DeviceID: 1, Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Valid: true, Latitude: 48.1, Longitude: 11.5, Speed: &speed, Accuracy: 4}
		_ = env.positions.Create(context.Background(), last)

		if rec := env.do(t, httptest.NewRequest(http.MethodGet, "/?id=123456&timestamp=1790000000&batt=50", nil)); rec.Code != http.StatusOK {
			t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
		}
		positions := env.positions.all()
		if len(positions) != 2 {
			t.Fatalf("stored %d positions, want 2", len(positions))
		}
		p := positions[1]
		if !p.Outdated || p.Latitude != 48.1 || p.Longitude != 11.5 || !p.Timestamp.Equal(last.Timestamp) || p.Accuracy != 4 {
			t.Errorf("position should reuse last location: %+v", p)
		}
		if p.DeviceTime == nil || !p.DeviceTime.Equal(time.Unix(1790000000, 0)) {
			t.Errorf("device time: %v", p.DeviceTime)
		}
		if p.Attributes["batteryLevel"] != 50.0 {
			t.Errorf("attributes: %v", p.Attributes)
		}
	})

	t.Run("without last location", func(t *testing.T) {
		env := newOsmAndTestEnv(t)

		if rec := env.do(t, httptest.NewRequest(http.MethodGet, "/?id=123456&batt=50", nil)); rec.Code != http.StatusOK {
			t.Fatalf("response: %d %q", rec.Code, rec.Body.String())
		}
		if n := len(env.positions.all()); n != 0 {
			t.Errorf("stored %d positions, want 0", n)
		}
	})
}

// failingPositionRepo fails every write, so the app is told to retry.
type failingPositionRepo struct{ osmandPositionRepo }

func (*failingPositionRepo) Create(context.Context, *model.Position) error {
	return fmt.Errorf("database down")
}

func TestOsmAndServer_StorageErrorAsksForRetry(t *testing.T) {
	env := newOsmAndTestEnv(t)
	hub := websocket.NewHub(nil, nil, func(*http.Request) int64 { return 0 })
	env.srv = NewOsmAndServer("0", env.devices, NewPositionHandler(&failingPositionRepo{}, env.devices, hub, nil))

	if rec := env.do(t, httptest.NewRequest(http.MethodGet, "/?id=123456&lat=1&lon=2", nil)); rec.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500", rec.Code)
	}
}

func TestOsmAndServer_StartAndShutdown(t *testing.T) {
	// Reserve a free port for the server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	env := newOsmAndTestEnv(t)
	env.srv = NewOsmAndServer(port, env.devices, env.srv.handler)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- env.srv.Start(ctx) }()

	target := "http://127.0.0.1:" + port + "/?" + url.Values{"id": {"123456"}, "lat": {"1"}, "lon": {"2"}}.Encode()
	var resp *http.Response
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err = http.Get(target)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}
