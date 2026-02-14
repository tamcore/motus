package protocol

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
	"github.com/tamcore/motus/internal/protocol/h02"
	"github.com/tamcore/motus/internal/protocol/watch"
	"github.com/tamcore/motus/internal/storage/repository"
)

// Decoder is the function signature for protocol-specific message decoding.
// It receives the connection context for database lookups and cancellation.
// It returns a position (may be nil for heartbeats), the device unique ID,
// a response to send back (may be empty), and an error.
type Decoder func(ctx context.Context, line string) (*model.Position, string, string, error)

// AutoCreateConfig holds device auto-creation settings for the protocol server.
type AutoCreateConfig struct {
	// Enabled controls whether unknown devices are automatically created.
	Enabled bool
	// DefaultUserEmail is the email of the user that auto-created devices
	// are assigned to. Must exist in the database.
	DefaultUserEmail string
}

// maxConnections is the default maximum number of concurrent GPS device connections.
// Override via SetMaxConnections(). Zero means unlimited (not recommended in production).
const defaultMaxConnections int64 = 1000

// Server is a TCP server that accepts GPS device connections,
// decodes protocol messages, and passes positions to a handler.
type Server struct {
	name     string
	port     string
	devices  repository.DeviceRepo
	handler  *PositionHandler
	decoder  Decoder
	listener net.Listener
	logger   *slog.Logger

	// Device auto-creation.
	users         repository.UserRepo
	autoCreate    AutoCreateConfig
	defaultUserID int64      // cached user ID for auto-creation (protected by userIDMu)
	userIDMu      sync.Mutex // protects defaultUserID caching

	// Optional relay target: "host:port" or "" if relay is disabled.
	relayTarget string

	// Connection tracking for graceful shutdown.
	activeConns    sync.WaitGroup
	connCount      atomic.Int64
	maxConnections int64

	// Optional: live connection registry for command dispatch.
	registry *DeviceRegistry

	// Optional: command repository for storing device responses.
	commands repository.CommandRepo

	// Optional: custom scanner split function for protocol-specific framing.
	// When nil, the default bufio.ScanLines is used.
	scannerSplit bufio.SplitFunc
}

// NewH02Server creates a TCP server for the H02 GPS protocol.
func NewH02Server(port string, devices repository.DeviceRepo, handler *PositionHandler) *Server {
	s := &Server{
		name:           "h02",
		port:           port,
		devices:        devices,
		handler:        handler,
		logger:         slog.Default(),
		maxConnections: defaultMaxConnections,
		scannerSplit:   h02SplitFunc,
	}
	s.decoder = s.decodeH02
	return s
}

// NewWatchServer creates a TCP server for the WATCH GPS protocol.
func NewWatchServer(port string, devices repository.DeviceRepo, handler *PositionHandler) *Server {
	s := &Server{
		name:           "watch",
		port:           port,
		devices:        devices,
		handler:        handler,
		logger:         slog.Default(),
		maxConnections: defaultMaxConnections,
	}
	s.decoder = s.decodeWatch
	return s
}

// SetLogger configures the structured logger for this server.
func (s *Server) SetLogger(l *slog.Logger) {
	if l != nil {
		s.logger = l
	}
}

// log returns the server's logger, falling back to slog.Default() if nil.
// This ensures tests that create Server structs directly (without the
// constructor) do not panic on nil logger access.
func (s *Server) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

// SetRelay configures an optional TCP relay target ("host:port"). When set,
// every raw message received from a device is forwarded verbatim to that
// address before decoding. Relay errors are non-fatal; Motus continues
// serving the device normally if the relay is unreachable or drops.
func (s *Server) SetRelay(target string) {
	s.relayTarget = target
}

// SetAutoCreate configures device auto-creation. When enabled, unknown
// devices are automatically created and assigned to the default user.
// The users repo is required to look up the default user by email.
func (s *Server) SetAutoCreate(cfg AutoCreateConfig, users repository.UserRepo) {
	s.autoCreate = cfg
	s.users = users
}

// SetRegistry configures the device connection registry for outbound command dispatch.
func (s *Server) SetRegistry(r *DeviceRegistry) {
	s.registry = r
}

