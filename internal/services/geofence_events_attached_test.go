package services

import (
	"context"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/storage/repository/testutil"
)

// TestCheckGeofences_AttachedGeofencesOptIn verifies the opt-in filter: once
// geofences are attached to a device, only those produce events, and the
// geofences dropped from evaluation do not emit a spurious exit just because
// the previous position's stored membership still lists them.
func TestCheckGeofences_AttachedGeofencesOptIn(t *testing.T) {
	svc, geoRepo, _, deviceRepo, posRepo, _ := setupGeofenceService(t)
	ctx := context.Background()

	user := testutil.CreateUser(t, "attached-optin@example.com")
	device := testutil.CreateDevice(t, user.ID, "attached-optin-dev")

	home := &model.Geofence{Name: "Home", Geometry: testGeoJSON}
	park := &model.Geofence{Name: "Park", Geometry: testGeoJSON}
	for _, g := range []*model.Geofence{home, park} {
		if err := geoRepo.Create(ctx, g); err != nil {
			t.Fatal(err)
		}
		if err := geoRepo.AssociateUser(ctx, user.ID, g.ID); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC()
	report := func(lat, lon float64, at time.Time) *model.Position {
		t.Helper()
		pos := &model.Position{DeviceID: device.ID, Latitude: lat, Longitude: lon, Timestamp: at}
		if err := posRepo.Create(ctx, pos); err != nil {
			t.Fatal(err)
		}
		if err := svc.CheckGeofences(ctx, pos); err != nil {
			t.Fatal(err)
		}
		// Persist membership like the protocol handler does.
		if len(pos.GeofenceIDs) > 0 {
			if err := posRepo.UpdateGeofenceIDs(ctx, pos.ID, pos.GeofenceIDs); err != nil {
				t.Fatal(err)
			}
		}
		return pos
	}

	// No attachments: both geofences are evaluated -> two enters.
	report(52.52, 13.37, now.Add(-10*time.Minute))
	assertEventCounts(t, device.ID, home.ID, park.ID, map[string]int{"home:geofenceEnter": 1, "park:geofenceEnter": 1})

	// Attach only Home. Still inside both polygons: no exit for Park.
	if err := deviceRepo.SetGeofences(ctx, device.ID, []int64{home.ID}); err != nil {
		t.Fatal(err)
	}
	pos := report(52.521, 13.371, now.Add(-5*time.Minute))
	if len(pos.GeofenceIDs) != 1 || pos.GeofenceIDs[0] != home.ID {
		t.Errorf("GeofenceIDs = %v, want [%d]", pos.GeofenceIDs, home.ID)
	}
	assertEventCounts(t, device.ID, home.ID, park.ID, map[string]int{"home:geofenceEnter": 1, "park:geofenceEnter": 1})

	// Leave both polygons: only the attached Home emits an exit.
	report(52.60, 13.60, now)
	assertEventCounts(t, device.ID, home.ID, park.ID, map[string]int{
		"home:geofenceEnter": 1, "park:geofenceEnter": 1, "home:geofenceExit": 1,
	})
}

func assertEventCounts(t *testing.T, deviceID, homeID, parkID int64, want map[string]int) {
	t.Helper()
	events, err := deviceEvents(t, deviceID)
	if err != nil {
		t.Fatalf("deviceEvents: %v", err)
	}
	got := map[string]int{}
	for _, e := range events {
		name := "other"
		if e.GeofenceID != nil {
			switch *e.GeofenceID {
			case homeID:
				name = "home"
			case parkID:
				name = "park"
			}
		}
		got[name+":"+e.Type]++
	}
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}
