//go:build windows

package windows

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	relayBufSize    = 131072 // 128KB transfer buffer
	relaySocketBuf  = 524288 // 512KB socket buffer
	relayTimeout    = 30 * time.Second
)

// Relay accepts transparent TCP connections on a local port, looks up the
// original destination via the NAT table, and forwards the traffic to the
// Echo HTTP proxy using standard HTTP proxy protocol (CONNECT or absolute URL).
type Relay struct {
	listenAddr string
	echoAddr   string
	nat        *NATTable
	listener   net.Listener
	wg         sync.WaitGroup
	running    bool
	mu         sync.Mutex
}

// NewRelay creates a new TCP relay.
func NewRelay(listenAddr, echoAddr string, nat *NATTable) *Relay {
	return &Relay{
		listenAddr: listenAddr,
		echoAddr:   echoAddr,
		nat:        nat,
	}
}

// Start begins listening for connections.
func (r *Relay) Start() error {
	ln, err := net.Listen("tcp4", r.listenAddr)
	if err != nil {
		return fmt.Errorf("relay listen failed: %w", err)
	}
	r.mu.Lock()
	r.listener = ln
	r.running = true
	r.mu.Unlock()

	r.wg.Add(1)
	go r.acceptLoop()
	return nil
}

// Stop shuts down the relay and waits for all connections to finish.
func (r *Relay) Stop() {
	r.mu.Lock()
	r.running = false
	if r.listener != nil {
		r.listener.Close()
	}
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *Relay) acceptLoop() {
	defer r.wg.Done()

	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.mu.Lock()
			running := r.running
			r.mu.Unlock()
			if !running {
				return
			}
			continue
		}

		r.wg.Add(1)
		go r.handleConnection(conn)
	}
}

func (r *Relay) handleConnection(clientConn net.Conn) {
	defer r.wg.Done()
	defer clientConn.Close()

	// Get client's source port to look up NAT table
	clientAddr := clientConn.RemoteAddr().(*net.TCPAddr)
	srcPort := uint16(clientAddr.Port)

	destIP, destPort, ok := r.nat.Lookup(srcPort)
	if !ok {
		log.Printf("[relay] no NAT entry for src_port=%d", srcPort)
		return
	}

	host := IP32to4(destIP)

	// Connect to Echo proxy
	echoConn, err := net.DialTimeout("tcp4", r.echoAddr, relayTimeout)
	if err != nil {
		log.Printf("[relay] failed to connect to echo proxy %s: %v", r.echoAddr, err)
		return
	}
	defer echoConn.Close()

	// Configure socket buffers and timeouts
	setConnOptions(clientConn)
	setConnOptions(echoConn)

	if destPort == 80 {
		r.handleHTTP(clientConn, echoConn, host, destPort)
	} else {
		r.handleCONNECT(clientConn, echoConn, host, destPort)
	}
}

// handleCONNECT sends a CONNECT request to establish a tunnel through Echo.
func (r *Relay) handleCONNECT(clientConn, echoConn net.Conn, host string, port uint16) {
	target := fmt.Sprintf("%s:%d", host, port)

	// Send CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := echoConn.Write([]byte(connectReq)); err != nil {
		log.Printf("[relay] CONNECT write failed: %v", err)
		return
	}

	// Read response
	br := bufio.NewReader(echoConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		log.Printf("[relay] CONNECT response read failed: %v", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[relay] CONNECT failed: %s", resp.Status)
		return
	}

	// Bidirectional pipe. If bufio reader has buffered data, we need to
	// use it instead of the raw conn for the echo side.
	var echoReader io.Reader = br
	if br.Buffered() == 0 {
		echoReader = echoConn
	}

	// Bidirectional relay
	biRelay(clientConn, echoConn, echoReader)
}

// handleHTTP rewrites plain HTTP requests to absolute-URL form for the proxy.
func (r *Relay) handleHTTP(clientConn, echoConn net.Conn, host string, port uint16) {
	clientReader := bufio.NewReader(clientConn)
	echoReader := bufio.NewReader(echoConn)

	for {
		req, err := http.ReadRequest(clientReader)
		if err != nil {
			return
		}

		// Rewrite to absolute URL
		req.URL.Scheme = "http"
		if port != 80 {
			req.URL.Host = fmt.Sprintf("%s:%d", host, port)
		} else {
			req.URL.Host = host
		}
		req.RequestURI = req.URL.String()

		// Forward to Echo using proxy format
		if err := req.WriteProxy(echoConn); err != nil {
			req.Body.Close()
			return
		}
		req.Body.Close()

		// Read Echo's response and forward back to client
		resp, err := http.ReadResponse(echoReader, req)
		if err != nil {
			return
		}
		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return
		}
		resp.Body.Close()

		// Check Connection: close
		if resp.Close || req.Close {
			return
		}
	}
}

// biRelay performs bidirectional data transfer between two connections.
func biRelay(client net.Conn, server net.Conn, serverReader io.Reader) {
	done := make(chan struct{}, 2)

	go func() {
		buf := make([]byte, relayBufSize)
		io.CopyBuffer(server, client, buf)
		// Signal the other direction by closing write half if possible
		if tc, ok := server.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		buf := make([]byte, relayBufSize)
		io.CopyBuffer(client, serverReader, buf)
		if tc, ok := client.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Wait for both directions to finish
	<-done
	<-done
}

func setConnOptions(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetReadBuffer(relaySocketBuf)
		tc.SetWriteBuffer(relaySocketBuf)
		tc.SetDeadline(time.Now().Add(relayTimeout))
	}
}
