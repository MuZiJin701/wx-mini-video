package echo

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	tcpRelayBufSize  = 131072 // 128KB transfer buffer
	tcpRelayTimeout  = 30 * time.Second
	tcpRelayPeekSize = 8192 // enough for ClientHello with SNI
)

// TCPRelay accepts raw TCP connections on a listen port, infers the target
// destination from TLS SNI or HTTP Host header, and forwards traffic to an
// upstream Echo HTTP proxy using standard HTTP proxy protocol.
//
// Chrome → ProxyBridge (raw TCP) → TCPRelay :9900 → (HTTP proxy) → Echo :8899
type TCPRelay struct {
	listenAddr string
	echoAddr   string
	listener   net.Listener
	wg         sync.WaitGroup
	running    bool
	mu         sync.Mutex
}

// NewTCPRelay creates a new TCP relay.
func NewTCPRelay(listenAddr, echoAddr string) *TCPRelay {
	return &TCPRelay{
		listenAddr: listenAddr,
		echoAddr:   echoAddr,
	}
}

// Start begins listening for raw TCP connections.
func (r *TCPRelay) Start() error {
	ln, err := net.Listen("tcp", r.listenAddr)
	if err != nil {
		return fmt.Errorf("tcp_relay listen failed: %w", err)
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
func (r *TCPRelay) Stop() {
	r.mu.Lock()
	r.running = false
	if r.listener != nil {
		r.listener.Close()
	}
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *TCPRelay) acceptLoop() {
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

func (r *TCPRelay) handleConnection(clientConn net.Conn) {
	defer r.wg.Done()
	defer clientConn.Close()

	clientConn.SetDeadline(time.Now().Add(tcpRelayTimeout))

	br := bufio.NewReaderSize(clientConn, tcpRelayPeekSize)

	// Peek first byte to determine protocol: TLS (0x16) or HTTP
	firstByte, err := br.Peek(1)
	if err != nil {
		log.Printf("[tcp_relay] failed to peek first byte: %v", err)
		return
	}

	// Connect to Echo proxy
	echoConn, err := net.DialTimeout("tcp", r.echoAddr, tcpRelayTimeout)
	if err != nil {
		log.Printf("[tcp_relay] failed to connect to echo %s: %v", r.echoAddr, err)
		return
	}
	defer echoConn.Close()

	setTCPRelayConnOptions(clientConn)
	setTCPRelayConnOptions(echoConn)

	if firstByte[0] == 0x16 {
		// TLS ClientHello
		r.handleCONNECT(clientConn, echoConn, br)
	} else {
		// Assume HTTP
		r.handleHTTP(clientConn, echoConn, br)
	}
}

// handleCONNECT parses the TLS ClientHello SNI, sends a CONNECT request to
// Echo, then relays raw bytes bidirectionally.
func (r *TCPRelay) handleCONNECT(clientConn, echoConn net.Conn, clientReader *bufio.Reader) {
	peeked, err := peekTLSClientHello(clientReader)
	if err != nil {
		log.Printf("[tcp_relay] failed to peek TLS data: %v", err)
		return
	}

	sni, err := parseSNI(peeked)
	if err != nil {
		log.Printf("[tcp_relay] SNI parse failed: %v", err)
		return
	}

	target := fmt.Sprintf("%s:443", sni)

	// Send CONNECT to Echo
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := echoConn.Write([]byte(connectReq)); err != nil {
		log.Printf("[tcp_relay] CONNECT write failed: %v", err)
		return
	}

	// Read CONNECT response
	echoReader := bufio.NewReader(echoConn)
	resp, err := http.ReadResponse(echoReader, nil)
	if err != nil {
		log.Printf("[tcp_relay] CONNECT response read failed: %v", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[tcp_relay] CONNECT failed: %s", resp.Status)
		return
	}

	// Clear deadlines for long-lived tunnel
	clientConn.SetDeadline(time.Time{})
	echoConn.SetDeadline(time.Time{})

	// Bidirectional relay. Use echoReader if it has buffered data from the
	// CONNECT response, otherwise use the raw conn.
	var echoIn io.Reader = echoReader
	if echoReader.Buffered() == 0 {
		echoIn = echoConn
	}

	biRelay(clientConn, echoConn, clientReader, echoIn)
}

func peekTLSClientHello(clientReader *bufio.Reader) ([]byte, error) {
	header, err := clientReader.Peek(5)
	if err != nil {
		return nil, err
	}
	if header[0] != 0x16 {
		return nil, fmt.Errorf("not a TLS handshake record: 0x%02x", header[0])
	}

	recordLen := int(binary.BigEndian.Uint16(header[3:5]))
	totalLen := 5 + recordLen
	if totalLen > tcpRelayPeekSize {
		return nil, fmt.Errorf("TLS ClientHello too large: %d bytes", totalLen)
	}

	return clientReader.Peek(totalLen)
}

// handleHTTP rewrites plain HTTP requests to absolute-URL proxy form, forwards
// them to Echo, and writes responses back to the client.
func (r *TCPRelay) handleHTTP(clientConn, echoConn net.Conn, clientReader *bufio.Reader) {
	echoReader := bufio.NewReader(echoConn)

	for {
		clientConn.SetDeadline(time.Now().Add(tcpRelayTimeout))
		echoConn.SetDeadline(time.Now().Add(tcpRelayTimeout))

		req, err := http.ReadRequest(clientReader)
		if err != nil {
			return
		}

		// Extract target host
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		if host == "" {
			log.Printf("[tcp_relay] HTTP request without Host header")
			req.Body.Close()
			return
		}

		// Rewrite to absolute URL for proxy forwarding
		req.URL.Scheme = "http"
		req.URL.Host = host
		req.RequestURI = req.URL.String()

		// Forward to Echo
		if err := req.WriteProxy(echoConn); err != nil {
			req.Body.Close()
			return
		}
		req.Body.Close()

		// Read Echo's response
		resp, err := http.ReadResponse(echoReader, req)
		if err != nil {
			return
		}

		// Write response back to client
		if err := resp.Write(clientConn); err != nil {
			resp.Body.Close()
			return
		}
		resp.Body.Close()

		// Respect Connection: close
		if resp.Close || req.Close {
			return
		}
	}
}

// parseSNI extracts the SNI hostname from a TLS ClientHello message.
// It implements a minimal parser that walks the TLS record → handshake →
// extensions chain to find the server_name (type 0x0000) extension.
func parseSNI(data []byte) (string, error) {
	// Minimum: 5-byte TLS record header + 38-byte handshake fixed header
	if len(data) < 43 {
		return "", fmt.Errorf("data too short for TLS record: %d bytes", len(data))
	}

	// TLS Record Layer
	if data[0] != 0x16 { // Handshake content type
		return "", fmt.Errorf("not a TLS handshake record: 0x%02x", data[0])
	}

	recordLen := int(binary.BigEndian.Uint16(data[3:5]))
	payloadEnd := 5 + recordLen
	if payloadEnd > len(data) {
		payloadEnd = len(data)
	}
	payload := data[5:payloadEnd]

	if len(payload) < 38 {
		return "", fmt.Errorf("handshake body too short: %d bytes", len(payload))
	}

	// Handshake protocol
	if payload[0] != 0x01 { // ClientHello
		return "", fmt.Errorf("not a ClientHello: 0x%02x", payload[0])
	}

	// Skip: handshake type (1) + length (3) + version (2) + random (32) = 38 bytes
	pos := 38

	// Session ID
	if pos >= len(payload) {
		return "", fmt.Errorf("unexpected end at session ID")
	}
	sessionIDLen := int(payload[pos])
	pos += 1 + sessionIDLen
	if pos+2 > len(payload) {
		return "", fmt.Errorf("unexpected end at cipher suites")
	}

	// Cipher Suites
	cipherSuitesLen := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2 + cipherSuitesLen
	if pos+1 > len(payload) {
		return "", fmt.Errorf("unexpected end at compression methods")
	}

	// Compression Methods
	compressionLen := int(payload[pos])
	pos += 1 + compressionLen
	if pos+2 > len(payload) {
		return "", fmt.Errorf("no extensions in ClientHello")
	}

	// Extensions
	extensionsLen := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
	pos += 2
	extensionsEnd := pos + extensionsLen
	if extensionsEnd > len(payload) {
		extensionsEnd = len(payload)
	}

	// Walk extensions looking for server_name (type 0x0000)
	for pos+4 <= extensionsEnd {
		extType := binary.BigEndian.Uint16(payload[pos : pos+2])
		extLen := int(binary.BigEndian.Uint16(payload[pos+2 : pos+4]))
		pos += 4

		if extType == 0x0000 { // server_name
			if pos+5 > len(payload) {
				return "", fmt.Errorf("SNI extension too short")
			}

			listLen := int(binary.BigEndian.Uint16(payload[pos : pos+2]))
			pos += 2
			listEnd := pos + listLen
			if listEnd > len(payload) {
				listEnd = len(payload)
			}

			for pos+3 <= listEnd {
				nameType := payload[pos]
				nameLen := int(binary.BigEndian.Uint16(payload[pos+1 : pos+3]))
				pos += 3
				if nameType == 0x00 && nameLen > 0 && pos+nameLen <= len(payload) {
					return string(payload[pos : pos+nameLen]), nil
				}
				pos += nameLen
			}
			return "", fmt.Errorf("SNI extension missing hostname entry")
		}

		pos += extLen
	}

	return "", fmt.Errorf("no SNI extension found")
}

// biRelay performs bidirectional data transfer between two connections.
func biRelay(clientConn, serverConn net.Conn, clientReader, serverReader io.Reader) {
	done := make(chan struct{}, 2)
	buf1 := make([]byte, tcpRelayBufSize)
	buf2 := make([]byte, tcpRelayBufSize)

	go func() {
		io.CopyBuffer(serverConn, clientReader, buf1)
		if tc, ok := serverConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		io.CopyBuffer(clientConn, serverReader, buf2)
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	<-done
	<-done
}

func setTCPRelayConnOptions(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
		tc.SetReadBuffer(tcpRelayBufSize)
		tc.SetWriteBuffer(tcpRelayBufSize)
	}
}
