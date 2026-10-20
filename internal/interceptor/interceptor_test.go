package interceptor

import (
	"errors"
	"testing"

	"wx_channel/pkg/certificate"
	"wx_channel/pkg/system"
)

func TestIsOwnProxySnapshot(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "plain", raw: "127.0.0.1:2023", want: true},
		{name: "per scheme", raw: "http=127.0.0.1:2023;https=127.0.0.1:2023", want: true},
		{name: "other proxy", raw: "proxy.example.com:8080", want: false},
		{name: "longer port", raw: "http=127.0.0.1:20230", want: false},
		{name: "address suffix", raw: "https=localhost:2023.example.com:8080", want: false},
		{name: "mixed proxies", raw: "http=proxy.example.com:8080;https=127.0.0.1:2023", want: false},
		{name: "spaces", raw: "http=127.0.0.1:2023; https=localhost:2023", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isOwnProxySnapshot(&system.ProxySnapshot{
				Enabled:   true,
				HasServer: true,
				Server:    tc.raw,
			}, "127.0.0.1", "2023")
			if got != tc.want {
				t.Fatalf("isOwnProxySnapshot(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestStartRollsBackPartiallyEnabledProxy(t *testing.T) {
	originalCapture, originalEnable, originalRestore := captureProxySnapshot, enableSystemProxy, restoreProxySnapshot
	t.Cleanup(func() {
		captureProxySnapshot, enableSystemProxy, restoreProxySnapshot = originalCapture, originalEnable, originalRestore
	})
	snapshot := &system.ProxySnapshot{Enabled: true, HasServer: true, Server: "proxy.example.com:8080"}
	captureProxySnapshot = func() (*system.ProxySnapshot, error) { return snapshot, nil }
	enableErr := errors.New("proxy address write failed")
	enableSystemProxy = func(system.ProxySettings) error { return enableErr }
	restored := 0
	rollbackErr := errors.New("proxy restore failed")
	restoreProxySnapshot = func(got *system.ProxySnapshot) error {
		if got != snapshot {
			t.Fatalf("restored snapshot = %#v, want %#v", got, snapshot)
		}
		restored++
		if restored == 1 {
			return rollbackErr
		}
		return nil
	}
	c := NewInterceptor(&InterceptorConfig{
		ProxySetSystem: true, ProxySkipInstallRootCert: true,
		ProxyServerHostname: "127.0.0.1", ProxyServerPort: 2023,
	}, testRootCertificate(t))
	if err := c.Start(); !errors.Is(err, enableErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("Start() = %v, want setup and rollback errors", err)
	}
	if restored != 1 || c.proxy != nil || c.proxyState != snapshot {
		t.Fatalf("failed rollback lost cleanup state: restores=%d, proxy=%v, snapshot=%v", restored, c.proxy, c.proxyState)
	}
	if err := c.Stop(); err != nil || restored != 2 || c.proxyState != nil {
		t.Fatalf("retry cleanup = %v, restores=%d, snapshot=%v", err, restored, c.proxyState)
	}
	if err := c.Stop(); err != nil || restored != 2 {
		t.Fatalf("Stop after cleanup = %v, restores=%d", err, restored)
	}
}

func TestStartLeavesSystemProxyAloneWithoutSnapshot(t *testing.T) {
	originalCapture, originalRestore := captureProxySnapshot, restoreProxySnapshot
	t.Cleanup(func() { captureProxySnapshot, restoreProxySnapshot = originalCapture, originalRestore })
	captureProxySnapshot = func() (*system.ProxySnapshot, error) { return nil, errors.New("snapshot unavailable") }
	restoreProxySnapshot = func(*system.ProxySnapshot) error {
		t.Fatal("must not change system proxy without a captured snapshot")
		return nil
	}
	c := NewInterceptor(&InterceptorConfig{ProxySetSystem: true, ProxySkipInstallRootCert: true}, testRootCertificate(t))
	if err := c.Start(); err == nil {
		t.Fatal("Start succeeded without a proxy snapshot")
	}
	if c.proxy != nil {
		t.Fatal("failed start left proxy open")
	}
	if err := c.Stop(); err != nil {
		t.Fatal(err)
	}
}

func testRootCertificate(t *testing.T) *certificate.CertFileAndKeyFile {
	t.Helper()
	cert, err := certificate.LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
