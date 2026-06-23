package http

import (
	"game/deps/transport"
	"net/http"
)

var _ transport.Transporter = (*Transport)(nil)

// Transport is an HTTP transport.
type Transport struct {
	req       *http.Request
	resHeader http.Header
	operation string
	clientIP  string
}

// NewTransport creates a new HTTP transport.
func NewTransport(req *http.Request, operation string, clientIP string) *Transport {
	return &Transport{
		req:       req,
		resHeader: make(http.Header),
		operation: operation,
		clientIP:  clientIP,
	}
}

// Kind returns the transport kind.
func (t *Transport) Kind() transport.Kind {
	return transport.KindHTTP
}

// Endpoint returns the transport endpoint.
func (t *Transport) Endpoint() string {
	return t.req.URL.Host
}

// Operation returns the transport operation.
func (t *Transport) Operation() string {
	if t.operation != "" {
		return t.operation
	}
	return t.req.URL.Path
}

// RequestHeader returns the request header.
func (t *Transport) RequestHeader() transport.Header {
	return headerCarrier(t.req.Header)
}

// ReplyHeader returns the reply header.
func (t *Transport) ReplyHeader() transport.Header {
	return headerCarrier(t.resHeader)
}

// ClientIP returns the client's IP address.
func (t *Transport) ClientIP() string {
	return t.clientIP
}

// headerCarrier adapts http.Header to transport.Header
type headerCarrier http.Header

func (h headerCarrier) Get(key string) string      { return http.Header(h).Get(key) }
func (h headerCarrier) Set(key, value string)      { http.Header(h).Set(key, value) }
func (h headerCarrier) Add(key, value string)      { http.Header(h).Add(key, value) }
func (h headerCarrier) Values(key string) []string { return http.Header(h).Values(key) }
func (h headerCarrier) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range http.Header(h) {
		keys = append(keys, k)
	}
	return keys
}
