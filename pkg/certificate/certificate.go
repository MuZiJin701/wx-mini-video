package certificate

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CertFileAndKeyFile struct {
	Name       string
	Cert       []byte
	PrivateKey []byte
}

const LegacyRootThumbprint = "D70CD039051F77C30673B8209FC15EFA650ED52C"

var fetchCertificatesForCheck = fetchCertificates

func Parse(certPEM, keyPEM []byte) (*CertFileAndKeyFile, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("证书与私钥无效或不匹配: %w", err)
	}
	root, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	if !root.IsCA || !root.BasicConstraintsValid || root.KeyUsage&x509.KeyUsageCertSign == 0 {
		return nil, fmt.Errorf("证书不具备 CA 签发能力")
	}
	if time.Now().Before(root.NotBefore) || time.Now().After(root.NotAfter) {
		return nil, fmt.Errorf("根证书尚未生效或已经过期")
	}
	key, ok := pair.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Echo 代理需要 RSA 根证书私钥")
	}
	var publicPEM []byte
	for _, der := range pair.Certificate {
		publicPEM = append(publicPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	return &CertFileAndKeyFile{Name: root.Subject.CommonName, Cert: publicPEM, PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})}, nil
}

func Thumbprint(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("根证书不是有效的 PEM 证书")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", err
	}
	sum := sha1.Sum(block.Bytes)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

type CertificateSubject struct {
	// label
	CN string
	// cenc
	OU string
	// hpky
	O string
	// hpky
	L string
	// subj
	S string
	// cenc
	C string
}
type Certificate struct {
	Thumbprint string
	Subject    CertificateSubject
}

// 获取所有证书
func FetchCertificates() ([]Certificate, error) {
	return fetchCertificates()
}

func CheckHasCertificate(thumbprint string) (bool, error) {
	certificates, err := fetchCertificatesForCheck()
	if err != nil {
		return false, err
	}
	for _, cert := range certificates {
		if strings.EqualFold(strings.ReplaceAll(cert.Thumbprint, " ", ""), thumbprint) {
			return true, nil
		}
	}
	return false, nil
}

// 安装指定证书
func InstallCertificate(cert_data []byte) error {
	return installCertificate(cert_data)
}

func RemoveLegacyRoot() error {
	cleanupErr := uninstallCertificate(LegacyRootThumbprint)
	found, err := CheckHasCertificate(LegacyRootThumbprint)
	if err != nil {
		return errors.Join(cleanupErr, err)
	}
	if found {
		return errors.Join(cleanupErr, fmt.Errorf("旧共享根证书删除后仍在信任库中"))
	}
	return nil
}
