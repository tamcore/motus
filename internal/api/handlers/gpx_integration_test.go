package handlers_test

// Integration tests for the live ogen Handler.ImportGPX endpoint. Ported
// from the deleted chi GPXImportHandler tests.
//
// Dropped tests (no live equivalent):
//   - TestGPXHandler_Import_BodyTooLarge: request size limiting is enforced
//     by the router-level limitRequestBody middleware, not the handler.
//   - TestGPXHandler_Import_InvalidDeviceID: ogen decodes the typed int64
//     path param; invalid IDs are rejected before the handler runs.

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	ht "github.com/ogen-go/ogen/http"
	"github.com/tamcore/motus/internal/api"
	"github.com/tamcore/motus/internal/api/handlers"
	oas "github.com/tamcore/motus/internal/api/oas"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// gpxTrackStart is the timestamp of the first generated trackpoint.
var gpxTrackStart = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

// minimalGPX builds a valid GPX XML document with n trackpoints.
// Points are spaced 1 second and ~100 m apart (due north) starting at Berlin,
// which yields a speed of ~360 km/h and a course of ~0 degrees.
func minimalGPX(n int) string {
	base := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><trkseg>`
	for i := 0; i < n; i++ {
		lat := 52.520000 + float64(i)*0.0009 // ~100 m north per step
		base += fmt.Sprintf(`
    <trkpt lat="%f" lon="13.404954">
      <ele>35.0</ele>
      <time>%s</time>
    </trkpt>`, lat, gpxTrackStart.Add(time.Duration(i)*time.Second).Format(time.RFC3339))
	}
	base += `
  </trkseg></trk>
</gpx>`
	return base
}

// gpxWithUntimed returns a GPX file with one timed and one untimed point.
func gpxWithUntimed() string {
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
</gpx>`, gpxTrackStart.Format(time.RFC3339))
}

// gpxMultipartReq wraps GPX content in the ogen multipart/form-data request union member.
func gpxMultipartReq(gpxContent string) *oas.ImportGPXReqMultipartFormData {
	return &oas.ImportGPXReqMultipartFormData{
		File: oas.NewOptMultipartFile(ht.MultipartFile{
			Name: "track.gpx",
			File: strings.NewReader(gpxContent),
			Size: int64(len(gpxContent)),
		}),
	}
}

// setupGPXTest builds a live ogen Handler over real repositories plus a test user.
func setupGPXTest(t *testing.T) (*handlers.Handler, *repository.DeviceRepository, *repository.PositionRepository, *model.User) {
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

	h := handlers.NewHandler(handlers.HandlerConfig{
		Devices:   deviceRepo,
		Positions: posRepo,
	})
	return h, deviceRepo, posRepo, user
}

// gpxUserCtx returns a context carrying the given authenticated user.
func gpxUserCtx(user *model.User) context.Context {
	return api.ContextWithUser(context.Background(), user)
}

func TestImportGPX_AccessDenied(t *testing.T) {
	h, deviceRepo, _, user := setupGPXTest(t)
	ctx := context.Background()

	// Create a device owned by a different user (IDOR check).
	otherUser := &model.User{Email: "other@example.com", PasswordHash: "$2a$10$hash", Name: "Other"}
	pool := testutil.SetupTestDB(t)
	userRepo := repository.NewUserRepository(pool)
	if err := userRepo.Create(ctx, otherUser); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	device := &model.Device{UniqueID: "gpx-other-dev", Name: "Other Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, otherUser.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq(minimalGPX(2)), oas.ImportGPXParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ImportGPX returned error: %v", err)
	}
	if _, ok := res.(*oas.ImportGPXForbidden); !ok {
		t.Errorf("expected *oas.ImportGPXForbidden, got %T", res)
	}
}

func TestImportGPX_MissingOrEmptyFile(t *testing.T) {
	h, deviceRepo, _, user := setupGPXTest(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-nofile-dev", Name: "GPX Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	t.Run("file field not set", func(t *testing.T) {
		res, err := h.ImportGPX(gpxUserCtx(user), &oas.ImportGPXReqMultipartFormData{}, oas.ImportGPXParams{ID: device.ID})
		if err != nil {
			t.Fatalf("ImportGPX returned error: %v", err)
		}
		bad, ok := res.(*oas.ImportGPXBadRequest)
		if !ok {
			t.Fatalf("expected *oas.ImportGPXBadRequest, got %T", res)
		}
		if bad.Error != "missing file field" {
			t.Errorf("expected 'missing file field' error, got %q", bad.Error)
		}
	})

	t.Run("empty file content", func(t *testing.T) {
		res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq(""), oas.ImportGPXParams{ID: device.ID})
		if err != nil {
			t.Fatalf("ImportGPX returned error: %v", err)
		}
		if _, ok := res.(*oas.ImportGPXBadRequest); !ok {
			t.Errorf("expected *oas.ImportGPXBadRequest, got %T", res)
		}
	})
}

