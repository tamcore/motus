package notification

import (
	"fmt"
	"net"
	"net/url"
)

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

	// Require HTTPS, with exception for localhost during development.
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return fmt.Errorf("webhook URL must use HTTPS")
	}

	// Skip IP resolution for localhost (test/dev convenience).
	if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
		return nil
	}

	hostname := u.Hostname()

	// If the hostname is already an IP address, check it directly.
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL resolves to private IP address")
		}
		return nil
	}

	// Resolve hostname to IP addresses.
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return fmt.Errorf("could not resolve hostname: %w", err)
	}

	// Block private/internal IP ranges to prevent SSRF.
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("webhook URL resolves to private IP address")
		}
	}

	return nil
}

// isPrivateIP returns true if the IP belongs to a private or reserved range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
	}

	for _, cidr := range privateRanges {
		_, subnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}
