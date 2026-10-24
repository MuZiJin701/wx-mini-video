package echo

import (
	"bufio"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestPeekTLSClientHelloDoesNotWaitForFullPeekBuffer(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	const hostname = "www.qq.com"
	clientHello := testTLSClientHello(t, hostname)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Write(clientHello)
		errCh <- err
	}()

	done := make(chan struct{})
	var peeked []byte
	var err error
	go func() {
		peeked, err = peekTLSClientHello(bufio.NewReaderSize(server, tcpRelayPeekSize))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("peekTLSClientHello blocked waiting for the full peek buffer")
	}
	if err != nil {
		t.Fatalf("peekTLSClientHello: %v", err)
	}
	if len(peeked) != len(clientHello) {
		t.Fatalf("peeked length: got %d, want %d", len(peeked), len(clientHello))
	}
	if sni, err := parseSNI(peeked); err != nil || sni != hostname {
		t.Fatalf("parseSNI: got %q, err %v; want %q", sni, err, hostname)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("write ClientHello: %v", err)
	}
}

func testTLSClientHello(t *testing.T, hostname string) []byte {
	t.Helper()

	serverName := []byte(hostname)
	serverNameList := make([]byte, 2+1+2+len(serverName))
	binary.BigEndian.PutUint16(serverNameList[0:2], uint16(1+2+len(serverName)))
	serverNameList[2] = 0x00
	binary.BigEndian.PutUint16(serverNameList[3:5], uint16(len(serverName)))
	copy(serverNameList[5:], serverName)

	serverNameExt := make([]byte, 4+len(serverNameList))
	binary.BigEndian.PutUint16(serverNameExt[0:2], 0x0000)
	binary.BigEndian.PutUint16(serverNameExt[2:4], uint16(len(serverNameList)))
	copy(serverNameExt[4:], serverNameList)

	body := make([]byte, 0, 2+32+1+2+2+1+1+2+len(serverNameExt))
	body = append(body, 0x03, 0x03)
	body = append(body, make([]byte, 32)...)
	body = append(body, 0x00)
	body = append(body, 0x00, 0x02, 0x13, 0x01)
	body = append(body, 0x01, 0x00)
	body = append(body, byte(len(serverNameExt)>>8), byte(len(serverNameExt)))
	body = append(body, serverNameExt...)

	handshake := make([]byte, 4+len(body))
	handshake[0] = 0x01
	handshake[1] = byte(len(body) >> 16)
	handshake[2] = byte(len(body) >> 8)
	handshake[3] = byte(len(body))
	copy(handshake[4:], body)

	record := make([]byte, 5+len(handshake))
	record[0] = 0x16
	record[1] = 0x03
	record[2] = 0x01
	binary.BigEndian.PutUint16(record[3:5], uint16(len(handshake)))
	copy(record[5:], handshake)

	return record
}
