package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/api/handlers"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// minimalGPX builds a valid GPX XML document with n trackpoints.
// Points are spaced 1 second and ~100 m apart starting at Berlin.
func minimalGPX(n int) string {
	base := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><trkseg>`
	ts := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		lat := 52.520000 + float64(i)*0.0009 // ~100 m north per step
		base += fmt.Sprintf(`
    <trkpt lat="%f" lon="13.404954">
      <ele>35.0</ele>
      <time>%s</time>
    </trkpt>`, lat, ts.Add(time.Duration(i)*time.Second).Format(time.RFC3339))
	}
	base += `
  </trkseg></trk>
</gpx>`
	return base
}

// gpxWithUntimed returns a GPX file with one timed and one untimed point.
func gpxWithUntimed() string {
	ts := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><trkseg>
    <trkpt lat="52.52" lon="13.40">
      <ele>35.0</ele>
      <time>%s</time>
    </trkpt>
    <trkpt lat="52.521" lon="13.401">
      <ele>35.0</ele>
    </trkpt>
  </trkseg></trk>
</gpx>`, ts.Format(time.RFC3339))
}

// makeMultipartGPX creates a multipart request body with the given GPX content in a "file" field.
func makeMultipartGPX(t *testing.T, gpxContent string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "track.gpx")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte(gpxContent)); err != nil {
		t.Fatalf("write gpx: %v", err)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func setupGPXHandler(t *testing.T) (*handlers.GPXImportHandler, *repository.DeviceRepository, *repository.PositionRepository, *model.User) {
	t.Helper()
	pool := testutil.SetupTestDB(t)
	testutil.CleanTables(t, pool)

	userRepo := repository.NewUserRepository(pool)
	deviceRepo := repository.NewDeviceRepository(pool)
	posRepo := repository.NewPositionRepository(pool)

	user := &model.User{Email: "gpxhandler@example.com", PasswordHash: "$2a$10$hash", Name: "GPX Handler"}
	if err := userRepo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := handlers.NewGPXImportHandler(deviceRepo, posRepo, nil)
	return h, deviceRepo, posRepo, user
}

func TestGPXHandler_Import_InvalidDeviceID(t *testing.T) {
	h, _, _, user := setupGPXHandler(t)

	body, ct := makeMultipartGPX(t, minimalGPX(2))
	req := httptest.NewRequest(http.MethodPost, "/api/devices/bad/gpx", body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user)
	req = withChiParam(req, "id", "bad")
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGPXHandler_Import_AccessDenied(t *testing.T) {
	h, deviceRepo, _, user := setupGPXHandler(t)
	ctx := context.Background()

	// Create a device owned by a different user.
	otherUser := &model.User{Email: "other@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	_ = userRepo.Create(ctx, otherUser)
	device := &model.Device{UniqueID: "gpx-other-dev", Name: "Other Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, otherUser.ID)

	body, ct := makeMultipartGPX(t, minimalGPX(2))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user) // user != device owner
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGPXHandler_Import_NoFileField(t *testing.T) {
	h, deviceRepo, _, user := setupGPXHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-nofile-dev", Name: "GPX Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// Send multipart without a "file" field.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withUser(req, user)
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGPXHandler_Import_InvalidGPXXML(t *testing.T) {
	h, deviceRepo, _, user := setupGPXHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-badxml-dev", Name: "GPX Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	body, ct := makeMultipartGPX(t, "this is not xml")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user)
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGPXHandler_Import_NoTimedPoints(t *testing.T) {
	h, deviceRepo, _, user := setupGPXHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-notimed-dev", Name: "GPX Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// GPX with only untimed points.
	gpx := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><trkseg>
    <trkpt lat="52.52" lon="13.40"><ele>35.0</ele></trkpt>
  </trkseg></trk>
</gpx>`

	body, ct := makeMultipartGPX(t, gpx)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user)
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for no timed points, got %d", rr.Code)
	}
}

func TestGPXHandler_Import_Success(t *testing.T) {
	h, deviceRepo, posRepo, user := setupGPXHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-ok-dev", Name: "GPX Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	// 3-point track with spacing for speed calculation.
	body, ct := makeMultipartGPX(t, minimalGPX(3))
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user)
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]int
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["imported"] != 3 {
		t.Errorf("expected imported=3, got %d", result["imported"])
	}
	if result["skipped"] != 0 {
		t.Errorf("expected skipped=0, got %d", result["skipped"])
	}

	// Verify positions were persisted (use a wide range since GPX timestamps
	// may differ from current time).
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	positions, err := posRepo.GetByDeviceAndTimeRange(ctx, device.ID, from, to, 10)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 3 {
		t.Errorf("expected 3 positions in DB, got %d", len(positions))
	}
}

func TestGPXHandler_Import_WithUntimedPoints(t *testing.T) {
	h, deviceRepo, _, user := setupGPXHandler(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-mix-dev", Name: "GPX Device", Status: "online"}
	_ = deviceRepo.Create(ctx, device, user.ID)

	body, ct := makeMultipartGPX(t, gpxWithUntimed())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/devices/%d/gpx", device.ID), body)
	req.Header.Set("Content-Type", ct)
	req = withUser(req, user)
	req = withChiParam(req, "id", fmt.Sprintf("%d", device.ID))
	rr := httptest.NewRecorder()

	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var result map[string]int
	_ = json.NewDecoder(rr.Body).Decode(&result)
	if result["imported"] != 1 {
		t.Errorf("expected imported=1, got %d", result["imported"])
	}
	if result["skipped"] != 1 {
		t.Errorf("expected skipped=1, got %d", result["skipped"])
	}
}
