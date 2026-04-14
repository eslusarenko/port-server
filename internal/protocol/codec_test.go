package protocol

import (
	"testing"
)

func TestEncodeDecodeMessage(t *testing.T) {
	tests := []struct {
		name      string
		msgType   MessageType
		requestID uint32
		payload   []byte
	}{
		{"empty payload", TypePing, 0, nil},
		{"with request ID", TypeHttpRequest, 42, []byte("hello")},
		{"max request ID", TypeHttpResponse, 0xFFFFFFFF, []byte{0x00, 0xFF}},
		{"tunnel ready", TypeTunnelReady, 0, []byte(`{"subdomain":"abc"}`)},
		{"shutdown", TypeShutdown, 0, []byte(`{"reason":"bye"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeMessage(tt.msgType, tt.requestID, tt.payload)

			msgType, requestID, payload, err := DecodeMessage(encoded)
			if err != nil {
				t.Fatalf("DecodeMessage: %v", err)
			}
			if msgType != tt.msgType {
				t.Errorf("msgType = %d, want %d", msgType, tt.msgType)
			}
			if requestID != tt.requestID {
				t.Errorf("requestID = %d, want %d", requestID, tt.requestID)
			}
			if len(payload) != len(tt.payload) {
				t.Fatalf("payload len = %d, want %d", len(payload), len(tt.payload))
			}
			for i := range payload {
				if payload[i] != tt.payload[i] {
					t.Errorf("payload[%d] = %d, want %d", i, payload[i], tt.payload[i])
				}
			}
		})
	}
}

func TestDecodeMessageTooShort(t *testing.T) {
	_, _, _, err := DecodeMessage([]byte{0x01, 0x02})
	if err != ErrMessageTooShort {
		t.Errorf("err = %v, want ErrMessageTooShort", err)
	}
}

func TestEncodeDecodeHttpRequestMeta(t *testing.T) {
	meta := HttpRequestMeta{
		Method:        "POST",
		Path:          "/api/data",
		Host:          "abc123.port.rootlabs.eu",
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		ContentLength: 13,
	}
	body := []byte(`{"key":"val"}`)

	payload, err := EncodeHttpMeta(meta, body)
	if err != nil {
		t.Fatalf("EncodeHttpMeta: %v", err)
	}

	gotMeta, gotBody, err := DecodeHttpRequestMeta(payload)
	if err != nil {
		t.Fatalf("DecodeHttpRequestMeta: %v", err)
	}
	if gotMeta.Method != meta.Method {
		t.Errorf("Method = %q, want %q", gotMeta.Method, meta.Method)
	}
	if gotMeta.Path != meta.Path {
		t.Errorf("Path = %q, want %q", gotMeta.Path, meta.Path)
	}
	if gotMeta.Host != meta.Host {
		t.Errorf("Host = %q, want %q", gotMeta.Host, meta.Host)
	}
	if gotMeta.ContentLength != meta.ContentLength {
		t.Errorf("ContentLength = %d, want %d", gotMeta.ContentLength, meta.ContentLength)
	}
	if string(gotBody) != string(body) {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
}

func TestEncodeDecodeHttpResponseMeta(t *testing.T) {
	meta := HttpResponseMeta{
		StatusCode:    200,
		Headers:       map[string][]string{"Content-Type": {"text/html"}, "X-Custom": {"a", "b"}},
		ContentLength: 5,
	}
	body := []byte("hello")

	payload, err := EncodeHttpMeta(meta, body)
	if err != nil {
		t.Fatalf("EncodeHttpMeta: %v", err)
	}

	gotMeta, gotBody, err := DecodeHttpResponseMeta(payload)
	if err != nil {
		t.Fatalf("DecodeHttpResponseMeta: %v", err)
	}
	if gotMeta.StatusCode != meta.StatusCode {
		t.Errorf("StatusCode = %d, want %d", gotMeta.StatusCode, meta.StatusCode)
	}
	if string(gotBody) != string(body) {
		t.Errorf("body = %q, want %q", gotBody, body)
	}
	if len(gotMeta.Headers["X-Custom"]) != 2 {
		t.Errorf("X-Custom header count = %d, want 2", len(gotMeta.Headers["X-Custom"]))
	}
}

func TestDecodeHttpMetaTooShort(t *testing.T) {
	_, _, err := DecodeHttpRequestMeta([]byte{0x01})
	if err != ErrMetaTooShort {
		t.Errorf("err = %v, want ErrMetaTooShort", err)
	}
}

func TestDecodeHttpMetaEmptyBody(t *testing.T) {
	meta := HttpRequestMeta{
		Method: "GET",
		Path:   "/",
		Host:   "test.localhost",
	}

	payload, err := EncodeHttpMeta(meta, nil)
	if err != nil {
		t.Fatalf("EncodeHttpMeta: %v", err)
	}

	gotMeta, gotBody, err := DecodeHttpRequestMeta(payload)
	if err != nil {
		t.Fatalf("DecodeHttpRequestMeta: %v", err)
	}
	if gotMeta.Method != "GET" {
		t.Errorf("Method = %q, want GET", gotMeta.Method)
	}
	if len(gotBody) != 0 {
		t.Errorf("body len = %d, want 0", len(gotBody))
	}
}
