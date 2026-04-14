// SYNC: keep in sync with client/internal/protocol/codec.go
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrMessageTooShort = errors.New("protocol: message shorter than header")
	ErrMetaTooShort    = errors.New("protocol: payload too short for metadata length prefix")
)

// EncodeMessage builds a complete wire-format message:
//
//	[header (8 bytes)] [payload]
func EncodeMessage(msgType MessageType, requestID uint32, payload []byte) []byte {
	buf := make([]byte, HeaderSize+len(payload))
	buf[0] = byte(msgType)
	binary.BigEndian.PutUint32(buf[1:5], requestID)
	// bytes 5-7 reserved (zero)
	copy(buf[HeaderSize:], payload)
	return buf
}

// DecodeMessage splits a raw WebSocket message into its header fields and payload.
func DecodeMessage(data []byte) (msgType MessageType, requestID uint32, payload []byte, err error) {
	if len(data) < HeaderSize {
		return 0, 0, nil, ErrMessageTooShort
	}
	msgType = MessageType(data[0])
	requestID = binary.BigEndian.Uint32(data[1:5])
	payload = data[HeaderSize:]
	return msgType, requestID, payload, nil
}

// EncodeHttpMeta builds the payload for HttpRequest / HttpResponse frames:
//
//	[json-length (4 bytes, big-endian)] [json metadata] [raw body]
func EncodeHttpMeta(meta any, body []byte) ([]byte, error) {
	jsonData, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("protocol: marshal metadata: %w", err)
	}
	buf := make([]byte, 4+len(jsonData)+len(body))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(jsonData)))
	copy(buf[4:], jsonData)
	copy(buf[4+len(jsonData):], body)
	return buf, nil
}

// DecodeHttpRequestMeta extracts HttpRequestMeta and body from a payload.
func DecodeHttpRequestMeta(payload []byte) (HttpRequestMeta, []byte, error) {
	var meta HttpRequestMeta
	body, err := decodeMetaJSON(payload, &meta)
	return meta, body, err
}

// DecodeHttpResponseMeta extracts HttpResponseMeta and body from a payload.
func DecodeHttpResponseMeta(payload []byte) (HttpResponseMeta, []byte, error) {
	var meta HttpResponseMeta
	body, err := decodeMetaJSON(payload, &meta)
	return meta, body, err
}

func decodeMetaJSON(payload []byte, dest any) ([]byte, error) {
	if len(payload) < 4 {
		return nil, ErrMetaTooShort
	}
	jsonLen := binary.BigEndian.Uint32(payload[0:4])
	if uint32(len(payload)-4) < jsonLen {
		return nil, fmt.Errorf("protocol: payload too short: need %d bytes for metadata, have %d", jsonLen, len(payload)-4)
	}
	if err := json.Unmarshal(payload[4:4+jsonLen], dest); err != nil {
		return nil, fmt.Errorf("protocol: unmarshal metadata: %w", err)
	}
	body := payload[4+jsonLen:]
	return body, nil
}
