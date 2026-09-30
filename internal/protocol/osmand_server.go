package protocol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol/osmand"
	"github.com/tamcore/motus/internal/storage/repository"
)

// osmandMaxBodySize bounds a single OsmAnd request body. Reports are a few
// hundred bytes; the limit only protects against abuse.
const osmandMaxBodySize = 64 << 10

// OsmAndServer is an HTTP server for the OsmAnd / Traccar Client protocol
// (Traccar's default port 5055). Unlike H02 and WATCH, every report is a
// separate HTTP request, so there is no persistent connection: devices are
// marked offline by the device timeout service only.
type OsmAndServer struct {
	// core provides device resolution/auto-creation, the position handler
	// and logging shared with the TCP protocol servers.
	core *Server
}

// NewOsmAndServer creates an HTTP server for the OsmAnd protocol.
func NewOsmAndServer(port string, devices repository.DeviceRepo, handler *PositionHandler) *OsmAndServer {
	return &OsmAndServer{core: &Server{
		name:    "osmand",
		port:    port,
		devices: devices,
		handler: handler,
		logger:  slog.Default(),
	}}
}

// SetLogger configures the structured logger for this server.
func (s *OsmAndServer) SetLogger(l *slog.Logger) { s.core.SetLogger(l) }

// SetAutoCreate configures device auto-creation for unknown device IDs.
func (s *OsmAndServer) SetAutoCreate(cfg AutoCreateConfig, users repository.UserRepo) {
	s.core.SetAutoCreate(cfg, users)
}

// Start listens for OsmAnd HTTP reports. It blocks until ctx is cancelled and
// then shuts the server down gracefully.
func (s *OsmAndServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", ":"+s.core.port)
	if err != nil {
		return fmt.Errorf("%s: listen on port %s: %w", s.core.name, s.core.port, err)
	}

	srv := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	s.core.log().Info("GPS protocol server listening",
		slog.String("type", "gps"),
		slog.String("protocol", s.core.name),
		slog.String("port", s.core.port),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return fmt.Errorf("%s: serve: %w", s.core.name, err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.core.log().Warn("shutdown timeout, connections still active",
			slog.String("type", "gps"),
			slog.String("protocol", s.core.name),
			slog.Any("error", err),
		)
	}
	return nil
}

// ServeHTTP handles one OsmAnd report, mirroring Traccar's OsmAndProtocolDecoder:
// JSON bodies (application/json) and query/form parameters on any path.
// Responses: 200 on success; 400 for malformed reports and unknown devices
// in the query format; 404 for unknown devices in the JSON format.
func (s *OsmAndServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, osmandMaxBodySize))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	isJSON := mediaType == "application/json"

	var msg *osmand.Message
	if isJSON {
		msg, err = osmand.DecodeJSON(body)
	} else {
		params := r.URL.Query()
		if len(params) == 0 {
			params, err = url.ParseQuery(string(body))
		}
		if err == nil {
			msg, err = osmand.DecodeQuery(params, time.Now().UTC())
		}
	}
	if err != nil {
		s.core.log().Warn("decode error",
			slog.String("type", "gps"),
			slog.String("protocol", s.core.name),
			slog.String("remoteAddr", r.RemoteAddr),
			slog.Any("error", err),
		)
		metrics.GPSDecodeErrors.WithLabelValues(s.core.name).Inc()
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	metrics.GPSMessagesReceived.WithLabelValues(s.core.name).Inc()

	ctx := r.Context()
	device, err := s.core.resolveOrCreateDevice(ctx, msg.DeviceID)
	if err != nil {
		s.core.log().Warn("unknown device",
			slog.String("type", "gps"),
			slog.String("protocol", s.core.name),
			slog.String("remoteAddr", r.RemoteAddr),
			slog.String("device", msg.DeviceID),
			slog.Any("error", err),
		)
		status := http.StatusBadRequest
		if isJSON {
			status = http.StatusNotFound
		}
		http.Error(w, "unknown device", status)
		return
	}

	if position := s.position(ctx, device, msg); position != nil {
		if err := s.core.handler.HandlePosition(ctx, position); err != nil {
			s.core.log().Error("handle position error",
				slog.String("type", "gps"),
				slog.String("protocol", s.core.name),
				slog.String("device", msg.DeviceID),
				slog.Any("error", err),
			)
			// A non-2xx status makes the app keep the report and retry.
			http.Error(w, "store position failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// position converts a decoded report to a model position. Reports without
// coordinates are placed at the device's last known location and marked
// outdated; without a last known location nothing is stored (nil).
func (s *OsmAndServer) position(ctx context.Context, device *model.Device, msg *osmand.Message) *model.Position {
	now := time.Now().UTC()
	deviceTime := msg.Timestamp
	speed, course, altitude := msg.Speed, msg.Course, msg.Altitude

	p := &model.Position{
		DeviceID:   device.ID,
		Protocol:   s.core.name,
		ServerTime: &now,
		DeviceTime: &deviceTime,
		Timestamp:  msg.Timestamp,
		Valid:      msg.Valid,
		Latitude:   msg.Latitude,
		Longitude:  msg.Longitude,
		Speed:      &speed,
		Course:     &course,
		Altitude:   &altitude,
		Accuracy:   msg.Accuracy,
		Network:    msg.Network,
		Attributes: msg.Attributes,
	}
	if msg.HasLocation {
		return p
	}

	last := s.lastPosition(ctx, device.ID)
	if last == nil {
		return nil
	}
	p.Timestamp = last.Timestamp
	p.Valid = last.Valid
	p.Latitude = last.Latitude
	p.Longitude = last.Longitude
	p.Altitude = last.Altitude
	p.Speed = last.Speed
	p.Course = last.Course
	p.Accuracy = last.Accuracy
	p.Outdated = true
	return p
}

// lastPosition returns the most recent stored position of a device, or nil.
func (s *OsmAndServer) lastPosition(ctx context.Context, deviceID int64) *model.Position {
	h := s.core.handler
	if h == nil || h.positions == nil {
		return nil
	}
	p, err := h.positions.GetLatestByDevice(ctx, deviceID)
	if err != nil {
		return nil
	}
	return p
}
