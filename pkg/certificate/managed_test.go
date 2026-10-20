package certificate

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	echoCert "github.com/ltaoo/echo/cert"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReloadedManagedRootWorksWithEchoTLS(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	root, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	parsed, key, err := echoCert.LoadRootCA(root.Cert, root.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := echoCert.NewManager(parsed, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("signed by independent root")) }))
	server.TLS = &tls.Config{GetCertificate: manager.GetCertificateFunc()}
	server.StartTLS()
	defer server.Close()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(root.Cert) {
		t.Fatal("invalid root")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "media.example.com"}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Echo TLS handshake with independent root failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "signed by independent root" {
		t.Fatalf("TLS response = %q, %v", body, err)
	}
}

func TestManagedRootIsIndependentAndReusable(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	other, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Cert, again.Cert) || !bytes.Equal(first.PrivateKey, again.PrivateKey) {
		t.Fatal("local root changed across startup")
	}
	if bytes.Equal(first.Cert, other.Cert) || bytes.Equal(first.PrivateKey, other.PrivateKey) {
		t.Fatal("independent stores share a root or private key")
	}
	if bytes.Contains(again.Cert, []byte("PRIVATE KEY")) {
		t.Fatal("public export contains a private key")
	}
	if _, err := Parse(first.Cert, other.PrivateKey); err == nil {
		t.Fatal("accepted a mismatched private key")
	}
	if got, err := Thumbprint(first.Cert); err != nil || len(got) != 40 || got == LegacyRootThumbprint {
		t.Fatalf("invalid or legacy root: %q, %v", got, err)
	}
}

func TestManagedRootNeverReplacesCorruptPrivateKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "root-ca.pem")
	if err := os.WriteFile(path, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(dir); err == nil {
		t.Fatal("silently replaced a damaged root")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "damaged" {
		t.Fatal("damaged root was changed")
	}
}

func TestCertificateTrustUsesFingerprintInsteadOfName(t *testing.T) {
	original := fetchCertificatesForCheck
	t.Cleanup(func() { fetchCertificatesForCheck = original })
	fetchCertificatesForCheck = func() ([]Certificate, error) {
		return []Certificate{{Thumbprint: "AAAA", Subject: CertificateSubject{CN: "same name"}}}, nil
	}
	if found, err := CheckHasCertificate("BBBB"); err != nil || found {
		t.Fatalf("trusted a different certificate: %v, %v", found, err)
	}
	if found, err := CheckHasCertificate("aaaa"); err != nil || !found {
		t.Fatalf("missed the exact fingerprint: %v, %v", found, err)
	}
}
