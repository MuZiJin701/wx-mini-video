package echo

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/ltaoo/echo/cert"
)

// newTestCertManager builds an in-memory CA so cleanup tests do not depend on
// fixture files.
func newTestCertManager(t *testing.T) *cert.Manager {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "echo cleanup test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := cert.NewManager(caCert, key)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestConnectHandlerCloseReleasesMitmServers(t *testing.T) {
	manager := newTestCertManager(t)
	loader, err := NewPluginLoader(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &ConnectHandler{CertManager: manager, PluginLoader: loader}

	server, err := handler.getMitmServer("cleanup.example")
	if err != nil {
		t.Fatal(err)
	}
	if server.server == nil {
		t.Fatal("mitm server has no http.Server to shut down")
	}
	cached, err := manager.GetCertificate("cache.example")
	if err != nil {
		t.Fatal(err)
	}

	if err := handler.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.mitmServers.Load("cleanup.example"); ok {
		t.Fatal("mitm server map entry survived Close")
	}
	assertListenerClosed(t, server.Listener)
	if _, err := handler.getMitmServer("after-close.example"); err == nil {
		t.Fatal("getMitmServer accepted a new listener after Close")
	}
	regenerated, err := manager.GetCertificate("cache.example")
	if err != nil {
		t.Fatal(err)
	}
	if regenerated == cached {
		t.Fatal("certificate cache survived Close")
	}
	if err := handler.Close(); err != nil {
		t.Fatalf("Close is not idempotent: %v", err)
	}
}

func TestEchoCloseReleasesConnectHandlerResources(t *testing.T) {
	manager := newTestCertManager(t)
	loader, err := NewPluginLoader(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := &ConnectHandler{CertManager: manager, PluginLoader: loader}
	if _, err := handler.getMitmServer("echo-close.example"); err != nil {
		t.Fatal(err)
	}
	e := &Echo{connectHandler: handler}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := handler.mitmServers.Load("echo-close.example"); ok {
		t.Fatal("Echo.Close left a MITM listener behind")
	}
}

func assertListenerClosed(t *testing.T, listener net.Listener) {
	t.Helper()
	if tcp, ok := listener.(*net.TCPListener); ok {
		_ = tcp.SetDeadline(time.Now().Add(time.Second))
	}
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("listener Accept() error = %v, want net.ErrClosed", err)
	}
}
