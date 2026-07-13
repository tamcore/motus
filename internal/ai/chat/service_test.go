package chat

import (
	"context"
	"encoding/json"
	"io"
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

// guardrailUpstream routes mock requests: non-streaming bodies hit the
// classifier handler, streaming bodies ("stream":true) hit the main handler.
// It counts requests per path so tests can assert which model ran.
type guardrailUpstream struct {
	classifierReply  string
	classifierStatus int
	classifierCalls  int
	mainCalls        int
}

func (g *guardrailUpstream) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &req)

		if !req.Stream {
			g.classifierCalls++
			if g.classifierStatus != 0 && g.classifierStatus != http.StatusOK {
				w.WriteHeader(g.classifierStatus)
				_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(completionResponse(g.classifierReply)))
			return
		}

		g.mainCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"main answer"},"finish_reason":"stop"}]}` +
				"\n\ndata: [DONE]\n\n"))
	}
}

func TestStream_Guardrail_OffTopic_Refuses(t *testing.T) {
	up := &guardrailUpstream{classifierReply: "OFF_TOPIC"}
	upstream := httptest.NewServer(up.handler(t))
	defer upstream.Close()

	svc := newGuardrailService(t, upstream.URL)
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "what is the capital of France?"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if up.mainCalls != 0 {
		t.Errorf("main model called %d times, want 0", up.mainCalls)
	}
	if len(sink.events) != 2 {
		t.Fatalf("expected exactly 2 events (token, done), got %d: %+v", len(sink.events), sink.events)
	}
	if sink.events[0].Type != "token" || sink.events[0].Delta != refusalMessage {
		t.Errorf("first event = %+v, want token with refusal message", sink.events[0])
	}
	if sink.events[1].Type != "done" {
		t.Errorf("second event type = %q, want done", sink.events[1].Type)
	}
	last := hist.msgs[len(hist.msgs)-1]
	if last.Role != "assistant" || last.Content != refusalMessage {
		t.Errorf("refusal not persisted to history, last message: %+v", last)
	}
}

func TestStream_Guardrail_OnTopic_Proceeds(t *testing.T) {
	up := &guardrailUpstream{classifierReply: "ON_TOPIC"}
	upstream := httptest.NewServer(up.handler(t))
	defer upstream.Close()

	svc := newGuardrailService(t, upstream.URL)
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "where is my car?"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if up.classifierCalls != 1 {
		t.Errorf("classifier called %d times, want 1", up.classifierCalls)
	}
	if up.mainCalls != 1 {
		t.Errorf("main model called %d times, want 1", up.mainCalls)
	}
	var text strings.Builder
	for _, e := range sink.events {
		if e.Type == "token" {
			text.WriteString(e.Delta)
		}
	}
	if text.String() != "main answer" {
		t.Errorf("streamed text = %q, want %q", text.String(), "main answer")
	}
}

func TestStream_Guardrail_ClassifierError_FailsOpen(t *testing.T) {
	up := &guardrailUpstream{classifierStatus: http.StatusInternalServerError}
	upstream := httptest.NewServer(up.handler(t))
	defer upstream.Close()

	svc := newGuardrailService(t, upstream.URL)
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "where is my car?"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if up.mainCalls != 1 {
		t.Errorf("main model called %d times, want 1 (fail-open)", up.mainCalls)
	}
	if _, hasErr := sink.errorEvent(); hasErr {
		t.Error("classifier failure must not surface an error event")
	}
}

func TestStream_Guardrail_Disabled_SkipsClassifier(t *testing.T) {
	up := &guardrailUpstream{classifierReply: "OFF_TOPIC"}
	upstream := httptest.NewServer(up.handler(t))
	defer upstream.Close()

	svc := newTestService(t, upstream.URL, 5*time.Second) // GuardrailEnabled defaults false
	hist := &memHistory{msgs: []Message{{Role: "user", Content: "hello"}}}
	sink := &captureSink{}

	if err := svc.Stream(context.Background(), hist, sink); err != nil {
		t.Fatalf("Stream: %v", err)
	}

	if up.classifierCalls != 0 {
		t.Errorf("classifier called %d times, want 0", up.classifierCalls)
	}
	if up.mainCalls != 1 {
		t.Errorf("main model called %d times, want 1", up.mainCalls)
	}
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
