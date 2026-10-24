/*
Package echo provides a simple proxy server implementation in Go, inspired by [Whistle].

# Features

  - HTTP Proxy: Supports standard HTTP proxying.
  - HTTPS/TCP Tunneling: Supports CONNECT method for HTTPS and generic TCP tunneling.
  - WebSocket Support: Supports WebSocket upgrades (hijacking) and tunneling.
  - Plugin System: Flexible plugin system to modify requests and responses.

# Quick Start

To start the proxy server, provide a Root CA certificate and private key:

	certFile, _ := os.ReadFile("certs/rootCA.crt")
	keyFile, _ := os.ReadFile("certs/rootCA.key")

	e, err := echo.NewEcho(certFile, keyFile)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:    ":8888",
		Handler: e,
	}
	server.ListenAndServe()

# Plugins

Add plugins to intercept and modify requests/responses:

	e.AddPlugin(&echo.Plugin{
		Match: "example.com",
		OnRequest: func(ctx *echo.Context) {
			ctx.SetRequestHeader("X-Custom-Header", "value")
		},
		OnResponse: func(ctx *echo.Context) {
			body, _ := ctx.GetResponseBody()
			ctx.SetResponseBody(strings.ReplaceAll(body, "old", "new"))
		},
	})

# Forwarding

Use [TargetConfig] to forward requests to a different server:

	e.AddPlugin(&echo.Plugin{
		Match:  "example.com",
		Target: &echo.TargetConfig{Protocol: "http", Host: "localhost", Port: 3000},
	})

# Mock Response

Use [MockResponse] to return a static response:

	e.AddPlugin(&echo.Plugin{
		Match: "example.com/api",
		MockResponse: &echo.MockResponse{
			StatusCode: 200,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       `{"status":"ok"}`,
		},
	})

[Whistle]: https://github.com/avwo/whistle
*/
package echo

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ltaoo/echo/cert"
	"github.com/ltaoo/echo/tun"
)

