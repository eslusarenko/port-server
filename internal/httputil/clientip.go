package httputil

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the client IP address for logging, with any port suffix stripped.
// If trustProxyHeaders is false, r.RemoteAddr is used.
// If true, it checks X-Forwarded-For (left-most entry) then X-Real-IP,
// falling back to r.RemoteAddr if neither is present.
func ClientIP(r *http.Request, trustProxyHeaders bool) string {
	var raw string
	if !trustProxyHeaders {
		raw = r.RemoteAddr
	} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			raw = ip
		}
	} else if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		raw = xri
	} else {
		raw = r.RemoteAddr
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}
	return raw
}
