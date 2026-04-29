package notification

import (
	"strings"
	"sync"
)

// allowedHosts is the package-level set of hostnames treated as trusted
// internal endpoints. Membership grants two relaxations to the default
// webhook security checks:
//   - the URL/dialer skip the private-IP SSRF block, so RFC1918 targets
//     are reachable;
//   - the TLS handshake skips certificate verification, so self-signed
//     or private-CA endpoints work without baking custom roots in.
//
// Empty (default) preserves strict behavior. Both relaxations are coupled
// intentionally because the typical use case (self-hosted ntfy on a home
// network) needs them together; operators who want only one should not
// add the host here.
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
