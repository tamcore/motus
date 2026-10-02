package reports

import (
	"math"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

var base = time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC)

func pos(offset time.Duration, lat float64, speed float64) *model.Position {
	return &model.Position{Timestamp: base.Add(offset), Latitude: lat, Longitude: 13.0, Speed: &speed}
}

func trips(ps ...*model.Position) []Trip {
	var d TripDetector
	for _, p := range ps {
		d.Add(p)
	}
	return d.Trips()
}

func stops(ps ...*model.Position) []Stop {
	var d StopDetector
	for _, p := range ps {
		d.Add(p)
	}
	return d.Stops()
}

func moving(n int, start time.Duration, step time.Duration, lat float64) []*model.Position {
	ps := make([]*model.Position, n)
	for i := range ps {
		ps[i] = pos(start+time.Duration(i)*step, lat+float64(i)*0.001, 60)
	}
	return ps
}

func TestTrips_Empty(t *testing.T) {
	if got := trips(); len(got) != 0 {
		t.Fatalf("expected no trips, got %d", len(got))
	}
}

func TestTrips_ContinuousMovementIsOneTrip(t *testing.T) {
	got := trips(moving(200, 0, 10*time.Second, 52)...)
	if len(got) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(got))
	}
	tr := got[0]
	if !tr.StartTime.Equal(base) || !tr.EndTime.Equal(base.Add(1990*time.Second)) {
		t.Errorf("unexpected bounds %v - %v", tr.StartTime, tr.EndTime)
	}
	if tr.Duration != 1990 {
		t.Errorf("duration = %v, want 1990", tr.Duration)
	}
	if tr.AvgSpeed != 60 || tr.MaxSpeed != 60 {
		t.Errorf("avg/max = %v/%v, want 60/60", tr.AvgSpeed, tr.MaxSpeed)
	}
	// 199 segments of 0.001 deg latitude ≈ 0.1112 km each.
	if math.Abs(tr.Distance-22.13) > 0.01 {
		t.Errorf("distance = %v, want ≈22.13", tr.Distance)
	}
}

func TestTrips_SplitOnHourGap(t *testing.T) {
	ps := append(moving(10, 0, 10*time.Second, 52), moving(10, 2*time.Hour, 10*time.Second, 53)...)
	if got := trips(ps...); len(got) != 2 {
		t.Fatalf("expected 2 trips, got %d", len(got))
	}
}

func TestTrips_BriefStopStaysInTrip(t *testing.T) {
	ps := moving(10, 0, 10*time.Second, 52)
	ps = append(ps, pos(100*time.Second, 52.01, 0), pos(200*time.Second, 52.01, 0))
	ps = append(ps, moving(10, 300*time.Second, 10*time.Second, 52.02)...)
	got := trips(ps...)
	if len(got) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(got))
	}
	if got[0].AvgSpeed != 60 {
		t.Errorf("avg speed must ignore stopped positions, got %v", got[0].AvgSpeed)
	}
}

func TestTrips_ExtendedStopEndsTrip(t *testing.T) {
	ps := moving(10, 0, 10*time.Second, 52)
	stopStart := 100 * time.Second
	for i := range 10 {
		ps = append(ps, pos(stopStart+time.Duration(i)*time.Minute, 52.01, 0))
	}
	ps = append(ps, moving(10, stopStart+10*time.Minute, 10*time.Second, 52.02)...)
	got := trips(ps...)
	if len(got) != 2 {
		t.Fatalf("expected 2 trips, got %d", len(got))
	}
	// First trip keeps stopped positions until the 5-minute mark (exclusive).
	if want := base.Add(stopStart + 4*time.Minute); !got[0].EndTime.Equal(want) {
		t.Errorf("first trip end = %v, want %v", got[0].EndTime, want)
	}
}

func TestTrips_ShortTripDropped(t *testing.T) {
	if got := trips(moving(5, 0, 10*time.Second, 52)...); len(got) != 0 {
		t.Fatalf("trip under 60s must be dropped, got %d", len(got))
	}
	if got := trips(pos(0, 52, 60)); len(got) != 0 {
		t.Fatalf("single position must not form a trip, got %d", len(got))
	}
}

func TestTrips_StationaryOnlyIgnored(t *testing.T) {
	var ps []*model.Position
	for i := range 20 {
		ps = append(ps, pos(time.Duration(i)*time.Minute, 52, 3))
	}
	if got := trips(ps...); len(got) != 0 {
		t.Fatalf("expected no trips, got %d", len(got))
	}
}

