package certificate

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"wx_channel/pkg/system"
)

var macCertificateCommand = system.RunMacCommand
var macCertificateAdmin = system.RunMacAdmin

func parseKeychainRoots(data []byte) ([]*x509.Certificate, error) {
	var roots []*x509.Certificate
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析钥匙串证书失败: %w", err)
		}
		if cert.IsCA {
			roots = append(roots, cert)
		}
	}
	return roots, nil
}

func macDefaultKeychain() (string, error) {
	output, err := macCertificateCommand("/usr/bin/security", "default-keychain", "-d", "user")
	if err != nil {
		return "", err
	}
	path := strings.Trim(strings.TrimSpace(string(output)), "\"")
	if path == "" {
		return "", fmt.Errorf("未找到当前用户的默认钥匙串")
	}
	return path, nil
}

func macWithPublicCertificate(data []byte, fn func(string) error) error {
	file, err := os.CreateTemp("", "wx-mini-video-public-*.pem")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return fn(file.Name())
}

func macFetchCertificates() ([]Certificate, error) {
	userKeychain, err := macDefaultKeychain()
	if err != nil {
		return nil, err
	}
	var certificates []Certificate
	seen := make(map[string]bool)
	for _, keychain := range []string{userKeychain, "/Library/Keychains/System.keychain"} {
		output, err := macCertificateCommand("/usr/bin/security", "find-certificate", "-a", "-p", keychain)
		if err != nil {
			if strings.Contains(err.Error(), "The specified item could not be found in the keychain") {
				continue
			}
			return nil, err
		}
		roots, err := parseKeychainRoots(output)
		if err != nil {
			return nil, err
		}
		for _, root := range roots {
			public := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
			fingerprint, err := Thumbprint(public)
			if err != nil {
				return nil, err
			}
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			if err := macWithPublicCertificate(public, func(path string) error {
				_, err := macCertificateCommand("/usr/bin/security", "verify-cert", "-c", path, "-p", "basic", "-L")
				return err
			}); err != nil {
				continue
			}
			certificates = append(certificates, Certificate{Thumbprint: fingerprint, Subject: CertificateSubject{CN: root.Subject.CommonName}})
		}
	}
	return certificates, nil
}

func macInstallCertificate(data []byte) error {
	keychain, err := macDefaultKeychain()
	if err != nil {
		return err
	}
	return macWithPublicCertificate(data, func(path string) error {
		_, err := macCertificateCommand("/usr/bin/security", "add-trusted-cert", "-r", "trustRoot", "-k", keychain, path)
		if err != nil {
			return fmt.Errorf("请在 macOS 提示中允许信任本机根证书；也可在“钥匙串访问”中导入公开证书并核对指纹后设置信任: %w", err)
		}
		return nil
	})
}

func macUninstallCertificate(fingerprint string) error {
	if fingerprint != LegacyRootThumbprint {
		return fmt.Errorf("只允许按固定指纹清理旧共享根证书")
	}
	userKeychain, err := macDefaultKeychain()
	if err != nil {
		return err
	}
	for _, keychain := range []string{userKeychain, "/Library/Keychains/System.keychain"} {
		output, err := macCertificateCommand("/usr/bin/security", "find-certificate", "-a", "-p", keychain)
		if err != nil {
			if strings.Contains(err.Error(), "The specified item could not be found in the keychain") {
				continue
			}
			return err
		}
		roots, err := parseKeychainRoots(output)
		if err != nil {
			return err
		}
		for _, root := range roots {
			public := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
			got, err := Thumbprint(public)
			if err != nil {
				return err
			}
			if got != fingerprint {
				continue
			}
			args := []string{"/usr/bin/security", "delete-certificate", "-t", "-Z", fingerprint, keychain}
			if keychain == userKeychain {
				_, err = macCertificateCommand(args[0], args[1:]...)
			} else {
				err = macCertificateAdmin([][]string{args})
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}
