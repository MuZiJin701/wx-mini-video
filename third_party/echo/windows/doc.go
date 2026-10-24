//go:build windows

// Package windows provides transparent per-process TCP interception for the
// Echo HTTP MITM proxy on Windows.
//
// It uses WinDivert to intercept outbound TCP connections from specified
// processes at the kernel level, NAT-rewrites them to a local relay port,
// and the relay forwards the traffic to Echo using standard HTTP proxy
// protocol (CONNECT for HTTPS, absolute-URL for HTTP).
//
// Architecture:
//
//	Target Process (chrome.exe)            Echo Proxy (:8899)
//	    │ outbound TCP                          ▲
//	    ▼                                       │ HTTP proxy protocol
//	WinDivert kernel intercept                  │
//	    │ NAT rewrite dst → 127.0.0.1:34010    │
//	    ▼                                       │
//	Relay Server (:34010) ──────────────────────┘
//	    (CONNECT / absolute-URL conversion)
//
// Requirements:
//   - Windows 10+ (64-bit)
//   - Administrator privileges
//   - WinDivert.dll and WinDivert64.sys in the executable directory
//
// Example usage:
//
//	// Create and start Echo proxy
//	e, _ := echo.NewEcho(certPEM, keyPEM)
//	e.AddPlugin(&echo.Plugin{
//	    Match: "*.example.com",
//	    OnRequest: func(ctx *echo.Context) {
//	        // inspect/modify request
//	    },
//	})
//	go http.ListenAndServe(":8899", e)
//
//	// Create the interceptor targeting Echo
//	interceptor := windows.NewInterceptor("127.0.0.1:8899")
//
//	// Add rules for which processes to intercept
//	interceptor.AddRule(&windows.ProcessRule{
//	    ProcessName: "chrome.exe",
//	    TargetHosts: "*",
//	    TargetPorts: "*",
//	    Action:      windows.ActionProxy,
//	    Enabled:     true,
//	})
//
//	// Optionally monitor intercepted connections
//	interceptor.SetConnectionCallback(func(pid uint32, name string,
//	    srcIP, dstIP uint32, srcPort, dstPort uint16, action windows.Action) {
//	    fmt.Printf("[%s] %s:%d → %s:%d (%s)\n",
//	        name, windows.IP32to4(srcIP), srcPort,
//	        windows.IP32to4(dstIP), dstPort, action)
//	})
//
//	// Start intercepting (requires admin privileges)
//	if err := interceptor.Start(); err != nil {
//	    log.Fatal(err)
//	}
//	defer interceptor.Stop()
package windows
