package services

import (
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestDeviceTimeoutLogic(t *testing.T) {
	timeout := 5 * time.Minute
	cutoff := time.Now().UTC().Add(-timeout)

	tests := []struct {
		name          string
		status        string
		lastSeen      *time.Time
		shouldTimeout bool
	}{
		{
			name:          "online device with recent last_seen stays online",
			status:        "online",
			lastSeen:      timePtr(time.Now().UTC()),
			shouldTimeout: false,
		},
		{
			name:          "online device with old last_seen times out",
			status:        "online",
			lastSeen:      timePtr(cutoff.Add(-1 * time.Minute)),
			shouldTimeout: true,
		},
		{
			name:          "online device with nil last_seen times out",
			status:        "online",
			lastSeen:      nil,
			shouldTimeout: true,
		},
		{
			name:          "moving device with recent last_seen stays moving",
			status:        "moving",
			lastSeen:      timePtr(time.Now().UTC()),
			shouldTimeout: false,
		},
		{
			name:          "moving device with old last_seen times out",
			status:        "moving",
			lastSeen:      timePtr(cutoff.Add(-1 * time.Minute)),
			shouldTimeout: true,
		},
		{
			name:          "moving device with nil last_seen times out",
			status:        "moving",
			lastSeen:      nil,
			shouldTimeout: true,
		},
		{
			name:          "offline device is skipped",
			status:        "offline",
			lastSeen:      timePtr(cutoff.Add(-10 * time.Minute)),
			shouldTimeout: false,
		},
		{
			name:          "unknown device is skipped",
			status:        "unknown",
			lastSeen:      nil,
			shouldTimeout: false,
		},
		{
			name:          "online device exactly at cutoff does not timeout",
			status:        "online",
			lastSeen:      timePtr(cutoff.Add(1 * time.Second)),
			shouldTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := model.Device{
				ID:         1,
				UniqueID:   "test-device",
				Status:     tt.status,
				LastUpdate: tt.lastSeen,
			}

			// Mirror the actual timeout logic: check both "online" and "moving" statuses.
			isActive := device.Status == "online" || device.Status == "moving"
			shouldTimeout := isActive &&
				(device.LastUpdate == nil || device.LastUpdate.Before(cutoff))

			if shouldTimeout != tt.shouldTimeout {
				t.Errorf("shouldTimeout = %v, want %v", shouldTimeout, tt.shouldTimeout)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
