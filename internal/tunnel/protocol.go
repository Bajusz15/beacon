// Package tunnel implements reverse tunneling from BeaconInfra cloud to local services.
package tunnel

// MessageType identifies the kind of tunnel protocol message.
type MessageType string

const (
	MsgHTTPRequest  MessageType = "http_request"
	MsgHTTPResponse MessageType = "http_response"
	MsgWSOpen       MessageType = "ws_open"
	MsgWSOpenResult MessageType = "ws_open_result"
	MsgWSFrame      MessageType = "ws_frame"
	MsgWSClose      MessageType = "ws_close"
	MsgPing         MessageType = "ping"
	MsgPong         MessageType = "pong"
)

// Message is the JSON envelope sent over the tunnel WebSocket.
// Fields are populated depending on the Type.
type Message struct {
	Type      MessageType       `json:"type"`
	RequestID string            `json:"request_id,omitempty"`
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body,omitempty"`   // base64-encoded
	Status    int               `json:"status,omitempty"` // HTTP status code (response only)

	// WebSocket passthrough fields
	StreamID    string `json:"stream_id,omitempty"`
	WSFrameType int    `json:"ws_frame_type,omitempty"` // websocket.TextMessage or BinaryMessage
	WSPayload   string `json:"ws_payload,omitempty"`    // base64-encoded for binary frames

	Error string `json:"error,omitempty"`
}
