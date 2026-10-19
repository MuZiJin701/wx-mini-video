package interceptor

import (
	"net"
	"strconv"
	"strings"
	"testing"

	"wx_channel/pkg/certificate"
)

func TestServerRejectsOccupiedPortBeforeProxySetup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	server := NewInterceptorServer(&InterceptorConfig{
		ProxyServerHostname: "127.0.0.1",
		ProxyServerPort:     port,
	}, &certificate.CertFileAndKeyFile{})
	err = server.Start()
	if err == nil || !strings.Contains(err.Error(), "127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("Start() = %v, want listener bind error before certificate setup", err)
	}
}

func TestServerReleasesListenerAfterSetupFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	server := NewInterceptorServer(&InterceptorConfig{
		ProxyServerHostname: "127.0.0.1", ProxyServerPort: port,
		ProxySetSystem: true, ProxySkipInstallRootCert: true,
	}, &certificate.CertFileAndKeyFile{})
	if err := server.Start(); err == nil {
		t.Fatal("Start succeeded with invalid certificate")
	}
	listener, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("failed start left listener open: %v", err)
	}
	defer listener.Close()
}
