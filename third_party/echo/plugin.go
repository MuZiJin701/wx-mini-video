package echo

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Plugin represents a forwarding rule configuration
type Plugin struct {
	Match        string
	Target       *TargetConfig
	MockResponse *MockResponse
	Bypass       bool // If true, skip MITM and tunnel directly
	Disabled     bool // If true, the plugin is ignored by matchers

	// Hooks
	OnRequest  func(ctx *Context)
	OnResponse func(ctx *Context)
}

// Context provides access to the request and response for plugins
type Context struct {
	Req *http.Request
	Res *http.Response // Nil in OnRequest

	mockResp *MockResponse
}

// Mock sets a mock response to be returned immediately
func (c *Context) Mock(status int, headers map[string]string, body interface{}) {
	c.mockResp = &MockResponse{
		StatusCode: status,
		Headers:    headers,
		Body:       body,
	}
}

// GetMockResponse returns the set mock response
func (c *Context) GetMockResponse() *MockResponse {
	return c.mockResp
}

// SetRequestHeader sets a header on the request
func (c *Context) SetRequestHeader(key, value string) {
	if c.Req != nil {
		c.Req.Header.Set(key, value)
	}
}

// DelRequestHeader deletes a header from the request
func (c *Context) DelRequestHeader(key string) {
	if c.Req != nil {
		c.Req.Header.Del(key)
	}
}

// GetRequestHeader gets a header from the request
func (c *Context) GetRequestHeader(key string) string {
	if c.Req != nil {
		return c.Req.Header.Get(key)
	}
	return ""
}

// SetResponseHeader sets a header on the response
func (c *Context) SetResponseHeader(key, value string) {
	if c.Res != nil {
		c.Res.Header.Set(key, value)
	}
}

// DelResponseHeader deletes a header from the response
func (c *Context) DelResponseHeader(key string) {
	if c.Res != nil {
		c.Res.Header.Del(key)
	}
}

// GetResponseHeader gets a header from the response
func (c *Context) GetResponseHeader(key string) string {
	if c.Res != nil {
		return c.Res.Header.Get(key)
	}
	return ""
}

// GetResponseBody reads and returns the response body as a string
// It automatically decompresses the body if needed and updates the response
// to be uncompressed for subsequent reads.
func (c *Context) GetResponseBody() (string, error) {
	if c.Res == nil || c.Res.Body == nil {
		return "", nil
	}

	// Decompress if needed
	reader, err := DecompressBody(c.Res)
	if err != nil {
		return "", err
	}

	// Read body
	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	reader.Close()

	// Restore body as uncompressed
	c.Res.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Update headers to reflect uncompressed state
	c.Res.ContentLength = int64(len(bodyBytes))
	c.Res.Header.Set("Content-Length", fmt.Sprintf("%d", len(bodyBytes)))
	c.Res.Header.Del("Content-Encoding")

	return string(bodyBytes), nil
}

// SetResponseBody sets the response body
func (c *Context) SetResponseBody(body string) {
	if c.Res != nil {
		c.Res.Body = io.NopCloser(strings.NewReader(body))
		c.Res.ContentLength = int64(len(body))
		c.Res.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))
		c.Res.Header.Del("Content-Encoding") // Remove encoding if we modified body
	}
}

// TargetConfig defines where to forward requests
type TargetConfig struct {
	Protocol string // http, https, ws, wss
	Host     string
	Port     int
}

// MockResponse defines a static response to return
type MockResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       interface{} // string or []byte
}

// GetTargetURL returns the full target URL for forwarding
func (t *TargetConfig) GetTargetURL(path string) string {
	if t == nil {
		return ""
	}
	return t.Protocol + "://" + t.Host + ":" + fmt.Sprintf("%d", t.Port) + path
}

// GetHostPort returns the host:port combination
func (t *TargetConfig) GetHostPort() string {
	if t == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}

// GetDefaultPort returns the default port for the protocol
func (t *TargetConfig) GetDefaultPort() int {
	if t.Port > 0 {
		return t.Port
	}
	switch t.Protocol {
	case "https", "wss":
		return 443
	default:
		return 80
	}
}