func TestTrips_NilSpeedIsStopped(t *testing.T) {
	ps := moving(10, 0, 10*time.Second, 52)
	ps = append(ps, &model.Position{Timestamp: base.Add(100 * time.Second), Latitude: 52.01, Longitude: 13})
	got := trips(ps...)
	if len(got) != 1 || got[0].MaxSpeed != 60 || got[0].AvgSpeed != 60 {
		t.Fatalf("unexpected trips %+v", got)
	}
	if !got[0].EndTime.Equal(base.Add(100 * time.Second)) {
		t.Errorf("nil-speed position must join the trip as stopped")
	}
}

func TestTrips_ManyShortTrips(t *testing.T) {
	var ps []*model.Position
	for n := range 100 {
		start := time.Duration(n) * time.Hour
		ps = append(ps, pos(start, 52, 60), pos(start+5*time.Second, 52.001, 60))
		for j := range 10 {
			ps = append(ps, pos(start+10*time.Second+time.Duration(j)*time.Minute, 52.002, 0))
		}
	}
	if got := trips(ps...); len(got) != 100 {
		t.Fatalf("expected 100 trips, got %d", len(got))
	}
}

func TestStops_Empty(t *testing.T) {
	if got := stops(); len(got) != 0 {
		t.Fatalf("expected no stops, got %d", len(got))
	}
}

func TestStops_DetectsStopBetweenMovement(t *testing.T) {
	addr := "Main St 1"
	ps := []*model.Position{pos(0, 52, 30)}
	for i := range 6 {
		p := pos(time.Minute+time.Duration(i)*time.Minute, 52+float64(i)*0.0001, 0)
		if i == 2 {
			p.Address = &addr
		}
		ps = append(ps, p)
	}
	ps = append(ps, pos(10*time.Minute, 52.01, 30))
	got := stops(ps...)
	if len(got) != 1 {
		t.Fatalf("expected 1 stop, got %d", len(got))
	}
	s := got[0]
	if !s.ArrivalTime.Equal(base.Add(time.Minute)) || !s.DepartureTime.Equal(base.Add(6*time.Minute)) {
		t.Errorf("unexpected times %v - %v", s.ArrivalTime, s.DepartureTime)
	}
	// Duration runs until the first moving position, as in the TS implementation.
	if s.Duration != 540 {
		t.Errorf("duration = %v, want 540", s.Duration)
	}
	if s.Address != addr {
		t.Errorf("address = %q, want %q", s.Address, addr)
	}
	if math.Abs(s.Latitude-52.00025) > 1e-9 || s.Longitude != 13 {
		t.Errorf("mean coords = %v,%v", s.Latitude, s.Longitude)
	}
}

func TestStops_ShortStopIgnored(t *testing.T) {
	got := stops(pos(0, 52, 30), pos(time.Minute, 52, 0), pos(3*time.Minute, 52, 0), pos(4*time.Minute, 52, 30))
	if len(got) != 0 {
		t.Fatalf("expected no stops, got %d", len(got))
	}
}

func TestStops_InProgressAtEnd(t *testing.T) {
	got := stops(pos(0, 52, 30), pos(time.Minute, 52.5, 0), pos(7*time.Minute, 52.5, 0.5))
	if len(got) != 1 {
		t.Fatalf("expected 1 stop, got %d", len(got))
	}
	if got[0].Duration != 360 || !got[0].DepartureTime.Equal(base.Add(7*time.Minute)) {
		t.Errorf("unexpected stop %+v", got[0])
	}
	if got[0].Address != "52.50000, 13.00000" {
		t.Errorf("fallback address = %q", got[0].Address)
	}
}

func TestStops_InProgressTooShort(t *testing.T) {
	if got := stops(pos(0, 52, 0), pos(4*time.Minute, 52, 0)); len(got) != 0 {
		t.Fatalf("expected no stops, got %d", len(got))
	}
}

func TestStops_SlowSpeedEndsStop(t *testing.T) {
	got := stops(pos(0, 52, 0), pos(6*time.Minute, 52, 0), pos(7*time.Minute, 52, 1), pos(8*time.Minute, 52, 0))
	if len(got) != 1 || got[0].Duration != 420 {
		t.Fatalf("speed >= 1 km/h must end the stop, got %+v", got)
	}
}
