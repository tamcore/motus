package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// captureSink records every event sent by the service.
type captureSink struct {
	events []ChatEvent
}

func (c *captureSink) Send(event ChatEvent) error {
	c.events = append(c.events, event)
	return nil
}

func (c *captureSink) Flush() error { return nil }

func (c *captureSink) errorEvent() (ChatEvent, bool) {
	for _, e := range c.events {
		if e.Type == "error" {
			return e, true
		}
	}
	return ChatEvent{}, false
}

// memHistory is an in-memory HistoryHandle.
type memHistory struct {
	msgs []Message
}

func (m *memHistory) Messages() []Message { return m.msgs }

func (m *memHistory) Append(_ context.Context, msgs ...Message) error {
	m.msgs = append(m.msgs, msgs...)
	return nil
}

func newTestService(t *testing.T, baseURL string, timeout time.Duration) *Service {
	t.Helper()
	return NewService(Config{
		BaseURL:   baseURL,
		APIKey:    "test-key",
		Model:     "test-model",
		MaxTokens: 128,
		MaxLoops:  2,
		Timeout:   timeout,
		MCPServer: mcpserver.NewMCPServer("test", "0.0.0"),
	})
}

func TestStream_UpstreamError_SendsGenericMessage(t *testing.T) {
	const sentinel = "sk-secret-internal-detail"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"` + sentinel + `"}}`))
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, 30*time.Second)
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "hello"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	errEvent, ok := sink.errorEvent()
	if !ok {
		t.Fatal("expected an error event in the SSE stream")
	}
	if strings.Contains(errEvent.Message, sentinel) {
		t.Errorf("error event leaks upstream error detail: %q", errEvent.Message)
	}
	if errEvent.Message != genericErrorMessage {
		t.Errorf("expected generic error message %q, got %q", genericErrorMessage, errEvent.Message)
	}
}

func TestStream_Timeout_SendsTimeoutMessage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, 100*time.Millisecond)
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "hello"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	errEvent, ok := sink.errorEvent()
	if !ok {
		t.Fatal("expected an error event in the SSE stream")
	}
	if errEvent.Message != timeoutErrorMessage {
		t.Errorf("expected timeout message %q, got %q", timeoutErrorMessage, errEvent.Message)
	}
}
