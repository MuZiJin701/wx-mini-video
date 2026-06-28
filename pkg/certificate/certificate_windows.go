//go:build windows

package certificate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func fetchCertificates() ([]Certificate, error) {
	certificates, err := fetchCertificatesWithPowerShell()
	if err == nil {
		return certificates, nil
	}
	if fallback, fallbackErr := fetchCertificatesWithCertutil(); fallbackErr == nil {
		return fallback, nil
	}
	return nil, err
}

func fetchCertificatesWithPowerShell() ([]Certificate, error) {
	// PowerShell 2.0 compatible command
	cmd := "Get-ChildItem Cert:\\LocalMachine\\Root | ForEach-Object { $_.Thumbprint + \"###\" + $_.Subject }"
	ps := exec.Command("powershell.exe", "-NoProfile", "-Command", cmd)
	output, err2 := ps.CombinedOutput()
	if err2 != nil {
		return nil, fmt.Errorf("获取证书时发生错误，%v, %s\n", err2.Error(), strings.TrimSpace(string(output)))
	}

	var certificates []Certificate
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "###", 2)
		if len(parts) < 2 {
			continue
		}
		thumbprint := parts[0]
		subject_str := parts[1]

		subj := parseCertificateSubject(subject_str)
		certificates = append(certificates, Certificate{
			Thumbprint: thumbprint,
			Subject:    subj,
		})
	}
	return certificates, nil
}

func fetchCertificatesWithCertutil() ([]Certificate, error) {
	cmd := exec.Command("certutil", "-store", "Root")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("certutil 获取证书失败，%v, %s\n", err.Error(), strings.TrimSpace(string(output)))
	}
	return parseCertutilCertificates(string(output)), nil
}

func parseCertificateSubject(subject string) CertificateSubject {
	subj := CertificateSubject{}
	pairs := strings.Split(subject, ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		value := kv[1]
		switch key {
		case "CN":
			subj.CN = value
		case "OU":
			subj.OU = value
		case "O":
			subj.O = value
		case "L":
			subj.L = value
		case "S":
			subj.S = value
		case "C":
			subj.C = value
		}
	}
	return subj
}

func parseCertutilCertificates(output string) []Certificate {
	thumbprintPattern := regexp.MustCompile(`(?i)(?:Cert Hash\(sha1\)|证书哈希\(sha1\)):\s*([0-9a-f ]+)`)
	var certificates []Certificate
	var current *Certificate
	flush := func() {
		if current != nil && (current.Thumbprint != "" || current.Subject.CN != "") {
			certificates = append(certificates, *current)
		}
		current = nil
	}
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Contains(line, "===============") {
			flush()
			current = &Certificate{}
			continue
		}
		if current == nil {
			current = &Certificate{}
		}
		if strings.HasPrefix(line, "Subject:") {
			current.Subject = parseCertificateSubject(strings.TrimSpace(strings.TrimPrefix(line, "Subject:")))
			continue
		}
		if strings.HasPrefix(line, "使用者:") {
			current.Subject = parseCertificateSubject(strings.TrimSpace(strings.TrimPrefix(line, "使用者:")))
			continue
		}
		if match := thumbprintPattern.FindStringSubmatch(line); len(match) == 2 {
			current.Thumbprint = strings.ToUpper(strings.ReplaceAll(match[1], " ", ""))
		}
	}
	flush()
	return certificates
}

func installCertificate(cert_data []byte) error {
	cert_file, err := os.CreateTemp("", "SunnyRoot.cer")
	if err != nil {
		return fmt.Errorf("没有创建证书的权限，%v\n", err.Error())
	}
	defer os.Remove(cert_file.Name())
	if _, err := cert_file.Write(cert_data); err != nil {
		return fmt.Errorf("获取证书失败，%v\n", err.Error())
	}
	if err := cert_file.Close(); err != nil {
		return fmt.Errorf("生成证书失败，%v\n", err.Error())
	}
	// Use certutil for Windows 7 compatibility
	cmd := exec.Command("certutil", "-addstore", "Root", cert_file.Name())
	output, err2 := cmd.CombinedOutput()
	if err2 != nil {
		return fmt.Errorf("安装证书时发生错误，%v\n", string(output))
	}
	return nil
}

func uninstallCertificate(name string) error {
	fmt.Println(name)
	// Remove-Item "Cert:\LocalMachine\Root\D70CD039051F77C30673B8209FC15EFA650ED52C"
	certificates, err := fetchCertificates()
	if err != nil {
		return err
	}
	var matched *Certificate
	for _, cert := range certificates {
		if cert.Subject.CN == name {
			matched = &cert
			break
		}
	}
	if matched == nil {
		return errors.New("没有找到要删除的证书")
	}
	cmd := fmt.Sprintf("Get-ChildItem Cert:\\LocalMachine\\Root\\%v | Remove-Item", matched.Thumbprint)
	ps := exec.Command("powershell.exe", "-NoProfile", "-Command", cmd)
	output, err2 := ps.CombinedOutput()
	if err2 != nil {
		return fmt.Errorf("删除证书时发生错误，%v\n", string(output))
	}
	return nil
}
