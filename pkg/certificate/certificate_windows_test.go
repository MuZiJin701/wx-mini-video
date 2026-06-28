//go:build windows

package certificate

import "testing"

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