// SetCommandRepo configures the command repository for storing device responses.
func (s *Server) SetCommandRepo(r repository.CommandRepo) {
	s.commands = r
}

// SetMaxConnections sets the maximum number of concurrent device connections.
// When the limit is reached, new connections are rejected. Set to 0 to disable
// the limit (not recommended in production).
func (s *Server) SetMaxConnections(max int64) {
	s.maxConnections = max
}

// resolveOrCreateDevice looks up a device by unique ID. If the device is not
// found and auto-creation is enabled, it creates the device and assigns it to
// the configured default user. Returns the device or an error.
func (s *Server) resolveOrCreateDevice(ctx context.Context, uniqueID string) (*model.Device, error) {
	device, err := s.devices.GetByUniqueID(ctx, uniqueID)
	if err == nil {
		return device, nil
	}

	// Device not found. If auto-create is disabled, return the original error.
	if !s.autoCreate.Enabled {
		return nil, fmt.Errorf("unknown device %s: %w", uniqueID, err)
	}

	// Resolve the default user ID (cached after first successful lookup).
	userID, userErr := s.resolveDefaultUserID(ctx)
	if userErr != nil {
		return nil, fmt.Errorf("auto-create device %s: %w", uniqueID, userErr)
	}

	// Create the device.
	newDevice := &model.Device{
		UniqueID: uniqueID,
		Name:     uniqueID, // Use unique ID as name; user can rename later.
		Protocol: s.name,
		Status:   "unknown",
	}
	if createErr := s.devices.Create(ctx, newDevice, userID); createErr != nil {
		return nil, fmt.Errorf("auto-create device %s: %w", uniqueID, createErr)
	}

	s.log().Info("auto-created device",
		slog.String("type", "gps"),
		slog.String("protocol", s.name),
		slog.String("uniqueID", uniqueID),
		slog.Int64("assignedToUser", userID),
	)

	return newDevice, nil
}

// resolveDefaultUserID returns the cached default user ID, performing a
// lookup by email on first call. On failure the next call will retry,
// allowing the user to be created after the server starts.
// Thread-safe: concurrent callers may each perform the lookup on first miss,
// but they will all store the same value.
func (s *Server) resolveDefaultUserID(ctx context.Context) (int64, error) {
	if s.users == nil {
		return 0, fmt.Errorf("user repository not configured for auto-creation")
	}

	s.userIDMu.Lock()
	cached := s.defaultUserID
	s.userIDMu.Unlock()

	if cached != 0 {
		return cached, nil
	}

	user, err := s.users.GetByEmail(ctx, s.autoCreate.DefaultUserEmail)
	if err != nil {
		return 0, fmt.Errorf("default user %q not found: %w", s.autoCreate.DefaultUserEmail, err)
	}

	s.userIDMu.Lock()
	s.defaultUserID = user.ID
	s.userIDMu.Unlock()

	return user.ID, nil
}

// Start begins listening for GPS device connections. It blocks until the
// context is cancelled, at which point it stops accepting new connections
// and waits for active connections to drain.
func (s *Server) Start(ctx context.Context) error {
	var err error
	s.listener, err = net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("%s: listen on port %s: %w", s.name, s.port, err)
	}

	s.log().Info("GPS protocol server listening",
		slog.String("type", "gps"),
		slog.String("protocol", s.name),
		slog.String("port", s.port),
	)

	// Accept loop runs in a goroutine so we can select on ctx.Done().
	go s.acceptLoop(ctx)

	// Block until context is cancelled.
	<-ctx.Done()

	// Stop accepting new connections.
	_ = s.listener.Close()

	// Wait for active connections to finish (with timeout).
	done := make(chan struct{})
	go func() {
		s.activeConns.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.log().Info("all connections drained", slog.String("type", "gps"), slog.String("protocol", s.name))
	case <-time.After(10 * time.Second):
		s.log().Warn("shutdown timeout, connections still active",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.Int64("activeConnections", s.connCount.Load()),
		)
	}

	return nil
}

