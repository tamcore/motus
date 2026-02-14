// Package metrics defines Prometheus metrics for the Motus application.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts HTTP requests by method, endpoint, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestDuration tracks HTTP request latency by method and endpoint.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "motus_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// WebSocketConnections tracks the current number of WebSocket connections.
	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "motus_websocket_connections",
			Help: "Current WebSocket connections",
		},
	)

	// GPSMessagesReceived counts GPS messages by protocol and device.
	GPSMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_gps_messages_total",
			Help: "GPS messages received",
		},
		[]string{"protocol"},
	)

	// PositionsStored counts the total number of positions stored.
	PositionsStored = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "motus_positions_stored_total",
			Help: "Total positions stored in the database",
		},
	)

	// GeofenceEvents counts geofence events by type (enter/exit).
	GeofenceEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_geofence_events_total",
			Help: "Geofence events triggered",
		},
		[]string{"type"},
	)

	// NotificationsSent counts notifications by channel and delivery status.
	NotificationsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "motus_notifications_sent_total",
			Help: "Notifications sent",
		},
		[]string{"channel", "status"},
	)

	// ActiveDevices tracks the number of currently online devices.
	ActiveDevices = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "motus_active_devices",
			Help: "Number of currently online devices",
		},
	)
)