func TestImportGPX_InvalidGPXXML(t *testing.T) {
	h, deviceRepo, _, user := setupGPXTest(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-badxml-dev", Name: "GPX Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq("this is not xml"), oas.ImportGPXParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ImportGPX returned error: %v", err)
	}
	bad, ok := res.(*oas.ImportGPXBadRequest)
	if !ok {
		t.Fatalf("expected *oas.ImportGPXBadRequest, got %T", res)
	}
	if bad.Error != "invalid GPX file" {
		t.Errorf("expected 'invalid GPX file' error, got %q", bad.Error)
	}
}

func TestImportGPX_NoTimedPoints(t *testing.T) {
	h, deviceRepo, _, user := setupGPXTest(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-notimed-dev", Name: "GPX Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// GPX with only untimed points.
	gpx := `<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
  <trk><trkseg>
    <trkpt lat="52.52" lon="13.40"><ele>35.0</ele></trkpt>
  </trkseg></trk>
</gpx>`

	res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq(gpx), oas.ImportGPXParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ImportGPX returned error: %v", err)
	}
	bad, ok := res.(*oas.ImportGPXBadRequest)
	if !ok {
		t.Fatalf("expected *oas.ImportGPXBadRequest, got %T", res)
	}
	if bad.Error != "no timed positions found in GPX file" {
		t.Errorf("expected 'no timed positions found in GPX file' error, got %q", bad.Error)
	}
}

func TestImportGPX_Success(t *testing.T) {
	h, deviceRepo, posRepo, user := setupGPXTest(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-ok-dev", Name: "GPX Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// 3-point track with spacing for speed calculation.
	res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq(minimalGPX(3)), oas.ImportGPXParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ImportGPX returned error: %v", err)
	}
	ok, isOK := res.(*oas.ImportGPXOK)
	if !isOK {
		t.Fatalf("expected *oas.ImportGPXOK, got %T", res)
	}
	if ok.Imported.Value != 3 {
		t.Errorf("expected imported=3, got %d", ok.Imported.Value)
	}

	// Verify positions were persisted (use a wide range since GPX timestamps
	// may differ from current time). Results are ordered timestamp ascending.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	positions, err := posRepo.GetByDeviceAndTimeRange(ctx, device.ID, from, to, 10)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions in DB, got %d", len(positions))
	}

	// First point has no predecessor: speed and course must be 0.
	first := positions[0]
	if first.Speed == nil || *first.Speed != 0 {
		t.Errorf("expected first position speed=0, got %v", first.Speed)
	}
	if first.Course == nil || *first.Course != 0 {
		t.Errorf("expected first position course=0, got %v", first.Course)
	}

	// Subsequent points: ~100 m north per second => ~360 km/h, course ~0 deg.
	for i, pos := range positions[1:] {
		if pos.Speed == nil {
			t.Fatalf("position %d: expected speed, got nil", i+1)
		}
		if math.Abs(*pos.Speed-360.0) > 5.0 {
			t.Errorf("position %d: expected speed ~360 km/h, got %.2f", i+1, *pos.Speed)
		}
		if pos.Course == nil {
			t.Fatalf("position %d: expected course, got nil", i+1)
		}
		if math.Abs(*pos.Course) > 1.0 && math.Abs(*pos.Course-360.0) > 1.0 {
			t.Errorf("position %d: expected course ~0 deg, got %.2f", i+1, *pos.Course)
		}
	}

	// Device rollup: LastUpdate and PositionID must point at the last track point.
	updated, err := deviceRepo.GetByID(ctx, device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	lastPos := positions[2]
	if updated.LastUpdate == nil || !updated.LastUpdate.Equal(lastPos.Timestamp) {
		t.Errorf("expected device LastUpdate=%v, got %v", lastPos.Timestamp, updated.LastUpdate)
	}
	if updated.PositionID == nil || *updated.PositionID != lastPos.ID {
		t.Errorf("expected device PositionID=%d, got %v", lastPos.ID, updated.PositionID)
	}
}

func TestImportGPX_UntimedPointsSkipped(t *testing.T) {
	h, deviceRepo, posRepo, user := setupGPXTest(t)
	ctx := context.Background()

	device := &model.Device{UniqueID: "gpx-mix-dev", Name: "GPX Device", Status: "online"}
	if err := deviceRepo.Create(ctx, device, user.ID); err != nil {
		t.Fatalf("create device: %v", err)
	}

	res, err := h.ImportGPX(gpxUserCtx(user), gpxMultipartReq(gpxWithUntimed()), oas.ImportGPXParams{ID: device.ID})
	if err != nil {
		t.Fatalf("ImportGPX returned error: %v", err)
	}
	ok, isOK := res.(*oas.ImportGPXOK)
	if !isOK {
		t.Fatalf("expected *oas.ImportGPXOK, got %T", res)
	}
	if ok.Imported.Value != 1 {
		t.Errorf("expected imported=1, got %d", ok.Imported.Value)
	}

	// The untimed point must be skipped: only one position in the DB.
	// (ImportGPXOK carries no skipped count, so assert via persistence.)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	positions, err := posRepo.GetByDeviceAndTimeRange(ctx, device.ID, from, to, 10)
	if err != nil {
		t.Fatalf("get positions: %v", err)
	}
	if len(positions) != 1 {
		t.Errorf("expected 1 position in DB (untimed point skipped), got %d", len(positions))
	}
}