func (s *Server) acceptLoop(ctx context.Context) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				// Transient error, log and continue.
				s.log().Error("accept error",
					slog.String("type", "gps"),
					slog.String("protocol", s.name),
					slog.Any("error", err),
				)
				continue
			}
		}

		// Enforce connection limit to prevent resource exhaustion.
		if s.maxConnections > 0 && s.connCount.Load() >= s.maxConnections {
			s.log().Warn("connection rejected: limit reached",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.Int64("current", s.connCount.Load()),
				slog.Int64("max", s.maxConnections),
			)
			_ = conn.Close()
			continue
		}

		s.activeConns.Add(1)
		s.connCount.Add(1)
		go func() {
			defer s.activeConns.Done()
			defer s.connCount.Add(-1)
			s.handleConnection(ctx, conn)
		}()
	}
}

// connID generates a short random hex ID to correlate log lines for a single connection.
func connID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleConnection processes a single GPS device connection.
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	id := connID()
	remoteAddr := conn.RemoteAddr().String()
	s.log().Info("new connection",
		slog.String("type", "gps"),
		slog.String("protocol", s.name),
		slog.String("conn", id),
		slog.String("remoteAddr", remoteAddr),
	)

	// Set an initial read deadline. Each successful read resets it.
	const readTimeout = 5 * time.Minute
	_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

	// Outbound channel: commands are written here and forwarded to the device.
	outCh := make(chan []byte, 16)
	defer close(outCh)

	// Write goroutine: reads from outCh and sends to the device connection.
	// On write error, closes the connection so the scanner loop exits
	// naturally and runs the post-loop cleanup (Deregister, markDeviceOffline).
	go func() {
		for data := range outCh {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := conn.Write(data); err != nil {
				s.log().Warn("write to device failed",
					slog.String("type", "gps"),
					slog.String("protocol", s.name),
					slog.String("conn", id),
					slog.Any("error", err),
				)
				_ = conn.Close() // wake the scanner so it exits promptly
				return
			}
			// Reset the write deadline so the scanner loop's fmt.Fprintf
			// calls are not affected by this per-command deadline.
			_ = conn.SetWriteDeadline(time.Time{})
			s.log().Debug("tx (command)",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.String("conn", id),
				slog.String("data", truncate(string(data), 200)),
			)
		}
	}()

	// Establish relay connection if configured.
	var relayConn net.Conn
	if s.relayTarget != "" {
		var relayErr error
		relayConn, relayErr = net.DialTimeout("tcp", s.relayTarget, 10*time.Second)
		if relayErr != nil {
			s.log().Warn("relay connect failed, continuing without relay",
				slog.String("protocol", s.name),
				slog.String("target", s.relayTarget),
				slog.Any("error", relayErr),
			)
		} else {
			defer func() { _ = relayConn.Close() }()
			// Drain relay responses so the connection doesn't block.
			go func() { _, _ = io.Copy(io.Discard, relayConn) }()
		}
	}

	scanner := bufio.NewScanner(conn)
	// H02 messages are typically under 200 bytes, but a tracker may batch
	// dozens of messages in a single TCP segment. Set a generous max.
	scanner.Buffer(make([]byte, 8192), 8192)
	if s.scannerSplit != nil {
		scanner.Split(s.scannerSplit)
	}

	var deviceID string

	// Always deregister when this connection ends, regardless of exit path
	// (early return on write error, scanner EOF, context cancel, etc.).
	// Uses a closure so it captures deviceID by reference; at defer-execution
	// time the variable holds the final assigned value (or "" if never set).
	if s.registry != nil {
		defer func() {
			if deviceID != "" {
				s.registry.Deregister(deviceID)
			}
		}()
	}

	for scanner.Scan() {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Reset read deadline on each message.
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

		// Forward raw line to relay target before decode.
		if relayConn != nil {
			_ = relayConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprintf(relayConn, "%s\r\n", line); err != nil {
				s.log().Warn("relay write failed, disabling relay for this connection",
					slog.String("protocol", s.name),
					slog.String("target", s.relayTarget),
					slog.Any("error", err),
				)
				relayConn = nil
			}
		}

		s.log().Debug("rx",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("conn", id),
			slog.String("remoteAddr", remoteAddr),
			slog.String("device", deviceID),
			slog.String("data", line),
		)

		position, devID, response, err := s.decoder(ctx, line)
		if err != nil {
			s.log().Warn("decode error",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.String("conn", id),
				slog.String("remoteAddr", remoteAddr),
				slog.Any("error", err),
			)
			metrics.GPSDecodeErrors.WithLabelValues(s.name).Inc()
			continue
		}
		metrics.GPSMessagesReceived.WithLabelValues(s.name).Inc()

		// Track the device ID for this connection.
		if devID != "" && deviceID == "" {
			deviceID = devID
			// Register the outbound channel so commands can be dispatched to this device.
			if s.registry != nil {
				s.registry.Register(deviceID, outCh)
			}
		}

		// Store and broadcast valid positions.
		if position != nil {
			if err := s.handler.HandlePosition(ctx, position); err != nil {
				s.log().Error("handle position error",
					slog.String("type", "gps"),
					slog.String("protocol", s.name),
					slog.String("conn", id),
					slog.String("device", deviceID),
					slog.Any("error", err),
				)
			}
		}

		// Send protocol response/acknowledgment.
		if response != "" {
			if _, err := fmt.Fprintf(conn, "%s\r\n", response); err != nil {
				s.log().Error("write response error",
					slog.String("type", "gps"),
					slog.String("protocol", s.name),
					slog.String("conn", id),
					slog.Any("error", err),
				)
				return
			}
			s.log().Debug("tx",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.String("conn", id),
				slog.String("remoteAddr", remoteAddr),
				slog.String("device", deviceID),
				slog.String("data", response),
			)
		}
	}

	if err := scanner.Err(); err != nil {
		// Don't log expected errors on shutdown.
		if !strings.Contains(err.Error(), "use of closed network connection") {
			s.log().Warn("scanner error",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.String("remoteAddr", remoteAddr),
				slog.Any("error", err),
			)
		}
	}

	// Mark device offline on disconnect.
	if deviceID != "" {
		s.log().Info("device disconnected",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("conn", id),
			slog.String("device", deviceID),
			slog.String("remoteAddr", remoteAddr),
		)
		s.markDeviceOffline(ctx, deviceID)
	} else {
		s.log().Debug("connection closed without device identification",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("remoteAddr", remoteAddr),
		)
	}
}

