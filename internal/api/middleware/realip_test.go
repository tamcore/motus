package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRealIP(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("fc00::/7"),
	}
	tests := []struct {
		name       string
		remoteAddr string
		xff        []string
		xRealIP    string
		want       string
	}{
		{"untrusted peer keeps RemoteAddr", "198.51.100.7:1234", []string{"203.0.113.1"}, "", "198.51.100.7:1234"},
		{"untrusted peer ignores X-Real-Ip", "198.51.100.7:1234", nil, "203.0.113.1", "198.51.100.7:1234"},
		{"trusted peer, single hop", "10.0.0.5:1234", []string{"203.0.113.1"}, "", "203.0.113.1"},
		{"spoofed leftmost entry ignored", "10.0.0.5:1234", []string{"1.2.3.4, 203.0.113.1"}, "", "203.0.113.1"},
		{"trusted hops skipped", "10.0.0.5:1234", []string{"203.0.113.1, 10.0.0.9"}, "", "203.0.113.1"},
		{"multiple header lines", "10.0.0.5:1234", []string{"1.2.3.4", "203.0.113.1"}, "", "203.0.113.1"},
		{"all hops trusted uses leftmost", "10.0.0.5:1234", []string{"10.1.1.1, 10.0.0.9"}, "", "10.1.1.1"},
		{"malformed hop keeps RemoteAddr", "10.0.0.5:1234", []string{"203.0.113.1, nope"}, "", "10.0.0.5:1234"},
		{"X-Real-Ip from trusted peer", "10.0.0.5:1234", nil, "203.0.113.1", "203.0.113.1"},
		{"no headers keeps RemoteAddr", "10.0.0.5:1234", nil, "", "10.0.0.5:1234"},
		{"IPv6 trusted peer", "[fd00::1]:443", []string{"2001:db8::7"}, "", "2001:db8::7"},
		{"IPv4-mapped peer", "[::ffff:10.0.0.5]:1234", []string{"203.0.113.1"}, "", "203.0.113.1"},
		{"no trusted proxies", "10.0.0.5:1234", []string{"203.0.113.1"}, "", "10.0.0.5:1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefixes := trusted
			if tt.name == "no trusted proxies" {
				prefixes = nil
			}
			var got string
			h := RealIP(prefixes)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r.RemoteAddr }))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for _, v := range tt.xff {
				req.Header.Add("X-Forwarded-For", v)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-Ip", tt.xRealIP)
			}
			h.ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7:1234":           "203.0.113.7",
		"[2001:db8:1:2:3:4:5:6]:443": "2001:db8:1:2::",
		"[::ffff:203.0.113.7]:80":    "203.0.113.7",
		"not-an-ip":                  "not-an-ip",
	} {
		if got := clientIP(in); got != want {
			t.Errorf("clientIP(%q) = %q, want %q", in, got, want)
		}
	}
}
