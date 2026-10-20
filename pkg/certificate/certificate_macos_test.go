package certificate

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestKeychainRootParsingUsesCertificateDER(t *testing.T) {
	root, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots, err := parseKeychainRoots(append(append([]byte("keychain output\n"), root.Cert...), root.Cert...))
	if err != nil || len(roots) != 2 || !roots[0].IsCA {
		t.Fatalf("roots = %v, %v", roots, err)
	}
	if _, err := parseKeychainRoots([]byte("-----BEGIN CERTIFICATE-----\nYmFk\n-----END CERTIFICATE-----\n")); err == nil {
		t.Fatal("accepted corrupt certificate DER")
	}
}

func TestMacTrustRequiresSuccessfulVerificationAndOnlyWritesPublicCertificates(t *testing.T) {
	trusted, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	untrusted, err := LoadOrCreate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	oldCommand := macCertificateCommand
	t.Cleanup(func() { macCertificateCommand = oldCommand })
	installed := false
	macCertificateCommand = func(name string, args ...string) ([]byte, error) {
		if name != "/usr/bin/security" {
			t.Fatal("unexpected executable")
		}
		switch args[0] {
		case "default-keychain":
			return []byte("  \"/Users/test/Library/Keychains/login.keychain-db\"\n"), nil
		case "find-certificate":
			return append(append([]byte{}, trusted.Cert...), untrusted.Cert...), nil
		case "verify-cert":
			data, err := os.ReadFile(args[2])
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("PRIVATE KEY")) {
				t.Fatal("private key passed to security tool")
			}
			if bytes.Equal(data, trusted.Cert) {
				return nil, nil
			}
			return nil, errors.New("not trusted")
		case "add-trusted-cert":
			data, err := os.ReadFile(args[len(args)-1])
			if err != nil || !bytes.Equal(data, trusted.Cert) || bytes.Contains(data, []byte("PRIVATE KEY")) {
				t.Fatal("invalid public certificate export")
			}
			installed = true
			return nil, nil
		default:
			t.Fatalf("unexpected command %v", args)
		}
		return nil, nil
	}
	certificates, err := macFetchCertificates()
	want, _ := Thumbprint(trusted.Cert)
	if err != nil || len(certificates) != 1 || certificates[0].Thumbprint != want {
		t.Fatalf("trusted certificates = %+v, %v", certificates, err)
	}
	if err := macInstallCertificate(trusted.Cert); err != nil || !installed {
		t.Fatalf("installation = %v, %v", installed, err)
	}
	if err := macUninstallCertificate(want); err == nil {
		t.Fatal("allowed removing a non-legacy root")
	}
}
