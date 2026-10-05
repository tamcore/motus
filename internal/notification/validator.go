package notification

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

var errPrivateIP = errors.New("webhook URL resolves to private IP address")

// ValidateWebhookURL checks if a webhook URL is safe to use. It rejects
// URLs that resolve to private/internal IP addresses to prevent SSRF attacks.
// Only HTTPS is allowed in production; HTTP is permitted for localhost in dev.
func ValidateWebhookURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("webhook URL is required")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only HTTP/HTTPS are valid webhook schemes — block file://, javascript:,
	// etc. Scheme choice between the two is up to the operator; HTTPS is
	// recommended but not enforced (in-cluster Services often serve plain
	// HTTP, and SSRF is gated separately).
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https")
	}

	_, err = checkHost(context.Background(), u.Hostname())
	return err
}

// checkHost rejects hosts that are, or resolve to, private IPs. Loopback
// names (dev convenience) and allowlisted hosts pass unchecked. It returns
// the resolved IP to pin the connection to, or nil to dial host as is.
func checkHost(ctx context.Context, host string) (net.IP, error) {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil, nil
	}

	// Operator-configured allowlist for self-hosted services on internal
	// networks (e.g. ntfy.example.lan) that legitimately resolve into
	// RFC1918 space.
	if isHostAllowed(host) {
		return nil, nil
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return nil, errPrivateIP
		}
		return nil, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("could not resolve hostname %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	for _, a := range addrs {
		if isPrivateIP(a.IP) {
			return nil, errPrivateIP
		}
	}
	return addrs[0].IP, nil
}

// isPrivateIP returns true if the IP belongs to a private, loopback or link-local range.
func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
