package tunnel

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var tunnelHTTPClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	},
}

func skipLoopbackProxyHeader(lower string) bool {
	switch lower {
	case "connection", "upgrade", "keep-alive", "proxy-connection",
		"transfer-encoding", "te", "trailer",
		"host",
		"cookie", "authorization",
		"x-forwarded-host", "x-forwarded-server", "forwarded",
		"sec-websocket-key", "sec-websocket-version", "sec-websocket-extensions",
		"alt-svc":
		return true
	default:
		return false
	}
}

// ProxyHTTPRequest forwards an HTTP request message to the configured upstream and returns the response message.
func ProxyHTTPRequest(dt DialTarget, msg *Message) (*Message, error) {
	method := strings.TrimSpace(msg.Method)
	if method == "" {
		method = http.MethodGet
	}
	if !validHTTPMethod(method) {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     "unsupported HTTP method",
		}, nil
	}

	target, err := buildUpstreamURL(dt.Protocol, dt.Host, dt.Port, msg.Path)
	if err != nil {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     "invalid request path",
		}, nil
	}

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

	var reqBody io.ReadCloser
	if bodyReader != nil {
		reqBody = io.NopCloser(bodyReader)
	}
	req := &http.Request{
		Method:     method,
		URL:        target,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       reqBody,
	}

	for k, v := range msg.Headers {
		if skipLoopbackProxyHeader(strings.ToLower(k)) {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Host = target.Host

	resp, err := tunnelHTTPClient.Do(req)
	if err != nil {
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     fmt.Sprintf("upstream: %v", err),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

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
		lower := strings.ToLower(k)
		if lower == "transfer-encoding" || lower == "connection" || lower == "keep-alive" {
			continue
		}
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

// ProxyWSOpen dials the upstream WebSocket and returns the connection.
func ProxyWSOpen(ctx context.Context, dt DialTarget, path string, headers map[string]string) (*websocket.Conn, error) {
	target, err := buildUpstreamURL(dt.wsScheme(), dt.Host, dt.Port, path)
	if err != nil {
		return nil, fmt.Errorf("invalid ws path: %w", err)
	}

	reqHeaders := http.Header{}
	for k, v := range headers {
		if skipLoopbackProxyHeader(strings.ToLower(k)) {
			continue
		}
		reqHeaders.Set(k, v)
	}
	reqHeaders.Set("Host", target.Host)

	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12},
	}
	conn, _, err := dialer.DialContext(ctx, target.String(), reqHeaders)
	if err != nil {
		return nil, fmt.Errorf("dial upstream ws %s: %w", target.String(), err)
	}
	return conn, nil
}
