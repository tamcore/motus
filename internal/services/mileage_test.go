package services

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

// --- Mock repos for mileage tests ---

type mileageMockPositionRepo struct {
	prev       *model.Position
	latest     *model.Position
	lastMoving *model.Position
}

func (r *mileageMockPositionRepo) Create(_ context.Context, _ *model.Position) error { return nil }
func (r *mileageMockPositionRepo) GetByID(_ context.Context, _ int64) (*model.Position, error) {
	return nil, nil
}
func (r *mileageMockPositionRepo) GetByIDs(_ context.Context, _ []int64) ([]*model.Position, error) {
	return nil, nil
}
func (r *mileageMockPositionRepo) GetLatestByDevice(_ context.Context, _ int64) (*model.Position, error) {
	return r.latest, nil
}
func (r *mileageMockPositionRepo) GetLatestByUser(_ context.Context, _ int64) ([]*model.Position, error) {
	return nil, nil
}
func (r *mileageMockPositionRepo) GetLatestAll(_ context.Context) ([]*model.Position, error) {
	return nil, nil
}
func (r *mileageMockPositionRepo) UpdateGeofenceIDs(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (r *mileageMockPositionRepo) GetByDeviceAndTimeRange(_ context.Context, _ int64, _, _ time.Time, _ int) ([]*model.Position, error) {
	return nil, nil
}
func (r *mileageMockPositionRepo) UpdateAddress(_ context.Context, _ int64, _ string) error {
	return nil
}
func (r *mileageMockPositionRepo) GetPreviousByDevice(_ context.Context, _ int64, _ time.Time) (*model.Position, error) {
	return r.prev, nil
}
func (r *mileageMockPositionRepo) GetLastMovingPosition(_ context.Context, _ int64, _ float64) (*model.Position, error) {
	return r.lastMoving, nil
}

type mileageMockDeviceRepo struct {
	updated *model.Device
}

func (r *mileageMockDeviceRepo) UserHasAccess(_ context.Context, _ *model.User, _ int64) bool {
	return true
}
func (r *mileageMockDeviceRepo) GetByID(_ context.Context, _ int64) (*model.Device, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) GetByUniqueID(_ context.Context, _ string) (*model.Device, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) GetByUser(_ context.Context, _ int64) ([]*model.Device, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) GetAll(_ context.Context) ([]model.Device, error) { return nil, nil }
func (r *mileageMockDeviceRepo) GetAllWithOwners(_ context.Context) ([]model.Device, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) GetTimedOut(_ context.Context, _ time.Time) ([]model.Device, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) GetUserIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (r *mileageMockDeviceRepo) Create(_ context.Context, _ *model.Device, _ int64) error {
	return nil
}
func (r *mileageMockDeviceRepo) Update(_ context.Context, d *model.Device) error {
	r.updated = d
	return nil
}
func (r *mileageMockDeviceRepo) Delete(_ context.Context, _ int64) error { return nil }

type mileageMockEventRepo struct {
	created []*model.Event
}

func (r *mileageMockEventRepo) Create(_ context.Context, ev *model.Event) error {
	ev.ID = int64(len(r.created) + 1)
	r.created = append(r.created, ev)
	return nil
}
func (r *mileageMockEventRepo) GetByDevice(_ context.Context, _ int64, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *mileageMockEventRepo) GetRecentByDeviceAndType(_ context.Context, _ int64, _ string, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *mileageMockEventRepo) GetByUser(_ context.Context, _ int64, _ int) ([]*model.Event, error) {
	return nil, nil
}
func (r *mileageMockEventRepo) GetByFilters(_ context.Context, _ int64, _ []int64, _ []string, _, _ time.Time) ([]*model.Event, error) {
	return nil, nil
}

// --- Helper functions ---

func makeMovingPos(deviceID int64, lat, lon, speed float64, ts time.Time) *model.Position {
	return &model.Position{
		ID:        ts.Unix(),
		DeviceID:  deviceID,
		Latitude:  lat,
		Longitude: lon,
		Speed:     &speed,
		Timestamp: ts,
	}
}

func makeStoppedPos(deviceID int64, lat, lon float64, ts time.Time) *model.Position {
	speed := 0.0
	return &model.Position{
		ID:        ts.Unix(),
		DeviceID:  deviceID,
		Latitude:  lat,
		Longitude: lon,
		Speed:     &speed,
		Timestamp: ts,
	}
}

// --- Tests ---

func TestProcessPosition_SkipsWhenMileageNil(t *testing.T) {
	svc := NewMileageService(nil, nil, nil, nil, nil)

	device := &model.Device{ID: 1, Mileage: nil}
	pos := makeMovingPos(1, 52.52, 13.405, 60, time.Now())

	if err := svc.ProcessPosition(context.Background(), pos, device); err != nil {
		t.Fatal(err)
	}
	if device.PendingMileage != 0 {
		t.Errorf("expected pending_mileage=0 for nil mileage device, got %f", device.PendingMileage)
	}
}

func TestProcessPosition_AccumulatesDistanceWhenMoving(t *testing.T) {
	now := time.Now()
	// Berlin (52.52, 13.405) → ~500m away
	prev := makeMovingPos(1, 52.52, 13.405, 60, now.Add(-10*time.Second))
	posRepo := &mileageMockPositionRepo{prev: prev}
	svc := NewMileageService(posRepo, nil, nil, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 0}
	curr := makeMovingPos(1, 52.5245, 13.405, 60, now)

	if err := svc.ProcessPosition(context.Background(), curr, device); err != nil {
		t.Fatal(err)
	}
	if device.PendingMileage <= 0 {
		t.Errorf("expected pending_mileage > 0 after moving, got %f", device.PendingMileage)
	}
	if *device.Mileage != 1000.0 {
		t.Errorf("mileage should not change during accumulation, got %f", *device.Mileage)
	}
}

func TestProcessPosition_DoesNotAccumulateWhenStopped(t *testing.T) {
	now := time.Now()
	prev := makeStoppedPos(1, 52.52, 13.405, now.Add(-10*time.Second))
	posRepo := &mileageMockPositionRepo{prev: prev}
	svc := NewMileageService(posRepo, nil, nil, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 0}
	curr := makeStoppedPos(1, 52.52, 13.405, now)

	if err := svc.ProcessPosition(context.Background(), curr, device); err != nil {
		t.Fatal(err)
	}
	if device.PendingMileage != 0 {
		t.Errorf("expected pending_mileage=0 when stopped with no pending, got %f", device.PendingMileage)
	}
}

func TestProcessPosition_CommitsOnTripCompletion(t *testing.T) {
	now := time.Now()
	prev := makeStoppedPos(1, 52.52, 13.405, now.Add(-10*time.Second))
	// Last moving position was 6 minutes ago (> MinStopDuration).
	lastMoving := makeMovingPos(1, 52.52, 13.405, 60, now.Add(-6*time.Minute))

	posRepo := &mileageMockPositionRepo{prev: prev, lastMoving: lastMoving}
	devRepo := &mileageMockDeviceRepo{}
	evRepo := &mileageMockEventRepo{}
	svc := NewMileageService(posRepo, devRepo, evRepo, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 25.5}
	curr := makeStoppedPos(1, 52.52, 13.405, now)

	if err := svc.ProcessPosition(context.Background(), curr, device); err != nil {
		t.Fatal(err)
	}

	// Mileage should be committed.
	if *device.Mileage != 1025.5 {
		t.Errorf("expected mileage=1025.5, got %f", *device.Mileage)
	}
	if device.PendingMileage != 0 {
		t.Errorf("expected pending_mileage=0 after commit, got %f", device.PendingMileage)
	}

	// Event should be created.
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	ev := evRepo.created[0]
	if ev.Type != "tripCompleted" {
		t.Errorf("event type = %q, want tripCompleted", ev.Type)
	}
	if dist, ok := ev.Attributes["distance"].(float64); !ok || dist != 25.5 {
		t.Errorf("event distance = %v, want 25.5", ev.Attributes["distance"])
	}

	// Device should be updated.
	if devRepo.updated == nil {
		t.Error("expected device to be updated")
	}
}

func TestProcessPosition_DoesNotCommitOnBriefStop(t *testing.T) {
	now := time.Now()
	prev := makeStoppedPos(1, 52.52, 13.405, now.Add(-10*time.Second))
	// Last moving position was only 2 minutes ago (< MinStopDuration).
	lastMoving := makeMovingPos(1, 52.52, 13.405, 60, now.Add(-2*time.Minute))

	posRepo := &mileageMockPositionRepo{prev: prev, lastMoving: lastMoving}
	svc := NewMileageService(posRepo, nil, nil, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 25.5}
	curr := makeStoppedPos(1, 52.52, 13.405, now)

	if err := svc.ProcessPosition(context.Background(), curr, device); err != nil {
		t.Fatal(err)
	}
	if *device.Mileage != 1000.0 {
		t.Errorf("mileage should not change on brief stop, got %f", *device.Mileage)
	}
	if device.PendingMileage != 25.5 {
		t.Errorf("pending_mileage should not change on brief stop, got %f", device.PendingMileage)
	}
}

func TestProcessPosition_FiltersGPSJumps(t *testing.T) {
	now := time.Now()
	// Previous position: Berlin. Current: very far away (GPS jump).
	prev := makeMovingPos(1, 52.52, 13.405, 60, now.Add(-10*time.Second))
	posRepo := &mileageMockPositionRepo{prev: prev}
	svc := NewMileageService(posRepo, nil, nil, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 0}
	// Jump to New York (massive distance).
	curr := makeMovingPos(1, 40.71, -74.01, 60, now)

	if err := svc.ProcessPosition(context.Background(), curr, device); err != nil {
		t.Fatal(err)
	}
	if device.PendingMileage != 0 {
		t.Errorf("expected GPS jump to be filtered, got pending_mileage=%f", device.PendingMileage)
	}
}

func TestCommitPendingMileage_CommitsAndCreatesEvent(t *testing.T) {
	now := time.Now()
	latest := makeStoppedPos(1, 52.52, 13.405, now.Add(-10*time.Minute))

	posRepo := &mileageMockPositionRepo{latest: latest}
	devRepo := &mileageMockDeviceRepo{}
	evRepo := &mileageMockEventRepo{}
	svc := NewMileageService(posRepo, devRepo, evRepo, nil, nil)

	mileage := 5000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 42.3}

	if err := svc.CommitPendingMileage(context.Background(), device); err != nil {
		t.Fatal(err)
	}

	if *device.Mileage != 5042.3 {
		t.Errorf("expected mileage=5042.3, got %f", *device.Mileage)
	}
	if device.PendingMileage != 0 {
		t.Errorf("expected pending_mileage=0, got %f", device.PendingMileage)
	}
	if len(evRepo.created) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evRepo.created))
	}
	if evRepo.created[0].Type != "tripCompleted" {
		t.Errorf("event type = %q, want tripCompleted", evRepo.created[0].Type)
	}
}

func TestCommitPendingMileage_SkipsWhenNoPending(t *testing.T) {
	svc := NewMileageService(nil, nil, nil, nil, nil)

	mileage := 1000.0
	device := &model.Device{ID: 1, Mileage: &mileage, PendingMileage: 0}

	if err := svc.CommitPendingMileage(context.Background(), device); err != nil {
		t.Fatal(err)
	}
	if *device.Mileage != 1000.0 {
		t.Errorf("mileage should not change when no pending, got %f", *device.Mileage)
	}
}

func TestCommitPendingMileage_SkipsWhenMileageNil(t *testing.T) {
	svc := NewMileageService(nil, nil, nil, nil, nil)

	device := &model.Device{ID: 1, Mileage: nil, PendingMileage: 10}

	if err := svc.CommitPendingMileage(context.Background(), device); err != nil {
		t.Fatal(err)
	}
}
