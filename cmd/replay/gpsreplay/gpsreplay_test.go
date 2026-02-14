package gpsreplay

import (
	"os"
	"testing"
	"time"
)

// --- NewCmd ---

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()
	if cmd == nil {
		t.Fatal("NewCmd returned nil")
	}
	if cmd.Use != "replay" {
		t.Errorf("Use = %q, want %q", cmd.Use, "replay")
	}
	for _, flag := range []string{"input", "type", "host", "port", "device-id", "speed", "loop", "verbose"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("flag %q not registered", flag)
		}
	}
	// --input is required
	if cmd.Flags().Lookup("input").DefValue != "" {
		t.Errorf("input default should be empty, got %q", cmd.Flags().Lookup("input").DefValue)
	}
}

// --- truncate ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"", 10, ""},
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"abcdef", 3, "abc..."},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

// --- calculateDelays ---

func TestCalculateDelays(t *testing.T) {
	tests := []struct {
		n     int
		speed float64
		want  time.Duration
	}{
		{3, 1.0, 5000 * time.Millisecond},
		{3, 2.0, 2500 * time.Millisecond},
		{3, 0.5, 10000 * time.Millisecond},
	}

	for _, tt := range tests {
		messages := make([]string, tt.n)
		delays := calculateDelays(messages, tt.speed)
		if len(delays) != tt.n {
			t.Errorf("calculateDelays(n=%d, speed=%.1f): got %d delays, want %d",
				tt.n, tt.speed, len(delays), tt.n)
			continue
		}
		for i, d := range delays {
			if d != tt.want {
				t.Errorf("calculateDelays(n=%d, speed=%.1f)[%d] = %v, want %v",
					tt.n, tt.speed, i, d, tt.want)
			}
		}
	}
}

func TestCalculateDelays_Empty(t *testing.T) {
	delays := calculateDelays(nil, 1.0)
	if len(delays) != 0 {
		t.Errorf("calculateDelays(nil): got %d delays, want 0", len(delays))
	}
}

// --- extractFromH02Log ---

func TestExtractFromH02Log(t *testing.T) {
	content := `2024-01-01 12:00:00 recv: *HQ,123456789012,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#
some other log line
*HQ,123456789012,LINK,120000,0,0,0,0,0#
`
	f, err := os.CreateTemp("", "h02log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f2.Close() }()

	config := &Config{}
	msgs, err := extractFromH02Log(f2, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("got %d messages, want 2", len(msgs))
	}
}

func TestExtractFromH02Log_DeviceIDReplacement(t *testing.T) {
	content := "*HQ,ORIGINAL123,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#\n"

	f, err := os.CreateTemp("", "h02log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f2.Close() }()

	config := &Config{DeviceID: "NEWDEVICE"}
	msgs, err := extractFromH02Log(f2, config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	// Device ID should be replaced
	expected := "*HQ,NEWDEVICE,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#"
	if msgs[0] != expected {
		t.Errorf("got %q, want %q", msgs[0], expected)
	}
}

func TestExtractFromH02Log_Empty(t *testing.T) {
	f, err := os.CreateTemp("", "h02log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	f2, err := os.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f2.Close() }()

	msgs, err := extractFromH02Log(f2, &Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages, want 0", len(msgs))
	}
}

// --- extractFromPcap ---

func TestExtractFromPcap(t *testing.T) {
	// Write binary file that contains embedded H02 messages
	content := []byte("some binary header\x00\x01*HQ,PCAP001,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#\x00\x02more binary")

	f, err := os.CreateTemp("", "pcap-*.pcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	config := &Config{InputFile: f.Name()}
	msgs, err := extractFromPcap(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
}

func TestExtractFromPcap_DeviceIDReplacement(t *testing.T) {
	content := []byte("*HQ,ORIGINAL,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#")

	f, err := os.CreateTemp("", "pcap-*.pcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	config := &Config{InputFile: f.Name(), DeviceID: "REPLACED"}
	msgs, err := extractFromPcap(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0] != "*HQ,REPLACED,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#" {
		t.Errorf("unexpected message: %q", msgs[0])
	}
}

func TestExtractFromPcap_NotFound(t *testing.T) {
	config := &Config{InputFile: "/nonexistent/file.pcap"}
	_, err := extractFromPcap(config)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// --- extractMessages ---

func TestExtractMessages_H02Log(t *testing.T) {
	content := "*HQ,123456789,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#\n"

	f, err := os.CreateTemp("", "h02log-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	config := &Config{InputFile: f.Name(), InputType: "h02log"}
	msgs, err := extractMessages(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("got %d messages, want 1", len(msgs))
	}
}

func TestExtractMessages_Pcap(t *testing.T) {
	content := []byte("*HQ,PCAPDEV,V1,120000,A,5000.0000,N,00800.0000,E,000,000,010124,FFFFFFFF#")

	f, err := os.CreateTemp("", "pcap-*.pcap")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	config := &Config{InputFile: f.Name(), InputType: "pcap"}
	msgs, err := extractMessages(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("got %d messages, want 1", len(msgs))
	}
}

func TestExtractMessages_UnsupportedType(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	config := &Config{InputFile: f.Name(), InputType: "unknown"}
	_, err = extractMessages(config)
	if err == nil {
		t.Error("expected error for unsupported input type")
	}
}

func TestExtractMessages_FileNotFound(t *testing.T) {
	config := &Config{InputFile: "/nonexistent/file.txt", InputType: "h02log"}
	_, err := extractMessages(config)
	if err == nil {
		t.Error("expected error for missing file")
	}
}
