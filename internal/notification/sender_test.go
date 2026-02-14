package notification

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamcore/motus/internal/model"
)

func TestSender_SendWebhook(t *testing.T) {
	// Set up a test HTTP server to receive the webhook.
	var receivedBody string
	var receivedContentType string
	var receivedCustomHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedCustomHeader = r.Header.Get("X-Custom")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sender := NewSender()

	rule := &model.NotificationRule{
		Channel: "webhook",
		Config: map[string]interface{}{
			"webhookUrl": server.URL,
			"headers": map[string]interface{}{
				"X-Custom": "test-value",
			},
		},
		Template: `{"device":"{{device.name}}","event":"{{event.type}}"}`,
	}

	templateCtx := &TemplateContext{
		Device: &model.Device{Name: "GT3 RS"},
		Event:  &model.Event{Type: "geofenceEnter", Timestamp: time.Now()},
	}

	statusCode, err := sender.Send(context.Background(), rule, templateCtx)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if statusCode != http.StatusOK {
		t.Errorf("Send() statusCode = %d, want %d", statusCode, http.StatusOK)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", receivedContentType, "application/json")
	}
	if receivedCustomHeader != "test-value" {
		t.Errorf("X-Custom = %q, want %q", receivedCustomHeader, "test-value")
	}
	want := `{"device":"GT3 RS","event":"geofenceEnter"}`
	if receivedBody != want {
		t.Errorf("body = %q, want %q", receivedBody, want)
	}
}

func TestSender_SendWebhookError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sender := NewSender()

	rule := &model.NotificationRule{
		Channel: "webhook",
		Config: map[string]interface{}{
			"webhookUrl": server.URL,
		},
		Template: "test",
	}

	statusCode, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for 500 response")
	}
	if statusCode != http.StatusInternalServerError {
		t.Errorf("Send() statusCode = %d, want %d", statusCode, http.StatusInternalServerError)
	}
}

func TestSender_SendWebhookMissingURL(t *testing.T) {
	sender := NewSender()

	rule := &model.NotificationRule{
		Channel:  "webhook",
		Config:   map[string]interface{}{},
		Template: "test",
	}

	_, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for missing webhookUrl")
	}
}

func TestSender_SendWebhookNetworkError(t *testing.T) {
	// Use a URL that refuses the connection to trigger client.Do error.
	sender := NewSender()
	sender.client = &http.Client{
		Transport: &networkErrTransport{},
	}

	rule := &model.NotificationRule{
		Channel:  "webhook",
		Config:   map[string]interface{}{"webhookUrl": "http://localhost:12345/hook"},
		Template: "test",
	}

	_, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for network failure")
	}
}

// networkErrTransport is an http.RoundTripper that always returns a network error.
type networkErrTransport struct{}

func (t *networkErrTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network error")
}

func TestSender_SendWebhookInvalidURL(t *testing.T) {
	// A URL with a newline character causes http.NewRequestWithContext to fail.
	sender := NewSender()

	rule := &model.NotificationRule{
		Channel: "webhook",
		Config: map[string]interface{}{
			"webhookUrl": "http://foo\nbar",
		},
		Template: "test",
	}

	_, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for invalid webhook URL")
	}
}

func TestSender_NtfyChannelRejected(t *testing.T) {
	sender := NewSender()

	rule := &model.NotificationRule{
		Channel:  "ntfy",
		Config:   map[string]interface{}{"topic": "test-topic"},
		Template: "test",
	}

	_, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for ntfy channel (removed)")
	}
	if got := err.Error(); got != "unsupported notification channel: ntfy" {
		t.Errorf("Send() error = %q, want %q", got, "unsupported notification channel: ntfy")
	}
}

func TestSender_UnsupportedChannel(t *testing.T) {
	sender := NewSender()

	rule := &model.NotificationRule{
		Channel:  "email",
		Config:   map[string]interface{}{},
		Template: "test",
	}

	_, err := sender.Send(context.Background(), rule, &TemplateContext{})
	if err == nil {
		t.Fatal("Send() expected error for unsupported channel")
	}
}
