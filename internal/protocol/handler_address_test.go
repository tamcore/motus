package protocol

import (
	"testing"
	"time"

	"github.com/tamcore/motus/internal/geocoding"
	"github.com/tamcore/motus/internal/model"
)

type point struct{ lat, lon float64 }

type fakeAddressLookup struct {
	cache      map[point]string
	resolved   map[int64]geocoding.Resolved
	peeks      int
	prefetched map[int64]point
}

func (f *fakeAddressLookup) Peek(lat, lon float64) (string, bool) {
	f.peeks++
	addr, ok := f.cache[point{lat, lon}]
	return addr, ok
}

func (f *fakeAddressLookup) Prefetch(key int64, lat, lon float64) {
	f.prefetched[key] = point{lat, lon}
}

func (f *fakeAddressLookup) Resolved(key int64) (geocoding.Resolved, bool) {
	r, ok := f.resolved[key]
	return r, ok
}

func newAddressHandler(cache map[point]string) (*PositionHandler, *fakeAddressLookup) {
	lookup := &fakeAddressLookup{cache: cache, resolved: map[int64]geocoding.Resolved{}, prefetched: map[int64]point{}}
	h := NewPositionHandler(nil, nil, nil, nil)
	h.SetAddressLookup(lookup)
	return h, lookup
}

func addressAt(h *PositionHandler, device int64, lat, lon float64) string {
	if a := h.address(&model.Position{DeviceID: device, Latitude: lat, Longitude: lon}); a != nil {
		return *a
	}
	return "<nil>"
}

func TestPositionHandler_AddressCacheHitAndReuse(t *testing.T) {
	h, lookup := newAddressHandler(map[point]string{{52.52, 13.405}: "Berlin"})

	if got := addressAt(h, 1, 52.52, 13.405); got != "Berlin" {
		t.Fatalf("cache hit = %q, want Berlin", got)
	}
	peeks := lookup.peeks
	if got := addressAt(h, 1, 52.5202, 13.405); got != "Berlin" || lookup.peeks != peeks {
		t.Errorf("within ~22 m = %q with %d extra peeks, want Berlin without Peek", got, lookup.peeks-peeks)
	}
	if got := addressAt(h, 2, 52.5202, 13.405); got != "<nil>" {
		t.Errorf("other device = %q, want <nil> (no reuse across devices)", got)
	}
}

func TestPositionHandler_AddressMissPrefetchesLatestPoint(t *testing.T) {
	h, lookup := newAddressHandler(map[point]string{})

	if got := addressAt(h, 1, 48.1, 11.5); got != "<nil>" {
		t.Errorf("first miss = %q, want <nil>", got)
	}
	addressAt(h, 1, 48.11, 11.5)
	if p := lookup.prefetched[1]; p != (point{48.11, 11.5}) {
		t.Errorf("prefetched %v, want latest point {48.11 11.5}", p)
	}
}

func TestPositionHandler_AddressPicksUpResolvedResult(t *testing.T) {
	h, lookup := newAddressHandler(map[point]string{})
	addressAt(h, 1, 48.1, 11.5)
	lookup.resolved[1] = geocoding.Resolved{Lat: 48.1, Lon: 11.5, Address: "Munich", RequestedAt: time.Now()}

	if got := addressAt(h, 1, 48.105, 11.5); got != "Munich" {
		t.Errorf("~550 m on = %q, want stale Munich while refreshing", got)
	}
	if got := addressAt(h, 1, 48.2, 11.5); got != "<nil>" {
		t.Errorf("~11 km on = %q, want <nil> (too stale)", got)
	}
}

func TestPositionHandler_AddressIgnoresOlderResolvedResult(t *testing.T) {
	h, lookup := newAddressHandler(map[point]string{{52.52, 13.405}: "Berlin"})
	lookup.resolved[1] = geocoding.Resolved{Lat: 52.52, Lon: 13.41, Address: "Old", RequestedAt: time.Now().Add(-time.Minute)}

	addressAt(h, 1, 52.52, 13.405)
	if got := addressAt(h, 1, 52.5201, 13.405); got != "Berlin" {
		t.Errorf("after cache hit = %q, want Berlin (older resolved result ignored)", got)
	}
}
