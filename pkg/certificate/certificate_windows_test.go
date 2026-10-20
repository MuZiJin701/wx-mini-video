//go:build windows

package certificate

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRootRemovalTargetsBothStoresAndVerifiesResult(t *testing.T) {
	originalDelete, originalFetch := runCertificateDelete, fetchCertificatesForCheck
	t.Cleanup(func() { runCertificateDelete, fetchCertificatesForCheck = originalDelete, originalFetch })
	var calls [][]string
	runCertificateDelete = func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		if args[0] == "-user" {
			return nil, errors.New("already absent")
		}
		return nil, nil
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) { return nil, nil }
	if err := RemoveLegacyRoot(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || strings.Join(calls[0], " ") != "-delstore Root "+LegacyRootThumbprint || strings.Join(calls[1], " ") != "-user -delstore Root "+LegacyRootThumbprint {
		t.Fatalf("unexpected removal targets: %v", calls)
	}
	fetchCertificatesForCheck = func() ([]Certificate, error) { return []Certificate{{Thumbprint: LegacyRootThumbprint}}, nil }
	if err := RemoveLegacyRoot(); err == nil {
		t.Fatal("reported success while shared root remains")
	}
}

func TestManagedPrivateKeyHasProtectedUserACL(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreate(dir); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, filepath.Join(dir, "root-ca.pem")} {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		control, _, err := sd.Control()
		if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatal("private key inherits unrestricted ACL")
		}
		text := sd.String()
		if strings.Count(text, "(A;") != 2 || !strings.Contains(text, user.User.Sid.String()) || !strings.Contains(text, ";;;SY)") {
			t.Fatalf("unexpected access control: %s", text)
		}
	}
}

func TestParseCertutilCertificatesChineseOutput(t *testing.T) {
	output := `Root
================ 证书 2 ================
序列号: 0100000000000000000000000000000000
颁发者: CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN
 NotBefore: 2022-11-04 15:05
 NotAfter: 2122-10-11 15:05
使用者: CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN
 ""

证书哈希(sha1): d70cd039051f77c30673b8209fc15efa650ed52c

CertUtil: -store 命令成功完成。`
	certificates := parseCertutilCertificates(output)
	if len(certificates) != 1 {
		t.Fatalf("len(parseCertutilCertificates()) = %d, want 1: %#v", len(certificates), certificates)
	}
	if certificates[0].Subject.CN != "SunnyNet" {
		t.Fatalf("CN = %q", certificates[0].Subject.CN)
	}
	if certificates[0].Thumbprint != "D70CD039051F77C30673B8209FC15EFA650ED52C" {
		t.Fatalf("Thumbprint = %q", certificates[0].Thumbprint)
	}
}

func TestParseCertificateSubject(t *testing.T) {
	got := parseCertificateSubject("CN=SunnyNet, OU=SunnyNet, O=SunnyNet, L=BeiJing, S=BeiJing, C=CN")
	if got.CN != "SunnyNet" || got.OU != "SunnyNet" || got.O != "SunnyNet" || got.L != "BeiJing" || got.S != "BeiJing" || got.C != "CN" {
		t.Fatalf("parseCertificateSubject() = %#v", got)
	}
}
