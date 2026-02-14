// Package gpsreplay sends GPS protocol messages from a log file or pcap to a
// GPS server to simulate live device traffic. Useful for testing and
// demonstration.
package gpsreplay

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Config holds replay CLI flags.
type Config struct {
	InputFile  string
	InputType  string // "h02log" or "pcap"
	ServerHost string
	ServerPort int
	DeviceID   string  // Device unique ID to attribute messages to
	Speed      float64 // Playback speed multiplier (1.0 = realtime, 0 = instant)
	Loop       bool    // Loop messages indefinitely
	Verbose    bool
}

// NewCmd returns a cobra command for the replay subcommand.
func NewCmd() *cobra.Command {
	config := &Config{}

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay GPS protocol messages from a log file",
		Long: `Send GPS protocol messages from a log file or pcap capture to a GPS server,
simulating live device traffic. Useful for testing, demonstration, and load testing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := runReplay(config); err != nil {
				slog.Error("replay failed", slog.Any("error", err))
				os.Exit(1)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&config.InputFile, "input", "", "Input file (h02.log or .pcap)")
	f.StringVar(&config.InputType, "type", "h02log", "Input type: h02log or pcap")
	f.StringVar(&config.ServerHost, "host", "localhost", "GPS server host")
	f.IntVar(&config.ServerPort, "port", 5013, "GPS server port (5013 for H02)")
	f.StringVar(&config.DeviceID, "device-id", "", "Device unique ID")
	f.Float64Var(&config.Speed, "speed", 0, "Playback speed multiplier (0=instant, 1=realtime, 2=2x speed)")
	f.BoolVar(&config.Loop, "loop", false, "Loop messages indefinitely")
	f.BoolVar(&config.Verbose, "verbose", false, "Verbose logging")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}

func runReplay(config *Config) error {
	// Extract messages from input file
	messages, err := extractMessages(config)
	if err != nil {
		return fmt.Errorf("extract messages: %w", err)
	}

	if len(messages) == 0 {
		return fmt.Errorf("no messages found in %s", config.InputFile)
	}

	slog.Info("extracted messages",
		slog.Int("count", len(messages)),
		slog.String("file", config.InputFile))
	slog.Info("replay target",
		slog.String("host", config.ServerHost),
		slog.Int("port", config.ServerPort),
		slog.String("deviceID", config.DeviceID))

	// Connect to GPS server
	conn, err := net.Dial("tcp", net.JoinHostPort(config.ServerHost, fmt.Sprintf("%d", config.ServerPort)))
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer func() { _ = conn.Close() }()

	slog.Info("connected to GPS server")

	// Calculate inter-message delays if realtime playback
	var delays []time.Duration
	if config.Speed > 0 {
		delays = calculateDelays(messages, config.Speed)
	}

	// Send messages
	count := 0
	for {
		for i, msg := range messages {
			// Apply delay if realtime playback
			if config.Speed > 0 && i > 0 && i < len(delays) {
				time.Sleep(delays[i])
			}

			// Send message
			_, err := conn.Write([]byte(msg + "\r\n"))
			if err != nil {
				return fmt.Errorf("send message %d: %w", count, err)
			}

			count++
			if config.Verbose {
				slog.Debug("sent message",
					slog.Int("seq", count),
					slog.String("msg", truncate(msg, 80)))
			} else if count%100 == 0 {
				slog.Info("replay progress", slog.Int("sent", count))
			}

			// Read response (optional)
			if config.Verbose {
				_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				buf := make([]byte, 1024)
				n, _ := conn.Read(buf)
				if n > 0 {
					slog.Debug("server response",
						slog.Int("seq", count),
						slog.String("response", truncate(string(buf[:n]), 80)))
				}
			}
		}

		if !config.Loop {
			break
		}
		slog.Info("looping replay")
	}

	slog.Info("replay complete", slog.Int("messagesSent", count))
	return nil
}

// extractMessages extracts H02 protocol messages from input file
func extractMessages(config *Config) ([]string, error) {
	f, err := os.Open(config.InputFile)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var messages []string

	switch config.InputType {
	case "h02log":
		messages, err = extractFromH02Log(f, config)
	case "pcap":
		messages, err = extractFromPcap(config)
	default:
		return nil, fmt.Errorf("unsupported input type: %s", config.InputType)
	}

	return messages, err
}

// extractFromH02Log extracts H02 messages from h02.log format
func extractFromH02Log(f *os.File, config *Config) ([]string, error) {
	var messages []string
	scanner := bufio.NewScanner(f)

	// H02 message pattern: *HQ,...#
	h02Pattern := regexp.MustCompile(`\*HQ,[^#]+#`)

	for scanner.Scan() {
		line := scanner.Text()

		// Extract H02 messages from line
		matches := h02Pattern.FindAllString(line, -1)
		for _, match := range matches {
			// Replace device ID if specified
			if config.DeviceID != "" {
				// H02 format: *HQ,<device_id>,<command>,...#
				// Replace device ID in message
				parts := strings.Split(match, ",")
				if len(parts) >= 2 {
					parts[1] = config.DeviceID
					match = strings.Join(parts, ",")
				}
			}
			messages = append(messages, match)
		}
	}

	return messages, scanner.Err()
}

// extractFromPcap extracts H02 messages from pcap file
// This is a simple implementation that reads pcap as text and extracts H02 patterns
func extractFromPcap(config *Config) ([]string, error) {
	// Read pcap file as binary
	data, err := os.ReadFile(config.InputFile)
	if err != nil {
		return nil, err
	}

	// Extract H02 messages from pcap payload
	// H02 messages are typically in the TCP payload
	h02Pattern := regexp.MustCompile(`\*HQ,[^#]+#`)
	matches := h02Pattern.FindAllString(string(data), -1)

	var messages []string
	for _, match := range matches {
		// Replace device ID if specified
		if config.DeviceID != "" {
			parts := strings.Split(match, ",")
			if len(parts) >= 2 {
				parts[1] = config.DeviceID
				match = strings.Join(parts, ",")
			}
		}
		messages = append(messages, match)
	}

	return messages, nil
}

// calculateDelays calculates inter-message delays based on timestamps
// For now, just use a fixed delay since extracting timestamps from H02 log is complex
func calculateDelays(messages []string, speedMultiplier float64) []time.Duration {
	delays := make([]time.Duration, len(messages))

	// For H02 replay, use a fixed 5-second interval between messages (typical GPS update rate)
	baseDelay := time.Duration(5000/speedMultiplier) * time.Millisecond

	for i := range delays {
		delays[i] = baseDelay
	}

	return delays
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
