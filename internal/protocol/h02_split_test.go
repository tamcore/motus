package protocol

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestH02SplitFunc_SingleMessage(t *testing.T) {
	input := "*HQ,123,V1,data#"
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(h02SplitFunc)

	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if len(tokens) != 1 || tokens[0] != input {
		t.Errorf("got %v, want [%q]", tokens, input)
	}
}

func TestH02SplitFunc_ConcatenatedMessages(t *testing.T) {
	// Simulates a real GPS tracker sending multiple messages without newlines.
	messages := []string{
		"*HQ,9172876323,V1,153714,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153720,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153725,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
	}
	input := strings.Join(messages, "")

	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(h02SplitFunc)

	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if len(tokens) != len(messages) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(messages))
	}
	for i, want := range messages {
		if tokens[i] != want {
			t.Errorf("token %d: got %q, want %q", i, tokens[i], want)
		}
	}
}

func TestH02SplitFunc_WithNewlines(t *testing.T) {
	// Messages separated by \r\n (demo simulator style).
	messages := []string{
		"*HQ,123,V1,data1#",
		"*HQ,123,V1,data2#",
	}
	input := strings.Join(messages, "\r\n") + "\r\n"

	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(h02SplitFunc)

	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	if len(tokens) != len(messages) {
		t.Fatalf("got %d tokens, want %d", len(tokens), len(messages))
	}
	for i, want := range messages {
		if tokens[i] != want {
			t.Errorf("token %d: got %q, want %q", i, tokens[i], want)
		}
	}
}

func TestH02SplitFunc_GarbageBetweenMessages(t *testing.T) {
	input := "garbage*HQ,123,V1,data#morejunk*HQ,456,V4,V1,ts#"
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(h02SplitFunc)

	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if len(tokens) != 2 {
		t.Fatalf("got %d tokens, want 2: %v", len(tokens), tokens)
	}
	if tokens[0] != "*HQ,123,V1,data#" {
		t.Errorf("token 0: got %q", tokens[0])
	}
	if tokens[1] != "*HQ,456,V4,V1,ts#" {
		t.Errorf("token 1: got %q", tokens[1])
	}
}

func TestH02SplitFunc_IncompleteMessage(t *testing.T) {
	// Incomplete message at EOF should be discarded.
	input := "*HQ,123,V1,data#*HQ,456,V1,no_end"
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(h02SplitFunc)

	var tokens []string
	for scanner.Scan() {
		tokens = append(tokens, scanner.Text())
	}
	if len(tokens) != 1 {
		t.Fatalf("got %d tokens, want 1: %v", len(tokens), tokens)
	}
	if tokens[0] != "*HQ,123,V1,data#" {
		t.Errorf("token 0: got %q", tokens[0])
	}
}

func TestH02SplitFunc_EmptyInput(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader(""))
	scanner.Split(h02SplitFunc)

	var count int
	for scanner.Scan() {
		count++
	}
	if count != 0 {
		t.Errorf("got %d tokens from empty input", count)
	}
}

// TestServer_H02BatchedMessages verifies that concatenated H02 messages
// (sent without newlines) are each decoded and processed individually.
func TestServer_H02BatchedMessages(t *testing.T) {
	decoded := make(chan string, 20)

	srv := &Server{
		name:         "h02",
		port:         "0",
		handler:      &PositionHandler{},
		scannerSplit: h02SplitFunc,
		decoder: func(_ context.Context, line string) (*model.Position, string, string, error) {
			decoded <- line
			return nil, "dev", "ACK", nil
		},
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Send 5 concatenated messages in a single TCP write (no newlines).
	messages := []string{
		"*HQ,9172876323,V1,153714,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153720,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153725,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153730,A,4832.2788,N,01744.1399,E,000.00,000,040426,FBFFDFFF,231,06,42302,3218278#",
		"*HQ,9172876323,V1,153736,A,4832.2788,N,01744.1399,E,000.00,000,040426,FFFFDFFF,231,06,42302,3218278#",
	}
	batch := strings.Join(messages, "")
	if _, err := fmt.Fprint(conn, batch); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Collect decoded messages.
	var received []string
	deadline := time.After(3 * time.Second)
	for len(received) < len(messages) {
		select {
		case line := <-decoded:
			received = append(received, line)
		case <-deadline:
			t.Fatalf("timeout: decoded %d/%d messages", len(received), len(messages))
		}
	}

	_ = conn.Close()
	cancel()
	srv.activeConns.Wait()

	for i, want := range messages {
		if received[i] != want {
			t.Errorf("message %d:\n  got  %q\n  want %q", i, received[i], want)
		}
	}
}

// TestServer_H02BatchedRelay verifies that each message in a concatenated
// batch is individually relayed.
func TestServer_H02BatchedRelay(t *testing.T) {
	relayAddr, relayLines := startMockRelay(t)

	srv := &Server{
		name:         "h02",
		port:         "0",
		relayTarget:  relayAddr,
		scannerSplit: h02SplitFunc,
		decoder:      noopDecoder(nil),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv.listener = listener
	port := listener.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.acceptLoop(ctx)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	messages := []string{
		"*HQ,123,V1,data1#",
		"*HQ,123,V1,data2#",
		"*HQ,123,V1,data3#",
	}
	// Send all concatenated without newlines.
	batch := strings.Join(messages, "")
	if _, err := fmt.Fprint(conn, batch); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Each message should be individually relayed.
	var received []string
	deadline := time.After(3 * time.Second)
	for len(received) < len(messages) {
		select {
		case line, ok := <-relayLines:
			if !ok {
				t.Fatalf("relay closed after %d/%d messages", len(received), len(messages))
			}
			received = append(received, line)
		case <-deadline:
			t.Fatalf("timeout: relayed %d/%d messages", len(received), len(messages))
		}
	}

	_ = conn.Close()
	cancel()
	srv.activeConns.Wait()

	for i, want := range messages {
		if received[i] != want {
			t.Errorf("relay message %d: got %q, want %q", i, received[i], want)
		}
	}
}
