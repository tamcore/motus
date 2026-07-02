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

// completionResponse builds a minimal non-streaming chat completion body.
func completionResponse(content string) string {
	return `{"id":"cmpl-1","object":"chat.completion","created":1,"model":"test-model",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":` + jsonString(content) + `},"finish_reason":"stop"}]}`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func newGuardrailService(t *testing.T, baseURL string) *Service {
	t.Helper()
	return NewService(Config{
		BaseURL:          baseURL,
		APIKey:           "test-key",
		Model:            "test-model",
		MaxTokens:        128,
		MaxLoops:         2,
		Timeout:          5 * time.Second,
		MCPServer:        mcpserver.NewMCPServer("test", "0.0.0"),
		GuardrailEnabled: true,
	})
}

func TestClassifyTopic(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		reply        string
		wantOffTopic bool
		wantErr      bool
	}{
		{name: "on topic", status: http.StatusOK, reply: "ON_TOPIC", wantOffTopic: false},
		{name: "off topic", status: http.StatusOK, reply: "OFF_TOPIC", wantOffTopic: true},
		{name: "off topic lowercase", status: http.StatusOK, reply: "off_topic", wantOffTopic: true},
		{name: "garbage reply fails open", status: http.StatusOK, reply: "banana", wantOffTopic: false},
		{name: "upstream error", status: http.StatusInternalServerError, reply: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if tt.status != http.StatusOK {
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
					return
				}
				_, _ = w.Write([]byte(completionResponse(tt.reply)))
			}))
			defer upstream.Close()

			svc := newGuardrailService(t, upstream.URL)
			offTopic, err := svc.classifyTopic(context.Background(), []Message{{Role: "user", Content: "hello"}})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if offTopic {
					t.Error("errors must not classify as off-topic")
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyTopic: %v", err)
			}
			if offTopic != tt.wantOffTopic {
				t.Errorf("offTopic = %v, want %v", offTopic, tt.wantOffTopic)
			}
		})
	}
}

func TestLastUserMessageSnippet(t *testing.T) {
	long := strings.Repeat("x", guardrailLogSnippetLen+10)
	tests := []struct {
		name string
		msgs []Message
		want string
	}{
		{name: "empty history", msgs: nil, want: ""},
		{name: "no user turns", msgs: []Message{{Role: "assistant", Content: "hi"}}, want: ""},
		{
			name: "last user wins",
			msgs: []Message{
				{Role: "user", Content: "first"},
				{Role: "assistant", Content: "reply"},
				{Role: "user", Content: "second"},
			},
			want: "second",
		},
		{
			name: "long message truncated",
			msgs: []Message{{Role: "user", Content: long}},
			want: strings.Repeat("x", guardrailLogSnippetLen) + "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastUserMessageSnippet(tt.msgs); got != tt.want {
				t.Errorf("lastUserMessageSnippet = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyTopic_HistoryWindow(t *testing.T) {
	var captured struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens   int64   `json:"max_tokens"`
		Temperature float64 `json:"temperature"`
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Errorf("unmarshal classifier request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(completionResponse("ON_TOPIC")))
	}))
	defer upstream.Close()

	svc := newGuardrailService(t, upstream.URL)

	// 8 text turns plus tool noise; only the last 6 text turns may be forwarded.
	msgs := []Message{
		{Role: "user", Content: "turn 1"},
		{Role: "assistant", Content: "turn 2"},
		{Role: "user", Content: "turn 3"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "tc1", Name: "list_devices"}}},
		{Role: "tool", Content: `{"devices":[]}`, ToolCallID: "tc1"},
		{Role: "assistant", Content: "turn 4"},
		{Role: "user", Content: "turn 5"},
		{Role: "assistant", Content: "turn 6"},
		{Role: "user", Content: "turn 7"},
		{Role: "assistant", Content: "turn 8"},
	}

	if _, err := svc.classifyTopic(context.Background(), msgs); err != nil {
		t.Fatalf("classifyTopic: %v", err)
	}

	if len(captured.Messages) != 7 { // system + 6 text turns
		t.Fatalf("expected 7 messages (system + 6), got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" {
		t.Errorf("first message role = %q, want system", captured.Messages[0].Role)
	}
	wantTurns := []string{"turn 3", "turn 4", "turn 5", "turn 6", "turn 7", "turn 8"}
	for i, want := range wantTurns {
		if got := captured.Messages[i+1].Content; got != want {
			t.Errorf("message %d content = %q, want %q", i+1, got, want)
		}
	}
	for _, m := range captured.Messages {
		if m.Role == "tool" {
			t.Error("tool turns must not be forwarded to the classifier")
		}
	}
	if captured.MaxTokens != guardrailMaxTokens {
		t.Errorf("max_tokens = %d, want %d", captured.MaxTokens, guardrailMaxTokens)
	}
	if captured.Temperature != 0 {
		t.Errorf("temperature = %v, want 0", captured.Temperature)
	}
}
