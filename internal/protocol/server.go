package protocol

import (
	"bufio"
	"bytes"
	"cmp"
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

// defaultMaxConnections is the maximum number of concurrent GPS device connections.
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

	// deviceCache maps unique ID to cachedDevice for deviceCacheTTL.
	deviceCache sync.Map

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

	// Optional: maximum size of a single frame. When zero, defaultMaxFrameSize is used.
	maxFrameSize int

	// rawFrames disables the CRLF terminator on responses and relayed frames,
	// for protocols whose frames are self-delimiting (WATCH).
	rawFrames bool
}

// defaultMaxFrameSize is the default scanner buffer size. H02 messages are
// typically under 200 bytes, but a tracker may batch dozens of messages in a
// single TCP segment.
const defaultMaxFrameSize = 8192

// watchMaxFrameSize bounds a single WATCH frame. Frames can carry binary voice
// and image payloads, so they are much larger than position reports.
const watchMaxFrameSize = 1 << 20

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
		scannerSplit:   watch.SplitFunc,
		maxFrameSize:   watchMaxFrameSize,
		rawFrames:      true,
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

// deviceCacheTTL bounds how long a resolved device is reused without reading it
// again, so renames, deletions and protocol changes apply within this window.
const deviceCacheTTL = 30 * time.Second

type cachedDevice struct {
	device  model.Device
	expires time.Time
}

// resolveOrCreateDevice is lookupOrCreateDevice with a short per-server cache,
// so a connected device is not looked up on every message.
func (s *Server) resolveOrCreateDevice(ctx context.Context, uniqueID string) (*model.Device, error) {
	if v, ok := s.deviceCache.Load(uniqueID); ok {
		if c := v.(cachedDevice); time.Now().Before(c.expires) {
			return new(c.device), nil
		}
	}
	device, err := s.lookupOrCreateDevice(ctx, uniqueID)
	if err != nil {
		return nil, err
	}
	s.deviceCache.Store(uniqueID, cachedDevice{*device, time.Now().Add(deviceCacheTTL)})
	return device, nil
}