func init() {
	v := os.Getenv("ECHO_LOG")
	if v == "" {
		return
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "0" || v == "false" || v == "no" || v == "off" {
		log.SetOutput(io.Discard)
	}
}

type Echo struct {
	connectHandler *ConnectHandler
	wsHandler      *WebSocketHandler
	httpHandler    *HTTPHandler
	pluginLoader   *PluginLoader
	tcpRelay       *TCPRelay
	tunServer      *tun.Server
}

// Options configures Echo behavior
type Options struct {
	// EnableBuiltinBypass enables built-in bypass rules for common services
	// that use certificate pinning (Apple, Google, ChatGPT, etc.)
	EnableBuiltinBypass bool

	// InterceptOnlyMatched if true, only intercept requests that match a plugin.
	// By default (false), all HTTPS traffic on port 443 is intercepted.
	// When enabled, unmatched requests are tunneled directly without MITM.
	InterceptOnlyMatched bool

	// UpstreamProxy specifies an upstream proxy to forward requests to.
	// Format: "http://proxy:port" or "socks5://proxy:port"
	// When set, echo will forward all outbound requests through this proxy
	// instead of connecting directly to targets.
	UpstreamProxy string

	// Tun enables TUN-level traffic forwarding (process-based routing).
	// When true, TunConfig is required.
	Tun bool

	// TunConfig is the TUN configuration. Only used when Tun is true.
	// Can be loaded from a file via tun.LoadConfig() or built programmatically.
	TunConfig *tun.TunConfig

	// TunDefaultInterface overrides TunConfig.Route.DefaultInterface.
	// Use this as an initialization-level fallback when automatic default
	// interface detection fails on multi-adapter Windows machines.
	TunDefaultInterface string
}

func NewEcho(certFile []byte, certKey []byte) (*Echo, error) {
	return NewEchoWithOptions(certFile, certKey, nil)
}

// NewEchoWithOptions creates a new Echo instance with custom options
func NewEchoWithOptions(certFile []byte, certKey []byte, opts *Options) (*Echo, error) {
	caCert, caKey, err := cert.LoadRootCA(certFile, certKey)
	if err != nil {
		return nil, err
	}

	// Initialize Certificate Manager
	certManager, err := cert.NewManager(caCert, caKey)
	if err != nil {
		return nil, err
	}

	// Initialize plugins
	var plugins []*Plugin
	if opts != nil && opts.EnableBuiltinBypass {
		plugins = createBypassPlugins()
	}

	pluginLoader, err := NewPluginLoader(plugins)
	if err != nil {
		return nil, err
	}

	// Initialize Proxy Handlers
	var upstreamProxy string
	if opts != nil {
		upstreamProxy = opts.UpstreamProxy
	}
	httpHandler := NewHTTPHandlerWithUpstream(pluginLoader, upstreamProxy)
	connectHandler := &ConnectHandler{
		CertManager:          certManager,
		PluginLoader:         pluginLoader,
		HTTPHandler:          httpHandler,
		InterceptOnlyMatched: opts != nil && opts.InterceptOnlyMatched,
		UpstreamProxy:        upstreamProxy,
	}
	wsHandler := &WebSocketHandler{PluginLoader: pluginLoader}

	e := &Echo{
		connectHandler: connectHandler,
		wsHandler:      wsHandler,
		httpHandler:    httpHandler,
		pluginLoader:   pluginLoader,
	}

	// Initialize TUN if enabled
	if opts != nil && opts.Tun {
		cfg := opts.TunConfig
		if cfg == nil {
			cfg = tun.DefaultConfig()
		}
		if defaultInterface := strings.TrimSpace(opts.TunDefaultInterface); defaultInterface != "" {
			cfg.Route.DefaultInterface = defaultInterface
		}
		// If upstream proxy is configured, route unmatched traffic through it
		// instead of direct, so VPN/proxy users don't experience routing conflicts.
		if upstreamProxy != "" {
			if u, err := url.Parse(upstreamProxy); err == nil {
				p, _ := strconv.Atoi(u.Port())
				if p == 0 {
					if u.Scheme == "socks5" {
						p = 1080
					} else {
						p = 7890
					}
				}
				cfg.Outbounds = append(cfg.Outbounds, tun.OutboundConfig{
					Tag:    "upstream",
					Type:   u.Scheme,
					Server: u.Hostname(),
					Port:   uint16(p),
				})
				cfg.Route.Final = "upstream"
			}
		}
		tunServer, err := tun.New(cfg)
		if err != nil {
			return nil, fmt.Errorf("tun.New: %w", err)
		}
		if err := tunServer.Start(); err != nil {
			tunServer.Close()
			return nil, fmt.Errorf("tun.Start: %w", err)
		}
		e.tunServer = tunServer
	}

	return e, nil
}

func (e *Echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle CONNECT (HTTPS Tunneling)
	if r.Method == http.MethodConnect {
		e.connectHandler.HandleTunnel(w, r)
		return
	}
	// Handle WebSocket Upgrades (HTTP)
	if IsWebSocketRequest(r) {
		e.wsHandler.HandleUpgrade(w, r, false) // false = not secure (ws://)
		return
	}
	// Handle Standard HTTP
	e.httpHandler.HandleRequest(w, r)
}
func (e *Echo) AddPlugin(plugin *Plugin) {
	e.pluginLoader.AddPlugin(plugin)
}

// ListenTCP starts a TCP relay on listenAddr that accepts raw TCP connections and
// forwards them to echoAddr using HTTP proxy protocol.
// The relay infers the target destination from TLS SNI or HTTP Host header.
func (e *Echo) ListenTCP(listenAddr, echoAddr string) error {
	e.tcpRelay = NewTCPRelay(listenAddr, echoAddr)
	return e.tcpRelay.Start()
}

// ShutdownTCP stops the TCP relay if one is running.
func (e *Echo) ShutdownTCP() {
	if e.tcpRelay != nil {
		e.tcpRelay.Stop()
	}
}

// ShutdownTUN stops the TUN forwarder if one is running.
func (e *Echo) ShutdownTUN() {
	if e.tunServer != nil {
		e.tunServer.Close()
		e.tunServer = nil
	}
}

// Close shuts down all components: per-host MITM listeners and their servers,
// the TCP relay, and TUN.
func (e *Echo) Close() error {
	var err error
	if e.connectHandler != nil {
		err = e.connectHandler.Close()
	}
	e.ShutdownTCP()
	e.ShutdownTUN()
	return err
}

func SetLogEnabled(enabled bool) {
	if enabled {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(io.Discard)
	}
	tun.SetLogEnabled(enabled)
}
