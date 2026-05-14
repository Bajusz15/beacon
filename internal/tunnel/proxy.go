package tunnel

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

func safeTunnelLogPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/"
	}
	u, err := url.Parse(path)
	if err != nil {
		return "<invalid-path>"
	}
	q := u.Query()
	for key := range q {
		q.Set(key, "<redacted>")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func proxyHeaderSummary(headers map[string]string) string {
	parts := []string{}
	for _, key := range []string{"Origin", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-For", "Accept"} {
		if value := strings.TrimSpace(headers[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func skipLoopbackProxyHeader(lower string) bool {
	switch lower {
	case "connection", "upgrade", "keep-alive", "proxy-connection",
		"transfer-encoding", "te", "trailer", "content-length",
		"host",
		"cookie", "authorization",
		"x-forwarded-server", "forwarded",
		"sec-websocket-key", "sec-websocket-version", "sec-websocket-extensions",
		"alt-svc":
		return true
	default:
		return false
	}
}

func upstreamHostHeader(headers map[string]string, fallback string) string {
	if host := strings.TrimSpace(headers["X-Forwarded-Host"]); host != "" {
		return host
	}
	return fallback
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

	var bodyBytes []byte
	if msg.Body != "" {
		var err error
		bodyBytes, err = base64.StdEncoding.DecodeString(msg.Body)
		if err != nil {
			return &Message{
				Type:      MsgHTTPResponse,
				RequestID: msg.RequestID,
				Status:    502,
				Error:     "failed to decode request body",
			}, nil
		}
	}
	log.Printf("[Beacon tunnel proxy] HTTP upstream request method=%s target=%s path=%s headers=%s body_bytes=%d",
		method, target.Redacted(), safeTunnelLogPath(msg.Path), proxyHeaderSummary(msg.Headers), len(bodyBytes))

	var reqBody io.ReadCloser
	var contentLength int64
	if len(bodyBytes) > 0 {
		reqBody = io.NopCloser(strings.NewReader(string(bodyBytes)))
		contentLength = int64(len(bodyBytes))
	}
	req := &http.Request{
		Method:        method,
		URL:           target,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          reqBody,
		ContentLength: contentLength,
	}

	for k, v := range msg.Headers {
		if skipLoopbackProxyHeader(strings.ToLower(k)) {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Host = upstreamHostHeader(msg.Headers, target.Host)

	resp, err := tunnelHTTPClient.Do(req)
	if err != nil {
		log.Printf("[Beacon tunnel proxy] HTTP upstream failed method=%s path=%s target=%s err=%v",
			method, safeTunnelLogPath(msg.Path), target.Redacted(), err)
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
		log.Printf("[Beacon tunnel proxy] HTTP upstream response read failed method=%s path=%s status=%d err=%v",
			method, safeTunnelLogPath(msg.Path), resp.StatusCode, err)
		return &Message{
			Type:      MsgHTTPResponse,
			RequestID: msg.RequestID,
			Status:    502,
			Error:     fmt.Sprintf("read response: %v", err),
		}, nil
	}
	log.Printf("[Beacon tunnel proxy] HTTP upstream response method=%s path=%s status=%d body_bytes=%d",
		method, safeTunnelLogPath(msg.Path), resp.StatusCode, len(body))
	if resp.StatusCode >= 400 {
		log.Printf("[Beacon tunnel proxy] HTTP upstream error body method=%s path=%s status=%d body=%q",
			method, safeTunnelLogPath(msg.Path), resp.StatusCode, strings.TrimSpace(string(body)))
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
	log.Printf("[Beacon tunnel proxy] Dialing upstream WS target=%s path=%s headers=%s",
		target.Redacted(), safeTunnelLogPath(path), proxyHeaderSummary(headers))

	reqHeaders := http.Header{}
	for k, v := range headers {
		if skipLoopbackProxyHeader(strings.ToLower(k)) {
			continue
		}
		reqHeaders.Set(k, v)
	}
	reqHeaders.Set("Host", upstreamHostHeader(headers, target.Host))

	dialer := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  &tls.Config{MinVersion: tls.VersionTLS12},
	}
	conn, resp, err := dialer.DialContext(ctx, target.String(), reqHeaders)
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			log.Printf("[Beacon tunnel proxy] Upstream WS dial failed target=%s path=%s status=%d response_body=%q err=%v",
				target.Redacted(), safeTunnelLogPath(path), resp.StatusCode, strings.TrimSpace(string(body)), err)
		} else {
			log.Printf("[Beacon tunnel proxy] Upstream WS dial failed target=%s path=%s err=%v",
				target.Redacted(), safeTunnelLogPath(path), err)
		}
		return nil, fmt.Errorf("dial upstream ws %s: %w", target.String(), err)
	}
	log.Printf("[Beacon tunnel proxy] Upstream WS connected target=%s path=%s", target.Redacted(), safeTunnelLogPath(path))
	return conn, nil
}
