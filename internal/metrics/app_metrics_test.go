package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestWebSocketMessagesSent(t *testing.T) {
	tests := []struct {
		messageType string
	}{
		{"position"},
		{"device"},
		{"event"},
	}

	for _, tt := range tests {
		t.Run(tt.messageType, func(t *testing.T) {
			before := testutil.ToFloat64(WebSocketMessagesSent.WithLabelValues(tt.messageType))
			WebSocketMessagesSent.WithLabelValues(tt.messageType).Inc()
			after := testutil.ToFloat64(WebSocketMessagesSent.WithLabelValues(tt.messageType))

			if after != before+1 {
				t.Errorf("WebSocketMessagesSent(%s) = %v, want %v", tt.messageType, after, before+1)
			}
		})
	}
}

func TestGPSDecodeErrors(t *testing.T) {
	tests := []struct {
		protocol string
	}{
		{"h02"},
		{"watch"},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			before := testutil.ToFloat64(GPSDecodeErrors.WithLabelValues(tt.protocol))
			GPSDecodeErrors.WithLabelValues(tt.protocol).Inc()
			after := testutil.ToFloat64(GPSDecodeErrors.WithLabelValues(tt.protocol))

			if after != before+1 {
				t.Errorf("GPSDecodeErrors(%s) = %v, want %v", tt.protocol, after, before+1)
			}
		})
	}
}

func TestPositionStorageErrors(t *testing.T) {
	before := testutil.ToFloat64(PositionStorageErrors)
	PositionStorageErrors.Inc()
	after := testutil.ToFloat64(PositionStorageErrors)

	if after != before+1 {
		t.Errorf("PositionStorageErrors = %v, want %v", after, before+1)
	}
}