// decodeH02 decodes an H02 protocol message line.
func (s *Server) decodeH02(ctx context.Context, line string) (*model.Position, string, string, error) {
	msg, err := h02.Decode(line)
	if err != nil {
		return nil, "", "", err
	}

	// Heartbeat: no position, but acknowledge.
	if msg.Type == "V4" {
		return nil, msg.DeviceID, "", nil
	}

	// SMS: command response from the device.
	if msg.Type == "SMS" {
		if s.commands != nil && msg.Result != "" {
			// Use a detached context with timeout so the goroutine is not
			// cancelled when the connection (and its ctx) closes before the
			// DB write completes.
			go func(deviceID, result string) {
				gCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				device, devErr := s.resolveOrCreateDevice(gCtx, deviceID)
				if devErr != nil {
					s.log().Warn("SMS: cannot resolve device",
						slog.String("uniqueID", deviceID),
						slog.Any("error", devErr),
					)
					return
				}
				cmd, cmdErr := s.commands.GetLatestSentByDevice(gCtx, device.ID)
				if cmdErr != nil {
					s.log().Debug("SMS: no sent command to attach result to",
						slog.String("uniqueID", deviceID),
						slog.Any("error", cmdErr),
					)
					return
				}
				if appendErr := s.commands.AppendResult(gCtx, cmd.ID, result); appendErr != nil {
					s.log().Warn("SMS: failed to append result",
						slog.Int64("commandID", cmd.ID),
						slog.Any("error", appendErr),
					)
				}
			}(msg.DeviceID, msg.Result)
		}
		return nil, msg.DeviceID, "", nil
	}

	// Look up or auto-create the device.
	device, err := s.resolveOrCreateDevice(ctx, msg.DeviceID)
	if err != nil {
		return nil, msg.DeviceID, "", err
	}

	// Build the position model.
	speed := msg.Speed
	course := msg.Course
	altitude := msg.Altitude

	now := time.Now().UTC()
	position := &model.Position{
		DeviceID:   device.ID,
		Protocol:   "h02",
		ServerTime: &now,
		DeviceTime: &msg.Timestamp,
		Timestamp:  msg.Timestamp,
		Valid:      msg.Valid,
		Latitude:   msg.Latitude,
		Longitude:  msg.Longitude,
		Speed:      &speed,
		Course:     &course,
		Altitude:   &altitude,
		Attributes: map[string]interface{}{
			"flags":    msg.Flags,
			"ignition": msg.Ignition,
		},
	}

	// Add cell tower info if present.
	if msg.MCC > 0 {
		position.Attributes["mcc"] = msg.MCC
		position.Attributes["mnc"] = msg.MNC
		position.Attributes["lac"] = msg.LAC
		position.Attributes["cellId"] = msg.CellID
	}

	// Add alarm type if one is active.
	if msg.Alarm != "" {
		position.Attributes["alarm"] = msg.Alarm
	}

	// Add ICCID for V6 messages.
	if msg.ICCID != "" {
		position.Attributes["iccid"] = msg.ICCID
	}

	// Build response: ACK the message type.
	response := h02.EncodeResponse(msg.DeviceID, msg.Type)

	return position, msg.DeviceID, response, nil
}

