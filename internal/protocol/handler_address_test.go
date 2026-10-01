package protocol

import (
	"testing"

	"github.com/tamcore/motus/internal/model"
)

type fakeAddressLookup struct {
	cache      map[geoPoint]string
	peeks      int
	prefetched []geoPoint
}

func (f *fakeAddressLookup) Peek(lat, lon float64) (string, bool) {
	f.peeks++
	addr, ok := f.cache[geoPoint{lat, lon}]
	return addr, ok
}

func (f *fakeAddressLookup) Prefetch(lat, lon float64) {
	f.prefetched = append(f.prefetched, geoPoint{lat, lon})
}

func newAddressHandler(cache map[geoPoint]string) (*PositionHandler, *fakeAddressLookup) {
	lookup := &fakeAddressLookup{cache: cache}
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
	h, lookup := newAddressHandler(map[geoPoint]string{{52.52, 13.405}: "Berlin"})

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

func TestPositionHandler_AddressMissQueuesOneLookupPerDevice(t *testing.T) {
	h, lookup := newAddressHandler(map[geoPoint]string{})

	if got := addressAt(h, 1, 48.1, 11.5); got != "<nil>" {
		t.Errorf("first miss = %q, want <nil>", got)
	}
	addressAt(h, 1, 48.11, 11.5)
	addressAt(h, 1, 48.12, 11.5)
	if len(lookup.prefetched) != 1 {
		t.Errorf("prefetched %d, want 1 while a lookup is pending", len(lookup.prefetched))
	}
}

func TestPositionHandler_AddressPicksUpPendingResult(t *testing.T) {
	cache := map[geoPoint]string{}
	h, _ := newAddressHandler(cache)

	addressAt(h, 1, 48.1, 11.5)
	cache[geoPoint{48.1, 11.5}] = "Munich"

	if got := addressAt(h, 1, 48.105, 11.5); got != "Munich" {
		t.Errorf("~550 m on = %q, want stale Munich while refreshing", got)
	}
	if got := addressAt(h, 1, 48.2, 11.5); got != "<nil>" {
		t.Errorf("~11 km on = %q, want <nil> (too stale)", got)
	}
}
