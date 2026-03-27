package tunnel

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// ProxyHTTPRequest forwards an HTTP request message to a local service and returns the response message.
func ProxyHTTPRequest(localPort int, msg *Message) (*Message, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", localPort, msg.Path)

	var bodyReader io.Reader
	if msg.Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(msg.Body)
		if err != nil {
			return &Message{
				Type:      MsgHTTPResponse,
				RequestID: msg.RequestID,
				Status:    502,
				Error:     "failed to decode request body",
			}, nil
		}
		bodyReader = strings.NewReader(string(decoded))
	}

	req, err := http.NewRequest(msg.Method, url, bodyReader)
	if err != nil {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     fmt.Sprintf("create request: %v", err),
		}, nil
	}

	for k, v := range msg.Headers {
		// Skip hop-by-hop headers
		lower := strings.ToLower(k)
		if lower == "connection" || lower == "upgrade" || lower == "transfer-encoding" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     fmt.Sprintf("local service: %v", err),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body (cap at 10MB to prevent OOM)
	const maxBody = 10 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     fmt.Sprintf("read response: %v", err),
		}, nil
	}

	headers := make(map[string]string, len(resp.Header))
	for k := range resp.Header {
		headers[k] = resp.Header.Get(k)
	}

	return &Message{
		Type:      MsgHTTPResponse,
		RequestID: msg.RequestID,
		Status:    resp.StatusCode,
		Headers:   headers,
		Body:      base64.StdEncoding.EncodeToString(body),
	}, nil
}

// ProxyWSOpen dials a local WebSocket and returns the connection.
func ProxyWSOpen(ctx context.Context, localPort int, path string, headers map[string]string) (*websocket.Conn, error) {
	url := fmt.Sprintf("ws://127.0.0.1:%d%s", localPort, path)

	reqHeaders := http.Header{}
	for k, v := range headers {
		lower := strings.ToLower(k)
		if lower == "connection" || lower == "upgrade" || lower == "sec-websocket-key" ||
			lower == "sec-websocket-version" || lower == "sec-websocket-extensions" {
			continue
		}
		reqHeaders.Set(k, v)
	}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, reqHeaders)
	if err != nil {
		return nil, fmt.Errorf("dial local ws %s: %w", url, err)
	}
	return conn, nil
}
