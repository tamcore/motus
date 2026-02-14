package middleware

import "net/http"

// SecurityHeaders returns middleware that sets standard security response
// headers on every HTTP response. These headers instruct browsers to enable
// protective behaviors that mitigate common web vulnerabilities.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Prevent MIME-type sniffing; browser must use declared Content-Type.
		h.Set("X-Content-Type-Options", "nosniff")

		// Prevent the page from being embedded in an iframe (clickjacking).
		h.Set("X-Frame-Options", "DENY")

		// Content Security Policy: restrict resource loading to same origin.
		// Note: 'unsafe-inline' is required for SvelteKit adapter-static which
		// emits inline scripts for hydration and dynamic imports. Nonce-based
		// CSP requires SSR (adapter-node) and is not feasible with the current
		// static embedded frontend architecture. This is acceptable as CSP is a
		// defense-in-depth layer; primary XSS mitigation is input validation
		// and output encoding.
		// worker-src 'self' allows the PWA service worker to be loaded.
		// manifest-src 'self' allows the web app manifest for PWA install.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' https://unpkg.com; "+
				"style-src 'self' 'unsafe-inline' https://unpkg.com; "+
				"img-src 'self' data: https://*.tile.openstreetmap.org https://*.basemaps.cartocdn.com https://unpkg.com; "+
				"connect-src 'self' wss:; "+
				"font-src 'self'; "+
				"worker-src 'self'; "+
				"manifest-src 'self'; "+
				"frame-ancestors 'none'")

		// HTTP Strict Transport Security: force HTTPS for 1 year.
		// Only effective when served over HTTPS; browsers ignore it on HTTP.
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Permissions Policy: disable unused browser features.
		h.Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=()")

		// Referrer Policy: send origin only for cross-origin requests.
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}
