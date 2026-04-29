package notification

import (
	"strings"
	"sync"
)

// allowedHosts is the package-level set of hostnames whose webhook URLs are
// permitted to resolve to private IP addresses. Self-hosted services on
// internal networks (e.g. ntfy.example.lan) need this opt-in to coexist with
// the default SSRF protection. Empty (default) preserves strict behavior.
var (
	allowedHostsMu sync.RWMutex
	allowedHosts   map[string]struct{}
)

// SetAllowedHosts configures hostnames whose webhook URLs may resolve to
// private IP addresses. Hostnames are matched case-insensitively and exactly
// (no suffix matching, to avoid accidental over-permissive rules). Pass nil
// or an empty slice to clear the allowlist.
func SetAllowedHosts(hosts []string) {
	m := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			m[h] = struct{}{}
		}
	}
	allowedHostsMu.Lock()
	allowedHosts = m
	allowedHostsMu.Unlock()
}

// isHostAllowed reports whether hostname is in the SSRF allowlist.
func isHostAllowed(hostname string) bool {
	if hostname == "" {
		return false
	}
	allowedHostsMu.RLock()
	defer allowedHostsMu.RUnlock()
	_, ok := allowedHosts[strings.ToLower(hostname)]
	return ok
}
