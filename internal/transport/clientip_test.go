package transport

import (
	"net/http"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name               string
		trustProxyHeaders  bool
		remoteAddr         string
		xff                string
		xri                string
		want               string
	}{
		{
			name:              "trust=false returns RemoteAddr",
			trustProxyHeaders: false,
			remoteAddr:        "10.1.77.50:12345",
			xff:               "1.2.3.4",
			want:              "10.1.77.50:12345",
		},
		{
			name:              "XFF single entry",
			trustProxyHeaders: true,
			remoteAddr:        "10.0.0.1:9999",
			xff:               "203.0.113.1",
			want:              "203.0.113.1",
		},
		{
			name:              "XFF multi entry left-most wins",
			trustProxyHeaders: true,
			remoteAddr:        "10.0.0.1:9999",
			xff:               "203.0.113.1, 10.0.0.2, 172.16.0.3",
			want:              "203.0.113.1",
		},
		{
			name:              "XFF whitespace trimmed",
			trustProxyHeaders: true,
			remoteAddr:        "10.0.0.1:9999",
			xff:               "  203.0.113.42  , 10.0.0.2",
			want:              "203.0.113.42",
		},
		{
			name:              "X-Real-IP fallback when no XFF",
			trustProxyHeaders: true,
			remoteAddr:        "10.0.0.1:9999",
			xri:               "198.51.100.7",
			want:              "198.51.100.7",
		},
		{
			name:              "no proxy headers returns RemoteAddr",
			trustProxyHeaders: true,
			remoteAddr:        "10.0.0.1:9999",
			want:              "10.0.0.1:9999",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				r.Header.Set("X-Forwarded-For", tc.xff)
			}
			if tc.xri != "" {
				r.Header.Set("X-Real-IP", tc.xri)
			}
			got := clientIP(r, tc.trustProxyHeaders)
			if got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
