# Echo

A simplest implementation of a proxy server in Go, inspired by [Whistle](https://github.com/avwo/whistle).

## Features

- **HTTP Proxy**: Supports standard HTTP proxying.
- **HTTPS/TCP Tunneling**: Supports `CONNECT` method for HTTPS and generic TCP tunneling.
- **WebSocket Support**: Supports WebSocket upgrades (hijacking) and tunneling.
- **Plugin System**: Flexible plugin system to modify requests and responses.

## Installation

```bash
go get github.com/ltaoo/echo
```

## Quick Start

To start the proxy server, you need to provide a Root CA certificate and private key.

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ltaoo/echo"
)

func main() {
	// 1. Load Root CA (You need to generate these or use existing ones)
	// Ensure you have 'certs/rootCA.crt' and 'certs/rootCA.key'
	certFile, err := os.ReadFile("certs/rootCA.crt")
	if err != nil {
		log.Fatalf("Failed to read cert file: %v", err)
	}
	keyFile, err := os.ReadFile("certs/rootCA.key")
	if err != nil {
		log.Fatalf("Failed to read key file: %v", err)
	}

	// 2. Initialize Echo
	e, err := echo.NewEcho(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to initialize Echo: %v", err)
	}

	// 3. Start Server
	server := &http.Server{
		Addr:    ":8888",
		Handler: e,
	}

	log.Println("Echo Proxy listening on :8888")
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
```

## Plugins

You can add plugins to intercept and modify requests/responses.

```go
package main

import (
	"fmt"
	"strings"

	"github.com/ltaoo/echo"
	"github.com/ltaoo/echo/plugin"
)

func main() {
	// ... (Load certs as above) ...

	e, _ := echo.NewEcho(certFile, keyFile)

	// Define a plugin
	myPlugin := &plugin.Plugin{
		Match: "example.com", // Match requests to example.com
		OnRequest: func(ctx *plugin.Context) {
			fmt.Println("Intercepted request to example.com")
			ctx.SetRequestHeader("X-Custom-Header", "MyPlugin")
		},
		OnResponse: func(ctx *plugin.Context) {
			// Modify response body
			body, _ := ctx.GetResponseBody()
			newBody := strings.ReplaceAll(body, "Example Domain", "Hacked Domain")
			ctx.SetResponseBody(newBody)
		},
	}

	// Add plugin
	e.AddPlugin(myPlugin)

	// ... (Start server) ...
}
```

## Usage

1. Configure your browser or client to use the proxy:
   - **Proxy Host**: `127.0.0.1`
   - **Proxy Port**: `8888`

2. Test with curl:
   ```bash
   # HTTP
   curl -x http://127.0.0.1:8888 http://example.com

   # HTTPS
   curl -x http://127.0.0.1:8888 https://example.com
   ```

## Upstream Proxy Support

Echo supports forwarding requests through an upstream proxy, enabling it to work with other proxy software:

```go
echo_proxy, err := echo.NewEchoWithOptions(certFile, keyFile, &echo.Options{
    UpstreamProxy: "http://127.0.0.1:7890", // HTTP/HTTPS proxy
    // Or SOCKS5:
    // UpstreamProxy: "socks5://127.0.0.1:1080",
})
```

**Usage with other proxies:**
1. Set your system/network proxy to another proxy (e.g., port 8899)
2. Configure Echo's `UpstreamProxy` to point to itself or another forward proxy
3. Traffic flow: App → Other Proxy → Echo → UpstreamProxy → Target

This allows Echo to coexist with VPN clients, Clash, V2Ray, or other proxy tools.

## Implementation Details

- Uses Go's `net/http` for server handling.
- `handleHTTP` for standard proxy requests (removes hop-by-hop headers).
- `handleTunnel` for `CONNECT` requests (hijacks connection and tunnels TCP).
- `handleWebSocket` for `Upgrade: websocket` requests (hijacks connection and tunnels TCP).
