package middleware

import (
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/tamcore/motus/internal/audit"
)

// RealIP sets r.RemoteAddr to the client IP. Proxy headers are honoured only
// when the direct peer is in trusted. X-Forwarded-For is read right to left,
// skipping trusted hops, so entries a client adds itself are ignored.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip, ok := realIP(r, trusted); ok {
				r.RemoteAddr = ip.String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func realIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	peer, ok := audit.ParseRemoteAddr(r.RemoteAddr)
	if !ok || !isTrusted(peer, trusted) {
		return netip.Addr{}, false
	}
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		hops := strings.Split(strings.Join(xff, ","), ",")
		var leftmost netip.Addr
		for _, hop := range slices.Backward(hops) {
			ip, err := netip.ParseAddr(strings.TrimSpace(hop))
			if err != nil {
				return netip.Addr{}, false
			}
			ip = ip.Unmap()
			if !isTrusted(ip, trusted) {
				return ip, true
			}
			leftmost = ip
		}
		return leftmost, true
	}
	if ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-Ip"))); err == nil {
		return ip.Unmap(), true
	}
	return netip.Addr{}, false
}

func isTrusted(ip netip.Addr, trusted []netip.Prefix) bool {
	return slices.ContainsFunc(trusted, func(p netip.Prefix) bool { return p.Contains(ip) })
}
