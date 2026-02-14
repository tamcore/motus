package notification

import (
	"net"
	"testing"
)

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty URL", "", true},
		{"HTTP not allowed for external host", "http://hooks.example.com/webhook", true},
		{"HTTP localhost allowed", "http://localhost:8080/hook", false},
		{"HTTP 127.0.0.1 allowed", "http://127.0.0.1:8080/hook", false},
		{"invalid URL", "://bad", true},
		{"FTP scheme", "ftp://example.com/file", true},
		// HTTPS with private IPs (resolves but blocked)
		{"HTTPS to private 10.x", "https://10.0.0.1/hook", true},
		{"HTTPS to private 192.168", "https://192.168.1.1/hook", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWebhookURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWebhookURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWebhookURL_PublicIPAllowed(t *testing.T) {
	// A public IP passed directly (no DNS lookup) should succeed.
	err := ValidateWebhookURL("https://8.8.8.8/webhook")
	if err != nil {
		t.Errorf("ValidateWebhookURL(public IP) unexpected error: %v", err)
	}
}

func TestValidateWebhookURL_UnresolvableHost(t *testing.T) {
	// .invalid TLD is guaranteed by RFC 2606 to never resolve.
	// Skip only if the error isn't a DNS lookup error (e.g., DNS broken in unexpected ways).
	err := ValidateWebhookURL("https://this-definitely-does-not-exist.invalid/webhook")
	if err == nil {
		t.Error("expected error for unresolvable hostname")
	}
}

func TestValidateWebhookURL_ResolvableHost(t *testing.T) {
	// google.com is a well-known public domain that should resolve.
	// Skip if DNS is unavailable in the test environment.
	_, err := net.LookupIP("google.com")
	if err != nil {
		t.Skip("DNS resolution unavailable, skipping resolvable host test")
	}

	err = ValidateWebhookURL("https://google.com/webhook")
	if err != nil {
		t.Errorf("ValidateWebhookURL(\"https://google.com/webhook\") unexpected error: %v", err)
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		private bool
	}{
		{"10.x private", "10.0.0.1", true},
		{"172.16 private", "172.16.0.1", true},
		{"172.31 private", "172.31.255.255", true},
		{"172.15 public", "172.15.0.1", false},
		{"192.168 private", "192.168.1.1", true},
		{"127.0.0.1 loopback", "127.0.0.1", true},
		{"169.254 link-local", "169.254.1.1", true},
		{"8.8.8.8 public", "8.8.8.8", false},
		{"1.1.1.1 public", "1.1.1.1", false},
		{"IPv6 loopback", "::1", true},
		{"IPv6 ULA", "fd00::1", true},
		{"IPv6 public", "2001:db8::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			got := isPrivateIP(ip)
			if got != tt.private {
				t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}