// lookupOrCreateDevice looks up a device by unique ID. If the device is not
// found and auto-creation is enabled, it creates the device and assigns it to
// the configured default user. Returns the device or an error.
func (s *Server) lookupOrCreateDevice(ctx context.Context, uniqueID string) (*model.Device, error) {
	device, err := s.devices.GetByUniqueID(ctx, uniqueID)
	if err == nil {
		if device.Protocol != s.name {
			if upErr := s.devices.UpdateProtocol(ctx, device.ID, s.name); upErr != nil {
				s.log().Warn("failed to resync device protocol",
					slog.String("type", "gps"),
					slog.String("protocol", s.name),
					slog.String("uniqueID", uniqueID),
					slog.String("oldProtocol", device.Protocol),
					slog.Any("error", upErr),
				)
			} else {
				device.Protocol = s.name
			}
		}
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
	// Close on shutdown so the device sees the close while this pod is still
	// reachable and reconnects to another one.
	defer context.AfterFunc(ctx, func() { _ = conn.Close() })()

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
	// outCh is intentionally never closed — closing it from the consumer side
	// would race with in-flight Send calls and cause a send-on-closed-channel
	// panic. The done channel signals the writer goroutine to exit instead.
	outCh := make(chan []byte, 16)
	done := make(chan struct{})
	defer close(done)

	// Write goroutine: reads from outCh and sends to the device connection.
	// On write error, closes the connection so the scanner loop exits
	// naturally and runs the post-loop cleanup (Deregister, markDeviceOffline).
	go func() {
		for {
			select {
			case <-done:
				return
			case data := <-outCh:
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
		}
	}()

	// Relay client: lazily dials on first message, reconnects transparently on
	// broken-pipe errors. nil when no relay is configured; safe to call methods
	// on a nil pointer.
	var relay *relayClient
	if s.relayTarget != "" {
		relay = &relayClient{target: s.relayTarget, protocol: s.name, logger: s.log(), raw: s.rawFrames}
		defer relay.close()
	}

	scanner := bufio.NewScanner(conn)
	maxFrameSize := cmp.Or(s.maxFrameSize, defaultMaxFrameSize)
	scanner.Buffer(make([]byte, min(maxFrameSize, defaultMaxFrameSize)), maxFrameSize)
	if s.scannerSplit != nil {
		scanner.Split(s.scannerSplit)
	}

	var deviceID string

	// Per-connection protocol session, filled in by decoders that need it.
	session := &DeviceSession{}
	var publishedSession DeviceSession
	decodeCtx := context.WithValue(ctx, connSessionKey{}, session)

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

		// Forward raw line to relay target before decode. Lazy dial + reconnect:
		// the relay TCP is opened on the first message and re-dialed transparently
		// if the existing conn returns a broken-pipe-style error. Probes that
		// connect to motus but never send data therefore never touch Traccar.
		relay.send(line)

		s.log().Debug("rx",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("conn", id),
			slog.String("remoteAddr", remoteAddr),
			slog.String("device", deviceID),
			slog.String("data", truncate(line, maxLoggedFrame)),
		)

		position, devID, response, err := s.decoder(decodeCtx, line)
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

		// Publish protocol session details (e.g. WATCH manufacturer) so
		// commands are encoded in the format this device speaks.
		if s.registry != nil && deviceID != "" && *session != publishedSession {
			s.registry.SetSession(deviceID, *session)
			publishedSession = *session
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
			if !s.rawFrames {
				response += "\r\n"
			}
			if _, err := io.WriteString(conn, response); err != nil {
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
			// WithoutCancel inherits trace spans from the connection context but
			// is not cancelled when the connection closes before the DB write
			// completes.
			go func(deviceID, result string) {
				gCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
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
		Speed:      new(msg.Speed),
		Course:     new(msg.Course),
		Altitude:   new(msg.Altitude),
		Attributes: map[string]any{
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

// decodeWatch decodes a single WATCH protocol frame.
//
// Position reports (UD*, AL*, WT*) look up or auto-create the device. Other
// messages (LK, INIT, TKQ, ...) are acknowledged and only mark an already
// known device online. LK heartbeats carrying battery/steps and health
// measurements (heart rate, blood pressure, temperature, SpO2) produce a
// position at the last known location, like Traccar.
func (s *Server) decodeWatch(ctx context.Context, line string) (*model.Position, string, string, error) {
	msg, err := watch.Decode(line)
	if err != nil {
		return nil, "", "", err
	}

	// Remember how the device frames its messages, like Traccar's decoder
	// state, so commands can be sent in the same format.
	if session, ok := ctx.Value(connSessionKey{}).(*DeviceSession); ok {
		session.Manufacturer = msg.Manufacturer
		if msg.Index != "" {
			session.Indexed = true
		}
	}

	if msg.PositionErr != nil {
		s.log().Warn("watch position decode error",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("device", msg.DeviceID),
			slog.String("messageType", msg.Type),
			slog.Any("error", msg.PositionErr),
		)
		metrics.GPSDecodeErrors.WithLabelValues(s.name).Inc()
	}

	var device *model.Device
	if msg.HasPosition() {
		if s.devices == nil {
			return nil, msg.DeviceID, "", fmt.Errorf("unknown device %s: no device repository", msg.DeviceID)
		}
		device, err = s.resolveOrCreateDevice(ctx, msg.DeviceID)
		if err != nil {
			return nil, msg.DeviceID, "", err
		}
	} else if s.devices != nil {
		// Non-position messages do not auto-create devices.
		device, _ = s.devices.GetByUniqueID(ctx, msg.DeviceID)
	}
	if device == nil {
		return nil, msg.DeviceID, msg.Response, nil
	}

	if !msg.IsDeviceInitiated() {
		s.recordWatchCommandReply(ctx, device, msg)
	}

	var position *model.Position
	switch {
	case msg.Position != nil:
		position = s.watchPosition(ctx, device, msg.Position)
	case msg.Attributes != nil:
		position = s.watchLastKnownPosition(ctx, device, msg.Attributes)
	}
	if position == nil {
		s.handler.MarkOnline(ctx, device)
	}

	return position, msg.DeviceID, msg.Response, nil
}

// watchReplyLookback is how many recent commands are searched for the one a
// WATCH reply belongs to.
const watchReplyLookback = 10

// recordWatchCommandReply stores a WATCH command reply as the result of the
// command it answers. Watches reply by echoing the command keyword (e.g.
// [3G*id*0006*UPLOAD] for UPLOAD,60), so the reply is attached to the newest
// command still in "sent" state whose keyword matches the message type; the
// first result marks the command executed.
func (s *Server) recordWatchCommandReply(ctx context.Context, device *model.Device, msg *watch.Message) {
	if s.commands == nil {
		return
	}
	cmds, err := s.commands.ListByDevice(ctx, device.ID, watchReplyLookback)
	if err != nil {
		s.log().Warn("command reply: cannot list commands",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("device", msg.DeviceID),
			slog.Any("error", err),
		)
		return
	}

	for _, cmd := range cmds { // newest first
		if cmd.Status != model.CommandStatusSent || !strings.EqualFold(watchCommandKeyword(cmd), msg.Type) {
			continue
		}
		result := msg.Type
		if msg.Content != "" {
			result += "," + msg.Content
		}
		if err := s.commands.AppendResult(ctx, cmd.ID, result); err != nil {
			s.log().Warn("command reply: failed to store result",
				slog.String("type", "gps"),
				slog.String("protocol", s.name),
				slog.Int64("commandID", cmd.ID),
				slog.Any("error", err),
			)
			return
		}
		s.log().Debug("command reply recorded",
			slog.String("type", "gps"),
			slog.String("protocol", s.name),
			slog.String("device", msg.DeviceID),
			slog.Int64("commandID", cmd.ID),
			slog.String("result", truncate(result, maxLoggedFrame)),
		)
		return
	}
}

// watchPosition converts a decoded WATCH position report to a model position.
//
// Watches without a fix often report 0,0. Such reports still carry fresh
// status data (battery, alarms such as SOS), so they are stored at the last
// known location instead of being dropped or shown at 0,0. Without a last
// known location they are only stored when they carry an alarm.
func (s *Server) watchPosition(ctx context.Context, device *model.Device, p *watch.Position) *model.Position {
	now := time.Now().UTC()
	position := &model.Position{
		DeviceID:   device.ID,
		Protocol:   "watch",
		ServerTime: &now,
		DeviceTime: new(p.Timestamp),
		Timestamp:  p.Timestamp,
		Valid:      p.Valid,
		Latitude:   p.Latitude,
		Longitude:  p.Longitude,
		Speed:      new(p.Speed),
		Course:     new(p.Course),
		Altitude:   new(p.Altitude),
		Network:    p.Network,
		Attributes: p.Attributes,
	}

	if p.Latitude != 0 || p.Longitude != 0 {
		return position
	}

	if last := s.handler.LastPosition(ctx, device.ID); last != nil {
		applyLastLocation(position, last)
		return position
	}
	if p.Alarm() != "" {
		return position
	}
	return nil
}

// watchLastKnownPosition builds a position at the device's last known
// location carrying the given attributes (Traccar's getLastLocation). Returns
// nil when the device has no stored position yet.
func (s *Server) watchLastKnownPosition(ctx context.Context, device *model.Device, attrs map[string]any) *model.Position {
	last := s.handler.LastPosition(ctx, device.ID)
	if last == nil {
		return nil
	}
	now := time.Now().UTC()
	position := &model.Position{
		DeviceID:   device.ID,
		Protocol:   "watch",
		ServerTime: &now,
		DeviceTime: &now,
		Attributes: attrs,
	}
	applyLastLocation(position, last)
	return position
}

// applyLastLocation copies the fix of a previous position onto p and marks p
// as outdated.
func applyLastLocation(p, last *model.Position) {
	p.Timestamp = last.Timestamp
	p.Valid = last.Valid
	p.Latitude = last.Latitude
	p.Longitude = last.Longitude
	p.Altitude = last.Altitude
	p.Speed = last.Speed
	p.Course = last.Course
	p.Accuracy = last.Accuracy
	p.Outdated = true
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

// relayDialTimeout bounds how long the relay client waits when establishing a
// TCP connection to the relay target. Kept short so a dead Traccar doesn't
// stall device handling.
const relayDialTimeout = 3 * time.Second

// relayWriteTimeout bounds how long a single relay write may block.
const relayWriteTimeout = 3 * time.Second

// relayClient is a per-device-connection helper that forwards H02 frames to a
// configured relay target (typically Traccar). It dials lazily on the first
// frame and reconnects transparently if the existing conn returns a write
// error. All errors are non-fatal: they are logged and the device session
// continues. Not safe for concurrent use; one instance per connection
// goroutine.
type relayClient struct {
	target   string
	protocol string
	logger   *slog.Logger
	conn     net.Conn
	// raw forwards frames verbatim, without appending CRLF.
	raw bool
}

// send forwards a single line to the relay target with CRLF termination
// (or verbatim when raw is set).
// On a write error the stale conn is closed and a redial is attempted once.
// Safe to call on a nil receiver — that case is a no-op so callers can use a
// single code path whether relay is configured or not.
func (r *relayClient) send(line string) {
	if r == nil {
		return
	}
	data := []byte(line)
	if !r.raw {
		data = append(data, "\r\n"...)
	}

	if r.conn != nil {
		_ = r.conn.SetWriteDeadline(time.Now().Add(relayWriteTimeout))
		if _, err := r.conn.Write(data); err == nil {
			return
		}
		_ = r.conn.Close()
		r.conn = nil
	}

	conn, err := net.DialTimeout("tcp", r.target, relayDialTimeout)
	if err != nil {
		r.logger.Warn("relay dial failed",
			slog.String("protocol", r.protocol),
			slog.String("target", r.target),
			slog.Any("error", err),
		)
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(relayWriteTimeout))
	if _, err := conn.Write(data); err != nil {
		r.logger.Warn("relay write failed after redial",
			slog.String("protocol", r.protocol),
			slog.String("target", r.target),
			slog.Any("error", err),
		)
		_ = conn.Close()
		return
	}
	r.conn = conn
	go func(c net.Conn) { _, _ = io.Copy(io.Discard, c) }(conn)
}

// close terminates the active relay connection, if any. Safe to call on a nil
// receiver.
func (r *relayClient) close() {
	if r == nil || r.conn == nil {
		return
	}
	_ = r.conn.Close()
	r.conn = nil
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

// maxLoggedFrame bounds how much of a received frame is written to the debug
// log. WATCH voice and image frames carry up to watchMaxFrameSize bytes of
// binary data.
const maxLoggedFrame = 1024

// connSessionKey is the context key under which handleConnection passes the
// per-connection *DeviceSession to decoders.
type connSessionKey struct{}