// decodeWatch decodes a WATCH protocol message line.
func (s *Server) decodeWatch(ctx context.Context, line string) (*model.Position, string, string, error) {
	msg, err := watch.Decode(line)
	if err != nil {
		return nil, "", "", err
	}

	// Build response for heartbeats.
	response := ""
	if msg.Type == "LK" {
		response = watch.EncodeResponse(msg.Manufacturer, msg.DeviceID, "LK")
	}

	// No position data for heartbeats or unknown types.
	if !msg.Valid {
		return nil, msg.DeviceID, response, nil
	}

	// Look up or auto-create the device.
	device, err := s.resolveOrCreateDevice(ctx, msg.DeviceID)
	if err != nil {
		return nil, msg.DeviceID, response, err
	}

	speed := msg.Speed
	course := msg.Course
	nowWatch := time.Now().UTC()

	position := &model.Position{
		DeviceID:   device.ID,
		Protocol:   "watch",
		ServerTime: &nowWatch,
		DeviceTime: &msg.Timestamp,
		Timestamp:  msg.Timestamp,
		Valid:      msg.Valid,
		Latitude:   msg.Latitude,
		Longitude:  msg.Longitude,
		Speed:      &speed,
		Course:     &course,
		Attributes: map[string]interface{}{
			"satellites": msg.Satellites,
		},
	}

	return position, msg.DeviceID, response, nil
}

// markDeviceOffline updates the device status to offline.
func (s *Server) markDeviceOffline(ctx context.Context, uniqueID string) {
	if s.devices == nil {
		return
	}
	device, err := s.devices.GetByUniqueID(ctx, uniqueID)
	if err != nil {
		s.log().Error("cannot mark device offline",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("uniqueID", uniqueID),
			slog.Any("error", err),
		)
		return
	}

	now := time.Now().UTC()
	device.Status = "offline"
	device.LastUpdate = &now
	if err := s.devices.Update(ctx, device); err != nil {
		s.log().Error("failed to mark device offline",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("uniqueID", uniqueID),
			slog.Any("error", err),
		)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func ptrFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// h02SplitFunc is a bufio.SplitFunc that extracts individual H02 protocol
// messages. H02 messages are framed as *...# (start with *, end with #).
// Real GPS trackers may send multiple messages concatenated in a single TCP
// segment without newline separators. This split function handles both
// newline-separated and concatenated messages.
func h02SplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Find the start of an H02 message.
	start := bytes.IndexByte(data, '*')
	if start < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		// Discard bytes before the next start marker but request more data.
		return 0, nil, nil
	}

	// Find the end marker '#' after the start.
	end := bytes.IndexByte(data[start+1:], '#')
	if end < 0 {
		if atEOF {
			// Incomplete message at EOF, discard.
			return len(data), nil, nil
		}
		// Need more data to find the end marker.
		return 0, nil, nil
	}

	// Token is from * to # inclusive.
	tokenEnd := start + 1 + end + 1
	return tokenEnd, data[start:tokenEnd], nil
}
