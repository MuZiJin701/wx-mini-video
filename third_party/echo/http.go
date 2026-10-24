package echo

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

// HTTPHandler handles standard HTTP proxy requests
type HTTPHandler struct {
	PluginLoader      *PluginLoader
	Transport         *http.Transport
	FallbackTransport *http.Transport // 直连，用于上游代理不可用时 fallback
	UpstreamProxy     string
}

// NewHTTPHandler creates a new HTTP handler with a custom transport
func NewHTTPHandler(loader *PluginLoader) *HTTPHandler {
	return NewHTTPHandlerWithUpstream(loader, "")
}

// NewHTTPHandlerWithUpstream creates a new HTTP handler with upstream proxy support
func NewHTTPHandlerWithUpstream(loader *PluginLoader, upstreamProxy string) *HTTPHandler {
	var proxyFunc func(*http.Request) (*url.URL, error)
	if upstreamProxy != "" {
		proxyURL, err := url.Parse(upstreamProxy)
		if err != nil {
			log.Printf("[UpstreamProxy] Invalid proxy URL: %v, falling back to direct", err)
			proxyFunc = nil
		} else {
			// 自定义 proxy 函数，带 fallback 到直连
			proxyURL := proxyURL
			proxyFunc = func(req *http.Request) (*url.URL, error) {
				return proxyURL, nil
			}
			log.Printf("[UpstreamProxy] Using upstream proxy: %s (with fallback to direct)", upstreamProxy)
		}
	} else {
		proxyFunc = http.ProxyFromEnvironment
	}

	return &HTTPHandler{
		PluginLoader:  loader,
		UpstreamProxy: upstreamProxy,
		Transport: &http.Transport{
			Proxy: proxyFunc,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false, // Disable HTTP/2
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// Disable HTTP/2 by setting TLSNextProto to non-nil empty map
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
		FallbackTransport: &http.Transport{
			Proxy: nil, // 直连
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
	}
}

// HandleRequest processes the HTTP request
func (h *HTTPHandler) HandleRequest(w http.ResponseWriter, r *http.Request) {
	// Remove proxy headers
	DelHopHeaders(r.Header)

	// Parse target URL
	if r.URL.Scheme == "" {
		r.URL.Scheme = "http"
	}
	if r.URL.Host == "" {
		r.URL.Host = r.Host
	}

	hostname := r.URL.Hostname()
	path := r.URL.Path + r.URL.RawQuery

	log.Printf("[HTTP] %s %s (Host: %s)", r.Method, r.URL.String(), hostname)

	// Find all matching plugins
	matched_plugins := h.PluginLoader.MatchPluginsForRequest(r)

	// Create Plugin Context
	ctx := &Context{Req: r}

	// Apply OnRequest hooks in order; last Target wins
	var selected_target *TargetConfig
	if len(matched_plugins) > 0 {
		log.Printf("[HTTP] %d plugin(s) matched for %s", len(matched_plugins), hostname)
		for _, p := range matched_plugins {
			if p.OnRequest != nil {
				p.OnRequest(ctx)
				if mockResp := ctx.GetMockResponse(); mockResp != nil {
					log.Printf("[PLUGIN] Returning direct response for %s", path)
					h.sendMockResponse(w, mockResp)
					return
				}
			}
			if p.Target != nil {
				selected_target = p.Target
			}
		}
		if selected_target != nil {
			targetProtocol := selected_target.Protocol
			if targetProtocol == "" {
				if selected_target.Port == 443 {
					targetProtocol = "https"
				} else {
					targetProtocol = "http"
				}
			}

			// Construct target URL for logging
			targetURL := targetProtocol + "://" + selected_target.GetHostPort() + path
			log.Printf("[PLUGIN] Forwarding %s -> %s", hostname, targetURL)

			r.URL.Scheme = targetProtocol
			r.URL.Host = selected_target.GetHostPort()
			r.Host = selected_target.GetHostPort()
		}
	}

	// Create client with custom transport
	client := &http.Client{
		Transport: h.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Create new request
	// We need to read body if present
	var bodyReader io.Reader
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		bodyReader = bytes.NewReader(bodyBytes)
		// Re-assign body to request for potential reuse if we weren't creating a new request
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	proxyReq, err := http.NewRequest(r.Method, r.URL.String(), bodyReader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Copy headers
	CopyHeader(proxyReq.Header, r.Header)
	DelHopHeaders(proxyReq.Header)

	// Send request with fallback to direct when upstream proxy fails
	var resp *http.Response
	sendErr := error(nil)
	resp, sendErr = client.Do(proxyReq)
	if sendErr != nil && h.UpstreamProxy != "" && h.FallbackTransport != nil {
		log.Printf("[UpstreamProxy] Failed, falling back to direct: %v", sendErr)
		fallbackClient := &http.Client{
			Transport: h.FallbackTransport,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, sendErr = fallbackClient.Do(proxyReq)
	}
	if sendErr != nil {
		log.Printf("[HTTP Error] %v", sendErr)
		http.Error(w, sendErr.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Apply OnResponse hooks of all matched plugins in order
	if len(matched_plugins) > 0 {
		ctx.Res = resp
		for _, p := range matched_plugins {
			if p.OnResponse != nil {
				p.OnResponse(ctx)
			}
		}
	}

	// Copy response headers
	DelHopHeaders(resp.Header)
	CopyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)
}

// forwardDirect forwards requests directly without MITM for sensitive services
func (h *HTTPHandler) forwardDirect(w http.ResponseWriter, r *http.Request) {
	// Remove proxy headers
	DelHopHeaders(r.Header)

	// Create client with custom transport that doesn't verify proxy certificates for Apple domains
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true, // Enable HTTP/2 for better performance
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			// Use system root CAs for proper certificate validation
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Read body if present
	var bodyReader io.Reader
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(r.Body)
		r.Body.Close()
		bodyReader = bytes.NewReader(bodyBytes)
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Create new request
	proxyReq, err := http.NewRequest(r.Method, r.URL.String(), bodyReader)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Copy headers
	CopyHeader(proxyReq.Header, r.Header)
	DelHopHeaders(proxyReq.Header)

	// Send request
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("[Apple Direct Error] %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	DelHopHeaders(resp.Header)
	CopyHeader(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)
}

func (h *HTTPHandler) sendMockResponse(w http.ResponseWriter, mock *MockResponse) {
	for k, v := range mock.Headers {
		w.Header().Set(k, v)
	}
	w.WriteHeader(mock.StatusCode)

	switch v := mock.Body.(type) {
	case string:
		w.Write([]byte(v))
	case []byte:
		w.Write(v)
	}
}
