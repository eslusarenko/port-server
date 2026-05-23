package transport

import (
	"net/http"
	"strings"
)

// clientIP returns the client IP address for logging.
// If trustProxyHeaders is false, r.RemoteAddr is returned unchanged.
// If true, it checks X-Forwarded-For (left-most entry) then X-Real-IP,
// falling back to r.RemoteAddr if neither is present.
func clientIP(r *http.Request, trustProxyHeaders bool) string {
	if !trustProxyHeaders {
		return r.RemoteAddr
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
