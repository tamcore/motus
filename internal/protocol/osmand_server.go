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
// It embeds Server for device resolution, auto-creation and logging.
type OsmAndServer struct {
	*Server
}

// NewOsmAndServer creates an HTTP server for the OsmAnd protocol.
func NewOsmAndServer(port string, devices repository.DeviceRepo, handler *PositionHandler) *OsmAndServer {
	return &OsmAndServer{&Server{
		name:    "osmand",
		port:    port,
		devices: devices,
		handler: handler,
		logger:  slog.Default(),
	}}
}

// Start listens for OsmAnd HTTP reports. It blocks until ctx is cancelled and
// then shuts the server down gracefully.
func (s *OsmAndServer) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("%s: listen on port %s: %w", s.name, s.port, err)
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

	s.log().Info("GPS protocol server listening",
		slog.String("type", "gps"),
		slog.String("protocol", s.name),
		slog.String("port", s.port),
	)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return fmt.Errorf("%s: serve: %w", s.name, err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log().Warn("shutdown timeout, connections still active",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
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
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
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
		s.log().Warn("decode error",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("remoteAddr", r.RemoteAddr),
			slog.Any("error", err),
		)
		metrics.GPSDecodeErrors.WithLabelValues(s.name).Inc()
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	metrics.GPSMessagesReceived.WithLabelValues(s.name).Inc()

	ctx := r.Context()
	device, err := s.resolveOrCreateDevice(ctx, msg.DeviceID)
	if err != nil {
		s.log().Warn("unknown device",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
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
		if err := s.handler.HandlePosition(ctx, position); err != nil {
			s.log().Error("handle position error",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
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
	p := &model.Position{
		DeviceID:   device.ID,
		Protocol:   s.name,
		ServerTime: &now,
		DeviceTime: new(msg.Timestamp),
		Timestamp:  msg.Timestamp,
		Valid:      msg.Valid,
		Latitude:   msg.Latitude,
		Longitude:  msg.Longitude,
		Speed:      new(msg.Speed),
		Course:     new(msg.Course),
		Altitude:   new(msg.Altitude),
		Accuracy:   msg.Accuracy,
		Network:    msg.Network,
		Attributes: msg.Attributes,
	}
	if msg.HasLocation {
		return p
	}

	last := s.handler.LastPosition(ctx, device.ID)
	if last == nil {
		return nil
	}
	applyLastLocation(p, last)
	return p
}
