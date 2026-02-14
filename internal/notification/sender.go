package notification

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/tamcore/motus/internal/metrics"
	"github.com/tamcore/motus/internal/model"
)

// Sender dispatches notifications via configured channels.
type Sender struct {
	client *http.Client
}

// NewSender creates a new notification sender. The underlying HTTP client uses
// a custom dialer that resolves DNS once per request, validates the resolved IP
// against private ranges, and pins the connection to that IP. This closes the
// TOCTOU window in SSRF protection where DNS could rebind between validation
// time and request time.
func NewSender() *Sender {
	return &Sender{
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: newSSRFSafeTransport(),
		},
	}
}

// newSSRFSafeTransport returns an HTTP transport whose dialer resolves the
// target hostname, checks every resolved IP against private ranges, and dials
// the first safe IP directly — preventing DNS rebinding attacks.
//
// Explicit loopback addresses (127.0.0.1, ::1) and "localhost" are allowed
// to match the ValidateWebhookURL exception used for development.
func newSSRFSafeTransport() *http.Transport {
	baseDialer := &net.Dialer{Timeout: 10 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("parse addr: %w", err)
			}

			// Allow localhost/127.0.0.1 explicitly for development and testing,
			// matching the exception in ValidateWebhookURL. These addresses
			// cannot DNS-rebound.
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				return baseDialer.DialContext(ctx, network, addr)
			}

			// For explicit IP addresses there is no DNS to rebound; validate
			// the IP directly. Loopback is already handled above.
			if ip := net.ParseIP(host); ip != nil {
				if isPrivateIP(ip) {
					return nil, fmt.Errorf("webhook URL resolves to private IP address")
				}
				return baseDialer.DialContext(ctx, network, addr)
			}

			// Resolve DNS here and validate every returned IP before dialing.
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", host, err)
			}
			if len(addrs) == 0 {
				return nil, fmt.Errorf("no addresses for %s", host)
			}
			for _, a := range addrs {
				if isPrivateIP(a.IP) {
					return nil, fmt.Errorf("webhook URL resolves to private IP address")
				}
			}

			// Pin to the first resolved IP to prevent DNS rebinding between
			// the lookup and the actual TCP dial.
			pinnedAddr := net.JoinHostPort(addrs[0].IP.String(), port)
			return baseDialer.DialContext(ctx, network, pinnedAddr)
		},
	}
}

// Send dispatches a notification based on the rule's channel type.
func (s *Sender) Send(ctx context.Context, rule *model.NotificationRule, templateCtx *TemplateContext) (int, error) {
	var code int
	var err error

	switch rule.Channel {
	case "webhook":
		code, err = s.sendWebhook(ctx, rule, templateCtx)
	default:
		return 0, fmt.Errorf("unsupported notification channel: %s", rule.Channel)
	}

	if err != nil {
		metrics.NotificationsSent.WithLabelValues(rule.Channel, "error").Inc()
	} else {
		metrics.NotificationsSent.WithLabelValues(rule.Channel, "success").Inc()
	}
	return code, err
}

// sendWebhook sends an HTTP POST with the rendered template body.
func (s *Sender) sendWebhook(ctx context.Context, rule *model.NotificationRule, templateCtx *TemplateContext) (int, error) {
	webhookURL, ok := rule.Config["webhookUrl"].(string)
	if !ok || webhookURL == "" {
		return 0, fmt.Errorf("webhookUrl not configured")
	}

	body := RenderTemplate(rule.Template, templateCtx)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewBufferString(body))
	if err != nil {
		return 0, fmt.Errorf("create webhook request: %w", err)
	}

	// Default content type; can be overridden by custom headers.
	req.Header.Set("Content-Type", "application/json")

	// Apply custom headers from config.
	if headers, ok := rule.Config["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strVal, ok := value.(string); ok {
				req.Header.Set(key, strVal)
			}
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send webhook request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain body to allow connection reuse.
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	slog.Debug("webhook sent",
		slog.String("url", webhookURL),
		slog.Int("statusCode", resp.StatusCode),
	)
	return resp.StatusCode, nil
}
