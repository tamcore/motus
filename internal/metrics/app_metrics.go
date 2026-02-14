package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// WebSocketConnectionsByPod tracks current WebSocket connections per pod instance.
	// This complements the existing WebSocketConnections gauge by adding pod_id
	// dimensionality for multi-replica deployments.
	WebSocketConnectionsByPod = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "motus_websocket_connections_by_pod",
			Help: "Current WebSocket connections by pod instance",
		},
		[]string{"pod_id"},
	)

	// WebSocketMessagesSent counts WebSocket messages sent to clients by message type.
	// message_type is one of: position, device, event.
	WebSocketMessagesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_websocket_messages_sent_total",
			Help: "Total WebSocket messages sent to clients by message type",
		},
		[]string{"message_type"},
	)

	// GPSDecodeErrors counts GPS protocol decode errors by protocol name.
	GPSDecodeErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_gps_decode_errors_total",
			Help: "Total GPS protocol decode errors by protocol",
		},
		[]string{"protocol"},
	)

	// PositionStorageErrors counts errors when storing positions in the database.
	PositionStorageErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "motus_position_storage_errors_total",
			Help: "Total errors when storing positions in the database",
		},
	)
)
