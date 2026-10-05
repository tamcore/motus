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
		return
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

// --- messageDelay ---

func TestMessageDelay(t *testing.T) {
	tests := []struct {
		speed float64
		want  time.Duration
	}{
		{1.0, 5000 * time.Millisecond},
		{2.0, 2500 * time.Millisecond},
		{0.5, 10000 * time.Millisecond},
	}
	for _, tt := range tests {
		if got := messageDelay(tt.speed); got != tt.want {
			t.Errorf("messageDelay(%.1f) = %v, want %v", tt.speed, got, tt.want)
		}
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

	config := &Config{InputFile: f.Name(), InputType: "pcap"}
	msgs, err := extractMessages(config)
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

	config := &Config{InputFile: f.Name(), InputType: "pcap", DeviceID: "REPLACED"}
	msgs, err := extractMessages(config)
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
