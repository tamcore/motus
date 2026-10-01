package protocol

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/tamcore/motus/internal/model"
)

type countingDeviceRepo struct {
	*memDeviceRepo
	lookups atomic.Int32
}

func (r *countingDeviceRepo) GetByUniqueID(ctx context.Context, uniqueID string) (*model.Device, error) {
	r.lookups.Add(1)
	return r.memDeviceRepo.GetByUniqueID(ctx, uniqueID)
}

func TestResolveOrCreateDevice_CachesLookups(t *testing.T) {
	repo := &countingDeviceRepo{memDeviceRepo: newMemDeviceRepo(&model.Device{UniqueID: "cached", Protocol: "h02"})}
	s := &Server{name: "h02", devices: repo}

	first, err := s.resolveOrCreateDevice(t.Context(), "cached")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	first.Name = "mutated by caller"
	second, err := s.resolveOrCreateDevice(t.Context(), "cached")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if n := repo.lookups.Load(); n != 1 {
		t.Errorf("GetByUniqueID calls = %d, want 1 within the cache TTL", n)
	}
	if second.ID != first.ID || second.Name == "mutated by caller" {
		t.Errorf("second = %+v, want same device as an independent copy", second)
	}
}

func TestResolveOrCreateDevice_DoesNotCacheMisses(t *testing.T) {
	repo := &countingDeviceRepo{memDeviceRepo: newMemDeviceRepo()}
	s := &Server{name: "h02", devices: repo}

	for range 2 {
		if _, err := s.resolveOrCreateDevice(t.Context(), "unknown"); err == nil {
			t.Fatal("expected an error for an unknown device without auto-create")
		}
	}
	if n := repo.lookups.Load(); n != 2 {
		t.Errorf("GetByUniqueID calls = %d, want 2 (misses not cached)", n)
	}
}
